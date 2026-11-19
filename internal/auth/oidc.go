// Package auth implements OIDC (Google / Apple) and encrypted cookie sessions.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rim-buei/kinframe/internal/config"
	"golang.org/x/oauth2"
)

// ErrNotConfigured is returned when a provider's client credentials are missing.
var ErrNotConfigured = errors.New("OIDC provider not configured")

// Provider identifies an SSO provider.
type Provider string

const (
	ProviderGoogle Provider = "google"
	ProviderApple  Provider = "apple"
	ProviderDev    Provider = "dev" // local fake IdP only
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
type OIDCClient interface {
	Configured(provider Provider) bool
	AuthCodeURL(provider Provider, state string) (string, error)
	Exchange(ctx context.Context, provider Provider, code string, form url.Values) (Identity, error)
}

type providerBundle struct {
	oauth2   *oauth2.Config
	verifier *oidc.IDTokenVerifier
	// appleClientSecret is called before each token exchange when Apple JWT must be refreshed.
	appleClientSecret func() (string, error)
}

// RealOIDC talks to Google and/or Apple using authorization-code + ID token verification.
type RealOIDC struct {
	mu        sync.Mutex
	providers map[Provider]*providerBundle
	http      *http.Client
}

// NewOIDC builds a client from config. Unconfigured providers are simply omitted.
func NewOIDC(ctx context.Context, cfg config.Config) (*RealOIDC, error) {
	c := &RealOIDC{
		providers: make(map[Provider]*providerBundle),
		http:      &http.Client{Timeout: 15 * time.Second},
	}
	redirect := func(p Provider) string {
		return cfg.PublicBaseURL + "/auth/callback/" + string(p)
	}

	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
		if err != nil {
			return nil, fmt.Errorf("google oidc discovery: %w", err)
		}
		oauth2Cfg := &oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  redirect(ProviderGoogle),
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		}
		c.providers[ProviderGoogle] = &providerBundle{
			oauth2:   oauth2Cfg,
			verifier: provider.Verifier(&oidc.Config{ClientID: cfg.GoogleClientID}),
		}
	}

	if cfg.AppleClientID != "" {
		secretFn, err := appleSecretFunc(cfg)
		if err != nil {
			return nil, err
		}
		if secretFn == nil {
			// Apple ID set but no secret/key material — skip.
		} else {
			provider, err := oidc.NewProvider(ctx, "https://appleid.apple.com")
			if err != nil {
				return nil, fmt.Errorf("apple oidc discovery: %w", err)
			}
			sec, err := secretFn()
			if err != nil {
				return nil, fmt.Errorf("apple client secret: %w", err)
			}
			oauth2Cfg := &oauth2.Config{
				ClientID:     cfg.AppleClientID,
				ClientSecret: sec,
				RedirectURL:  redirect(ProviderApple),
				Endpoint:     provider.Endpoint(),
				Scopes:       []string{oidc.ScopeOpenID, "email", "name"},
			}
			c.providers[ProviderApple] = &providerBundle{
				oauth2:            oauth2Cfg,
				verifier:          provider.Verifier(&oidc.Config{ClientID: cfg.AppleClientID}),
				appleClientSecret: secretFn,
			}
		}
	}

	return c, nil
}

// Configured reports whether the provider can start a login.
func (c *RealOIDC) Configured(provider Provider) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.providers[provider]
	return ok
}

// AuthCodeURL returns the IdP authorization URL.
func (c *RealOIDC) AuthCodeURL(provider Provider, state string) (string, error) {
	c.mu.Lock()
	b, ok := c.providers[provider]
	c.mu.Unlock()
	if !ok {
		return "", ErrNotConfigured
	}
	opts := []oauth2.AuthCodeOption{oauth2.AccessTypeOnline}
	if provider == ProviderApple {
		// Apple commonly returns the code via form_post for web apps.
		opts = append(opts, oauth2.SetAuthURLParam("response_mode", "form_post"))
	}
	return b.oauth2.AuthCodeURL(state, opts...), nil
}

// Exchange redeems the authorization code and verifies the ID token.
// form may carry Apple's first-login "user" JSON (display name).
func (c *RealOIDC) Exchange(ctx context.Context, provider Provider, code string, form url.Values) (Identity, error) {
	c.mu.Lock()
	b, ok := c.providers[provider]
	c.mu.Unlock()
	if !ok {
		return Identity{}, ErrNotConfigured
	}
	if code == "" {
		return Identity{}, fmt.Errorf("missing authorization code")
	}

	if b.appleClientSecret != nil {
		sec, err := b.appleClientSecret()
		if err != nil {
			return Identity{}, err
		}
		c.mu.Lock()
		b.oauth2.ClientSecret = sec
		c.mu.Unlock()
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)
	tok, err := b.oauth2.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return Identity{}, fmt.Errorf("missing id_token")
	}
	idToken, err := b.verifier.Verify(ctx, rawID)
	if err != nil {
		return Identity{}, fmt.Errorf("verify id_token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("id_token claims: %w", err)
	}

	display := claims.Name
	if provider == ProviderApple && form != nil {
		if u := form.Get("user"); u != "" {
			var appleUser struct {
				Name struct {
					FirstName string `json:"firstName"`
					LastName  string `json:"lastName"`
				} `json:"name"`
			}
			if json.Unmarshal([]byte(u), &appleUser) == nil {
				n := strings.TrimSpace(appleUser.Name.FirstName + " " + appleUser.Name.LastName)
				if n != "" {
					display = n
				}
			}
		}
	}

	return Identity{
		Provider:        provider,
		ProviderSubject: idToken.Subject,
		Email:           claims.Email,
		DisplayName:     display,
		AvatarURL:       claims.Picture,
	}, nil
}

// ParseProvider maps a path segment to a Provider.
func ParseProvider(s string) (Provider, error) {
	switch Provider(strings.ToLower(s)) {
	case ProviderGoogle:
		return ProviderGoogle, nil
	case ProviderApple:
		return ProviderApple, nil
	case ProviderDev:
		return ProviderDev, nil
	default:
		return "", fmt.Errorf("unknown provider %q", s)
	}
}
