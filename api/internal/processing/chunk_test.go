package processing

import (
	"testing"

	"github.com/berzz/searcher/backend/internal/domain"
)

func TestBuildChunksPreservesCuesAndIsDeterministic(t *testing.T) {
	cues := []domain.TranscriptCue{
		{Seq: 1, StartMs: 100, EndMs: 200, Text: " one\n two "},
		{Seq: 2, StartMs: 300, EndMs: 400, Text: "three."},
		{Seq: 3, StartMs: 500, EndMs: 600, Text: "   "},
	}
	cfg := ChunkConfig{Target: 3, Min: 2, Max: 4, Overlap: 0, Version: "v1"}
	first := BuildChunks("video", cues, cfg)
	second := BuildChunks("video", cues, cfg)
	if len(first) != 1 || first[0].Text != "one two three." || first[0].StartMs != 100 || first[0].EndMs != 400 {
		t.Fatalf("unexpected chunks: %#v", first)
	}
	if first[0].ID != second[0].ID || first[0].Text != second[0].Text {
		t.Fatalf("chunking is not deterministic: %#v %#v", first, second)
	}
}

func TestBuildChunksLongCueAndOverlap(t *testing.T) {
	cues := make([]domain.TranscriptCue, 0, 5)
	for i := 0; i < 5; i++ {
		cues = append(cues, domain.TranscriptCue{Seq: i + 1, StartMs: int64(i * 1000), EndMs: int64(i*1000 + 900), Text: "one two three four five"})
	}
	chunks := BuildChunks("video", cues, ChunkConfig{Target: 10, Min: 5, Max: 10, Overlap: 5, Version: "v1"})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[1].StartMs != 1000 || chunks[1].EndMs != 3900 {
		t.Fatalf("overlap timestamps should include the complete previous cue: %#v", chunks[1])
	}
	for _, chunk := range chunks {
		if chunk.Text == "" || chunk.WordCount == 0 {
			t.Fatalf("empty chunk: %#v", chunk)
		}
	}
}

func TestBuildChunksShortAndEmpty(t *testing.T) {
	chunks := BuildChunks("video", []domain.TranscriptCue{{StartMs: 1, EndMs: 2, Text: "short"}, {Text: " "}}, ChunkConfig{Target: 180, Min: 80, Max: 250, Version: "v1"})
	if len(chunks) != 1 || chunks[0].Text != "short" {
		t.Fatalf("short transcript should remain searchable: %#v", chunks)
	}
	if got := BuildChunks("video", []domain.TranscriptCue{{Text: "\n\t"}}, ChunkConfig{}); len(got) != 0 {
		t.Fatalf("empty cues should produce no chunks: %#v", got)
	}
}
