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
