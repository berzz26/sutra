// Package domain defines the core business entities of the application.
package domain

import "time"

type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

type JobKind string

const (
	JobKindIngestYouTube JobKind = "ingest_youtube"
	JobKindIngestUpload  JobKind = "ingest_upload"
	JobKindEmbed         JobKind = "embed"
	JobKindIndex         JobKind = "index"
)

type Job struct {
	ID         string     `json:"id"`
	VideoID    *string    `json:"video_id,omitempty"`
	Kind       JobKind    `json:"kind"`
	Status     JobStatus  `json:"status"`
	Attempts   int        `json:"attempts"`
	LastError  *string    `json:"last_error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
