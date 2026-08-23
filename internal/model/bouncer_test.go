package model

import (
	"testing"
	"time"
)

func TestBouncerMiddlewareValidate(t *testing.T) {
	base := func(mutate func(*BouncerMiddleware)) Middleware {
		b := BouncerMiddleware{
			Provider: BouncerProviderCrowdSec,
			URL:      "http://crowdsec:8080",
			APIKey:   Secret("${ENV:CROWDSEC_BOUNCER_KEY}"),
		}
		if mutate != nil {
			mutate(&b)
		}
		return Middleware{ObjectMeta: ObjectMeta{Name: "bnc"}, Type: MWTypeBouncer, Bouncer: &b}
	}

	tests := []struct {
		name    string
		mw      Middleware
		wantErr bool
	}{
		{name: "valid crowdsec", mw: base(nil)},
		{name: "provider defaults to crowdsec", mw: base(func(b *BouncerMiddleware) { b.Provider = "" })},
		{name: "valid http without a key", mw: base(func(b *BouncerMiddleware) {
			b.Provider, b.APIKey = BouncerProviderHTTP, ""
		})},
		{name: "every optional field set", mw: base(func(b *BouncerMiddleware) {
			b.Timeout, b.CacheTTL, b.CacheMaxEntries = "1s", "30s", 512
			b.OnError, b.DenyStatus, b.DenyWith = BouncerOnErrorFailClosed, 429, BouncerDenyWithPlain
			b.AllowFrom, b.Stream = []string{"10.0.0.0/8", "192.0.2.1"}, true
		})},

		{name: "missing spec", mw: Middleware{ObjectMeta: ObjectMeta{Name: "bnc"}, Type: MWTypeBouncer}, wantErr: true},
		{name: "unknown provider", mw: base(func(b *BouncerMiddleware) { b.Provider = "modsecurity" }), wantErr: true},
		{name: "missing url", mw: base(func(b *BouncerMiddleware) { b.URL = "" }), wantErr: true},
		{name: "relative url", mw: base(func(b *BouncerMiddleware) { b.URL = "/v1/decisions" }), wantErr: true},
		{name: "non-http scheme", mw: base(func(b *BouncerMiddleware) { b.URL = "file:///etc/passwd" }), wantErr: true},
		// The LAPI authenticates every bouncer call; without a key the middleware
		// would silently run on its onError policy forever.
		{name: "crowdsec without an api key", mw: base(func(b *BouncerMiddleware) { b.APIKey = "" }), wantErr: true},
		{name: "stream on the http provider", mw: base(func(b *BouncerMiddleware) {
			b.Provider, b.APIKey, b.Stream = BouncerProviderHTTP, "", true
		}), wantErr: true},
		{name: "bad timeout", mw: base(func(b *BouncerMiddleware) { b.Timeout = "2 seconds" }), wantErr: true},
		{name: "zero timeout", mw: base(func(b *BouncerMiddleware) { b.Timeout = "0s" }), wantErr: true},
		{name: "negative cacheTTL", mw: base(func(b *BouncerMiddleware) { b.CacheTTL = "-1m" }), wantErr: true},
		{name: "negative cacheMaxEntries", mw: base(func(b *BouncerMiddleware) { b.CacheMaxEntries = -1 }), wantErr: true},
		{name: "unknown onError", mw: base(func(b *BouncerMiddleware) { b.OnError = "retry" }), wantErr: true},
		{name: "unknown denyWith", mw: base(func(b *BouncerMiddleware) { b.DenyWith = "redirect" }), wantErr: true},
		{name: "2xx denyStatus", mw: base(func(b *BouncerMiddleware) { b.DenyStatus = 204 }), wantErr: true},
		{name: "invalid allowFrom", mw: base(func(b *BouncerMiddleware) { b.AllowFrom = []string{"10.0.0.0/33"} }), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mw.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestBouncerMiddlewareDefaults(t *testing.T) {
	var b BouncerMiddleware
	if got := b.ProviderOrDefault(); got != BouncerProviderCrowdSec {
		t.Errorf("ProviderOrDefault() = %q, want crowdsec", got)
	}
	if got := b.TimeoutOrDefault(); got != 2*time.Second {
		t.Errorf("TimeoutOrDefault() = %v, want 2s", got)
	}
	if got := b.CacheTTLOrDefault(); got != 60*time.Second {
		t.Errorf("CacheTTLOrDefault() = %v, want 60s", got)
	}
	if got := b.CacheMaxEntriesOrDefault(); got != 10000 {
		t.Errorf("CacheMaxEntriesOrDefault() = %d, want 10000", got)
	}
	if !b.FailOpen() {
		t.Error("FailOpen() = false, want true (fail-open is the documented default)")
	}
	if got := b.DenyStatusOrDefault(); got != 403 {
		t.Errorf("DenyStatusOrDefault() = %d, want 403", got)
	}

	b = BouncerMiddleware{Timeout: "5s", CacheTTL: "10m", CacheMaxEntries: 7, OnError: BouncerOnErrorFailClosed, DenyStatus: 451}
	if got := b.TimeoutOrDefault(); got != 5*time.Second {
		t.Errorf("TimeoutOrDefault() = %v, want 5s", got)
	}
	if got := b.CacheTTLOrDefault(); got != 10*time.Minute {
		t.Errorf("CacheTTLOrDefault() = %v, want 10m", got)
	}
	if got := b.CacheMaxEntriesOrDefault(); got != 7 {
		t.Errorf("CacheMaxEntriesOrDefault() = %d, want 7", got)
	}
	if b.FailOpen() {
		t.Error("FailOpen() = true, want false under fail-closed")
	}
	if got := b.DenyStatusOrDefault(); got != 451 {
		t.Errorf("DenyStatusOrDefault() = %d, want 451", got)
	}
}

// A literal apiKey must be caught by the store's plaintext-secret guard, which
// walks Secret-typed fields reflectively - so the field must be a Secret, not a
// string.
func TestBouncerAPIKeyIsASecret(t *testing.T) {
	cfg := Config{Middlewares: []Middleware{{
		ObjectMeta: ObjectMeta{Name: "bnc"}, Type: MWTypeBouncer,
		Bouncer: &BouncerMiddleware{URL: "http://crowdsec:8080", APIKey: Secret("literal-key")},
	}}}
	if got := LiteralSecrets(cfg); len(got) == 0 {
		t.Error("LiteralSecrets did not flag a literal bouncer.apiKey")
	}

	cfg.Middlewares[0].Bouncer.APIKey = Secret("${ENV:CROWDSEC_BOUNCER_KEY}")
	if got := LiteralSecrets(cfg); len(got) != 0 {
		t.Errorf("LiteralSecrets flagged a placeholder: %v", got)
	}
}
