package cache

import (
	"sync"

	"github.com/UsmanXTech/shorty/internal/links"

)

// Cache is a small thread-safe in-memory cache keyed by link slug.
type Cache struct {
	mu sync.RWMutex
	items map[string]links.Link
}

func New() *Cache { return &Cache{items: make(map[string]links.Link)} }

func (c *Cache) Get(slug string) (links.Link, bool) {
	c.mu.RLock(); defer c.mu.RUnlock()
	link, ok := c.items[slug]
	return link, ok
}

func (c *Cache) Set(link links.Link) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.items[link.Slug] = link
}

func (c *Cache) Delete(slug string) {
	c.mu.Lock(); defer c.mu.Unlock()
	delete(c.items, slug)
}

func (c *Cache) Clear() {
	c.mu.Lock(); defer c.mu.Unlock()
	c.items = make(map[string]links.Link)
}
