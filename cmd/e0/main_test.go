package main

import (
	"database/sql"
	"testing"
	"time"

	"github.com/local/cdc-crashtest/internal/observe"
)

func TestLSNValue(t *testing.T) {
	got, err := lsnValue("1/00000010")
	if err != nil {
		t.Fatal(err)
	}
	if want := uint64(1)<<32 | 0x10; got != want {
		t.Fatalf("LSN value = %d, want %d", got, want)
	}
	if _, err := lsnValue("not-an-lsn"); err == nil {
		t.Fatal("invalid LSN was accepted")
	}
}

func TestAnalyzeBaseline(t *testing.T) {
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	samples := []observe.Observation{
		{Timestamp: start, Active: true, ConfirmedFlushLSN: "0/100", RetainedWALBytes: sql.NullInt64{Int64: 1000, Valid: true}, DebeziumHealth: "UP"},
		{Timestamp: start.Add(5 * time.Second), Active: true, ConfirmedFlushLSN: "0/200", RetainedWALBytes: sql.NullInt64{Int64: 2000, Valid: true}, DebeziumHealth: "UP"},
	}
	a, err := analyze(samples, 16*1024*1024, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Pass || !a.FlushAdvanced || a.GrowthBytesPerSec != 200 {
		t.Fatalf("analysis = %+v, want passing baseline and 200 bytes/s", a)
	}

	samples[1].RetainedWALBytes.Int64 = 17 * 1024 * 1024
	a, err = analyze(samples, 16*1024*1024, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Pass {
		t.Fatal("oversized retained WAL passed baseline")
	}
	samples[1].RetainedWALBytes.Int64 = 2000
	samples[1].ConfirmedFlushLSN = samples[0].ConfirmedFlushLSN
	a, err = analyze(samples, 16*1024*1024, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Pass {
		t.Fatal("unchanged confirmed flush LSN passed baseline")
	}
	samples[1].ConfirmedFlushLSN = "0/200"
	samples[1].Timestamp = start.Add(4 * time.Minute)
	a, err = analyze(samples, 16*1024*1024, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if a.Pass || a.MaxSampleGap != 4*time.Minute {
		t.Fatalf("large sample gap was not rejected: %+v", a)
	}
}
