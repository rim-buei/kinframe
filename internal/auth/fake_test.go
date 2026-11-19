package auth

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/rim-buei/kinframe/internal/config"
)

func TestFakeOIDCExchange(t *testing.T) {
	f := NewFakeOIDC(config.Config{PublicBaseURL: "http://localhost:8080"}, nil)
	if !f.Configured(ProviderDev) {
		t.Fatal("dev should be configured")
	}
	u, err := f.AuthCodeURL(ProviderDev, "state-value")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/auth/callback/dev") || !strings.Contains(u, "code=") {
		t.Fatalf("unexpected url: %s", u)
	}
	id, err := f.Exchange(context.Background(), ProviderDev, fakeAuthCode, url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "dev@localhost" || id.ProviderSubject == "" {
		t.Fatalf("identity: %+v", id)
	}
	if _, err := f.Exchange(context.Background(), ProviderDev, "bad", nil); err == nil {
		t.Fatal("expected bad code error")
	}
}
