package analytics

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestQueryStoreSummary(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE click_events (id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL, created_at TEXT NOT NULL, referrer TEXT, user_agent TEXT, country TEXT)`)
	if err != nil { t.Fatal(err) }
	base := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	events := []struct{ at time.Time; ref, ua, country string }{
		{base.Add(5 * time.Minute), "https://google.com", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/140.0", "PK"},
		{base.Add(20 * time.Minute), "https://google.com", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) CriOS/140.0 Mobile", "PK"},
		{base.Add(time.Hour + 2*time.Minute), "https://news.example", "Mozilla/5.0 (Macintosh; Intel Mac OS X) Safari/18.0", "US"},
	}
	for _, e := range events { _, err = db.Exec(`INSERT INTO click_events (slug, created_at, referrer, user_agent, country) VALUES (?, ?, ?, ?, ?)`, "abc", e.at.Format(time.RFC3339Nano), e.ref, e.ua, e.country); if err != nil { t.Fatal(err) } }
	_, err = db.Exec(`INSERT INTO click_events (slug, created_at) VALUES (?, ?)`, "other", base.Format(time.RFC3339Nano)); if err != nil { t.Fatal(err) }
	got, err := NewQueryStore(db).Summary("abc", 9, base, base.Add(3*time.Hour), "hour")
	if err != nil { t.Fatal(err) }
	if got.LifetimeClicks != 9 || got.ClicksInRange != 3 { t.Fatalf("unexpected totals: %+v", got) }
	if len(got.Buckets) != 2 || got.Buckets[0].Clicks != 2 || got.Buckets[1].Clicks != 1 { t.Fatalf("unexpected buckets: %+v", got.Buckets) }
	if got.Referrers[0].Name != "https://google.com" || got.Referrers[0].Clicks != 2 { t.Fatalf("unexpected referrers: %+v", got.Referrers) }
	if got.Countries[0].Name != "PK" || got.Countries[0].Clicks != 2 { t.Fatalf("unexpected countries: %+v", got.Countries) }
	if got.Browsers[0].Name != "Chrome" || got.Browsers[0].Clicks != 2 { t.Fatalf("unexpected browsers: %+v", got.Browsers) }
	if got.Devices[0].Name != "Desktop" || got.Devices[0].Clicks != 2 { t.Fatalf("unexpected devices: %+v", got.Devices) }
}

func TestClassifyUserAgent(t *testing.T) {
	cases := []struct{ ua, browser, device string }{
		{"Mozilla/5.0 (iPhone) CriOS/140.0 Mobile", "Chrome", "Mobile"},
		{"Mozilla/5.0 (Windows NT 10.0) Edg/140.0", "Edge", "Desktop"},
		{"Googlebot/2.1", "Bot / crawler", "Desktop"},
	}
	for _, tc := range cases { browser, device := classifyUserAgent(tc.ua); if browser != tc.browser || device != tc.device { t.Errorf("classify(%q) = %q/%q, want %q/%q", tc.ua, browser, device, tc.browser, tc.device) } }
}

func TestQueryStoreRejectsInvalidInterval(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:"); if err != nil { t.Fatal(err) }; defer db.Close()
	_, err = NewQueryStore(db).Summary("abc", 0, time.Now().Add(-time.Hour), time.Now(), "minute")
	if err == nil { t.Fatal("expected invalid interval error") }
}
