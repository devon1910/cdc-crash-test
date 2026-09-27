package load

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Config struct {
	Orders     bool
	Noise      bool
	Rate       int // Writes per second to each selected table.
	NoiseBytes int
}

type Counts struct {
	Orders int64
	Noise  int64
}

func (c Config) Validate() error {
	if !c.Orders && !c.Noise {
		return errors.New("select -orders, -noise, or both")
	}
	if c.Rate < 1 || c.Rate > 1000 {
		return errors.New("rate must be between 1 and 1000 writes per second per selected table")
	}
	if c.NoiseBytes < 1 || c.NoiseBytes > 65536 {
		return errors.New("noise-bytes must be between 1 and 65536")
	}
	return nil
}

func Run(ctx context.Context, db *sql.DB, c Config) (Counts, error) {
	var counts Counts
	if err := c.Validate(); err != nil {
		return counts, err
	}
	ticker := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer ticker.Stop()
	payload := strings.Repeat("x", c.NoiseBytes)
	for {
		select {
		case <-ctx.Done():
			return counts, nil
		case <-ticker.C:
			if c.Orders {
				_, err := db.ExecContext(ctx,
					"INSERT INTO public.orders (customer_id, amount_cents) VALUES ($1, $2)",
					time.Now().UnixNano(), 1000)
				if err != nil {
					if ctx.Err() != nil {
						return counts, nil
					}
					return counts, fmt.Errorf("insert orders row: %w", err)
				}
				counts.Orders++
			}
			if c.Noise {
				_, err := db.ExecContext(ctx,
					"INSERT INTO public.noise (payload) VALUES ($1)", payload)
				if err != nil {
					if ctx.Err() != nil {
						return counts, nil
					}
					return counts, fmt.Errorf("insert noise row: %w", err)
				}
				counts.Noise++
			}
		}
	}
}
