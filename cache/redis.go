package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisCache is a Redis-based cache implementation
type RedisCache struct {
	client *redis.Client
	prefix string
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	opts := &redis.Options{
		Addr:         config.Address,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	if config.EnableTLS {
		if tlsConfig, ok := config.TLSConfig.(*tls.Config); ok {
			opts.TLSConfig = tlsConfig
		} else {
			opts.TLSConfig = &tls.Config{
				InsecureSkipVerify: config.TLSSkipVerify,
			}
		}
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
		prefix: "alchemy_webhook:",
	}, nil
}

// IsProcessed checks if a transaction has been processed
func (c *RedisCache) IsProcessed(ctx context.Context, txHash string) (bool, error) {
	key := c.prefix + txHash
	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check Redis key: %w", err)
	}
	return exists > 0, nil
}

// MarkProcessed marks a transaction as processed
func (c *RedisCache) MarkProcessed(ctx context.Context, txHash string, ttl time.Duration) error {
	key := c.prefix + txHash
	err := c.client.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set Redis key: %w", err)
	}
	return nil
}

// Close closes the cache and releases resources
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// RedisConfig contains Redis connection configuration
type RedisConfig struct {
	Address       string
	Password      string
	DB            int
	PoolSize      int
	MinIdleConns  int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	EnableTLS     bool
	TLSSkipVerify bool
	TLSConfig     interface{} // *tls.Config - using interface{} to avoid import
}
