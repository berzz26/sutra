package processing

import (
	"fmt"
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

// BuildChunks groups complete cues. Overlap is made from complete cues too,
// so no subtitle cue is ever cut in the middle of a chunk.
func BuildChunks(videoID string, cues []domain.TranscriptCue, cfg ChunkConfig) []Chunk {
	cfg = normalizedChunkConfig(cfg)
	clean := make([]domain.TranscriptCue, 0, len(cues))
	for _, cue := range cues {
		text := normalizeCueText(cue.Text)
		if text == "" {
			continue
		}
		cue.VideoID = videoID
		cue.Text = text
		clean = append(clean, cue)
	}
	if len(clean) == 0 {
		return nil
	}

	cores := make([][]domain.TranscriptCue, 0)
	current := make([]domain.TranscriptCue, 0)
	words := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		cores = append(cores, current)
		current = nil
		words = 0
	}
	for _, cue := range clean {
		n := wordCount(cue.Text)
		if len(current) > 0 && words+n > cfg.Max && words >= cfg.Min {
			flush()
		}
		current = append(current, cue)
		words += n
		if words >= cfg.Target {
			flush()
		}
	}
	flush()

	out := make([]Chunk, 0, len(cores))
	for i, core := range cores {
		members := core
		if i > 0 && cfg.Overlap > 0 {
			overlap := overlapCues(cores[i-1], cfg.Overlap)
			members = make([]domain.TranscriptCue, 0, len(overlap)+len(core))
			members = append(members, overlap...)
			members = append(members, core...)
		}
		text := joinCueText(members)
		if text == "" {
			continue
		}
		start, end := members[0].StartMs, members[len(members)-1].EndMs
		sequence := len(out) + 1
		id := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d\x00%s", videoID, cfg.Version, sequence, start, end, text))).String()
		out = append(out, Chunk{ID: id, VideoID: videoID, Sequence: sequence, StartMs: start, EndMs: end, Text: text, WordCount: wordCount(text)})
	}
	return out
}

func normalizedChunkConfig(cfg ChunkConfig) ChunkConfig {
	if cfg.Target <= 0 {
		cfg.Target = 180
	}
	if cfg.Min <= 0 {
		cfg.Min = 80
	}
	if cfg.Max < cfg.Target {
		cfg.Max = cfg.Target
	}
	if cfg.Overlap < 0 {
		cfg.Overlap = 0
	}
	return cfg
}

func overlapCues(previous []domain.TranscriptCue, target int) []domain.TranscriptCue {
	if target <= 0 {
		return nil
	}
	words := 0
	start := len(previous)
	for start > 0 && words < target {
		start--
		words += wordCount(previous[start].Text)
	}
	return previous[start:]
}

func normalizeCueText(text string) string {
	return strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ")
}

func joinCueText(cues []domain.TranscriptCue) string {
	parts := make([]string, 0, len(cues))
	for _, cue := range cues {
		if text := normalizeCueText(cue.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func wordCount(s string) int { return len(strings.FieldsFunc(s, unicode.IsSpace)) }
