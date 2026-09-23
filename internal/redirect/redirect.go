package redirect

import (
	"errors"
	"html/template"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/geoip"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/password"
	"github.com/UsmanXTech/shorty/internal/variants"
	"github.com/UsmanXTech/shorty/internal/webhooks"
)

type Handler struct {
	repo         links.Repository
	cache        *cache.Cache
	analytics    *analytics.Recorder
	geo          *geoip.Database
	variants     variants.Store
	variantLists *variants.ListCache
	domains      domains.Store
	signer       *password.CookieSigner
	dispatcher   *webhooks.Dispatcher
	rng          *rand.Rand
	// unlockAttempts rate-limits password guesses per client IP: every
	// attempt costs a bcrypt comparison, so unbounded attempts are a
	// CPU-exhaustion vector as well as a brute-force one.
	unlockAttempts *attemptLimiter
	// expiredEmitted tracks the last link.expired emission per slug so a
	// hot loop hitting an expired link can't flood webhook deliveries.
	expiredEmitted map[string]time.Time
	expiredMu      sync.Mutex
}

// attemptLimiter is a fixed-window per-key rate limiter.
type attemptLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64
	window time.Duration
	max    int
}

func newAttemptLimiter(window time.Duration, max int) *attemptLimiter {
	return &attemptLimiter{hits: make(map[string][]int64), window: window, max: max}
}

func (l *attemptLimiter) allow(key string) bool {
	now := time.Now().Unix()
	cutoff := now - int64(l.window/time.Second)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 {
		// Bound memory under a distributed flood: drop old state.
		l.hits = make(map[string][]int64)
	}
	kept := l.hits[key][:0]
	for _, ts := range l.hits[key] {
		if ts > cutoff {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

func newHandler() *Handler {
	return &Handler{
		rng:            rand.New(&lockedSource{src: rand.NewSource(time.Now().UnixNano())}),
		unlockAttempts: newAttemptLimiter(time.Minute, 10),
		expiredEmitted: make(map[string]time.Time),
	}
}

func New(repo links.Repository, caches ...*cache.Cache) *Handler {
	var c *cache.Cache
	if len(caches) > 0 {
		c = caches[0]
	}
	h := newHandler()
	h.repo = repo
	h.cache = c
	return h
}

func NewWithAnalytics(repo links.Repository, c *cache.Cache, recorder *analytics.Recorder) *Handler {
	h := newHandler()
	h.repo = repo
	h.cache = c
	h.analytics = recorder
	return h
}

func NewWithAnalyticsAndGeoIP(repo links.Repository, c *cache.Cache, recorder *analytics.Recorder, geo *geoip.Database) *Handler {
	h := newHandler()
	h.repo = repo
	h.cache = c
	h.analytics = recorder
	h.geo = geo
	return h
}

// lockedSource wraps a rand.Source with a mutex: *rand.Rand is not safe for
// concurrent use, and the redirect handler serves requests concurrently.
type lockedSource struct {
	mu  sync.Mutex
	src rand.Source
}

func (s *lockedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.src.Int63()
}

func (s *lockedSource) Seed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.src.Seed(seed)
}

// WithVariants attaches the A/B variant store.
func (h *Handler) WithVariants(s variants.Store) *Handler {
	h.variants = s
	return h
}

// WithVariantListCache shares the variant-list cache with the variant API so
// the redirect hot path avoids a database query per click. The API
// invalidates entries on mutation, so cached lists stay correct.
func (h *Handler) WithVariantListCache(c *variants.ListCache) *Handler {
	h.variantLists = c
	return h
}

// WithDomains attaches the custom domain store for host-based routing.
func (h *Handler) WithDomains(s domains.Store) *Handler {
	h.domains = s
	return h
}

// WithPasswordSigner attaches the signer used for password-unlock cookies.
func (h *Handler) WithPasswordSigner(s *password.CookieSigner) *Handler {
	h.signer = s
	return h
}

// WithWebhooks attaches the dispatcher used to emit link.clicked events.
func (h *Handler) WithWebhooks(d *webhooks.Dispatcher) *Handler {
	h.dispatcher = d
	return h
}

func clientAddr(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	return addr, err == nil
}

var passwordForm = template.Must(template.New("password").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected link</title>
<style>body{font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#0f172a;color:#e2e8f0}
.card{background:#1e293b;padding:2rem;border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.4);width:min(90vw,360px)}
h1{font-size:1.25rem;margin:0 0 .5rem}.muted{color:#94a3b8;font-size:.9rem;margin:0 0 1rem}
input{width:100%;box-sizing:border-box;padding:.6rem;border-radius:8px;border:1px solid #334155;background:#0f172a;color:#e2e8f0;margin-bottom:.75rem}
button{width:100%;padding:.6rem;border:0;border-radius:8px;background:#6366f1;color:#fff;font-weight:600;cursor:pointer}
.err{color:#f87171;font-size:.85rem;margin:0 0 .75rem;min-height:1.2em}</style></head>
<body><div class="card"><h1>&#128274; Protected link</h1>
<p class="muted">This link is password protected.</p>
{{if .Error}}<p class="err">Wrong password, try again.</p>{{else}}<p class="err"></p>{{end}}
<form method="post"><input type="password" name="password" placeholder="Password" autofocus autocomplete="off"><button type="submit">Unlock</button></form>
</div></body></html>`))

func unlockCookieName(slug string) string {
	return "shorty_unlock_" + slug
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")
	if slug == "" || strings.Contains(slug, "/") {
		http.NotFound(w, r)
		return
	}

	link, err := h.resolveLink(r, slug)
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
			h.cache.Delete(cacheKey(link))
		}
		if h.shouldEmitExpired(link.Slug) {
			h.emit(link.TeamID, webhooks.EventLinkExpired, map[string]any{"id": link.ID, "slug": link.Slug})
		}
		http.Error(w, "link expired", http.StatusGone)
		return
	}

	// Password gate: verify before counting the click.
	if link.Protected() {
		if !h.unlocked(r, link.Slug) {
			if r.Method == http.MethodPost {
				if !h.unlockAttempts.allow(clientIP(r)) {
					http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
					return
				}
				if err := r.ParseForm(); err == nil && password.Verify(link.PasswordHash, r.Form.Get("password")) {
					h.setUnlocked(w, r, link.Slug)
				} else {
					w.WriteHeader(http.StatusUnauthorized)
					_ = passwordForm.Execute(w, map[string]bool{"Error": true})
					return
				}
			} else {
				_ = passwordForm.Execute(w, map[string]bool{"Error": false})
				return
			}
		}
	}

	// A/B variants: pick a weighted destination. The list cache (shared
	// with the variant API, invalidated on mutation) means links without
	// variants cost no database query on the hot path.
	destination := link.URL
	var picked *variants.Variant
	if h.variants != nil {
		list, ok := h.variantLists.Get(link.ID)
		if !ok {
			if l, err := h.variants.List(link.ID); err == nil {
				h.variantLists.Set(link.ID, l)
				list, ok = l, true
			}
		}
		if ok && len(list) > 0 {
			picked = variants.Pick(list, h.rng.Int63())
		}
	}
	if picked != nil {
		destination = picked.URL
	}

	updated, err := h.repo.IncrementClicks(link.Slug)
	if errors.Is(err, links.ErrClickLimit) {
		if h.cache != nil {
			h.cache.Delete(cacheKey(link))
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
	if picked != nil {
		_ = h.variants.IncrementClicks(picked.ID)
	}
	if h.analytics != nil {
		event := analytics.Event{
			Slug:      link.Slug,
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
	payload := map[string]any{"id": link.ID, "slug": link.Slug, "url": link.URL, "destination": destination}
	if picked != nil {
		payload["variant_id"] = picked.ID
	}
	h.emit(link.TeamID, webhooks.EventLinkClicked, payload)
	http.Redirect(w, r, destination, http.StatusFound)
}

// resolveLink finds the link for slug, honoring custom-domain host routing.
// The cache is only used for default-domain links; domain routing always hits the store.
func (h *Handler) resolveLink(r *http.Request, slug string) (links.Link, error) {
	if h.domains != nil {
		if d, err := h.domains.GetByHost(r.Host); err == nil {
			domainID := d.ID
			return h.repo.GetBySlugAndDomain(slug, &domainID)
		}
	}

	var link links.Link
	var err error
	if h.cache != nil {
		if cached, ok := h.cache.Get(slug); ok {
			link = cached
		} else {
			link, err = h.repo.GetBySlugAndDomain(slug, nil)
			if err == nil {
				h.cache.Set(link)
			}
		}
	} else {
		link, err = h.repo.GetBySlugAndDomain(slug, nil)
	}
	return link, err
}

func cacheKey(link links.Link) string {
	return link.Slug
}

func (h *Handler) unlocked(r *http.Request, slug string) bool {
	if h.signer == nil {
		return false
	}
	cookie, err := r.Cookie(unlockCookieName(slug))
	if err != nil {
		return false
	}
	return h.signer.Valid(slug, cookie.Value)
}

func (h *Handler) setUnlocked(w http.ResponseWriter, r *http.Request, slug string) {
	if h.signer == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     unlockCookieName(slug),
		Value:    h.signer.Sign(slug, time.Now().UTC().Add(24*time.Hour)),
		Path:     "/" + slug,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// shouldEmitExpired rate-limits link.expired webhook emissions per slug so
// repeated hits on an expired link can't flood webhook deliveries.
func (h *Handler) shouldEmitExpired(slug string) bool {
	h.expiredMu.Lock()
	defer h.expiredMu.Unlock()
	if last, ok := h.expiredEmitted[slug]; ok && time.Since(last) < 10*time.Minute {
		return false
	}
	if len(h.expiredEmitted) > 10000 {
		h.expiredEmitted = make(map[string]time.Time)
	}
	h.expiredEmitted[slug] = time.Now()
	return true
}

// clientIP returns the client address used for rate limiting.
func clientIP(r *http.Request) string {
	if addr, ok := clientAddr(r); ok {
		return addr.String()
	}
	return r.RemoteAddr
}

func (h *Handler) emit(teamID int64, event string, payload map[string]any) {
	if h.dispatcher != nil {
		h.dispatcher.Emit(teamID, event, payload)
	}
}
