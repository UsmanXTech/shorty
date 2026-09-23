package api

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/teams"
)

type CSVAPI struct {
	repo links.Repository
}

func NewCSVAPI(repo links.Repository) *CSVAPI {
	return &CSVAPI{repo: repo}
}

func (a *CSVAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/links/export", a.export)
	mux.HandleFunc("POST /api/v1/links/import", a.importCSV)
}

var csvHeader = []string{"slug", "url", "expires_at", "max_clicks", "clicks", "created_at"}

func (a *CSVAPI) export(w http.ResponseWriter, r *http.Request) {
	var items []links.Link
	var err error
	if team, ok := teamFromRequest(r); ok {
		items, err = a.repo.ListByTeam(team.ID)
	} else {
		items, err = a.repo.List()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="shorty-links.csv"`)
	writer := csv.NewWriter(w)
	if err := writer.Write(csvHeader); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write CSV")
		return
	}
	for _, link := range items {
		record := []string{link.Slug, link.URL, formatCSVTime(link.ExpiresAt), formatCSVInt(link.MaxClicks),
			strconv.FormatInt(link.Clicks, 10), link.CreatedAt.UTC().Format(time.RFC3339)}
		if err := writer.Write(record); err != nil {
			writeError(w, http.StatusInternalServerError, "could not write CSV")
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write CSV")
		return
	}
}

type importSummary struct {
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func (a *CSVAPI) importCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "expected a multipart form upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	teamID := int64(teams.DefaultTeamID)
	if team, ok := teamFromRequest(r); ok {
		teamID = team.ID
	}

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read CSV header")
		return
	}
	cols := map[string]int{}
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	urlIdx, ok := cols["url"]
	if !ok {
		writeError(w, http.StatusBadRequest, "CSV must contain a url column")
		return
	}
	slugIdx, hasSlug := cols["slug"]
	expiresIdx, hasExpires := cols["expires_at"]
	maxClicksIdx, hasMaxClicks := cols["max_clicks"]

	summary := importSummary{}
	now := time.Now().UTC()
	// Cap rows per request: a 32 MiB CSV of tiny rows could otherwise mean
	// hundreds of thousands of synchronous DB writes in one request.
	const maxImportRows = 50000
	for line := 2; ; line++ {
		if line-2 >= maxImportRows {
			summary.Errors = append(summary.Errors, fmt.Sprintf("row limit of %d exceeded; import stopped", maxImportRows))
			break
		}
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: %v", line, err))
			continue
		}
		rawURL := valueAt(record, urlIdx)
		if !validURL(rawURL) {
			summary.Skipped++
			summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: invalid url", line))
			continue
		}
		slug := ""
		if hasSlug {
			slug = strings.TrimSpace(valueAt(record, slugIdx))
		}
		if slug == "" {
			slug = generateSlug()
		}
		if !validSlug(slug) {
			summary.Skipped++
			summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: invalid slug %q", line, slug))
			continue
		}
		var expiresAt *time.Time
		if hasExpires {
			if raw := strings.TrimSpace(valueAt(record, expiresIdx)); raw != "" {
				if t, err := time.Parse(time.RFC3339, raw); err == nil {
					expiresAt = &t
				} else if t, err := time.Parse("2006-01-02", raw); err == nil {
					expiresAt = &t
				} else {
					summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: invalid expires_at %q, ignored", line, raw))
				}
			}
		}
		var maxClicks *int64
		if hasMaxClicks {
			if raw := strings.TrimSpace(valueAt(record, maxClicksIdx)); raw != "" {
				if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
					maxClicks = &n
				} else {
					summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: invalid max_clicks %q, ignored", line, raw))
				}
			}
		}
		_, err = a.repo.Create(links.Link{
			Slug: slug, URL: rawURL, CreatedAt: now, UpdatedAt: now,
			ExpiresAt: expiresAt, MaxClicks: maxClicks, TeamID: teamID,
		})
		if err != nil {
			summary.Skipped++
			summary.Errors = append(summary.Errors, fmt.Sprintf("line %d: %v", line, err))
			continue
		}
		summary.Created++
	}
	writeJSON(w, http.StatusOK, summary)
}

func valueAt(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return record[idx]
}

func formatCSVTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatCSVInt(n *int64) string {
	if n == nil {
		return ""
	}
	return strconv.FormatInt(*n, 10)
}
