package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/obito1903/media-server-cleaner/internal/collect"
	"github.com/obito1903/media-server-cleaner/internal/config"
	"github.com/obito1903/media-server-cleaner/internal/scoring"
	"github.com/obito1903/media-server-cleaner/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Fprintln(os.Stderr, "Fetching data from Plex, Tautulli and Seerr...")
	items, warnings, err := collect.Collect(ctx, cfg)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	scoring.ScoreAll(items, cfg.Scoring, time.Now())
	return tui.Run(items, warnings)
}
