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
	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/password"
	"github.com/UsmanXTech/shorty/internal/teams"
	"github.com/UsmanXTech/shorty/internal/webhooks"
)

type LinkAPI struct {
	repo       links.Repository
	cache      *cache.Cache
	dispatcher *webhooks.Dispatcher
	domains    domains.Store
}

func NewLinkAPI(repo links.Repository) *LinkAPI {
	return &LinkAPI{repo: repo}
}

func NewLinkAPIWithCache(repo links.Repository, c *cache.Cache) *LinkAPI {
	return &LinkAPI{repo: repo, cache: c}
}

// WithWebhooks attaches a webhook dispatcher used to emit link.created events.
func (a *LinkAPI) WithWebhooks(d *webhooks.Dispatcher) *LinkAPI {
	a.dispatcher = d
	return a
}

// WithDomains attaches the domain store used to resolve custom domains on create.
func (a *LinkAPI) WithDomains(s domains.Store) *LinkAPI {
	a.domains = s
	return a
}

type linkRequest struct {
	URL       string  `json:"url"`
	Slug      string  `json:"slug,omitempty"`
	ExpiresAt optTime `json:"expires_at,omitempty"`
	MaxClicks optInt  `json:"max_clicks,omitempty"`
	Password  *string `json:"password,omitempty"`
	DomainID  optInt  `json:"domain_id,omitempty"`
	Domain    string  `json:"domain,omitempty"`
}

// optTime distinguishes a missing expires_at field from an explicit null:
// missing leaves the current value untouched, null clears it.
type optTime struct {
	Set   bool
	Value *time.Time
}

func (o *optTime) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	o.Value = &t
	return nil
}

// optInt distinguishes a missing max_clicks field from an explicit null:
// missing leaves the current value untouched, null clears it.
type optInt struct {
	Set   bool
	Value *int64
}

func (o *optInt) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	o.Value = &n
	return nil
}

func requestTeam(r *http.Request) int64 {
	if team, ok := teamFromRequest(r); ok {
		return team.ID
	}
	return teams.DefaultTeamID
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
	if req.MaxClicks.Set && req.MaxClicks.Value != nil && *req.MaxClicks.Value <= 0 {
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

	var passwordHash string
	if req.Password != nil && *req.Password != "" {
		hash, err := password.Hash(*req.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		passwordHash = hash
	}

	domainID, err := a.resolveDomain(r, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	link, err := a.repo.Create(links.Link{
		Slug: req.Slug, URL: req.URL, CreatedAt: now, UpdatedAt: now,
		ExpiresAt: req.ExpiresAt.Value, MaxClicks: req.MaxClicks.Value,
		PasswordHash: passwordHash, TeamID: requestTeam(r), DomainID: domainID,
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
	if a.dispatcher != nil {
		a.dispatcher.Emit(link.TeamID, webhooks.EventLinkCreated, map[string]any{
			"id": link.ID, "slug": link.Slug, "url": link.URL, "team_id": link.TeamID,
		})
	}
	writeJSON(w, http.StatusCreated, link)
}

func (a *LinkAPI) resolveDomain(r *http.Request, req linkRequest) (*int64, error) {
	// Explicit null clears the domain (Set with nil Value); the caller only
	// invokes this when the field is set or Domain is non-empty.
	if req.DomainID.Set && req.DomainID.Value == nil && req.Domain == "" {
		return nil, nil
	}
	if req.DomainID.Set && a.domains == nil {
		return nil, errors.New("custom domains are not configured")
	}
	teamID := requestTeam(r)
	admin := isAdmin(r)
	owned := func(d domains.Domain) (*int64, error) {
		if !admin && d.TeamID != teamID {
			return nil, errors.New("unknown domain")
		}
		return &d.ID, nil
	}
	if req.Domain != "" {
		if a.domains == nil {
			return nil, errors.New("custom domains are not configured")
		}
		d, err := a.domains.GetByHost(req.Domain)
		if err != nil {
			return nil, errors.New("unknown domain")
		}
		return owned(d)
	}
	if req.DomainID.Set {
		d, err := a.domains.Get(*req.DomainID.Value)
		if err != nil {
			return nil, errors.New("unknown domain")
		}
		return owned(d)
	}
	return nil, nil
}

func (a *LinkAPI) list(w http.ResponseWriter, r *http.Request) {
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

func (a *LinkAPI) scopedGet(r *http.Request) (links.Link, bool) {
	id, ok := pathID(r)
	if !ok {
		return links.Link{}, false
	}
	link, err := a.repo.GetByID(id)
	if err != nil {
		return links.Link{}, false
	}
	if team, ok := teamFromRequest(r); ok && link.TeamID != team.ID {
		return links.Link{}, false
	}
	return link, true
}

func (a *LinkAPI) get(w http.ResponseWriter, r *http.Request) {
	link, ok := a.scopedGet(r)
	if !ok {
		if _, idOK := pathID(r); !idOK {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (a *LinkAPI) update(w http.ResponseWriter, r *http.Request) {
	current, ok := a.scopedGet(r)
	if !ok {
		if _, idOK := pathID(r); !idOK {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		writeError(w, http.StatusNotFound, "link not found")
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
	// expires_at: omitted leaves the current value, explicit null clears it.
	if req.ExpiresAt.Set {
		current.ExpiresAt = req.ExpiresAt.Value
	}
	// max_clicks: omitted leaves the current value, explicit null clears it.
	if req.MaxClicks.Set {
		if req.MaxClicks.Value != nil && *req.MaxClicks.Value <= 0 {
			writeError(w, http.StatusBadRequest, "max_clicks must be greater than zero")
			return
		}
		current.MaxClicks = req.MaxClicks.Value
	}
	if req.Password != nil {
		if *req.Password == "" {
			current.PasswordHash = ""
		} else {
			hash, err := password.Hash(*req.Password)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			current.PasswordHash = hash
		}
	}
	// domain: omitted preserves; explicit null (or empty domain_id) clears.
	if req.Domain != "" || req.DomainID.Set {
		domainID, err := a.resolveDomain(r, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		current.DomainID = domainID
	}
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
	link, ok := a.scopedGet(r)
	if !ok {
		if _, idOK := pathID(r); !idOK {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err := a.repo.Delete(link.ID); err != nil {
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
