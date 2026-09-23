package teams

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/UsmanXTech/shorty/internal/database"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSQLiteStore(db.DB)
}

func TestDefaultTeamExists(t *testing.T) {
	s := testStore(t)
	team, err := s.GetTeam(DefaultTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if team.Name != "Default" {
		t.Fatalf("expected Default team, got %q", team.Name)
	}
}

func TestTeamAndKeyLifecycle(t *testing.T) {
	s := testStore(t)
	team, err := s.CreateTeam("Acme")
	if err != nil {
		t.Fatal(err)
	}
	if team.ID == DefaultTeamID {
		t.Fatal("expected a new team id")
	}

	key, err := s.CreateKey(team.ID, "ci", false)
	if err != nil {
		t.Fatal(err)
	}
	if key.Plaintext == "" {
		t.Fatal("expected plaintext key on creation")
	}
	if key.Prefix == "" || len(key.Prefix) >= len(key.Plaintext) {
		t.Fatalf("unexpected prefix %q", key.Prefix)
	}

	// Plaintext must not be returned on later reads.
	keys, err := s.ListKeys(team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Plaintext != "" {
		t.Fatalf("plaintext leaked: %+v", keys)
	}

	resolved, err := s.TeamForKey(key.Plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != team.ID {
		t.Fatalf("expected team %d, got %d", team.ID, resolved.ID)
	}

	// LastUsedAt should be set after use.
	keys, _ = s.ListKeys(team.ID)
	if keys[0].LastUsedAt == nil {
		t.Fatal("expected last_used_at to be set")
	}

	if _, err := s.TeamForKey("shrt_bogus"); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if err := s.DeleteKey(key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TeamForKey(key.Plaintext); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey after delete, got %v", err)
	}
	if _, err := s.CreateKey(999999, "x", false); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing team, got %v", err)
	}
}

func TestMiddleware(t *testing.T) {
	s := testStore(t)
	team, _ := s.CreateTeam("Acme")
	key, _ := s.CreateKey(team.ID, "ci", false)

	var gotTeam Team
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTeam, gotOK = TeamFrom(r.Context())
	})
	h := Middleware(s)(next)

	// Bearer key resolves the team.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	req.Header.Set("Authorization", "Bearer "+key.Plaintext)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !gotOK || gotTeam.ID != team.ID {
		t.Fatalf("expected team %d, got %+v (ok=%v)", team.ID, gotTeam, gotOK)
	}

	// X-API-Key also works.
	gotOK = false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	req.Header.Set("X-API-Key", key.Plaintext)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !gotOK || gotTeam.ID != team.ID {
		t.Fatalf("expected team %d via X-API-Key, got %+v", team.ID, gotTeam)
	}

	// No key falls back to the default team.
	gotOK = false
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/links", nil))
	if !gotOK || gotTeam.ID != DefaultTeamID {
		t.Fatalf("expected default team, got %+v (ok=%v)", gotTeam, gotOK)
	}

	// Invalid key is rejected.
	rec := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	req.Header.Set("Authorization", "Bearer shrt_invalid")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
