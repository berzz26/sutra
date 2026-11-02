package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/artifact"
	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/transcript"
)

// Service orchestrates ingestion: validation, yt-dlp extraction, parsing,
// artifact storage and transactional persistence. Handlers only pass input
// through; the service is Fiber-independent so a background worker can call
// the same methods later.
type Service struct {
	pool           *pgxpool.Pool
	extractor      *Extractor
	store          artifact.Store // nil = artifact storage disabled
	maxUploadBytes int64
	limiter        chan struct{}
	videoLocks     sync.Map
}

func NewService(pool *pgxpool.Pool, extractor *Extractor, store artifact.Store, maxUploadBytes int64, maxConcurrent int) *Service {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Service{
		pool:           pool,
		extractor:      extractor,
		store:          store,
		maxUploadBytes: maxUploadBytes,
		limiter:        make(chan struct{}, maxConcurrent),
	}
}

// IngestYouTube ingests a YouTube video's existing subtitles.
//
// Idempotency: the canonical video ID is the dedup key. READY and
// NO_TRANSCRIPT videos are terminal states and returned as-is; FAILED videos
// are retried (safe replacement: failed runs never wrote cues, and the
// finalize transaction replaces them if any existed); in-flight states
// (pending/fetching_transcript/processing) return the existing row instead of
// racing.
func (s *Service) IngestYouTube(ctx context.Context, rawURL, language string) (persistence.VideoWithTranscript, error) {
	return s.ingestYouTube(ctx, rawURL, language, "", "")
}

// IngestYouTubeTrack ingests the exact subtitle key selected from
// DiscoverYouTubeSubtitles. source may be manual or automatic; when empty,
// the existing language-preference selector is used.
func (s *Service) IngestYouTubeTrack(ctx context.Context, rawURL, language, track, source string) (persistence.VideoWithTranscript, error) {
	return s.ingestYouTube(ctx, rawURL, language, track, source)
}

func (s *Service) ingestYouTube(ctx context.Context, rawURL, language, track, source string) (persistence.VideoWithTranscript, error) {
	ref, err := ParseYouTubeURL(rawURL)
	if err != nil {
		return persistence.VideoWithTranscript{}, URLValidationError(err)
	}

	video, _, err := persistence.EnsureVideoPending(ctx, s.pool, domain.VideoSourceYouTube, ref.VideoID,
		persistence.VideoSeed{OriginalURL: &ref.CanonicalURL})
	if err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodePersistenceFailed, err, "failed to persist video row")
	}
	lockValue, _ := s.videoLocks.LoadOrStore(ref.VideoID, &sync.Mutex{})
	videoLock := lockValue.(*sync.Mutex)
	videoLock.Lock()
	defer videoLock.Unlock()
	// Serialize retries for one video inside this process so concurrent requests
	// cannot extract and replace the same cue set twice.
	video, err = persistence.GetVideoBySource(ctx, s.pool, domain.VideoSourceYouTube, ref.VideoID)
	if err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodePersistenceFailed, err, "failed to reload video row")
	}

	switch video.Status {
	case domain.VideoStatusReady, domain.VideoStatusNoTranscript:
		return s.getWithTranscript(ctx, video.ID)
	case domain.VideoStatusFetchingTranscript, domain.VideoStatusProcessing:
		return s.getWithTranscript(ctx, video.ID)
	}

	if err := s.setStatus(ctx, video.ID, domain.VideoStatusFetchingTranscript); err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	result, err := s.extractAndPersistYouTube(ctx, video, ref, language, track, source)
	if err != nil {
		s.setStatusBestEffort(ctx, video.ID, failureStatus(err))
		return persistence.VideoWithTranscript{}, err
	}
	return result, nil
}

// SubtitleOption is an exact track exposed by yt-dlp metadata. Language is
// intentionally not normalized so clients can submit keys such as en-orig.
type SubtitleOption struct {
	Language string   `json:"language"`
	Name     string   `json:"name,omitempty"`
	Source   string   `json:"source"`
	Formats  []string `json:"formats"`
}

// DiscoverYouTubeSubtitles fetches metadata without downloading a subtitle or
// media file and returns only compatible VTT/SRT tracks.
func (s *Service) DiscoverYouTubeSubtitles(ctx context.Context, rawURL string) (string, string, []SubtitleOption, error) {
	ref, err := ParseYouTubeURL(rawURL)
	if err != nil {
		return "", "", nil, URLValidationError(err)
	}
	s.limiter <- struct{}{}
	defer func() { <-s.limiter }()
	meta, err := s.extractor.FetchMetadata(ctx, ref.CanonicalURL)
	if err != nil {
		return "", "", nil, err
	}
	options := manualSubtitleOptions(meta)
	return meta.ID, meta.Title, options, nil
}

// IngestUpload ingests an uploaded VTT/SRT transcript.
//
// Dedup: videos.source_ref holds the file's SHA-256, so identical uploads
// resolve to the same video (idempotent) while different files always create
// separate videos regardless of filename.
func (s *Service) IngestUpload(ctx context.Context, params UploadParams) (persistence.VideoWithTranscript, error) {
	data, err := readBounded(ctx, params.File, s.maxUploadBytes)
	if err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	cues, err := transcript.Parse(params.Format, data)
	if err != nil {
		return persistence.VideoWithTranscript{}, uploadParseError(params.Format, err)
	}
	detected, ok := sniffSubtitleFormat(data)
	if !ok || transcript.Format(detected) != params.Format {
		return persistence.VideoWithTranscript{}, Errorf(ErrCodeInvalidSubtitle, "uploaded content does not match the .%s format", params.Format)
	}

	hash := sha256.Sum256(data)
	sourceRef := hex.EncodeToString(hash[:])

	title := params.Title
	if title == nil || *title == "" {
		title = &params.FallbackTitle
	}

	video, _, err := persistence.EnsureVideoPending(ctx, s.pool, domain.VideoSourceUpload, sourceRef,
		persistence.VideoSeed{Title: title, OriginalURL: params.OriginalURL, Language: params.Language})
	if err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodePersistenceFailed, err, "failed to persist video row")
	}

	switch video.Status {
	case domain.VideoStatusReady, domain.VideoStatusNoTranscript:
		return s.getWithTranscript(ctx, video.ID)
	case domain.VideoStatusFetchingTranscript, domain.VideoStatusProcessing:
		return s.getWithTranscript(ctx, video.ID)
	}

	if err := s.setStatus(ctx, video.ID, domain.VideoStatusProcessing); err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	result, err := s.persistUpload(ctx, video, params, data, cues)
	if err != nil {
		s.setStatusBestEffort(ctx, video.ID, failureStatus(err))
		return persistence.VideoWithTranscript{}, err
	}
	return result, nil
}

// UploadParams carries everything the upload handler validated.
type UploadParams struct {
	FallbackTitle string // used when Title is nil/empty (defaults to filename)
	Title         *string
	OriginalURL   *string // canonical HTTPS URL, validated by the caller
	Language      *string
	Format        transcript.Format
	File          io.Reader
}

// extractAndPersistYouTube runs extraction (bounded by the concurrency
// limiter) and the persistence transaction.
func (s *Service) extractAndPersistYouTube(ctx context.Context, video domain.Video, ref *YouTubeRef, language, track, source string) (persistence.VideoWithTranscript, error) {
	s.limiter <- struct{}{}
	defer func() { <-s.limiter }()

	meta, err := s.extractor.FetchMetadata(ctx, ref.CanonicalURL)
	if err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	var selection subSelection
	if track != "" {
		selection, err = selectExactSubtitleTrack(meta, track, source)
	} else {
		selection, err = selectSubtitleTrack(meta, language)
	}
	if err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	dir, err := os.MkdirTemp("", "searcher-ingest-")
	if err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodeYTDLPFailed, err, "failed to create temp directory")
	}
	defer os.RemoveAll(dir)

	path, format, err := s.extractor.DownloadSubtitle(ctx, ref.CanonicalURL, selection, dir)
	if err != nil {
		return persistence.VideoWithTranscript{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodeInvalidSubtitle, err, "failed to read downloaded subtitle file")
	}

	cues, err := transcript.Parse(transcript.Format(format), data)
	if err != nil {
		return persistence.VideoWithTranscript{}, Errorf(ErrCodeInvalidSubtitle, "subtitle file could not be parsed: %v", err)
	}

	method := domain.ExtractionAutoGenerated
	if selection.manual {
		method = domain.ExtractionManualSubtitle
	}

	finalize := persistence.FinalizedMetadata{
		Title:        strPtrOrEmpty(meta.Title),
		OriginalURL:  &ref.CanonicalURL,
		ThumbnailURL: strPtrOrEmpty(meta.Thumbnail),
		Channel:      firstNonEmpty(meta.Channel, meta.Uploader),
		DurationMs:   durationMS(meta.Duration),
		Language:     &selection.language,
		Status:       domain.VideoStatusReady,
	}
	transcriptRecord := domain.Transcript{
		ExtractionMethod: method,
		OriginalFormat:   format,
		Language:         &selection.language,
		CueCount:         len(cues),
	}

	return s.persistCommon(ctx, video.ID, finalize, transcriptRecord, data, cues, path)
}

// persistUpload finalizes an upload: same transactional path as YouTube.
func (s *Service) persistUpload(ctx context.Context, video domain.Video, params UploadParams, data []byte, cues []transcript.Cue) (persistence.VideoWithTranscript, error) {
	finalize := persistence.FinalizedMetadata{
		Title:       params.Title,
		OriginalURL: params.OriginalURL,
		Language:    params.Language,
		Status:      domain.VideoStatusReady,
	}
	transcriptRecord := domain.Transcript{
		ExtractionMethod: domain.ExtractionDirectUpload,
		OriginalFormat:   string(params.Format),
		Language:         params.Language,
		CueCount:         len(cues),
	}

	_, err := s.persistCommon(ctx, video.ID, finalize, transcriptRecord, data, cues, "")
	if err != nil {
		return persistence.VideoWithTranscript{}, err
	}
	return s.getWithTranscript(ctx, video.ID)
}

// persistCommon stores the artifact (if enabled), then commits video metadata
// + transcript + cues in one database transaction, then removes superseded
// artifacts. On any failure the database is left without fake artifact keys.
func (s *Service) persistCommon(ctx context.Context, videoID string, finalize persistence.FinalizedMetadata, record domain.Transcript, data []byte, cues []transcript.Cue, sourcePath string) (persistence.VideoWithTranscript, error) {
	record.ID = uuid.NewString()
	record.VideoID = videoID

	var artifactKey *string
	if s.store != nil {
		ext := "vtt"
		if record.OriginalFormat == string(transcript.FormatSRT) {
			ext = "srt"
		}
		key := fmt.Sprintf("transcripts/%s/%s/original.%s", videoID, record.ID, ext)
		contentType := contentTypeFor(ext)
		var size int64
		if sourcePath != "" {
			info, err := os.Stat(sourcePath)
			if err != nil {
				return persistence.VideoWithTranscript{}, WrapError(ErrCodeMinioUploadFailed, err, "failed to stat subtitle file")
			}
			size = info.Size()
		} else {
			size = int64(len(data))
		}
		reader := io.Reader(bytes.NewReader(data))
		if sourcePath != "" {
			f, err := os.Open(sourcePath)
			if err != nil {
				return persistence.VideoWithTranscript{}, WrapError(ErrCodeMinioUploadFailed, err, "failed to open subtitle file")
			}
			defer f.Close()
			reader = f
		}
		if err := s.store.Put(ctx, key, reader, size, contentType); err != nil {
			return persistence.VideoWithTranscript{}, WrapError(ErrCodeMinioUploadFailed, err, "failed to store artifact")
		}
		artifactKey = &key
		record.ArtifactKey = artifactKey
	}

	domainCues := make([]domain.TranscriptCue, len(cues))
	for i, cue := range cues {
		domainCues[i] = domain.TranscriptCue{
			VideoID: videoID,
			Seq:     cue.Seq,
			StartMs: cue.StartMs,
			EndMs:   cue.EndMs,
			Text:    cue.Text,
		}
	}

	var supersededArtifactKeys []string
	err := persistence.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if old, err := persistence.GetTranscriptByVideo(ctx, tx, videoID); err == nil && old.ArtifactKey != nil {
			supersededArtifactKeys = append(supersededArtifactKeys, *old.ArtifactKey)
		}
		if err := persistence.FinalizeVideo(ctx, tx, videoID, finalize); err != nil {
			return err
		}
		if _, err := persistence.UpsertTranscript(ctx, tx, record); err != nil {
			return err
		}
		return persistence.ReplaceCues(ctx, tx, videoID, domainCues)
	})
	if err != nil {
		if artifactKey != nil && s.store != nil {
			if delErr := s.store.Delete(context.WithoutCancel(ctx), *artifactKey); delErr != nil {
				slog.Error("artifact cleanup after failed persistence failed", "key", artifactKey, "error", delErr)
			}
		}
		return persistence.VideoWithTranscript{}, WrapError(ErrCodePersistenceFailed, err, "failed to persist transcript")
	}

	s.cleanArtifacts(ctx, supersededArtifactKeys)
	if err := persistence.EnqueueEmbedding(ctx, s.pool, videoID); err != nil {
		return persistence.VideoWithTranscript{}, WrapError(ErrCodePersistenceFailed, err, "failed to enqueue transcript processing")
	}
	slog.Info("transcript processing job created", "video_id", videoID, "cues", len(domainCues))

	return s.getWithTranscript(ctx, videoID)
}

// DeleteVideo removes a video, its transcript data (CASCADE) and its stored
// artifacts. Artifacts are removed first; if that fails, nothing misleading
// is recorded — the DB row still points at an artifact that exists.
func (s *Service) DeleteVideo(ctx context.Context, id string) error {
	video, err := persistence.GetVideo(ctx, s.pool, id)
	if err != nil {
		return mapPersistenceError(err)
	}
	keys := s.artifactKeys(video)
	if err := persistence.DeleteVideo(ctx, s.pool, id); err != nil {
		return mapPersistenceError(err)
	}
	for _, key := range keys {
		if err := s.store.Delete(context.WithoutCancel(ctx), key); err != nil {
			slog.Error("video deleted but artifact cleanup failed", "video_id", id, "key", key, "error", err)
			return WrapError(ErrCodeMinioUploadFailed, err, "video deleted but failed to remove transcript artifact")
		}
	}
	return nil
}

func (s *Service) artifactKeys(v persistence.VideoWithTranscript) []string {
	if s.store == nil || v.Transcript == nil || v.Transcript.ArtifactKey == nil {
		return nil
	}
	return []string{*v.Transcript.ArtifactKey}
}

func (s *Service) cleanArtifacts(ctx context.Context, keys []string) {
	if s.store == nil || len(keys) == 0 {
		return
	}
	for _, key := range keys {
		if err := s.store.Delete(context.WithoutCancel(ctx), key); err != nil {
			slog.Error("failed to delete superseded artifact", "key", key, "error", err)
		}
	}
}

func (s *Service) getWithTranscript(ctx context.Context, videoID string) (persistence.VideoWithTranscript, error) {
	vt, err := persistence.GetVideo(ctx, s.pool, videoID)
	if err != nil {
		return persistence.VideoWithTranscript{}, mapPersistenceError(err)
	}
	return vt, nil
}

func (s *Service) setStatus(ctx context.Context, videoID string, status domain.VideoStatus) error {
	if err := persistence.UpdateVideoStatus(ctx, s.pool, videoID, status); err != nil {
		return mapPersistenceError(err)
	}
	return nil
}

// setStatusBestEffort records a terminal failure state; the original error is
// more important than a persistence blip here.
func (s *Service) setStatusBestEffort(ctx context.Context, videoID string, status domain.VideoStatus) {
	if err := persistence.UpdateVideoStatus(ctx, s.pool, videoID, status); err != nil {
		slog.Error("failed to record ingestion failure status", "video_id", videoID, "status", status, "error", err)
	}
}

func (s *Service) ListVideos(ctx context.Context, limit, offset int) ([]persistence.VideoWithTranscript, error) {
	videos, err := persistence.ListVideos(ctx, s.pool, limit, offset)
	if err != nil {
		return nil, WrapError(ErrCodePersistenceFailed, err, "failed to list videos")
	}
	return videos, nil
}

// failureStatus maps an ingestion error to the persisted lifecycle status.
func failureStatus(err error) domain.VideoStatus {
	switch {
	case hasCode(err, ErrCodeNoTranscript, ErrCodeLanguageUnavailable):
		return domain.VideoStatusNoTranscript
	default:
		return domain.VideoStatusFailed
	}
}
