// Command server is the entry point of the searcher backend. It loads
// configuration, connects to PostgreSQL, runs schema migrations and starts
// the HTTP server until termination signals arrive.
package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/berzz/searcher/backend/internal/config"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/server"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	pool, err := persistence.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	slog.Info("connected to database")

	if err := persistence.Migrate(ctx, pool, migrationsFS); err != nil {
		return err
	}
	slog.Info("migrations applied")

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg.Host+":"+cfg.Port, pool)

	err = srv.Run(runCtx)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
