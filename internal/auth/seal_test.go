package auth

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSealerRoundTrip(t *testing.T) {
	s, err := NewSealer("test-session-secret-please-change")
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("session-token-abc")
	sealed, err := s.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == string(plain) {
		t.Fatal("sealed value should not equal plaintext")
	}
	out, err := s.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("got %q want %q", out, plain)
	}
}

func TestSealerRejectsTamper(t *testing.T) {
	s, err := NewSealer("test-session-secret-please-change")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xff
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := s.Open(tampered); err == nil {
		t.Fatal("expected error for tampered seal")
	}
}

func TestSealerDifferentSecrets(t *testing.T) {
	a, err := NewSealer("secret-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSealer("secret-b")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := a.Seal([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Open(sealed); err == nil {
		t.Fatal("expected open failure with different secret")
	}
}

func TestNewSealerEmpty(t *testing.T) {
	if _, err := NewSealer(""); err == nil {
		t.Fatal("expected error for empty secret")
	}
}
