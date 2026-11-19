package auth

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rim-buei/kinframe/internal/config"
)

// appleSecretFunc returns a function that yields Apple's client_secret JWT,
// or nil when Apple is not fully configured.
func appleSecretFunc(cfg config.Config) (func() (string, error), error) {
	if cfg.AppleClientID == "" {
		return nil, nil
	}
	if cfg.AppleClientSecret != "" {
		sec := cfg.AppleClientSecret
		return func() (string, error) { return sec, nil }, nil
	}
	if cfg.AppleTeamID == "" || cfg.AppleKeyID == "" || cfg.ApplePrivateKey == "" {
		return nil, nil
	}
	key, err := parseApplePrivateKey(cfg.ApplePrivateKey)
	if err != nil {
		return nil, fmt.Errorf("apple private key: %w", err)
	}

	var mu sync.Mutex
	var cached string
	var expires time.Time
	clientID := cfg.AppleClientID
	teamID := cfg.AppleTeamID
	keyID := cfg.AppleKeyID

	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if cached != "" && time.Now().Before(expires.Add(-1*time.Minute)) {
			return cached, nil
		}
		now := time.Now()
		exp := now.Add(150 * 24 * time.Hour) // Apple allows up to ~6 months
		t := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
			"iss": teamID,
			"iat": now.Unix(),
			"exp": exp.Unix(),
			"aud": "https://appleid.apple.com",
			"sub": clientID,
		})
		t.Header["kid"] = keyID
		signed, err := t.SignedString(key)
		if err != nil {
			return "", err
		}
		cached = signed
		expires = exp
		return cached, nil
	}, nil
}

func parseApplePrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an ECDSA private key")
		}
		return ec, nil
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
