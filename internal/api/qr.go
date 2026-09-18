package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/qr"
)

type QRAPI struct {
	repo links.Repository
	cfg  config.Config
}

func NewQRAPI(repo links.Repository, cfg config.Config) *QRAPI {
	return &QRAPI{repo: repo, cfg: cfg}
}

func (a *QRAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/links/{id}/qr", a.generate)
}

func (a *QRAPI) generate(w http.ResponseWriter, r *http.Request) {
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

	size := qr.DefaultSize
	if raw := strings.TrimSpace(r.URL.Query().Get("size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "size must be an integer")
			return
		}
		size = parsed
	}

	shortURL, err := a.shortURL(r, link.Slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not determine public URL")
		return
	}
	data, err := qr.Generate(shortURL, size)
	if errors.Is(err, qr.ErrInvalidSize) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate QR code")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *QRAPI) shortURL(r *http.Request, slug string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(a.cfg.BaseURL), "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		}
		if strings.TrimSpace(r.Host) == "" {
			return "", errors.New("request host is empty")
		}
		base = scheme + "://" + r.Host
	}

	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("invalid public URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + slug
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
