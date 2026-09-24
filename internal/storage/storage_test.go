package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
)

func openTestDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shorty.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func seedClicks(t *testing.T, db *database.DB, n int, at time.Time) {
	t.Helper()
	ts := at.UTC().Format(time.RFC3339)
	for i := 0; i < n; i++ {
		if _, err := db.Exec(
			`INSERT INTO click_events (slug, created_at, referrer, user_agent, country) VALUES (?, ?, ?, ?, ?)`,
			"docs", ts, "https://example.com", "test-agent", "PK"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStatsEmptyDB(t *testing.T) {
	db, path := openTestDB(t)
	st, err := NewStore(db.DB, path).Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.DatabaseBytes <= 0 {
		t.Fatalf("expected positive db size, got %d", st.DatabaseBytes)
	}
	if st.PageSize <= 0 || st.PageCount <= 0 {
		t.Fatalf("expected page info, got size=%d count=%d", st.PageSize, st.PageCount)
	}
	if len(st.Tables) == 0 {
		t.Fatal("expected tracked tables")
	}
	if len(st.Suggestions) == 0 {
		t.Fatal("expected at least the healthy suggestion")
	}
	if st.Suggestions[0].Kind != "healthy" {
		t.Fatalf("expected healthy suggestion on empty db, got %q", st.Suggestions[0].Kind)
	}
}

func TestStatsGrowthAndSuggestions(t *testing.T) {
	db, path := openTestDB(t)
	now := time.Now().UTC()

	// 140 recent clicks (10/day) + 50 old clicks (>90d).
	seedClicks(t, db, 140, now.Add(-time.Hour))
	seedClicks(t, db, 50, now.Add(-100*24*time.Hour))

	// One expired link and one stale unused link.
	if _, err := db.Exec(`INSERT INTO links (slug, url, created_at, updated_at, expires_at, clicks)
		VALUES ('old', 'https://example.com/old', ?, ?, ?, 0)`,
		now.Add(-40*24*time.Hour).Format(time.RFC3339),
		now.Add(-40*24*time.Hour).Format(time.RFC3339),
		now.Add(-time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO links (slug, url, created_at, updated_at, clicks)
		VALUES ('stale', 'https://example.com/stale', ?, ?, 0)`,
		now.Add(-40*24*time.Hour).Format(time.RFC3339),
		now.Add(-40*24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	st, err := NewStore(db.DB, path).Stats()
	if err != nil {
		t.Fatal(err)
	}

	if st.ClicksPerDay != 10 {
		t.Fatalf("expected 10 clicks/day, got %v", st.ClicksPerDay)
	}
	if st.BytesPerClick <= 0 {
		t.Fatalf("expected positive bytes/click, got %v", st.BytesPerClick)
	}
	if st.Projected30d <= 0 || st.Projected90d <= st.Projected30d || st.Projected365d <= st.Projected90d {
		t.Fatalf("projections not monotonic: %d %d %d", st.Projected30d, st.Projected90d, st.Projected365d)
	}

	kinds := map[string]bool{}
	for _, s := range st.Suggestions {
		kinds[s.Kind] = true
	}
	for _, want := range []string{"retention", "expired_links", "unused_links"} {
		if !kinds[want] {
			t.Fatalf("expected %q suggestion, got kinds %v", want, kinds)
		}
	}

	var clickTable *TableStat
	for i := range st.Tables {
		if st.Tables[i].Name == "click_events" {
			clickTable = &st.Tables[i]
		}
	}
	if clickTable == nil || clickTable.Rows != 190 {
		t.Fatalf("expected 190 click rows, got %+v", clickTable)
	}
	if clickTable.Bytes <= 0 {
		t.Fatal("expected positive click table bytes estimate")
	}
}

func TestStatsNoPath(t *testing.T) {
	db, _ := openTestDB(t)
	st, err := NewStore(db.DB, "").Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.WALBytes != 0 {
		t.Fatalf("expected 0 wal bytes without path, got %d", st.WALBytes)
	}
}
