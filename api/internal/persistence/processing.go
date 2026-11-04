package persistence

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProcessingState struct {
	VideoID           string  `json:"video_id"`
	Status            string  `json:"status"`
	Stage             string  `json:"stage"`
	ProgressPercent   int     `json:"progress_percent"`
	CueCount          int     `json:"cue_count"`
	EmbeddingCount    int     `json:"embedding_count"`
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
	rows, err := p.Query(ctx, `SELECT id,video_id,seq,start_ms,end_ms,text FROM transcript_cues WHERE video_id=$1 ORDER BY seq`, id)
	if err != nil {
		return nil, fmt.Errorf("persistence: load cues: %w", err)
	}
	defer rows.Close()
	var cues []domain.TranscriptCue
	for rows.Next() {
		var c domain.TranscriptCue
		if err := rows.Scan(&c.ID, &c.VideoID, &c.Seq, &c.StartMs, &c.EndMs, &c.Text); err != nil {
			return nil, fmt.Errorf("persistence: scan cue: %w", err)
		}
		cues = append(cues, c)
	}
	return cues, rows.Err()
}

func SaveChunks(ctx context.Context, p *pgxpool.Pool, videoID, version, model string, chunks []ChunkRecord, vectors [][]float32, cueCount int) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunk/vector length mismatch")
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("persistence: begin chunk save: %w", err)
	}
	defer tx.Rollback(ctx)

	if len(chunks) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM transcript_chunks WHERE video_id=$1`, videoID); err != nil {
			return fmt.Errorf("persistence: clear empty chunk set: %w", err)
		}
	} else {
		parts := make([]string, 0, len(chunks))
		args := make([]any, 0, len(chunks)*10)
		for i, c := range chunks {
			if len(vectors[i]) != 384 {
				return fmt.Errorf("persistence: vector %d has dimension %d, expected 384", i, len(vectors[i]))
			}
			for _, value := range vectors[i] {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					return fmt.Errorf("persistence: vector %d contains a non-finite value", i)
				}
			}
			base := i*10 + 1
			parts = append(parts, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d::vector)", base, base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9))
			args = append(args, c.ID, c.VideoID, c.Sequence, c.StartMs, c.EndMs, c.Text, c.WordCount, version, model, vectorLiteral(vectors[i]))
		}
		_, err = tx.Exec(ctx, `INSERT INTO transcript_chunks(id,video_id,seq,start_ms,end_ms,text,word_count,processing_version,embedding_model,embedding) VALUES `+strings.Join(parts, ",")+` ON CONFLICT (video_id,seq) DO UPDATE SET id=EXCLUDED.id,start_ms=EXCLUDED.start_ms,end_ms=EXCLUDED.end_ms,text=EXCLUDED.text,word_count=EXCLUDED.word_count,processing_version=EXCLUDED.processing_version,embedding_model=EXCLUDED.embedding_model,embedding=EXCLUDED.embedding`, args...)
		if err != nil {
			return fmt.Errorf("persistence: upsert chunks: %w", err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM transcript_chunks WHERE video_id=$1 AND seq > $2`, videoID, len(chunks)); err != nil {
			return fmt.Errorf("persistence: remove stale chunks: %w", err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO video_processing(video_id,status,stage,progress_percent,processing_version,model_version,last_error,chunk_count,cue_count,embedding_count,started_at,completed_at,updated_at) VALUES($1,'completed','complete',100,$2,$3,NULL,$4,$5,$6,now(),now(),now()) ON CONFLICT(video_id) DO UPDATE SET status='completed',stage='complete',progress_percent=100,processing_version=EXCLUDED.processing_version,model_version=EXCLUDED.model_version,last_error=NULL,chunk_count=EXCLUDED.chunk_count,cue_count=EXCLUDED.cue_count,embedding_count=EXCLUDED.embedding_count,completed_at=now(),updated_at=now()`, videoID, version, model, len(chunks), cueCount, len(vectors)); err != nil {
		return fmt.Errorf("persistence: mark processing complete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("persistence: commit chunks: %w", err)
	}
	return nil
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmt.Sprintf("%.9g", x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func SetProcessing(ctx context.Context, p *pgxpool.Pool, id, status, version, model string, lastErr *string) error {
	stage, progress := "queued", 0
	if status == "processing" {
		stage, progress = "chunking", 55
	} else if status == "failed" {
		stage = "failed"
	}
	_, err := p.Exec(ctx, `INSERT INTO video_processing(video_id,status,stage,progress_percent,processing_version,model_version,last_error,started_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $2='processing' THEN now() END,now()) ON CONFLICT(video_id) DO UPDATE SET status=EXCLUDED.status,stage=EXCLUDED.stage,progress_percent=EXCLUDED.progress_percent,processing_version=EXCLUDED.processing_version,model_version=EXCLUDED.model_version,last_error=EXCLUDED.last_error,started_at=CASE WHEN EXCLUDED.status='processing' THEN now() ELSE video_processing.started_at END,updated_at=now()`, id, status, stage, progress, version, model, lastErr)
	return err
}

func UpdateProcessingProgress(ctx context.Context, p *pgxpool.Pool, id, stage string, progress, cues, embeddings int) error {
	_, err := p.Exec(ctx, `UPDATE video_processing SET stage=$2,progress_percent=$3,cue_count=$4,embedding_count=$5,updated_at=now() WHERE video_id=$1`, id, stage, progress, cues, embeddings)
	return err
}

func GetProcessing(ctx context.Context, p *pgxpool.Pool, id string) (ProcessingState, error) {
	var s ProcessingState
	err := p.QueryRow(ctx, `SELECT video_id,status,stage,progress_percent,cue_count,embedding_count,processing_version,model_version,last_error,chunk_count FROM video_processing WHERE video_id=$1`, id).Scan(&s.VideoID, &s.Status, &s.Stage, &s.ProgressPercent, &s.CueCount, &s.EmbeddingCount, &s.ProcessingVersion, &s.ModelVersion, &s.LastError, &s.ChunkCount)
	if errors.Is(err, pgx.ErrNoRows) {
		var videoStatus string
		if videoErr := p.QueryRow(ctx, `SELECT status FROM videos WHERE id=$1`, id).Scan(&videoStatus); videoErr != nil {
			if errors.Is(videoErr, pgx.ErrNoRows) {
				return s, ErrVideoNotFound
			}
			return s, fmt.Errorf("persistence: get video status: %w", videoErr)
		}
		return ProcessingState{VideoID: id, Status: "pending", Stage: "queued", ProgressPercent: 40}, nil
	}
	return s, err
}

func EnqueueEmbedding(ctx context.Context, p *pgxpool.Pool, videoID string) error {
	_, err := p.Exec(ctx, `INSERT INTO jobs(video_id,kind,status,run_after) VALUES($1,'embed','queued',now()) ON CONFLICT DO NOTHING`, videoID)
	return err
}

func EnqueueBackfill(ctx context.Context, p *pgxpool.Pool) (int64, error) {
	tag, err := p.Exec(ctx, `INSERT INTO jobs(video_id,kind,status,run_after) SELECT v.id,'embed','queued',now() FROM videos v WHERE EXISTS (SELECT 1 FROM transcript_cues c WHERE c.video_id=v.id) AND NOT EXISTS (SELECT 1 FROM transcript_chunks c WHERE c.video_id=v.id) AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.video_id=v.id AND j.kind='embed' AND j.status IN ('queued','running')) ON CONFLICT DO NOTHING`)
	return tag.RowsAffected(), err
}

func ClaimEmbeddingJob(ctx context.Context, p *pgxpool.Pool, workerID string) (domain.Job, error) {
	tx, err := p.Begin(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	defer tx.Rollback(ctx)
	var job domain.Job
	err = tx.QueryRow(ctx, `SELECT id,video_id,kind,status,attempts,last_error,created_at,updated_at,started_at,finished_at FROM jobs WHERE kind='embed' AND status='queued' AND run_after <= now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&job.ID, &job.VideoID, &job.Kind, &job.Status, &job.Attempts, &job.LastError, &job.CreatedAt, &job.UpdatedAt, &job.StartedAt, &job.FinishedAt)
	if err != nil {
		return domain.Job{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status='running',attempts=attempts+1,started_at=now(),updated_at=now() WHERE id=$1`, job.ID); err != nil {
		return domain.Job{}, err
	}
	job.Status = domain.JobStatusRunning
	job.Attempts++
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func FinishJob(ctx context.Context, p *pgxpool.Pool, id string, success bool, cause error, retryAt *time.Time) error {
	status := "succeeded"
	if !success {
		status = "failed"
		if retryAt != nil {
			status = "queued"
		}
	}
	var message *string
	if cause != nil {
		value := cause.Error()
		message = &value
	}
	_, err := p.Exec(ctx, `UPDATE jobs SET status=$2,last_error=$3,finished_at=CASE WHEN $2 IN ('succeeded','failed') THEN now() ELSE NULL END,updated_at=now(),run_after=COALESCE($4,run_after) WHERE id=$1`, id, status, message, retryAt)
	return err
}
