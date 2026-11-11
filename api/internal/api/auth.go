package api

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminTokenHeader is an alternative to the Authorization header for
// presenting the administrative token.
const AdminTokenHeader = "X-Admin-Token"

// requireAdminToken guards administrative endpoints (global video listing,
// video deletion) with a static bearer token from configuration.
//
// It fails closed: when no token is configured every request is rejected, so
// an unconfigured deployment never exposes these endpoints publicly.
// The presented token is read from `Authorization: Bearer <token>` or the
// X-Admin-Token header and compared in constant time. The configured token
// is never included in responses or log output.
func requireAdminToken(token string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token == "" {
			return unauthorized(c)
		}
		presented := c.Get("Authorization")
		presented = strings.TrimSpace(strings.TrimPrefix(presented, "Bearer "))
		if presented == "" {
			presented = strings.TrimSpace(c.Get(AdminTokenHeader))
		}
		if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			return unauthorized(c)
		}
		return c.Next()
	}
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "administrative authentication required",
		"code":  "UNAUTHORIZED",
	})
}
