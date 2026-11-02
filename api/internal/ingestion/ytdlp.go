package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Metadata mirrors the subset of yt-dlp's --dump-single-json output that
// ingestion needs. Fields may legitimately be missing.
type Metadata struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Duration          float64   `json:"duration"` // seconds; 0 when unknown (e.g. live)
	Channel           string    `json:"channel"`
	Uploader          string    `json:"uploader"`
	Thumbnail         string    `json:"thumbnail"`
	Subtitles         trackList `json:"subtitles"`
	AutomaticCaptions trackList `json:"automatic_captions"`
}

// trackList is a language -> supported subtitle entries map.
type trackList map[string][]subtitleEntry

type subtitleEntry struct {
	Ext string `json:"ext"`
}

// subSelection records which subtitle track extraction picked.
type subSelection struct {
	language string // exact track key from yt-dlp
	manual   bool   // true = subtitles map; false = automatic_captions
}

// Extractor runs the yt-dlp binary in separate processes.
type Extractor struct {
	Binary  string
	Timeout time.Duration
}

func NewExtractor(binary string, timeout time.Duration) *Extractor {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &Extractor{Binary: binary, Timeout: timeout}
}

// FetchMetadata retrieves video metadata without downloading media.
func (e *Extractor) FetchMetadata(ctx context.Context, videoURL string) (*Metadata, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	out, err := e.run(ctx, []string{
		"--dump-single-json", "--skip-download",
		"--no-warnings", "--no-playlist", videoURL,
	})
	if err != nil {
		return nil, err
	}

	var meta Metadata
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, WrapError(ErrCodeYTDLPFailed, err, "could not parse yt-dlp metadata output")
	}
	return &meta, nil
}

// DownloadSubtitle downloads only the selected subtitle track into dir (a
// unique per-invocation temp directory) and returns the file path and format.
func (e *Extractor) DownloadSubtitle(ctx context.Context, videoURL string, sel subSelection, dir string) (path, format string, err error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	args := subtitleDownloadArgs(sel, dir, videoURL)

	if _, err := e.run(ctx, args); err != nil {
		return "", "", err
	}

	path, ok := findSubtitleFile(dir)
	if !ok {
		return "", "", NewError(ErrCodeYTDLPFailed, "yt-dlp produced no subtitle file")
	}
	format, ok = detectSubtitleFormat(path)
	if !ok {
		return "", "", NewError(ErrCodeInvalidSubtitle, "downloaded subtitle file is empty or not VTT/SRT")
	}
	return path, format, nil
}

func subtitleDownloadArgs(sel subSelection, dir, videoURL string) []string {
	args := []string{
		"--skip-download", "--no-warnings", "--no-playlist",
		"--sub-langs", sel.language,
		"--sub-format", "vtt/srt/best",
		"-o", dir + "/original",
	}
	if sel.manual {
		args = append(args, "--write-subs")
	} else {
		args = append(args, "--write-auto-subs")
	}
	return append(args, videoURL)
}

// run executes a single yt-dlp invocation: no shell, first arg = binary.
func (e *Extractor) run(ctx context.Context, args []string) ([]byte, error) {
	if _, err := exec.LookPath(e.Binary); err != nil {
		return nil, NewError(ErrCodeYTDLPNotInstalled, "yt-dlp is not installed or not on PATH")
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, Errorf(ErrCodeYTDLPTimeout, "yt-dlp timed out after %s",
				time.Since(start).Truncate(time.Second))
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, NewError(ErrCodeYTDLPFailed, "ingestion canceled")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, classifyYTDLPEnd(ctx, string(exitErr.Stderr))
		}
		return nil, WrapError(ErrCodeYTDLPFailed, err, "failed to run yt-dlp")
	}
	return out, nil
}

// classifyYTDLPEnd maps yt-dlp stderr text to stable error codes.
func classifyYTDLPEnd(ctx context.Context, stderr string) *Error {
	lower := strings.ToLower(stderr)
	var code string
	switch {
	case ctx.Err() != nil:
		return NewError(ErrCodeYTDLPTimeout, "yt-dlp timed out")
	case containsAny(lower, "video unavailable", "video has been removed",
		"is not available", "removed by the uploader", "terminated"):
		code = ErrCodeVideoUnavailable
	case containsAny(lower, "private video", "sign in to confirm",
		"login required", "members-only", "age-restricted",
		"join this channel", "account is under 18"):
		code = ErrCodeVideoInaccessible
	case containsAny(lower, "http error 429", "too many requests", "rate limit", "rate-limited"):
		code = ErrCodeYTDLPRateLimited
	default:
		code = ErrCodeYTDLPFailed
	}
	return Errorf(code, "yt-dlp failed: %s", firstLine(stderr))
}

func containsAny(text string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown error"
	}
	return s
}
