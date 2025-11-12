package cache

import (
	"context"
	"time"
)

// NoOpCache is a no-op cache implementation used when caching is disabled.
type NoOpCache struct{}

// NewNoOpCache creates a new no-op cache instance.
func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

// IsProcessed returns false indicating the transaction has not been processed.
func (c *NoOpCache) IsProcessed(ctx context.Context, txHash string) (bool, error) {
	return false, nil
}

// MarkProcessed is a no-op that does not persist any state.
func (c *NoOpCache) MarkProcessed(ctx context.Context, txHash string, ttl time.Duration) error {
	return nil
}

// Close is a no-op that does not release any resources.
func (c *NoOpCache) Close() error {
	return nil
}

