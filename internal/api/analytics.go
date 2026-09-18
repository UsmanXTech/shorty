package api

import (
	"net/http"
	"time"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/links"
)

type AnalyticsAPI struct {
	repo  links.Repository
	store *analytics.QueryStore
}

func NewAnalyticsAPI(repo links.Repository, store *analytics.QueryStore) *AnalyticsAPI {
	return &AnalyticsAPI{repo: repo, store: store}
}

func (a *AnalyticsAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/links/{id}/analytics", a.get)
}

func (a *AnalyticsAPI) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	link, err := a.repo.GetByID(id)
	if err != nil {
		if err == links.ErrNotFound {
			writeError(w, http.StatusNotFound, "link not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not get link")
		return
	}

	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour)
	to := now.Add(time.Nanosecond)
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
	}
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "hour"
	}
	if interval != "hour" && interval != "day" {
		writeError(w, http.StatusBadRequest, "interval must be hour or day")
		return
	}

	result, err := a.store.Summary(link.Slug, link.Clicks, from, to, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not query analytics")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
