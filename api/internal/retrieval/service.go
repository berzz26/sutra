// Package retrieval implements hybrid lexical and semantic transcript search.
package retrieval

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/berzz/searcher/backend/internal/embedding"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmptyQuery     = errors.New("search query is empty")
	ErrQueryTooLong   = errors.New("search query is too long")
	ErrInvalidLimit   = errors.New("search result limit is invalid")
	ErrInvalidOffset  = errors.New("search result offset is invalid")
	ErrInvalidVideoID = errors.New("search video id is invalid")
)

const (
	MaxQueryLength = 2000
	MaxResultLimit = 50
)

type Config struct {
	CandidatePool int
	RRFK          int
	Timeout       time.Duration
}

type Service struct {
	pool     *pgxpool.Pool
	embedder *embedding.Embedder
	cfg      Config
}

type Result struct {
	ChunkID       string  `json:"chunk_id"`
	VideoID       string  `json:"video_id"`
	VideoTitle    *string `json:"video_title,omitempty"`
	OriginalURL   *string `json:"original_url,omitempty"`
	TimestampURL  *string `json:"timestamp_url,omitempty"`
	Text          string  `json:"text"`
	StartMs       int64   `json:"start_ms"`
	EndMs         int64   `json:"end_ms"`
	Score         float64 `json:"score"`
	SemanticScore float64 `json:"semantic_score,omitempty"`
	LexicalScore  float64 `json:"lexical_score,omitempty"`
	SemanticRank  int     `json:"semantic_rank,omitempty"`
	LexicalRank   int     `json:"lexical_rank,omitempty"`
}

type Candidate struct {
	ChunkID     string
	VideoID     string
	VideoTitle  *string
	OriginalURL *string
	Text        string
	StartMs     int64
	EndMs       int64
	Score       float64
}

func NewService(pool *pgxpool.Pool, embedder *embedding.Embedder, cfg Config) *Service {
	if cfg.CandidatePool <= 0 {
		cfg.CandidatePool = 100
	}
	if cfg.RRFK <= 0 {
		cfg.RRFK = 60
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Service{pool: pool, embedder: embedder, cfg: cfg}
}

func Validate(query string, limit, offset int, videoID string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", ErrEmptyQuery
	}
	if len([]rune(query)) > MaxQueryLength {
		return "", ErrQueryTooLong
	}
	if limit < 1 || limit > MaxResultLimit {
		return "", ErrInvalidLimit
	}
	if offset < 0 {
		return "", ErrInvalidOffset
	}
	if videoID != "" {
		if _, err := uuid.Parse(videoID); err != nil {
			return "", ErrInvalidVideoID
		}
	}
	return query, nil
}

func (s *Service) Search(ctx context.Context, query string, limit, offset int, videoID string) ([]Result, error) {
	query, err := Validate(query, limit, offset, videoID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	semantic, lexical, err := s.Candidates(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	return Fuse(semantic, lexical, s.cfg.RRFK, limit, offset), nil
}

// SearchIn searches within an explicit set of video IDs.
//
// The empty list is a real scope, not a wildcard: it matches nothing and
// returns zero results without touching the embedder or the database. This
// is the privacy invariant the browser-local library search relies on — an
// empty or malformed client library must never widen into a global search.
func (s *Service) SearchIn(ctx context.Context, query string, limit, offset int, videoIDs []string) ([]Result, error) {
	query, err := Validate(query, limit, offset, "")
	if err != nil {
		return nil, err
	}
	ids, err := ValidateVideoIDs(videoIDs)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Result{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	semantic, lexical, err := s.scopedCandidates(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	return Fuse(semantic, lexical, s.cfg.RRFK, limit, offset), nil
}

// Candidates returns raw semantic and lexical candidates for a query.
// An empty videoID means unrestricted retrieval; it is reserved for internal
// tooling — API handlers decide the scope.
func (s *Service) Candidates(ctx context.Context, query, videoID string) ([]Candidate, []Candidate, error) {
	ids := []string(nil)
	if videoID != "" {
		var err error
		if ids, err = ValidateVideoIDs([]string{videoID}); err != nil {
			return nil, nil, err
		}
	}
	return s.scopedCandidates(ctx, query, ids)
}

func (s *Service) scopedCandidates(ctx context.Context, query string, videoIDs []string) ([]Candidate, []Candidate, error) {
	query, err := Validate(query, 1, 0, "")
	if err != nil {
		return nil, nil, err
	}
	if len(videoIDs) > 0 {
		for _, id := range videoIDs {
			if _, err := uuid.Parse(id); err != nil {
				return nil, nil, fmt.Errorf("search: %w", ErrInvalidVideoID)
			}
		}
	}
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, nil, fmt.Errorf("search: embed query: %w", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 384 {
		return nil, nil, fmt.Errorf("search: query embedder returned an invalid vector")
	}
	vectorCandidates, err := persistence.SearchVector(ctx, s.pool, vectors[0], videoIDs, s.cfg.CandidatePool)
	if err != nil {
		return nil, nil, err
	}
	lexicalCandidates, err := persistence.SearchLexical(ctx, s.pool, query, videoIDs, s.cfg.CandidatePool)
	if err != nil {
		return nil, nil, err
	}

	semantic := make([]Candidate, len(vectorCandidates))
	for i, candidate := range vectorCandidates {
		semantic[i] = fromPersistenceCandidate(candidate)
		semantic[i].Score = 1 - candidate.Score
	}
	lexical := make([]Candidate, len(lexicalCandidates))
	for i, candidate := range lexicalCandidates {
		lexical[i] = fromPersistenceCandidate(candidate)
	}
	return semantic, lexical, nil
}

// ValidateVideoIDs trims, deduplicates and validates video IDs. Values must
// be UUIDs; the returned slice is safe to bind as a PostgreSQL uuid[].
func ValidateVideoIDs(videoIDs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(videoIDs))
	ids := make([]string, 0, len(videoIDs))
	for _, raw := range videoIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("%w: %q is not a valid video id", ErrInvalidVideoID, raw)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func fromPersistenceCandidate(candidate persistence.SearchCandidate) Candidate {
	return Candidate{ChunkID: candidate.ChunkID, VideoID: candidate.VideoID, VideoTitle: candidate.VideoTitle, OriginalURL: candidate.OriginalURL, Text: candidate.Text, StartMs: candidate.StartMs, EndMs: candidate.EndMs, Score: candidate.Score}
}

func Fuse(semantic, lexical []Candidate, rrfK, limit, offset int) []Result {
	if rrfK <= 0 {
		rrfK = 60
	}
	type merged struct {
		result       Result
		semanticRank int
		lexicalRank  int
	}
	mergedByID := make(map[string]*merged, len(semantic)+len(lexical))
	for rank, candidate := range semantic {
		item := mergedByID[candidate.ChunkID]
		if item == nil {
			item = &merged{result: baseResult(candidate)}
			mergedByID[candidate.ChunkID] = item
		}
		if item.semanticRank == 0 {
			item.semanticRank = rank + 1
			item.result.SemanticRank = rank + 1
			item.result.SemanticScore = candidate.Score
			item.result.Score += 1 / float64(rrfK+rank+1)
		}
	}
	for rank, candidate := range lexical {
		item := mergedByID[candidate.ChunkID]
		if item == nil {
			item = &merged{result: baseResult(candidate)}
			mergedByID[candidate.ChunkID] = item
		}
		if item.lexicalRank == 0 {
			item.lexicalRank = rank + 1
			item.result.LexicalRank = rank + 1
			item.result.LexicalScore = candidate.Score
			item.result.Score += 1 / float64(rrfK+rank+1)
		}
	}
	all := make([]*merged, 0, len(mergedByID))
	for _, item := range mergedByID {
		all = append(all, item)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].result.Score != all[j].result.Score {
			return all[i].result.Score > all[j].result.Score
		}
		if all[i].semanticRank != all[j].semanticRank {
			return rankOrMax(all[i].semanticRank) < rankOrMax(all[j].semanticRank)
		}
		if all[i].lexicalRank != all[j].lexicalRank {
			return rankOrMax(all[i].lexicalRank) < rankOrMax(all[j].lexicalRank)
		}
		return all[i].result.ChunkID < all[j].result.ChunkID
	})
	if offset >= len(all) {
		return []Result{}
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}
	results := make([]Result, len(all))
	for i, item := range all {
		results[i] = item.result
	}
	return results
}

func baseResult(candidate Candidate) Result {
	return Result{ChunkID: candidate.ChunkID, VideoID: candidate.VideoID, VideoTitle: candidate.VideoTitle, OriginalURL: candidate.OriginalURL, TimestampURL: timestampURL(candidate.OriginalURL, candidate.StartMs), Text: candidate.Text, StartMs: candidate.StartMs, EndMs: candidate.EndMs}
}

func timestampURL(original *string, startMs int64) *string {
	if original == nil || strings.TrimSpace(*original) == "" || startMs < 0 {
		return nil
	}
	parsed, err := url.Parse(*original)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil
	}
	query := parsed.Query()
	query.Set("t", strconv.FormatInt(startMs/1000, 10))
	parsed.RawQuery = query.Encode()
	value := parsed.String()
	return &value
}

func rankOrMax(rank int) int {
	if rank == 0 {
		return int(^uint(0) >> 1)
	}
	return rank
}
