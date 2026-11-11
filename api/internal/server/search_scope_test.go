package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/berzz/searcher/backend/internal/api"
)

// newInvariantApp builds the production server with no database and no
// retrieval service: the paths under test must never reach either.
func newInvariantApp() *Server {
	return New(":0", nil, nil, nil, nil, 0, "")
}

func postSearch(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newInvariantApp().app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	payload, _ := readAll(resp.Body)
	rec.Body.Write(payload)
	return rec
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			return buf, err
		}
	}
}

// The core invariant: video_ids: [] is an empty scope, never a global search.
// The response must contain zero results even though there is no database
// connection available at all.
func TestSearchWithEmptyVideoIDsReturnsZeroResults(t *testing.T) {
	resp := postSearch(t, `{"query":"database replication","video_ids":[]}`)
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if body := resp.Body.String(); !strings.Contains(body, `"results":[]`) {
		t.Fatalf("expected an empty results array, got %s", body)
	}
}

func TestSearchWithBlankVideoIDsReturnsZeroResults(t *testing.T) {
	resp := postSearch(t, `{"query":"database replication","video_ids":["", "  "]}`)
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if body := resp.Body.String(); !strings.Contains(body, `"results":[]`) {
		t.Fatalf("expected an empty results array, got %s", body)
	}
}

func TestSearchWithMalformedVideoIDsIsRejected(t *testing.T) {
	resp := postSearch(t, `{"query":"database replication","video_ids":["definitely-not-a-uuid"]}`)
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestSearchWithOversizedVideoIDListIsRejected(t *testing.T) {
	const maxScopedVideoIDs = api.MaxScopedVideoIDs
	ids := make([]string, 0, maxScopedVideoIDs+1)
	for i := 0; i <= maxScopedVideoIDs; i++ {
		ids = append(ids, "00000000-0000-0000-0000-000000000001")
	}
	resp := postSearch(t, `{"query":"q","video_ids":["`+strings.Join(ids, `","`)+`"]}`)
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
