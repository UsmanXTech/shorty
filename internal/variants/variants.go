package variants

import (
	"database/sql"
	"errors"
	"math"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("variant not found")
	ErrNoLink   = errors.New("link not found")
)

type Variant struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"link_id"`
	URL       string    `json:"url"`
	Weight    int       `json:"weight"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Create(linkID int64, url string, weight int) (Variant, error)
	List(linkID int64) ([]Variant, error)
	Get(id int64) (Variant, error)
	Update(id int64, url string, weight int) (Variant, error)
	Delete(id int64) error
	IncrementClicks(id int64) error
}

// ListCache is a small bounded cache of variant lists keyed by link ID.
//
// The redirect hot path lists variants on every click; most links have none,
// so caching (including empty lists) avoids a database query per redirect.
// VariantAPI invalidates entries on create/update/delete, and both sides
// share one instance in-process, so cached lists are never stale.
type ListCache struct {
	mu    sync.RWMutex
	items map[int64][]Variant
}

// maxCachedLists bounds ListCache memory.
const maxCachedLists = 5000

// NewListCache returns an empty variant-list cache.
func NewListCache() *ListCache { return &ListCache{items: make(map[int64][]Variant)} }

// Get returns the cached list for a link.
func (c *ListCache) Get(linkID int64) ([]Variant, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	list, ok := c.items[linkID]
	return list, ok
}

// Set stores the list for a link, evicting an arbitrary entry when full.
func (c *ListCache) Set(linkID int64, list []Variant) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[linkID]; !ok && len(c.items) >= maxCachedLists {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[linkID] = list
}

// Invalidate drops the cached list for a link after a mutation.
func (c *ListCache) Invalidate(linkID int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, linkID)
}

// Pick returns the variant chosen by weighted random selection, or nil when empty.
func Pick(list []Variant, n int64) *Variant {
	var total int64
	for _, v := range list {
		if v.Weight > 0 {
			// Saturate instead of overflowing on adversarial weights.
			if w := int64(v.Weight); total > math.MaxInt64-w {
				total = math.MaxInt64
			} else {
				total += w
			}
		}
	}
	if total == 0 || len(list) == 0 {
		return nil
	}
	r := n % total
	for i := range list {
		w := int64(list[i].Weight)
		if w <= 0 {
			continue
		}
		if r < w {
			return &list[i]
		}
		r -= w
	}
	return &list[len(list)-1]
}

type MemoryStore struct {
	mu       sync.RWMutex
	nextID   int64
	variants map[int64]Variant
	links    map[int64]bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1, variants: make(map[int64]Variant), links: make(map[int64]bool)}
}

func (s *MemoryStore) AddLink(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[id] = true
}

func (s *MemoryStore) Create(linkID int64, url string, weight int) (Variant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.links[linkID] {
		return Variant{}, ErrNoLink
	}
	// weight < 0 (omitted) defaults to 1; 0 disables the variant.
	if weight < 0 {
		weight = 1
	}
	v := Variant{ID: s.nextID, LinkID: linkID, URL: url, Weight: weight, CreatedAt: time.Now().UTC()}
	s.nextID++
	s.variants[v.ID] = v
	return v, nil
}

func (s *MemoryStore) List(linkID int64) ([]Variant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Variant
	for _, v := range s.variants {
		if v.LinkID == linkID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (s *MemoryStore) Get(id int64) (Variant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.variants[id]
	if !ok {
		return Variant{}, ErrNotFound
	}
	return v, nil
}

func (s *MemoryStore) Update(id int64, url string, weight int) (Variant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.variants[id]
	if !ok {
		return Variant{}, ErrNotFound
	}
	if url != "" {
		v.URL = url
	}
	// weight >= 0 applies (0 disables the variant); -1 preserves.
	if weight >= 0 {
		v.Weight = weight
	}
	s.variants[id] = v
	return v, nil
}

func (s *MemoryStore) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.variants[id]; !ok {
		return ErrNotFound
	}
	delete(s.variants, id)
	return nil
}

func (s *MemoryStore) IncrementClicks(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.variants[id]
	if !ok {
		return ErrNotFound
	}
	v.Clicks++
	s.variants[id] = v
	return nil
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) Create(linkID int64, url string, weight int) (Variant, error) {
	// weight < 0 (omitted) defaults to 1; 0 disables the variant.
	if weight < 0 {
		weight = 1
	}
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE id = ?)", linkID).Scan(&exists); err != nil {
		return Variant{}, err
	}
	if !exists {
		return Variant{}, ErrNoLink
	}
	res, err := s.db.Exec(
		"INSERT INTO link_variants (link_id, url, weight, created_at) VALUES (?, ?, ?, ?)",
		linkID, url, weight, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return Variant{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Variant{}, err
	}
	return s.Get(id)
}

func (s *SQLiteStore) List(linkID int64) ([]Variant, error) {
	rows, err := s.db.Query(
		"SELECT id, link_id, url, weight, clicks, created_at FROM link_variants WHERE link_id = ? ORDER BY id", linkID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Variant
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Get(id int64) (Variant, error) {
	return scanVariant(s.db.QueryRow(
		"SELECT id, link_id, url, weight, clicks, created_at FROM link_variants WHERE id = ?", id,
	))
}

func (s *SQLiteStore) Update(id int64, url string, weight int) (Variant, error) {
	v, err := s.Get(id)
	if err != nil {
		return Variant{}, err
	}
	if url != "" {
		v.URL = url
	}
	// weight >= 0 applies (0 disables the variant); -1 preserves.
	if weight >= 0 {
		v.Weight = weight
	}
	res, err := s.db.Exec("UPDATE link_variants SET url = ?, weight = ? WHERE id = ?", v.URL, v.Weight, id)
	if err != nil {
		return Variant{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Variant{}, ErrNotFound
	}
	return s.Get(id)
}

func (s *SQLiteStore) Delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM link_variants WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) IncrementClicks(id int64) error {
	res, err := s.db.Exec("UPDATE link_variants SET clicks = clicks + 1 WHERE id = ?", id)
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

func scanVariant(row scanner) (Variant, error) {
	var v Variant
	var created string
	if err := row.Scan(&v.ID, &v.LinkID, &v.URL, &v.Weight, &v.Clicks, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Variant{}, ErrNotFound
		}
		return Variant{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Variant{}, err
	}
	v.CreatedAt = t
	return v, nil
}
