// Package api registers the HTTP routes and handlers.
package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/ingestion"
)

type Handlers struct {
	pool    *pgxpool.Pool
	service *ingestion.Service
}

func New(pool *pgxpool.Pool, service *ingestion.Service) *Handlers {
	return &Handlers{pool: pool, service: service}
}

func (h *Handlers) Register(app *fiber.App) {
	// Health/readiness live at the root, outside /api, so orchestrators
	// probe them without prefix knowledge.
	app.Get("/healthz", h.healthz)
	app.Get("/readyz", h.readyz)

	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Get("/health", h.healthz)

	v1.Post("/videos", h.ingestYouTube)
	v1.Post("/transcripts", h.uploadTranscript)
	v1.Get("/videos", h.listVideos)
	v1.Get("/videos/subtitles", h.listSubtitles)
	v1.Get("/videos/:id", h.getVideo)
	v1.Get("/videos/:id/transcript", h.getTranscript)
	v1.Delete("/videos/:id", h.deleteVideo)
}
