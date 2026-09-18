package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/links"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) CreateLink(link links.Link) (links.Link, error) {
	payload := struct {
		URL string `json:"url"`
		Slug string `json:"slug,omitempty"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
		MaxClicks *int64 `json:"max_clicks,omitempty"`
	}{link.URL, link.Slug, link.ExpiresAt, link.MaxClicks}
	var out links.Link
	if err := c.do(http.MethodPost, "/api/v1/links", payload, &out); err != nil { return links.Link{}, err }
	return out, nil
}

func (c *Client) Stats(id int64, from, to time.Time, interval string) (analytics.Summary, error) {
	path := "/api/v1/links/" + strconv.FormatInt(id, 10) + "/analytics"
	q := url.Values{}
	if !from.IsZero() { q.Set("from", from.UTC().Format(time.RFC3339)) }
	if !to.IsZero() { q.Set("to", to.UTC().Format(time.RFC3339)) }
	if interval != "" { q.Set("interval", interval) }
	if encoded := q.Encode(); encoded != "" { path += "?" + encoded }
	var out analytics.Summary
	if err := c.do(http.MethodGet, path, nil, &out); err != nil { return analytics.Summary{}, err }
	return out, nil
}

func (c *Client) do(method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil { data, err := json.Marshal(payload); if err != nil { return fmt.Errorf("encode request: %w", err) }; body = bytes.NewReader(data) }
	req, err := http.NewRequest(method, c.BaseURL+path, body); if err != nil { return fmt.Errorf("create request: %w", err) }
	if payload != nil { req.Header.Set("Content-Type", "application/json") }
	resp, err := c.HTTP.Do(req); if err != nil { return fmt.Errorf("request failed: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { var e struct{ Error string `json:"error"` }; _ = json.NewDecoder(resp.Body).Decode(&e); if e.Error == "" { e.Error = resp.Status }; return fmt.Errorf("shorty: %s", e.Error) }
	if out != nil { if err := json.NewDecoder(resp.Body).Decode(out); err != nil { return fmt.Errorf("decode response: %w", err) } }
	return nil
}
