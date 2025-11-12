package solana

// AlchemySolanaWebhookPayload represents the webhook payload from Alchemy for Solana
type AlchemySolanaWebhookPayload struct {
	WebhookID string `json:"webhookId"`
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Type      string `json:"type"`
	Event     struct {
		Transaction []AlchemySolanaTransaction `json:"transaction"`
		Slot        uint64                     `json:"slot"`
		Network     string                     `json:"network"`
	} `json:"event"`
}

// AlchemySolanaTransaction represents a Solana transaction in the webhook payload
type AlchemySolanaTransaction struct {
	Signature   string                  `json:"signature"`
	Transaction []AlchemySolanaTxDetail `json:"transaction"`
	Meta        []AlchemySolanaTxMeta   `json:"meta"`
	Index       int                     `json:"index"`
	IsVote      bool                    `json:"is_vote"`
}

// AlchemySolanaTxDetail represents transaction details
type AlchemySolanaTxDetail struct {
	Signatures []string                 `json:"signatures"`
	Message    []AlchemySolanaTxMessage `json:"message"`
}

// AlchemySolanaTxMessage represents a transaction message
type AlchemySolanaTxMessage struct {
	Header          []AlchemySolanaTxHeader    `json:"header"`
	Instructions    []AlchemySolanaInstruction `json:"instructions"`
	Versioned       bool                       `json:"versioned"`
	AccountKeys     []string                   `json:"account_keys"`
	RecentBlockhash string                     `json:"recent_blockhash"`
}

// AlchemySolanaTxHeader represents transaction header
type AlchemySolanaTxHeader struct {
	NumRequiredSignatures       int `json:"num_required_signatures"`
	NumReadonlySignedAccounts   int `json:"num_readonly_signed_accounts"`
	NumReadonlyUnsignedAccounts int `json:"num_readonly_unsigned_accounts"`
}

// AlchemySolanaInstruction represents an instruction
type AlchemySolanaInstruction struct {
	Data           string `json:"data,omitempty"`
	ProgramIDIndex int    `json:"program_id_index,omitempty"`
	Accounts       []int  `json:"accounts,omitempty"`
}

// AlchemySolanaTxMeta represents transaction metadata
type AlchemySolanaTxMeta struct {
	Fee                   int64                           `json:"fee"`
	PreBalances           []int64                         `json:"pre_balances"`
	PostBalances          []int64                         `json:"post_balances"`
	InnerInstructions     []AlchemySolanaInnerInstruction `json:"inner_instructions,omitempty"`
	InnerInstructionsNone bool                            `json:"inner_instructions_none"`
	LogMessages           []string                        `json:"log_messages"`
	LogMessagesNone       bool                            `json:"log_messages_none"`
	ReturnDataNone        bool                            `json:"return_data_none"`
	ComputeUnitsConsumed  int64                           `json:"compute_units_consumed"`
}

// AlchemySolanaInnerInstruction represents an inner instruction
type AlchemySolanaInnerInstruction struct {
	Index        int                        `json:"index"`
	Instructions []AlchemySolanaInstruction `json:"instructions"`
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

// NativeTransfer represents a native SOL transfer
type NativeTransfer struct {
	FromUserAccount string
	ToUserAccount   string
	Amount          int64 // lamports
}

// TokenTransfer represents an SPL token transfer
type TokenTransfer struct {
	FromUserAccount  string
	ToUserAccount    string
	FromTokenAccount string
	ToTokenAccount   string
	TokenAmount      float64
	Mint             string
	Currency         string
}

// ProcessedTransaction represents a processed transaction ready for callback
type ProcessedTransaction struct {
	Signature       string
	Slot            uint64
	NativeTransfers []NativeTransfer
	TokenTransfers  []TokenTransfer
	Fee             int64
	Timestamp       int64
}
