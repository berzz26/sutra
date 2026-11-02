// Package api registers the HTTP routes and handlers.
package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool}
}

func (h *Handlers) Register(app *fiber.App) {
	// Health/readiness live at the root, outside /api, so orchestrators
	// probe them without prefix knowledge.
	app.Get("/healthz", h.healthz)
	app.Get("/readyz", h.readyz)

	api := app.Group("/api")
	v1 := api.Group("/v1")
	v1.Get("/health", h.healthz)

	// Application endpoints (submit videos, search) are added in later phases.
}
