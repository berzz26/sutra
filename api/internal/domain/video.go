// Package domain defines the core business entities of the application.
package domain

import "time"

// Video status lifecycle: pending -> fetching_transcript -> processing ->
// ready | failed | no_transcript.
type VideoStatus string

const (
	VideoStatusPending            VideoStatus = "pending"
	VideoStatusFetchingTranscript VideoStatus = "fetching_transcript"
	VideoStatusProcessing         VideoStatus = "processing"
	VideoStatusReady              VideoStatus = "ready"
	VideoStatusFailed             VideoStatus = "failed"
	VideoStatusNoTranscript       VideoStatus = "no_transcript"
)

type VideoSourceType string

const (
	VideoSourceYouTube VideoSourceType = "youtube"
	VideoSourceUpload  VideoSourceType = "upload"
)

type Video struct {
	ID           string          `json:"id"`
	SourceType   VideoSourceType `json:"source_type"`
	SourceRef    string          `json:"source_ref"`
	Title        *string         `json:"title,omitempty"`
	OriginalURL  *string         `json:"original_url,omitempty"`
	ThumbnailURL *string         `json:"thumbnail_url,omitempty"`
	DurationMs   *int64          `json:"duration_ms,omitempty"`
	Channel      *string         `json:"channel,omitempty"`
	Language     *string         `json:"language,omitempty"`
	Status       VideoStatus     `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
