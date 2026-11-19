package auth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/rim-buei/kinframe/internal/config"
)

const fakeAuthCode = "kinframe-dev-fake-code"

// FakeOIDC is a local-only IdP for smoke tests (DEV_FAKE_OIDC=1).
// AuthCodeURL points at the app callback with a fixed code; Exchange returns a fixed identity.
type FakeOIDC struct {
	baseURL string
	inner   OIDCClient // optional real client for google/apple when also configured
}

// NewFakeOIDC wraps an optional real client and adds the "dev" provider.
func NewFakeOIDC(cfg config.Config, inner OIDCClient) *FakeOIDC {
	return &FakeOIDC{baseURL: cfg.PublicBaseURL, inner: inner}
}

// Configured implements OIDCClient.
func (f *FakeOIDC) Configured(provider Provider) bool {
	if provider == ProviderDev {
		return true
	}
	if f.inner != nil {
		return f.inner.Configured(provider)
	}
	return false
}

// AuthCodeURL implements OIDCClient.
func (f *FakeOIDC) AuthCodeURL(provider Provider, state string) (string, error) {
	if provider == ProviderDev {
		u, err := url.Parse(f.baseURL + "/auth/callback/dev")
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("state", state)
		q.Set("code", fakeAuthCode)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	if f.inner != nil {
		return f.inner.AuthCodeURL(provider, state)
	}
	return "", ErrNotConfigured
}

// Exchange implements OIDCClient.
func (f *FakeOIDC) Exchange(ctx context.Context, provider Provider, code string, form url.Values) (Identity, error) {
	if provider == ProviderDev {
		if code != fakeAuthCode {
			return Identity{}, fmt.Errorf("invalid fake auth code")
		}
		return DevIdentity("dev"), nil
	}
	if f.inner != nil {
		return f.inner.Exchange(ctx, provider, code, form)
	}
	return Identity{}, ErrNotConfigured
}

// DevIdentity returns a stable fake identity for local smoke tests.
// persona "guest" is a second account for invite-accept smoke; anything else is the primary "dev" user.
// Identities are stored as provider=google so the accounts CHECK constraint holds.
func DevIdentity(persona string) Identity {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "guest":
		return Identity{
			Provider:        ProviderGoogle,
			ProviderSubject: "dev-fake-guest",
			Email:           "guest@localhost",
			DisplayName:     "Guest User",
		}
	default:
		return Identity{
			Provider:        ProviderGoogle,
			ProviderSubject: "dev-fake-subject",
			Email:           "dev@localhost",
			DisplayName:     "Dev User",
		}
	}
}
