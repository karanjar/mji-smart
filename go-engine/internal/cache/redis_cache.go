//// internal/cache/redis_cache.go
//package cache
//
//import (
//	"context"
//	"encoding/json"
//	"fmt"
//	"time"
//
//	"github.com/redis/go-redis/v9"
//	"go.uber.org/zap"
//)
//
//// RedisQueryCache implements QueryCache with go-redis/v9
//type RedisQueryCache struct {
//	client *redis.Client
//	logger *zap.Logger
//	prefix string
//}
//
//// NewRedisQueryCache creates a new Redis cache instance
//func NewRedisQueryCache(client *redis.Client, logger *zap.Logger, prefix string) *RedisQueryCache {
//	return &RedisQueryCache{
//		client: client,
//		logger: logger,
//		prefix: prefix,
//	}
//}
//
//func (c *RedisQueryCache) buildKey(key string) string {
//	return fmt.Sprintf("%s:%s", c.prefix, key)
//}
//
//func (c *RedisQueryCache) Get(ctx context.Context, key string, dest interface{}) error {
//	fullKey := c.buildKey(key)
//
//	data, err := c.client.Get(ctx, fullKey).Bytes()
//	if err != nil {
//		if err == redis.Nil {
//			return ErrCacheMiss
//		}
//		return fmt.Errorf("cache get failed: %w", err)
//	}
//
//	if err := json.Unmarshal(data, dest); err != nil {
//		return fmt.Errorf("cache unmarshal failed: %w", err)
//	}
//
//	c.logger.Debug("Cache hit", zap.String("key", fullKey))
//	return nil
//}
//
//func (c *RedisQueryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
//	fullKey := c.buildKey(key)
//
//	data, err := json.Marshal(value)
//	if err != nil {
//		return fmt.Errorf("cache marshal failed: %w", err)
//	}
//
//	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
//		return fmt.Errorf("cache set failed: %w", err)
//	}
//
//	c.logger.Debug("Cache set", zap.String("key", fullKey), zap.Duration("ttl", ttl))
//	return nil
//}
//
//func (c *RedisQueryCache) Delete(ctx context.Context, key string) error {
//	fullKey := c.buildKey(key)
//
//	if err := c.client.Del(ctx, fullKey).Err(); err != nil {
//		return fmt.Errorf("cache delete failed: %w", err)
//	}
//
//	c.logger.Debug("Cache deleted", zap.String("key", fullKey))
//	return nil
//}
//
//func (c *RedisQueryCache) DeletePattern(ctx context.Context, pattern string) error {
//	fullPattern := c.buildKey(pattern)
//
//	// Use SCAN to find keys matching pattern
//	var cursor uint64
//	var keys []string
//
//	for {
//		var scanKeys []string
//		var err error
//
//		scanKeys, cursor, err = c.client.Scan(ctx, cursor, fullPattern, 100).Result()
//		if err != nil {
//			return fmt.Errorf("cache scan failed: %w", err)
//		}
//
//		keys = append(keys, scanKeys...)
//
//		if cursor == 0 {
//			break
//		}
//	}
//
//	if len(keys) > 0 {
//		if err := c.client.Del(ctx, keys...).Err(); err != nil {
//			return fmt.Errorf("cache delete pattern failed: %w", err)
//		}
//
//		c.logger.Debug("Cache pattern deleted",
//			zap.String("pattern", fullPattern),
//			zap.Int("count", len(keys)))
//	}
//
//	return nil
//}
//
//func (c *RedisQueryCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fetcher func() (interface{}, error)) error {
//	// Try to get from cache
//	err := c.Get(ctx, key, dest)
//	if err == nil {
//		return nil
//	}
//
//	if err != ErrCacheMiss {
//		c.logger.Warn("Cache get failed, falling back to fetcher", zap.Error(err))
//	}
//
//	// Fetch from source
//	//result, err := fetcher()
//	if err != nil {
//		return err
//	}
//
//	// Set in cache
//	if err := c.Set(ctx, key, result, ttl); err != nil {
//		c.logger.Warn("Failed to set cache", zap.Error(err))
//		// Don't fail the request if cache set fails
//	}
//
//	// Copy result to dest
//	data, _ := json.Marshal(result)
//	return json.Unmarshal(data, dest)
//}
//
////var ErrCacheMiss = fmt.Errorf("cache miss")
