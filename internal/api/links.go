package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/links"
)

type LinkAPI struct {
	repo links.Repository
	cache *cache.Cache
}

func NewLinkAPI(repo links.Repository) *LinkAPI {
	return &LinkAPI{repo: repo}
}

func NewLinkAPIWithCache(repo links.Repository, c *cache.Cache) *LinkAPI {
	return &LinkAPI{repo: repo, cache: c}
}

type linkRequest struct {
	URL       string     `json:"url"`
	Slug      string     `json:"slug,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	MaxClicks *int64     `json:"max_clicks,omitempty"`
}

func (a *LinkAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/links", a.create)
	mux.HandleFunc("GET /api/v1/links", a.list)
	mux.HandleFunc("GET /api/v1/links/{id}", a.get)
	mux.HandleFunc("PUT /api/v1/links/{id}", a.update)
	mux.HandleFunc("DELETE /api/v1/links/{id}", a.delete)
}

func (a *LinkAPI) create(w http.ResponseWriter, r *http.Request) {
	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL")
		return
	}
	if req.MaxClicks != nil && *req.MaxClicks <= 0 {
		writeError(w, http.StatusBadRequest, "max_clicks must be greater than zero")
		return
	}
	if req.Slug == "" {
		req.Slug = generateSlug()
	}
	if !validSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	now := time.Now().UTC()
	link, err := a.repo.Create(links.Link{
		Slug: req.Slug, URL: req.URL, CreatedAt: now, UpdatedAt: now, ExpiresAt: req.ExpiresAt, MaxClicks: req.MaxClicks,
	})
	if err != nil {
		if errors.Is(err, links.ErrConflict) {
			writeError(w, http.StatusConflict, "slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	if a.cache != nil {
		a.cache.Delete(link.Slug)
	}
	writeJSON(w, http.StatusCreated, link)
}

func (a *LinkAPI) list(w http.ResponseWriter, r *http.Request) {
	items, err := a.repo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}

	q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	activity := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("activity")))
	if status != "" && status != "active" && status != "expired" && status != "limit-reached" {
		writeError(w, http.StatusBadRequest, "status must be active, expired, or limit-reached")
		return
	}
	if activity != "" && activity != "clicked" && activity != "unvisited" {
		writeError(w, http.StatusBadRequest, "activity must be clicked or unvisited")
		return
	}

	now := time.Now().UTC()
	filtered := make([]links.Link, 0, len(items))
	for _, link := range items {
		if q != "" && !strings.Contains(strings.ToLower(link.Slug), q) && !strings.Contains(strings.ToLower(link.URL), q) {
			continue
		}
		expired := link.ExpiresAt != nil && !now.Before(link.ExpiresAt.UTC())
		limited := link.MaxClicks != nil && link.Clicks >= *link.MaxClicks
		switch status {
		case "active":
			if expired || limited {
				continue
			}
		case "expired":
			if !expired {
				continue
			}
		case "limit-reached":
			if !limited {
				continue
			}
		}
		switch activity {
		case "clicked":
			if link.Clicks == 0 {
				continue
			}
		case "unvisited":
			if link.Clicks != 0 {
				continue
			}
		}
		filtered = append(filtered, link)
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (a *LinkAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	link, err := a.repo.GetByID(id)
	if errors.Is(err, links.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get link")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (a *LinkAPI) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	current, err := a.repo.GetByID(id)
	if errors.Is(err, links.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get link")
		return
	}
	oldSlug := current.Slug

	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL != "" {
		if !validURL(req.URL) {
			writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL")
			return
		}
		current.URL = req.URL
	}
	if req.Slug != "" {
		if !validSlug(req.Slug) {
			writeError(w, http.StatusBadRequest, "invalid slug")
			return
		}
		current.Slug = req.Slug
	}
	current.ExpiresAt = req.ExpiresAt
	if req.MaxClicks != nil && *req.MaxClicks <= 0 {
		writeError(w, http.StatusBadRequest, "max_clicks must be greater than zero")
		return
	}
	current.MaxClicks = req.MaxClicks
	current.UpdatedAt = time.Now().UTC()

	updated, err := a.repo.Update(current)
	if errors.Is(err, links.ErrConflict) {
		writeError(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update link")
		return
	}
	if a.cache != nil {
		a.cache.Delete(oldSlug)
		a.cache.Delete(updated.Slug)
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *LinkAPI) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	link, err := a.repo.GetByID(id)
	if errors.Is(err, links.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get link")
		return
	}
	if err := a.repo.Delete(id); err != nil {
		if errors.Is(err, links.ErrNotFound) {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not delete link")
		return
	}
	if a.cache != nil {
		a.cache.Delete(link.Slug)
	}
	w.WriteHeader(http.StatusNoContent)
}

func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validSlug(slug string) bool {
	if len(slug) < 2 || len(slug) > 64 {
		return false
	}
	for _, c := range slug {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return false
		}
	}
	return !strings.HasPrefix(slug, "-") && !strings.HasPrefix(slug, "_")
}

func generateSlug() string {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("shorty"))
	}
	return hex.EncodeToString(buf)
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
