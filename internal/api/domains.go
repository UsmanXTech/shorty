package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/UsmanXTech/shorty/internal/domains"
)

type DomainAPI struct {
	store domains.Store
}

func NewDomainAPI(store domains.Store) *DomainAPI {
	return &DomainAPI{store: store}
}

func (a *DomainAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/domains", a.list)
	mux.HandleFunc("POST /api/v1/domains", a.create)
	mux.HandleFunc("DELETE /api/v1/domains/{id}", a.delete)
}

func (a *DomainAPI) list(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.List(requestTeam(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list domains")
		return
	}
	if list == nil {
		list = []domains.Domain{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *DomainAPI) create(w http.ResponseWriter, r *http.Request) {
	// Domain registration must not be anonymous: domains route live traffic
	// by Host header, so anyone could otherwise squat or disrupt routing.
	if !requireAPIKey(w, r) {
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if domains.Normalize(req.Domain) == "" {
		writeError(w, http.StatusBadRequest, "domain must not be empty")
		return
	}
	d, err := a.store.Create(requestTeam(r), req.Domain)
	if errors.Is(err, domains.ErrConflict) {
		writeError(w, http.StatusConflict, "domain already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create domain")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (a *DomainAPI) delete(w http.ResponseWriter, r *http.Request) {
	// Deleting a domain breaks live links routed by it; never anonymous.
	if !requireAPIKey(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := a.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}
	// Domains are team-scoped; admins may delete any team's domains.
	if !isAdmin(r) && d.TeamID != requestTeam(r) {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err := a.store.Delete(id); errors.Is(err, domains.ErrNotFound) {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete domain")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
