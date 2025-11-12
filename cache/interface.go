package cache

import (
	"context"
	"time"
)

// Cache defines the interface for transaction deduplication cache
type Cache interface {
	// IsProcessed checks if a transaction has been processed
	IsProcessed(ctx context.Context, txHash string) (bool, error)

	// MarkProcessed marks a transaction as processed
	MarkProcessed(ctx context.Context, txHash string, ttl time.Duration) error

	// Close closes the cache and releases resources
	Close() error
}

