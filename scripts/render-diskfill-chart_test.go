package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRunIncludesFullFilesystemSampleAndValidatesHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observations.csv")
	csv := strings.Join([]string{
		"timestamp,batch,slot_active,restart_lsn,confirmed_flush_lsn,heartbeat_update_count,retained_wal_bytes,pg_wal_bytes,pgdata_used_bytes,pgdata_available_bytes,pgdata_capacity_bytes,postgres_state,debezium_health,workload_status,postgres_query_error",
		"2026-09-28T00:00:00Z,0,true,0/A,0/B,0,96,16777216,48000000,220000000,268435456,running,UP,waiting,",
		"2026-09-28T00:00:10Z,1,true,0/A,0/B,0,9000000,33554432,60000000,208000000,268435456,running,UP,committed,",
		"2026-09-28T00:00:20Z,2,,,,,,,268435456,0,268435456,running,UP,PostgreSQL SQLSTATE 53100: No space left on device,metrics unavailable",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readRun("No heartbeat", "#D55E00", path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.diskFull || !got.healthUp || !got.slotActive {
		t.Fatalf("unexpected run validation: diskFull=%t healthUp=%t slotActive=%t", got.diskFull, got.healthUp, got.slotActive)
	}
	if len(got.retained) != 2 || len(got.used) != 3 || got.crashAt != 20 {
		t.Fatalf("terminal sample not represented correctly: retained=%d used=%d crashAt=%v", len(got.retained), len(got.used), got.crashAt)
	}
}
