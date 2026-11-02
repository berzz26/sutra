package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/domain"
)

// FinalizedMetadata carries everything written when an ingestion succeeds.
type FinalizedMetadata struct {
	Title        *string
	OriginalURL  *string
	ThumbnailURL *string
	DurationMs   *int64
	Channel      *string
	Language     *string
	Status       domain.VideoStatus
}

// WithinTx runs fn in a single transaction: failures roll back everything so
// no half-ingested video, transcript or cue set is ever left behind.
func WithinTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("persistence: begin tx: %w", err)
	}
	err = fn(tx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("persistence: commit tx: %w", err)
	}
	return nil
}

// FinalizeVideo updates video metadata + status after successful extraction.
// Scoped to the ingestion transaction so video, transcript and cues commit
// (or roll back) atomically.
func FinalizeVideo(ctx context.Context, tx pgx.Tx, videoID string, meta FinalizedMetadata) error {
	tag, err := tx.Exec(ctx, `
		UPDATE videos SET
			title = COALESCE($2, title),
			original_url = COALESCE($3, original_url),
			thumbnail_url = $4,
			duration_ms = $5,
			channel = $6,
			language = $7,
			status = $8,
			updated_at = now()
		WHERE id = $1`,
		videoID, meta.Title, meta.OriginalURL, meta.ThumbnailURL,
		meta.DurationMs, meta.Channel, meta.Language, meta.Status,
	)
	if err != nil {
		return fmt.Errorf("persistence: update video metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVideoNotFound
	}
	return nil
}

// UpsertTranscript inserts or replaces the video's transcript row. It returns
// the previously stored artifact key so callers can clean superseded objects.
func UpsertTranscript(ctx context.Context, tx pgx.Tx, t domain.Transcript) (oldArtifactKey *string, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO transcripts (id, video_id, extraction_method, original_format, language, artifact_key, cue_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (video_id) DO UPDATE SET
			id = EXCLUDED.id,
			extraction_method = EXCLUDED.extraction_method,
			original_format = EXCLUDED.original_format,
			language = EXCLUDED.language,
			artifact_key = EXCLUDED.artifact_key,
			cue_count = EXCLUDED.cue_count
		RETURNING artifact_key`,
		t.ID, t.VideoID, t.ExtractionMethod, t.OriginalFormat, t.Language, t.ArtifactKey, t.CueCount,
	).Scan(&oldArtifactKey)
	if err != nil {
		return nil, fmt.Errorf("persistence: upsert transcript: %w", err)
	}
	return oldArtifactKey, nil
}

// ReplaceCues deletes all cues for the video and bulk-inserts the new set.
// Safe because callers only reach this when the video is not READY: the only
// state where replacing raw cues is legitimate (retry after failed ingestion).
func ReplaceCues(ctx context.Context, tx pgx.Tx, videoID string, cues []domain.TranscriptCue) error {
	if _, err := tx.Exec(ctx, `DELETE FROM transcript_cues WHERE video_id = $1`, videoID); err != nil {
		return fmt.Errorf("persistence: clear old cues: %w", err)
	}
	if len(cues) == 0 {
		return nil
	}
	rows := make([][]any, len(cues))
	for i, cue := range cues {
		rows[i] = []any{videoID, cue.Seq, cue.StartMs, cue.EndMs, cue.Text}
	}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{"transcript_cues"},
		[]string{"video_id", "seq", "start_ms", "end_ms", "text"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("persistence: insert cues: %w", err)
	}
	if copied != int64(len(cues)) {
		return fmt.Errorf("persistence: inserted %d cues, expected %d", copied, len(cues))
	}
	return nil
}

const cueColumns = `id, video_id, seq, start_ms, end_ms, text`

// ListCues returns a page of cues ordered by sequence number.
func ListCues(ctx context.Context, pool *pgxpool.Pool, videoID string, limit, offset int) ([]domain.TranscriptCue, error) {
	rows, err := pool.Query(ctx,
		`SELECT `+cueColumns+` FROM transcript_cues WHERE video_id = $1 ORDER BY seq LIMIT $2 OFFSET $3`,
		videoID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("persistence: list cues: %w", err)
	}
	defer rows.Close()

	cues := []domain.TranscriptCue{}
	for rows.Next() {
		var cue domain.TranscriptCue
		if err := rows.Scan(&cue.ID, &cue.VideoID, &cue.Seq, &cue.StartMs, &cue.EndMs, &cue.Text); err != nil {
			return nil, fmt.Errorf("persistence: scan cue: %w", err)
		}
		cues = append(cues, cue)
	}
	return cues, rows.Err()
}

// GetTranscriptByVideo appeals to the transcripts table (one row per video).
func GetTranscriptByVideo(ctx context.Context, db Querier, videoID string) (domain.Transcript, error) {
	var t domain.Transcript
	err := db.QueryRow(ctx,
		`SELECT id, video_id, extraction_method, original_format, language, artifact_key, cue_count, created_at
		 FROM transcripts WHERE video_id = $1`, videoID,
	).Scan(&t.ID, &t.VideoID, &t.ExtractionMethod, &t.OriginalFormat,
		&t.Language, &t.ArtifactKey, &t.CueCount, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transcript{}, fmt.Errorf("persistence: transcript not found: %w", err)
	}
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("persistence: get transcript: %w", err)
	}
	return t, nil
}
