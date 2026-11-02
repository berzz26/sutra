package transcript

import (
	"fmt"
	"strings"
)

// splitLines normalizes CRLF/LF line endings and splits into lines.
func splitLines(data []byte) []string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

// intoBlocks groups lines into blank-line-separated blocks (cue candidates).
func intoBlocks(lines []string) [][]string {
	var (
		blocks [][]string
		cur    []string
	)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(cur) > 0 {
				blocks = append(blocks, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	return blocks
}

// timestampsLine returns the index of the line containing a cue timing, or -1.
func timestampsLine(lines []string) int {
	for i, line := range lines {
		if strings.Contains(line, "-->") {
			return i
		}
	}
	return -1
}

// parseCueTiming splits a "start --> end [settings]" line into milliseconds.
func parseCueTiming(line string) (startMs, endMs int64, err error) {
	sep := strings.Index(line, "-->")
	if sep < 0 {
		return 0, 0, fmt.Errorf("%w: missing -->", ErrInvalidTimestamp)
	}
	startStr := strings.TrimSpace(line[:sep])
	fields := strings.Fields(strings.TrimSpace(line[sep+3:]))
	if len(fields) == 0 {
		return 0, 0, fmt.Errorf("%w: missing end timestamp in %q", ErrInvalidTimestamp, line)
	}
	start, err := parseTimestamp(startStr)
	if err != nil {
		return 0, 0, fmt.Errorf("start timestamp in %q: %w", line, err)
	}
	end, err := parseTimestamp(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("end timestamp in %q: %w", line, err)
	}
	if start < 0 || end < start {
		return 0, 0, fmt.Errorf("%w: end before start in %q", ErrInvalidTimestamp, line)
	}
	return start, end, nil
}

// collectCues turns candidate blocks into normalized cues, dropping empty
// text and zero-length cues, and failing explicitly on malformed timestamps.
func collectCues(blocks [][]string) ([]Cue, error) {
	var out []Cue
	for _, block := range blocks {
		if isMetadataBlock(block) {
			continue
		}
		idx := timestampsLine(block)
		if idx < 0 {
			continue // header (WEBVTT), cue identifiers, NOTE/STYLE metadata
		}
		start, end, err := parseCueTiming(block[idx])
		if err != nil {
			return nil, err
		}
		text := normalizeText(strings.Join(block[idx+1:], " "))
		if end == start {
			return nil, fmt.Errorf("%w: cue end must be after cue start", ErrInvalidTimestamp)
		}
		if text == "" {
			continue
		}
		out = append(out, Cue{StartMs: start, EndMs: end, Text: text})
	}
	return out, nil
}

func isMetadataBlock(block []string) bool {
	if len(block) == 0 {
		return true
	}
	first := strings.ToUpper(strings.TrimSpace(block[0]))
	return first == "NOTE" || first == "STYLE" || first == "REGION" || strings.HasPrefix(first, "WEBVTT")
}

// parseVTT and parseSRT share the same block algorithm: both formats separate
// cues with blank lines and contain "start --> end" timing lines that follow
// an optional identifier (numeric for SRT, free-form for VTT).
func parseVTT(data []byte) ([]Cue, error) {
	lines := splitLines(data)
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(lines[0], "\ufeff")), "WEBVTT") {
		return nil, fmt.Errorf("transcript: invalid WebVTT header")
	}
	return collectCues(intoBlocks(lines))
}

func parseSRT(data []byte) ([]Cue, error) {
	blocks := intoBlocks(splitLines(data))
	for _, block := range blocks {
		if len(block) > 0 && numericLine(strings.TrimSpace(block[0])) && timestampsLine(block) < 0 {
			return nil, fmt.Errorf("%w: SRT cue %q has no timing line", ErrInvalidTimestamp, block[0])
		}
	}
	return collectCues(blocks)
}

func numericLine(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
