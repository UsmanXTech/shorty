package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/UsmanXTech/shorty/internal/webhooks"
)

type WebhookAPI struct {
	store webhooks.Store
	// validateURL is injectable for tests; defaults to webhooks.ValidateURL.
	validateURL func(string) error
}

func NewWebhookAPI(store webhooks.Store) *WebhookAPI {
	return &WebhookAPI{store: store, validateURL: webhooks.ValidateURL}
}

func (a *WebhookAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/webhooks", a.list)
	mux.HandleFunc("POST /api/v1/webhooks", a.create)
	mux.HandleFunc("GET /api/v1/webhooks/{id}", a.get)
	mux.HandleFunc("PUT /api/v1/webhooks/{id}", a.update)
	mux.HandleFunc("POST /api/v1/webhooks/{id}/rotate-secret", a.rotateSecret)
	mux.HandleFunc("DELETE /api/v1/webhooks/{id}", a.delete)
	mux.HandleFunc("GET /api/v1/webhooks/{id}/deliveries", a.deliveries)
}

type webhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Active *bool    `json:"active,omitempty"`
}

func (a *WebhookAPI) list(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.List(requestTeam(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list webhooks")
		return
	}
	if list == nil {
		list = []webhooks.Webhook{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *WebhookAPI) create(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := a.validateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Events) == 0 {
		req.Events = webhooks.AllEvents
	}
	hook, err := a.store.Create(requestTeam(r), req.URL, req.Events)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The signing secret is shown exactly once, at creation.
	hook.PlaintextSecret = hook.Secret
	writeJSON(w, http.StatusCreated, hook)
}

func (a *WebhookAPI) get(w http.ResponseWriter, r *http.Request) {
	hook, ok := a.findHook(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (a *WebhookAPI) update(w http.ResponseWriter, r *http.Request) {
	hook, ok := a.findHook(w, r)
	if !ok {
		return
	}
	var req webhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	url := hook.URL
	if req.URL != "" {
		if err := a.validateURL(req.URL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		url = req.URL
	}
	events := hook.Events
	if req.Events != nil {
		events = req.Events
	}
	active := hook.Active
	if req.Active != nil {
		active = *req.Active
	}
	updated, err := a.store.Update(hook.ID, url, events, active)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *WebhookAPI) delete(w http.ResponseWriter, r *http.Request) {
	hook, ok := a.findHook(w, r)
	if !ok {
		return
	}
	if err := a.store.Delete(hook.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete webhook")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rotateSecret issues a fresh signing secret. The new secret is returned
// exactly once in this response, mirroring creation.
func (a *WebhookAPI) rotateSecret(w http.ResponseWriter, r *http.Request) {
	hook, ok := a.findHook(w, r)
	if !ok {
		return
	}
	rotated, err := a.store.RotateSecret(hook.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not rotate secret")
		return
	}
	rotated.PlaintextSecret = rotated.Secret
	writeJSON(w, http.StatusOK, rotated)
}

func (a *WebhookAPI) deliveries(w http.ResponseWriter, r *http.Request) {
	hook, ok := a.findHook(w, r)
	if !ok {
		return
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}
	list, err := a.store.ListDeliveries(hook.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list deliveries")
		return
	}
	if list == nil {
		list = []webhooks.Delivery{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *WebhookAPI) findHook(w http.ResponseWriter, r *http.Request) (webhooks.Webhook, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return webhooks.Webhook{}, false
	}
	hook, err := a.store.Get(id)
	if errors.Is(err, webhooks.ErrNotFound) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return webhooks.Webhook{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not get webhook")
		return webhooks.Webhook{}, false
	}
	// Webhooks are team-scoped; admins may reach any team's webhooks.
	if !isAdmin(r) && hook.TeamID != requestTeam(r) {
		writeError(w, http.StatusNotFound, "webhook not found")
		return webhooks.Webhook{}, false
	}
	return hook, true
}
