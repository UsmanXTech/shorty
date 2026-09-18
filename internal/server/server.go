package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/api"
	"github.com/UsmanXTech/shorty/internal/cache"
	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/dashboard"
	"github.com/UsmanXTech/shorty/internal/geoip"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/redirect"
)

type Server struct {
	cfg       config.Config
	repo      links.Repository
	linkCache *cache.Cache
	analytics *analytics.Recorder
	geo       *geoip.Database
	db        *sql.DB
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
	return &Server{cfg: cfg, repo: repo, linkCache: cache.New(), analytics: recorder, geo: geo, db: db}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", dashboard.Handler())
	mux.HandleFunc("GET /health", s.health)
	api.NewLinkAPIWithCache(s.repo, s.linkCache).Routes(mux)
	api.NewQRAPI(s.repo, s.cfg).Routes(mux)
	if s.db != nil {
		api.NewAnalyticsAPI(s.repo, analytics.NewQueryStore(s.db)).Routes(mux)
	}
	mux.Handle("GET /{slug}", redirect.NewWithAnalyticsAndGeoIP(s.repo, s.linkCache, s.analytics, s.geo))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
