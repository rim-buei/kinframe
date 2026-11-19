package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidState is returned when OIDC CSRF state fails validation.
var ErrInvalidState = errors.New("invalid OIDC state")

const stateTTL = 10 * time.Minute

// OIDCState is the CSRF payload sealed into a short-lived cookie during login start.
type OIDCState struct {
	Nonce      string   `json:"n"`
	Provider   Provider `json:"p"`
	IssuedAt   int64    `json:"iat"`
	ReturnTo   string   `json:"rt,omitempty"`
	DevPersona string   `json:"dp,omitempty"` // DEV_FAKE_OIDC only: "dev" | "guest"
}

// NewOIDCState creates a fresh state for provider.
func NewOIDCState(provider Provider, returnTo string) (OIDCState, error) {
	nonce, err := randomToken(32)
	if err != nil {
		return OIDCState{}, err
	}
	return OIDCState{
		Nonce:    nonce,
		Provider: provider,
		IssuedAt: time.Now().UTC().Unix(),
		ReturnTo: returnTo,
	}, nil
}

// SealState encrypts OIDC state for a cookie.
func SealState(sealer *Sealer, st OIDCState) (string, error) {
	b, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	return sealer.Seal(b)
}

// OpenState decrypts and validates OIDC state (provider + TTL).
func OpenState(sealer *Sealer, sealed string, want Provider) (OIDCState, error) {
	plain, err := sealer.Open(sealed)
	if err != nil {
		return OIDCState{}, ErrInvalidState
	}
	var st OIDCState
	if err := json.Unmarshal(plain, &st); err != nil {
		return OIDCState{}, ErrInvalidState
	}
	if st.Provider != want || st.Nonce == "" {
		return OIDCState{}, ErrInvalidState
	}
	issued := time.Unix(st.IssuedAt, 0).UTC()
	if time.Since(issued) > stateTTL || issued.After(time.Now().UTC().Add(time.Minute)) {
		return OIDCState{}, ErrInvalidState
	}
	return st, nil
}

// StateParam is the opaque value sent to the IdP as the OAuth "state" query param.
// It is the same sealed blob stored in the cookie so start/callback can be matched.
func StateParam(sealedCookieValue string) string {
	return sealedCookieValue
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
