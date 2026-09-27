package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/local/cdc-crashtest/internal/lab"
	"github.com/local/cdc-crashtest/internal/load"
)

func main() {
	orders := flag.Bool("orders", false, "write to public.orders")
	noise := flag.Bool("noise", false, "write to public.noise")
	rate := flag.Int("rate", 10, "writes per second to each selected table")
	noiseBytes := flag.Int("noise-bytes", 4096, "bytes per public.noise payload")
	duration := flag.Duration("duration", 10*time.Minute, "how long to write rows")
	dsn := flag.String("dsn", envOr("DATABASE_URL", lab.DefaultDSN), "PostgreSQL connection string (or set DATABASE_URL)")
	flag.Parse()
	cfg := load.Config{Orders: *orders, Noise: *noise, Rate: *rate, NoiseBytes: *noiseBytes}
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if *duration <= 0 {
		log.Fatal("duration must be greater than zero")
	}
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatal(fmt.Errorf("open PostgreSQL connection: %w", err))
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()
	log.Printf("loading orders=%t noise=%t rate=%d/s duration=%s", *orders, *noise, *rate, *duration)
	counts, err := load.Run(ctx, db, cfg)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
	log.Printf("wrote %d orders rows and %d noise rows", counts.Orders, counts.Noise)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
