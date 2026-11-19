// Package auth stubs OIDC (Google / Apple) and session helpers.
package auth

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned until OIDC client credentials are wired.
var ErrNotConfigured = errors.New("OIDC not configured (scaffold stub)")

// Provider identifies an SSO provider.
type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderApple  Provider = "apple"
)

// Identity is the verified subject from an ID token / userinfo response.
type Identity struct {
	Provider        Provider
	ProviderSubject string
	Email           string
	DisplayName     string
	AvatarURL       string
}

// OIDCClient starts authorization and exchanges callbacks.
// TODO: implement authorization-code flow for Google and Sign in with Apple.
type OIDCClient interface {
	AuthCodeURL(provider Provider, state string) (string, error)
	Exchange(ctx context.Context, provider Provider, code string) (Identity, error)
}

// StubOIDC is a placeholder OIDCClient.
type StubOIDC struct{}

// AuthCodeURL implements OIDCClient.
func (StubOIDC) AuthCodeURL(Provider, string) (string, error) {
	return "", ErrNotConfigured
}

// Exchange implements OIDCClient.
func (StubOIDC) Exchange(context.Context, Provider, string) (Identity, error) {
	return Identity{}, ErrNotConfigured
}
