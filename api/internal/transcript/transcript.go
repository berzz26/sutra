// Package transcript parses and normalizes WebVTT and SubRip subtitle files
// into raw cues. It deliberately has no dependencies on HTTP, storage or the
// database.
package transcript

import (
	"errors"
	"fmt"
)

// Cue is a single parsed subtitle cue with original timing preserved.
type Cue struct {
	Seq     int
	StartMs int64
	EndMs   int64
	Text    string
}

// Format identifies a subtitle file format.
type Format string

const (
	FormatVTT Format = "vtt"
	FormatSRT Format = "srt"
)

var (
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	ErrEmptyTranscript  = errors.New("empty transcript")
)

// Parse parses a subtitle document into normalized cues. Cues are renumbered
// 1..N after normalization; original timings are preserved.
func Parse(format Format, data []byte) ([]Cue, error) {
	var (
		cues []Cue
		err  error
	)
	switch format {
	case FormatVTT:
		cues, err = parseVTT(data)
	case FormatSRT:
		cues, err = parseSRT(data)
	default:
		return nil, fmt.Errorf("transcript: unsupported format %q", format)
	}
	if err != nil {
		return nil, err
	}
	for i := range cues {
		cues[i].Seq = i + 1
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("transcript: %w (no usable cues found in %s)", ErrEmptyTranscript, format)
	}
	return cues, nil
}

// parseTimestamp accepts VTT (HH:MM:SS.mmm / MM:SS.mmm with '.')
// and SRT (with ',') timestamps; both separators are leniently accepted.
func parseTimestamp(s string) (int64, error) {
	timePart, fracPart, ok := splitFraction(s)
	if !ok {
		return 0, fmt.Errorf("%w: missing milliseconds in %q", ErrInvalidTimestamp, s)
	}
	parts := splitDigits(timePart, ':')
	if parts == nil || len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidTimestamp, s)
	}

	values := make([]int64, len(parts))
	for i, p := range parts {
		v, err := parseUnsigned(p)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidTimestamp, s)
		}
		values[i] = v
	}

	var h, m, sec int64
	if len(parts) == 3 {
		h, m, sec = values[0], values[1], values[2]
	} else {
		m, sec = values[0], values[1]
	}
	if m >= 60 || sec >= 60 {
		return 0, fmt.Errorf("%w: out-of-range component in %q", ErrInvalidTimestamp, s)
	}

	fracMs, err := fractionMillis(fracPart)
	if err != nil {
		return 0, fmt.Errorf("%w in %q: %v", ErrInvalidTimestamp, s, err)
	}
	return h*3_600_000 + m*60_000 + sec*1_000 + fracMs, nil
}

// splitFraction splits seconds and fraction digits on '.' or ','.
func splitFraction(s string) (timePart string, fracPart string, ok bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ',' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// splitDigits splits s on sep into digit-only groups; nil if malformed.
func splitDigits(s string, sep byte) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] != sep {
			continue
		}
		group := s[start:i]
		if group == "" {
			return nil
		}
		for j := 0; j < len(group); j++ {
			if group[j] < '0' || group[j] > '9' {
				return nil
			}
		}
		out = append(out, group)
		start = i + 1
	}
	return out
}

func parseUnsigned(digits string) (int64, error) {
	var v int64
	for i := 0; i < len(digits); i++ {
		d := int64(digits[i] - '0')
		if digits[i] < '0' || digits[i] > '9' {
			return 0, fmt.Errorf("%w: non-digit value", ErrInvalidTimestamp)
		}
		v = v*10 + d
		if v > 300_000_000_000 { // ~83 hours in ms sanity cap
			return 0, fmt.Errorf("%w: value too large", ErrInvalidTimestamp)
		}
	}
	return v, nil
}

// fractionMillis converts 1-3 fraction digits to milliseconds.
func fractionMillis(frac string) (int64, error) {
	if frac == "" || len(frac) > 3 {
		return 0, fmt.Errorf("%w: bad millis precision", ErrInvalidTimestamp)
	}
	num, err := parseUnsigned(frac)
	if err != nil {
		return 0, err
	}
	mult := int64(1)
	for i := len(frac); i < 3; i++ {
		mult *= 10
	}
	return num * mult, nil
}
