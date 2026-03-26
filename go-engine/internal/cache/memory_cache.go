// internal/cache/memory_cache.go
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MemoryQueryCache implements QueryCache with in-memory storage (for testing)
type MemoryQueryCache struct {
	mu     sync.RWMutex
	data   map[string]cacheItem
	logger *zap.Logger
}

type cacheItem struct {
	value     []byte
	expiresAt time.Time
}

func NewMemoryQueryCache(logger *zap.Logger) *MemoryQueryCache {
	c := &MemoryQueryCache{
		data:   make(map[string]cacheItem),
		logger: logger,
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

func (c *MemoryQueryCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[key]
	if !exists {
		return ErrCacheMiss
	}

	if time.Now().After(item.expiresAt) {
		// Item expired
		go c.Delete(ctx, key) // Delete in background
		return ErrCacheMiss
	}

	if err := json.Unmarshal(item.value, dest); err != nil {
		return fmt.Errorf("cache unmarshal failed: %w", err)
	}

	return nil
}

func (c *MemoryQueryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheItem{
		value:     data,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

func (c *MemoryQueryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	return nil
}

func (c *MemoryQueryCache) DeletePattern(ctx context.Context, pattern string) error {
	// Simple pattern matching with * wildcard
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.data {
		if matchPattern(pattern, key) {
			delete(c.data, key)
		}
	}

	return nil
}

func (c *MemoryQueryCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fetcher func() (interface{}, error)) error {
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil
	}

	result, err := fetcher()
	if err != nil {
		return err
	}

	if err := c.Set(ctx, key, result, ttl); err != nil {
		return err
	}

	data, _ := json.Marshal(result)
	return json.Unmarshal(data, dest)
}

func (c *MemoryQueryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.data {
			if now.After(item.expiresAt) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

func matchPattern(pattern, key string) bool {
	if pattern == "*" {
		return true
	}
	// Simple pattern matching - can be extended
	return key == pattern
}
