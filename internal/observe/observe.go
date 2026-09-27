package observe

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var Header = []string{
	"timestamp", "slot_name", "active", "restart_lsn", "confirmed_flush_lsn", "wal_status",
	"retained_wal_bytes", "pg_wal_bytes", "debezium_health_status", "checkpoint_count",
}

type Observation struct {
	Timestamp         time.Time
	SlotName          string
	Active            bool
	RestartLSN        string
	ConfirmedFlushLSN string
	WALStatus         string
	RetainedWALBytes  sql.NullInt64
	PGWALBytes        int64
	DebeziumHealth    string
	CheckpointCount   int64
}

func (o Observation) Record() []string {
	retainedBytes := ""
	if o.RetainedWALBytes.Valid {
		retainedBytes = strconv.FormatInt(o.RetainedWALBytes.Int64, 10)
	}
	return []string{
		o.Timestamp.UTC().Format(time.RFC3339Nano), o.SlotName, strconv.FormatBool(o.Active),
		o.RestartLSN, o.ConfirmedFlushLSN, o.WALStatus, retainedBytes,
		strconv.FormatInt(o.PGWALBytes, 10), o.DebeziumHealth, strconv.FormatInt(o.CheckpointCount, 10),
	}
}

type CSVWriter struct {
	file *os.File
	csv  *csv.Writer
}

func OpenCSV(path string) (*CSVWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create CSV directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open CSV file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat CSV file: %w", err)
	}
	w := &CSVWriter{file: file, csv: csv.NewWriter(file)}
	if info.Size() == 0 {
		if err := w.write(Header); err != nil {
			file.Close()
			return nil, fmt.Errorf("write CSV header: %w", err)
		}
	}
	return w, nil
}

func (w *CSVWriter) write(record []string) error {
	if err := w.csv.Write(record); err != nil {
		return err
	}
	w.csv.Flush()
	return w.csv.Error()
}

func (w *CSVWriter) Write(o Observation) error { return w.write(o.Record()) }

func (w *CSVWriter) Close() error {
	w.csv.Flush()
	return errors.Join(w.csv.Error(), w.file.Close())
}

func Sample(ctx context.Context, db *sql.DB, slotName, healthURL string, client *http.Client) (Observation, error) {
	var o Observation
	o.Timestamp = time.Now().UTC()
	err := db.QueryRowContext(ctx, `
		SELECT s.slot_name,
		       s.active,
		       COALESCE(s.restart_lsn::text, ''),
		       COALESCE(s.confirmed_flush_lsn::text, ''),
		       COALESCE(s.wal_status, ''),
		       CASE WHEN s.restart_lsn IS NULL THEN NULL
		            ELSE pg_wal_lsn_diff(pg_current_wal_lsn(), s.restart_lsn)::bigint END,
		       (SELECT COALESCE(sum(size), 0)::bigint FROM pg_ls_waldir()),
		       (SELECT num_timed + num_requested FROM pg_stat_checkpointer)
		FROM pg_replication_slots AS s
		WHERE s.slot_name = $1`, slotName).Scan(
		&o.SlotName, &o.Active, &o.RestartLSN, &o.ConfirmedFlushLSN, &o.WALStatus,
		&o.RetainedWALBytes, &o.PGWALBytes, &o.CheckpointCount,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return o, fmt.Errorf("replication slot %q was not found", slotName)
		}
		return o, fmt.Errorf("query PostgreSQL metrics: %w", err)
	}
	o.DebeziumHealth = healthStatus(ctx, client, healthURL)
	return o, nil
}

func healthStatus(ctx context.Context, client *http.Client, healthURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Sprintf("HTTP_%d", resp.StatusCode)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return "ERROR: invalid health response"
	}
	if status.Status == "" {
		return "ERROR: missing health status"
	}
	return status.Status
}
