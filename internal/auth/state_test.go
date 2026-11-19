package auth

import (
	"testing"
	"time"
)

func TestOIDCStateRoundTrip(t *testing.T) {
	sealer, err := NewSealer("csrf-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewOIDCState(ProviderGoogle, "/spaces")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealState(sealer, st)
	if err != nil {
		t.Fatal(err)
	}
	got, err := OpenState(sealer, sealed, ProviderGoogle)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nonce != st.Nonce || got.ReturnTo != "/spaces" {
		t.Fatalf("mismatch: %+v vs %+v", got, st)
	}
}

func TestOIDCStateRejectsWrongProvider(t *testing.T) {
	sealer, err := NewSealer("csrf-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewOIDCState(ProviderGoogle, "")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealState(sealer, st)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenState(sealer, sealed, ProviderApple); err == nil {
		t.Fatal("expected provider mismatch error")
	}
}

func TestOIDCStateRejectsExpired(t *testing.T) {
	sealer, err := NewSealer("csrf-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	st := OIDCState{
		Nonce:    "n",
		Provider: ProviderApple,
		IssuedAt: time.Now().Add(-time.Hour).Unix(),
	}
	sealed, err := SealState(sealer, st)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenState(sealer, sealed, ProviderApple); err == nil {
		t.Fatal("expected expired state error")
	}
}

func TestOIDCStateRejectsGarbage(t *testing.T) {
	sealer, err := NewSealer("csrf-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenState(sealer, "not-a-seal", ProviderGoogle); err == nil {
		t.Fatal("expected error")
	}
}
