package alchemywebhook

import (
	"context"
)

// Backfill handles historical transaction backfill
type Backfill interface {
	// Backfill performs backfill for the given addresses
	Backfill(ctx context.Context, addresses []string) error
}

// NoOpBackfill is a no-op implementation when backfill is disabled
type NoOpBackfill struct{}

// NewNoOpBackfill creates a new no-op backfill
func NewNoOpBackfill() *NoOpBackfill {
	return &NoOpBackfill{}
}

func (b *NoOpBackfill) Backfill(ctx context.Context, addresses []string) error {
	return nil
}
