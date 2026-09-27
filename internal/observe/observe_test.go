package observe

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCSVWritingAndAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "observations.csv")
	w, err := OpenCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	o := Observation{
		Timestamp:         time.Date(2026, 9, 27, 12, 0, 0, 0, time.FixedZone("test", 3600)),
		SlotName:          "cdc_crashtest_slot",
		Active:            true,
		RestartLSN:        "0/16B6C50",
		ConfirmedFlushLSN: "0/16B6C88",
		WALStatus:         "reserved",
		RetainedWALBytes:  sql.NullInt64{Int64: 1024, Valid: true},
		PGWALBytes:        16777216,
		DebeziumHealth:    "UP",
		CheckpointCount:   7,
	}
	if err := w.Write(o); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	w, err = OpenCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(Observation{Timestamp: o.Timestamp, DebeziumHealth: "ERROR: server said, \"not ready\""}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || !reflect.DeepEqual(rows[0], Header) {
		t.Fatalf("CSV rows = %#v, want one header and two samples", rows)
	}
	wantFirst := []string{"2026-09-27T11:00:00Z", "cdc_crashtest_slot", "true", "0/16B6C50", "0/16B6C88", "reserved", "1024", "16777216", "UP", "7"}
	if !reflect.DeepEqual(rows[1], wantFirst) {
		t.Fatalf("first sample = %#v, want %#v", rows[1], wantFirst)
	}
	if rows[2][6] != "" || rows[2][8] != "ERROR: server said, \"not ready\"" {
		t.Fatalf("null or escaped fields were lost: %#v", rows[2])
	}
}
