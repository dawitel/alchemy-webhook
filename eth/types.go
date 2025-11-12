package eth

// AlchemyWebhookPayload represents the webhook payload from Alchemy
type AlchemyWebhookPayload struct {
	WebhookID string `json:"webhookId"`
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Type      string `json:"type"`
	Event     struct {
		Network  string            `json:"network"`
		Activity []AlchemyActivity `json:"activity"`
	} `json:"event"`
}

// AlchemyActivity represents a single activity in the webhook payload
type AlchemyActivity struct {
	BlockNum         string              `json:"blockNum"`
	Hash             string              `json:"hash"`
	FromAddress      string              `json:"fromAddress"`
	ToAddress        string              `json:"toAddress"`
	Value            *float64            `json:"value,omitempty"`
	ERC721TokenID    *string             `json:"erc721TokenId,omitempty"`
	ERC1155Metadata  interface{}         `json:"erc1155Metadata,omitempty"`
	Asset            string              `json:"asset,omitempty"`
	Category         string              `json:"category"`
	RawContract      *AlchemyRawContract `json:"rawContract,omitempty"`
	TypeTraceAddress *string             `json:"typeTraceAddress,omitempty"`
	Log              *AlchemyLog         `json:"log,omitempty"`
}

// AlchemyRawContract represents raw contract data
type AlchemyRawContract struct {
	RawValue string      `json:"rawValue"`
	Address  string      `json:"address"`
	Decimals interface{} `json:"decimals,omitempty"`
}

// AlchemyLog represents a log entry
type AlchemyLog struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	BlockHash        string   `json:"blockHash"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
}

// AlchemyCreateWebhookRequest represents a request to create a webhook
type AlchemyCreateWebhookRequest struct {
	Name       string   `json:"name"`
	WebhookURL string   `json:"webhook_url"`
	Network    string   `json:"network"`
	Addresses  []string `json:"addresses"`
	Type       string   `json:"webhook_type"`
}

// AlchemyCreateWebhookResponse represents the response from creating a webhook
type AlchemyCreateWebhookResponse struct {
	ID string `json:"id"`
}

// AlchemyUpdateWebhookRequest represents a request to update a webhook
type AlchemyUpdateWebhookRequest struct {
	WebhookID         string   `json:"webhook_id"`
	Addresses         []string `json:"addresses,omitempty"`
	AddressesToAdd    []string `json:"addresses_to_add"`
	AddressesToRemove []string `json:"addresses_to_remove"`
}

// AlchemyUpdateWebhookResponse represents the response from updating a webhook
type AlchemyUpdateWebhookResponse struct {
	Success bool `json:"success"`
}

// AlchemyWebhook represents a webhook
type AlchemyWebhook struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	URL        string   `json:"webhook_url"`
	Network    string   `json:"network"`
	Addresses  []string `json:"addresses,omitempty"`
	Type       string   `json:"webhook_type"`
	IsActive   bool     `json:"is_active"`
	SigningKey string   `json:"signing_key,omitempty"`
}

// AlchemyListWebhooksResponse represents the response from listing webhooks
type AlchemyListWebhooksResponse struct {
	Data []AlchemyWebhook `json:"data"`
}

// AlchemyWebhookAddressesResponse represents the response from getting webhook addresses
type AlchemyWebhookAddressesResponse struct {
	Data       []string `json:"data"`
	Pagination struct {
		Cursors struct {
			After string `json:"after"`
		} `json:"cursors"`
		TotalCount int `json:"total_count"`
	} `json:"pagination"`
}

// AlchemyAssetTransfer represents an asset transfer for backfill
type AlchemyAssetTransfer struct {
	BlockNum    string   `json:"blockNum"`
	Hash        string   `json:"hash"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Value       *float64 `json:"value"`
	Asset       string   `json:"asset"`
	Category    string   `json:"category"`
	RawContract struct {
		Value   string `json:"value"`
		Address string `json:"address"`
		Decimal string `json:"decimal"`
	} `json:"rawContract"`
}

// AlchemyAssetTransfersResponse represents the response from getAssetTransfers
type AlchemyAssetTransfersResponse struct {
	Transfers []AlchemyAssetTransfer `json:"transfers"`
	PageKey   string                 `json:"pageKey"`
}

// ProcessedActivity represents a processed activity ready for callback
type ProcessedActivity struct {
	TxHash      string
	FromAddress string
	ToAddress   string
	Value       string // BigInt as string
	Currency    string
	Category    string
	BlockNumber uint64
	Network     string
	IsInternal  bool
}
