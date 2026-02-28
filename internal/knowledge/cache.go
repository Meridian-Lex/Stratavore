package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// KnowledgeCache wraps Redis for query result caching.
type KnowledgeCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewKnowledgeCache creates a cache client connecting to host:port.
// ttlSeconds of 0 uses the default CacheTTLSeconds constant.
func NewKnowledgeCache(host string, port int, ttlSeconds int) *KnowledgeCache {
	if ttlSeconds <= 0 {
		ttlSeconds = CacheTTLSeconds
	}
	return &KnowledgeCache{
		client: redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", host, port),
		}),
		ttl: time.Duration(ttlSeconds) * time.Second,
	}
}

// cacheKey returns the Redis key for a (query, k) pair.
func (c *KnowledgeCache) cacheKey(query string, k int) string {
	raw := fmt.Sprintf("%s|%d", query, k)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%s%x", CacheKeyPrefix, hash)
}

// Get retrieves cached chunks for (query, k). Returns nil, nil on cache miss.
func (c *KnowledgeCache) Get(ctx context.Context, query string, k int) ([]*Chunk, error) {
	key := c.cacheKey(query, k)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var chunks []*Chunk
	if err := json.Unmarshal(data, &chunks); err != nil {
		return nil, fmt.Errorf("unmarshal cached chunks: %w", err)
	}
	return chunks, nil
}

// Set stores chunks for (query, k) with the configured TTL.
func (c *KnowledgeCache) Set(ctx context.Context, query string, k int, chunks []*Chunk) error {
	key := c.cacheKey(query, k)
	data, err := json.Marshal(chunks)
	if err != nil {
		return fmt.Errorf("marshal chunks: %w", err)
	}
	return c.client.Set(ctx, key, data, c.ttl).Err()
}

// InvalidateFile clears all query cache entries when a source file changes.
// Any cached result could contain chunks from the modified file, so we flush
// the entire query cache rather than tracking per-file cache membership.
// Keys are deleted in batches as they are scanned to bound memory usage.
func (c *KnowledgeCache) InvalidateFile(ctx context.Context) error {
	var cursor uint64
	pattern := CacheKeyPrefix + "*"
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan cache keys: %w", err)
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete cache keys: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

// Ping checks Redis connectivity.
func (c *KnowledgeCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (c *KnowledgeCache) Close() error {
	return c.client.Close()
}
