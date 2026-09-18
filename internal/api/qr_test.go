package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/links"
)

func TestQRAPI(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	link, err := repo.Create(links.Link{Slug: "docs", URL: "https://example.com/docs", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	api := NewQRAPI(repo, config.Config{BaseURL: "https://short.example"})
	mux := http.NewServeMux()
	api.Routes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/"+itoa(link.ID)+"/qr", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %q", rec.Header().Get("Content-Type"))
	}
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("response is not a PNG")
	}
}

func TestQRAPIUsesRequestHostWhenBaseURLIsUnset(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	link, _ := repo.Create(links.Link{Slug: "docs", URL: "https://example.com", CreatedAt: now, UpdatedAt: now})
	api := NewQRAPI(repo, config.Config{})
	mux := http.NewServeMux()
	api.Routes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/"+itoa(link.ID)+"/qr", nil)
	req.Host = "short.example"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestQRAPIRejectsInvalidSize(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	link, _ := repo.Create(links.Link{Slug: "docs", URL: "https://example.com", CreatedAt: now, UpdatedAt: now})
	api := NewQRAPI(repo, config.Config{BaseURL: "https://short.example"})
	mux := http.NewServeMux()
	api.Routes(mux)
	for _, size := range []string{"abc", "127", "1025"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/links/"+itoa(link.ID)+"/qr?size="+size, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("size %s: expected 400, got %d", size, rec.Code)
		}
	}
}

func itoa(id int64) string {
	if id == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for id > 0 {
		i--
		buf[i] = byte('0' + id%10)
		id /= 10
	}
	return string(buf[i:])
}
