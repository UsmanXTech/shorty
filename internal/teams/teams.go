package teams

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("team not found")
	ErrNoKey    = errors.New("api key not found")
)

const DefaultTeamID = 1

type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKey struct {
	ID         int64      `json:"id"`
	TeamID     int64      `json:"team_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"key_prefix"`
	IsAdmin    bool       `json:"is_admin"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// Plaintext is only populated at creation time; never stored.
	Plaintext string `json:"key,omitempty"`
}

type Store interface {
	CreateTeam(name string) (Team, error)
	ListTeams() ([]Team, error)
	GetTeam(id int64) (Team, error)
	CreateKey(teamID int64, name string, admin bool) (APIKey, error)
	GetKey(id int64) (APIKey, error)
	ListKeys(teamID int64) ([]APIKey, error)
	DeleteKey(id int64) error
	TeamForKey(key string) (Team, error)
	// KeyForKey resolves the full API key record (including admin flag) for a raw key.
	KeyForKey(key string) (APIKey, error)
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// NewKey generates a random API key and its public prefix.
func NewKey() (key, prefix string) {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	key = "shrt_" + hex.EncodeToString(buf)
	return key, key[:12]
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) CreateTeam(name string) (Team, error) {
	if strings.TrimSpace(name) == "" {
		return Team{}, errors.New("team name must not be empty")
	}
	res, err := s.db.Exec(
		"INSERT INTO teams (name, created_at) VALUES (?, ?)",
		strings.TrimSpace(name), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return Team{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Team{}, err
	}
	return s.GetTeam(id)
}

func (s *SQLiteStore) ListTeams() ([]Team, error) {
	rows, err := s.db.Query("SELECT id, name, created_at FROM teams ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Team
	for rows.Next() {
		t, err := scanTeam(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetTeam(id int64) (Team, error) {
	return scanTeam(s.db.QueryRow("SELECT id, name, created_at FROM teams WHERE id = ?", id))
}

func (s *SQLiteStore) CreateKey(teamID int64, name string, admin bool) (APIKey, error) {
	if _, err := s.GetTeam(teamID); err != nil {
		return APIKey{}, err
	}
	if strings.TrimSpace(name) == "" {
		return APIKey{}, errors.New("key name must not be empty")
	}
	key, prefix := NewKey()
	adminInt := 0
	if admin {
		adminInt = 1
	}
	res, err := s.db.Exec(
		"INSERT INTO api_keys (team_id, name, key_prefix, key_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		teamID, strings.TrimSpace(name), prefix, hashKey(key), adminInt, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return APIKey{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIKey{}, err
	}
	out, err := s.getKey(id)
	if err != nil {
		return APIKey{}, err
	}
	out.Plaintext = key
	return out, nil
}

func (s *SQLiteStore) ListKeys(teamID int64) ([]APIKey, error) {
	rows, err := s.db.Query(
		"SELECT id, team_id, name, key_prefix, is_admin, created_at, last_used_at FROM api_keys WHERE team_id = ? ORDER BY id", teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) DeleteKey(id int64) error {
	res, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoKey
	}
	return nil
}

func (s *SQLiteStore) TeamForKey(key string) (Team, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Team{}, ErrNoKey
	}
	var teamID int64
	err := s.db.QueryRow("SELECT team_id FROM api_keys WHERE key_hash = ?", hashKey(key)).Scan(&teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Team{}, ErrNoKey
		}
		return Team{}, err
	}
	_, _ = s.db.Exec("UPDATE api_keys SET last_used_at = ? WHERE key_hash = ?",
		time.Now().UTC().Format(time.RFC3339Nano), hashKey(key))
	return s.GetTeam(teamID)
}

func (s *SQLiteStore) getKey(id int64) (APIKey, error) {
	return scanKey(s.db.QueryRow(
		"SELECT id, team_id, name, key_prefix, is_admin, created_at, last_used_at FROM api_keys WHERE id = ?", id,
	))
}

// GetKey returns an API key by its numeric id.
func (s *SQLiteStore) GetKey(id int64) (APIKey, error) {
	return s.getKey(id)
}

// KeyForKey resolves the stored API key record for a raw key value.
func (s *SQLiteStore) KeyForKey(key string) (APIKey, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return APIKey{}, ErrNoKey
	}
	return scanKey(s.db.QueryRow(
		"SELECT id, team_id, name, key_prefix, is_admin, created_at, last_used_at FROM api_keys WHERE key_hash = ?",
		hashKey(key),
	))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTeam(row scanner) (Team, error) {
	var t Team
	var created string
	if err := row.Scan(&t.ID, &t.Name, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Team{}, ErrNotFound
		}
		return Team{}, err
	}
	ts, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Team{}, err
	}
	t.CreatedAt = ts
	return t, nil
}

func scanKey(row scanner) (APIKey, error) {
	var k APIKey
	var created string
	var lastUsed sql.NullString
	var isAdmin int
	if err := row.Scan(&k.ID, &k.TeamID, &k.Name, &k.Prefix, &isAdmin, &created, &lastUsed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIKey{}, ErrNoKey
		}
		return APIKey{}, err
	}
	k.IsAdmin = isAdmin == 1
	ts, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return APIKey{}, err
	}
	k.CreatedAt = ts
	if lastUsed.Valid && lastUsed.String != "" {
		t, err := time.Parse(time.RFC3339Nano, lastUsed.String)
		if err == nil {
			k.LastUsedAt = &t
		}
	}
	return k, nil
}

// context plumbing

type ctxKey struct{}
type ctxKeyKey struct{}

func WithTeam(ctx context.Context, t Team) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

func TeamFrom(ctx context.Context) (Team, bool) {
	t, ok := ctx.Value(ctxKey{}).(Team)
	return t, ok
}

// WithAPIKey stores the resolved API key alongside the team.
func WithAPIKey(ctx context.Context, k APIKey) context.Context {
	return context.WithValue(ctx, ctxKeyKey{}, k)
}

// KeyFrom returns the API key that authenticated the request, if any.
func KeyFrom(ctx context.Context) (APIKey, bool) {
	k, ok := ctx.Value(ctxKeyKey{}).(APIKey)
	return k, ok && k.ID != 0
}

// Middleware resolves the team from an API key and stores it in the request context.
// Requests without a key fall back to the default team.
func Middleware(store Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := apiKeyFromRequest(r)
			var team Team
			var apiKey APIKey
			if key != "" {
				var err error
				// TeamForKey validates the key and touches last_used_at.
				team, err = store.TeamForKey(key)
				if err != nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}
				apiKey, err = store.KeyForKey(key)
				if err != nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}
			} else {
				var err error
				team, err = store.GetTeam(DefaultTeamID)
				if err != nil {
					team = Team{ID: DefaultTeamID, Name: "Default"}
				}
			}
			ctx := WithTeam(r.Context(), team)
			if apiKey.ID != 0 {
				ctx = WithAPIKey(ctx, apiKey)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func apiKeyFromRequest(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if rest, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}
