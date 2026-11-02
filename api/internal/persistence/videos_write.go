package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/domain"
)

// UpdateVideoStatus moves a video along its lifecycle.
func UpdateVideoStatus(ctx context.Context, pool *pgxpool.Pool, id string, status domain.VideoStatus) error {
	tag, err := pool.Exec(ctx,
		`UPDATE videos SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("persistence: update video status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVideoNotFound
	}
	return nil
}

// DeleteVideo removes a video; associated cues/transcripts CASCADE.
func DeleteVideo(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM videos WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("persistence: delete video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVideoNotFound
	}
	return nil
}
