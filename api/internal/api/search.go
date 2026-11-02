package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/berzz/searcher/backend/internal/ingestion"
	"github.com/berzz/searcher/backend/internal/persistence"
	"github.com/berzz/searcher/backend/internal/retrieval"
)

type searchRequest struct {
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	VideoID string `json:"video_id"`
}

func (h *Handlers) search(c *fiber.Ctx) error {
	started := time.Now()
	var req searchRequest
	if err := c.BodyParser(&req); err != nil {
		return ingestion.NewError(ingestion.ErrCodeInvalidSearch, "request body must be valid JSON")
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	query, err := retrieval.Validate(req.Query, req.Limit, req.Offset, strings.TrimSpace(req.VideoID))
	if err != nil {
		return invalidSearchError(err)
	}
	req.VideoID = strings.TrimSpace(req.VideoID)
	if req.VideoID != "" {
		if _, err := persistence.GetVideo(c.Context(), h.pool, req.VideoID); err != nil {
			return mapAPIError(err)
		}
	}
	results, err := h.retriever.Search(c.Context(), query, req.Limit, req.Offset, req.VideoID)
	if err != nil {
		if errors.Is(err, retrieval.ErrEmptyQuery) || errors.Is(err, retrieval.ErrQueryTooLong) || errors.Is(err, retrieval.ErrInvalidLimit) || errors.Is(err, retrieval.ErrInvalidOffset) || errors.Is(err, retrieval.ErrInvalidVideoID) {
			return invalidSearchError(err)
		}
		return ingestion.WrapError(ingestion.ErrCodeSearchFailed, err, "search failed")
	}
	durationMs := time.Since(started).Microseconds() / 1000
	return c.Status(http.StatusOK).JSON(fiber.Map{"query": query, "limit": req.Limit, "offset": req.Offset, "duration_ms": durationMs, "results": results})
}

func invalidSearchError(err error) *ingestion.Error {
	return ingestion.WrapError(ingestion.ErrCodeInvalidSearch, err, err.Error())
}
