package server

import (
	"encoding/json"
	"net/http"

	"github.com/UsmanXTech/shorty/internal/api"
	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/redirect"
)

type Server struct {
	cfg  config.Config
	repo links.Repository
}

func New(cfg config.Config) *Server {
	return NewWithRepository(cfg, links.NewMemoryRepository())
}

func NewWithRepository(cfg config.Config, repo links.Repository) *Server {
	return &Server{cfg: cfg, repo: repo}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	api.NewLinkAPI(s.repo).Routes(mux)
	mux.Handle("GET /{slug}", redirect.New(s.repo))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
