package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache is an in-memory cache implementation
type MemoryCache struct {
	mu       sync.RWMutex
	entries  map[string]time.Time
	maxSize  int
	cleanup  *time.Ticker
	stop     chan struct{}
	enableLRU bool
	accessOrder []string // For LRU eviction
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int, cleanupInterval time.Duration, enableLRU bool) *MemoryCache {
	cache := &MemoryCache{
		entries:    make(map[string]time.Time),
		maxSize:    maxSize,
		cleanup:    time.NewTicker(cleanupInterval),
		stop:       make(chan struct{}),
		enableLRU:  enableLRU,
		accessOrder: make([]string, 0, maxSize),
	}

	go cache.cleanupExpired()

	return cache
}

// IsProcessed checks if a transaction has been processed
func (c *MemoryCache) IsProcessed(ctx context.Context, txHash string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	expiresAt, exists := c.entries[txHash]
	if !exists {
		return false, nil
	}

	if time.Now().After(expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, txHash)
		if c.enableLRU {
			c.removeFromAccessOrder(txHash)
		}
		c.mu.Unlock()
		c.mu.RLock()
		return false, nil
	}

	if c.enableLRU {
		c.mu.RUnlock()
		c.mu.Lock()
		c.updateAccessOrder(txHash)
		c.mu.Unlock()
		c.mu.RLock()
	}

	return true, nil
}

// MarkProcessed marks a transaction as processed
func (c *MemoryCache) MarkProcessed(ctx context.Context, txHash string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		if c.enableLRU {
			if len(c.accessOrder) > 0 {
				oldest := c.accessOrder[0]
				delete(c.entries, oldest)
				c.accessOrder = c.accessOrder[1:]
			}
		} else {
			now := time.Now()
			evicted := false
			for key, expiresAt := range c.entries {
				if now.After(expiresAt) {
					delete(c.entries, key)
					evicted = true
					break
				}
			}
			if !evicted && len(c.entries) > 0 {
				for key := range c.entries {
					delete(c.entries, key)
					break
				}
			}
		}
	}

	expiresAt := time.Now().Add(ttl)
	c.entries[txHash] = expiresAt

	if c.enableLRU {
		c.updateAccessOrder(txHash)
	}

	return nil
}

// Close closes the cache and releases resources
func (c *MemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanup.Stop()
	close(c.stop)
	c.entries = nil
	c.accessOrder = nil

	return nil
}

// cleanupExpired periodically removes expired entries
func (c *MemoryCache) cleanupExpired() {
	for {
		select {
		case <-c.cleanup.C:
			c.mu.Lock()
			now := time.Now()
			for key, expiresAt := range c.entries {
				if now.After(expiresAt) {
					delete(c.entries, key)
					if c.enableLRU {
						c.removeFromAccessOrder(key)
					}
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}

// updateAccessOrder updates the access order for LRU
func (c *MemoryCache) updateAccessOrder(txHash string) {
	c.removeFromAccessOrder(txHash)
	c.accessOrder = append(c.accessOrder, txHash)
}

// removeFromAccessOrder removes an entry from access order
func (c *MemoryCache) removeFromAccessOrder(txHash string) {
	for i, key := range c.accessOrder {
		if key == txHash {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			return
		}
	}
}

