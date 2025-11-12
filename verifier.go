package alchemywebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Verifier handles signature verification for webhook payloads
type Verifier struct {
	secret string
}

// NewVerifier creates a new signature verifier
func NewVerifier(secret string) *Verifier {
	return &Verifier{
		secret: secret,
	}
}

// Verify verifies the HMAC-SHA256 signature of the payload
func (v *Verifier) Verify(payload []byte, signature string) error {
	if v.secret == "" {
		return fmt.Errorf("signature secret not configured")
	}

	if signature == "" {
		return fmt.Errorf("signature header is missing")
	}

	mac := hmac.New(sha256.New, []byte(v.secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)
	expectedSignature := hex.EncodeToString(expectedMAC)

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}
