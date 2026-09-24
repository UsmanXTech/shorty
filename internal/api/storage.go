package api

import (
	"database/sql"
	"net/http"

	"github.com/UsmanXTech/shorty/internal/storage"
)

// StorageAPI exposes database storage stats for the dashboard.
type StorageAPI struct {
	store *storage.Store
}

// NewStorageAPI builds the handler. dbPath may be empty (WAL size unknown).
func NewStorageAPI(db *sql.DB, dbPath string) *StorageAPI {
	return &StorageAPI{store: storage.NewStore(db, dbPath)}
}

func (a *StorageAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/storage", a.get)
}

func (a *StorageAPI) get(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not gather storage stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
