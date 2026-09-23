package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestLinkAPIListSearchAndFilters(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	_, _ = repo.Create(links.Link{Slug: "docs", URL: "https://example.com/docs", CreatedAt: now, UpdatedAt: now, Clicks: 3})
	expired := now.Add(-time.Hour)
	_, _ = repo.Create(links.Link{Slug: "old", URL: "https://example.com/archive", CreatedAt: now, UpdatedAt: now, ExpiresAt: &expired})
	limit := int64(2)
	_, _ = repo.Create(links.Link{Slug: "campaign", URL: "https://example.com/campaign", CreatedAt: now, UpdatedAt: now, MaxClicks: &limit, Clicks: 2})
	_, _ = repo.Create(links.Link{Slug: "new", URL: "https://other.example/new", CreatedAt: now, UpdatedAt: now})

	api := NewLinkAPI(repo)
	mux := http.NewServeMux()
	api.Routes(mux)

	cases := []struct {
		name string
		path string
		want []string
	}{
		{"search slug", "/api/v1/links?q=doc", []string{"docs"}},
		{"search destination", "/api/v1/links?q=campaign", []string{"campaign"}},
		{"active", "/api/v1/links?status=active", []string{"docs", "new"}},
		{"expired", "/api/v1/links?status=expired", []string{"old"}},
		{"limit reached", "/api/v1/links?status=limit-reached", []string{"campaign"}},
		{"clicked", "/api/v1/links?activity=clicked", []string{"docs", "campaign"}},
		{"unvisited", "/api/v1/links?activity=unvisited", []string{"old", "new"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var got []links.Link
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d links, got %d: %+v", len(tc.want), len(got), got)
			}
			seen := make(map[string]bool, len(got))
			for _, link := range got {
				seen[link.Slug] = true
			}
			for _, slug := range tc.want {
				if !seen[slug] {
					t.Errorf("missing %q in %+v", slug, got)
				}
			}
		})
	}
}

func TestLinkAPIListRejectsInvalidFilter(t *testing.T) {
	api := NewLinkAPI(links.NewMemoryRepository())
	mux := http.NewServeMux()
	api.Routes(mux)
	for _, path := range []string{"/api/v1/links?status=unknown", "/api/v1/links?activity=unknown"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", path, rec.Code)
		}
	}
}
