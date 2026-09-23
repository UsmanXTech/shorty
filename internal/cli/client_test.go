package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/links"
)

func TestCreateLinkUsesRESTAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/links" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"slug":"abc","url":"https://example.com","clicks":0}`))
	}))
	defer srv.Close()
	got, err := NewClient(srv.URL).CreateLink(links.Link{Slug: "abc", URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 1 || got.Slug != "abc" || got.URL != "https://example.com" {
		t.Fatalf("unexpected link: %+v", got)
	}
}

func TestStatsBuildsQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/links/7/analytics" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("interval") != "day" {
			t.Fatal("missing interval")
		}
		if r.URL.Query().Get("from") == "" || r.URL.Query().Get("to") == "" {
			t.Fatal("missing date filters")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"slug":"abc","lifetime_clicks":4,"clicks_in_range":2,"interval":"day","buckets":[]}`))
	}))
	defer srv.Close()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	got, err := NewClient(srv.URL).Stats(7, from, to, "day")
	if err != nil {
		t.Fatal(err)
	}
	if got.LifetimeClicks != 4 || got.ClicksInRange != 2 {
		t.Fatalf("unexpected stats: %+v", got)
	}
}
