package eth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dawitel/alchemy-webhook/cache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/rs/zerolog"
)

// Backfill handles Ethereum historical transaction backfill
type Backfill struct {
	rpcClient   *ethclient.Client
	processor   *Processor
	logger      zerolog.Logger
	cache       cache.Cache
	timeRange   time.Duration
	batchSize   int
	backfilling int32
}

// NewBackfill creates a new Ethereum backfill instance
func NewBackfill(
	rpcClient *ethclient.Client,
	processor *Processor,
	logger zerolog.Logger,
	cache cache.Cache,
	timeRange time.Duration,
	batchSize int,
) *Backfill {
	return &Backfill{
		rpcClient: rpcClient,
		processor: processor,
		logger:    logger,
		cache:     cache,
		timeRange: timeRange,
		batchSize: batchSize,
	}
}

// Backfill performs backfill for the given addresses
func (b *Backfill) Backfill(ctx context.Context, addresses []string) error {
	if !atomic.CompareAndSwapInt32(&b.backfilling, 0, 1) {
		b.logger.Debug().Msg("Backfill already in progress, skipping")
		return nil
	}
	defer atomic.StoreInt32(&b.backfilling, 0)

	if b.rpcClient == nil {
		return fmt.Errorf("RPC client not available")
	}

	if len(addresses) == 0 {
		b.logger.Debug().Msg("No addresses to backfill")
		return nil
	}

	b.logger.Info().
		Int("address_count", len(addresses)).
		Dur("time_range", b.timeRange).
		Msg("Starting Ethereum historical deposit backfill")

	currentBlock, err := b.rpcClient.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current block number: %w", err)
	}

	const blocksPer12Hours = 3600
	const safetyMargin = 400
	const confirmationBlocks = 12

	fromBlock := uint64(0)
	if currentBlock > blocksPer12Hours+safetyMargin+confirmationBlocks {
		fromBlock = currentBlock - blocksPer12Hours - safetyMargin
	}
	toBlock := currentBlock - confirmationBlocks

	if fromBlock >= toBlock {
		b.logger.Debug().
			Uint64("current_block", currentBlock).
			Msg("Block range too small for backfill, skipping")
		return nil
	}

	b.logger.Info().
		Uint64("from_block", fromBlock).
		Uint64("to_block", toBlock).
		Uint64("current_block", currentBlock).
		Msg("Backfilling historical deposits")

	processedCount := 0
	skippedCount := 0

	addressList := make([]common.Address, 0, len(addresses))
	for _, addrStr := range addresses {
		addr := common.HexToAddress(addrStr)
		addressList = append(addressList, addr)
	}

	for _, addr := range addressList {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		transfers, err := b.getAssetTransfers(ctx, fromBlock, toBlock, []common.Address{addr}, nil)
		if err != nil {
			b.logger.Warn().
				Err(err).
				Str("address", addr.Hex()).
				Msg("Failed to get asset transfers, skipping address")
			time.Sleep(2 * time.Second)
			continue
		}

		for _, transfer := range transfers {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			txHash := strings.ToLower(strings.TrimPrefix(transfer.Hash, "0x"))
			if !strings.HasPrefix(txHash, "0x") {
				txHash = "0x" + txHash
			}

			if b.cache != nil {
				processed, err := b.cache.IsProcessed(ctx, txHash)
				if err == nil && processed {
					skippedCount++
					continue
				}
			}

			if err := b.processHistoricalTransfer(ctx, transfer, addr); err != nil {
				b.logger.Warn().
					Err(err).
					Str("tx_hash", txHash).
					Msg("Failed to process historical transfer")
				continue
			}

			processedCount++
		}

		time.Sleep(1 * time.Second)
	}

	b.logger.Info().
		Int("processed", processedCount).
		Int("skipped", skippedCount).
		Uint64("from_block", fromBlock).
		Uint64("to_block", toBlock).
		Msg("Ethereum historical deposit backfill completed")

	return nil
}

// getAssetTransfers fetches asset transfers using alchemy_getAssetTransfers
func (b *Backfill) getAssetTransfers(ctx context.Context, fromBlock, toBlock uint64, toAddresses, fromAddresses []common.Address) ([]AlchemyAssetTransfer, error) {
	if b.rpcClient == nil {
		return nil, fmt.Errorf("RPC client not initialized")
	}

	var result struct {
		Transfers []AlchemyAssetTransfer `json:"transfers"`
		PageKey   string                 `json:"pageKey"`
	}

	allTransfers := []AlchemyAssetTransfer{}

	toAddressStrs := make([]string, len(toAddresses))
	for i, addr := range toAddresses {
		toAddressStrs[i] = addr.Hex()
	}

	blockRange := toBlock - fromBlock + 1
	maxCount := "0x3e8"
	if blockRange > 100 {
		maxCount = "0x3e8"
	} else if blockRange > 10 {
		maxCount = "0x3e8"
	} else {
		maxCount = "0x1f4"
	}

	params := map[string]interface{}{
		"fromBlock":        fmt.Sprintf("0x%x", fromBlock),
		"toBlock":          fmt.Sprintf("0x%x", toBlock),
		"toAddress":        toAddressStrs,
		"category":         []string{"external", "internal", "erc20", "erc721", "erc1155"},
		"withMetadata":     false,
		"excludeZeroValue": blockRange > 10,
		"maxCount":         maxCount,
	}

	if len(fromAddresses) > 0 {
		fromAddressStrs := make([]string, len(fromAddresses))
		for i, addr := range fromAddresses {
			fromAddressStrs[i] = addr.Hex()
		}
		params["fromAddress"] = fromAddressStrs
	}

	pageKey := ""
	maxPaginationIterations := 100
	maxTotalTransfers := 10000
	iterationCount := 0

	for iterationCount < maxPaginationIterations {
		select {
		case <-ctx.Done():
			return allTransfers, ctx.Err()
		default:
		}

		if len(allTransfers) >= maxTotalTransfers {
			b.logger.Warn().
				Int("total_transfers", len(allTransfers)).
				Msg("Reached maximum transfers limit, stopping pagination")
			break
		}

		if pageKey != "" {
			params["pageKey"] = pageKey
		}

		err := b.rpcClient.Client().CallContext(ctx, &result, "alchemy_getAssetTransfers", params)
		if err != nil {
			return allTransfers, fmt.Errorf("alchemy_getAssetTransfers failed: %w", err)
		}

		allTransfers = append(allTransfers, result.Transfers...)
		iterationCount++

		if result.PageKey == "" {
			break
		}

		if len(result.Transfers) == 0 && result.PageKey != "" {
			b.logger.Warn().
				Str("page_key", result.PageKey).
				Msg("Received empty transfers but pageKey exists, stopping pagination")
			break
		}

		pageKey = result.PageKey
	}

	return allTransfers, nil
}

// processHistoricalTransfer processes a historical transfer
func (b *Backfill) processHistoricalTransfer(ctx context.Context, transfer AlchemyAssetTransfer, toAddress common.Address) error {
	toAddrStr := strings.ToLower(toAddress.Hex())
	if !strings.HasPrefix(toAddrStr, "0x") {
		toAddrStr = "0x" + toAddrStr
	}

	fromAddrStr := strings.ToLower(strings.TrimPrefix(transfer.From, "0x"))
	if !strings.HasPrefix(fromAddrStr, "0x") {
		fromAddrStr = "0x" + fromAddrStr
	}

	// Convert to AlchemyActivity format
	activity := AlchemyActivity{
		BlockNum:    transfer.BlockNum,
		Hash:        transfer.Hash,
		FromAddress: fromAddrStr,
		ToAddress:   toAddrStr,
		Value:       transfer.Value,
		Asset:       transfer.Asset,
		Category:    transfer.Category,
	}

	if transfer.RawContract.Address != "" {
		activity.RawContract = &AlchemyRawContract{
			RawValue: transfer.RawContract.Value,
			Address:  transfer.RawContract.Address,
		}

		if transfer.RawContract.Decimal != "" {
			if dec, err := strconv.Atoi(transfer.RawContract.Decimal); err == nil {
				activity.RawContract.Decimals = dec
			} else if dec, err := strconv.ParseInt(strings.TrimPrefix(transfer.RawContract.Decimal, "0x"), 16, 64); err == nil {
				activity.RawContract.Decimals = int(dec)
			}
		}
	}

	// Process using the processor
	return b.processor.ProcessActivity(ctx, activity)
}
