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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/local/cdc-crashtest/internal/lab"
	"github.com/local/cdc-crashtest/internal/load"
	"github.com/local/cdc-crashtest/internal/observe"
)

type analysis struct {
	First, Last       observe.Observation
	MinRetained       int64
	MaxRetained       int64
	MinPGWAL          int64
	MaxPGWAL          int64
	HealthCounts      map[string]int
	ActiveSamples     int
	FlushAdvanced     bool
	GrowthBytesPerSec float64
	SegmentBytes      int64
	MaxSampleGap      time.Duration
	AllowedSampleGap  time.Duration
	Pass              bool
}

func lsnValue(lsn string) (uint64, error) {
	parts := strings.Split(lsn, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid LSN %q", lsn)
	}
	hi, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid LSN %q: %w", lsn, err)
	}
	lo, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid LSN %q: %w", lsn, err)
	}
	return hi<<32 | lo, nil
}

func analyze(samples []observe.Observation, segmentBytes int64, interval time.Duration) (analysis, error) {
	if len(samples) < 2 || segmentBytes <= 0 || interval <= 0 {
		return analysis{}, errors.New("E0 needs at least two samples, a positive WAL segment size, and a positive sample interval")
	}
	a := analysis{
		First:            samples[0],
		Last:             samples[len(samples)-1],
		MinRetained:      samples[0].RetainedWALBytes.Int64,
		MaxRetained:      samples[0].RetainedWALBytes.Int64,
		MinPGWAL:         samples[0].PGWALBytes,
		MaxPGWAL:         samples[0].PGWALBytes,
		HealthCounts:     make(map[string]int),
		SegmentBytes:     segmentBytes,
		AllowedSampleGap: 2*interval + 2*time.Second,
	}
	firstLSN, err := lsnValue(a.First.ConfirmedFlushLSN)
	if err != nil {
		return a, err
	}
	lastLSN, err := lsnValue(a.Last.ConfirmedFlushLSN)
	if err != nil {
		return a, err
	}
	for index, o := range samples {
		if index > 0 {
			gap := o.Timestamp.Sub(samples[index-1].Timestamp)
			if gap <= 0 {
				return a, errors.New("sample timestamps did not advance")
			}
			if gap > a.MaxSampleGap {
				a.MaxSampleGap = gap
			}
		}
		if !o.RetainedWALBytes.Valid {
			return a, errors.New("retained WAL is NULL in one or more samples")
		}
		if o.RetainedWALBytes.Int64 < a.MinRetained {
			a.MinRetained = o.RetainedWALBytes.Int64
		}
		if o.RetainedWALBytes.Int64 > a.MaxRetained {
			a.MaxRetained = o.RetainedWALBytes.Int64
		}
		if o.PGWALBytes < a.MinPGWAL {
			a.MinPGWAL = o.PGWALBytes
		}
		if o.PGWALBytes > a.MaxPGWAL {
			a.MaxPGWAL = o.PGWALBytes
		}
		a.HealthCounts[o.DebeziumHealth]++
		if o.Active {
			a.ActiveSamples++
		}
	}
	seconds := a.Last.Timestamp.Sub(a.First.Timestamp).Seconds()
	if seconds <= 0 {
		return a, errors.New("sample timestamps did not advance")
	}
	a.FlushAdvanced = lastLSN > firstLSN
	a.GrowthBytesPerSec = float64(a.Last.RetainedWALBytes.Int64-a.First.RetainedWALBytes.Int64) / seconds
	a.Pass = a.FlushAdvanced && a.MaxRetained <= segmentBytes && a.MaxSampleGap <= a.AllowedSampleGap &&
		a.ActiveSamples == len(samples) && a.HealthCounts["UP"] == len(samples)
	return a, nil
}

func formatMiB(bytes int64) string {
	return fmt.Sprintf("%d bytes (%.2f MiB)", bytes, float64(bytes)/(1024*1024))
}

func renderSummary(samples []observe.Observation, a analysis, counts load.Counts, duration, actualLoadTime, interval, settle time.Duration, rate int, pgVersion, images string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# E0 baseline: orders only")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Question: with Debezium consuming continuous `orders` changes, does the slot advance while retained WAL stays small?")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Configuration and versions")
	fmt.Fprintf(&b, "- Load: `public.orders` only, %d writes/second, requested duration %s; %d committed rows.\n", rate, duration, counts.Orders)
	fmt.Fprintf(&b, "- Actual load wall time: %s; observed average %.2f committed rows/second.\n", actualLoadTime.Round(time.Millisecond), float64(counts.Orders)/actualLoadTime.Seconds())
	fmt.Fprintf(&b, "- Observer: %d samples, %s interval, %s settling time, slot `%s`.\n", len(samples), interval, settle, a.First.SlotName)
	fmt.Fprintf(&b, "- PostgreSQL runtime version: %s.\n", pgVersion)
	fmt.Fprintf(&b, "- Go runtime version: %s.\n", runtime.Version())
	fmt.Fprintln(&b, "- Configured Compose images:")
	for _, image := range strings.Split(strings.TrimSpace(images), "\n") {
		fmt.Fprintf(&b, "  - `%s`\n", strings.TrimSpace(image))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Observations")
	fmt.Fprintf(&b, "- Sample window: %s to %s (UTC).\n", a.First.Timestamp.Format(time.RFC3339), a.Last.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Confirmed flush LSN: `%s` to `%s`; advanced: %t.\n", a.First.ConfirmedFlushLSN, a.Last.ConfirmedFlushLSN, a.FlushAdvanced)
	fmt.Fprintf(&b, "- Restart LSN: `%s` to `%s`.\n", a.First.RestartLSN, a.Last.RestartLSN)
	fmt.Fprintf(&b, "- Retained-WAL distance: start %s; end %s; minimum %s; peak %s.\n",
		formatMiB(a.First.RetainedWALBytes.Int64), formatMiB(a.Last.RetainedWALBytes.Int64),
		formatMiB(a.MinRetained), formatMiB(a.MaxRetained))
	fmt.Fprintf(&b, "- Net retained-WAL growth rate: %.2f bytes/second over the sample window.\n", a.GrowthBytesPerSec)
	fmt.Fprintf(&b, "- `pg_wal` directory size: start %s; end %s; minimum %s; peak %s.\n",
		formatMiB(a.First.PGWALBytes), formatMiB(a.Last.PGWALBytes), formatMiB(a.MinPGWAL), formatMiB(a.MaxPGWAL))
	fmt.Fprintf(&b, "- Active slot: %d/%d samples. Debezium health: ", a.ActiveSamples, len(samples))
	statuses := make([]string, 0, len(a.HealthCounts))
	for status := range a.HealthCounts {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for i, status := range statuses {
		if i > 0 {
			fmt.Fprint(&b, ", ")
		}
		fmt.Fprintf(&b, "`%s` %d/%d", status, a.HealthCounts[status], len(samples))
	}
	fmt.Fprintln(&b, ".")
	fmt.Fprintf(&b, "- Cumulative checkpoint count: %d to %d.\n", a.First.CheckpointCount, a.Last.CheckpointCount)
	fmt.Fprintf(&b, "- Largest gap between samples: %s; coverage rule allows at most %s.\n", a.MaxSampleGap.Round(time.Millisecond), a.AllowedSampleGap)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Verdict")
	if a.Pass {
		fmt.Fprintf(&b, "E0 expected behavior observed: the slot confirmed newer WAL, remained active and healthy, and peak retained-WAL distance stayed within one WAL segment (%s).\n", formatMiB(a.SegmentBytes))
	} else if a.MaxSampleGap > a.AllowedSampleGap {
		fmt.Fprintf(&b, "E0 is inconclusive: the %.1f-second sampling gap exceeds the %s coverage limit. Slot activity, health, and WAL retention during that gap are unknown. Repeat E0 before continuing to E1.\n", a.MaxSampleGap.Seconds(), a.AllowedSampleGap)
	} else {
		fmt.Fprintf(&b, "E0 baseline criterion was not met. It requires confirmed flush LSN to advance, every sample to show an active slot and `UP` health, and peak retained-WAL distance at or below one WAL segment (%s). Inspect the CSV before continuing to E1.\n", formatMiB(a.SegmentBytes))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This one-segment bound is a local experiment rule, not a PostgreSQL safety limit. LSN distance measures WAL progress from the slot's oldest needed position; `pg_wal` size measures files on disk. Neither metric proves that every event reached the receiver.")
	return b.String()
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func readySample(ctx context.Context, db *sql.DB, slot, healthURL string, client *http.Client) (observe.Observation, error) {
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	var lastProblem string
	for {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		o, err := observe.Sample(queryCtx, db, slot, healthURL, client)
		cancel()
		if err == nil && o.Active && o.DebeziumHealth == "UP" && o.RetainedWALBytes.Valid && o.ConfirmedFlushLSN != "" {
			return o, nil
		}
		if err != nil {
			lastProblem = err.Error()
		} else {
			lastProblem = fmt.Sprintf("active=%t health=%s restart_lsn=%s confirmed_flush_lsn=%s", o.Active, o.DebeziumHealth, o.RestartLSN, o.ConfirmedFlushLSN)
		}
		select {
		case <-ctx.Done():
			return observe.Observation{}, ctx.Err()
		case <-deadline.C:
			return observe.Observation{}, fmt.Errorf("E0 preflight did not find a ready slot and Debezium health within 60s: %s", lastProblem)
		case <-time.After(2 * time.Second):
		}
	}
}

func run() error {
	duration := flag.Duration("duration", 10*time.Minute, "orders load duration")
	interval := flag.Duration("interval", 5*time.Second, "observer sample interval")
	settle := flag.Duration("settle", 5*time.Second, "wait after load before final sample")
	rate := flag.Int("rate", 10, "orders inserts per second")
	slot := flag.String("slot", lab.DefaultSlotName, "replication slot name")
	dsn := flag.String("dsn", envOr("DATABASE_URL", lab.DefaultDSN), "PostgreSQL connection string (or set DATABASE_URL)")
	healthURL := flag.String("health-url", lab.DefaultHealthURL, "Debezium health URL")
	resultsRoot := flag.String("results-root", "results/e0", "directory containing timestamped E0 runs")
	flag.Parse()
	if *duration <= 0 || *interval <= 0 || *settle < 0 {
		return errors.New("duration and interval must be positive; settle must be nonnegative")
	}
	cfg := load.Config{Orders: true, Rate: *rate, NoiseBytes: 4096}
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	client := &http.Client{Timeout: 3 * time.Second}
	log.Print("waiting for active slot and Debezium health")
	first, err := readySample(ctx, db, *slot, *healthURL, client)
	if err != nil {
		return err
	}
	var pgVersion string
	var segmentBytes int64
	if err := db.QueryRowContext(ctx, "SELECT current_setting('server_version'), pg_size_bytes(current_setting('wal_segment_size'))").Scan(&pgVersion, &segmentBytes); err != nil {
		return fmt.Errorf("read PostgreSQL version and WAL segment size: %w", err)
	}
	imageOutput, err := exec.CommandContext(ctx, "docker", "compose", "config", "--images").Output()
	if err != nil {
		return fmt.Errorf("read configured Compose images: %w", err)
	}
	runDir := filepath.Join(*resultsRoot, time.Now().UTC().Format("20060102T150405.000Z"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create E0 result directory: %w", err)
	}
	csvPath := filepath.Join(runDir, "observations.csv")
	writer, err := observe.OpenCSV(csvPath)
	if err != nil {
		return err
	}
	defer writer.Close()
	samples := []observe.Observation{first}
	if err := writer.Write(first); err != nil {
		return fmt.Errorf("write first E0 sample: %w", err)
	}
	log.Printf("E0 started: %s", runDir)

	loadCtx, cancelLoad := context.WithTimeout(ctx, *duration)
	defer cancelLoad()
	loadStart := time.Now()
	type loadResult struct {
		counts load.Counts
		err    error
	}
	loadDone := make(chan loadResult, 1)
	go func() {
		counts, err := load.Run(loadCtx, db, cfg)
		loadDone <- loadResult{counts, err}
	}()
	sampleOnce := func() error {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		o, err := observe.Sample(queryCtx, db, *slot, *healthURL, client)
		if err != nil {
			return err
		}
		if err := writer.Write(o); err != nil {
			return err
		}
		samples = append(samples, o)
		if len(samples)%12 == 0 {
			log.Printf("sample %d: confirmed_flush_lsn=%s retained_wal_bytes=%s health=%s", len(samples), o.ConfirmedFlushLSN, o.Record()[6], o.DebeziumHealth)
		}
		return nil
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	var counts load.Counts
	var actualLoadTime time.Duration
loading:
	for {
		select {
		case <-ctx.Done():
			cancelLoad()
			<-loadDone
			return ctx.Err()
		case result := <-loadDone:
			actualLoadTime = time.Since(loadStart)
			if result.err != nil {
				return result.err
			}
			counts = result.counts
			break loading
		case <-ticker.C:
			if err := sampleOnce(); err != nil {
				cancelLoad()
				<-loadDone
				return fmt.Errorf("sample E0: %w", err)
			}
		}
	}
	log.Printf("orders load finished: %d rows; waiting %s for the final sample", counts.Orders, *settle)
	if *settle > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(*settle):
		}
	}
	if err := sampleOnce(); err != nil {
		return fmt.Errorf("final E0 sample: %w", err)
	}
	a, err := analyze(samples, segmentBytes, *interval)
	if err != nil {
		return fmt.Errorf("analyze E0 samples in %s: %w", csvPath, err)
	}
	summaryPath := filepath.Join(runDir, "summary.md")
	summary := renderSummary(samples, a, counts, *duration, actualLoadTime, *interval, *settle, *rate, pgVersion, string(imageOutput))
	if err := os.WriteFile(summaryPath, []byte(summary), 0o644); err != nil {
		return fmt.Errorf("write E0 summary: %w", err)
	}
	log.Printf("E0 results: %s and %s", csvPath, summaryPath)
	if !a.Pass {
		return fmt.Errorf("E0 baseline criterion was not met; read %s", summaryPath)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
