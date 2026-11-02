package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/berzz/searcher/backend/internal/domain"
	"github.com/berzz/searcher/backend/internal/ingestion"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/transcript"
)

type youtubeRequest struct {
	URL            string `json:"url"`
	Language       string `json:"language"`
	SubtitleKey    string `json:"subtitle_key"`
	SubtitleSource string `json:"subtitle_source"`
}

func (h *Handlers) ingestYouTube(c *fiber.Ctx) error {
	var req youtubeRequest
	if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		return ingestion.NewError(ingestion.ErrCodeInvalidYouTubeURL, "url is required")
	}
	var result persistence.VideoWithTranscript
	var err error
	if strings.TrimSpace(req.SubtitleKey) != "" {
		result, err = h.service.IngestYouTubeTrack(c.Context(), req.URL, req.Language, req.SubtitleKey, req.SubtitleSource)
	} else {
		result, err = h.service.IngestYouTube(c.Context(), req.URL, req.Language)
	}
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(videoResponse(result))
}

func (h *Handlers) listSubtitles(c *fiber.Ctx) error {
	rawURL := strings.TrimSpace(c.Query("url"))
	if rawURL == "" {
		return ingestion.NewError(ingestion.ErrCodeInvalidYouTubeURL, "url is required")
	}
	videoID, title, subtitles, err := h.service.DiscoverYouTubeSubtitles(c.Context(), rawURL)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"video_id":  videoID,
		"title":     title,
		"subtitles": subtitles,
	})
}

func (h *Handlers) uploadTranscript(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return ingestion.NewError(ingestion.ErrCodeUnsupportedFileType, "file is required")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file.Filename), "."))
	var format transcript.Format
	switch ext {
	case string(transcript.FormatVTT):
		format = transcript.FormatVTT
	case string(transcript.FormatSRT):
		format = transcript.FormatSRT
	default:
		return ingestion.Errorf(ingestion.ErrCodeUnsupportedFileType, "unsupported transcript extension %q", ext)
	}
	handle, err := file.Open()
	if err != nil {
		return ingestion.WrapError(ingestion.ErrCodeInvalidSubtitle, err, "failed to open uploaded transcript")
	}
	defer handle.Close()

	title := strings.TrimSpace(c.FormValue("title"))
	var titlePtr *string
	if title != "" {
		titlePtr = &title
	}
	var originalURL *string
	if raw := strings.TrimSpace(c.FormValue("original_url")); raw != "" {
		ref, err := ingestion.ParseYouTubeURL(raw)
		if err != nil {
			return ingestion.URLValidationError(err)
		}
		originalURL = &ref.CanonicalURL
	}
	var language *string
	if value := strings.TrimSpace(c.FormValue("language")); value != "" {
		language = &value
	}
	result, err := h.service.IngestUpload(c.Context(), ingestion.UploadParams{
		FallbackTitle: file.Filename,
		Title:         titlePtr,
		OriginalURL:   originalURL,
		Language:      language,
		Format:        format,
		File:          handle,
	})
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(videoResponse(result))
}

func (h *Handlers) listVideos(c *fiber.Ctx) error {
	limit, offset, err := page(c)
	if err != nil {
		return err
	}
	items, err := h.service.ListVideos(c.Context(), limit, offset)
	if err != nil {
		return err
	}
	result := make([]videoResponseBody, 0, len(items))
	for _, item := range items {
		result = append(result, videoResponse(item))
	}
	return c.JSON(fiber.Map{"videos": result, "limit": limit, "offset": offset})
}

func (h *Handlers) getVideo(c *fiber.Ctx) error {
	item, err := persistence.GetVideo(c.Context(), h.pool, c.Params("id"))
	if err != nil {
		return mapAPIError(err)
	}
	return c.JSON(videoResponse(item))
}

func (h *Handlers) getTranscript(c *fiber.Ctx) error {
	limit, offset, err := page(c)
	if err != nil {
		return err
	}
	if _, err := persistence.GetVideo(c.Context(), h.pool, c.Params("id")); err != nil {
		return mapAPIError(err)
	}
	cues, err := persistence.ListCues(c.Context(), h.pool, c.Params("id"), limit, offset)
	if err != nil {
		return ingestion.WrapError(ingestion.ErrCodePersistenceFailed, err, "failed to retrieve transcript")
	}
	return c.JSON(fiber.Map{"video_id": c.Params("id"), "cues": cues, "limit": limit, "offset": offset})
}

func (h *Handlers) deleteVideo(c *fiber.Ctx) error {
	if err := h.service.DeleteVideo(c.Context(), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func page(c *fiber.Ctx) (int, int, error) {
	limit, err := queryInt(c, "limit", 50)
	if err != nil || limit < 1 || limit > 200 {
		return 0, 0, ingestion.NewError(ingestion.ErrCodeInvalidSubtitle, "limit must be between 1 and 200")
	}
	offset, err := queryInt(c, "offset", 0)
	if err != nil || offset < 0 {
		return 0, 0, ingestion.NewError(ingestion.ErrCodeInvalidSubtitle, "offset must be zero or greater")
	}
	return limit, offset, nil
}

func queryInt(c *fiber.Ctx, key string, fallback int) (int, error) {
	value := c.Query(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

type videoResponseBody struct {
	domain.Video
	Transcript *domain.Transcript `json:"transcript,omitempty"`
}

func videoResponse(value persistence.VideoWithTranscript) videoResponseBody {
	return videoResponseBody{Video: value.Video, Transcript: value.Transcript}
}

func mapAPIError(err error) error {
	if errors.Is(err, persistence.ErrVideoNotFound) {
		return ingestion.NewError(ingestion.ErrCodeVideoNotFound, "video not found")
	}
	return ingestion.WrapError(ingestion.ErrCodePersistenceFailed, err, "database operation failed")
}
