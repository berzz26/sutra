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

	"github.com/berzz/searcher/backend/internal/artifact"
	"github.com/berzz/searcher/backend/internal/config"
	"github.com/berzz/searcher/backend/internal/embedding"
	"github.com/berzz/searcher/backend/internal/ingestion"
	"github.com/berzz/searcher/backend/internal/jobs"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/processing"
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
	emb, err := embedding.Load(embedding.Config{ModelPath: cfg.Embedding.ONNXPath, TokenizerPath: cfg.Embedding.TokenizerPath, RuntimePath: cfg.Embedding.RuntimePath, Dimensions: cfg.Embedding.Dimensions, BatchSize: cfg.Embedding.BatchSize, MaxTokens: cfg.Embedding.MaxTokens})
	if err != nil {
		return err
	}
	defer emb.Close()

	store, err := artifact.StoreOrNoop(cfg.Artifact)
	if err != nil {
		return err
	}
	extractor := ingestion.NewExtractor(cfg.YTDLPBinary, cfg.YTDLPTimeout)
	service := ingestion.NewService(pool, extractor, store, cfg.MaxUpload, cfg.MaxConcurrent)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	processor := processing.NewService(pool, emb, processing.ChunkConfig{Target: cfg.Embedding.TargetWords, Min: cfg.Embedding.MinWords, Max: cfg.Embedding.MaxWords, Overlap: cfg.Embedding.OverlapWords, Version: "v1"}, cfg.Embedding.Model+"/"+cfg.Embedding.Version, cfg.Embedding.Timeout)
	worker := jobs.NewWorker(pool, processor, cfg.Workers)
	worker.Start(runCtx)
	srv := server.New(cfg.Host+":"+cfg.Port, pool, service, processor, cfg.MaxUpload)

	err = srv.Run(runCtx)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Shutdown)
	defer shutdownCancel()
	if stopErr := worker.Stop(shutdownCtx); stopErr != nil && err == nil {
		err = fmt.Errorf("processing worker shutdown: %w", stopErr)
	}
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
