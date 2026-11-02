package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/berzz/searcher/backend/internal/persistence"
)

const probeTimeout = 2 * time.Second

// healthz reports process liveness only: if we can answer, we are alive.
func (h *Handlers) healthz(c *fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
}

// readyz verifies that the PostgreSQL pool can actually serve traffic.
func (h *Handlers) readyz(c *fiber.Ctx) error {
	if err := persistence.Ping(c.Context(), h.pool, probeTimeout); err != nil {
		slog.Error("readiness check failed", "error", err)
		return c.Status(http.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "unavailable", "error": "database unreachable"})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ready"})
}
