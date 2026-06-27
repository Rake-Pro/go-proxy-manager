// Package oidc implements an OpenID Connect client supporting authorization
// code flow with PKCE (S256), discovery, and ID-token verification.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config holds the configuration for an OIDC Client.
type Config struct {
	IssuerURL            string
	ClientID             string
	ClientSecret         string // empty allowed for public clients
	RedirectURL          string
	Scopes               []string // default ["openid","profile","email","groups"] if empty
	UsePKCE              bool     // when true, use S256 PKCE
	RequireVerifiedEmail bool
	GroupsClaim          string // default "groups"
}

// Client is a configured OIDC client built from discovery.
type Client struct {
	cfg         Config
	provider    *oidc.Provider
	verifier    *oidc.IDTokenVerifier
	oauth2Cfg   *oauth2.Config
	groupsClaim string
}

// New performs OIDC discovery against IssuerURL and builds the verifier and
// oauth2 config.
func New(ctx context.Context, cfg Config) (*Client, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}

	groupsClaim := cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
	}

	return &Client{
		cfg:         cfg,
		provider:    provider,
		verifier:    verifier,
		oauth2Cfg:   oauth2Cfg,
		groupsClaim: groupsClaim,
	}, nil
}

// AuthCodeURL builds the authorization URL. When the client uses PKCE, it adds
// the S256 code challenge derived from verifier; it always adds the nonce.
func (c *Client) AuthCodeURL(state, nonce, verifier string) string {
	opts := []oauth2.AuthCodeOption{oidc.Nonce(nonce)}
	if c.cfg.UsePKCE {
		opts = append(opts, oauth2.S256ChallengeOption(verifier))
	}
	return c.oauth2Cfg.AuthCodeURL(state, opts...)
}

// Claims holds the verified identity claims extracted from the ID token.
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
	AMR           []string
	ACR           string
	Raw           map[string]any
}

// Exchange swaps the auth code for tokens, verifies the ID token (signature via
// JWKS, issuer, audience, expiry), checks the nonce matches expectedNonce,
// enforces RequireVerifiedEmail when set, and extracts claims.
func (c *Client) Exchange(ctx context.Context, code, verifier, expectedNonce string) (*Claims, error) {
	var exchangeOpts []oauth2.AuthCodeOption
	if c.cfg.UsePKCE {
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(verifier))
	}

	tok, err := c.oauth2Cfg.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}

	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}

	idToken, err := c.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return nil, fmt.Errorf("nonce mismatch")
	}

	var raw map[string]any
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}

	claims := &Claims{
		Subject: idToken.Subject,
		Raw:     raw,
	}

	if v, ok := raw["email"].(string); ok {
		claims.Email = v
	}
	if v, ok := raw["email_verified"].(bool); ok {
		claims.EmailVerified = v
	}
	if v, ok := raw["name"].(string); ok {
		claims.Name = v
	}
	if v, ok := raw["acr"].(string); ok {
		claims.ACR = v
	}
	claims.Groups = stringSlice(raw[c.groupsClaim])
	claims.AMR = stringSlice(raw["amr"])

	if c.cfg.RequireVerifiedEmail && !claims.EmailVerified {
		return nil, fmt.Errorf("email not verified")
	}

	return claims, nil
}

// stringSlice coerces a JSON-decoded claim value into a []string, handling both
// []string and []any-of-string representations.
func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// NewState returns 32 random bytes encoded as base64url for use as the OAuth2
// state parameter.
func NewState() (string, error) {
	return randomURL(32)
}

// NewNonce returns 32 random bytes encoded as base64url for use as the OIDC
// nonce.
func NewNonce() (string, error) {
	return randomURL(32)
}

// GenerateVerifier wraps oauth2.GenerateVerifier to produce a PKCE code
// verifier.
func GenerateVerifier() string {
	return oauth2.GenerateVerifier()
}

func randomURL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
