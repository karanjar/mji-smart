// internal/cache/query_cache.go
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// QueryCache interface defines cache operations
type QueryCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
	GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fetcher func() (interface{}, error)) error
}

// RedisQueryCache implements QueryCache with Redis
type RedisQueryCache struct {
	client *redis.Client
	logger *zap.Logger
	prefix string
}

func NewRedisQueryCache(client *redis.Client, logger *zap.Logger, prefix string) *RedisQueryCache {
	return &RedisQueryCache{
		client: client,
		logger: logger,
		prefix: prefix,
	}
}

func (c *RedisQueryCache) buildKey(key string) string {
	return fmt.Sprintf("%s:%s", c.prefix, key)
}

func (c *RedisQueryCache) Get(ctx context.Context, key string, dest interface{}) error {
	fullKey := c.buildKey(key)

	data, err := c.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return ErrCacheMiss
		}
		return fmt.Errorf("cache get failed: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("cache unmarshal failed: %w", err)
	}

	c.logger.Debug("Cache hit", zap.String("key", fullKey))
	return nil
}

func (c *RedisQueryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := c.buildKey(key)

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal failed: %w", err)
	}

	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache set failed: %w", err)
	}

	c.logger.Debug("Cache set", zap.String("key", fullKey), zap.Duration("ttl", ttl))
	return nil
}

func (c *RedisQueryCache) Delete(ctx context.Context, key string) error {
	fullKey := c.buildKey(key)

	if err := c.client.Del(ctx, fullKey).Err(); err != nil {
		return fmt.Errorf("cache delete failed: %w", err)
	}

	c.logger.Debug("Cache deleted", zap.String("key", fullKey))
	return nil
}

func (c *RedisQueryCache) DeletePattern(ctx context.Context, pattern string) error {
	fullPattern := c.buildKey(pattern)

	iter := c.client.Scan(ctx, 0, fullPattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			c.logger.Error("Failed to delete cache key",
				zap.String("key", iter.Val()),
				zap.Error(err))
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("cache delete pattern failed: %w", err)
	}

	c.logger.Debug("Cache pattern deleted", zap.String("pattern", fullPattern))
	return nil
}

func (c *RedisQueryCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fetcher func() (interface{}, error)) error {
	// Try to get from cache
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil
	}

	if err != ErrCacheMiss {
		c.logger.Warn("Cache get failed, falling back to fetcher", zap.Error(err))
	}

	// Fetch from source
	result, err := fetcher()
	if err != nil {
		return err
	}

	// Set in cache
	if err := c.Set(ctx, key, result, ttl); err != nil {
		c.logger.Warn("Failed to set cache", zap.Error(err))
		// Don't fail the request if cache set fails
	}

	// Copy result to dest
	data, _ := json.Marshal(result)
	return json.Unmarshal(data, dest)
}

var ErrCacheMiss = fmt.Errorf("cache miss")
