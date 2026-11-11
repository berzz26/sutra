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

// MaxScopedVideoIDs bounds the size of a multi-video search scope so a
// request cannot carry an unbounded ID list.
const MaxScopedVideoIDs = 500

type searchRequest struct {
	Query   string   `json:"query"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	VideoID string   `json:"video_id"`
	VideoIDs []string `json:"video_ids"`
}

// search resolves the retrieval scope in this order:
//
//  1. `video_id` — single video (existing behavior, video must exist).
//  2. `video_ids` — explicit multi-video scope. An empty list matches
//     nothing and returns zero results immediately; it is never interpreted
//     as a global search. Unavailable (deleted) IDs simply contribute no
//     rows instead of failing the request.
//  3. Neither — unrestricted retrieval. Kept for backward compatibility
//     with existing API clients and internal tooling; the browser frontend
//     always sends an explicit scope.
func (h *Handlers) search(c *fiber.Ctx) error {
	started := time.Now()
	var req searchRequest
	if err := c.BodyParser(&req); err != nil {
		return ingestion.NewError(ingestion.ErrCodeInvalidSearch, "request body must be valid JSON")
	}
	if req.Limit == 0 {
		req.Limit = 10
	}
	req.VideoID = strings.TrimSpace(req.VideoID)
	query, err := retrieval.Validate(req.Query, req.Limit, req.Offset, req.VideoID)
	if err != nil {
		return invalidSearchError(err)
	}

	var results []retrieval.Result
	switch {
	case req.VideoID != "":
		if _, err := persistence.GetVideo(c.Context(), h.pool, req.VideoID); err != nil {
			return mapAPIError(err)
		}
		results, err = h.retriever.Search(c.Context(), query, req.Limit, req.Offset, req.VideoID)
	default:
		if len(req.VideoIDs) > MaxScopedVideoIDs {
			return invalidSearchError(ingestion.Errorf(ingestion.ErrCodeInvalidSearch, "video_ids cannot exceed %d entries", MaxScopedVideoIDs))
		}
		ids, idsErr := retrieval.ValidateVideoIDs(req.VideoIDs)
		if idsErr != nil {
			return invalidSearchError(idsErr)
		}
		if req.VideoIDs != nil && len(ids) == 0 {
			// Empty scope: zero results, no embedder or database work.
			results = []retrieval.Result{}
			break
		}
		if len(ids) == 0 {
			// No scope at all: legacy unrestricted retrieval, kept for
			// backward compatibility with existing API clients and tooling.
			results, err = h.retriever.Search(c.Context(), query, req.Limit, req.Offset, "")
		} else {
			results, err = h.retriever.SearchIn(c.Context(), query, req.Limit, req.Offset, ids)
		}
	}
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
