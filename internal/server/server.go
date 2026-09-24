package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/api"
	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/dashboard"
	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/geoip"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/password"
	"github.com/UsmanXTech/shorty/internal/redirect"
	"github.com/UsmanXTech/shorty/internal/teams"
	"github.com/UsmanXTech/shorty/internal/variants"
	"github.com/UsmanXTech/shorty/internal/webhooks"
)

type Server struct {
	cfg        config.Config
	repo       links.Repository
	linkCache  *cache.Cache
	analytics  *analytics.Recorder
	geo        *geoip.Database
	db         *sql.DB
	dispatcher *webhooks.Dispatcher
}

func New(cfg config.Config) *Server {
	return NewWithRepository(cfg, links.NewMemoryRepository())
}

func NewWithRepository(cfg config.Config, repo links.Repository) *Server {
	return &Server{cfg: cfg, repo: repo, linkCache: cache.New()}
}

func NewWithRepositoryAndAnalytics(cfg config.Config, repo links.Repository, recorder *analytics.Recorder) *Server {
	return &Server{cfg: cfg, repo: repo, linkCache: cache.New(), analytics: recorder}
}

func NewWithRepositoryAndAnalyticsAndGeoIP(cfg config.Config, repo links.Repository, recorder *analytics.Recorder, geo *geoip.Database) *Server {
	return &Server{cfg: cfg, repo: repo, linkCache: cache.New(), analytics: recorder, geo: geo}
}

func NewWithRepositoryAndAnalyticsAndGeoIPAndDB(cfg config.Config, repo links.Repository, recorder *analytics.Recorder, geo *geoip.Database, db *sql.DB) *Server {
	s := &Server{cfg: cfg, repo: repo, linkCache: cache.New(), analytics: recorder, geo: geo, db: db}
	if db != nil {
		s.dispatcher = webhooks.NewDispatcher(webhooks.NewSQLiteStore(db), 2)
	}
	return s
}

// Close releases server-owned background resources.
func (s *Server) Close() {
	if s.dispatcher != nil {
		s.dispatcher.Close()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", dashboard.Handler())
	mux.HandleFunc("GET /health", s.health)

	linkAPI := api.NewLinkAPIWithCache(s.repo, s.linkCache)
	domainAPI := api.NewDomainAPI(domains.NewMemoryStore())
	webhookAPI := api.NewWebhookAPI(noopWebhookStore{})
	teamAPI := api.NewTeamAPI(noopTeamStore{})
	csvAPI := api.NewCSVAPI(s.repo)

	// One shared variant store and list cache for the API and the redirect
	// handler: the API invalidates the cache on mutation, so the hot path
	// can serve variant lists (including "no variants") without a query.
	var variantStore variants.Store = variants.NewMemoryStore()
	variantListCache := variants.NewListCache()
	variantAPI := api.NewVariantAPI(s.repo, variantStore).WithListCache(variantListCache)
	if s.db != nil {
		variantStore = variants.NewSQLiteStore(s.db)
		domainStore := domains.NewSQLiteStore(s.db)
		webhookStore := webhooks.NewSQLiteStore(s.db)
		teamStore := teams.NewSQLiteStore(s.db)

		linkAPI.WithWebhooks(s.dispatcher).WithDomains(domainStore)
		variantAPI = api.NewVariantAPI(s.repo, variantStore).WithListCache(variantListCache)
		domainAPI = api.NewDomainAPI(domainStore)
		webhookAPI = api.NewWebhookAPI(webhookStore)
		teamAPI = api.NewTeamAPI(teamStore)
	}

	authed := func(h http.Handler) http.Handler { return h }
	if s.db != nil {
		authed = teams.Middleware(teams.NewSQLiteStore(s.db))
	}

	linkAPI.Routes(mux)
	variantAPI.Routes(mux)
	domainAPI.Routes(mux)
	webhookAPI.Routes(mux)
	teamAPI.Routes(mux)
	csvAPI.Routes(mux)
	api.NewQRAPI(s.repo, s.cfg).Routes(mux)
	api.NewUTMAPI().Routes(mux)
	if s.db != nil {
		api.NewAnalyticsAPI(s.repo, analytics.NewQueryStore(s.db)).Routes(mux)
		api.NewStorageAPI(s.db, s.cfg.Database).Routes(mux)
	}

	redirectHandler := redirect.NewWithAnalyticsAndGeoIP(s.repo, s.linkCache, s.analytics, s.geo).
		WithPasswordSigner(password.NewCookieSigner(os.Getenv("SHORTY_SECRET"))).
		WithWebhooks(s.dispatcher).
		WithVariants(variantStore).
		WithVariantListCache(variantListCache)
	if s.db != nil {
		redirectHandler.WithDomains(domains.NewSQLiteStore(s.db))
	}
	mux.Handle("GET /{slug}", redirectHandler)
	mux.Handle("POST /{slug}", redirectHandler)

	// Team auth applies to the API surface; dashboard, health and redirects stay public.
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/", authed(mux))
	apiMux.Handle("/", mux)
	return maxBodyLimit(apiMux)
}

// maxBodyLimit caps request bodies to blunt memory-exhaustion DoS via huge
// payloads. The JSON API gets 1 MiB; CSV import gets a larger budget.
func maxBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(1 << 20)
		if r.URL.Path == "/api/v1/links/import" {
			limit = 32 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// No-op stores keep the API handlers constructible when no database is configured.

type noopWebhookStore struct{}

func (noopWebhookStore) Create(int64, string, []string) (webhooks.Webhook, error) {
	return webhooks.Webhook{}, webhooks.ErrNotFound
}
func (noopWebhookStore) List(int64) ([]webhooks.Webhook, error) { return nil, nil }
func (noopWebhookStore) Get(int64) (webhooks.Webhook, error) {
	return webhooks.Webhook{}, webhooks.ErrNotFound
}
func (noopWebhookStore) Update(int64, string, []string, bool) (webhooks.Webhook, error) {
	return webhooks.Webhook{}, webhooks.ErrNotFound
}
func (noopWebhookStore) RotateSecret(int64) (webhooks.Webhook, error) {
	return webhooks.Webhook{}, webhooks.ErrNotFound
}
func (noopWebhookStore) Delete(int64) error { return webhooks.ErrNotFound }
func (noopWebhookStore) ListActive(int64, string) ([]webhooks.Webhook, error) {
	return nil, nil
}
func (noopWebhookStore) RecordDelivery(webhooks.Delivery, string) error { return nil }
func (noopWebhookStore) ListDeliveries(int64, int) ([]webhooks.Delivery, error) {
	return nil, nil
}

type noopTeamStore struct{}

func (noopTeamStore) CreateTeam(string) (teams.Team, error) { return teams.Team{}, teams.ErrNotFound }
func (noopTeamStore) ListTeams() ([]teams.Team, error)      { return nil, nil }
func (noopTeamStore) GetTeam(int64) (teams.Team, error)     { return teams.Team{}, teams.ErrNotFound }
func (noopTeamStore) CreateKey(int64, string, bool) (teams.APIKey, error) {
	return teams.APIKey{}, teams.ErrNoKey
}
func (noopTeamStore) GetKey(int64) (teams.APIKey, error) { return teams.APIKey{}, teams.ErrNoKey }
func (noopTeamStore) KeyForKey(string) (teams.APIKey, error) {
	return teams.APIKey{}, teams.ErrNoKey
}
func (noopTeamStore) ListKeys(int64) ([]teams.APIKey, error) { return nil, nil }
func (noopTeamStore) DeleteKey(int64) error                  { return teams.ErrNoKey }
func (noopTeamStore) TeamForKey(string) (teams.Team, error)  { return teams.Team{}, teams.ErrNoKey }
