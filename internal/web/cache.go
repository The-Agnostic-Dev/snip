package web

import (
	"sync"

	"github.com/claretnnamocha/snip/internal/store"
)

// linkCache keeps recently used links in memory so redirects skip the database.
type linkCache struct {
	mu    sync.RWMutex
	items map[string]store.Link
	max   int
}

func newLinkCache(max int) *linkCache {
	return &linkCache{items: make(map[string]store.Link), max: max}
}

func (c *linkCache) get(alias string) (store.Link, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	l, ok := c.items[alias]
	return l, ok
}

func (c *linkCache) put(l store.Link) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		// Simple eviction: start over. Hot links are re-cached on their next visit.
		c.items = make(map[string]store.Link)
	}
	c.items[l.Alias] = l
}

func (c *linkCache) forget(alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, alias)
}
