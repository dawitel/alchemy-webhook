package eth

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/dawitel/alchemy-webhook/cache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"
)

// ActivityHandler is a callback function for processed activities
type ActivityHandler func(ctx context.Context, activity ProcessedActivity) error

// Processor processes Ethereum webhook activities
type Processor struct {
	logger         zerolog.Logger
	cache          cache.Cache
	tokenAddresses map[string]common.Address // symbol -> address mapping
	handler        ActivityHandler
	chainID        string
}

// NewProcessor creates a new Ethereum processor
func NewProcessor(
	logger zerolog.Logger,
	cache cache.Cache,
	tokenAddresses map[string]string, // symbol -> address string
	handler ActivityHandler,
	chainID string,
) *Processor {
	tokenAddrs := make(map[string]common.Address)
	for symbol, addr := range tokenAddresses {
		tokenAddrs[symbol] = common.HexToAddress(addr)
	}

	return &Processor{
		logger:         logger,
		cache:          cache,
		tokenAddresses: tokenAddrs,
		handler:        handler,
		chainID:        chainID,
	}
}

// ProcessActivity processes a single activity
func (p *Processor) ProcessActivity(ctx context.Context, activity AlchemyActivity) error {
	if err := validateEthereumAddress(activity.ToAddress); err != nil {
		return fmt.Errorf("invalid to address: %w", err)
	}
	if err := validateEthereumAddress(activity.FromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}

	txHash := strings.ToLower(strings.TrimPrefix(activity.Hash, "0x"))
	if !strings.HasPrefix(txHash, "0x") {
		txHash = "0x" + txHash
	}

	if err := validateTransactionHash(txHash); err != nil {
		return fmt.Errorf("invalid transaction hash: %w", err)
	}

	uniqueID := txHash
	if activity.TypeTraceAddress != nil && *activity.TypeTraceAddress != "" {
		uniqueID = txHash + "_" + *activity.TypeTraceAddress
	}

	if p.cache != nil {
		processed, err := p.cache.IsProcessed(ctx, uniqueID)
		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("unique_id", uniqueID).
				Msg("Failed to check if transaction is processed, continuing")
		} else if processed {
			p.logger.Debug().
				Str("unique_id", uniqueID).
				Msg("Transaction already processed, skipping")
			return nil
		}
	}

	if err := validateBlockNumber(activity.BlockNum); err != nil {
		return fmt.Errorf("invalid block number: %w", err)
	}

	blockNumStr := strings.TrimPrefix(activity.BlockNum, "0x")
	blockNum, err := strconv.ParseUint(blockNumStr, 16, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block number: %w", err)
	}

	var amount *big.Int
	var currency string
	var network string

	category := strings.ToLower(activity.Category)
	isInternalTx := category == "internal" || (activity.TypeTraceAddress != nil && *activity.TypeTraceAddress != "")

	getDecimals := func() int {
		if activity.RawContract == nil || activity.RawContract.Decimals == nil {
			return 18
		}

		switch v := activity.RawContract.Decimals.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		case string:
			decimalsStr := strings.TrimPrefix(v, "0x")
			if dec, err := strconv.ParseInt(decimalsStr, 16, 64); err == nil {
				return int(dec)
			}
			if dec, err := strconv.Atoi(decimalsStr); err == nil {
				return dec
			}
		}
		return 18
	}

	if category == "external" || category == "internal" {
		if activity.Value == nil {
			return nil
		}
		ethValue := *activity.Value
		ethValueWei := new(big.Float).Mul(big.NewFloat(ethValue), big.NewFloat(1e18))
		amount, _ = ethValueWei.Int(nil)
		if amount == nil {
			amount = big.NewInt(0)
		}
		currency = "ETH"
		if category == "internal" {
			network = "INTERNAL"
		} else {
			network = "MAINNET"
		}
		if p.chainID == "eth-testnet" {
			if category == "internal" {
				network = "INTERNAL-TESTNET"
			} else {
				network = "TESTNET"
			}
		}
	} else if category == "token" || category == "erc20" || (activity.RawContract != nil && activity.RawContract.Address != "") {
		if activity.RawContract == nil {
			return nil
		}
		rawValueStr := activity.RawContract.RawValue
		if rawValueStr == "" {
			if activity.Value != nil {
				decimals := getDecimals()
				tokenValue := *activity.Value
				tokenValueRaw := new(big.Float).Mul(big.NewFloat(tokenValue), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)))
				amount, _ = tokenValueRaw.Int(nil)
			} else {
				return nil
			}
		} else {
			rawValueStr = strings.TrimPrefix(rawValueStr, "0x")
			amount, _ = new(big.Int).SetString(rawValueStr, 16)
		}
		if amount == nil {
			amount = big.NewInt(0)
		}

		tokenAddr := common.HexToAddress(activity.RawContract.Address)
		currency = p.getTokenSymbol(tokenAddr)
		if currency == "" {
			currency = activity.Asset
			if currency == "" {
				currency = "UNKNOWN"
			}
		}

		if isInternalTx {
			network = "ERC-20-INTERNAL"
			if p.chainID == "eth-testnet" {
				network = "ERC-20-INTERNAL-TESTNET"
			}
		} else {
			network = "ERC-20"
			if p.chainID == "eth-testnet" {
				network = "ERC-20-TESTNET"
			}
		}
	} else if category == "erc721" {
		if activity.ERC721TokenID == nil || activity.RawContract == nil {
			return nil
		}
		amount = big.NewInt(1)
		tokenAddr := common.HexToAddress(activity.RawContract.Address)
		currency = p.getTokenSymbol(tokenAddr)
		if currency == "" {
			currency = activity.Asset
			if currency == "" {
				currency = "UNKNOWN"
			}
		}
		if isInternalTx {
			network = "ERC-721-INTERNAL"
			if p.chainID == "eth-testnet" {
				network = "ERC-721-INTERNAL-TESTNET"
			}
		} else {
			network = "ERC-721"
			if p.chainID == "eth-testnet" {
				network = "ERC-721-TESTNET"
			}
		}
	} else if category == "erc1155" {
		if activity.RawContract == nil {
			return nil
		}
		amount = big.NewInt(1)
		tokenAddr := common.HexToAddress(activity.RawContract.Address)
		currency = p.getTokenSymbol(tokenAddr)
		if currency == "" {
			currency = activity.Asset
			if currency == "" {
				currency = "UNKNOWN"
			}
		}
		if isInternalTx {
			network = "ERC-1155-INTERNAL"
			if p.chainID == "eth-testnet" {
				network = "ERC-1155-INTERNAL-TESTNET"
			}
		} else {
			network = "ERC-1155"
			if p.chainID == "eth-testnet" {
				network = "ERC-1155-TESTNET"
			}
		}
	} else {
		p.logger.Debug().Str("category", category).Msg("Unsupported transaction category")
		return nil
	}

	if amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}

	processedActivity := ProcessedActivity{
		TxHash:      txHash,
		FromAddress: activity.FromAddress,
		ToAddress:   activity.ToAddress,
		Value:       amount.String(),
		Currency:    currency,
		Category:    category,
		BlockNumber: blockNum,
		Network:     network,
		IsInternal:  isInternalTx,
	}

	if p.handler != nil {
		if err := p.handler(ctx, processedActivity); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	if p.cache != nil {
		ttl := 24 * time.Hour
		if err := p.cache.MarkProcessed(ctx, uniqueID, ttl); err != nil {
			p.logger.Warn().Err(err).Str("unique_id", uniqueID).Msg("Failed to mark transaction as processed")
		}
	}

	return nil
}

// getTokenSymbol returns the token symbol for an address
func (p *Processor) getTokenSymbol(tokenAddr common.Address) string {
	for symbol, addr := range p.tokenAddresses {
		if addr == tokenAddr {
			return symbol
		}
	}
	return ""
}

// validateEthereumAddress validates an Ethereum address
func validateEthereumAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is empty")
	}
	addr = strings.TrimPrefix(strings.ToLower(addr), "0x")
	if len(addr) != 40 {
		return fmt.Errorf("invalid address length: expected 40 hex characters, got %d", len(addr))
	}

	for _, c := range addr {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("invalid hex character in address: %c", c)
		}
	}
	return nil
}

// validateTransactionHash validates a transaction hash
func validateTransactionHash(hash string) error {
	if hash == "" {
		return fmt.Errorf("transaction hash is empty")
	}
	hash = strings.TrimPrefix(strings.ToLower(hash), "0x")
	if len(hash) != 64 {
		return fmt.Errorf("invalid hash length: expected 64 hex characters, got %d", len(hash))
	}
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("invalid hex character in hash: %c", c)
		}
	}
	return nil
}

// validateBlockNumber validates a block number
func validateBlockNumber(blockNumStr string) error {
	if blockNumStr == "" {
		return fmt.Errorf("block number is empty")
	}
	blockNumStr = strings.TrimPrefix(strings.ToLower(blockNumStr), "0x")
	if len(blockNumStr) == 0 {
		return fmt.Errorf("block number is empty after removing prefix")
	}
	for _, c := range blockNumStr {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("invalid hex character in block number: %c", c)
		}
	}
	return nil
}
