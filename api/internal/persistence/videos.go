package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/domain"
)

// ErrVideoNotFound is returned by lookups that miss.
var ErrVideoNotFound = errors.New("persistence: video not found")

// Querier accepts a pool or a transaction so single-statement helpers and
// transaction-scoped helpers share signatures.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

const videoColumns = `id, source_type, source_ref, title, original_url,
	thumbnail_url, duration_ms, channel, language, status, created_at, updated_at`

func scanVideo(row pgx.Row) (domain.Video, error) {
	var (
		v                                                domain.Video
		title, originalURL, thumbnail, channel, language *string
		durationMs                                       *int64
	)
	err := row.Scan(
		&v.ID, &v.SourceType, &v.SourceRef, &title,
		&originalURL, &thumbnail, &durationMs, &channel, &language,
		&v.Status, &v.CreatedAt, &v.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Video{}, ErrVideoNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("persistence: scan video: %w", err)
	}
	v.Title, v.OriginalURL, v.ThumbnailURL = title, originalURL, thumbnail
	v.DurationMs, v.Channel, v.Language = durationMs, channel, language
	return v, nil
}

type VideoSeed struct {
	OriginalURL *string
	Title       *string
	Language    *string
}

// EnsureVideoPending inserts a pending video or returns the existing row
// (existed=true). The UNIQUE (source_type, source_ref) constraint is the
// final guard against concurrent duplicate ingestion.
func EnsureVideoPending(ctx context.Context, db Querier, sourceType domain.VideoSourceType, sourceRef string, seed VideoSeed) (domain.Video, bool, error) {
	var v domain.Video
	err := db.QueryRow(ctx,
		`INSERT INTO videos (source_type, source_ref, original_url, title, language)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_type, source_ref) DO NOTHING
		 RETURNING `+videoColumns,
		sourceType, sourceRef, seed.OriginalURL, seed.Title, seed.Language,
	).Scan(
		&v.ID, &v.SourceType, &v.SourceRef, &v.Title,
		&v.OriginalURL, &v.ThumbnailURL, &v.DurationMs, &v.Channel, &v.Language,
		&v.Status, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == nil {
		return v, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Video{}, false, fmt.Errorf("persistence: insert video: %w", err)
	}
	existing, err := GetVideoBySource(ctx, db, sourceType, sourceRef)
	if err != nil {
		return domain.Video{}, false, err
	}
	return existing, true, nil
}

// GetVideoBySource looks a video up by its canonical source identity.
func GetVideoBySource(ctx context.Context, db Querier, sourceType domain.VideoSourceType, sourceRef string) (domain.Video, error) {
	return scanVideo(db.QueryRow(ctx,
		`SELECT `+videoColumns+` FROM videos WHERE source_type = $1 AND source_ref = $2`,
		sourceType, sourceRef,
	))
}

// VideoWithTranscript pairs a video with its optional transcript summary.
type VideoWithTranscript struct {
	Video      domain.Video
	Transcript *domain.Transcript
}

// GetVideo returns a video together with its transcript summary (if any).
func GetVideo(ctx context.Context, pool *pgxpool.Pool, id string) (VideoWithTranscript, error) {
	row := pool.QueryRow(ctx, `
		SELECT v.id, v.source_type, v.source_ref, v.title, v.original_url,
		       v.thumbnail_url, v.duration_ms, v.channel, v.language, v.status, v.created_at, v.updated_at,
		       t.id, t.extraction_method, t.original_format, t.language, t.artifact_key, t.cue_count, t.created_at
		FROM videos v
		LEFT JOIN transcripts t ON t.video_id = v.id
		WHERE v.id = $1`, id)
	return scanVideoWithTranscript(row)
}

// ListVideos returns the newest videos with transcript summaries.
func ListVideos(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]VideoWithTranscript, error) {
	rows, err := pool.Query(ctx, `
		SELECT v.id, v.source_type, v.source_ref, v.title, v.original_url,
		       v.thumbnail_url, v.duration_ms, v.channel, v.language, v.status, v.created_at, v.updated_at,
		       t.id, t.extraction_method, t.original_format, t.language, t.artifact_key, t.cue_count, t.created_at
		FROM videos v
		LEFT JOIN transcripts t ON t.video_id = v.id
		ORDER BY v.created_at DESC
		NULLS LAST
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("persistence: list videos: %w", err)
	}
	defer rows.Close()

	var out []VideoWithTranscript
	for rows.Next() {
		vt, err := scanVideoWithTranscript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, vt)
	}
	return out, rows.Err()
}

func scanVideoWithTranscript(row rowScanner) (VideoWithTranscript, error) {
	var v domain.Video
	var title, originalURL, thumbnail, channel, language *string
	var durationMs *int64

	var (
		t                     = domain.Transcript{}
		tID, tMethod, tFormat *string
		tLang, tArtifact      *string
		tCueCount             *int
		tCreated              *time.Time
	)
	err := row.Scan(
		&v.ID, &v.SourceType, &v.SourceRef, &title,
		&originalURL, &thumbnail, &durationMs, &channel, &language,
		&v.Status, &v.CreatedAt, &v.UpdatedAt,
		&tID, &tMethod, &tFormat, &tLang, &tArtifact, &tCueCount, &tCreated,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return VideoWithTranscript{}, ErrVideoNotFound
	}
	if err != nil {
		return VideoWithTranscript{}, fmt.Errorf("persistence: scan video+transcript: %w", err)
	}
	v.Title, v.OriginalURL, v.ThumbnailURL = title, originalURL, thumbnail
	v.DurationMs, v.Channel, v.Language = durationMs, channel, language

	vt := VideoWithTranscript{Video: v}
	if tID != nil {
		t.ID, t.VideoID = *tID, v.ID
		t.ExtractionMethod = domain.ExtractionMethod(deref(tMethod))
		t.OriginalFormat = deref(tFormat)
		t.Language, t.ArtifactKey = tLang, tArtifact
		t.CueCount, t.CreatedAt = derefInt(tCueCount), derefTime(tCreated)
		vt.Transcript = &t
	}
	return vt, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
