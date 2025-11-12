package alchemywebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dawitel/alchemy-webhook/eth"
	"github.com/dawitel/alchemy-webhook/solana"
	"github.com/rs/zerolog"
)

// EthereumProcessor interface for processing Ethereum activities
type EthereumProcessor interface {
	ProcessActivity(ctx context.Context, activity eth.AlchemyActivity) error
}

// SolanaProcessor interface for processing Solana transactions
type SolanaProcessor interface {
	ProcessTransaction(ctx context.Context, tx solana.AlchemySolanaTransaction, slot uint64) error
}

// Handler handles HTTP webhook requests
type Handler struct {
	verifier     *Verifier
	ethProcessor EthereumProcessor
	solProcessor SolanaProcessor
	logger       zerolog.Logger
	maxBodySize  int64
	chainType    string
}

// NewEthereumHandler creates a new handler for Ethereum webhooks
func NewEthereumHandler(
	verifier *Verifier,
	processor EthereumProcessor,
	logger zerolog.Logger,
	maxBodySize int64,
) *Handler {
	return &Handler{
		verifier:     verifier,
		ethProcessor: processor,
		logger:       logger,
		maxBodySize:  maxBodySize,
		chainType:    "ethereum",
	}
}

// NewSolanaHandler creates a new handler for Solana webhooks
func NewSolanaHandler(
	verifier *Verifier,
	processor SolanaProcessor,
	logger zerolog.Logger,
	maxBodySize int64,
) *Handler {
	return &Handler{
		verifier:     verifier,
		solProcessor: processor,
		logger:       logger,
		maxBodySize:  maxBodySize,
		chainType:    "solana",
	}
}

// HandleWebhook handles incoming webhook requests
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.Error().
				Interface("panic", rec).
				Msg("Panic recovered in webhook handler")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitedBody := http.MaxBytesReader(w, r.Body, h.maxBodySize)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		if err.Error() == "http: request body too large" {
			h.logger.Warn().
				Int64("max_size", h.maxBodySize).
				Msg("Webhook request body exceeds maximum size")
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		h.logger.Error().Err(err).Msg("Failed to read webhook body")
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		h.logger.Warn().Msg("Empty webhook body received")
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("X-Alchemy-Signature")
	if err := h.verifier.Verify(body, signature); err != nil {
		h.logger.Warn().Err(err).Msg("Invalid webhook signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	switch h.chainType {
	case "ethereum":
		if err := h.handleEthereumWebhook(r.Context(), body); err != nil {
			h.logger.Error().Err(err).Msg("Failed to process Ethereum webhook")
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}
	case "solana":
		if err := h.handleSolanaWebhook(r.Context(), body); err != nil {
			h.logger.Error().Err(err).Msg("Failed to process Solana webhook")
			http.Error(w, "Failed to process webhook", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleEthereumWebhook processes Ethereum webhook payload
func (h *Handler) handleEthereumWebhook(ctx context.Context, body []byte) error {
	if h.ethProcessor == nil {
		return fmt.Errorf("Ethereum processor not configured")
	}

	var payload eth.AlchemyWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if len(payload.Event.Activity) == 0 {
		h.logger.Debug().Msg("Webhook received with no activities")
		return nil
	}

	h.logger.Debug().
		Int("activity_count", len(payload.Event.Activity)).
		Msg("Processing Ethereum webhook activities")

	for _, activity := range payload.Event.Activity {
		if err := h.ethProcessor.ProcessActivity(ctx, activity); err != nil {
			h.logger.Error().Err(err).
				Str("hash", activity.Hash).
				Msg("Failed to process activity")
		}
	}

	return nil
}

// handleSolanaWebhook processes Solana webhook payload
func (h *Handler) handleSolanaWebhook(ctx context.Context, body []byte) error {
	if h.solProcessor == nil {
		return fmt.Errorf("Solana processor not configured")
	}

	var payload solana.AlchemySolanaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if len(payload.Event.Transaction) == 0 {
		h.logger.Debug().Msg("Webhook received with no transactions")
		return nil
	}

	h.logger.Debug().
		Int("transaction_count", len(payload.Event.Transaction)).
		Msg("Processing Solana webhook transactions")

	for _, tx := range payload.Event.Transaction {
		if err := h.solProcessor.ProcessTransaction(ctx, tx, payload.Event.Slot); err != nil {
			h.logger.Error().Err(err).
				Str("signature", tx.Signature).
				Msg("Failed to process transaction")
		}
	}

	return nil
}
