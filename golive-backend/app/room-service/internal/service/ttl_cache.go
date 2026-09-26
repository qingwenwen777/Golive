package service

import (
	"sync"
	"time"
)

// ttlCache is a small in-process cache for hot, viewer-independent read
// paths. Concurrent misses for one key share a single load, and failed loads
// are not cached. A nil cache or a non-positive TTL disables caching.
type ttlCache[V any] struct {
	ttl        time.Duration
	maxEntries int
	now        func() time.Time

	mu      sync.Mutex
	entries map[string]*ttlCacheEntry[V]
}

type ttlCacheEntry[V any] struct {
	done    chan struct{}
	value   V
	err     error
	expires time.Time
}

func newTTLCache[V any](ttl time.Duration, maxEntries int) *ttlCache[V] {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &ttlCache[V]{
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
		entries:    map[string]*ttlCacheEntry[V]{},
	}
}

// get returns the cached value for key, calling load when it is missing or
// expired. The returned value is shared: callers must not modify it.
func (c *ttlCache[V]) get(key string, load func() (V, error)) (V, error) {
	if c == nil || c.ttl <= 0 {
		return load()
	}
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		select {
		case <-entry.done:
			if entry.err == nil && c.now().Before(entry.expires) {
				c.mu.Unlock()
				return entry.value, nil
			}
		default:
			c.mu.Unlock()
			<-entry.done
			if entry.err != nil {
				// The shared load failed, possibly only because the first
				// caller's context ended; retry with this caller's.
				return load()
			}
			return entry.value, nil
		}
	}
	if len(c.entries) >= c.maxEntries {
		c.evictLocked()
	}
	entry := &ttlCacheEntry[V]{done: make(chan struct{})}
	c.entries[key] = entry
	c.mu.Unlock()

	entry.value, entry.err = load()
	entry.expires = c.now().Add(c.ttl)
	close(entry.done)
	if entry.err != nil {
		c.mu.Lock()
		if c.entries[key] == entry {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	return entry.value, entry.err
}

// evictLocked drops expired entries, then arbitrary finished ones, until
// there is room for one more. In-flight loads are kept.
func (c *ttlCache[V]) evictLocked() {
	now := c.now()
	for key, entry := range c.entries {
		select {
		case <-entry.done:
			if !now.Before(entry.expires) {
				delete(c.entries, key)
			}
		default:
		}
	}
	for key, entry := range c.entries {
		if len(c.entries) < c.maxEntries {
			return
		}
		select {
		case <-entry.done:
			delete(c.entries, key)
		default:
		}
	}
}
