package analytics

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Bucket struct {
	Period time.Time `json:"period"`
	Clicks int64     `json:"clicks"`
}
type BreakdownItem struct {
	Name   string `json:"name"`
	Clicks int64  `json:"clicks"`
}
type Summary struct {
	Slug           string          `json:"slug"`
	LifetimeClicks int64           `json:"lifetime_clicks"`
	ClicksInRange  int64           `json:"clicks_in_range"`
	From           time.Time       `json:"from"`
	To             time.Time       `json:"to"`
	Interval       string          `json:"interval"`
	Buckets        []Bucket        `json:"buckets"`
	Referrers      []BreakdownItem `json:"referrers"`
	Browsers       []BreakdownItem `json:"browsers"`
	Devices        []BreakdownItem `json:"devices"`
	Countries      []BreakdownItem `json:"countries"`
}
type QueryStore struct{ db *sql.DB }

func NewQueryStore(db *sql.DB) *QueryStore { return &QueryStore{db: db} }
func (s *QueryStore) Summary(slug string, lifetimeClicks int64, from, to time.Time, interval string) (Summary, error) {
	if !from.Before(to) {
		return Summary{}, fmt.Errorf("from must be before to")
	}
	if interval != "hour" && interval != "day" {
		return Summary{}, fmt.Errorf("interval must be hour or day")
	}
	fromUTC, toUTC := from.UTC(), to.UTC()
	fromText, toText := fromUTC.Format(time.RFC3339Nano), toUTC.Format(time.RFC3339Nano)
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ?`, slug, fromText, toText).Scan(&total); err != nil {
		return Summary{}, fmt.Errorf("count click events: %w", err)
	}
	format := "%Y-%m-%dT%H:00:00Z"
	if interval == "day" {
		format = "%Y-%m-%dT00:00:00Z"
	}
	rows, err := s.db.Query(`SELECT strftime(?, created_at), COUNT(*) FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ? GROUP BY 1 ORDER BY 1`, format, slug, fromText, toText)
	if err != nil {
		return Summary{}, fmt.Errorf("query click buckets: %w", err)
	}
	buckets, err := scanBuckets(rows)
	if err != nil {
		return Summary{}, err
	}
	referrers, browsers, devices, countries, err := s.breakdowns(slug, fromText, toText)
	if err != nil {
		return Summary{}, err
	}
	return Summary{Slug: slug, LifetimeClicks: lifetimeClicks, ClicksInRange: total, From: fromUTC, To: toUTC, Interval: interval, Buckets: buckets, Referrers: referrers, Browsers: browsers, Devices: devices, Countries: countries}, nil
}
func scanBuckets(rows *sql.Rows) ([]Bucket, error) {
	defer rows.Close()
	buckets := make([]Bucket, 0)
	for rows.Next() {
		var period string
		var clicks int64
		if err := rows.Scan(&period, &clicks); err != nil {
			return nil, fmt.Errorf("scan click bucket: %w", err)
		}
		t, err := time.Parse("2006-01-02T15:04:05Z", period)
		if err != nil {
			return nil, fmt.Errorf("parse click bucket: %w", err)
		}
		buckets = append(buckets, Bucket{Period: t.UTC(), Clicks: clicks})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate click buckets: %w", err)
	}
	return buckets, nil
}
func (s *QueryStore) breakdowns(slug, from, to string) ([]BreakdownItem, []BreakdownItem, []BreakdownItem, []BreakdownItem, error) {
	// Aggregate in SQL with GROUP BY so a link with millions of clicks
	// never materializes every row in memory. Browsers/devices need Go-side
	// user-agent classification, so group by the raw agent and classify only
	// the distinct values.
	refs, err := s.groupedCounts(`SELECT referrer, COUNT(*) AS n FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ? GROUP BY referrer ORDER BY n DESC LIMIT 10`, slug, from, to)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	countries, err := s.groupedCounts(`SELECT country, COUNT(*) AS n FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ? GROUP BY country ORDER BY n DESC LIMIT 10`, slug, from, to)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	agents, err := s.groupedCounts(`SELECT user_agent, COUNT(*) AS n FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ? GROUP BY user_agent ORDER BY n DESC LIMIT 100`, slug, from, to)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	refItems := make([]BreakdownItem, 0, len(refs))
	for _, kv := range refs {
		name := kv.name
		if name == "" {
			name = "Direct / none"
		}
		refItems = append(refItems, BreakdownItem{Name: name, Clicks: kv.count})
	}
	countryItems := make([]BreakdownItem, 0, len(countries))
	for _, kv := range countries {
		name := kv.name
		if name == "" {
			name = "Unknown"
		}
		countryItems = append(countryItems, BreakdownItem{Name: name, Clicks: kv.count})
	}
	browserCounts, deviceCounts := map[string]int64{}, map[string]int64{}
	for _, kv := range agents {
		browser, device := classifyUserAgent(kv.name)
		browserCounts[browser] += kv.count
		deviceCounts[device] += kv.count
	}
	return refItems, topBreakdown(browserCounts), topBreakdown(deviceCounts), countryItems, nil
}

type nameCount struct {
	name  string
	count int64
}

func (s *QueryStore) groupedCounts(query, slug, from, to string) ([]nameCount, error) {
	rows, err := s.db.Query(query, slug, from, to)
	if err != nil {
		return nil, fmt.Errorf("query analytics breakdown: %w", err)
	}
	defer rows.Close()
	var out []nameCount
	for rows.Next() {
		var kv nameCount
		if err := rows.Scan(&kv.name, &kv.count); err != nil {
			return nil, fmt.Errorf("scan analytics breakdown: %w", err)
		}
		out = append(out, kv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics breakdown: %w", err)
	}
	return out, nil
}
func topBreakdown(values map[string]int64) []BreakdownItem {
	items := make([]BreakdownItem, 0, len(values))
	for name, clicks := range values {
		items = append(items, BreakdownItem{Name: name, Clicks: clicks})
	}
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && (items[j].Clicks > items[j-1].Clicks || (items[j].Clicks == items[j-1].Clicks && items[j].Name < items[j-1].Name)); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	if len(items) > 10 {
		items = items[:10]
	}
	return items
}
func classifyUserAgent(ua string) (browser, device string) {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "bot") || strings.Contains(lower, "spider") || strings.Contains(lower, "crawler"):
		browser = "Bot / crawler"
	case strings.Contains(lower, "edg/") || strings.Contains(lower, "edge/"):
		browser = "Edge"
	case strings.Contains(lower, "opr/") || strings.Contains(lower, "opera"):
		browser = "Opera"
	case strings.Contains(lower, "firefox"):
		browser = "Firefox"
	case strings.Contains(lower, "chrome") || strings.Contains(lower, "crios"):
		browser = "Chrome"
	case strings.Contains(lower, "safari"):
		browser = "Safari"
	default:
		browser = "Other"
	}
	switch {
	case strings.Contains(lower, "ipad") || strings.Contains(lower, "tablet"):
		device = "Tablet"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "android") || strings.Contains(lower, "mobile"):
		device = "Mobile"
	default:
		device = "Desktop"
	}
	return browser, device
}
