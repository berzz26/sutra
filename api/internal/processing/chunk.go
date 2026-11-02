package processing

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/google/uuid"
)

type Chunk struct {
	ID             string
	VideoID        string
	Sequence       int
	StartMs, EndMs int64
	Text           string
	WordCount      int
}
type ChunkConfig struct {
	Target, Min, Max, Overlap int
	Version                   string
}

var sentenceBreak = regexp.MustCompile(`([.!?])\s+`)

func BuildChunks(videoID string, cues []domain.TranscriptCue, cfg ChunkConfig) []Chunk {
	var sentences []domain.TranscriptCue
	for _, cue := range cues {
		text := strings.TrimSpace(cue.Text)
		if text == "" {
			continue
		}
		parts := sentenceBreak.Split(text, -1)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				sentences = append(sentences, domain.TranscriptCue{StartMs: cue.StartMs, EndMs: cue.EndMs, Text: p})
			}
		}
	}
	var out []Chunk
	var group []domain.TranscriptCue
	words := 0
	flush := func() {
		if len(group) == 0 {
			return
		}
		parts := make([]string, len(group))
		for i, c := range group {
			parts[i] = c.Text
		}
		text := strings.Join(parts, " ")
		out = append(out, Chunk{ID: uuid.NewString(), VideoID: videoID, Sequence: len(out) + 1, StartMs: group[0].StartMs, EndMs: group[len(group)-1].EndMs, Text: text, WordCount: len(strings.Fields(text))})
		group = nil
		words = 0
	}
	for _, s := range sentences {
		n := len(strings.Fields(s.Text))
		if len(group) > 0 && words+n > cfg.Max && words >= cfg.Min {
			flush()
		}
		group = append(group, s)
		words += n
		if words >= cfg.Target {
			flush()
		}
	}
	flush()
	if cfg.Overlap > 0 && len(out) > 1 {
		for i := 1; i < len(out); i++ {
			prior := strings.Fields(out[i-1].Text)
			if len(prior) > cfg.Overlap {
				prior = prior[len(prior)-cfg.Overlap:]
			}
			current := strings.Fields(out[i].Text)
			out[i].Text = strings.Join(append(prior, current...), " ")
			out[i].WordCount = len(strings.Fields(out[i].Text))
		}
	}
	return out
}

func wordCount(s string) int { return len(strings.FieldsFunc(s, unicode.IsSpace)) }
