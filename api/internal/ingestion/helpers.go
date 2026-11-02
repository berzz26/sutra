package ingestion

import (
	"context"
	"errors"
	"io"
	"math"

	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/transcript"
)

func strPtrOrEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func firstNonEmpty(values ...string) *string {
	for _, v := range values {
		if v != "" {
			return &v
		}
	}
	return nil
}

func durationMS(duration float64) *int64 {
	if duration <= 0 {
		return nil
	}
	ms := int64(math.Round(duration * 1000))
	if ms == 0 {
		return nil
	}
	return &ms
}

func contentTypeFor(ext string) string {
	if ext == "srt" {
		return "application/x-subrip"
	}
	return "text/vtt"
}

// readBounded reads at most limit bytes (+1 detection byte) and rejects
// oversized payloads with FILE_TOO_LARGE.
func readBounded(_ context.Context, r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, Errorf(ErrCodeFileTooLarge, "upload limit not configured")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, WrapError(ErrCodeInvalidSubtitle, err, "failed to read upload")
	}
	if int64(len(data)) > limit {
		return nil, Errorf(ErrCodeFileTooLarge, "upload exceeds the limit of %d bytes", limit)
	}
	return data, nil
}

// uploadParseError maps parser failures to format-specific error codes.
func uploadParseError(format transcript.Format, err error) error {
	code := ErrCodeInvalidVTT
	if format == transcript.FormatSRT {
		code = ErrCodeInvalidSRT
	}
	switch {
	case errors.Is(err, transcript.ErrEmptyTranscript):
		return Errorf(ErrCodeEmptyTranscript, "the uploaded %s file contains no usable cues", format)
	case errors.Is(err, transcript.ErrInvalidTimestamp):
		return WrapError(ErrCodeInvalidTimestamp, err, "the uploaded "+string(format)+" file contains malformed timestamps")
	default:
		return WrapError(code, err, "the uploaded "+string(format)+" file could not be parsed")
	}
}

// mapPersistenceError converts persistence sentinel errors into stable
// ingestion errors.
func mapPersistenceError(err error) error {
	switch {
	case errors.Is(err, persistence.ErrVideoNotFound):
		return NewError(ErrCodeVideoNotFound, "video not found")
	default:
		return WrapError(ErrCodePersistenceFailed, err, "database operation failed")
	}
}

// hasCode checks an error chain for an ingestion Error code.
func hasCode(err error, codes ...string) bool {
	for _, c := range codes {
		var ie *Error
		if errors.As(err, &ie) && ie.Code == c {
			return true
		}
	}
	return false
}
