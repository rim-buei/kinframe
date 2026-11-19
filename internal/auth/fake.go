package auth

import (
	"context"
	"fmt"
	"net/url"

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
		return Identity{
			Provider:        ProviderGoogle, // store as google-shaped row for schema CHECK
			ProviderSubject: "dev-fake-subject",
			Email:           "dev@localhost",
			DisplayName:     "Dev User",
			AvatarURL:       "",
		}, nil
	}
	if f.inner != nil {
		return f.inner.Exchange(ctx, provider, code, form)
	}
	return Identity{}, ErrNotConfigured
}

// Note: FakeOIDC maps the "dev" login button to ProviderDev in routes, but persists
// the account as provider=google with a stable fake subject so the DB CHECK constraint holds.
