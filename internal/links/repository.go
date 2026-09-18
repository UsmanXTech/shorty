package links

import (
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("link not found")
	ErrConflict = errors.New("slug already exists")
)

type Repository interface {
	Create(Link) (Link, error)
	List() ([]Link, error)
	GetByID(int64) (Link, error)
	GetBySlug(string) (Link, error)
	Update(Link) (Link, error)
	Delete(int64) error
}

type MemoryRepository struct {
	mu     sync.RWMutex
	nextID int64
	links  map[int64]Link
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{nextID: 1, links: make(map[int64]Link)}
}

func (r *MemoryRepository) Create(link Link) (Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.links {
		if existing.Slug == link.Slug {
			return Link{}, ErrConflict
		}
	}
	link.ID = r.nextID
	r.nextID++
	r.links[link.ID] = link
	return link, nil
}

func (r *MemoryRepository) List() ([]Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Link, 0, len(r.links))
	for _, link := range r.links {
		out = append(out, link)
	}
	return out, nil
}

func (r *MemoryRepository) GetByID(id int64) (Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	link, ok := r.links[id]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func (r *MemoryRepository) GetBySlug(slug string) (Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, link := range r.links {
		if link.Slug == slug {
			return link, nil
		}
	}
	return Link{}, ErrNotFound
}

func (r *MemoryRepository) Update(link Link) (Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[link.ID]; !ok {
		return Link{}, ErrNotFound
	}
	for id, existing := range r.links {
		if id != link.ID && existing.Slug == link.Slug {
			return Link{}, ErrConflict
		}
	}
	r.links[link.ID] = link
	return link, nil
}

func (r *MemoryRepository) Delete(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[id]; !ok {
		return ErrNotFound
	}
	delete(r.links, id)
	return nil
}
