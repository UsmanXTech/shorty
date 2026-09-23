package domains

import (
	"path/filepath"
	"testing"

	"github.com/UsmanXTech/shorty/internal/database"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Example.COM":         "example.com",
		"example.com:8080":    "example.com",
		"example.com.":        "example.com",
		"  Sub.Example.COM  ": "sub.example.com",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()
	d, err := s.Create(1, "Example.COM:8080")
	if err != nil {
		t.Fatal(err)
	}
	if d.Domain != "example.com" {
		t.Fatalf("expected normalized domain, got %q", d.Domain)
	}
	if _, err := s.Create(1, "example.com"); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	got, err := s.GetByHost("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != d.ID {
		t.Fatalf("unexpected domain: %+v", got)
	}
	if _, err := s.GetByHost("other.com"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(d.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStore(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewSQLiteStore(db.DB)
	d, err := s.Create(1, "short.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(1, "short.example"); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	list, err := s.List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Domain != "short.example" {
		t.Fatalf("unexpected list: %+v", list)
	}
	got, err := s.GetByHost("short.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != d.ID {
		t.Fatalf("unexpected domain: %+v", got)
	}
	if err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
}
