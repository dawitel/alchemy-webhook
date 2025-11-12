package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/dawitel/alchemy-webhook/cache"
	"github.com/rs/zerolog"
)

// Backfill handles Solana historical transaction backfill
type Backfill struct {
	heliusAPIKey string
	heliusURL    string
	processor    *Processor
	logger       zerolog.Logger
	cache        cache.Cache
	timeRange    time.Duration
	batchSize    int
	httpClient   *http.Client
	backfilling  int32
}

// NewBackfill creates a new Solana backfill instance
func NewBackfill(
	heliusAPIKey string,
	heliusURL string,
	processor *Processor,
	logger zerolog.Logger,
	cache cache.Cache,
	timeRange time.Duration,
	batchSize int,
	httpClient *http.Client,
) *Backfill {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Backfill{
		heliusAPIKey: heliusAPIKey,
		heliusURL:    heliusURL,
		processor:    processor,
		logger:       logger,
		cache:        cache,
		timeRange:    timeRange,
		batchSize:    batchSize,
		httpClient:   httpClient,
	}
}

// Backfill performs backfill for the given addresses
func (b *Backfill) Backfill(ctx context.Context, addresses []string) error {
	if !atomic.CompareAndSwapInt32(&b.backfilling, 0, 1) {
		b.logger.Debug().Msg("Backfill already in progress, skipping")
		return nil
	}
	defer atomic.StoreInt32(&b.backfilling, 0)

	if b.heliusAPIKey == "" {
		return fmt.Errorf("Helius API key not configured")
	}

	if len(addresses) == 0 {
		b.logger.Debug().Msg("No addresses to backfill")
		return nil
	}

	b.logger.Info().
		Int("address_count", len(addresses)).
		Dur("time_range", b.timeRange).
		Msg("Starting Solana historical deposit backfill")

	toTime := time.Now().Unix()
	fromTime := toTime - int64(b.timeRange.Seconds())

	processedCount := 0
	skippedCount := 0

	for _, address := range addresses {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		transactions, err := b.getTransactionsForAddress(ctx, address, fromTime, toTime)
		if err != nil {
			b.logger.Warn().
				Err(err).
				Str("address", address).
				Msg("Failed to get transactions, skipping address")
			time.Sleep(2 * time.Second)
			continue
		}

		for _, tx := range transactions {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if b.cache != nil {
				processed, err := b.cache.IsProcessed(ctx, tx.Signature)
				if err == nil && processed {
					skippedCount++
					continue
				}
			}

			alchemyTx := b.convertToAlchemyTx(tx)
			if alchemyTx != nil {
				if err := b.processor.ProcessTransaction(ctx, *alchemyTx, uint64(tx.Slot)); err != nil {
					b.logger.Warn().
						Err(err).
						Str("signature", tx.Signature).
						Msg("Failed to process historical transaction")
					continue
				}
				processedCount++
			}
		}

		time.Sleep(1 * time.Second)
	}

	b.logger.Info().
		Int("processed", processedCount).
		Int("skipped", skippedCount).
		Int64("from_time", fromTime).
		Int64("to_time", toTime).
		Msg("Solana historical deposit backfill completed")

	return nil
}

// getTransactionsForAddress fetches transactions for an address using Helius RPC
func (b *Backfill) getTransactionsForAddress(ctx context.Context, address string, fromTime, toTime int64) ([]ProcessedTransaction, error) {
	url := fmt.Sprintf("%s?api-key=%s", b.heliusURL, b.heliusAPIKey)

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "getTransactionsForAddress",
		"params": []interface{}{
			address,
			map[string]interface{}{
				"transactionDetails": "full",
				"limit":              100,
				"sortOrder":          "desc",
				"commitment":         "finalized",
				"encoding":           "jsonParsed",
				"filters": map[string]interface{}{
					"blockTime": map[string]interface{}{
						"gte": fromTime,
						"lte": toTime,
					},
					"status": "succeeded",
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get transactions: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var rpcResp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      string `json:"id"`
		Result  *struct {
			Data            []map[string]interface{} `json:"data"`
			PaginationToken *string                  `json:"paginationToken"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s (code: %d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	if rpcResp.Result == nil {
		return nil, fmt.Errorf("empty result in RPC response")
	}

	var signatures []string
	for _, txData := range rpcResp.Result.Data {
		var blockTime int64
		var found bool

		if bt, ok := txData["blockTime"].(float64); ok {
			blockTime = int64(bt)
			found = true
		} else if bt, ok := txData["blockTime"].(int64); ok {
			blockTime = bt
			found = true
		} else if bt, ok := txData["blockTime"].(json.Number); ok {
			btInt, err := bt.Int64()
			if err != nil {
				continue
			}
			blockTime = btInt
			found = true
		}

		if !found || blockTime < fromTime || blockTime > toTime {
			continue
		}

		var sig string
		if transactionObj, ok := txData["transaction"].(map[string]interface{}); ok {
			if sigs, ok := transactionObj["signatures"].([]interface{}); ok && len(sigs) > 0 {
				if s, ok := sigs[0].(string); ok {
					sig = s
				}
			}
		} else if s, ok := txData["signature"].(string); ok {
			sig = s
		}

		if sig != "" {
			signatures = append(signatures, sig)
		}
	}

	if len(signatures) == 0 {
		return nil, nil
	}

	enhancedTxs, err := b.getEnhancedTransactions(ctx, signatures)
	if err != nil {
		return nil, fmt.Errorf("failed to get enhanced transactions: %w", err)
	}

	return enhancedTxs, nil
}

// getEnhancedTransactions fetches enhanced transaction details
func (b *Backfill) getEnhancedTransactions(ctx context.Context, signatures []string) ([]ProcessedTransaction, error) {
	if len(signatures) == 0 {
		return nil, nil
	}

	batchSize := 100
	var allTransactions []ProcessedTransaction

	for i := 0; i < len(signatures); i += batchSize {
		end := i + batchSize
		if end > len(signatures) {
			end = len(signatures)
		}
		batch := signatures[i:end]

		url := fmt.Sprintf("https://api-mainnet.helius-rpc.com/v0/transactions?api-key=%s", b.heliusAPIKey)
		reqBody := map[string]interface{}{
			"transactions": batch,
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to get enhanced transactions: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to get enhanced transactions: status %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		var transactions []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&transactions); err != nil {
			return nil, fmt.Errorf("failed to decode enhanced transactions response: %w", err)
		}

		for _, tx := range transactions {
			if sig, ok := tx["signature"].(string); ok {
				processedTx := ProcessedTransaction{
					Signature: sig,
					Slot:      0,
					Timestamp: time.Now().Unix(),
				}
				allTransactions = append(allTransactions, processedTx)
			}
		}
	}

	return allTransactions, nil
}

func (b *Backfill) convertToAlchemyTx(tx ProcessedTransaction) *AlchemySolanaTransaction {
	return &AlchemySolanaTransaction{
		Signature: tx.Signature,
	}
}
