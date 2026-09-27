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
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/local/cdc-crashtest/internal/lab"
	"github.com/local/cdc-crashtest/internal/observe"
)

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func run() error {
	interval := flag.Duration("interval", 5*time.Second, "time between metric samples")
	slotName := flag.String("slot", lab.DefaultSlotName, "PostgreSQL replication slot name")
	dsn := flag.String("dsn", envOr("DATABASE_URL", lab.DefaultDSN), "PostgreSQL connection string (or set DATABASE_URL)")
	csvPath := flag.String("csv", "results/observations.csv", "CSV output path; rows append if the file exists")
	healthURL := flag.String("health-url", lab.DefaultHealthURL, "Debezium health endpoint")
	flag.Parse()
	if *interval <= 0 {
		return errors.New("interval must be greater than zero")
	}
	if strings.TrimSpace(*slotName) == "" {
		return errors.New("slot name must not be empty")
	}

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	writer, err := observe.OpenCSV(*csvPath)
	if err != nil {
		return err
	}
	defer writer.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	log.Printf("observing slot %q every %s; appending to %s", *slotName, *interval, *csvPath)

	writeSample := func() error {
		sampleCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		o, err := observe.Sample(sampleCtx, db, *slotName, *healthURL, client)
		if err != nil {
			return err
		}
		if err := writer.Write(o); err != nil {
			return fmt.Errorf("write CSV sample: %w", err)
		}
		log.Printf("sampled slot=%s active=%t retained_wal_bytes=%s health=%s", o.SlotName, o.Active, o.Record()[6], o.DebeziumHealth)
		return nil
	}
	if err := writeSample(); err != nil {
		return err
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("observer stopped")
			return nil
		case <-ticker.C:
			if err := writeSample(); err != nil {
				return err
			}
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
