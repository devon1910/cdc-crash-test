package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/local/cdc-crashtest/internal/lab"
	"github.com/local/cdc-crashtest/internal/load"
	"github.com/local/cdc-crashtest/internal/observe"
)

type stopResult struct {
	completed time.Time
	err       error
}

func compose(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Env = append(os.Environ(), "DEBEZIUM_CONFIG=application.properties")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ready(ctx context.Context, db *sql.DB, slot, healthURL string, client *http.Client) (observe.Observation, error) {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	var last string
	for {
		sampleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		o, err := observe.Sample(sampleCtx, db, slot, healthURL, client)
		cancel()
		if err == nil && o.Active && o.DebeziumHealth == "UP" && o.RetainedWALBytes.Valid && o.ConfirmedFlushLSN != "" {
			return o, nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = fmt.Sprintf("active=%t health=%s", o.Active, o.DebeziumHealth)
		}
		select {
		case <-ctx.Done():
			return o, ctx.Err()
		case <-deadline.C:
			return o, fmt.Errorf("Debezium not ready within 90s: %s", last)
		case <-time.After(2 * time.Second):
		}
	}
}

func lsnValue(lsn string) (uint64, error) {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid LSN %q", lsn)
	}
	hi, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return 0, err
	}
	lo, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return 0, err
	}
	return hi<<32 | lo, nil
}

func firstAfter(samples []observe.Observation, t time.Time, predicate func(observe.Observation) bool) *observe.Observation {
	for i := range samples {
		if !samples[i].Timestamp.Before(t) && predicate(samples[i]) {
			return &samples[i]
		}
	}
	return nil
}

func lastBefore(samples []observe.Observation, t time.Time) *observe.Observation {
	for i := len(samples) - 1; i >= 0; i-- {
		if samples[i].Timestamp.Before(t) {
			return &samples[i]
		}
	}
	return nil
}

func report(samples []observe.Observation, counts load.Counts, stopRequested, stopCompleted time.Time, duration, interval, settle, actualLoad time.Duration, rate, noiseBytes int, pgVersion, images string) (string, error) {
	if len(samples) < 2 || stopRequested.IsZero() || stopCompleted.IsZero() {
		return "", errors.New("E3 needs samples and a completed stop event")
	}
	first := samples[0]
	last := samples[len(samples)-1]
	pre := lastBefore(samples, stopRequested)
	if pre == nil || !pre.RetainedWALBytes.Valid {
		return "", errors.New("no valid sample before stop")
	}
	maxGap := time.Duration(0)
	peak := int64(0)
	for i, s := range samples {
		if !s.RetainedWALBytes.Valid {
			return "", errors.New("NULL retained-WAL sample")
		}
		if s.RetainedWALBytes.Int64 > peak {
			peak = s.RetainedWALBytes.Int64
		}
		if i > 0 {
			gap := s.Timestamp.Sub(samples[i-1].Timestamp)
			if gap <= 0 {
				return "", errors.New("non-increasing timestamps")
			}
			if gap > maxGap {
				maxGap = gap
			}
		}
	}
	firstInactive := firstAfter(samples, stopCompleted, func(s observe.Observation) bool { return !s.Active })
	firstUnhealthy := firstAfter(samples, stopCompleted, func(s observe.Observation) bool { return s.DebeziumHealth != "UP" })
	post := firstAfter(samples, stopCompleted, func(s observe.Observation) bool { return true })
	var firstGrowth *observe.Observation
	if post != nil {
		firstGrowth = firstAfter(samples, post.Timestamp.Add(time.Nanosecond), func(s observe.Observation) bool {
			return s.RetainedWALBytes.Valid && s.RetainedWALBytes.Int64 > post.RetainedWALBytes.Int64
		})
	}
	if !last.Timestamp.After(stopCompleted) {
		return "", errors.New("no measured time after stop")
	}
	var growthRate float64
	if post != nil && last.Timestamp.After(post.Timestamp) {
		growthRate = float64(last.RetainedWALBytes.Int64-post.RetainedWALBytes.Int64) / last.Timestamp.Sub(post.Timestamp).Seconds()
	}
	healthBefore := make(map[string]int)
	healthAfter := make(map[string]int)
	for _, s := range samples {
		status := "not UP"
		if s.DebeziumHealth == "UP" {
			status = "UP"
		}
		if s.Timestamp.Before(stopRequested) {
			healthBefore[status]++
		} else {
			healthAfter[status]++
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b, "# E3: connector stopped while writes continue")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Question: how soon after Debezium stops do slot inactivity, health failure, and retained-WAL growth appear?")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Workload: `orders` and `noise`, each requested at %d inserts/second for %s, %d-byte noise payload; committed %d orders and %d noise rows (actual load %s).\n", rate, duration, noiseBytes, counts.Orders, counts.Noise, actualLoad.Round(time.Millisecond))
	fmt.Fprintf(&b, "- Observer: %d samples at %s interval plus %s settle; largest sample gap %s (coverage limit %s).\n", len(samples), interval, settle, maxGap.Round(time.Millisecond), 2*interval+2*time.Second)
	fmt.Fprintf(&b, "- Slot: `%s`. Debezium config: heartbeat disabled.\n", first.SlotName)
	fmt.Fprintf(&b, "- Versions: PostgreSQL %s; Go %s; Compose images: %s.\n", pgVersion, runtime.Version(), strings.Join(strings.Fields(images), ", "))
	fmt.Fprintf(&b, "- Stop requested at %s UTC, completed at %s UTC (%s).\n", stopRequested.UTC().Format(time.RFC3339Nano), stopCompleted.UTC().Format(time.RFC3339Nano), stopCompleted.Sub(stopRequested).Round(time.Millisecond))
	fmt.Fprintf(&b, "- Last pre-stop sample at %s UTC: active=%t, health=`%s`, flush=`%s`, retained=%d bytes.\n", pre.Timestamp.Format(time.RFC3339Nano), pre.Active, pre.DebeziumHealth, pre.ConfirmedFlushLSN, pre.RetainedWALBytes.Int64)
	writeTransition := func(label string, s *observe.Observation) {
		if s == nil {
			fmt.Fprintf(&b, "- %s: not observed.\n", label)
			return
		}
		fmt.Fprintf(&b, "- %s: first sampled at %s UTC (%s after stop completed); active=%t, health=`%s`, retained=%d bytes.\n", label, s.Timestamp.Format(time.RFC3339Nano), s.Timestamp.Sub(stopCompleted).Round(time.Millisecond), s.Active, s.DebeziumHealth, s.RetainedWALBytes.Int64)
	}
	writeTransition("Inactive slot after completed stop", firstInactive)
	writeTransition("Health not UP after completed stop", firstUnhealthy)
	writeTransition("First completed-stop sample (growth baseline)", post)
	writeTransition("Retained WAL above completed-stop baseline", firstGrowth)
	if post != nil {
		fmt.Fprintf(&b, "- Confirmed flush LSN: pre-stop `%s`, first completed-stop sample `%s`, final `%s`. Restart LSN: pre-stop `%s`, first completed-stop sample `%s`, final `%s`.\n", pre.ConfirmedFlushLSN, post.ConfirmedFlushLSN, last.ConfirmedFlushLSN, pre.RestartLSN, post.RestartLSN, last.RestartLSN)
	} else {
		fmt.Fprintf(&b, "- Confirmed flush LSN: pre-stop `%s`, final `%s`. Restart LSN: pre-stop `%s`, final `%s`.\n", pre.ConfirmedFlushLSN, last.ConfirmedFlushLSN, pre.RestartLSN, last.RestartLSN)
	}
	fmt.Fprintf(&b, "- Retained-WAL distance: start %d, pre-stop %d, end %d, peak %d bytes; net growth rate after first completed-stop sample %.1f bytes/second.\n", first.RetainedWALBytes.Int64, pre.RetainedWALBytes.Int64, last.RetainedWALBytes.Int64, peak, growthRate)
	fmt.Fprintf(&b, "- `pg_wal` directory size: start %d, end %d bytes. Final health: `%s`.\n", first.PGWALBytes, last.PGWALBytes, last.DebeziumHealth)
	fmt.Fprintf(&b, "- Health samples before stop request: UP %d, not UP %d; from stop request through shutdown and stopped period: UP %d, not UP %d.\n", healthBefore["UP"], healthBefore["not UP"], healthAfter["UP"], healthAfter["not UP"])
	fmt.Fprintln(&b)
	if maxGap > 2*interval+2*time.Second || firstInactive == nil || firstUnhealthy == nil || firstGrowth == nil {
		fmt.Fprintln(&b, "Verdict: incomplete or inconclusive. Inspect the CSV for sample gaps or missing transitions before claiming timing.")
	} else {
		fmt.Fprintln(&b, "Verdict: after Debezium was stopped, the first observed slot inactivity, health failure, and post-stop retained-WAL increase occurred at the times above. Each is bounded by sampling cadence; the exact instant between samples is unknown. This demonstrates source-side retention pressure, not disk exhaustion or lost events.")
	}
	fmt.Fprintln(&b, "The runner restarts Debezium after recording the stopped-state observations. That recovery is outside this E3 measurement window.")
	return b.String(), nil
}

func run() (runErr error) {
	duration := flag.Duration("duration", 10*time.Minute, "total load duration")
	interval := flag.Duration("interval", 5*time.Second, "observer interval")
	settle := flag.Duration("settle", 5*time.Second, "wait after load before final sample")
	rate := flag.Int("rate", 10, "writes per second to each table")
	noiseBytes := flag.Int("noise-bytes", 4096, "noise payload bytes")
	dsn := flag.String("dsn", lab.DefaultDSN, "PostgreSQL DSN")
	slot := flag.String("slot", lab.DefaultSlotName, "replication slot")
	healthURL := flag.String("health-url", lab.DefaultHealthURL, "Debezium health URL")
	flag.Parse()
	if *interval <= 0 || *duration < 4**interval || *settle < 0 {
		return errors.New("duration must be at least four sample intervals; interval positive; settle nonnegative")
	}
	cfg := load.Config{Orders: true, Noise: true, Rate: *rate, NoiseBytes: *noiseBytes}
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	if err := compose(ctx, "up", "-d", "--force-recreate", "debezium"); err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			return
		}
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer restoreCancel()
		if err := compose(restoreCtx, "up", "-d", "debezium"); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("restore Debezium: %w", err))
		} else {
			log.Print("Debezium restarted after E3")
		}
	}()
	client := &http.Client{Timeout: 3 * time.Second}
	first, err := ready(ctx, db, *slot, *healthURL, client)
	if err != nil {
		return err
	}
	var pgVersion string
	if err := db.QueryRowContext(ctx, `SELECT current_setting('server_version')`).Scan(&pgVersion); err != nil {
		return err
	}
	imageOutput, err := exec.CommandContext(ctx, "docker", "compose", "config", "--images").Output()
	if err != nil {
		return fmt.Errorf("read Compose images: %w", err)
	}
	runDir := filepath.Join("results", "e3", time.Now().UTC().Format("20060102T150405.000Z"))
	writer, err := observe.OpenCSV(filepath.Join(runDir, "observations.csv"))
	if err != nil {
		return err
	}
	defer writer.Close()
	samples := []observe.Observation{first}
	if err := writer.Write(first); err != nil {
		return err
	}
	sample := func() error {
		sampleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		o, err := observe.Sample(sampleCtx, db, *slot, *healthURL, client)
		if err != nil {
			return err
		}
		if err := writer.Write(o); err != nil {
			return err
		}
		samples = append(samples, o)
		if len(samples)%12 == 0 {
			log.Printf("E3 sample %d: active=%t health=%s flush=%s retained=%d", len(samples), o.Active, o.DebeziumHealth, o.ConfirmedFlushLSN, o.RetainedWALBytes.Int64)
		}
		return nil
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, *duration)
	defer cancelLoad()
	type loadResult struct {
		counts load.Counts
		err    error
	}
	loadDone := make(chan loadResult, 1)
	start := time.Now()
	go func() { counts, err := load.Run(loadCtx, db, cfg); loadDone <- loadResult{counts, err} }()
	stopTimer := time.NewTimer(*duration / 2)
	defer stopTimer.Stop()
	stopDone := make(chan stopResult, 1)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	var stopRequested, stopCompleted time.Time
	var counts load.Counts
	var actualLoad time.Duration
	loaded := false
	for !loaded || stopCompleted.IsZero() {
		select {
		case <-ctx.Done():
			cancelLoad()
			if !loaded {
				<-loadDone
			}
			return ctx.Err()
		case <-stopTimer.C:
			stopRequested = time.Now().UTC()
			stopped = true
			log.Printf("E3 stopping Debezium at %s", stopRequested.Format(time.RFC3339Nano))
			go func() {
				err := compose(ctx, "stop", "debezium")
				stopDone <- stopResult{completed: time.Now().UTC(), err: err}
			}()
		case result := <-stopDone:
			stopCompleted = result.completed
			if result.err != nil {
				cancelLoad()
				if !loaded {
					<-loadDone
				}
				return result.err
			}
			log.Printf("E3 Debezium stopped at %s", stopCompleted.Format(time.RFC3339Nano))
		case result := <-loadDone:
			loaded = true
			actualLoad = time.Since(start)
			if result.err != nil {
				return result.err
			}
			counts = result.counts
		case <-ticker.C:
			if err := sample(); err != nil {
				cancelLoad()
				if !loaded {
					<-loadDone
				}
				return err
			}
		}
	}
	log.Printf("E3 committed %d orders and %d noise rows; settling %s", counts.Orders, counts.Noise, *settle)
	if *settle > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(*settle):
		}
	}
	if err := sample(); err != nil {
		return err
	}
	summary, err := report(samples, counts, stopRequested, stopCompleted, *duration, *interval, *settle, actualLoad, *rate, *noiseBytes, pgVersion, string(imageOutput))
	if err != nil {
		return err
	}
	path := filepath.Join(runDir, "summary.md")
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		return err
	}
	log.Printf("E3 results: %s", runDir)
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
