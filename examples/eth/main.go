package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	alchemywebhook "github.com/dawitel/alchemy-webhook"
	"github.com/dawitel/alchemy-webhook/eth"
	"github.com/rs/zerolog"
)

func main() {
	// Initialize logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Create configuration
	cfg, err := alchemywebhook.NewEthereumConfig().
		WithAPIKey(os.Getenv("ALCHEMY_API_KEY")).
		WithWebhookURL("https://your-app.com/webhook").
		WithSignatureSecret(os.Getenv("WEBHOOK_SECRET")).
		WithCache(alchemywebhook.CacheConfig{
			Enabled: true,
			Type:    "memory",
			Memory: alchemywebhook.MemoryConfig{
				MaxSize:         10000,
				CleanupInterval: 1 * 60 * 60 * 1000000000, // 1 hour
				EnableLRU:       false,
			},
		}).
		WithBackfill(alchemywebhook.BackfillConfig{
			Enabled:   true,
			TimeRange: 12 * 60 * 60 * 1000000000, // 12 hours
			RPCURL:    os.Getenv("ETH_RPC_URL"),
			BatchSize: 100,
		}).
		Build()

	if err != nil {
		log.Fatal("Failed to create config:", err)
	}

	// Create client
	client, err := alchemywebhook.NewEthereumClient(cfg, logger)
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Set up activity handler
	processor := eth.NewProcessor(
		logger,
		client.GetCache(),
		map[string]string{}, // token addresses
		func(ctx context.Context, activity eth.ProcessedActivity) error {
			logger.Info().
				Str("tx_hash", activity.TxHash).
				Str("from", activity.FromAddress).
				Str("to", activity.ToAddress).
				Str("currency", activity.Currency).
				Str("value", activity.Value).
				Msg("Processed Ethereum activity")
			// Handle the activity (e.g., send to Kafka, update database, etc.)
			return nil
		},
		"eth-mainnet",
	)
	client.SetEthereumProcessor(processor)

	// Start client
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		log.Fatal("Failed to start client:", err)
	}
	defer client.Stop()

	// Set up HTTP server
	http.HandleFunc("/webhook", client.HandleWebhook())

	// Start server
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal("Failed to start server:", err)
		}
	}()

	logger.Info().Msg("Ethereum webhook server started on :8080")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info().Msg("Shutting down...")
}
