package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

// newTestServer builds the handler with a lazily-connected DB. None of the
// routes exercised below touch the database, so no live MySQL is required.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	db, err := sql.Open("mysql", "u:p@tcp(127.0.0.1:3306)/none")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, "devkey")
}

func TestOpenAPISpecServed(t *testing.T) {
	h := newTestServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"openapi"`) {
		t.Fatalf("body does not look like an OpenAPI doc: %.80s", rr.Body.String())
	}
}

func TestSwaggerUIServed(t *testing.T) {
	h := newTestServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestAuthRequired(t *testing.T) {
	h := newTestServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/matches", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestWrongKeyRejected(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/matches", nil)
	req.Header.Set("X-API-Key", "nope")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestHealthPublic(t *testing.T) {
	// /health is public (no key) — it will report the DB as unreachable here,
	// but must not require auth and must not 404.
	h := newTestServer(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code == http.StatusNotFound || rr.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, health should be public and routed", rr.Code)
	}
}
