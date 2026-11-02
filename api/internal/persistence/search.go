package persistence

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SearchCandidate struct {
	ChunkID     string
	VideoID     string
	VideoTitle  *string
	OriginalURL *string
	Text        string
	StartMs     int64
	EndMs       int64
	Score       float64
}

func SearchVector(ctx context.Context, pool *pgxpool.Pool, vector []float32, videoID string, limit int) ([]SearchCandidate, error) {
	if err := validateSearchVector(vector); err != nil {
		return nil, err
	}
	where := "c.embedding IS NOT NULL"
	args := []any{vectorLiteral(vector)}
	if videoID != "" {
		args = append(args, videoID)
		where += fmt.Sprintf(" AND c.video_id = $%d", len(args))
	}
	args = append(args, limit)
	query := `SELECT c.id, c.video_id, v.title, v.original_url, c.text, c.start_ms, c.end_ms,
				  c.embedding <=> $1::vector AS distance
			  FROM transcript_chunks c
			  JOIN videos v ON v.id = c.video_id
			 WHERE ` + where + fmt.Sprintf(`
			  ORDER BY c.embedding <=> $1::vector, c.id
			  LIMIT $%d`, len(args))
	return scanSearchCandidates(ctx, pool, query, args...)
}

func SearchLexical(ctx context.Context, pool *pgxpool.Pool, queryText, videoID string, limit int) ([]SearchCandidate, error) {
	where := "c.search_vector @@ websearch_to_tsquery('english', $1)"
	args := []any{queryText}
	if videoID != "" {
		args = append(args, videoID)
		where += fmt.Sprintf(" AND c.video_id = $%d", len(args))
	}
	args = append(args, limit)
	query := `SELECT c.id, c.video_id, v.title, v.original_url, c.text, c.start_ms, c.end_ms,
				  ts_rank_cd(c.search_vector, websearch_to_tsquery('english', $1)) AS rank
			  FROM transcript_chunks c
			  JOIN videos v ON v.id = c.video_id
			 WHERE ` + where + fmt.Sprintf(`
			  ORDER BY rank DESC, c.id
			  LIMIT $%d`, len(args))
	return scanSearchCandidates(ctx, pool, query, args...)
}

func scanSearchCandidates(ctx context.Context, pool *pgxpool.Pool, query string, args ...any) ([]SearchCandidate, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("persistence: search candidates: %w", err)
	}
	defer rows.Close()
	results := make([]SearchCandidate, 0)
	for rows.Next() {
		var candidate SearchCandidate
		if err := rows.Scan(&candidate.ChunkID, &candidate.VideoID, &candidate.VideoTitle, &candidate.OriginalURL, &candidate.Text, &candidate.StartMs, &candidate.EndMs, &candidate.Score); err != nil {
			return nil, fmt.Errorf("persistence: scan search candidate: %w", err)
		}
		results = append(results, candidate)
	}
	return results, rows.Err()
}

func validateSearchVector(vector []float32) error {
	if len(vector) != 384 {
		return fmt.Errorf("search: query vector has dimension %d, expected 384", len(vector))
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("search: query vector contains a non-finite value")
		}
	}
	return nil
}
