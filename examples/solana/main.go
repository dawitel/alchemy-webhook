package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	alchemywebhook "github.com/dawitel/alchemy-webhook"
	"github.com/dawitel/alchemy-webhook/solana"
	"github.com/rs/zerolog"
)

func main() {
	// Initialize logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Create configuration
	cfg, err := alchemywebhook.NewSolanaConfig().
		WithAPIKey(os.Getenv("ALCHEMY_API_KEY")).
		WithWebhookURL("https://your-app.com/webhook").
		WithSignatureSecret(os.Getenv("WEBHOOK_SECRET")).
		WithCache(alchemywebhook.CacheConfig{
			Enabled: true,
			Type:    "redis",
			Redis: alchemywebhook.RedisConfig{
				Address:  "localhost:6379",
				Password: "",
				DB:       0,
			},
		}).
		WithBackfill(alchemywebhook.BackfillConfig{
			Enabled:      true,
			TimeRange:    72 * 60 * 60 * 1000000000, // 72 hours
			HeliusAPIKey: os.Getenv("HELIUS_API_KEY"),
			HeliusURL:    "https://mainnet.helius-rpc.com",
			BatchSize:    100,
		}).
		Build()

	if err != nil {
		log.Fatal("Failed to create config:", err)
	}

	// Create client
	client, err := alchemywebhook.NewSolanaClient(cfg, logger)
	if err != nil {
		log.Fatal("Failed to create client:", err)
	}

	// Set up transaction handler
	processor := solana.NewProcessor(
		logger,
		client.GetCache(),
		map[string]string{
			"USDC": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			"USDT": "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
		}, // token mints
		func(ctx context.Context, tx solana.ProcessedTransaction) error {
			logger.Info().
				Str("signature", tx.Signature).
				Int("native_transfers", len(tx.NativeTransfers)).
				Int("token_transfers", len(tx.TokenTransfers)).
				Msg("Processed Solana transaction")
			// Handle the transaction (e.g., send to Kafka, update database, etc.)
			return nil
		},
		"sol-mainnet",
	)
	client.SetSolanaProcessor(processor)

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

	logger.Info().Msg("Solana webhook server started on :8080")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info().Msg("Shutting down...")
}
