package ingestion

import (
	"errors"
	"fmt"
)

// Error codes are stable identifiers surfaced to API clients alongside a
// human-readable message. Status code mapping lives in internal/api.
const (
	ErrCodeInvalidYouTubeURL   = "INVALID_YOUTUBE_URL"
	ErrCodeUnsupportedURL      = "UNSUPPORTED_URL"
	ErrCodePlaylistURL         = "PLAYLIST_NOT_SUPPORTED"
	ErrCodeVideoUnavailable    = "VIDEO_UNAVAILABLE"
	ErrCodeVideoInaccessible   = "VIDEO_INACCESSIBLE"
	ErrCodeNoTranscript        = "NO_TRANSCRIPT"
	ErrCodeLanguageUnavailable = "LANGUAGE_UNAVAILABLE"
	ErrCodeYTDLPNotInstalled   = "YTDLP_NOT_INSTALLED"
	ErrCodeYTDLPTimeout        = "YTDLP_TIMEOUT"
	ErrCodeYTDLPRateLimited    = "YTDLP_RATE_LIMITED"
	ErrCodeYTDLPFailed         = "YTDLP_EXTRACTION_FAILED"
	ErrCodeInvalidSubtitle     = "INVALID_SUBTITLE_FILE"
	ErrCodeInvalidVTT          = "INVALID_VTT"
	ErrCodeInvalidSRT          = "INVALID_SRT"
	ErrCodeUnsupportedFileType = "UNSUPPORTED_FILE_TYPE"
	ErrCodeFileTooLarge        = "FILE_TOO_LARGE"
	ErrCodeEmptyTranscript     = "EMPTY_TRANSCRIPT"
	ErrCodeInvalidTimestamp    = "INVALID_TIMESTAMP"
	ErrCodeMinioUploadFailed   = "MINIO_UPLOAD_FAILED"
	ErrCodePersistenceFailed   = "DATABASE_PERSISTENCE_FAILED"
	ErrCodeVideoNotFound       = "VIDEO_NOT_FOUND"
	ErrCodeInvalidSearch       = "INVALID_SEARCH"
	ErrCodeSearchFailed        = "SEARCH_FAILED"
)

// Error is the unified ingestion error: every failure carries a stable code
// so API responses can expose a structured envelope.
type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Errorf(code string, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func WrapError(code string, err error, message string) *Error {
	return &Error{Code: code, Message: message, Cause: err}
}

// Sentinel errors re-exported for direct errors.Is checks in tests.
var (
	ErrInvalidYouTubeURL = errors.New("invalid YouTube URL")
	ErrUnsupportedURL    = errors.New("unsupported URL")
	ErrPlaylistURL       = errors.New("playlist URLs are not supported")
)

// URLValidationError converts the parser's wrapped sentinel errors into the
// stable API error type while preserving errors.Is behavior for callers.
func URLValidationError(err error) error {
	switch {
	case errors.Is(err, ErrPlaylistURL):
		return &Error{Code: ErrCodePlaylistURL, Message: err.Error()}
	case errors.Is(err, ErrUnsupportedURL):
		return &Error{Code: ErrCodeUnsupportedURL, Message: err.Error()}
	case errors.Is(err, ErrInvalidYouTubeURL):
		return &Error{Code: ErrCodeInvalidYouTubeURL, Message: err.Error()}
	default:
		return err
	}
}
