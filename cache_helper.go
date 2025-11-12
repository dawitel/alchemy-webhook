package alchemywebhook

import (
	"crypto/tls"

	"github.com/dawitel/alchemy-webhook/cache"
)

// newCache creates a cache instance from the configuration
func newCache(cfg CacheConfig) (cache.Cache, error) {
	if !cfg.Enabled {
		return cache.NewNoOpCache(), nil
	}

	cacheCfg := cache.CacheConfig{
		Enabled: cfg.Enabled,
		Type:    cfg.Type,
		Memory: cache.MemoryConfig{
			MaxSize:         cfg.Memory.MaxSize,
			CleanupInterval: cfg.Memory.CleanupInterval,
			EnableLRU:       cfg.Memory.EnableLRU,
		},
	}

	if cfg.Type == "redis" {
		var tlsConfig *tls.Config
		if cfg.Redis.EnableTLS {
			if cfg.Redis.TLSConfig != nil {
				tlsConfig = cfg.Redis.TLSConfig
			} else {
				tlsConfig = &tls.Config{
					InsecureSkipVerify: cfg.Redis.TLSSkipVerify,
				}
			}
		}

		cacheCfg.Redis = cache.RedisConfig{
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
	}

	return cache.NewCache(cacheCfg)
}
