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
	APIKey  string
}

func NewClient(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 10 * time.Second}}
}

// WithAPIKey attaches a team API key sent as a Bearer token.
func (c *Client) WithAPIKey(key string) *Client {
	c.APIKey = key
	return c
}

func (c *Client) CreateLink(link links.Link) (links.Link, error) {
	payload := struct {
		URL       string     `json:"url"`
		Slug      string     `json:"slug,omitempty"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
		MaxClicks *int64     `json:"max_clicks,omitempty"`
		Password  *string    `json:"password,omitempty"`
		DomainID  *int64     `json:"domain_id,omitempty"`
		Domain    string     `json:"domain,omitempty"`
	}{link.URL, link.Slug, link.ExpiresAt, link.MaxClicks, nil, link.DomainID, ""}
	var out links.Link
	if err := c.do(http.MethodPost, "/api/v1/links", payload, &out); err != nil {
		return links.Link{}, err
	}
	return out, nil
}

// CreateProtectedLink creates a password-protected link.
func (c *Client) CreateProtectedLink(link links.Link, password string) (links.Link, error) {
	payload := map[string]any{"url": link.URL, "password": password}
	if link.Slug != "" {
		payload["slug"] = link.Slug
	}
	if link.ExpiresAt != nil {
		payload["expires_at"] = link.ExpiresAt
	}
	if link.MaxClicks != nil {
		payload["max_clicks"] = link.MaxClicks
	}
	if link.DomainID != nil {
		payload["domain_id"] = link.DomainID
	}
	var out links.Link
	if err := c.do(http.MethodPost, "/api/v1/links", payload, &out); err != nil {
		return links.Link{}, err
	}
	return out, nil
}

// SetLinkPassword sets (or, with an empty password, clears) a link password.
func (c *Client) SetLinkPassword(id int64, password string) (links.Link, error) {
	var out links.Link
	if err := c.do(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(id, 10), map[string]any{"password": password}, &out); err != nil {
		return links.Link{}, err
	}
	return out, nil
}

func (c *Client) Stats(id int64, from, to time.Time, interval string) (analytics.Summary, error) {
	path := "/api/v1/links/" + strconv.FormatInt(id, 10) + "/analytics"
	q := url.Values{}
	if !from.IsZero() {
		q.Set("from", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	if interval != "" {
		q.Set("interval", interval)
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out analytics.Summary
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return analytics.Summary{}, err
	}
	return out, nil
}

func (c *Client) do(method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("shorty: %s", e.Error)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) doRaw(method, path string, body io.Reader, contentType string, out io.Writer) error {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("shorty: %s", e.Error)
	}
	if out != nil {
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("read response: %w", err)
		}
	}
	return nil
}
