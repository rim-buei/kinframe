package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// ErrInvalidSeal is returned when an encrypted cookie cannot be opened.
var ErrInvalidSeal = errors.New("invalid sealed value")

// Sealer encrypts short cookie payloads with AES-256-GCM.
// The key is derived as SHA-256(secret).
type Sealer struct {
	gcm cipher.AEAD
}

// NewSealer builds a Sealer from SESSION_SECRET. secret must be non-empty.
func NewSealer(secret string) (*Sealer, error) {
	if secret == "" {
		return nil, fmt.Errorf("session secret is empty")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{gcm: gcm}, nil
}

// Seal encrypts plaintext and returns a URL-safe base64 ciphertext (nonce||seal).
func (s *Sealer) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := s.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// Open decrypts a value produced by Seal.
func (s *Sealer) Open(sealed string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return nil, ErrInvalidSeal
	}
	ns := s.gcm.NonceSize()
	if len(raw) < ns {
		return nil, ErrInvalidSeal
	}
	nonce, ciphertext := raw[:ns], raw[ns:]
	plain, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidSeal
	}
	return plain, nil
}
