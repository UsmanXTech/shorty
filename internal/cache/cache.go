package cache

import (
	"sync"

	"github.com/UsmanXTech/shorty/internal/links"
)

// Cache is a small thread-safe in-memory cache keyed by link slug.
// It is size-bounded: once full, Set evicts an arbitrary entry to make
// room, so a flood of distinct slugs can't grow memory without bound.
type Cache struct {
	mu    sync.RWMutex
	items map[string]links.Link
}

// maxItems bounds cache memory. Links are small; 10k entries is ample for
// a self-hosted shortener while keeping the worst case predictable.
const maxItems = 10000

func New() *Cache { return &Cache{items: make(map[string]links.Link)} }

func (c *Cache) Get(slug string) (links.Link, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	link, ok := c.items[slug]
	return link, ok
}

func (c *Cache) Set(link links.Link) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[link.Slug]; !ok && len(c.items) >= maxItems {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
	c.items[link.Slug] = link
}

func (c *Cache) Delete(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, slug)
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]links.Link)
}

// Len reports the number of cached entries.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
