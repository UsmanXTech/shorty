package domains

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("domain not found")
	ErrConflict = errors.New("domain already exists")
)

type Domain struct {
	ID        int64     `json:"id"`
	TeamID    int64     `json:"team_id"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Create(teamID int64, domain string) (Domain, error)
	List(teamID int64) ([]Domain, error)
	Get(id int64) (Domain, error)
	GetByHost(host string) (Domain, error)
	Delete(id int64) error
}

// Normalize lowercases a host and strips any port.
func Normalize(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 {
		if h := host[:i]; !strings.Contains(h, ":") {
			host = h
		}
	}
	return strings.TrimSuffix(host, ".")
}

type MemoryStore struct {
	mu      sync.RWMutex
	nextID  int64
	domains map[int64]Domain
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1, domains: make(map[int64]Domain)}
}

func (s *MemoryStore) Create(teamID int64, domain string) (Domain, error) {
	domain = Normalize(domain)
	if domain == "" {
		return Domain{}, errors.New("domain must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.domains {
		if d.Domain == domain {
			return Domain{}, ErrConflict
		}
	}
	d := Domain{ID: s.nextID, TeamID: teamID, Domain: domain, CreatedAt: time.Now().UTC()}
	s.nextID++
	s.domains[d.ID] = d
	return d, nil
}

func (s *MemoryStore) List(teamID int64) ([]Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Domain
	for _, d := range s.domains {
		if d.TeamID == teamID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *MemoryStore) Get(id int64) (Domain, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domains[id]
	if !ok {
		return Domain{}, ErrNotFound
	}
	return d, nil
}

func (s *MemoryStore) GetByHost(host string) (Domain, error) {
	host = Normalize(host)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.domains {
		if d.Domain == host {
			return d, nil
		}
	}
	return Domain{}, ErrNotFound
}

func (s *MemoryStore) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[id]; !ok {
		return ErrNotFound
	}
	delete(s.domains, id)
	return nil
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Create(teamID int64, domain string) (Domain, error) {
	domain = Normalize(domain)
	if domain == "" {
		return Domain{}, errors.New("domain must not be empty")
	}
	res, err := s.db.Exec(
		"INSERT INTO domains (team_id, domain, created_at) VALUES (?, ?, ?)",
		teamID, domain, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "constraint failed") {
			return Domain{}, ErrConflict
		}
		return Domain{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Domain{}, err
	}
	return s.Get(id)
}

func (s *SQLiteStore) List(teamID int64) ([]Domain, error) {
	rows, err := s.db.Query("SELECT id, team_id, domain, created_at FROM domains WHERE team_id = ? ORDER BY id", teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Get(id int64) (Domain, error) {
	return scanDomain(s.db.QueryRow("SELECT id, team_id, domain, created_at FROM domains WHERE id = ?", id))
}

func (s *SQLiteStore) GetByHost(host string) (Domain, error) {
	return scanDomain(s.db.QueryRow("SELECT id, team_id, domain, created_at FROM domains WHERE domain = ?", Normalize(host)))
}

func (s *SQLiteStore) Delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM domains WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDomain(row scanner) (Domain, error) {
	var d Domain
	var created string
	if err := row.Scan(&d.ID, &d.TeamID, &d.Domain, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Domain{}, ErrNotFound
		}
		return Domain{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Domain{}, err
	}
	d.CreatedAt = t
	return d, nil
}
