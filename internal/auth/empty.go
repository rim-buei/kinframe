package auth

import (
	"context"
	"net/url"
)

// EmptyOIDC implements OIDCClient with no providers configured.
type EmptyOIDC struct{}

// NewOIDCEmpty returns a client that always reports not configured.
func NewOIDCEmpty() *EmptyOIDC { return &EmptyOIDC{} }

// Configured implements OIDCClient.
func (*EmptyOIDC) Configured(Provider) bool { return false }

// AuthCodeURL implements OIDCClient.
func (*EmptyOIDC) AuthCodeURL(Provider, string) (string, error) { return "", ErrNotConfigured }

// Exchange implements OIDCClient.
func (*EmptyOIDC) Exchange(context.Context, Provider, string, url.Values) (Identity, error) {
	return Identity{}, ErrNotConfigured
}
