package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UsmanXTech/shorty/internal/links"
)

func TestLinkAPICreateAndGet(t *testing.T) {
	api := NewLinkAPI(links.NewMemoryRepository())
	mux := http.NewServeMux()
	api.Routes(mux)

	body := bytes.NewBufferString(`{"url":"https://example.com","slug":"docs"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created links.Link
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Slug != "docs" || created.URL != "https://example.com" {
		t.Fatalf("unexpected link: %+v", created)
	}
}
