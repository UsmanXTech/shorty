package redirect

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/links"
)

type Handler struct {
	repo links.Repository
}

func New(repo links.Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	link, err := h.repo.GetBySlug(slug)
	if errors.Is(err, links.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if link.ExpiresAt != nil && !time.Now().UTC().Before(*link.ExpiresAt) {
		http.Error(w, "link expired", http.StatusGone)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}
