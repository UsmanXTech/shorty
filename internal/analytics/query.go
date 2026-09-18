package analytics

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Bucket struct { Period time.Time `json:"period"`; Clicks int64 `json:"clicks"` }
type BreakdownItem struct { Name string `json:"name"`; Clicks int64 `json:"clicks"` }
type Summary struct { Slug string `json:"slug"`; LifetimeClicks int64 `json:"lifetime_clicks"`; ClicksInRange int64 `json:"clicks_in_range"`; From time.Time `json:"from"`; To time.Time `json:"to"`; Interval string `json:"interval"`; Buckets []Bucket `json:"buckets"`; Referrers []BreakdownItem `json:"referrers"`; Browsers []BreakdownItem `json:"browsers"`; Devices []BreakdownItem `json:"devices"`; Countries []BreakdownItem `json:"countries"` }
type QueryStore struct { db *sql.DB }
func NewQueryStore(db *sql.DB) *QueryStore { return &QueryStore{db: db} }
func (s *QueryStore) Summary(slug string, lifetimeClicks int64, from, to time.Time, interval string) (Summary, error) {
	if !from.Before(to) { return Summary{}, fmt.Errorf("from must be before to") }; if interval != "hour" && interval != "day" { return Summary{}, fmt.Errorf("interval must be hour or day") }
	fromUTC, toUTC := from.UTC(), to.UTC(); fromText, toText := fromUTC.Format(time.RFC3339Nano), toUTC.Format(time.RFC3339Nano)
	var total int64; if err := s.db.QueryRow(`SELECT COUNT(*) FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ?`, slug, fromText, toText).Scan(&total); err != nil { return Summary{}, fmt.Errorf("count click events: %w", err) }
	format := "%Y-%m-%dT%H:00:00Z"; if interval == "day" { format = "%Y-%m-%dT00:00:00Z" }
	rows, err := s.db.Query(`SELECT strftime(?, created_at), COUNT(*) FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ? GROUP BY 1 ORDER BY 1`, format, slug, fromText, toText); if err != nil { return Summary{}, fmt.Errorf("query click buckets: %w", err) }
	buckets, err := scanBuckets(rows); if err != nil { return Summary{}, err }
	referrers, browsers, devices, countries, err := s.breakdowns(slug, fromText, toText); if err != nil { return Summary{}, err }
	return Summary{Slug: slug, LifetimeClicks: lifetimeClicks, ClicksInRange: total, From: fromUTC, To: toUTC, Interval: interval, Buckets: buckets, Referrers: referrers, Browsers: browsers, Devices: devices, Countries: countries}, nil
}
func scanBuckets(rows *sql.Rows) ([]Bucket, error) { defer rows.Close(); buckets := make([]Bucket, 0); for rows.Next() { var period string; var clicks int64; if err := rows.Scan(&period, &clicks); err != nil { return nil, fmt.Errorf("scan click bucket: %w", err) }; t, err := time.Parse("2006-01-02T15:04:05Z", period); if err != nil { return nil, fmt.Errorf("parse click bucket: %w", err) }; buckets = append(buckets, Bucket{Period: t.UTC(), Clicks: clicks}) }; if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate click buckets: %w", err) }; return buckets, nil }
func (s *QueryStore) breakdowns(slug, from, to string) ([]BreakdownItem, []BreakdownItem, []BreakdownItem, []BreakdownItem, error) { rows, err := s.db.Query(`SELECT referrer, user_agent, country FROM click_events WHERE slug = ? AND created_at >= ? AND created_at < ?`, slug, from, to); if err != nil { return nil,nil,nil,nil,fmt.Errorf("query analytics breakdowns: %w",err) }; defer rows.Close(); refs,browsers,devices,countries:=map[string]int64{},map[string]int64{},map[string]int64{},map[string]int64{}; for rows.Next(){var ref,ua,country string; if err:=rows.Scan(&ref,&ua,&country);err!=nil{return nil,nil,nil,nil,fmt.Errorf("scan analytics breakdown: %w",err)};if ref==""{ref="Direct / none"};if country==""{country="Unknown"};browser,device:=classifyUserAgent(ua);refs[ref]++;browsers[browser]++;devices[device]++;countries[country]++};if err:=rows.Err();err!=nil{return nil,nil,nil,nil,fmt.Errorf("iterate analytics breakdowns: %w",err)};return topBreakdown(refs),topBreakdown(browsers),topBreakdown(devices),topBreakdown(countries),nil }
func topBreakdown(values map[string]int64) []BreakdownItem { items:=make([]BreakdownItem,0,len(values));for name,clicks:=range values{items=append(items,BreakdownItem{Name:name,Clicks:clicks})};for i:=1;i<len(items);i++{for j:=i;j>0&&(items[j].Clicks>items[j-1].Clicks||(items[j].Clicks==items[j-1].Clicks&&items[j].Name<items[j-1].Name));j--{items[j],items[j-1]=items[j-1],items[j]}};if len(items)>10{items=items[:10]};return items }
func classifyUserAgent(ua string)(browser,device string){lower:=strings.ToLower(ua);switch{case strings.Contains(lower,"bot")||strings.Contains(lower,"spider")||strings.Contains(lower,"crawler"):browser="Bot / crawler";case strings.Contains(lower,"edg/")||strings.Contains(lower,"edge/"):browser="Edge";case strings.Contains(lower,"opr/")||strings.Contains(lower,"opera"):browser="Opera";case strings.Contains(lower,"firefox"):browser="Firefox";case strings.Contains(lower,"chrome")||strings.Contains(lower,"crios"):browser="Chrome";case strings.Contains(lower,"safari"):browser="Safari";default:browser="Other"};switch{case strings.Contains(lower,"ipad")||strings.Contains(lower,"tablet"):device="Tablet";case strings.Contains(lower,"iphone")||strings.Contains(lower,"android")||strings.Contains(lower,"mobile"):device="Mobile";default:device="Desktop"};return browser,device}
