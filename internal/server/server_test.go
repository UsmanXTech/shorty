package server

import (
	"net/http"
	"net/http/httptest"
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

	if got := rec.Body.String(); got != "{"status":"ok"}\n" {
		t.Fatalf("unexpected response: %q", got)
	}
}
