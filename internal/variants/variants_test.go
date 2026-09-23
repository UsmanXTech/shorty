package variants

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
	"github.com/UsmanXTech/shorty/internal/links"
)

func TestMemoryStoreCRUD(t *testing.T) {
	s := NewMemoryStore()
	s.AddLink(42)

	a, err := s.Create(42, "https://a.example", 3)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(42, "https://b.example", 1)
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.List(42)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(list))
	}

	updated, err := s.Update(a.ID, "https://a2.example", 5)
	if err != nil {
		t.Fatal(err)
	}
	if updated.URL != "https://a2.example" || updated.Weight != 5 {
		t.Fatalf("unexpected update: %+v", updated)
	}

	if err := s.IncrementClicks(b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(b.ID)
	if got.Clicks != 1 {
		t.Fatalf("expected 1 click, got %d", got.Clicks)
	}

	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(a.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := s.Create(99, "https://x.example", 1); err != ErrNoLink {
		t.Fatalf("expected ErrNoLink, got %v", err)
	}
}

func TestPickWeighted(t *testing.T) {
	list := []Variant{
		{ID: 1, Weight: 3},
		{ID: 2, Weight: 1},
	}
	counts := map[int64]int{}
	for i := int64(0); i < 400; i++ {
		v := Pick(list, i)
		if v == nil {
			t.Fatal("expected a pick")
		}
		counts[v.ID]++
	}
	if counts[1] != 300 || counts[2] != 100 {
		t.Fatalf("unexpected distribution: %v", counts)
	}
	if Pick(nil, 0) != nil {
		t.Fatal("expected nil for empty list")
	}
	if Pick([]Variant{{ID: 1, Weight: 0}}, 0) != nil {
		t.Fatal("expected nil when all weights are zero")
	}
}

func TestSQLiteStore(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	linkRepo := links.NewSQLiteRepository(db.DB)
	now := time.Now().UTC()
	link, err := linkRepo.Create(links.Link{Slug: "ab", URL: "https://example.com", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	s := NewSQLiteStore(db.DB)
	v, err := s.Create(link.ID, "https://v1.example", 2)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID == 0 || v.LinkID != link.ID || v.Weight != 2 {
		t.Fatalf("unexpected variant: %+v", v)
	}

	if err := s.IncrementClicks(v.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Clicks != 1 {
		t.Fatalf("expected 1 click, got %d", got.Clicks)
	}

	list, err := s.List(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(list))
	}

	if err := s.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(v.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := s.Create(999999, "https://x.example", 1); err != ErrNoLink {
		t.Fatalf("expected ErrNoLink, got %v", err)
	}
}
