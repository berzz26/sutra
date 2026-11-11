// Package api registers the HTTP routes and handlers.
package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/ingestion"
	"github.com/berzz/searcher/backend/internal/processing"
	"github.com/berzz/searcher/backend/internal/retrieval"
)

type Handlers struct {
	pool       *pgxpool.Pool
	service    *ingestion.Service
	processor  *processing.Service
	retriever  *retrieval.Service
	adminToken string
}

func New(pool *pgxpool.Pool, service *ingestion.Service, processor *processing.Service, retriever *retrieval.Service, adminToken string) *Handlers {
	return &Handlers{pool: pool, service: service, processor: processor, retriever: retriever, adminToken: adminToken}
}

func (h *Handlers) Register(app *fiber.App) {
	// Health/readiness live at the root, outside /api, so orchestrators
	// probe them without prefix knowledge.
	app.Get("/healthz", h.healthz)
	app.Get("/readyz", h.readyz)

	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Get("/health", h.healthz)
	v1.Post("/search", h.search)

	v1.Post("/videos", h.ingestYouTube)
	v1.Post("/transcripts", h.uploadTranscript)
	// The global video listing and video deletion are administrative: they
	// affect or expose the shared database across every browser's local
	// library, so they require the configured admin token.
	v1.Get("/videos", requireAdminToken(h.adminToken), h.listVideos)
	v1.Delete("/videos/:id", requireAdminToken(h.adminToken), h.deleteVideo)
	v1.Get("/videos/subtitles", h.listSubtitles)
	v1.Get("/videos/:id", h.getVideo)
	v1.Get("/videos/:id/transcript", h.getTranscript)
	v1.Post("/videos/:id/process", h.processVideo)
	v1.Get("/videos/:id/processing", h.processingState)
	v1.Get("/videos/:id/chunks", h.listChunks)
}
