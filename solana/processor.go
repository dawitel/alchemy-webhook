package solana

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/dawitel/alchemy-webhook/cache"
	"github.com/mr-tron/base58"
	"github.com/rs/zerolog"
)

// TransactionHandler is a callback function for processed transactions
type TransactionHandler func(ctx context.Context, tx ProcessedTransaction) error

// Processor processes Solana webhook transactions
type Processor struct {
	logger     zerolog.Logger
	cache      cache.Cache
	tokenMints map[string]string // currency -> mint address
	handler    TransactionHandler
	chainID    string
}

// NewProcessor creates a new Solana processor
func NewProcessor(
	logger zerolog.Logger,
	cache cache.Cache,
	tokenMints map[string]string, // currency -> mint address
	handler TransactionHandler,
	chainID string,
) *Processor {
	return &Processor{
		logger:     logger,
		cache:      cache,
		tokenMints: tokenMints,
		handler:    handler,
		chainID:    chainID,
	}
}

// ProcessTransaction processes a single Solana transaction from Alchemy webhook
func (p *Processor) ProcessTransaction(ctx context.Context, alchemyTx AlchemySolanaTransaction, slot uint64) error {
	if len(alchemyTx.Transaction) == 0 || len(alchemyTx.Meta) == 0 {
		p.logger.Debug().
			Str("signature", alchemyTx.Signature).
			Msg("Transaction or meta array is empty")
		return nil
	}

	txDetail := alchemyTx.Transaction[0]
	if len(txDetail.Message) == 0 {
		p.logger.Debug().
			Str("signature", alchemyTx.Signature).
			Msg("Message array is empty")
		return nil
	}

	msg := txDetail.Message[0]
	meta := alchemyTx.Meta[0]

	accountKeys := msg.AccountKeys
	if len(accountKeys) == 0 {
		p.logger.Debug().
			Str("signature", alchemyTx.Signature).
			Msg("Account keys array is empty")
		return nil
	}

	if p.cache != nil {
		processed, err := p.cache.IsProcessed(ctx, alchemyTx.Signature)
		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("signature", alchemyTx.Signature).
				Msg("Failed to check if transaction is processed, continuing")
		} else if processed {
			p.logger.Debug().
				Str("signature", alchemyTx.Signature).
				Msg("Transaction already processed, skipping")
			return nil
		}
	}

	nativeTransfers := p.extractNativeTransfers(accountKeys, meta, alchemyTx.Signature)
	tokenTransfers := p.extractTokenTransfers(accountKeys, msg, meta, alchemyTx.Signature)

	processedTx := ProcessedTransaction{
		Signature:       alchemyTx.Signature,
		Slot:            slot,
		NativeTransfers: nativeTransfers,
		TokenTransfers:  tokenTransfers,
		Fee:             meta.Fee,
		Timestamp:       time.Now().Unix(),
	}

	if len(nativeTransfers) > 0 || len(tokenTransfers) > 0 {
		if p.handler != nil {
			if err := p.handler(ctx, processedTx); err != nil {
				return fmt.Errorf("handler error: %w", err)
			}
		}

		if p.cache != nil {
			ttl := 24 * time.Hour
			if err := p.cache.MarkProcessed(ctx, alchemyTx.Signature, ttl); err != nil {
				p.logger.Warn().Err(err).Str("signature", alchemyTx.Signature).Msg("Failed to mark transaction as processed")
			}
		}
	}

	return nil
}

// extractNativeTransfers extracts native SOL transfers from balance changes
func (p *Processor) extractNativeTransfers(accountKeys []string, meta AlchemySolanaTxMeta, signature string) []NativeTransfer {
	var nativeTransfers []NativeTransfer

	if len(meta.PreBalances) == len(meta.PostBalances) && len(meta.PreBalances) == len(accountKeys) {
		for i := 0; i < len(accountKeys); i++ {
			balanceChange := meta.PostBalances[i] - meta.PreBalances[i]
			if balanceChange > 0 {
				for j := 0; j < len(accountKeys); j++ {
					if i != j && meta.PreBalances[j] > meta.PostBalances[j] {
						fromBalanceChange := meta.PreBalances[j] - meta.PostBalances[j]
						if fromBalanceChange > 0 {
							nativeTransfers = append(nativeTransfers, NativeTransfer{
								FromUserAccount: accountKeys[j],
								ToUserAccount:   accountKeys[i],
								Amount:          balanceChange,
							})
							p.logger.Debug().
								Str("signature", signature).
								Str("from", accountKeys[j]).
								Str("to", accountKeys[i]).
								Int64("amount", balanceChange).
								Msg("Detected SOL native transfer")
							break
						}
					}
				}
			}
		}
	} else {
		p.logger.Debug().
			Str("signature", signature).
			Int("pre_balances_len", len(meta.PreBalances)).
			Int("post_balances_len", len(meta.PostBalances)).
			Int("account_keys_len", len(accountKeys)).
			Msg("Balance arrays length mismatch, skipping native transfer detection")
	}

	return nativeTransfers
}

// extractTokenTransfers extracts SPL token transfers from instructions
func (p *Processor) extractTokenTransfers(accountKeys []string, msg AlchemySolanaTxMessage, meta AlchemySolanaTxMeta, signature string) []TokenTransfer {
	var tokenTransfers []TokenTransfer
	splTokenProgramID := "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"

	var allInstructions []struct {
		Instruction AlchemySolanaInstruction
		IsInner     bool
	}

	for _, instruction := range msg.Instructions {
		if instruction.ProgramIDIndex < 0 || instruction.ProgramIDIndex >= len(accountKeys) {
			continue
		}
		programID := accountKeys[instruction.ProgramIDIndex]
		if programID == splTokenProgramID {
			allInstructions = append(allInstructions, struct {
				Instruction AlchemySolanaInstruction
				IsInner     bool
			}{Instruction: instruction, IsInner: false})
		}
	}

	if !meta.InnerInstructionsNone && len(meta.InnerInstructions) > 0 {
		for _, innerInstrGroup := range meta.InnerInstructions {
			for _, instruction := range innerInstrGroup.Instructions {
				if instruction.ProgramIDIndex < 0 || instruction.ProgramIDIndex >= len(accountKeys) {
					continue
				}
				programID := accountKeys[instruction.ProgramIDIndex]
				if programID == splTokenProgramID {
					allInstructions = append(allInstructions, struct {
						Instruction AlchemySolanaInstruction
						IsInner     bool
					}{Instruction: instruction, IsInner: true})
				}
			}
		}
	}

	for _, instrWrapper := range allInstructions {
		instruction := instrWrapper.Instruction

		if instruction.Data == "" {
			continue
		}

		decodedData, err := base58.Decode(instruction.Data)
		if err != nil || len(decodedData) < 1 {
			continue
		}

		instructionType := decodedData[0]
		var mintAccount string
		var fromTokenAccountIdx, toTokenAccountIdx int = -1, -1
		var amount uint64

		if instructionType == 12 {
			if len(instruction.Accounts) < 4 || len(decodedData) < 9 {
				continue
			}

			fromTokenAccountIdx = instruction.Accounts[0]
			mintIdx := instruction.Accounts[1]
			toTokenAccountIdx = instruction.Accounts[2]

			if fromTokenAccountIdx < 0 || fromTokenAccountIdx >= len(accountKeys) ||
				mintIdx < 0 || mintIdx >= len(accountKeys) ||
				toTokenAccountIdx < 0 || toTokenAccountIdx >= len(accountKeys) {
				continue
			}

			mintAccount = accountKeys[mintIdx]
			amount = binary.LittleEndian.Uint64(decodedData[1:9])
		} else if instructionType == 3 {
			if len(instruction.Accounts) < 3 || len(decodedData) < 9 {
				continue
			}

			fromTokenAccountIdx = instruction.Accounts[0]
			toTokenAccountIdx = instruction.Accounts[1]

			if fromTokenAccountIdx < 0 || fromTokenAccountIdx >= len(accountKeys) ||
				toTokenAccountIdx < 0 || toTokenAccountIdx >= len(accountKeys) {
				continue
			}

			amount = binary.LittleEndian.Uint64(decodedData[1:9])

			for _, logMsg := range meta.LogMessages {
				for _, mintAddr := range p.tokenMints {
					if len(logMsg) > 0 && len(mintAddr) > 0 && len(logMsg) >= len(mintAddr) {
						for i := 0; i <= len(logMsg)-len(mintAddr); i++ {
							if logMsg[i:i+len(mintAddr)] == mintAddr {
								mintAccount = mintAddr
								break
							}
						}
						if mintAccount != "" {
							break
						}
					}
				}
				if mintAccount != "" {
					break
				}
			}

			if mintAccount == "" {
				for _, accountKey := range accountKeys {
					for _, mintAddr := range p.tokenMints {
						if accountKey == mintAddr {
							mintAccount = mintAddr
							break
						}
					}
					if mintAccount != "" {
						break
					}
				}
			}
		} else {
			continue
		}

		if mintAccount != "" && fromTokenAccountIdx >= 0 && toTokenAccountIdx >= 0 &&
			fromTokenAccountIdx < len(accountKeys) && toTokenAccountIdx < len(accountKeys) {
			fromTokenAccount := accountKeys[fromTokenAccountIdx]
			toTokenAccount := accountKeys[toTokenAccountIdx]

			currency, ok := p.getCurrencyFromMint(mintAccount)
			if !ok {
				p.logger.Debug().
					Str("signature", signature).
					Str("mint", mintAccount).
					Msg("Mint not in configured token mints, skipping")
				continue
			}

			var decimals int
			if currency == "USDC" || currency == "USDT" {
				decimals = 6
			} else {
				decimals = 9
			}

			tokenAmount := float64(amount) / math.Pow10(decimals)

			tokenTransfers = append(tokenTransfers, TokenTransfer{
				FromUserAccount:  fromTokenAccount,
				ToUserAccount:    toTokenAccount,
				FromTokenAccount: fromTokenAccount,
				ToTokenAccount:   toTokenAccount,
				TokenAmount:      tokenAmount,
				Mint:             mintAccount,
				Currency:         currency,
			})

			p.logger.Debug().
				Str("signature", signature).
				Str("currency", currency).
				Str("mint", mintAccount).
				Str("from", fromTokenAccount).
				Str("to", toTokenAccount).
				Float64("amount", tokenAmount).
				Msg("Detected token transfer")
		}
	}

	return tokenTransfers
}

// getCurrencyFromMint returns the currency symbol for a mint address
func (p *Processor) getCurrencyFromMint(mint string) (string, bool) {
	for currency, mintAddr := range p.tokenMints {
		if mintAddr == mint {
			return currency, true
		}
	}
	return "", false
}
