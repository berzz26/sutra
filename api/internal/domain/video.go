// Package domain defines the core business entities of the application.
package domain

import "time"

// Video status transitions: pending -> processing -> ready | failed.
type VideoStatus string

const (
	VideoStatusPending    VideoStatus = "pending"
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusReady      VideoStatus = "ready"
	VideoStatusFailed     VideoStatus = "failed"
)

type VideoSourceType string

const (
	VideoSourceYouTube VideoSourceType = "youtube"
	VideoSourceUpload  VideoSourceType = "upload"
)

type Video struct {
	ID         string          `json:"id"`
	SourceType VideoSourceType `json:"source_type"`
	SourceRef  string          `json:"source_ref"`
	Title      *string         `json:"title,omitempty"`
	Status     VideoStatus     `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
