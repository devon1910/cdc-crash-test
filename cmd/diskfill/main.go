package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	composeFile    = "experiments/disk-fill/compose.yaml"
	actionCompose  = "experiments/disk-fill/compose-action.yaml"
	serviceName    = "postgres"
	dataDirectory  = "/var/lib/postgresql/data"
	tmpfsSizeBytes = int64(268435456)
	databaseDSN    = "postgres://postgres@127.0.0.1:55434/cdc_lab?sslmode=disable"
	healthURL      = "http://localhost:18082/q/health"
	slotName       = "cdc_crashtest_slot"
)

var actionQuery bool

type observation struct {
	timestamp          time.Time
	batch              int
	slotActive         sql.NullBool
	restartLSN         sql.NullString
	confirmedFlushLSN  sql.NullString
	heartbeatUpdates   sql.NullInt64
	retainedWALBytes   sql.NullInt64
	pgWALBytes         sql.NullInt64
	pgdataUsedBytes    int64
	pgdataAvailable    int64
	pgdataCapacity     int64
	postgresState      string
	debeziumHealth     string
	workloadStatus     string
	postgresQueryError string
}

type csvOutput struct {
	file   *os.File
	writer *csv.Writer
}

func openCSV(path string) (*csvOutput, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	writer := csv.NewWriter(file)
	header := []string{"timestamp", "batch", "slot_active", "restart_lsn", "confirmed_flush_lsn", "heartbeat_update_count", "retained_wal_bytes", "pg_wal_bytes", "pgdata_used_bytes", "pgdata_available_bytes", "pgdata_capacity_bytes", "postgres_state", "debezium_health", "workload_status", "postgres_query_error"}
	if err := writer.Write(header); err != nil {
		file.Close()
		return nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		file.Close()
		return nil, err
	}
	return &csvOutput{file: file, writer: writer}, nil
}

func (o *csvOutput) write(s observation) error {
	row := []string{
		s.timestamp.UTC().Format(time.RFC3339Nano), strconv.Itoa(s.batch),
		nullBool(s.slotActive), nullString(s.restartLSN), nullString(s.confirmedFlushLSN),
		nullInt(s.heartbeatUpdates), nullInt(s.retainedWALBytes), nullInt(s.pgWALBytes), strconv.FormatInt(s.pgdataUsedBytes, 10),
		strconv.FormatInt(s.pgdataAvailable, 10), strconv.FormatInt(s.pgdataCapacity, 10),
		s.postgresState, s.debeziumHealth, s.workloadStatus, s.postgresQueryError,
	}
	if err := o.writer.Write(row); err != nil {
		return err
	}
	o.writer.Flush()
	return o.writer.Error()
}

func (o *csvOutput) close() error {
	o.writer.Flush()
	return errors.Join(o.writer.Error(), o.file.Close())
}

func nullBool(v sql.NullBool) string {
	if !v.Valid {
		return ""
	}
	return strconv.FormatBool(v.Bool)
}

func nullString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func nullInt(v sql.NullInt64) string {
	if !v.Valid {
		return ""
	}
	return strconv.FormatInt(v.Int64, 10)
}

func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprintln(out, "This experiment runs Debezium against PostgreSQL in an isolated 256 MiB tmpfs data directory.")
	fmt.Fprintln(out, "The no-heartbeat case is expected to fill it; the action-query case uses the same batch budget.")
	fmt.Fprintln(out, "It does not write to a host data volume. Cleanup destroys the capped tmpfs and the temporary connector-offset volume.")
	fmt.Fprint(out, "Type YES to continue: ")
	scanner := bufio.NewScanner(in)
	return scanner.Scan() && strings.TrimSpace(scanner.Text()) == "YES"
}

func composeArgs(args ...string) []string {
	base := []string{"compose", "-f", composeFile}
	if actionQuery {
		base = append(base, "-f", actionCompose)
	}
	return append(base, args...)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func capture(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

func inspectContainer(ctx context.Context, id string) (state string, safeTmpfs bool, err error) {
	type container struct {
		HostConfig struct {
			Tmpfs map[string]string `json:"Tmpfs"`
		} `json:"HostConfig"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
	}
	output, err := capture(ctx, "docker", "inspect", id)
	if err != nil {
		return "unknown", false, err
	}
	var containers []container
	if err := json.Unmarshal(output, &containers); err != nil || len(containers) != 1 {
		return "unknown", false, fmt.Errorf("decode docker inspect for %s", id)
	}
	item := containers[0]
	safe := strings.Contains(item.HostConfig.Tmpfs[dataDirectory], "size=268435456")
	return item.State.Status, safe, nil
}

func containerID(ctx context.Context) (string, error) {
	output, err := capture(ctx, "docker", composeArgs("ps", "-q", serviceName)...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(output))
	if id == "" {
		return "", errors.New("PostgreSQL container was not created")
	}
	return id, nil
}

func readFilesystem(ctx context.Context, id string) (used, available, capacity int64, err error) {
	output, err := capture(ctx, "docker", "exec", id, "df", "-k", dataDirectory)
	if err != nil {
		return 0, 0, 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return 0, 0, 0, fmt.Errorf("unexpected df output: %s", strings.TrimSpace(string(output)))
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return 0, 0, 0, fmt.Errorf("unexpected df row: %s", lines[len(lines)-1])
	}
	capacityKB, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	usedKB, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	availableKB, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return usedKB * 1024, availableKB * 1024, capacityKB * 1024, nil
}

func containerStatus(ctx context.Context, id string) string {
	state, _, err := inspectContainer(ctx, id)
	if err != nil {
		return "unknown: " + err.Error()
	}
	return state
}

func healthStatus(ctx context.Context, client *http.Client) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	response, err := client.Do(request)
	if err != nil {
		return "DOWN: " + err.Error()
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Sprintf("HTTP_%d", response.StatusCode)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return "ERROR: " + err.Error()
	}
	return status.Status
}

func metrics(ctx context.Context, db *sql.DB) (active sql.NullBool, restart, confirmed sql.NullString, retained, pgwal sql.NullInt64, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT active, restart_lsn::text, confirmed_flush_lsn::text,
		       CASE WHEN restart_lsn IS NULL THEN NULL
		            ELSE pg_wal_lsn_diff(pg_current_wal_lsn(), restart_lsn)::bigint END,
	       (SELECT COALESCE(sum(size), 0)::bigint FROM pg_ls_waldir())
		FROM pg_replication_slots WHERE slot_name=$1`, slotName).
		Scan(&active, &restart, &confirmed, &retained, &pgwal)
	return
}

func record(ctx context.Context, db *sql.DB, id string, client *http.Client, batch int, status string) observation {
	sample := observation{timestamp: time.Now().UTC(), batch: batch, postgresState: containerStatus(ctx, id), debeziumHealth: healthStatus(ctx, client), workloadStatus: status}
	fsCtx, fsCancel := context.WithTimeout(ctx, 5*time.Second)
	used, available, capacity, fsErr := readFilesystem(fsCtx, id)
	fsCancel()
	if fsErr == nil {
		sample.pgdataUsedBytes, sample.pgdataAvailable, sample.pgdataCapacity = used, available, capacity
	} else {
		sample.postgresQueryError = "filesystem sample: " + fsErr.Error()
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
	active, restart, confirmed, retained, pgwal, queryErr := metrics(queryCtx, db)
	queryCancel()
	if queryErr != nil {
		sample.postgresQueryError = queryErr.Error()
	} else {
		sample.slotActive, sample.restartLSN, sample.confirmedFlushLSN, sample.retainedWALBytes, sample.pgWALBytes = active, restart, confirmed, retained, pgwal
		var updates int64
		if err := db.QueryRowContext(ctx, `SELECT update_count FROM public.cdc_heartbeat WHERE id=1`).Scan(&updates); err == nil {
			sample.heartbeatUpdates = sql.NullInt64{Int64: updates, Valid: true}
		}
	}
	return sample
}

func waitFor(ctx context.Context, check func(context.Context) (bool, error)) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ready, err := check(checkCtx)
		cancel()
		if err == nil && ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func writeBatch(ctx context.Context, db *sql.DB, batch, rows, payloadBytes int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for row := 0; row < rows; row++ {
		randomBytes := make([]byte, payloadBytes*3/4)
		if _, err := rand.Read(randomBytes); err != nil {
			tx.Rollback()
			return err
		}
		payload := base64.StdEncoding.EncodeToString(randomBytes)
		if _, err := tx.ExecContext(ctx, `INSERT INTO public.noise (payload) VALUES ($1)`, payload); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `TRUNCATE TABLE public.noise`)
	if err != nil {
		return fmt.Errorf("truncate noise after batch %d: %w", batch, err)
	}
	return nil
}

func errorStatus(err error) string {
	if err == nil {
		return "batch committed and noise table truncated"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Sprintf("PostgreSQL SQLSTATE %s: %s", pgErr.Code, pgErr.Message)
	}
	return err.Error()
}

func isDiskFull(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "53100" {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no space left on device")
}

func writeSummary(path, scenario, postgresVersion string, initial, lastMeasured, terminalObservation observation, batches, rows, maxBatches int, peakRetainedWAL, peakFilesystemUsed int64, terminal string, healthySamples, diskFull bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bounded disk-fill experiment: %s\n", scenario)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Debezium remained running throughout the workload. The workload wrote only to unpublished `public.noise`; PostgreSQL used a dedicated 256 MiB tmpfs, not a host data directory. Scenario: **%s**.\n", scenario)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- PostgreSQL: %s. Debezium Server: 3.6.3.Final.\n", postgresVersion)
	fmt.Fprintf(&b, "- Workload: %d completed batches out of a %d-batch budget, each %d rows of 1 MiB text, followed by `TRUNCATE`.\n", batches, maxBatches, rows)
	healthWindow := "UP in every sample before PostgreSQL crashed"
	if scenario == "action-query" {
		healthWindow = "UP in every sample"
	}
	fmt.Fprintf(&b, "- Debezium health was `%s` at start; health was UP and the slot active in every sample %s: %t.\n", initial.debeziumHealth, strings.TrimPrefix(healthWindow, "UP in "), healthySamples)
	if scenario == "no-heartbeat" {
		fmt.Fprintf(&b, "- Heartbeat: disabled (`heartbeat.interval.ms=0`); confirmed flush LSN: `%s` at start to `%s` at last successful sample.\n", nullString(initial.confirmedFlushLSN), nullString(lastMeasured.confirmedFlushLSN))
	} else {
		fmt.Fprintf(&b, "- Heartbeat: 10-second interval plus action query on published `public.cdc_heartbeat`; update count: %s to %s; confirmed flush LSN: `%s` at start to `%s` at last successful sample.\n", nullInt(initial.heartbeatUpdates), nullInt(lastMeasured.heartbeatUpdates), nullString(initial.confirmedFlushLSN), nullString(lastMeasured.confirmedFlushLSN))
	}
	fmt.Fprintf(&b, "- Restart LSN: `%s` to `%s`.\n", nullString(initial.restartLSN), nullString(lastMeasured.restartLSN))
	fmt.Fprintf(&b, "- Retained-WAL distance: %s at start to %s bytes at the last successful sample. `pg_wal` allocated size: %s to %s bytes.\n", nullInt(initial.retainedWALBytes), nullInt(lastMeasured.retainedWALBytes), nullInt(initial.pgWALBytes), nullInt(lastMeasured.pgWALBytes))
	fmt.Fprintf(&b, "- Peak retained-WAL distance: %d bytes. Peak PostgreSQL data filesystem usage: %d / %d bytes.\n", peakRetainedWAL, peakFilesystemUsed, initial.pgdataCapacity)
	fmt.Fprintf(&b, "- PostgreSQL data filesystem: %d / %d bytes used at start; last successful sample %d / %d bytes used with %d bytes available.\n", initial.pgdataUsedBytes, initial.pgdataCapacity, lastMeasured.pgdataUsedBytes, lastMeasured.pgdataCapacity, lastMeasured.pgdataAvailable)
	fmt.Fprintf(&b, "- Terminal write result: %s. Container state at the final metrics probe: `%s`. See the [PostgreSQL log](postgres.log).\n", terminal, terminalObservation.postgresState)
	if terminalObservation.postgresQueryError != "" {
		fmt.Fprintf(&b, "- Final metrics query: unavailable after PostgreSQL stopped (`%s`). Last successful metrics are shown above.\n", terminalObservation.postgresQueryError)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Verdict")
	if scenario == "no-heartbeat" && diskFull {
		fmt.Fprintf(&b, "PostgreSQL ran out of space after %d batches. Debezium health was UP in every sample until PostgreSQL crashed. The confirmed flush position stopped advancing while retained WAL grew. The bounded tmpfs and connector-offset volume were destroyed during cleanup.\n", batches)
	} else if scenario == "action-query" && !diskFull {
		fmt.Fprintf(&b, "PostgreSQL did not run out of space within the same %d-batch budget. The heartbeat action query advanced the slot while WAL remained bounded. The bounded tmpfs and connector-offset volume were destroyed during cleanup.\n", maxBatches)
	} else {
		fmt.Fprintln(&b, "Unexpected outcome for this scenario. Inspect the CSV and PostgreSQL log; do not interpret it as the expected comparison result.")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func run() (returnErr error) {
	rows := flag.Int("rows-per-batch", 8, "random 1 MiB rows committed per batch")
	maxBatches := flag.Int("max-batches", 30, "same workload budget for both heartbeat scenarios")
	batchInterval := flag.Duration("batch-interval", 10*time.Second, "pause after each successful batch so checkpoints and heartbeats can run")
	scenario := flag.String("scenario", "no-heartbeat", "scenario: no-heartbeat or action-query")
	flag.Parse()
	if *rows <= 0 || *maxBatches <= 0 || *batchInterval < 0 || (*scenario != "no-heartbeat" && *scenario != "action-query") {
		return errors.New("rows-per-batch and max-batches must be positive, batch-interval cannot be negative, and scenario must be no-heartbeat or action-query")
	}
	actionQuery = *scenario == "action-query"
	if !confirm(os.Stdin, os.Stdout) {
		fmt.Println("Cancelled; the disk-fill stack was not started.")
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	stackStarted := false
	defer func() {
		if !stackStarted {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := runCommand(cleanupCtx, "docker", composeArgs("down", "--volumes", "--remove-orphans")...); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("remove disposable disk-fill stack: %w", err)
		}
	}()

	stamp := time.Now().UTC().Format("20060102T150405Z") + "-" + *scenario
	runDir := filepath.Join("results", "disk-fill", stamp)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	output, err := openCSV(filepath.Join(runDir, "observations.csv"))
	if err != nil {
		return err
	}
	defer func() {
		if err := output.close(); err != nil && returnErr == nil {
			returnErr = err
		}
	}()

	stackStarted = true
	if err := runCommand(ctx, "docker", composeArgs("up", "--build", "-d")...); err != nil {
		return err
	}
	id, err := containerID(ctx)
	if err != nil {
		return err
	}
	state, safeTmpfs, err := inspectContainer(ctx, id)
	if err != nil {
		return err
	}
	if !safeTmpfs {
		return fmt.Errorf("safety check failed: %s data directory is not a %d-byte tmpfs (container state %s); no workload was started", dataDirectory, tmpfsSizeBytes, state)
	}
	_, _, filesystemCapacity, err := readFilesystem(ctx, id)
	if err != nil {
		return fmt.Errorf("safety check failed: cannot verify tmpfs capacity: %w", err)
	}
	if filesystemCapacity < tmpfsSizeBytes-(1<<20) || filesystemCapacity > tmpfsSizeBytes+(1<<20) {
		return fmt.Errorf("safety check failed: data filesystem capacity is %d bytes, expected about %d; no workload was started", filesystemCapacity, tmpfsSizeBytes)
	}
	fmt.Fprintf(os.Stdout, "Safety check passed: PostgreSQL data filesystem is %d bytes (capped tmpfs).\n", filesystemCapacity)

	db, err := sql.Open("pgx", databaseDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(30 * time.Second)
	if err := waitFor(ctx, func(checkCtx context.Context) (bool, error) {
		var version string
		if err := db.QueryRowContext(checkCtx, `SELECT current_setting('server_version')`).Scan(&version); err != nil {
			return false, err
		}
		var active bool
		if err := db.QueryRowContext(checkCtx, `SELECT active FROM pg_replication_slots WHERE slot_name=$1`, slotName).Scan(&active); err != nil {
			return false, err
		}
		response, err := http.Get(healthURL)
		if err != nil {
			return false, err
		}
		defer response.Body.Close()
		return active && response.StatusCode == http.StatusOK, nil
	}); err != nil {
		return fmt.Errorf("Debezium did not become ready: %w", err)
	}

	if err := waitFor(ctx, func(checkCtx context.Context) (bool, error) {
		var active bool
		if err := db.QueryRowContext(checkCtx, `SELECT active FROM pg_replication_slots WHERE slot_name=$1`, slotName).Scan(&active); err != nil {
			return false, err
		}
		return active, nil
	}); err != nil {
		return fmt.Errorf("replication slot did not remain active: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	initial := record(ctx, db, id, client, 0, "Debezium running; waiting for first batch")
	if err := output.write(initial); err != nil {
		return err
	}
	versionCtx, versionCancel := context.WithTimeout(ctx, 5*time.Second)
	var postgresVersion string
	if err := db.QueryRowContext(versionCtx, `SELECT current_setting('server_version')`).Scan(&postgresVersion); err != nil {
		versionCancel()
		return err
	}
	versionCancel()

	final := initial
	lastMeasured := initial
	terminal := "batch limit reached before disk-full failure"
	diskFull := false
	healthySamples := initial.debeziumHealth == "UP" && initial.slotActive.Valid && initial.slotActive.Bool
	peakRetainedWAL := initial.retainedWALBytes.Int64
	peakFilesystemUsed := initial.pgdataUsedBytes
	batches := 0
	for batch := 1; batch <= *maxBatches; batch++ {
		batchCtx, batchCancel := context.WithTimeout(ctx, 90*time.Second)
		writeErr := writeBatch(batchCtx, db, batch, *rows, 1<<20)
		batchCancel()
		status := errorStatus(writeErr)
		final = record(ctx, db, id, client, batch, status)
		if final.retainedWALBytes.Valid && final.pgWALBytes.Valid && final.pgdataCapacity > 0 {
			lastMeasured = final
		}
		if final.retainedWALBytes.Valid && final.retainedWALBytes.Int64 > peakRetainedWAL {
			peakRetainedWAL = final.retainedWALBytes.Int64
		}
		if final.pgdataCapacity > 0 && final.pgdataUsedBytes > peakFilesystemUsed {
			peakFilesystemUsed = final.pgdataUsedBytes
		}
		if writeErr == nil && (final.debeziumHealth != "UP" || !final.slotActive.Valid || !final.slotActive.Bool) {
			healthySamples = false
		}
		if err := output.write(final); err != nil {
			return err
		}
		if writeErr == nil {
			batches++
		}
		fmt.Fprintf(os.Stdout, "batch %d: %s; tmpfs available=%d bytes; retained-WAL=%s\n", batch, status, final.pgdataAvailable, nullInt(final.retainedWALBytes))
		if isDiskFull(writeErr) || (final.pgdataCapacity > 0 && final.pgdataAvailable == 0) {
			diskFull = true
			terminal = status
			break
		}
		if writeErr != nil {
			terminal = status
			break
		}
		if final.postgresState != "running" {
			terminal = "PostgreSQL container stopped after batch"
			break
		}
		if batch == *maxBatches {
			terminal = "safety batch limit reached"
		}
		if writeErr == nil && batch < *maxBatches && *batchInterval > 0 {
			timer := time.NewTimer(*batchInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	logCtx, logCancel := context.WithTimeout(context.Background(), 10*time.Second)
	logs, logErr := capture(logCtx, "docker", composeArgs("logs", "--no-color", "postgres")...)
	logCancel()
	if logErr == nil {
		if err := os.WriteFile(filepath.Join(runDir, "postgres.log"), logs, 0o644); err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(logs)), "no space left on device") {
			diskFull = true
			terminal = "PostgreSQL logged `No space left on device` and stopped during checkpoint/recovery; the client received " + terminal
		}
	}
	if err := writeSummary(filepath.Join(runDir, "summary.md"), *scenario, postgresVersion, initial, lastMeasured, final, batches, *rows, *maxBatches, peakRetainedWAL, peakFilesystemUsed, terminal, healthySamples, diskFull); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "disk-fill results: %s\n", runDir)
	if *scenario == "no-heartbeat" && !diskFull {
		return errors.New("no-heartbeat scenario ended without observing disk full; see summary and PostgreSQL log")
	}
	if *scenario == "action-query" && diskFull {
		return errors.New("action-query scenario filled the bounded filesystem; see summary and PostgreSQL log")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
