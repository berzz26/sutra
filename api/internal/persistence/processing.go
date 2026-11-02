package persistence

import (
	"context"
	"fmt"
	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProcessingState struct {
	VideoID           string  `json:"video_id"`
	Status            string  `json:"status"`
	ProcessingVersion string  `json:"processing_version"`
	ModelVersion      string  `json:"model_version"`
	LastError         *string `json:"last_error,omitempty"`
	ChunkCount        int     `json:"chunk_count"`
}
type ChunkRecord struct {
	ID, VideoID    string
	Sequence       int
	StartMs, EndMs int64
	Text           string
	WordCount      int
}

func LoadCues(ctx context.Context, p *pgxpool.Pool, id string) ([]domain.TranscriptCue, error) {
	rows, err := p.Query(ctx, "SELECT id,video_id,seq,start_ms,end_ms,text FROM transcript_cues WHERE video_id=$1 ORDER BY seq", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cues []domain.TranscriptCue
	for rows.Next() {
		var c domain.TranscriptCue
		if err := rows.Scan(&c.ID, &c.VideoID, &c.Seq, &c.StartMs, &c.EndMs, &c.Text); err != nil {
			return nil, err
		}
		cues = append(cues, c)
	}
	return cues, rows.Err()
}

func SaveChunks(ctx context.Context, p *pgxpool.Pool, videoID, version, model string, chunks []ChunkRecord, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunk/vector length mismatch")
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM transcript_chunks WHERE video_id=$1`, videoID); err != nil {
		return err
	}
	for i, c := range chunks {
		vals := make([]float32, len(vectors[i]))
		copy(vals, vectors[i])
		if _, err = tx.Exec(ctx, `INSERT INTO transcript_chunks(id,video_id,seq,start_ms,end_ms,text,word_count,processing_version,embedding_model,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::vector)`, c.ID, c.VideoID, c.Sequence, c.StartMs, c.EndMs, c.Text, c.WordCount, version, model, vectorLiteral(vals)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO video_processing(video_id,status,processing_version,model_version,last_error,chunk_count,started_at,completed_at,updated_at) VALUES($1,'completed',$2,$3,NULL,$4,now(),now(),now()) ON CONFLICT(video_id) DO UPDATE SET status='completed',processing_version=EXCLUDED.processing_version,model_version=EXCLUDED.model_version,last_error=NULL,chunk_count=EXCLUDED.chunk_count,completed_at=now(),updated_at=now()`, videoID, version, model, len(chunks)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func vectorLiteral(v []float32) string {
	s := "["
	for i, x := range v {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%.8f", x)
	}
	return s + "]"
}
func SetProcessing(ctx context.Context, p *pgxpool.Pool, id, status, version, model string, lastErr *string) error {
	_, err := p.Exec(ctx, `INSERT INTO video_processing(video_id,status,processing_version,model_version,last_error,started_at,updated_at) VALUES($1,$2,$3,$4,$5,CASE WHEN $2='processing' THEN now() END,now()) ON CONFLICT(video_id) DO UPDATE SET status=EXCLUDED.status,processing_version=EXCLUDED.processing_version,model_version=EXCLUDED.model_version,last_error=EXCLUDED.last_error,started_at=CASE WHEN EXCLUDED.status='processing' THEN now() ELSE video_processing.started_at END,updated_at=now()`, id, status, version, model, lastErr)
	return err
}
func GetProcessing(ctx context.Context, p *pgxpool.Pool, id string) (ProcessingState, error) {
	var s ProcessingState
	err := p.QueryRow(ctx, `SELECT video_id,status,processing_version,model_version,last_error,chunk_count FROM video_processing WHERE video_id=$1`, id).Scan(&s.VideoID, &s.Status, &s.ProcessingVersion, &s.ModelVersion, &s.LastError, &s.ChunkCount)
	return s, err
}
