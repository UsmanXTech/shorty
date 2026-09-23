package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/UsmanXTech/shorty/internal/config"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	New(config.Config{Address: ":8080"}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected response: %q", got)
	}
}

func TestDashboard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	New(config.Config{Address: ":8080"}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("unexpected content type: %q", got)
	}
	if !strings.Contains(rec.Body.String(), "Shorty · Analytics Dashboard") {
		t.Fatal("dashboard title missing")
	}
}

func TestMaxBodyLimit(t *testing.T) {
	readAll := func(path, body string) (int, error) {
		var data []byte
		var readErr error
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, readErr = io.ReadAll(r.Body)
		})
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		maxBodyLimit(inner).ServeHTTP(rec, req)
		return len(data), readErr
	}

	// 2 MiB body against the default 1 MiB limit must not read fully.
	if n, err := readAll("/api/v1/links", strings.Repeat("x", 2<<20)); err == nil || n > 1<<20 {
		t.Fatalf("expected over-limit body to fail, read %d bytes err=%v", n, err)
	}

	// Small bodies still pass through untouched.
	if n, err := readAll("/api/v1/links", `{"url":"https://example.com"}`); err != nil || n == 0 {
		t.Fatalf("small body should read cleanly, read %d bytes err=%v", n, err)
	}
}
