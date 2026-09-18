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

	"github.com/UsmanXTech/shorty/internal/links"
)

type LinkAPI struct {
	repo links.Repository
}

func NewLinkAPI(repo links.Repository) *LinkAPI {
	return &LinkAPI{repo: repo}
}

type linkRequest struct {
	URL       string     `json:"url"`
	Slug      string     `json:"slug,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
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
	if req.Slug == "" {
		req.Slug = generateSlug()
	}
	if !validSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	now := time.Now().UTC()
	link, err := a.repo.Create(links.Link{
		Slug: req.Slug, URL: req.URL, CreatedAt: now, UpdatedAt: now, ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		if errors.Is(err, links.ErrConflict) {
			writeError(w, http.StatusConflict, "slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (a *LinkAPI) list(w http.ResponseWriter, _ *http.Request) {
	items, err := a.repo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}
	writeJSON(w, http.StatusOK, items)
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
	writeJSON(w, http.StatusOK, updated)
}

func (a *LinkAPI) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	err := a.repo.Delete(id)
	if errors.Is(err, links.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete link")
		return
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
