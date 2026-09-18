package links

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
)

func TestSQLiteRepositoryCRUD(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := NewSQLiteRepository(db.DB)
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := repo.Create(Link{Slug: "docs", URL: "https://example.com", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 {
		t.Fatal("expected generated ID")
	}

	got, err := repo.GetBySlug("docs")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://example.com" {
		t.Fatalf("unexpected URL: %s", got.URL)
	}

	got.URL = "https://example.org"
	got.UpdatedAt = time.Now().UTC()
	if _, err := repo.Update(got); err != nil {
		t.Fatal(err)
	}

	items, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 link, got %d", len(items))
	}

	if err := repo.Delete(got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(got.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
