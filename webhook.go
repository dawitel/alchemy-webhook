package alchemywebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

// WebhookInfo represents information about a webhook
type WebhookInfo struct {
	ID           string
	AddressCount int
	IsActive     bool
}

// WebhookManager handles webhook management operations
type WebhookManager struct {
	cfg            *Config
	logger         zerolog.Logger
	httpClient     *http.Client
	circuitBreaker *gobreaker.CircuitBreaker
	mu             sync.RWMutex
	webhooks       map[string]*WebhookInfo
	network        string
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(cfg *Config, logger zerolog.Logger, network string) *WebhookManager {
	circuitBreaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        fmt.Sprintf("webhook-manager-%s", network),
		MaxRequests: uint32(cfg.CircuitBreaker.MaxRequests),
		Interval:    cfg.CircuitBreaker.Interval,
		Timeout:     cfg.CircuitBreaker.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < uint32(cfg.CircuitBreaker.MaxRequests) {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.CircuitBreaker.Threshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info().
				Str("name", name).
				Str("from", from.String()).
				Str("to", to.String()).
				Msg("Webhook circuit breaker state changed")
		},
	})

	return &WebhookManager{
		cfg:            cfg,
		logger:         logger,
		httpClient:     &http.Client{Timeout: cfg.HTTPClient.Timeout},
		circuitBreaker: circuitBreaker,
		webhooks:       make(map[string]*WebhookInfo),
		network:        network,
	}
}

func (wm *WebhookManager) getAuthToken() string {
	authToken := wm.cfg.AlchemyAuthToken
	if authToken == "" || authToken == "YOUR_ALCHEMY_AUTH_TOKEN_HERE" {
		authToken = wm.cfg.AlchemyAPIKey
	}
	return authToken
}

// ListWebhooks fetches all webhooks from Alchemy
func (wm *WebhookManager) ListWebhooks(ctx context.Context) ([]WebhookInfo, error) {
	var result []WebhookInfo

	err := wm.executeWithRetry(ctx, "list_webhooks", func() error {
		_, err := wm.circuitBreaker.Execute(func() (interface{}, error) {
			req, err := http.NewRequestWithContext(ctx, "GET", wm.cfg.AlchemyNotifyURL+"/team-webhooks", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("X-Alchemy-Token", wm.getAuthToken())

			resp, err := wm.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch webhooks: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("failed to fetch webhooks: status %d, body: %s", resp.StatusCode, string(bodyBytes))
			}

			var listResp struct {
				Data []struct {
					ID         string   `json:"id"`
					Name       string   `json:"name"`
					URL        string   `json:"webhook_url"`
					Network    string   `json:"network"`
					Addresses  []string `json:"addresses,omitempty"`
					Type       string   `json:"webhook_type"`
					IsActive   bool     `json:"is_active"`
					SigningKey string   `json:"signing_key,omitempty"`
				} `json:"data"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
				return nil, fmt.Errorf("failed to decode webhooks response: %w", err)
			}

			result = make([]WebhookInfo, 0)
			for _, webhook := range listResp.Data {
				if webhook.Network == wm.network && webhook.Type == "ADDRESS_ACTIVITY" {
					result = append(result, WebhookInfo{
						ID:           webhook.ID,
						AddressCount: len(webhook.Addresses),
						IsActive:     webhook.IsActive,
					})
				}
			}

			return nil, nil
		})
		return err
	})

	return result, err
}

// CreateWebhook creates a new webhook
func (wm *WebhookManager) CreateWebhook(ctx context.Context, name string) (string, error) {
	var webhookID string

	err := wm.executeWithRetry(ctx, "create_webhook", func() error {
		_, err := wm.circuitBreaker.Execute(func() (interface{}, error) {
			reqBody := map[string]interface{}{
				"name":         name,
				"webhook_url":  wm.cfg.WebhookURL,
				"network":      wm.network,
				"addresses":    []string{},
				"webhook_type": "ADDRESS_ACTIVITY",
			}

			jsonData, err := json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal webhook request: %w", err)
			}

			req, err := http.NewRequestWithContext(ctx, "POST", wm.cfg.AlchemyNotifyURL+"/create-webhook", strings.NewReader(string(jsonData)))
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alchemy-Token", wm.getAuthToken())

			resp, err := wm.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to create webhook: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("failed to create webhook: status %d, body: %s", resp.StatusCode, string(bodyBytes))
			}

			var createResp struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
				return nil, fmt.Errorf("failed to decode webhook response: %w", err)
			}

			webhookID = createResp.ID
			return nil, nil
		})
		return err
	})

	if err == nil && webhookID != "" {
		wm.mu.Lock()
		wm.webhooks[webhookID] = &WebhookInfo{
			ID:           webhookID,
			AddressCount: 0,
			IsActive:     true,
		}
		wm.mu.Unlock()
	}

	return webhookID, err
}

func (wm *WebhookManager) GetWebhookAddresses(ctx context.Context, webhookID string) ([]string, error) {
	var allAddresses []string

	err := wm.executeWithRetry(ctx, fmt.Sprintf("get_webhook_addresses_%s", webhookID), func() error {
		_, err := wm.circuitBreaker.Execute(func() (interface{}, error) {
			pageKey := ""
			allAddresses = make([]string, 0)

			for {
				url := wm.cfg.AlchemyNotifyURL + "/webhook-addresses?webhook_id=" + webhookID
				if pageKey != "" {
					url += "&after=" + pageKey
				}

				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to create request: %w", err)
				}

				req.Header.Set("X-Alchemy-Token", wm.getAuthToken())

				resp, err := wm.httpClient.Do(req)
				if err != nil {
					return nil, fmt.Errorf("failed to fetch webhook addresses: %w", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					bodyBytes, _ := io.ReadAll(resp.Body)
					return nil, fmt.Errorf("failed to fetch webhook addresses: status %d, body: %s", resp.StatusCode, string(bodyBytes))
				}

				var addrResp struct {
					Data       []string `json:"data"`
					Pagination struct {
						Cursors struct {
							After string `json:"after"`
						} `json:"cursors"`
						TotalCount int `json:"total_count"`
					} `json:"pagination"`
				}

				if err := json.NewDecoder(resp.Body).Decode(&addrResp); err != nil {
					return nil, fmt.Errorf("failed to decode addresses response: %w", err)
				}

				allAddresses = append(allAddresses, addrResp.Data...)
				pageKey = addrResp.Pagination.Cursors.After

				if pageKey == "" || len(addrResp.Data) == 0 {
					break
				}
			}

			return nil, nil
		})
		return err
	})

	return allAddresses, err
}

// UpdateWebhookAddresses updates addresses for a webhook
func (wm *WebhookManager) UpdateWebhookAddresses(ctx context.Context, webhookID string, addressesToAdd, addressesToRemove []string) error {
	return wm.executeWithRetry(ctx, fmt.Sprintf("update_webhook_%s", webhookID), func() error {
		_, err := wm.circuitBreaker.Execute(func() (interface{}, error) {
			reqBody := map[string]interface{}{
				"webhook_id":          webhookID,
				"addresses_to_add":    addressesToAdd,
				"addresses_to_remove": addressesToRemove,
			}

			jsonData, err := json.Marshal(reqBody)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal update request: %w", err)
			}

			req, err := http.NewRequestWithContext(ctx, "PATCH", wm.cfg.AlchemyNotifyURL+"/update-webhook-addresses", strings.NewReader(string(jsonData)))
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Alchemy-Token", wm.getAuthToken())

			resp, err := wm.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to update webhook: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("failed to update webhook: status %d, body: %s", resp.StatusCode, string(bodyBytes))
			}

			return nil, nil
		})
		return err
	})
}

func (wm *WebhookManager) executeWithRetry(ctx context.Context, operation string, fn func() error) error {
	maxAttempts := wm.cfg.Retry.MaxAttempts
	delay := wm.cfg.Retry.InitialDelay

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if attempt < maxAttempts-1 {
			wm.logger.Warn().
				Err(err).
				Str("operation", operation).
				Int("attempt", attempt+1).
				Int("max_attempts", maxAttempts).
				Dur("retry_delay", delay).
				Msg("Operation failed, retrying")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * wm.cfg.Retry.Multiplier)
			if delay > wm.cfg.Retry.MaxDelay {
				delay = wm.cfg.Retry.MaxDelay
			}
		}
	}

	return fmt.Errorf("operation %s failed after %d attempts", operation, maxAttempts)
}
