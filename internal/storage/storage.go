// Package storage reports database size, per-table usage, growth
// projections and cleanup suggestions so operators can keep a
// self-hosted Shorty cheap to run.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// Tables we report row counts for. click_events and links also get a
// payload-bytes estimate; the rest are rows-only.
var trackedTables = []string{
	"links",
	"click_events",
	"teams",
	"api_keys",
	"domains",
	"link_variants",
	"webhooks",
	"webhook_deliveries",
}

// TableStat is usage info for one table.
type TableStat struct {
	Name  string `json:"name"`
	Rows  int64  `json:"rows"`
	Bytes int64  `json:"bytes"` // estimated payload bytes, 0 when unknown
}

// Suggestion is one actionable cleanup recommendation.
type Suggestion struct {
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	Detail          string `json:"detail"`
	ReclaimableByte int64  `json:"reclaimable_bytes"`
}

// Stats is the full storage report.
type Stats struct {
	DatabaseBytes int64       `json:"database_bytes"`
	WALBytes      int64       `json:"wal_bytes"`
	FreelistBytes int64       `json:"freelist_bytes"`
	PageSize      int64       `json:"page_size"`
	PageCount     int64       `json:"page_count"`
	Tables        []TableStat `json:"tables"`
	ClicksPerDay  float64     `json:"clicks_per_day"`
	BytesPerClick float64     `json:"bytes_per_click"`
	Projected30d  int64       `json:"projected_bytes_30d"`
	Projected90d  int64       `json:"projected_bytes_90d"`
	Projected365d int64       `json:"projected_bytes_365d"`
	Suggestions   []Suggestion `json:"suggestions"`
}

// Store gathers storage stats from the SQLite database.
type Store struct {
	db   *sql.DB
	path string
}

// NewStore builds a Store. path is the database file path and may be
// empty, in which case WAL size is reported as 0.
func NewStore(db *sql.DB, path string) *Store {
	return &Store{db: db, path: path}
}

func pragmaInt(db *sql.DB, name string) int64 {
	var v int64
	if err := db.QueryRow(fmt.Sprintf("PRAGMA %s;", name)).Scan(&v); err != nil {
		return 0
	}
	return v
}

func tableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?;`, name).Scan(&n)
	return err == nil && n > 0
}

// payloadBytes estimates the stored payload of click_events rows from the
// summed column lengths plus a per-row overhead allowance.
func clickPayloadBytes(db *sql.DB) (rows int64, bytes int64) {
	if !tableExists(db, "click_events") {
		return 0, 0
	}
	var payload sql.NullInt64
	err := db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(
		         LENGTH(slug) + LENGTH(created_at)
		         + LENGTH(COALESCE(referrer, '')) + LENGTH(COALESCE(user_agent, ''))
		         + LENGTH(COALESCE(country, '')) + 32
		       ), 0)
		FROM click_events;`).Scan(&rows, &payload)
	if err != nil {
		return 0, 0
	}
	return rows, payload.Int64
}

func linkPayloadBytes(db *sql.DB) (rows int64, bytes int64) {
	if !tableExists(db, "links") {
		return 0, 0
	}
	var payload sql.NullInt64
	err := db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(
		         LENGTH(slug) + LENGTH(url) + LENGTH(created_at) + 32
		       ), 0)
		FROM links;`).Scan(&rows, &payload)
	if err != nil {
		return 0, 0
	}
	return rows, payload.Int64
}

func countWhere(db *sql.DB, table, where string, args ...any) int64 {
	if !tableExists(db, table) {
		return 0
	}
	var n int64
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s;", table, where), args...).Scan(&n); err != nil {
		return 0
	}
	return n
}

// Stats collects the storage report.
func (s *Store) Stats() (Stats, error) {
	var st Stats

	st.PageSize = pragmaInt(s.db, "page_size")
	st.PageCount = pragmaInt(s.db, "page_count")
	freelist := pragmaInt(s.db, "freelist_count")
	st.DatabaseBytes = st.PageSize * st.PageCount
	st.FreelistBytes = freelist * st.PageSize

	if s.path != "" {
		if fi, err := os.Stat(s.path + "-wal"); err == nil {
			st.WALBytes = fi.Size()
		}
	}

	clickRows, clickBytes := clickPayloadBytes(s.db)
	linkRows, linkBytes := linkPayloadBytes(s.db)

	st.Tables = make([]TableStat, 0, len(trackedTables))
	for _, name := range trackedTables {
		ts := TableStat{Name: name}
		if !tableExists(s.db, name) {
			continue
		}
		_ = s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s;", name)).Scan(&ts.Rows)
		switch name {
		case "click_events":
			ts.Bytes = clickBytes
		case "links":
			ts.Bytes = linkBytes
		}
		st.Tables = append(st.Tables, ts)
	}
	_ = clickRows
	_ = linkRows

	// Growth: average clicks/day over the last 14 days.
	now := time.Now().UTC()
	cutoff := now.Add(-14 * 24 * time.Hour).Format(time.RFC3339)
	recentClicks := countWhere(s.db, "click_events", "created_at >= ?", cutoff)
	st.ClicksPerDay = float64(recentClicks) / 14.0
	if clickRows > 0 {
		st.BytesPerClick = float64(clickBytes) / float64(clickRows)
	} else {
		// Sensible default (~220B per click row) until data exists.
		st.BytesPerClick = 220
	}
	st.Projected30d = int64(st.ClicksPerDay * st.BytesPerClick * 30)
	st.Projected90d = int64(st.ClicksPerDay * st.BytesPerClick * 90)
	st.Projected365d = int64(st.ClicksPerDay * st.BytesPerClick * 365)

	st.Suggestions = s.suggestions(st, clickBytes, now)

	return st, nil
}

func (s *Store) suggestions(st Stats, clickBytes int64, now time.Time) []Suggestion {
	var out []Suggestion

	// 1. Reclaimable free pages.
	if st.FreelistBytes >= 1<<20 {
		out = append(out, Suggestion{
			Kind:            "vacuum",
			Title:           "Run VACUUM to reclaim space",
			Detail:          fmt.Sprintf("%s of free pages can be returned to the OS. Schedule 'VACUUM' during low traffic (it rewrites the DB file).", humanBytes(st.FreelistBytes)),
			ReclaimableByte: st.FreelistBytes,
		})
	}

	// 2. Old click data (retention candidate).
	oldCutoff := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	oldClicks := countWhere(s.db, "click_events", "created_at < ?", oldCutoff)
	if oldClicks > 0 {
		avg := st.BytesPerClick
		if avg <= 0 {
			avg = 220
		}
		reclaim := int64(float64(oldClicks) * avg)
		out = append(out, Suggestion{
			Kind:  "retention",
			Title: "Old click events can be archived",
			Detail: fmt.Sprintf("%s clicks are older than 90 days (~%s). Consider a retention policy or CSV export + delete to keep the DB small.",
				humanCount(oldClicks), humanBytes(reclaim)),
			ReclaimableByte: reclaim,
		})
	}

	// 3. Expired links.
	expired := countWhere(s.db, "links", "expires_at IS NOT NULL AND expires_at < ?", now.Format(time.RFC3339))
	if expired > 0 {
		out = append(out, Suggestion{
			Kind:            "expired_links",
			Title:           "Delete expired links",
			Detail:          fmt.Sprintf("%s links are past their expiry date and can be removed.", humanCount(expired)),
			ReclaimableByte: 0,
		})
	}

	// 4. Never-clicked old links.
	staleCutoff := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	stale := countWhere(s.db, "links", "clicks = 0 AND created_at < ?", staleCutoff)
	if stale > 0 {
		out = append(out, Suggestion{
			Kind:            "unused_links",
			Title:           "Review unused links",
			Detail:          fmt.Sprintf("%s links older than 30 days were never clicked. Deleting them keeps the dataset lean.", humanCount(stale)),
			ReclaimableByte: 0,
		})
	}

	if len(out) == 0 {
		out = append(out, Suggestion{
			Kind:            "healthy",
			Title:           "Storage looks healthy",
			Detail:          "No significant reclaimable space found. Keep an eye on click growth.",
			ReclaimableByte: 0,
		})
	}
	return out
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func humanCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}
