package redirect

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/links"
)

func TestRedirect(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	_, err := repo.Create(links.Link{Slug: "go", URL: "https://go.dev", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go", nil)
	rec := httptest.NewRecorder()
	New(repo).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://go.dev" {
		t.Fatalf("expected redirect to https://go.dev, got %q", got)
	}
}

func TestRedirectNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	New(links.NewMemoryRepository()).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRedirectExpired(t *testing.T) {
	repo := links.NewMemoryRepository()
	expired := time.Now().UTC().Add(-time.Minute)
	now := time.Now().UTC()
	_, err := repo.Create(links.Link{Slug: "old", URL: "https://example.com", CreatedAt: now, UpdatedAt: now, ExpiresAt: &expired})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	rec := httptest.NewRecorder()
	New(repo).ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", rec.Code)
	}
}
