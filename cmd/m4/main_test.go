package main

import (
    "database/sql"
    "strings"
    "testing"
    "time"

    "github.com/local/cdc-crashtest/internal/observe"
)

func TestLSNValue(t *testing.T) {
    got, err := lsnValue("1/10")
    if err != nil || got != (uint64(1)<<32)|16 { t.Fatalf("got %d, %v", got, err) }
    if _, err := lsnValue("invalid"); err == nil { t.Fatal("invalid LSN accepted") }
}

func TestSummaryDistinguishesFlushFromRetention(t *testing.T) {
    start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
    samples := []observe.Observation{
        {Timestamp:start, SlotName:"slot", Active:true, RestartLSN:"0/100", ConfirmedFlushLSN:"0/200", RetainedWALBytes:sql.NullInt64{Int64:100, Valid:true}, DebeziumHealth:"UP"},
        {Timestamp:start.Add(5*time.Second), SlotName:"slot", Active:true, RestartLSN:"0/100", ConfirmedFlushLSN:"0/300", RetainedWALBytes:sql.NullInt64{Int64:200, Valid:true}, DebeziumHealth:"UP"},
    }
    got, err := summary("e2", samples, 50, 5*time.Second, 3, 4, 10, 5*time.Second, 5*time.Second, 0)
    if err != nil { t.Fatal(err) }
    for _, want := range []string{"advanced: true", "Restart LSN: `0/100` to `0/100`", "Heartbeat row updates during run: 1"} {
        if !strings.Contains(got, want) { t.Fatalf("summary missing %q: %s", want, got) }
    }
}
