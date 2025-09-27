package forking

import "sync"

type simpleCache struct {
	mu    sync.RWMutex
	store map[string][]byte
	size  int
}

func NewSimpleCache(max int) Cache {
	return &simpleCache{
		store: make(map[string][]byte, max),
		size:  max,
	}
}

func (c *simpleCache) Get(key []byte) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.store[string(key)]
}

func (c *simpleCache) Set(key []byte, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.store) >= c.size {
		for k := range c.store {
			delete(c.store, k)
			break
		}
	}
	k := string(key)
	if value == nil {
		delete(c.store, k)
		return
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	c.store[k] = cp
}

func (c *simpleCache) Has(key []byte) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.store[string(key)]
	return ok, nil
}

func (c *simpleCache) Delete(key []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, string(key))
}

func (c *simpleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string][]byte, c.size)
}
