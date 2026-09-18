package redirect

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/geoip"
	"github.com/UsmanXTech/shorty/internal/links"
)

type Handler struct {
	repo      links.Repository
	cache     *cache.Cache
	analytics *analytics.Recorder
	geo       *geoip.Database
}

func New(repo links.Repository, caches ...*cache.Cache) *Handler {
	var c *cache.Cache
	if len(caches) > 0 {
		c = caches[0]
	}
	return &Handler{repo: repo, cache: c}
}

func NewWithAnalytics(repo links.Repository, c *cache.Cache, recorder *analytics.Recorder) *Handler {
	return &Handler{repo: repo, cache: c, analytics: recorder}
}

func NewWithAnalyticsAndGeoIP(repo links.Repository, c *cache.Cache, recorder *analytics.Recorder, geo *geoip.Database) *Handler {
	return &Handler{repo: repo, cache: c, analytics: recorder, geo: geo}
}

func clientAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	return addr, err == nil
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

	updated, err := h.repo.IncrementClicks(slug)
	if errors.Is(err, links.ErrClickLimit) {
		if h.cache != nil {
			h.cache.Delete(slug)
		}
		http.Error(w, "link click limit reached", http.StatusGone)
		return
	}
	if errors.Is(err, links.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if h.cache != nil {
		h.cache.Set(updated)
	}
	if h.analytics != nil {
		event := analytics.Event{
			Slug:      slug,
			CreatedAt: time.Now().UTC(),
			Referrer:  r.Referer(),
			UserAgent: r.UserAgent(),
		}
		if h.geo != nil {
			if addr, ok := clientAddr(r); ok {
				event.Country, _ = h.geo.Lookup(addr)
			}
		}
		h.analytics.Record(event)
	}
	http.Redirect(w, r, updated.URL, http.StatusFound)
}
