package analytics

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestQueryStoreSummary(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE click_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL,
		created_at TEXT NOT NULL,
		referrer TEXT,
		user_agent TEXT,
		country TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{base.Add(5 * time.Minute), base.Add(20 * time.Minute), base.Add(time.Hour + 2*time.Minute)} {
		_, err = db.Exec(`INSERT INTO click_events (slug, created_at) VALUES (?, ?)`, "abc", at.Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = db.Exec(`INSERT INTO click_events (slug, created_at) VALUES (?, ?)`, "other", base.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}

	store := NewQueryStore(db)
	got, err := store.Summary("abc", 9, base, base.Add(3*time.Hour), "hour")
	if err != nil {
		t.Fatal(err)
	}
	if got.LifetimeClicks != 9 || got.ClicksInRange != 3 {
		t.Fatalf("unexpected totals: %+v", got)
	}
	if len(got.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(got.Buckets))
	}
	if got.Buckets[0].Clicks != 2 || got.Buckets[1].Clicks != 1 {
		t.Fatalf("unexpected buckets: %+v", got.Buckets)
	}
}

func TestQueryStoreRejectsInvalidInterval(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewQueryStore(db)
	_, err = store.Summary("abc", 0, time.Now().Add(-time.Hour), time.Now(), "minute")
	if err == nil {
		t.Fatal("expected invalid interval error")
	}
}
