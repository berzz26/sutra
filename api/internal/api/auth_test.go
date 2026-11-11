package api

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func runProtectedRequest(t *testing.T, configuredToken, presentedToken string) *httptest.ResponseRecorder {
	t.Helper()
	app := fiber.New()
	app.Get("/videos", requireAdminToken(configuredToken), func(c *fiber.Ctx) error {
		return c.SendString("allowed")
	})
	req := httptest.NewRequest("GET", "/videos", nil)
	if presentedToken != "" {
		req.Header.Set("Authorization", "Bearer "+presentedToken)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	rec.Body.Write(body)
	return rec
}

func TestRequireAdminTokenAcceptsCorrectToken(t *testing.T) {
	resp := runProtectedRequest(t, "secret-token", "secret-token")
	if resp.Code != fiber.StatusOK {
		t.Fatalf("expected 200 for correct token, got %d", resp.Code)
	}
}

func TestRequireAdminTokenRejectsMissingAndWrongToken(t *testing.T) {
	for name, presented := range map[string]string{"missing": "", "wrong": "nope"} {
		resp := runProtectedRequest(t, "secret-token", presented)
		if resp.Code != fiber.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d", name, resp.Code)
		}
		if got := resp.Body.String(); got == "allowed" {
			t.Fatalf("%s: protected body must not be returned without a valid token", name)
		}
	}
}

func TestRequireAdminTokenFailsClosedWithoutConfiguration(t *testing.T) {
	// No token configured: every request must be rejected so the endpoint is
	// never exposed publicly.
	for name, presented := range map[string]string{"missing": "", "wrong": "nope"} {
		resp := runProtectedRequest(t, "", presented)
		if resp.Code != fiber.StatusUnauthorized {
			t.Fatalf("%s: expected 401 when no token is configured, got %d", name, resp.Code)
		}
	}
}

func TestRequireAdminTokenNeverEchoesConfiguredToken(t *testing.T) {
	resp := runProtectedRequest(t, "secret-token", "")
	if body := resp.Body.String(); strings.Contains(body, "secret-token") {
		t.Fatalf("response body must not contain the configured token: %q", body)
	}
}
