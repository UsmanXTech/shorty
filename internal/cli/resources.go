package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/teams"
	"github.com/UsmanXTech/shorty/internal/variants"
	"github.com/UsmanXTech/shorty/internal/webhooks"
)

// Variants (A/B testing)

func (c *Client) ListVariants(linkID int64) ([]variants.Variant, error) {
	var out []variants.Variant
	if err := c.do(http.MethodGet, "/api/v1/links/"+strconv.FormatInt(linkID, 10)+"/variants", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddVariant(linkID int64, url string, weight int) (variants.Variant, error) {
	var out variants.Variant
	payload := map[string]any{"url": url, "weight": weight}
	if err := c.do(http.MethodPost, "/api/v1/links/"+strconv.FormatInt(linkID, 10)+"/variants", payload, &out); err != nil {
		return variants.Variant{}, err
	}
	return out, nil
}

func (c *Client) RemoveVariant(linkID, variantID int64) error {
	return c.do(http.MethodDelete, "/api/v1/links/"+strconv.FormatInt(linkID, 10)+"/variants/"+strconv.FormatInt(variantID, 10), nil, nil)
}

// Domains

func (c *Client) ListDomains() ([]domains.Domain, error) {
	var out []domains.Domain
	if err := c.do(http.MethodGet, "/api/v1/domains", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddDomain(domain string) (domains.Domain, error) {
	var out domains.Domain
	if err := c.do(http.MethodPost, "/api/v1/domains", map[string]string{"domain": domain}, &out); err != nil {
		return domains.Domain{}, err
	}
	return out, nil
}

func (c *Client) RemoveDomain(id int64) error {
	return c.do(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(id, 10), nil, nil)
}

// Webhooks

func (c *Client) ListWebhooks() ([]webhooks.Webhook, error) {
	var out []webhooks.Webhook
	if err := c.do(http.MethodGet, "/api/v1/webhooks", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddWebhook(url string, events []string) (webhooks.Webhook, error) {
	var out webhooks.Webhook
	if err := c.do(http.MethodPost, "/api/v1/webhooks", map[string]any{"url": url, "events": events}, &out); err != nil {
		return webhooks.Webhook{}, err
	}
	return out, nil
}

func (c *Client) RemoveWebhook(id int64) error {
	return c.do(http.MethodDelete, "/api/v1/webhooks/"+strconv.FormatInt(id, 10), nil, nil)
}

func (c *Client) ListDeliveries(webhookID int64) ([]webhooks.Delivery, error) {
	var out []webhooks.Delivery
	if err := c.do(http.MethodGet, "/api/v1/webhooks/"+strconv.FormatInt(webhookID, 10)+"/deliveries", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Teams and API keys

func (c *Client) ListTeams() ([]teams.Team, error) {
	var out []teams.Team
	if err := c.do(http.MethodGet, "/api/v1/teams", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateTeam(name string) (teams.Team, error) {
	var out teams.Team
	if err := c.do(http.MethodPost, "/api/v1/teams", map[string]string{"name": name}, &out); err != nil {
		return teams.Team{}, err
	}
	return out, nil
}

func (c *Client) ListKeys(teamID int64) ([]teams.APIKey, error) {
	var out []teams.APIKey
	if err := c.do(http.MethodGet, "/api/v1/teams/"+strconv.FormatInt(teamID, 10)+"/keys", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateKey(teamID int64, name string, admin bool) (teams.APIKey, error) {
	var out teams.APIKey
	if err := c.do(http.MethodPost, "/api/v1/teams/"+strconv.FormatInt(teamID, 10)+"/keys", map[string]any{"name": name, "admin": admin}, &out); err != nil {
		return teams.APIKey{}, err
	}
	return out, nil
}

func (c *Client) DeleteKey(teamID, keyID int64) error {
	return c.do(http.MethodDelete, "/api/v1/teams/"+strconv.FormatInt(teamID, 10)+"/keys/"+strconv.FormatInt(keyID, 10), nil, nil)
}

// CSV import/export

type ImportSummary struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func (c *Client) ExportCSV(w io.Writer) error {
	return c.doRaw(http.MethodGet, "/api/v1/links/export", nil, "", w)
}

func (c *Client) ImportCSV(path string) (ImportSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return ImportSummary{}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return ImportSummary{}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return ImportSummary{}, err
	}
	if err := writer.Close(); err != nil {
		return ImportSummary{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/links/import", &body)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ImportSummary{}, fmt.Errorf("request failed: %w", err)
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
		return ImportSummary{}, fmt.Errorf("shorty: %s", e.Error)
	}
	var out ImportSummary
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ImportSummary{}, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}
