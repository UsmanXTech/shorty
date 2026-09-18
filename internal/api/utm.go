package api

import (
	"encoding/json"
	"net/http"

	"github.com/UsmanXTech/shorty/internal/utm"
)

type UTMAPI struct{}

func NewUTMAPI() *UTMAPI { return &UTMAPI{} }

type utmRequest struct {
	URL string `json:"url"`
	utm.Params
}

type utmResponse struct {
	URL string `json:"url"`
}

// Routes registers the UTM URL builder endpoint.
func (a *UTMAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/utm", a.build)
}

func (a *UTMAPI) build(w http.ResponseWriter, r *http.Request) {
	var req utmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	built, err := utm.Build(req.URL, req.Params)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, utmResponse{URL: built})
}
