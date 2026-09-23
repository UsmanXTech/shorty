package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/UsmanXTech/shorty/internal/teams"
)

type TeamAPI struct {
	store teams.Store
}

func NewTeamAPI(store teams.Store) *TeamAPI {
	return &TeamAPI{store: store}
}

func (a *TeamAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/teams", a.list)
	mux.HandleFunc("POST /api/v1/teams", a.create)
	mux.HandleFunc("GET /api/v1/teams/{id}/keys", a.listKeys)
	mux.HandleFunc("POST /api/v1/teams/{id}/keys", a.createKey)
	mux.HandleFunc("DELETE /api/v1/teams/{id}/keys/{key_id}", a.deleteKey)
}

func (a *TeamAPI) list(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	list, err := a.store.ListTeams()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list teams")
		return
	}
	if list == nil {
		list = []teams.Team{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *TeamAPI) create(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	team, err := a.store.CreateTeam(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

func (a *TeamAPI) listKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := a.teamID(w, r)
	if !ok {
		return
	}
	if !requireTeamAccess(w, r, id) {
		return
	}
	list, err := a.store.ListKeys(id)
	if errors.Is(err, teams.ErrNotFound) {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list api keys")
		return
	}
	if list == nil {
		list = []teams.APIKey{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (a *TeamAPI) createKey(w http.ResponseWriter, r *http.Request) {
	id, ok := a.teamID(w, r)
	if !ok {
		return
	}
	if !requireTeamAccess(w, r, id) {
		return
	}
	var req struct {
		Name  string `json:"name"`
		Admin bool   `json:"admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Only admins may mint admin keys.
	if req.Admin && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "admin api key required to mint admin keys")
		return
	}
	key, err := a.store.CreateKey(id, req.Name, req.Admin)
	if errors.Is(err, teams.ErrNotFound) {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (a *TeamAPI) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, ok := a.teamID(w, r)
	if !ok {
		return
	}
	if !requireTeamAccess(w, r, id) {
		return
	}
	keyID, err := strconv.ParseInt(r.PathValue("key_id"), 10, 64)
	if err != nil || keyID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	// A key can only be deleted through its own team.
	k, err := a.store.GetKey(keyID)
	if err != nil || k.TeamID != id {
		writeError(w, http.StatusNotFound, "api key not found")
		return
	}
	// Only admins may delete an admin key.
	if k.IsAdmin && !isAdmin(r) {
		writeError(w, http.StatusForbidden, "admin api key required")
		return
	}
	if err := a.store.DeleteKey(keyID); errors.Is(err, teams.ErrNoKey) {
		writeError(w, http.StatusNotFound, "api key not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete api key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *TeamAPI) teamID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid team id")
		return 0, false
	}
	return id, true
}

// teamFromRequest returns the team resolved by the auth middleware, if any.
func teamFromRequest(r *http.Request) (teams.Team, bool) {
	return teams.TeamFrom(r.Context())
}

// isAdmin reports whether the request authenticated with an admin API key.
func isAdmin(r *http.Request) bool {
	k, ok := teams.KeyFrom(r.Context())
	return ok && k.IsAdmin
}

// requireAdmin rejects the request unless it carries an admin API key.
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "admin api key required")
		return false
	}
	return true
}

// requireAPIKey rejects anonymous requests. Some mutations (e.g. domain
// management) must never be reachable without an authenticated key, even
// though anonymous callers are otherwise mapped to the Default team.
func requireAPIKey(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := teams.KeyFrom(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "api key required")
		return false
	}
	return true
}

// requireTeamAccess allows admins everywhere, and regular keys only for their own team.
// Anonymous requests (no API key) are always rejected here: key management
// must never be reachable without authentication.
func requireTeamAccess(w http.ResponseWriter, r *http.Request, teamID int64) bool {
	if isAdmin(r) {
		return true
	}
	key, ok := teams.KeyFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "api key required")
		return false
	}
	if key.TeamID == teamID {
		return true
	}
	writeError(w, http.StatusForbidden, "forbidden")
	return false
}
