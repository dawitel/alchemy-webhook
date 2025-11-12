package cache

import (
	"crypto/tls"
	"fmt"
	"time"
)

const defaultCleanupInterval = 1 * time.Hour

// CacheConfig represents the cache configuration
type CacheConfig struct {
	Enabled bool
	Type    string // "redis" or "memory"
	Redis   RedisConfig
	Memory  MemoryConfig
}

// MemoryConfig represents memory cache configuration
type MemoryConfig struct {
	MaxSize         int
	CleanupInterval time.Duration
	EnableLRU       bool
}

// NewCache creates a cache instance based on the configuration
func NewCache(cfg CacheConfig) (Cache, error) {
	if !cfg.Enabled {
		return NewNoOpCache(), nil
	}

	switch cfg.Type {
	case "memory":
		cleanupInterval := cfg.Memory.CleanupInterval
		if cleanupInterval == 0 {
			cleanupInterval = defaultCleanupInterval
		}
		return NewMemoryCache(
			cfg.Memory.MaxSize,
			cleanupInterval,
			cfg.Memory.EnableLRU,
		), nil

	case "redis":
		var tlsConfig *tls.Config
		if cfg.Redis.EnableTLS {
			if cfg.Redis.TLSConfig != nil {
				if tc, ok := cfg.Redis.TLSConfig.(*tls.Config); ok {
					tlsConfig = tc
				} else {
					tlsConfig = &tls.Config{
						InsecureSkipVerify: cfg.Redis.TLSSkipVerify,
					}
				}
			} else {
				tlsConfig = &tls.Config{
					InsecureSkipVerify: cfg.Redis.TLSSkipVerify,
				}
			}
		}

		redisConfig := RedisConfig{
			Address:       cfg.Redis.Address,
			Password:      cfg.Redis.Password,
			DB:            cfg.Redis.DB,
			PoolSize:      cfg.Redis.PoolSize,
			MinIdleConns:  cfg.Redis.MinIdleConns,
			DialTimeout:   cfg.Redis.DialTimeout,
			ReadTimeout:   cfg.Redis.ReadTimeout,
			WriteTimeout:  cfg.Redis.WriteTimeout,
			EnableTLS:     cfg.Redis.EnableTLS,
			TLSSkipVerify: cfg.Redis.TLSSkipVerify,
			TLSConfig:     tlsConfig,
		}

		return NewRedisCache(redisConfig)

	default:
		return nil, fmt.Errorf("unknown cache type: %s", cfg.Type)
	}
}
