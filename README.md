# Alchemy Webhook SDK

A robust, scalable SDK for handling Alchemy webhooks for both Ethereum and Solana blockchains.

## Features

- **Multi-chain support**: Ethereum and Solana
- **Configurable caching**: Redis or in-memory (optional)
- **Backfill support**: Historical transaction detection (optional)
- **Webhook management**: Create, update, and manage webhooks
- **Signature verification**: HMAC-SHA256 verification
- **Resilience**: Circuit breakers, retry strategies, and error handling
- **Scalable**: Concurrent processing, connection pooling, pagination

## Installation

```bash
go get github.com/dawitel/alchemy-webhook
```

## Quick Start

### Ethereum Example

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"

    "github.com/rs/zerolog"
    alchemywebhook "github.com/dawitel/alchemy-webhook"
    "github.com/dawitel/alchemy-webhook/eth"
)

func main() {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    cfg, err := alchemywebhook.NewEthereumConfig().
        WithAPIKey(os.Getenv("ALCHEMY_API_KEY")).
        WithWebhookURL("https://your-app.com/webhook").
        WithSignatureSecret(os.Getenv("WEBHOOK_SECRET")).
        Build()

    if err != nil {
        log.Fatal(err)
    }

    client, err := alchemywebhook.NewEthereumClient(cfg, logger)
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    if err := client.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer client.Stop()

    http.HandleFunc("/webhook", client.HandleWebhook())
    http.ListenAndServe(":8080", nil)
}
```

### Solana Example

```go
cfg, err := alchemywebhook.NewSolanaConfig().
    WithAPIKey(os.Getenv("ALCHEMY_API_KEY")).
    WithWebhookURL("https://your-app.com/webhook").
    WithSignatureSecret(os.Getenv("WEBHOOK_SECRET")).
    Build()

client, err := alchemywebhook.NewSolanaClient(cfg, logger)
```

## Configuration

### Basic Configuration

```go
cfg := alchemywebhook.NewEthereumConfig().
    WithAPIKey("your-api-key").
    WithWebhookURL("https://your-app.com/webhook").
    WithSignatureSecret("your-secret").
    Build()
```

### With Caching

```go
cfg := alchemywebhook.NewEthereumConfig().
    WithAPIKey("your-api-key").
    WithWebhookURL("https://your-app.com/webhook").
    WithSignatureSecret("your-secret").
    WithCache(alchemywebhook.CacheConfig{
        Enabled: true,
        Type:    "redis", // or "memory"
        Redis: alchemywebhook.RedisConfig{
            Address:  "localhost:6379",
            Password: "",
            DB:       0,
        },
    }).
    Build()
```

### With Backfill

```go
cfg := alchemywebhook.NewEthereumConfig().
    WithAPIKey("your-api-key").
    WithWebhookURL("https://your-app.com/webhook").
    WithSignatureSecret("your-secret").
    WithBackfill(alchemywebhook.BackfillConfig{
        Enabled:   true,
        TimeRange: 12 * time.Hour,
        RPCURL:    "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY",
        BatchSize: 100,
    }).
    Build()
```

## API Reference

### Client Interface

```go
type Client interface {
    Start(ctx context.Context) error
    Stop() error
    Health() error
    HandleWebhook() http.HandlerFunc
    CreateWebhook(ctx context.Context, name string) (string, error)
    UpdateWebhook(ctx context.Context, webhookID string, addressesToAdd, addressesToRemove []string) error
    ListWebhooks(ctx context.Context) ([]WebhookInfo, error)
    GetWebhookAddresses(ctx context.Context, webhookID string) ([]string, error)
    Backfill(ctx context.Context, addresses []string) error
    AddAddresses(ctx context.Context, webhookID string, addresses []string) error
    RemoveAddresses(ctx context.Context, webhookID string, addresses []string) error
}
```

### Configuration Options

#### Cache Configuration

- `Enabled`: Enable/disable caching (default: false)
- `Type`: Cache type - "redis" or "memory" (default: "memory")
- `Redis`: Redis configuration (address, password, DB, pool size, TLS)
- `Memory`: Memory cache configuration (max size, cleanup interval, LRU)
- `DefaultTTL`: Default TTL for cached entries

#### Backfill Configuration

- `Enabled`: Enable/disable backfill (default: false)
- `TimeRange`: Time range for backfill (default: 12h for ETH, 72h for SOL)
- `RPCURL`: Ethereum RPC URL (required for Ethereum backfill)
- `HeliusAPIKey`: Helius API key (required for Solana backfill)
- `HeliusURL`: Helius API URL (default: https://mainnet.helius-rpc.com)
- `BatchSize`: Batch size for processing (default: 100)
- `StartDelay`: Delay before starting backfill on startup

#### Circuit Breaker Configuration

- `MaxRequests`: Maximum requests before evaluating threshold
- `Interval`: Time window for request counting
- `Timeout`: Timeout for circuit breaker state
- `Threshold`: Failure ratio threshold (0.0-1.0)

#### Retry Configuration

- `InitialDelay`: Initial retry delay
- `MaxDelay`: Maximum retry delay
- `MaxAttempts`: Maximum retry attempts
- `Multiplier`: Backoff multiplier

## Webhook Management

### Create Webhook

```go
webhookID, err := client.CreateWebhook(ctx, "my-webhook")
```

### Add Addresses

```go
err := client.AddAddresses(ctx, webhookID, []string{
    "0x1234...",
    "0x5678...",
})
```

### Remove Addresses

```go
err := client.RemoveAddresses(ctx, webhookID, []string{
    "0x1234...",
})
```

### List Webhooks

```go
webhooks, err := client.ListWebhooks(ctx)
for _, webhook := range webhooks {
    fmt.Printf("Webhook %s: %d addresses\n", webhook.ID, webhook.AddressCount)
}
```

## Processing Transactions

### Ethereum

The SDK processes the following Ethereum transaction types:
- External ETH transfers
- Internal ETH transfers
- ERC-20 token transfers
- ERC-721 NFT transfers
- ERC-1155 NFT transfers

### Solana

The SDK processes the following Solana transaction types:
- Native SOL transfers
- SPL token transfers

## Error Handling

The SDK includes comprehensive error handling:
- Circuit breakers for external API calls
- Retry strategies with exponential backoff
- Graceful degradation
- Structured logging

## Examples

See the `examples/` directory for complete working examples:
- `eth_example.go`: Ethereum webhook handler
- `solana_example.go`: Solana webhook handler

## License

See LICENSE file for details.
