package redirect

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/database"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/variants"
)

func BenchmarkRedirectSQLite(b *testing.B) {
	db, err := database.Open(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	repo := links.NewSQLiteRepository(db.DB)
	if _, err := repo.Create(links.Link{Slug: "bench", URL: "https://example.com"}); err != nil {
		b.Fatal(err)
	}
	vstore := variants.NewSQLiteStore(db.DB)

	serve := func(b *testing.B, h *Handler) {
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest(http.MethodGet, "/bench", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusFound {
				b.Fatalf("expected 302, got %d", rec.Code)
			}
		}
	}

	b.Run("with-list-cache", func(b *testing.B) {
		h := New(repo, cache.New()).WithVariants(vstore).WithVariantListCache(variants.NewListCache())
		serve(b, h)
	})
	b.Run("no-list-cache", func(b *testing.B) {
		h := New(repo, cache.New()).WithVariants(vstore) // List() queried every click
		serve(b, h)
	})
}
