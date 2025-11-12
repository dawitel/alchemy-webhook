package alchemywebhook

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"
)

const (
	// Default values
	DefaultAlchemyNotifyURL       = "https://dashboard.alchemy.com/api"
	DefaultMaxRequestBodySize     = 10 * 1024 * 1024 // 10MB
	DefaultMaxAddressesPerWebhook = 100000
	DefaultUpdateInterval         = 30 * time.Second
	DefaultCacheTTL               = 24 * time.Hour
	DefaultBackfillTimeRangeETH   = 12 * time.Hour
	DefaultBackfillTimeRangeSOL   = 72 * time.Hour
	DefaultBackfillBatchSize      = 100
	DefaultBackfillStartDelay     = 30 * time.Second

	// Circuit breaker defaults
	DefaultCircuitBreakerMaxRequests = 5
	DefaultCircuitBreakerInterval    = 60 * time.Second
	DefaultCircuitBreakerTimeout     = 30 * time.Second
	DefaultCircuitBreakerThreshold   = 0.7

	// Retry defaults
	DefaultRetryInitialDelay = 1 * time.Second
	DefaultRetryMaxDelay     = 30 * time.Second
	DefaultRetryMaxAttempts  = 3
	DefaultRetryMultiplier   = 2.0

	// HTTP client defaults
	DefaultHTTPTimeout = 30 * time.Second

	// Redis defaults
	DefaultRedisPoolSize     = 10
	DefaultRedisMinIdleConns = 5
	DefaultRedisDialTimeout  = 5 * time.Second
	DefaultRedisReadTimeout  = 3 * time.Second
	DefaultRedisWriteTimeout = 3 * time.Second

	// Memory cache defaults
	DefaultMemoryCacheMaxSize         = 10000
	DefaultMemoryCacheCleanupInterval = 1 * time.Hour
)

// Config represents the main configuration for the SDK
type Config struct {
	AlchemyAPIKey    string
	AlchemyAuthToken string
	AlchemyNotifyURL string

	WebhookURL      string
	SignatureSecret string

	Cache CacheConfig

	Backfill BackfillConfig

	CircuitBreaker CircuitBreakerConfig

	Retry RetryConfig

	AddressManagement AddressManagementConfig

	HTTPClient HTTPClientConfig

	Logging LoggingConfig
}

// CacheConfig configures transaction caching
type CacheConfig struct {
	Enabled    bool
	Type       string // "redis" or "memory"
	Redis      RedisConfig
	Memory     MemoryConfig
	DefaultTTL time.Duration
}

// RedisConfig configures Redis connection
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
	TLSConfig     *tls.Config
}

// MemoryConfig configures in-memory cache
type MemoryConfig struct {
	MaxSize         int
	CleanupInterval time.Duration
	EnableLRU       bool
}

// BackfillConfig configures backfill functionality
type BackfillConfig struct {
	Enabled      bool
	TimeRange    time.Duration
	BatchSize    int
	RPCURL       string // For Ethereum
	HeliusAPIKey string // For Solana
	HeliusURL    string // For Solana
	StartDelay   time.Duration
}

// CircuitBreakerConfig configures circuit breaker
type CircuitBreakerConfig struct {
	MaxRequests int
	Interval    time.Duration
	Timeout     time.Duration
	Threshold   float64 // Failure ratio threshold (0.0-1.0)
}

// RetryConfig configures retry strategy
type RetryConfig struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	MaxAttempts  int
	Multiplier   float64
}

// AddressManagementConfig configures address management
type AddressManagementConfig struct {
	MaxAddressesPerWebhook int
	UpdateInterval         time.Duration
}

// HTTPClientConfig configures HTTP client
type HTTPClientConfig struct {
	Timeout            time.Duration
	MaxRequestBodySize int64
}

// LoggingConfig configures logging
type LoggingConfig struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "json", "console"
}

// ConfigBuilder provides a fluent interface for building Config
type ConfigBuilder struct {
	config *Config
}

// NewConfig creates a new ConfigBuilder with defaults
func NewConfig() *ConfigBuilder {
	return &ConfigBuilder{
		config: &Config{
			AlchemyNotifyURL: DefaultAlchemyNotifyURL,
			Cache: CacheConfig{
				Enabled:    false,
				Type:       "memory",
				DefaultTTL: DefaultCacheTTL,
				Redis: RedisConfig{
					PoolSize:     DefaultRedisPoolSize,
					MinIdleConns: DefaultRedisMinIdleConns,
					DialTimeout:  DefaultRedisDialTimeout,
					ReadTimeout:  DefaultRedisReadTimeout,
					WriteTimeout: DefaultRedisWriteTimeout,
				},
				Memory: MemoryConfig{
					MaxSize:         DefaultMemoryCacheMaxSize,
					CleanupInterval: DefaultMemoryCacheCleanupInterval,
					EnableLRU:       false,
				},
			},
			Backfill: BackfillConfig{
				Enabled:    false,
				BatchSize:  DefaultBackfillBatchSize,
				StartDelay: DefaultBackfillStartDelay,
			},
			CircuitBreaker: CircuitBreakerConfig{
				MaxRequests: DefaultCircuitBreakerMaxRequests,
				Interval:    DefaultCircuitBreakerInterval,
				Timeout:     DefaultCircuitBreakerTimeout,
				Threshold:   DefaultCircuitBreakerThreshold,
			},
			Retry: RetryConfig{
				InitialDelay: DefaultRetryInitialDelay,
				MaxDelay:     DefaultRetryMaxDelay,
				MaxAttempts:  DefaultRetryMaxAttempts,
				Multiplier:   DefaultRetryMultiplier,
			},
			AddressManagement: AddressManagementConfig{
				MaxAddressesPerWebhook: DefaultMaxAddressesPerWebhook,
				UpdateInterval:         DefaultUpdateInterval,
			},
			HTTPClient: HTTPClientConfig{
				Timeout:            DefaultHTTPTimeout,
				MaxRequestBodySize: DefaultMaxRequestBodySize,
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
		},
	}
}

// WithAPIKey sets the Alchemy API key
func (b *ConfigBuilder) WithAPIKey(key string) *ConfigBuilder {
	b.config.AlchemyAPIKey = key
	return b
}

// WithAuthToken sets the Alchemy auth token
func (b *ConfigBuilder) WithAuthToken(token string) *ConfigBuilder {
	b.config.AlchemyAuthToken = token
	return b
}

// WithNotifyURL sets the Alchemy notify URL
func (b *ConfigBuilder) WithNotifyURL(url string) *ConfigBuilder {
	b.config.AlchemyNotifyURL = url
	return b
}

// WithWebhookURL sets the webhook URL
func (b *ConfigBuilder) WithWebhookURL(url string) *ConfigBuilder {
	b.config.WebhookURL = url
	return b
}

// WithSignatureSecret sets the signature secret
func (b *ConfigBuilder) WithSignatureSecret(secret string) *ConfigBuilder {
	b.config.SignatureSecret = secret
	return b
}

// WithCache sets the cache configuration
func (b *ConfigBuilder) WithCache(cache CacheConfig) *ConfigBuilder {
	b.config.Cache = cache
	return b
}

// WithBackfill sets the backfill configuration
func (b *ConfigBuilder) WithBackfill(backfill BackfillConfig) *ConfigBuilder {
	b.config.Backfill = backfill
	return b
}

// WithCircuitBreaker sets the circuit breaker configuration
func (b *ConfigBuilder) WithCircuitBreaker(cb CircuitBreakerConfig) *ConfigBuilder {
	b.config.CircuitBreaker = cb
	return b
}

// WithRetry sets the retry configuration
func (b *ConfigBuilder) WithRetry(retry RetryConfig) *ConfigBuilder {
	b.config.Retry = retry
	return b
}

// WithAddressManagement sets the address management configuration
func (b *ConfigBuilder) WithAddressManagement(am AddressManagementConfig) *ConfigBuilder {
	b.config.AddressManagement = am
	return b
}

// WithHTTPClient sets the HTTP client configuration
func (b *ConfigBuilder) WithHTTPClient(hc HTTPClientConfig) *ConfigBuilder {
	b.config.HTTPClient = hc
	return b
}

// WithLogging sets the logging configuration
func (b *ConfigBuilder) WithLogging(logging LoggingConfig) *ConfigBuilder {
	b.config.Logging = logging
	return b
}

// Build validates and returns the Config
func (b *ConfigBuilder) Build() (*Config, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}
	return b.config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.AlchemyAPIKey == "" && c.AlchemyAuthToken == "" {
		return errors.New("either AlchemyAPIKey or AlchemyAuthToken must be set")
	}

	if c.WebhookURL == "" {
		return errors.New("WebhookURL is required")
	}

	if c.SignatureSecret == "" {
		return errors.New("SignatureSecret is required")
	}

	if c.Cache.Enabled {
		if c.Cache.Type != "redis" && c.Cache.Type != "memory" {
			return fmt.Errorf("invalid cache type: %s (must be 'redis' or 'memory')", c.Cache.Type)
		}

		if c.Cache.Type == "redis" {
			if c.Cache.Redis.Address == "" {
				return errors.New("Redis address is required when using Redis cache")
			}
		}
	}

	if c.Backfill.Enabled {
		if c.Backfill.RPCURL == "" && c.Backfill.HeliusAPIKey == "" {
			return errors.New("either RPCURL (for Ethereum) or HeliusAPIKey (for Solana) must be set when backfill is enabled")
		}
	}

	if c.CircuitBreaker.Threshold < 0 || c.CircuitBreaker.Threshold > 1 {
		return errors.New("circuit breaker threshold must be between 0 and 1")
	}

	if c.Retry.Multiplier <= 0 {
		return errors.New("retry multiplier must be greater than 0")
	}

	if c.AddressManagement.MaxAddressesPerWebhook <= 0 {
		return errors.New("max addresses per webhook must be greater than 0")
	}

	return nil
}

// NewEthereumConfig creates a new ConfigBuilder with Ethereum defaults
func NewEthereumConfig() *ConfigBuilder {
	builder := NewConfig()
	builder.config.Backfill.TimeRange = DefaultBackfillTimeRangeETH
	return builder
}

// NewSolanaConfig creates a new ConfigBuilder with Solana defaults
func NewSolanaConfig() *ConfigBuilder {
	builder := NewConfig()
	builder.config.Backfill.TimeRange = DefaultBackfillTimeRangeSOL
	return builder
}
