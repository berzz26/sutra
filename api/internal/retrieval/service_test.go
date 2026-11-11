package retrieval

import (
	"errors"
	"testing"
)

func TestValidateSearchRequest(t *testing.T) {
	if _, err := Validate("  ", 10, 0, ""); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected empty query error, got %v", err)
	}
	if _, err := Validate("ok", 0, 0, ""); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected zero limit error, got %v", err)
	}
	if _, err := Validate("ok", 51, 0, ""); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected limit error, got %v", err)
	}
	if _, err := Validate("ok", 10, 0, "not-a-uuid"); !errors.Is(err, ErrInvalidVideoID) {
		t.Fatalf("expected video ID error, got %v", err)
	}
}

func TestFuseUsesRRFAndDeduplicates(t *testing.T) {
	semantic := []Candidate{{ChunkID: "a", VideoID: "v", Text: "A"}, {ChunkID: "b", VideoID: "v", Text: "B"}}
	lexical := []Candidate{{ChunkID: "b", VideoID: "v", Text: "B"}, {ChunkID: "c", VideoID: "v", Text: "C"}}
	results := Fuse(semantic, lexical, 1, 10, 0)
	if len(results) != 3 {
		t.Fatalf("expected three unique results, got %d", len(results))
	}
	if results[0].ChunkID != "b" || results[0].SemanticRank != 2 || results[0].LexicalRank != 1 {
		t.Fatalf("expected fused duplicate to rank first, got %#v", results[0])
	}
	if results[1].ChunkID != "a" || results[2].ChunkID != "c" {
		t.Fatalf("unexpected deterministic order: %#v", results)
	}
}

func TestFuseOffsetAndLimit(t *testing.T) {
	results := Fuse([]Candidate{{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"}}, nil, 60, 1, 1)
	if len(results) != 1 || results[0].ChunkID != "b" {
		t.Fatalf("unexpected page: %#v", results)
	}
}

func TestTimestampURLPreservesQueryAndUsesChunkStart(t *testing.T) {
	original := "https://www.youtube.com/watch?v=JjDjNgkFo&si=abc"
	got := timestampURL(&original, 9_105_200)
	if got == nil || *got != "https://www.youtube.com/watch?si=abc&t=9105&v=JjDjNgkFo" {
		t.Fatalf("unexpected timestamp URL: %v", got)
	}
}

func TestTimestampURLRejectsInvalidURL(t *testing.T) {
	original := "not a URL"
	if got := timestampURL(&original, 1000); got != nil {
		t.Fatalf("expected nil timestamp URL, got %q", *got)
	}
}

func TestValidateVideoIDsTrimsDeduplicatesAndValidates(t *testing.T) {
	ids, err := ValidateVideoIDs([]string{"  00000000-0000-0000-0000-000000000001  ", "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "00000000-0000-0000-0000-000000000001" || ids[1] != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestValidateVideoIDsRejectsNonUUIDs(t *testing.T) {
	if _, err := ValidateVideoIDs([]string{"00000000-0000-0000-0000-000000000001", "not-a-uuid"}); !errors.Is(err, ErrInvalidVideoID) {
		t.Fatalf("expected invalid video id error, got %v", err)
	}
}

// The privacy invariant: an empty scope is a real scope that matches nothing.
// It must return zero results without embedding the query or touching the
// database (a nil pool/embedder is never dereferenced on this path).
func TestSearchInWithEmptyScopeReturnsNoResults(t *testing.T) {
	service := NewService(nil, nil, Config{})
	results, err := service.SearchIn(t.Context(), "database replication", 10, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected zero results for an empty scope, got %d", len(results))
	}
}

func TestSearchInWithBlankScopeReturnsNoResults(t *testing.T) {
	service := NewService(nil, nil, Config{})
	results, err := service.SearchIn(t.Context(), "database replication", 10, 0, []string{"", "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected zero results for a blank scope, got %d", len(results))
	}
}

func TestSearchInRejectsInvalidScopedIDs(t *testing.T) {
	service := NewService(nil, nil, Config{})
	if _, err := service.SearchIn(t.Context(), "query", 10, 0, []string{"not-a-uuid"}); !errors.Is(err, ErrInvalidVideoID) {
		t.Fatalf("expected invalid video id error, got %v", err)
	}
}

func TestSearchInStillValidatesQuery(t *testing.T) {
	service := NewService(nil, nil, Config{})
	if _, err := service.SearchIn(t.Context(), "  ", 10, 0, []string{"00000000-0000-0000-0000-000000000001"}); !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("expected empty query error, got %v", err)
	}
}
