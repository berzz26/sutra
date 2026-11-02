// Package server assembles the Fiber application, wires dependencies and
// implements graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/berzz/searcher/backend/internal/api"
	"github.com/berzz/searcher/backend/internal/ingestion"
	"github.com/berzz/searcher/backend/internal/processing"
	"github.com/berzz/searcher/backend/internal/retrieval"
)

type Server struct {
	app  *fiber.App
	addr string
}

func New(addr string, pool *pgxpool.Pool, service *ingestion.Service, processor *processing.Service, retriever *retrieval.Service, maxUploadBytes int64) *Server {
	app := fiber.New(fiber.Config{
        AppName:      "sutra",
		BodyLimit:    int(maxUploadBytes) + 1024*1024,
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	app.Use(requestid.New())

	api.New(pool, service, processor, retriever).Register(app)

	return &Server{app: app, addr: addr}
}

// Run blocks until ctx is cancelled, then drains in-flight requests.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("server: listen on %s: %w", s.addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.app.Listener(ln); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down gracefully")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.app.ShutdownWithContext(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
		} else {
			slog.Info("drained all connections")
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("server: serve: %w", err)
		}
		return nil
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	body := fiber.Map{"error": httpMessage(code)}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
	}
	var ie *ingestion.Error
	if errors.As(err, &ie) {
		code = ingestionHTTPStatus(ie.Code)
		body = fiber.Map{"error": ie.Message, "code": ie.Code}
	}
	slog.Error("request failed", "request_id", c.Get("x-request-id"), "method", c.Method(), "path", c.Path(), "status", code, "error", err)
	return c.Status(code).JSON(body)
}

func ingestionHTTPStatus(code string) int {
	switch code {
	case ingestion.ErrCodeVideoNotFound:
		return http.StatusNotFound
	case ingestion.ErrCodeFileTooLarge:
		return http.StatusRequestEntityTooLarge
	case ingestion.ErrCodeMinioUploadFailed, ingestion.ErrCodePersistenceFailed:
		return http.StatusInternalServerError
	case ingestion.ErrCodeSearchFailed:
		return http.StatusInternalServerError
	case ingestion.ErrCodeVideoUnavailable, ingestion.ErrCodeVideoInaccessible, ingestion.ErrCodeNoTranscript, ingestion.ErrCodeLanguageUnavailable:
		return http.StatusUnprocessableEntity
	case ingestion.ErrCodeYTDLPRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusBadRequest
	}
}

func httpMessage(code int) string {
	if text := http.StatusText(code); text != "" {
		return fmt.Sprintf("%d %s", code, text)
	}
	return "internal error"
}
