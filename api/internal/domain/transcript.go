// Package domain defines the core business entities of the application.
package domain

import "time"

// TranscriptCue is a raw subtitle line exactly as extracted from a YouTube
// video or read from an uploaded transcript. Cues are never rewritten.
type TranscriptCue struct {
	ID      string `json:"id"`
	VideoID string `json:"video_id"`
	Seq     int    `json:"seq"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// TranscriptChunk is an indexed search chunk produced by later processing
// (deduped, sentence-normalized, possibly spanning several cues). Kept as a
// separate entity so raw cues stay immutable.
type TranscriptChunk struct {
	ID        string    `json:"id"`
	VideoID   string    `json:"video_id"`
	StartMs   int64     `json:"start_ms"`
	EndMs     int64     `json:"end_ms"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}
