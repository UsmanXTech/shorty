package analytics

import (
	"database/sql"
	"fmt"
	"time"
)

type Bucket struct {
	Period time.Time `json:"period"`
	Clicks int64     `json:"clicks"`
}

type Summary struct {
	Slug          string  `json:"slug"`
	LifetimeClicks int64  `json:"lifetime_clicks"`
	ClicksInRange int64   `json:"clicks_in_range"`
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	Interval      string  `json:"interval"`
	Buckets       []Bucket `json:"buckets"`
}

type QueryStore struct {
	db *sql.DB
}

func NewQueryStore(db *sql.DB) *QueryStore {
	return &QueryStore{db: db}
}

func (s *QueryStore) Summary(slug string, lifetimeClicks int64, from, to time.Time, interval string) (Summary, error) {
	if !from.Before(to) {
		return Summary{}, fmt.Errorf("from must be before to")
	}
	if interval != "hour" && interval != "day" {
		return Summary{}, fmt.Errorf("interval must be hour or day")
	}

	var total int64
	if err := s.db.QueryRow(`
SELECT COUNT(*) FROM click_events
WHERE slug = ? AND created_at >= ? AND created_at < ?`,
		slug, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).Scan(&total); err != nil {
		return Summary{}, fmt.Errorf("count click events: %w", err)
	}

	format := "%Y-%m-%dT%H:00:00Z"
	if interval == "day" {
		format = "%Y-%m-%dT00:00:00Z"
	}
	rows, err := s.db.Query(`
SELECT strftime(?, created_at) AS period, COUNT(*) AS clicks
FROM click_events
WHERE slug = ? AND created_at >= ? AND created_at < ?
GROUP BY period
ORDER BY period`, format, slug, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Summary{}, fmt.Errorf("query click buckets: %w", err)
	}
	defer rows.Close()

	buckets := make([]Bucket, 0)
	for rows.Next() {
		var period string
		var clicks int64
		if err := rows.Scan(&period, &clicks); err != nil {
			return Summary{}, fmt.Errorf("scan click bucket: %w", err)
		}
		t, err := time.Parse("2006-01-02T15:04:05Z", period)
		if err != nil {
			return Summary{}, fmt.Errorf("parse click bucket: %w", err)
		}
		buckets = append(buckets, Bucket{Period: t.UTC(), Clicks: clicks})
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate click buckets: %w", err)
	}

	return Summary{Slug: slug, LifetimeClicks: lifetimeClicks, ClicksInRange: total, From: from.UTC(), To: to.UTC(), Interval: interval, Buckets: buckets}, nil
}
