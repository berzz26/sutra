package ingestion

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/berzz/searcher/backend/internal/transcript"
)

// findSubtitleFile locates the subtitle file yt-dlp wrote (its output
// template appends ".<lang>.<ext>").
func findSubtitleFile(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".vtt" || ext == ".srt" {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

// detectSubtitleFormat inspects content, not just the extension: a file whose
// first non-empty line opens a WEBVTT header is VTT; SRT documents start with
// a numeric cue block.
func detectSubtitleFormat(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	if len(data) == 0 {
		return "", false
	}
	return sniffSubtitleFormat(data)
}

// sniffSubtitleFormat returns the detected format of raw subtitle bytes.
func sniffSubtitleFormat(data []byte) (string, bool) {
	head := firstNonEmptyLine(data)
	if head == "" {
		return "", false
	}
	if strings.HasPrefix(head, "WEBVTT") {
		return string(transcript.FormatVTT), true
	}
	if isSRTHead(head, data) {
		return string(transcript.FormatSRT), true
	}
	return "", false
}

func firstNonEmptyLine(data []byte) string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, UTF8BOM))
		if line != "" {
			return line
		}
	}
	return ""
}

const UTF8BOM = "\xef\xbb\xbf"

// isSRTHead checks that the first non-empty line is a numeric cue id and the
// document contains a timing line.
func isSRTHead(head string, data []byte) bool {
	if !isNumeric(head) {
		return false
	}
	return strings.Contains(string(data), "-->")
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
