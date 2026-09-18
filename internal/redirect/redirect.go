package redirect

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/links"
)

type Handler struct {
	repo links.Repository
	cache *cache.Cache
}

func New(repo links.Repository, caches ...*cache.Cache) *Handler {
	var c *cache.Cache
	if len(caches) > 0 {
		c = caches[0]
	}
	return &Handler{repo: repo, cache: c}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	var link links.Link
	var err error
	if h.cache != nil {
		if cached, ok := h.cache.Get(slug); ok {
			link = cached
		} else {
			link, err = h.repo.GetBySlug(slug)
			if err == nil {
				h.cache.Set(link)
			}
		}
	} else {
		link, err = h.repo.GetBySlug(slug)
	}
	if errors.Is(err, links.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if link.ExpiresAt != nil && !time.Now().UTC().Before(*link.ExpiresAt) {
		if h.cache != nil {
			h.cache.Delete(slug)
		}
		http.Error(w, "link expired", http.StatusGone)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}
