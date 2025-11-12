package alchemywebhook

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dawitel/alchemy-webhook/cache"
	"github.com/dawitel/alchemy-webhook/eth"
	"github.com/dawitel/alchemy-webhook/solana"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
)

// Client is the main SDK client interface
type Client interface {
	// Start initializes and starts the client
	Start(ctx context.Context) error

	// Stop gracefully stops the client
	Stop() error

	// Health returns the health status
	Health() error

	// HandleWebhook returns the HTTP handler for webhook endpoints
	HandleWebhook() http.HandlerFunc

	// CreateWebhook creates a new webhook
	CreateWebhook(ctx context.Context, name string) (string, error)

	// UpdateWebhook updates webhook addresses
	UpdateWebhook(ctx context.Context, webhookID string, addressesToAdd, addressesToRemove []string) error

	// ListWebhooks lists all webhooks
	ListWebhooks(ctx context.Context) ([]WebhookInfo, error)

	// GetWebhookAddresses gets addresses for a webhook
	GetWebhookAddresses(ctx context.Context, webhookID string) ([]string, error)

	// Backfill triggers manual backfill (returns error if disabled)
	Backfill(ctx context.Context, addresses []string) error

	// AddAddresses adds addresses to webhook
	AddAddresses(ctx context.Context, webhookID string, addresses []string) error

	// RemoveAddresses removes addresses from webhook
	RemoveAddresses(ctx context.Context, webhookID string, addresses []string) error
}

// BaseClient is the base implementation of Client
type BaseClient struct {
	cfg            *Config
	logger         zerolog.Logger
	webhookManager *WebhookManager
	handler        *Handler
	backfill       Backfill
	cache          cache.Cache
	mu             sync.RWMutex
	started        bool
	ctx            context.Context
	cancel         context.CancelFunc
}

// EthereumClient is the Ethereum-specific client
type EthereumClient struct {
	*BaseClient
	Processor *eth.Processor
	rpcClient *ethclient.Client
}

// SolanaClient is the Solana-specific client
type SolanaClient struct {
	*BaseClient
	Processor *solana.Processor
}

// NewEthereumClient creates a new Ethereum client
func NewEthereumClient(cfg *Config, logger zerolog.Logger) (*EthereumClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cacheInstance, err := newCache(cfg.Cache)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	var rpcClient *ethclient.Client
	if cfg.Backfill.Enabled && cfg.Backfill.RPCURL != "" {
		client, err := ethclient.Dial(cfg.Backfill.RPCURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Ethereum RPC: %w", err)
		}
		rpcClient = client
	}

	processor := eth.NewProcessor(
		logger,
		cacheInstance,
		map[string]string{},
		nil,
		"eth-mainnet",
	)

	network := "ETH_MAINNET"
	webhookManager := NewWebhookManager(cfg, logger, network)
	verifier := NewVerifier(cfg.SignatureSecret)
	handler := NewEthereumHandler(verifier, processor, logger, cfg.HTTPClient.MaxRequestBodySize)
	var backfill Backfill = NewNoOpBackfill()
	if cfg.Backfill.Enabled && rpcClient != nil {
		ethBackfill := eth.NewBackfill(
			rpcClient,
			processor,
			logger,
			cacheInstance,
			cfg.Backfill.TimeRange,
			cfg.Backfill.BatchSize,
		)
		backfill = ethBackfill
	}

	baseClient := &BaseClient{
		cfg:            cfg,
		logger:         logger,
		webhookManager: webhookManager,
		handler:        handler,
		backfill:       backfill,
		cache:          cacheInstance,
	}

	return &EthereumClient{
		BaseClient: baseClient,
		Processor:  processor,
		rpcClient:  rpcClient,
	}, nil
}

// NewSolanaClient creates a new Solana client
func NewSolanaClient(cfg *Config, logger zerolog.Logger) (*SolanaClient, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cacheInstance, err := newCache(cfg.Cache)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	processor := solana.NewProcessor(
		logger,
		cacheInstance,
		map[string]string{},
		nil,
		"sol-mainnet",
	)

	network := "SOLANA_MAINNET"
	webhookManager := NewWebhookManager(cfg, logger, network)
	verifier := NewVerifier(cfg.SignatureSecret)
	handler := NewSolanaHandler(verifier, processor, logger, cfg.HTTPClient.MaxRequestBodySize)
	var backfill Backfill = NewNoOpBackfill()
	if cfg.Backfill.Enabled && cfg.Backfill.HeliusAPIKey != "" {
		httpClient := &http.Client{Timeout: cfg.HTTPClient.Timeout}
		heliusURL := cfg.Backfill.HeliusURL
		if heliusURL == "" {
			heliusURL = "https://mainnet.helius-rpc.com"
		}
		solBackfill := solana.NewBackfill(
			cfg.Backfill.HeliusAPIKey,
			heliusURL,
			processor,
			logger,
			cacheInstance,
			cfg.Backfill.TimeRange,
			cfg.Backfill.BatchSize,
			httpClient,
		)
		backfill = solBackfill
	}

	baseClient := &BaseClient{
		cfg:            cfg,
		logger:         logger,
		webhookManager: webhookManager,
		handler:        handler,
		backfill:       backfill,
		cache:          cacheInstance,
	}

	return &SolanaClient{
		BaseClient: baseClient,
		Processor:  processor,
	}, nil
}

// Start initializes and starts the client
func (c *BaseClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("client already started")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.started = true

	c.logger.Info().Msg("Alchemy webhook SDK client started")

	if c.cfg.Backfill.Enabled && c.cfg.Backfill.StartDelay > 0 {
		go func() {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.cfg.Backfill.StartDelay):
				webhooks, err := c.webhookManager.ListWebhooks(c.ctx)
				if err == nil {
					for _, webhook := range webhooks {
						addresses, err := c.webhookManager.GetWebhookAddresses(c.ctx, webhook.ID)
						if err == nil && len(addresses) > 0 {
							if err := c.backfill.Backfill(c.ctx, addresses); err != nil {
								c.logger.Warn().Err(err).Msg("Backfill failed")
							}
						}
					}
				}
			}
		}()
	}

	return nil
}

// Stop gracefully stops the client
func (c *BaseClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.cache != nil {
		if err := c.cache.Close(); err != nil {
			c.logger.Warn().Err(err).Msg("Failed to close cache")
		}
	}

	c.started = false
	c.logger.Info().Msg("Alchemy webhook SDK client stopped")

	return nil
}

// Health returns the health status
func (c *BaseClient) Health() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started {
		return fmt.Errorf("client not started")
	}

	return nil
}

// HandleWebhook returns the HTTP handler for webhook endpoints
func (c *BaseClient) HandleWebhook() http.HandlerFunc {
	return c.handler.HandleWebhook
}

// CreateWebhook creates a new webhook
func (c *BaseClient) CreateWebhook(ctx context.Context, name string) (string, error) {
	return c.webhookManager.CreateWebhook(ctx, name)
}

// UpdateWebhook updates webhook addresses
func (c *BaseClient) UpdateWebhook(ctx context.Context, webhookID string, addressesToAdd, addressesToRemove []string) error {
	return c.webhookManager.UpdateWebhookAddresses(ctx, webhookID, addressesToAdd, addressesToRemove)
}

// ListWebhooks lists all webhooks
func (c *BaseClient) ListWebhooks(ctx context.Context) ([]WebhookInfo, error) {
	return c.webhookManager.ListWebhooks(ctx)
}

// GetWebhookAddresses gets addresses for a webhook
func (c *BaseClient) GetWebhookAddresses(ctx context.Context, webhookID string) ([]string, error) {
	return c.webhookManager.GetWebhookAddresses(ctx, webhookID)
}

// Backfill triggers manual backfill
func (c *BaseClient) Backfill(ctx context.Context, addresses []string) error {
	if !c.cfg.Backfill.Enabled {
		return fmt.Errorf("backfill is disabled")
	}
	return c.backfill.Backfill(ctx, addresses)
}

// AddAddresses adds addresses to webhook
func (c *BaseClient) AddAddresses(ctx context.Context, webhookID string, addresses []string) error {
	return c.webhookManager.UpdateWebhookAddresses(ctx, webhookID, addresses, nil)
}

// RemoveAddresses removes addresses from webhook
func (c *BaseClient) RemoveAddresses(ctx context.Context, webhookID string, addresses []string) error {
	return c.webhookManager.UpdateWebhookAddresses(ctx, webhookID, nil, addresses)
}

// GetCache returns the cache instance
func (c *BaseClient) GetCache() cache.Cache {
	return c.cache
}

// SetEthereumProcessor updates the Ethereum processor and handler
func (ec *EthereumClient) SetEthereumProcessor(processor *eth.Processor) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.Processor = processor
	verifier := NewVerifier(ec.cfg.SignatureSecret)
	ec.handler = NewEthereumHandler(verifier, processor, ec.logger, ec.cfg.HTTPClient.MaxRequestBodySize)
}

// SetSolanaProcessor updates the Solana processor and handler
func (sc *SolanaClient) SetSolanaProcessor(processor *solana.Processor) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.Processor = processor
	verifier := NewVerifier(sc.cfg.SignatureSecret)
	sc.handler = NewSolanaHandler(verifier, processor, sc.logger, sc.cfg.HTTPClient.MaxRequestBodySize)
}
