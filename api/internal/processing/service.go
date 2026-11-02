package processing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/berzz/searcher/backend/internal/embedding"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	pool     *pgxpool.Pool
	embedder *embedding.Embedder
	cfg      ChunkConfig
	model    string
	timeout  time.Duration
}

func NewService(pool *pgxpool.Pool, e *embedding.Embedder, cfg ChunkConfig, model string, timeout time.Duration) *Service {
	return &Service{pool: pool, embedder: e, cfg: cfg, model: model, timeout: timeout}
}
func (s *Service) Process(ctx context.Context, id string) error {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if state, err := persistence.GetProcessing(ctx, s.pool, id); err == nil && state.Status == "completed" && state.ProcessingVersion == s.cfg.Version && state.ModelVersion == s.model {
		slog.Info("transcript processing already complete", "video_id", id, "chunks", state.ChunkCount)
		return nil
	}
	if err := persistence.SetProcessing(ctx, s.pool, id, "processing", s.cfg.Version, s.model, nil); err != nil {
		return err
	}
	cues, err := persistence.LoadCues(ctx, s.pool, id)
	if err != nil {
		return s.fail(ctx, id, err)
	}
	slog.Info("transcript processing cues loaded", "video_id", id, "cues", len(cues))
	chunks := BuildChunks(id, cues, s.cfg)
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return s.fail(ctx, id, err)
	}
	records := make([]persistence.ChunkRecord, len(chunks))
	for i, c := range chunks {
		records[i] = persistence.ChunkRecord{ID: c.ID, VideoID: c.VideoID, Sequence: c.Sequence, StartMs: c.StartMs, EndMs: c.EndMs, Text: c.Text, WordCount: c.WordCount}
	}
	if err = persistence.SaveChunks(ctx, s.pool, id, s.cfg.Version, s.model, records, vectors); err != nil {
		return s.fail(ctx, id, err)
	}
	slog.Info("transcript processing completed", "video_id", id, "cues", len(cues), "chunks", len(chunks), "embeddings", len(vectors), "duration", time.Since(started))
	return nil
}
func (s *Service) fail(ctx context.Context, id string, cause error) error {
	msg := cause.Error()
	_ = persistence.SetProcessing(context.WithoutCancel(ctx), s.pool, id, "failed", s.cfg.Version, s.model, &msg)
	return fmt.Errorf("processing video: %w", cause)
}
func (s *Service) List(ctx context.Context, id string, limit, offset int) ([]domain.TranscriptChunk, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,video_id,start_ms,end_ms,text,created_at FROM transcript_chunks WHERE video_id=$1 ORDER BY seq LIMIT $2 OFFSET $3`, id, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TranscriptChunk{}
	for rows.Next() {
		var c domain.TranscriptChunk
		if err := rows.Scan(&c.ID, &c.VideoID, &c.StartMs, &c.EndMs, &c.Text, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) State(ctx context.Context, id string) (persistence.ProcessingState, error) {
	return persistence.GetProcessing(ctx, s.pool, id)
}
