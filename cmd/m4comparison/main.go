package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprintln(out, "WARNING: this comparison deletes this Compose project's PostgreSQL and Debezium volumes before each run.")
	fmt.Fprintln(out, "The database, replication slot, and connector offsets in those volumes will be permanently erased; result files are kept.")
	fmt.Fprint(out, "Type YES to continue: ")
	scanner := bufio.NewScanner(in)
	return scanner.Scan() && strings.TrimSpace(scanner.Text()) == "YES"
}

func command(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func run() (returnErr error) {
	if !confirm(os.Stdin, os.Stdout) {
		fmt.Fprintln(os.Stdout, "Cancelled; no volumes were removed.")
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	composeStarted := false
	defer func() {
		if !composeStarted {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := command(cleanupCtx, "docker", "compose", "down"); err != nil && returnErr == nil {
			returnErr = fmt.Errorf("stop comparison stack: %w", err)
		}
	}()

	for _, scenario := range []string{"e1", "e2-timer", "e2"} {
		fmt.Fprintf(os.Stdout, "\n=== %s: clean baseline ===\n", scenario)
		composeStarted = true
		if err := command(ctx, "docker", "compose", "down", "--volumes", "--remove-orphans"); err != nil {
			return fmt.Errorf("reset volumes before %s: %w", scenario, err)
		}
		if err := command(ctx, "docker", "compose", "up", "--build", "-d"); err != nil {
			return fmt.Errorf("start stack for %s: %w", scenario, err)
		}
		if err := command(ctx, "go", "run", "./cmd/m4", "-scenario="+scenario); err != nil {
			return fmt.Errorf("run %s: %w", scenario, err)
		}
	}

	fmt.Fprintln(os.Stdout, "\nAll three scenarios completed. Each used fresh PostgreSQL and Debezium volumes.")
	return nil
}

func main() {
	if err := run(); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
