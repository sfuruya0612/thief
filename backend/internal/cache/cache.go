package cache

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Entry holds a cached value with timing metadata.
type Entry[V any] struct {
	Value    V
	CachedAt time.Time
	Expiry   time.Time
}

// Cache is a generic TTL cache with singleflight dogpile prevention.
type Cache[V any] struct {
	mu    sync.RWMutex
	items map[string]Entry[V]
	group singleflight.Group
	stop  chan struct{}
}

// New creates a Cache and starts a janitor goroutine that removes expired
// entries at the given interval. Call Close to stop it.
func New[V any](janitorInterval time.Duration) *Cache[V] {
	c := &Cache[V]{
		items: make(map[string]Entry[V]),
		stop:  make(chan struct{}),
	}
	go c.janitor(janitorInterval)
	return c
}

// Close stops the janitor goroutine.
func (c *Cache[V]) Close() {
	close(c.stop)
}

// Set stores a value under key with the given TTL, returning the Entry.
func (c *Cache[V]) Set(key string, v V, ttl time.Duration) Entry[V] {
	now := time.Now()
	e := Entry[V]{Value: v, CachedAt: now, Expiry: now.Add(ttl)}
	c.mu.Lock()
	c.items[key] = e
	c.mu.Unlock()
	return e
}

// Get returns the Entry for key and whether it was found and not expired.
func (c *Cache[V]) Get(key string) (Entry[V], bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.Expiry) {
		return Entry[V]{}, false
	}
	return e, true
}

// Invalidate removes the entry for key.
func (c *Cache[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// InvalidatePrefix removes every entry whose key starts with prefix.
// Used when a write affects an unknown set of cached keys that share a
// common namespace segment (e.g. all "s3-objects:profile:region:bucket:*"
// entries regardless of the trailing prefix query parameter).
func (c *Cache[V]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	for k := range c.items {
		if strings.HasPrefix(k, prefix) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

// Load is the primary entry point for all cached resource fetches.
// If refresh=true, the existing entry is invalidated before loading.
// Uses singleflight to prevent concurrent duplicate requests to loader.
// Returns the entry, whether it was a cache hit, and any error.
func (c *Cache[V]) Load(
	key string,
	ttl time.Duration,
	refresh bool,
	loader func() (V, error),
) (Entry[V], bool, error) {
	if refresh {
		c.Invalidate(key)
	}

	if e, ok := c.Get(key); ok {
		return e, true, nil
	}

	type result struct {
		entry Entry[V]
	}

	ch := c.group.DoChan(key, func() (any, error) {
		v, err := loader()
		if err != nil {
			return nil, err
		}
		e := c.Set(key, v, ttl)
		return result{entry: e}, nil
	})

	res := <-ch
	if res.Err != nil {
		var zero Entry[V]
		return zero, false, res.Err
	}
	r := res.Val.(result)
	return r.entry, false, nil
}

func (c *Cache[V]) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		case <-c.stop:
			return
		}
	}
}

func (c *Cache[V]) deleteExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.items {
		if now.After(e.Expiry) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}
