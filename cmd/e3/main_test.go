package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/local/cdc-crashtest/internal/load"
	"github.com/local/cdc-crashtest/internal/observe"
)

func TestFirstAfterAndLastBefore(t *testing.T) {
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	samples := []observe.Observation{{Timestamp: start}, {Timestamp: start.Add(5 * time.Second)}, {Timestamp: start.Add(10 * time.Second)}}
	if got := lastBefore(samples, start.Add(7*time.Second)); got != &samples[1] {
		t.Fatalf("lastBefore = %p", got)
	}
	if got := firstAfter(samples, start.Add(7*time.Second), func(observe.Observation) bool { return true }); got != &samples[2] {
		t.Fatalf("firstAfter = %p", got)
	}
}

func TestReportUsesCompletedStopAsGrowthBaseline(t *testing.T) {
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	sample := func(seconds int, active bool, health string, retained int64) observe.Observation {
		return observe.Observation{Timestamp: start.Add(time.Duration(seconds) * time.Second), SlotName: "slot", Active: active, ConfirmedFlushLSN: "0/200", RestartLSN: "0/100", RetainedWALBytes: sql.NullInt64{Int64: retained, Valid: true}, DebeziumHealth: health}
	}
	samples := []observe.Observation{sample(0, true, "UP", 100), sample(5, true, "UP", 110), sample(10, true, "UP", 120), sample(12, false, "ERROR: unavailable", 130), sample(15, false, "ERROR: unavailable", 150), sample(20, false, "ERROR: unavailable", 200)}
	got, err := report(samples, load.Counts{Orders: 10, Noise: 10}, start.Add(11*time.Second), start.Add(13*time.Second), 20*time.Second, 5*time.Second, 0, 20*time.Second, 1, 4096, "17", "postgres:17")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"First completed-stop sample (growth baseline): first sampled at 2026-09-28T00:00:15Z", "Retained WAL above completed-stop baseline: first sampled at 2026-09-28T00:00:20Z", "Inactive slot after completed stop: first sampled at 2026-09-28T00:00:15Z", "2s after stop completed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report missing %q: %s", want, got)
		}
	}
}
