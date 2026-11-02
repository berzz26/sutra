// Package persistence owns the PostgreSQL connection pool and the schema
// migration runner.
package persistence

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies all embedded SQL migrations exactly once, in filename order.
// Applied versions are recorded in schema_migrations; each migration runs in
// its own transaction.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("persistence: create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("persistence: read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && hasExt(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		applied, err := isApplied(ctx, pool, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := apply(ctx, pool, migrationsFS, name); err != nil {
			return err
		}
	}

	return nil
}

func isApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("persistence: check migration %s: %w", version, err)
	}
	return exists, nil
}

func apply(ctx context.Context, pool *pgxpool.Pool, migrationsFS embed.FS, name string) error {
	start := time.Now()

	f, err := migrationsFS.Open("migrations/" + name)
	if err != nil {
		return fmt.Errorf("persistence: open migration %s: %w", name, err)
	}
	defer f.Close()

	sql, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("persistence: read migration %s: %w", name, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("persistence: begin migration %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		_ = tx.Rollback(ctx)
		if name == "0003_transcript_embeddings.sql" || name == "0004_processing_pipeline.sql" {
			return fmt.Errorf("persistence: exec migration %s: pgvector is required; install the PostgreSQL pgvector extension and grant CREATE on the database: %w", name, err)
		}
		return fmt.Errorf("persistence: exec migration %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, name,
	); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("persistence: record migration %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("persistence: commit migration %s: %w", name, err)
	}

	fmt.Printf("persistence: applied migration %s in %v\n", name, time.Since(start))
	return nil
}

func hasExt(name, ext string) bool {
	return len(name) > len(ext) && name[len(name)-len(ext):] == ext
}
