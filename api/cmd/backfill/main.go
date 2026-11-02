// Command backfill enqueues every transcript that has raw cues but no chunks.
// It is safe to run repeatedly; the database queue prevents duplicate active
// embedding jobs for the same video.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/berzz/searcher/backend/internal/config"
	"github.com/berzz/searcher/backend/internal/persistence"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logFatal(err)
	}
	pool, err := persistence.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logFatal(err)
	}
	defer pool.Close()
	count, err := persistence.EnqueueBackfill(context.Background(), pool)
	if err != nil {
		logFatal(fmt.Errorf("backfill: enqueue: %w", err))
	}
	slog.Info("transcript backfill jobs enqueued", "jobs", count)
}

func logFatal(err error) {
	slog.Error("backfill failed", "error", err)
	os.Exit(1)
}
