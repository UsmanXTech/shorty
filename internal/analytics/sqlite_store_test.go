package analytics

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
)

func TestSQLiteStorePersistsCountry(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	event := Event{
		Slug:      "abc123",
		CreatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		Referrer:  "https://example.com",
		UserAgent: "ShortyTest/1.0",
		Country:   "PK",
	}
	if err := NewSQLiteStore(db.DB).RecordEvent(event); err != nil {
		t.Fatal(err)
	}

	var country string
	if err := db.QueryRow("SELECT country FROM click_events WHERE slug = ?", event.Slug).Scan(&country); err != nil {
		t.Fatal(err)
	}
	if country != "PK" {
		t.Fatalf("stored country = %q, want PK", country)
	}
}
