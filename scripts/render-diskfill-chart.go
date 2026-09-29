package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type point struct {
	seconds float64
	value   float64
}

type run struct {
	name, color        string
	retained, used     []point
	duration, crashAt  float64
	diskFull, healthUp bool
	slotActive         bool
}

func readRun(name, color, path string, expectDiskFull bool) (run, error) {
	result := run{name: name, color: color, healthUp: true, slotActive: true}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return result, err
	}
	columns := make(map[string]int, len(header))
	for i, column := range header {
		columns[column] = i
	}
	for _, required := range []string{"timestamp", "slot_active", "confirmed_flush_lsn", "retained_wal_bytes", "pgdata_used_bytes", "pgdata_capacity_bytes", "debezium_health", "workload_status"} {
		if _, ok := columns[required]; !ok {
			return result, fmt.Errorf("%s: missing column %q", path, required)
		}
	}

	var start, previous time.Time
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, fmt.Errorf("%s: read CSV: %w", path, err)
		}
		timestamp, err := time.Parse(time.RFC3339Nano, record[columns["timestamp"]])
		if err != nil {
			return result, fmt.Errorf("%s: parse timestamp: %w", path, err)
		}
		if !previous.IsZero() && !timestamp.After(previous) {
			return result, fmt.Errorf("%s: timestamps did not advance", path)
		}
		previous = timestamp
		if start.IsZero() {
			start = timestamp
		}
		elapsed := timestamp.Sub(start).Seconds()
		result.duration = elapsed
		status := strings.ToLower(record[columns["workload_status"]])
		terminalDiskFull := strings.Contains(status, "53100") || strings.Contains(status, "no space left on device")
		if terminalDiskFull {
			result.diskFull = true
			result.crashAt = elapsed
		}
		if !terminalDiskFull && record[columns["debezium_health"]] != "UP" {
			result.healthUp = false
		}

		retainedText := record[columns["retained_wal_bytes"]]
		if retainedText != "" {
			if record[columns["slot_active"]] != "true" {
				result.slotActive = false
			}
			bytes, err := strconv.ParseFloat(retainedText, 64)
			if err != nil {
				return result, fmt.Errorf("%s: parse retained_wal_bytes: %w", path, err)
			}
			result.retained = append(result.retained, point{seconds: elapsed, value: bytes / 1_000_000})
		}

		usedText := record[columns["pgdata_used_bytes"]]
		capacityText := record[columns["pgdata_capacity_bytes"]]
		if usedText != "" && capacityText != "" {
			used, err := strconv.ParseFloat(usedText, 64)
			if err != nil {
				return result, fmt.Errorf("%s: parse pgdata_used_bytes: %w", path, err)
			}
			capacity, err := strconv.ParseFloat(capacityText, 64)
			if err != nil || capacity <= 0 {
				return result, fmt.Errorf("%s: invalid pgdata_capacity_bytes %q", path, capacityText)
			}
			result.used = append(result.used, point{seconds: elapsed, value: used / (1024 * 1024)})
		}
	}
	if len(result.retained) < 2 || len(result.used) < 2 {
		return result, fmt.Errorf("%s: expected at least two complete metrics samples", path)
	}
	if !result.healthUp || !result.slotActive {
		return result, fmt.Errorf("%s: Debezium health was not UP or slot was inactive before the terminal outcome", path)
	}
	if result.diskFull != expectDiskFull {
		return result, fmt.Errorf("%s: disk-full outcome=%t, expected %t", path, result.diskFull, expectDiskFull)
	}
	return result, nil
}

func svgPath(points []point, x, y func(float64) float64) string {
	var path strings.Builder
	for i, sample := range points {
		command := "L"
		if i == 0 {
			command = "M"
		}
		fmt.Fprintf(&path, "%s%.1f,%.1f ", command, x(sample.seconds), y(sample.value))
	}
	return path.String()
}

func panel(data []run, selectPoints func(run) []point, title string, yTop, yBottom, yMax, yStep, xMax float64, showTime bool) string {
	const left, right = 112.0, 950.0
	x := func(value float64) float64 { return left + value/xMax*(right-left) }
	y := func(value float64) float64 { return yBottom - value/yMax*(yBottom-yTop) }
	var out strings.Builder
	fmt.Fprintf(&out, `<text x="%.0f" y="%.0f" font-family="Segoe UI, sans-serif" font-size="17" font-weight="700" fill="#26313b">%s</text>`, left, yTop-13, title)
	for value := 0.0; value <= yMax+0.001; value += yStep {
		lineY := y(value)
		fmt.Fprintf(&out, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dce2e8"/><text x="%.1f" y="%.1f" text-anchor="end" dominant-baseline="middle" font-family="Segoe UI, sans-serif" font-size="12" fill="#52606d">%.0f</text>`, left, lineY, right, lineY, left-12, lineY, value)
	}
	for minute := 0.0; minute <= xMax/60; minute++ {
		lineX := x(minute * 60)
		fmt.Fprintf(&out, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#edf0f3"/>`, lineX, yTop, lineX, yBottom)
		if showTime {
			fmt.Fprintf(&out, `<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="12" fill="#52606d">%.0f</text>`, lineX, yBottom+19, minute)
		}
	}
	fmt.Fprintf(&out, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#52606d" stroke-width="1.3"/><line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#52606d" stroke-width="1.3"/>`, left, yTop, left, yBottom, left, yBottom, right, yBottom)
	for _, item := range data {
		points := selectPoints(item)
		fmt.Fprintf(&out, `<path d="%s" fill="none" stroke="%s" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`, svgPath(points, x, y), item.color)
		last := points[len(points)-1]
		fmt.Fprintf(&out, `<circle cx="%.1f" cy="%.1f" r="4" fill="%s"/>`, x(last.seconds), y(last.value), item.color)
		if showTime && item.diskFull {
			fmt.Fprintf(&out, `<circle cx="%.1f" cy="%.1f" r="7" fill="#ffffff" stroke="#B42318" stroke-width="3"/>`, x(item.crashAt), y(yMax))
			fmt.Fprintf(&out, `<text x="%.1f" y="%.1f" font-family="Segoe UI, sans-serif" font-size="12" font-weight="700" fill="#B42318">disk full / PostgreSQL crash</text>`, x(item.crashAt)-155, y(yMax)+18)
		}
	}
	return out.String()
}

func render(data []run) string {
	const width, height = 1000.0, 700.0
	var xMax float64
	for _, item := range data {
		if item.duration > xMax {
			xMax = item.duration
		}
	}
	xMax = (float64(int(xMax/60) + 1)) * 60
	if xMax < 60 {
		xMax = 60
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-labelledby="title desc">`, width, height, width, height)
	out.WriteString(`<title id="title">Healthy CDC disk-fill comparison</title>`)
	out.WriteString(`<desc id="desc">Two matched PostgreSQL runs in separate 256 MiB tmpfs filesystems. Without a heartbeat PostgreSQL filled the filesystem while Debezium health remained UP until the crash. With the published-table action-query heartbeat, WAL was recycled and all 30 batches completed without filling the filesystem.</desc>`)
	out.WriteString(`<rect width="100%" height="100%" fill="#ffffff"/>`)
	out.WriteString(`<text x="80" y="38" font-family="Segoe UI, sans-serif" font-size="25" font-weight="700" fill="#18212b">A healthy connector can still let WAL fill the disk</text>`)
	out.WriteString(`<text x="80" y="65" font-family="Segoe UI, sans-serif" font-size="14" fill="#52606d">Matched disk-fill runs · 30-batch budget · 8 MiB per batch · PostgreSQL 17.11 · Debezium Server 3.6.3.Final</text>`)
	out.WriteString(panel(data, func(item run) []point { return item.retained }, "Retained-WAL distance (MB; this is not disk usage)", 110, 285, 250, 50, xMax, false))
	out.WriteString(panel(data, func(item run) []point { return item.used }, "PostgreSQL data-directory usage (MiB of 256 MiB tmpfs)", 385, 555, 256, 64, xMax, true))
	out.WriteString(`<text x="530" y="596" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="13" fill="#34404c">Elapsed time (minutes)</text>`)
	legendY := 632.0
	legendX := []float64{300, 565}
	for i, item := range data {
		fmt.Fprintf(&out, `<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="%s" stroke-width="4"/><text x="%.0f" y="%.0f" font-family="Segoe UI, sans-serif" font-size="14" fill="#26313b">%s</text>`, legendX[i], legendY-5, legendX[i]+28, legendY-5, item.color, legendX[i]+38, legendY, item.name)
	}
	out.WriteString(`<text x="500" y="675" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="13" font-weight="600" fill="#273746">Debezium health UP and slot active in every pre-crash sample; action-query run stayed healthy throughout</text>`)
	out.WriteString(`</svg>`)
	return out.String()
}

func main() {
	noHeartbeat := flag.String("no-heartbeat", "results/disk-fill/20260928T214209Z-no-heartbeat/observations.csv", "CSV for the no-heartbeat disk-fill run")
	actionQuery := flag.String("action-query", "results/disk-fill/20260928T214702Z-action-query/observations.csv", "CSV for the action-query run")
	output := flag.String("out", "docs/disk-fill-comparison.svg", "output SVG path")
	flag.Parse()
	noHeartbeatRun, err := readRun("No heartbeat", "#D55E00", *noHeartbeat, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	actionQueryRun, err := readRun("Published-table action query", "#009E73", *actionQuery, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, []byte(render([]run{noHeartbeatRun, actionQueryRun})), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s from two validated disk-fill CSVs\n", *output)
}
