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

func migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS public.cdc_heartbeat (id INTEGER PRIMARY KEY, touched_at TIMESTAMPTZ NOT NULL, update_count BIGINT NOT NULL DEFAULT 0)`,
		`INSERT INTO public.cdc_heartbeat (id, touched_at) VALUES (1, clock_timestamp()) ON CONFLICT (id) DO NOTHING`,
		`GRANT SELECT, UPDATE ON public.cdc_heartbeat TO debezium`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("heartbeat migration: %w", err)
		}
	}
	var included bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_publication_tables WHERE pubname='cdc_publication' AND schemaname='public' AND tablename='cdc_heartbeat')`).Scan(&included); err != nil {
		return err
	}
	if !included {
		if _, err := db.ExecContext(ctx, `ALTER PUBLICATION cdc_publication ADD TABLE public.cdc_heartbeat`); err != nil {
			return fmt.Errorf("add heartbeat table to publication: %w", err)
		}
	}
	return nil
}

func heartbeatCount(ctx context.Context, db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, `SELECT update_count FROM public.cdc_heartbeat WHERE id=1`).Scan(&count)
	return count, err
}

func configure(ctx context.Context, scenario string) error {
	config := "application.properties"
	if scenario == "e2-timer" {
		config = "application-e2-timer.properties"
	} else if scenario == "e2" {
		config = "application-e2.properties"
	}
	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d", "--force-recreate", "debezium")
	cmd.Env = append(os.Environ(), "DEBEZIUM_CONFIG="+config)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("configure Debezium for %s: %w: %s", scenario, err, strings.TrimSpace(string(output)))
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

func summary(scenario string, samples []observe.Observation, count int64, loadTime time.Duration, startHB, endHB int64, rate, noiseBytes int, duration, interval, settle time.Duration, pgVersion, images string) (string, error) {
	if len(samples) < 2 {
		return "", errors.New("at least two observations required")
	}
	first, last := samples[0], samples[len(samples)-1]
	firstLSN, err := lsnValue(first.ConfirmedFlushLSN)
	if err != nil {
		return "", err
	}
	lastLSN, err := lsnValue(last.ConfirmedFlushLSN)
	if err != nil {
		return "", err
	}
	maxGap := time.Duration(0)
	minRetained := samples[0].RetainedWALBytes.Int64
	peakRetained := int64(0)
	healthCounts := make(map[string]int)
	allActive := true
	for i, s := range samples {
		if !s.RetainedWALBytes.Valid {
			return "", errors.New("NULL retained-WAL sample")
		}
		if s.RetainedWALBytes.Int64 > peakRetained {
			peakRetained = s.RetainedWALBytes.Int64
		}
		if s.RetainedWALBytes.Int64 < minRetained {
			minRetained = s.RetainedWALBytes.Int64
		}
		healthCounts[s.DebeziumHealth]++
		if !s.Active {
			allActive = false
		}
		if i > 0 {
			gap := s.Timestamp.Sub(samples[i-1].Timestamp)
			if gap <= 0 {
				return "", errors.New("non-increasing sample timestamps")
			}
			if gap > maxGap {
				maxGap = gap
			}
		}
	}
	seconds := last.Timestamp.Sub(first.Timestamp).Seconds()
	if seconds <= 0 {
		return "", errors.New("sample timestamps did not advance")
	}
	flushAdvanced := lastLSN > firstLSN
	retainedDelta := last.RetainedWALBytes.Int64 - first.RetainedWALBytes.Int64
	growthRate := float64(retainedDelta) / seconds
	allHealthy := healthCounts["UP"] == len(samples)
	label, question, setting := scenarioText(scenario)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: noise-only workload\n\n", label)
	fmt.Fprintf(&b, "Question: %s\n\n", question)
	fmt.Fprintln(&b, "## Configuration and versions")
	fmt.Fprintf(&b, "- Workload: %d committed `noise` inserts at %d/second for requested %s (actual %s); payload %d bytes. The runner wrote no `orders` rows.\n", count, rate, duration, loadTime.Round(time.Millisecond), noiseBytes)
	fmt.Fprintf(&b, "- Observer: %d samples at %s; settle %s; largest gap %s (limit %s).\n", len(samples), interval, settle, maxGap.Round(time.Millisecond), 2*interval+2*time.Second)
	fmt.Fprintf(&b, "- PostgreSQL: %s. Go: %s.\n- Compose images:\n", pgVersion, runtime.Version())
	for _, image := range strings.Split(strings.TrimSpace(images), "\n") {
		fmt.Fprintf(&b, "  - `%s`\n", strings.TrimSpace(image))
	}
	fmt.Fprintf(&b, "- Slot: `%s`. Heartbeat setting: %s.\n", first.SlotName, setting)
	fmt.Fprintf(&b, "- Heartbeat table updates: %d.\n\n", endHB-startHB)
	fmt.Fprintln(&b, "## Observations")
	fmt.Fprintf(&b, "- Sample window: %s to %s UTC.\n", first.Timestamp.Format(time.RFC3339), last.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Slot active: %t throughout. Debezium health: `UP` %d/%d samples.", allActive, healthCounts["UP"], len(samples))
	for status, count := range healthCounts {
		if status != "UP" {
			fmt.Fprintf(&b, " `%s` %d/%d.", status, count, len(samples))
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Confirmed flush LSN: `%s` to `%s`; advanced: %t.\n", first.ConfirmedFlushLSN, last.ConfirmedFlushLSN, flushAdvanced)
	fmt.Fprintf(&b, "- Restart LSN: `%s` to `%s`.\n", first.RestartLSN, last.RestartLSN)
	fmt.Fprintf(&b, "- Retained-WAL distance: start %d, end %d, minimum %d, peak %d bytes; net change %d bytes (%.1f bytes/second).\n", first.RetainedWALBytes.Int64, last.RetainedWALBytes.Int64, minRetained, peakRetained, retainedDelta, growthRate)
	fmt.Fprintf(&b, "- `pg_wal` directory size: %d to %d bytes. Checkpoint counter: %d to %d.\n\n", first.PGWALBytes, last.PGWALBytes, first.CheckpointCount, last.CheckpointCount)
	fmt.Fprintln(&b, "## Verdict")
	if maxGap > 2*interval+2*time.Second || !allActive || !allHealthy {
		fmt.Fprintln(&b, "Inconclusive: slot activity, Debezium health, or sample coverage did not meet the experiment's observation requirements. Inspect the CSV and repeat before comparing this run.")
	} else {
		switch scenario {
		case "e1":
			if flushAdvanced {
				fmt.Fprintf(&b, "With heartbeats disabled, the confirmed flush LSN advanced while only unpublished `noise` rows were written. This run did not reproduce a stationary flush position; retained-WAL distance changed by %d bytes.\n", retainedDelta)
			} else {
				fmt.Fprintf(&b, "With heartbeats disabled, the confirmed flush LSN stayed fixed while Debezium remained healthy and active. Retained-WAL distance changed by %d bytes. This run reproduced the quiet-source stall.\n", retainedDelta)
			}
		case "e2-timer":
			if flushAdvanced {
				fmt.Fprintf(&b, "The 10-second timer-only heartbeat coincided with confirmed flush LSN advancement. Retained-WAL distance changed by %d bytes. This run supports timer-only progress under this setup; it does not establish delivery of every event.\n", retainedDelta)
			} else {
				fmt.Fprintf(&b, "The 10-second timer-only heartbeat did not advance the confirmed flush LSN while only unpublished `noise` rows were written. Retained-WAL distance changed by %d bytes. Under this setup, timer-only did not prevent the quiet-source stall.\n", retainedDelta)
			}
		case "e2":
			if flushAdvanced {
				fmt.Fprintf(&b, "The active heartbeat action query accompanied confirmed flush LSN advancement. Retained-WAL distance changed by %d bytes. Compare this trend with the separately reset E1 and timer-only runs; LSN progress alone does not prove sink delivery.\n", retainedDelta)
			} else {
				fmt.Fprintf(&b, "The active heartbeat action query did not advance the confirmed flush LSN in this run. Retained-WAL distance changed by %d bytes; inspect the heartbeat row update count and CSV before drawing a conclusion.\n", retainedDelta)
			}
		}
	}
	return b.String(), nil
}

func scenarioText(scenario string) (label, question, setting string) {
	switch scenario {
	case "e1":
		return "E1: quiet source, no heartbeat", "When only the unpublished noise table changes and heartbeats are disabled, does a healthy Debezium slot continue to advance?", "disabled"
	case "e2-timer":
		return "E2 timer-only: quiet source", "Is heartbeat.interval.ms alone enough to advance the slot when only the unpublished noise table changes?", "heartbeat.interval.ms=10000; no action query"
	default:
		return "E2 active: quiet source", "Does a heartbeat action query that updates a published table keep the slot advancing when only the noise table receives application writes?", "heartbeat.interval.ms=10000 plus published-table action query"
	}
}

func run() error {
	scenario := flag.String("scenario", "", "e1, e2-timer, or e2 (active heartbeat)")
	duration := flag.Duration("duration", 10*time.Minute, "noise load duration")
	interval := flag.Duration("interval", 5*time.Second, "observer interval")
	settle := flag.Duration("settle", 5*time.Second, "settling time")
	rate := flag.Int("rate", 10, "noise writes per second")
	noiseBytes := flag.Int("noise-bytes", 4096, "noise payload bytes")
	dsn := flag.String("dsn", lab.DefaultDSN, "PostgreSQL DSN")
	slot := flag.String("slot", lab.DefaultSlotName, "replication slot")
	healthURL := flag.String("health-url", lab.DefaultHealthURL, "Debezium health URL")
	flag.Parse()
	if *scenario != "e1" && *scenario != "e2-timer" && *scenario != "e2" {
		return errors.New("set -scenario=e1, -scenario=e2-timer, or -scenario=e2")
	}
	if *duration <= 0 || *interval <= 0 || *settle < 0 {
		return errors.New("duration and interval must be positive; settle must be nonnegative")
	}
	cfg := load.Config{Noise: true, Rate: *rate, NoiseBytes: *noiseBytes}
	if err := cfg.Validate(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(3)
	if err := migrate(ctx, db); err != nil {
		return err
	}
	if err := configure(ctx, *scenario); err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	log.Printf("waiting for Debezium %s readiness", *scenario)
	first, err := ready(ctx, db, *slot, *healthURL, client)
	if err != nil {
		return err
	}
	var pgVersion string
	if err := db.QueryRowContext(ctx, `SELECT current_setting('server_version')`).Scan(&pgVersion); err != nil {
		return fmt.Errorf("read PostgreSQL version: %w", err)
	}
	imageOutput, err := exec.CommandContext(ctx, "docker", "compose", "config", "--images").Output()
	if err != nil {
		return fmt.Errorf("read configured Compose images: %w", err)
	}
	startHB, err := heartbeatCount(ctx, db)
	if err != nil {
		return err
	}
	runDir := filepath.Join("results", *scenario, time.Now().UTC().Format("20060102T150405.000Z"))
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
			log.Printf("%s sample %d: flush=%s retained=%d", *scenario, len(samples), o.ConfirmedFlushLSN, o.RetainedWALBytes.Int64)
		}
		return nil
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, *duration)
	defer cancelLoad()
	type loadResult struct {
		counts load.Counts
		err    error
	}
	done := make(chan loadResult, 1)
	start := time.Now()
	go func() { counts, err := load.Run(loadCtx, db, cfg); done <- loadResult{counts, err} }()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	var counts load.Counts
	var loadTime time.Duration
loop:
	for {
		select {
		case <-ctx.Done():
			cancelLoad()
			<-done
			return ctx.Err()
		case result := <-done:
			loadTime = time.Since(start)
			if result.err != nil {
				return result.err
			}
			counts = result.counts
			break loop
		case <-ticker.C:
			if err := sample(); err != nil {
				cancelLoad()
				<-done
				return err
			}
		}
	}
	log.Printf("%s committed %d noise rows; settling %s", *scenario, counts.Noise, *settle)
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
	endHB, err := heartbeatCount(ctx, db)
	if err != nil {
		return err
	}
	report, err := summary(*scenario, samples, counts.Noise, loadTime, startHB, endHB, *rate, *noiseBytes, *duration, *interval, *settle, pgVersion, string(imageOutput))
	if err != nil {
		return err
	}
	path := filepath.Join(runDir, "summary.md")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		return err
	}
	log.Printf("%s results: %s", strings.ToUpper(*scenario), runDir)
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
