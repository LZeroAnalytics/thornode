package forking

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type CacheEntry struct {
	Value     []byte
	Timestamp time.Time
	Height    int64
}

type CacheStats struct {
	Size     int
	Capacity int
}

type lruCache struct {
	cache *lru.Cache[string, *CacheEntry]
	mu    sync.RWMutex
	ttl   time.Duration
}

func NewLRUCache(size int, ttl time.Duration) (Cache, error) {
	cache, err := lru.New[string, *CacheEntry](size)
	if err != nil {
		return nil, err
	}
	
	lruCache := &lruCache{
		cache: cache,
		ttl:   ttl,
	}
	
	if ttl > 0 {
		go lruCache.cleanupExpired()
	}
	
	return lruCache, nil
}

func (c *lruCache) Get(key []byte) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, ok := c.cache.Get(string(key))
	if !ok {
		return nil
	}
	
	if c.ttl > 0 && time.Since(entry.Timestamp) > c.ttl {
		c.mu.RUnlock()
		c.mu.Lock()
		c.cache.Remove(string(key))
		c.mu.Unlock()
		c.mu.RLock()
		return nil
	}
	
	return entry.Value
}

func (c *lruCache) Set(key []byte, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	
	entry := &CacheEntry{
		Value:     valueCopy,
		Timestamp: time.Now(),
		Height:    0, // Will be set by caller if needed
	}
	
	c.cache.Add(string(key), entry)
}

func (c *lruCache) Has(key []byte) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, ok := c.cache.Peek(string(key))
	if !ok {
		return false, nil
	}
	
	if c.ttl > 0 && time.Since(entry.Timestamp) > c.ttl {
		return false, nil
	}
	
	return true, nil
}

func (c *lruCache) Delete(key []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache.Remove(string(key))
}

func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache.Purge()
}

func (c *lruCache) cleanupExpired() {
	if c.ttl <= 0 {
		return // No TTL configured
	}
	
	ticker := time.NewTicker(c.ttl / 4) // Cleanup every quarter of TTL
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		
		keys := c.cache.Keys()
		for _, key := range keys {
			if entry, ok := c.cache.Peek(key); ok {
				if time.Since(entry.Timestamp) > c.ttl {
					c.cache.Remove(key)
				}
			}
		}
		
		c.mu.Unlock()
	}
}

func (c *lruCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return CacheStats{
		Size:     c.cache.Len(),
		Capacity: 0, // LRU cache doesn't expose capacity in this version
	}
}
