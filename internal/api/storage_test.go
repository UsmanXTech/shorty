package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/UsmanXTech/shorty/internal/database"
)

func TestStorageAPIGet(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`INSERT INTO links (slug, url, created_at, updated_at, clicks)
		VALUES ('docs', 'https://example.com', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 0)`); err != nil {
		t.Fatal(err)
	}

	api := NewStorageAPI(db.DB, "")
	mux := http.NewServeMux()
	api.Routes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/storage", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		DatabaseBytes int64 `json:"database_bytes"`
		Tables        []struct {
			Name string `json:"name"`
			Rows int64  `json:"rows"`
		} `json:"tables"`
		Suggestions []struct {
			Kind string `json:"kind"`
		} `json:"suggestions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DatabaseBytes <= 0 {
		t.Fatalf("expected positive database_bytes, got %d", body.DatabaseBytes)
	}
	found := false
	for _, tb := range body.Tables {
		if tb.Name == "links" && tb.Rows == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected links table with 1 row, got %+v", body.Tables)
	}
	if len(body.Suggestions) == 0 {
		t.Fatal("expected suggestions")
	}
}
