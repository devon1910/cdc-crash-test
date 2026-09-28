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
	"time"
)

type point struct {
	seconds float64
	bytes   float64
}

type series struct {
	name   string
	color  string
	points []point
}

func readSeries(name, color, path string) (series, error) {
	result := series{name: name, color: color}
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
	for index, column := range header {
		columns[column] = index
	}
	for _, required := range []string{"timestamp", "active", "retained_wal_bytes", "debezium_health_status"} {
		if _, ok := columns[required]; !ok {
			return result, fmt.Errorf("%s: missing column %q", path, required)
		}
	}
	var start time.Time
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
		if !start.IsZero() && !timestamp.After(start) && len(result.points) == 0 {
			return result, fmt.Errorf("%s: timestamps did not advance", path)
		}
		if start.IsZero() {
			start = timestamp
		}
		if record[columns["active"]] != "true" || record[columns["debezium_health_status"]] != "UP" {
			return result, fmt.Errorf("%s: slot was not active or Debezium health was not UP throughout", path)
		}
		bytes, err := strconv.ParseFloat(record[columns["retained_wal_bytes"]], 64)
		if err != nil {
			return result, fmt.Errorf("%s: parse retained_wal_bytes: %w", path, err)
		}
		result.points = append(result.points, point{seconds: timestamp.Sub(start).Seconds(), bytes: bytes})
	}
	if len(result.points) < 2 {
		return result, fmt.Errorf("%s: expected at least two samples", path)
	}
	return result, nil
}

func svgPath(points []point, x, y func(float64) float64) string {
	path := ""
	for index, sample := range points {
		command := "L"
		if index == 0 {
			command = "M"
		}
		path += fmt.Sprintf("%s%.1f,%.1f ", command, x(sample.seconds), y(sample.bytes))
	}
	return path
}

func render(data []series) string {
	const (
		width  = 960.0
		height = 590.0
		left   = 96.0
		top    = 100.0
		right  = 915.0
		bottom = 455.0
		xMax   = 610.0
		yMax   = 2_000_000.0
	)
	x := func(value float64) float64 { return left + value/xMax*(right-left) }
	y := func(value float64) float64 { return bottom - value/yMax*(bottom-top) }

	var out string
	out += fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-labelledby="title desc">`, width, height, width, height)
	out += `<title id="title">Retained WAL during a quiet-source workload</title>`
	out += `<desc id="desc">Three independent 10-minute runs. No-heartbeat and timer-only retained-WAL distance rise to about 1.9 megabytes; the published-table action-query run stays below 0.4 megabytes. Debezium health is UP and the slot active throughout every run.</desc>`
	out += `<rect width="100%" height="100%" fill="#ffffff"/>`
	out += `<text x="96" y="38" font-family="Segoe UI, sans-serif" font-size="25" font-weight="700" fill="#18212b">Retained WAL during a quiet-source workload</text>`
	out += `<text x="96" y="66" font-family="Segoe UI, sans-serif" font-size="14" fill="#52606d">Three independently reset 10-minute runs · PostgreSQL 17.11 · Debezium Server 3.6.3.Final</text>`

	for value := 0.0; value <= yMax; value += 500_000 {
		lineY := y(value)
		out += fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#dce2e8" stroke-width="1"/>`, left, lineY, right, lineY)
		out += fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="end" dominant-baseline="middle" font-family="Segoe UI, sans-serif" font-size="12" fill="#52606d">%.1f MB</text>`, left-12, lineY, value/1_000_000)
	}
	for minute := 0.0; minute <= 10; minute += 2 {
		lineX := x(minute * 60)
		out += fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#edf0f3" stroke-width="1"/>`, lineX, top, lineX, bottom)
		out += fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="12" fill="#52606d">%.0f</text>`, lineX, bottom+20, minute)
	}
	out += fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#52606d" stroke-width="1.3"/><line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#52606d" stroke-width="1.3"/>`, left, top, left, bottom, left, bottom, right, bottom)
	out += fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="13" fill="#34404c">Elapsed time (minutes)</text>`, (left+right)/2, bottom+47)
	out += fmt.Sprintf(`<text transform="translate(24 %.1f) rotate(-90)" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="13" fill="#34404c">Retained-WAL distance (MB; not disk usage)</text>`, (top+bottom)/2)
	for _, item := range data {
		out += fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`, svgPath(item.points, func(seconds float64) float64 { return x(seconds) }, y), item.color)
	}
	legendY := 535.0
	legendX := []float64{170, 405, 660}
	for index, item := range data {
		out += fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="3"/><text x="%.1f" y="%.1f" font-family="Segoe UI, sans-serif" font-size="13" fill="#26313b">%s</text>`, legendX[index], legendY-5, legendX[index]+25, legendY-5, item.color, legendX[index]+33, legendY, item.name)
	}
	out += `<text x="480" y="570" text-anchor="middle" font-family="Segoe UI, sans-serif" font-size="13" font-weight="600" fill="#273746">Debezium health UP and replication slot active throughout all three runs</text>`
	out += `</svg>`
	return out
}

func main() {
	noHeartbeat := flag.String("no-heartbeat", "results/m4-stability-repeat/e1/observations.csv", "CSV for the no-heartbeat run")
	timerOnly := flag.String("timer-only", "results/m4-stability-repeat/e2-timer/observations.csv", "CSV for timer-only heartbeats")
	actionQuery := flag.String("action-query", "results/m4-stability-repeat/e2/observations.csv", "CSV for the published-table action-query run")
	output := flag.String("out", "docs/heartbeat-comparison.svg", "output SVG path")
	flag.Parse()
	data := make([]series, 0, 3)
	for _, input := range []struct{ name, color, path string }{
		{"No heartbeat", "#0072B2", *noHeartbeat},
		{"Timer only (10s)", "#D55E00", *timerOnly},
		{"Published-table action query", "#009E73", *actionQuery},
	} {
		item, err := readSeries(input.name, input.color, input.path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		data = append(data, item)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, []byte(render(data)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s from three validated CSVs\n", *output)
}
