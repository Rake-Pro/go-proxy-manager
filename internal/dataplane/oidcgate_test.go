package dataplane

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
	jose "github.com/go-jose/go-jose/v4"
)

func TestSSOTokenRoundTrip(t *testing.T) {
	payload := []byte(`{"sub":"admin"}`)
	tok := signToken(macLabelSSOSession, payload)
	got, ok := verifyToken(macLabelSSOSession, tok)
	if !ok || string(got) != string(payload) {
		t.Fatalf("round trip failed: ok=%v got=%q", ok, got)
	}
	// Tampering with the payload is rejected.
	if _, ok := verifyToken(macLabelSSOSession, "ZmFrZQ."+tok[len(tok)-10:]); ok {
		t.Fatal("tampered token must not verify")
	}
}

// TestTokenLabelsAreDomainSeparated proves the three cookie types no longer
// share one MAC namespace: a token minted for one type must not verify as
// another, even though all three are signed with the same process-wide key.
func TestTokenLabelsAreDomainSeparated(t *testing.T) {
	payload := []byte(`{"sub":"admin"}`)
	labels := []string{macLabelSSOSession, macLabelSSOState, macLabelSticky}
	for _, mint := range labels {
		tok := signToken(mint, payload)
		for _, check := range labels {
			_, ok := verifyToken(check, tok)
			if want := mint == check; ok != want {
				t.Fatalf("token minted under %q verified under %q = %v, want %v", mint, check, ok, want)
			}
		}
	}
}

func TestSanitizeSSOReturn(t *testing.T) {
	cases := map[string]string{
		"/dash?x=1":  "/dash?x=1",
		"":           "/",
		"relative":   "/",
		"//evil.com": "/",
		`/\evil`:     "/",
		"/ok\nx":     "/",
	}
	for in, want := range cases {
		if got := sanitizeSSOReturn(in); got != want {
			t.Errorf("sanitizeSSOReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDataOIDCAuthorized(t *testing.T) {
	// No mapping, no required roles: any authenticated user passes.
	open := &dataOIDC{}
	if !open.authorized(nil) {
		t.Fatal("login-only gate must admit any authenticated user")
	}
	// With a role mapping, group membership is enforced.
	gated := &dataOIDC{roleMapping: &model.RoleMapping{AdminGroups: []string{"proxy-admins"}}}
	if gated.authorized([]string{"other"}) {
		t.Fatal("user without the mapped group must be denied")
	}
	if !gated.authorized([]string{"proxy-admins"}) {
		t.Fatal("user in the mapped admin group must be allowed")
	}
}

// --- end-to-end gate flow against a mock IdP -------------------------------

type mockIdP struct {
	server *httptest.Server
	signer jose.Signer
	pubJWK jose.JSONWebKey
	claims map[string]any
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "k1"
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256,
		Key: jose.JSONWebKey{Key: key, KeyID: kid, Algorithm: "RS256", Use: "sig"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockIdP{signer: signer, pubJWK: jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig"}}
	mux := http.NewServeMux()
	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	issuer := m.server.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
			"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{m.pubJWK}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "x", "token_type": "Bearer", "expires_in": 3600,
			"id_token": m.signIDToken(t),
		})
	})
	return m
}

func (m *mockIdP) signIDToken(t *testing.T) string {
	t.Helper()
	claims := map[string]any{
		"iss": m.server.URL, "aud": "gpm-client", "sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	for k, v := range m.claims {
		claims[k] = v
	}
	b, _ := json.Marshal(claims)
	obj, err := m.signer.Sign(b)
	if err != nil {
		t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cookieByName(res *http.Response, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestDataOIDCEndToEnd(t *testing.T) {
	idpSrv := newMockIdP(t)
	gate, err := compileDataOIDC(model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "idp"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
	}, nil, "app", []string{"app2.example.com", "app.example.com"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	h := gate.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream:" + r.Header.Get("X-Forwarded-Email")))
	}))

	// 1. Unauthenticated request -> redirect to the IdP + a state cookie.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://app2.example.com/dash", nil)
	req.Host = "app2.example.com"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("begin: got %d, want 302 (body %q)", rec.Code, rec.Body.String())
	}
	stateCookie := cookieByName(rec.Result(), oidcStateCookie)
	if stateCookie == nil {
		t.Fatal("begin must set a state cookie")
	}
	payload, ok := verifyToken(macLabelSSOState, stateCookie.Value)
	if !ok {
		t.Fatal("state cookie must be a valid signed token")
	}
	var st oidcLoginState
	if err := json.Unmarshal(payload, &st); err != nil {
		t.Fatal(err)
	}

	// The id_token the IdP will mint must echo the gpm-issued nonce.
	idpSrv.claims = map[string]any{"nonce": st.Nonce, "email": "admin@example.com", "groups": []string{"team"}}

	// 2. IdP redirects back to the callback with code+state.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "https://app2.example.com"+oidcCallbackPath+"?code=abc&state="+url.QueryEscape(st.State), nil)
	req2.Host = "app2.example.com"
	req2.AddCookie(stateCookie)
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("callback: got %d, want 302 (body %q)", rec2.Code, rec2.Body.String())
	}
	if loc := rec2.Header().Get("Location"); loc != "/dash" {
		t.Fatalf("callback should return to the original path, got %q", loc)
	}
	sessCookie := cookieByName(rec2.Result(), oidcSessionCookie)
	if sessCookie == nil {
		t.Fatal("callback must set the SSO session cookie")
	}
	// GPM-I2: the cookie carries the __Host- prefix and satisfies its invariants
	// (Secure, Path=/, no Domain), so a sibling subdomain cannot shadow it.
	if !strings.HasPrefix(sessCookie.Name, "__Host-") {
		t.Fatalf("SSO cookie must use the __Host- prefix, got %q", sessCookie.Name)
	}
	if !sessCookie.Secure || sessCookie.Path != "/" || sessCookie.Domain != "" {
		t.Fatalf("__Host- cookie invariants violated: secure=%v path=%q domain=%q", sessCookie.Secure, sessCookie.Path, sessCookie.Domain)
	}

	// 3. A request bearing the session cookie is admitted and gets identity headers.
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("GET", "https://app2.example.com/dash", nil)
	req3.Host = "app2.example.com"
	req3.AddCookie(sessCookie)
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || rec3.Body.String() != "upstream:admin@example.com" {
		t.Fatalf("authed request: got %d %q", rec3.Code, rec3.Body.String())
	}

	// 4. A callback with no state cookie is rejected.
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("GET", "https://app2.example.com"+oidcCallbackPath+"?code=abc&state=x", nil)
	req4.Host = "app2.example.com"
	h.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusBadRequest {
		t.Fatalf("callback without state cookie: got %d, want 400", rec4.Code)
	}
}

// TestDataOIDCSessionBoundToHost proves the M2 fix: the process-wide HMAC key
// means a session cookie minted for one gated host verifies on another, so the
// gate must additionally reject a cookie whose signed Host is not its own -
// otherwise a copied cookie carrying host A's groups would be re-evaluated
// against host B's role mapping.
func TestDataOIDCSessionBoundToHost(t *testing.T) {
	idpSrv := newMockIdP(t)
	newGate := func(host string) http.Handler {
		g, err := compileDataOIDC(model.IdentityProvider{
			ObjectMeta: model.ObjectMeta{Name: "idp"},
			Type:       model.IdPTypeOIDC,
			OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
		}, nil, host, []string{host + ".example.com"})
		if err != nil {
			t.Fatalf("compile %s: %v", host, err)
		}
		return g.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("upstream"))
		}))
	}

	// A valid session cookie bound to host "app".
	payload, _ := json.Marshal(oidcSession{Sub: "u", Host: "app", IdP: "idp", Exp: time.Now().Add(time.Hour).Unix()})
	appCookie := &http.Cookie{Name: oidcSessionCookie, Value: signToken(macLabelSSOSession, payload)}

	// Its own host admits it.
	recSame := httptest.NewRecorder()
	rSame := httptest.NewRequest("GET", "https://app.example.com/x", nil)
	rSame.AddCookie(appCookie)
	newGate("app").ServeHTTP(recSame, rSame)
	if recSame.Code != http.StatusOK || recSame.Body.String() != "upstream" {
		t.Fatalf("cookie must be honored by its own host: got %d %q", recSame.Code, recSame.Body.String())
	}

	// A session minted by ANOTHER provider on the same host must not pass this
	// gate either: the cookie is bound to the provider that issued it.
	otherPayload, _ := json.Marshal(oidcSession{Sub: "u", Host: "app", IdP: "other-idp", Exp: time.Now().Add(time.Hour).Unix()})
	recOther := httptest.NewRecorder()
	rOther := httptest.NewRequest("GET", "https://app.example.com/x", nil)
	rOther.AddCookie(&http.Cookie{Name: oidcSessionCookie, Value: signToken(macLabelSSOSession, otherPayload)})
	newGate("app").ServeHTTP(recOther, rOther)
	if recOther.Code == http.StatusOK {
		t.Fatalf("a session from another provider must not pass this gate: got %d", recOther.Code)
	}

	// A different host must not honor the replayed cookie; it starts a fresh login.
	recCross := httptest.NewRecorder()
	rCross := httptest.NewRequest("GET", "https://other.example.com/x", nil)
	rCross.AddCookie(appCookie)
	newGate("other").ServeHTTP(recCross, rCross)
	if recCross.Code == http.StatusOK {
		t.Fatalf("cookie bound to host \"app\" must not be honored by host \"other\"")
	}
	if recCross.Code != http.StatusFound {
		t.Fatalf("cross-host replay should trigger a fresh login redirect, got %d", recCross.Code)
	}
}

func TestSSORevocationWatermark(t *testing.T) {
	idpSrv := newMockIdP(t)
	g, err := compileDataOIDC(model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "idp"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client"},
	}, nil, "app", []string{"app2.example.com", "app.example.com"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	gate := g.handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream"))
	}))
	// The watermark is process-global; reset it so test order never matters.
	t.Cleanup(func() { ssoNotBefore.Store(0) })

	mkCookie := func(iat int64) *http.Cookie {
		payload, _ := json.Marshal(oidcSession{
			Sub: "u", Host: "app", IdP: "idp",
			Exp: time.Now().Add(time.Hour).Unix(),
			Iat: iat,
		})
		return &http.Cookie{Name: oidcSessionCookie, Value: signToken(macLabelSSOSession, payload)}
	}
	serve := func(c *http.Cookie) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://app.example.com/x", nil)
		r.AddCookie(c)
		gate.ServeHTTP(rec, r)
		return rec.Code
	}

	old := mkCookie(time.Now().Add(-10 * time.Second).Unix())
	legacy := mkCookie(0) // pre-Iat cookie shape

	if code := serve(old); code != http.StatusOK {
		t.Fatalf("pre-revocation session rejected: %d", code)
	}
	if code := serve(legacy); code != http.StatusOK {
		t.Fatalf("legacy (iat-less) session rejected before any revocation: %d", code)
	}

	if err := RevokeAllSSOSessions(); err != nil {
		t.Fatalf("RevokeAllSSOSessions: %v", err)
	}

	// Everything issued before the watermark - including legacy cookies - now
	// bounces to a fresh login instead of reaching the upstream.
	if code := serve(old); code != http.StatusFound {
		t.Fatalf("revoked session got %d, want 302 login redirect", code)
	}
	if code := serve(legacy); code != http.StatusFound {
		t.Fatalf("legacy session survived revocation: %d", code)
	}

	// A session minted after (or at) the watermark is valid.
	if code := serve(mkCookie(time.Now().Unix())); code != http.StatusOK {
		t.Fatalf("post-revocation session rejected: %d", code)
	}
}

func TestSSORevocationPersists(t *testing.T) {
	// With a state dir configured, the watermark is written next to the signing
	// key so revocation survives a restart.
	dir := t.TempDir()
	SetSSOKeyDir(dir)
	t.Cleanup(func() {
		SetSSOKeyDir("")
		ssoNotBefore.Store(0)
	})

	if err := RevokeAllSSOSessions(); err != nil {
		t.Fatalf("RevokeAllSSOSessions: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ssoNotBeforeFile))
	if err != nil {
		t.Fatalf("watermark not persisted: %v", err)
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || v <= 0 {
		t.Fatalf("persisted watermark %q unusable: %v", b, err)
	}
	if got := ssoNotBefore.Load(); got != v {
		t.Fatalf("in-memory watermark %d != persisted %d", got, v)
	}
}

// gateIdP is a minimal OIDC discovery endpoint whose response can be stalled,
// and which counts how many discoveries it served.
type gateIdP struct {
	url         string
	discoveries atomic.Int64
	stall       chan struct{} // non-nil = hold discovery until closed
	mu          sync.Mutex
}

func newGateIdP(t *testing.T) *gateIdP {
	t.Helper()
	g := &gateIdP{}
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	g.url = srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		g.discoveries.Add(1)
		g.mu.Lock()
		stall := g.stall
		g.mu.Unlock()
		if stall != nil {
			select {
			case <-stall:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"` + srv.URL + `","authorization_endpoint":"` + srv.URL +
			`/authorize","token_endpoint":"` + srv.URL + `/token","jwks_uri":"` + srv.URL +
			`/jwks","id_token_signing_alg_values_supported":["RS256"],"response_types_supported":["code"],"subject_types_supported":["public"]}`))
	})
	return g
}

func (g *gateIdP) setStall(c chan struct{}) {
	g.mu.Lock()
	g.stall = c
	g.mu.Unlock()
}

func newGate(t *testing.T, issuer, hostName string, domains []string) *dataOIDC {
	t.Helper()
	g, err := compileDataOIDC(model.IdentityProvider{
		ObjectMeta: model.ObjectMeta{Name: "idp"},
		Type:       model.IdPTypeOIDC,
		OIDC:       &model.OIDCSpec{IssuerURL: issuer, ClientID: "gpm-client"},
	}, nil, hostName, domains)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return g
}

// The per-request-Host client cache must be keyed by a CONFIGURED domain. A Host
// header the gate does not serve is refused outright: before the fix it minted a
// fresh cache entry and performed a live IdP discovery, so an attacker-chosen
// Host header both grew an unbounded map and drove outbound requests.
func TestDataOIDCRejectsUnconfiguredRequestHost(t *testing.T) {
	idp := newGateIdP(t)
	g := newGate(t, idp.url, "app", []string{"app.example.com", "alias.example.com"})
	h := g.handler(okHandler())

	serve := func(host, target string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", target, nil)
		r.Host = host
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	for _, host := range []string{"evil.example.com", "app.example.com.evil.net", ""} {
		if got := serve(host, "https://app.example.com/x"); got != http.StatusNotFound {
			t.Fatalf("unconfigured Host %q: got %d, want 404", host, got)
		}
	}
	// The callback is refused the same way.
	if got := serve("evil.example.com", "https://app.example.com"+oidcCallbackPath+"?code=a&state=b"); got != http.StatusNotFound {
		t.Fatalf("callback on an unconfigured Host: got %d, want 404", got)
	}
	if n := len(g.clients); n != 0 {
		t.Fatalf("refused hosts left %d cache entries, want 0", n)
	}
	if n := idp.discoveries.Load(); n != 0 {
		t.Fatalf("refused hosts triggered %d IdP discoveries, want 0", n)
	}

	// A configured domain is admitted, and case/port variants of it collapse onto
	// one cache entry rather than one per spelling.
	for _, host := range []string{"app.example.com", "APP.example.com", "app.example.com:443"} {
		if got := serve(host, "https://app.example.com/x"); got != http.StatusFound {
			t.Fatalf("configured Host %q: got %d, want 302", host, got)
		}
	}
	if n := len(g.clients); n != 1 {
		t.Fatalf("one domain in three spellings produced %d cache entries, want 1", n)
	}
	if n := idp.discoveries.Load(); n != 1 {
		t.Fatalf("cached client re-discovered: %d discoveries, want 1", n)
	}
	// The cache can never exceed the configured domain count.
	if got := serve("alias.example.com", "https://alias.example.com/x"); got != http.StatusFound {
		t.Fatalf("second configured domain: got %d, want 302", got)
	}
	if n := len(g.clients); n > len(g.domains) {
		t.Fatalf("cache holds %d entries for %d configured domains", n, len(g.domains))
	}
}

// Discovery must happen OUTSIDE the gate's mutex, and concurrent first requests
// for one host must share a single discovery. Before the fix a cache miss ran
// the whole network round trip under d.mu, so one slow IdP stalled every request
// on the gate - including ones whose client was already cached.
func TestDataOIDCDiscoveryDoesNotBlockCachedHosts(t *testing.T) {
	idp := newGateIdP(t)
	g := newGate(t, idp.url, "app", []string{"warm.example.com", "cold.example.com"})
	h := g.handler(okHandler())

	serve := func(host string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://"+host+"/x", nil)
		r.Host = host
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	if got := serve("warm.example.com"); got != http.StatusFound {
		t.Fatalf("warming the cache: got %d, want 302", got)
	}

	// Stall discovery, then start several concurrent first requests for the
	// cold host.
	stall := make(chan struct{})
	idp.setStall(stall)
	const concurrent = 8
	var wg sync.WaitGroup
	codes := make([]int, concurrent)
	for i := range concurrent {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = serve("cold.example.com")
		}()
	}

	// While they are all blocked on discovery, the warm host must still serve.
	warmDone := make(chan int, 1)
	go func() { warmDone <- serve("warm.example.com") }()
	select {
	case got := <-warmDone:
		if got != http.StatusFound {
			t.Errorf("cached host during a stalled discovery: got %d, want 302", got)
		}
	case <-time.After(5 * time.Second):
		t.Error("a cached host blocked behind another host's in-flight discovery: the mutex is held across it")
	}

	close(stall)
	idp.setStall(nil)
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusFound {
			t.Fatalf("concurrent request %d: got %d, want 302", i, c)
		}
	}
	// One discovery for the warm host, one shared by all the cold ones.
	if n := idp.discoveries.Load(); n != 2 {
		t.Fatalf("%d discoveries for 2 hosts and %d concurrent requests, want 2 (single-flight)", n, concurrent)
	}
}

// The first request of a shared discovery hanging up does not fail the
// discovery for the requests waiting on it.
func TestDataOIDCDiscoverySurvivesFirstCallerCancel(t *testing.T) {
	idp := newGateIdP(t)
	g := newGate(t, idp.url, "app", []string{"app.example.com"})
	h := g.handler(okHandler())
	stall := make(chan struct{})
	idp.setStall(stall)

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan struct{})
	go func() {
		defer close(first)
		r := httptest.NewRequest("GET", "https://app.example.com/x", nil).WithContext(ctx)
		r.Host = "app.example.com"
		h.ServeHTTP(httptest.NewRecorder(), r)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for idp.discoveries.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("discovery did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	second := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://app.example.com/y", nil)
		r.Host = "app.example.com"
		h.ServeHTTP(rec, r)
		second <- rec.Code
	}()
	time.Sleep(50 * time.Millisecond) // let the second request join the wait
	cancel()
	time.Sleep(50 * time.Millisecond)
	idp.setStall(nil)
	close(stall)
	<-first
	select {
	case code := <-second:
		if code != http.StatusFound {
			t.Fatalf("waiting request: got %d, want 302", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("waiting request did not complete")
	}
	if n := idp.discoveries.Load(); n != 1 {
		t.Errorf("%d discoveries, want 1 shared", n)
	}
}

// An error parameter on the callback is not echoed in the response body.
func TestDataOIDCCallbackErrorIsNotReflected(t *testing.T) {
	g := newGate(t, "http://127.0.0.1:1/not-a-real-idp", "app", []string{"app.example.com"})
	h := g.handler(okHandler())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "https://app.example.com"+oidcCallbackPath+"?error="+url.QueryEscape("<b>reflected-text</b>"), nil)
	r.Host = "app.example.com"
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "reflected-text") {
		t.Errorf("callback error text echoed in the body: %q", rec.Body.String())
	}
}

// A failed discovery must not be cached: the next request retries instead of
// serving 502 for the rest of the process's life.
func TestDataOIDCFailedDiscoveryIsNotCached(t *testing.T) {
	g := newGate(t, "http://127.0.0.1:1/not-a-real-idp", "app", []string{"app.example.com"})
	h := g.handler(okHandler())
	for i := range 2 {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://app.example.com/x", nil)
		r.Host = "app.example.com"
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("attempt %d: got %d, want 502", i, rec.Code)
		}
	}
	if n := len(g.clients); n != 0 {
		t.Fatalf("a failed discovery left %d cache entries, want 0", n)
	}
}

// With the provider's requireVerifiedEmail set, an oidc gate refuses a login
// whose email is not verified (clear 401, no session, nothing upstream) and
// admits one whose email is verified. Unset, an unverified email still signs in.
func TestOIDCGateRequireVerifiedEmail(t *testing.T) {
	login := func(t *testing.T, require bool, verified any) (*httptest.ResponseRecorder, *http.Cookie) {
		t.Helper()
		idpSrv := newMockIdP(t)
		gate, err := compileDataOIDC(model.IdentityProvider{
			ObjectMeta: model.ObjectMeta{Name: "idp"},
			Type:       model.IdPTypeOIDC,
			OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client", RequireVerifiedEmail: require},
		}, nil, "app", []string{"app.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		h := gate.handler(okHandler())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "https://app.example.com/", nil))
		sc := cookieByName(rec.Result(), oidcStateCookie)
		payload, _ := verifyToken(macLabelSSOState, sc.Value)
		var st oidcLoginState
		_ = json.Unmarshal(payload, &st)
		idpSrv.claims = map[string]any{"nonce": st.Nonce, "email": "user@example.com"}
		if verified != nil {
			idpSrv.claims["email_verified"] = verified
		}
		rec2 := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://app.example.com"+oidcCallbackPath+"?code=c&state="+url.QueryEscape(st.State), nil)
		req.AddCookie(sc)
		h.ServeHTTP(rec2, req)
		return rec2, cookieByName(rec2.Result(), oidcSessionCookie)
	}
	for _, verified := range []any{false, nil} {
		rec, sess := login(t, true, verified)
		if sess != nil || rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "not verified the email address") {
			t.Fatalf("required, email_verified=%v: status %d, session %v, body %q; want 401 saying the email is not verified and no session", verified, rec.Code, sess != nil, rec.Body.String())
		}
	}
	rec, sess := login(t, true, true)
	if sess == nil {
		t.Fatalf("required, email_verified=true: no session (status %d)", rec.Code)
	}
	var minted oidcSession
	if payload, ok := verifyToken(macLabelSSOSession, sess.Value); !ok || json.Unmarshal(payload, &minted) != nil || !minted.EmailVerified {
		t.Fatalf("required, email_verified=true: the session does not record the verified email (%+v)", minted)
	}
	if rec, sess := login(t, false, false); sess == nil {
		t.Fatalf("not required, email_verified=false: no session (status %d); the default must not change", rec.Code)
	}
}

// A session is checked against requireVerifiedEmail when it is used, not only
// at sign-in: with the option on, a session without the verified flag (minted
// before the option was turned on, or by an older build) goes back through
// sign-in and no identity reaches the upstream. With the option off such a
// session keeps working.
func TestOIDCGateRequireVerifiedEmailOnExistingSession(t *testing.T) {
	idpSrv := newMockIdP(t)
	serve := func(t *testing.T, require bool, sess oidcSession) (int, string) {
		t.Helper()
		gate, err := compileDataOIDC(model.IdentityProvider{
			ObjectMeta: model.ObjectMeta{Name: "idp"},
			Type:       model.IdPTypeOIDC,
			OIDC:       &model.OIDCSpec{IssuerURL: idpSrv.server.URL, ClientID: "gpm-client", RequireVerifiedEmail: require},
		}, nil, "app", []string{"app.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		var user string
		h := gate.handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user = r.Header.Get("X-Forwarded-User")
			w.WriteHeader(http.StatusNoContent)
		}))
		payload, _ := json.Marshal(sess)
		req := httptest.NewRequest("GET", "https://app.example.com/", nil)
		req.AddCookie(&http.Cookie{Name: oidcSessionCookie, Value: signToken(macLabelSSOSession, payload)})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, user
	}
	now := time.Now().Unix()
	old := oidcSession{Sub: "u", Email: "user@example.com", Host: "app", IdP: "idp", Exp: now + 3600, Iat: now}
	// The cookie an older build minted has no "ev" field at all.
	if b, _ := json.Marshal(old); strings.Contains(string(b), `"ev"`) {
		t.Fatalf("an unverified session marshals the verified flag: %s", b)
	}
	if code, user := serve(t, false, old); code != http.StatusNoContent || user != "user@example.com" {
		t.Fatalf("option off, session without the flag: %d user %q, want it served as before", code, user)
	}
	if code, user := serve(t, true, old); code != http.StatusFound || user != "" {
		t.Fatalf("option on, session without the flag: %d user %q, want a redirect to sign-in and nothing upstream", code, user)
	}
	verified := old
	verified.EmailVerified = true
	if code, user := serve(t, true, verified); code != http.StatusNoContent || user != "user@example.com" {
		t.Fatalf("option on, verified session: %d user %q, want it served", code, user)
	}
}

// The upstream behind an SSO gate does not receive the gate's own cookies;
// every other cookie, and the Cookie header of an auth subrequest further down
// the chain, is unchanged. A request no gate admitted is forwarded as is.
func TestSSOGateCookiesAreNotForwardedUpstream(t *testing.T) {
	var mu sync.Mutex
	var backendCookie []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		backendCookie = r.Header.Values("Cookie")
		mu.Unlock()
	}))
	t.Cleanup(backend.Close)
	target, _ := url.Parse(backend.URL)
	proxy := newProxyWith(target, http.DefaultTransport, "app", nil, nil, nil, "")

	stub := &outpostStub{authStatus: http.StatusOK}
	outpost := httptest.NewServer(stub.handler())
	t.Cleanup(outpost.Close)
	ar, err := compileAuthRequest(model.AuthRequestSpec{OutpostURL: outpost.URL})
	if err != nil {
		t.Fatalf("compileAuthRequest: %v", err)
	}
	gate := newGate(t, "http://127.0.0.1:1/not-used", "app", []string{"app.example.com"})
	gated := gate.handler(ar.handler(peerIP, nil, "app", nil, proxy))

	rec := httptest.NewRecorder()
	setSignedCookie(rec, oidcSessionCookie, macLabelSSOSession, oidcSession{
		Sub: "u", Email: "a@example.com", Host: "app", IdP: "idp",
		Iat: time.Now().Unix(), Exp: time.Now().Add(time.Hour).Unix(),
	}, time.Hour)
	sess := cookieByName(rec.Result(), oidcSessionCookie)
	if sess == nil {
		t.Fatal("no session cookie minted")
	}

	serve := func(h http.Handler, cookie string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "https://app.example.com/x", nil)
		r.Host = "app.example.com"
		r.Header.Set("Cookie", cookie)
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return backendCookie
	}

	inbound := "a=1; " + oidcSessionCookie + "=" + sess.Value + "; b=2; " + oidcStateCookie + "=s"
	if code := serve(gated, inbound); code != http.StatusOK {
		t.Fatalf("gated request: got %d, want 200", code)
	}
	if c := got(); len(c) != 1 || c[0] != "a=1; b=2" {
		t.Errorf("upstream Cookie = %q, want [\"a=1; b=2\"]", c)
	}
	if stub.gotCookie != inbound {
		t.Errorf("auth subrequest Cookie = %q, want the inbound header %q", stub.gotCookie, inbound)
	}

	if code := serve(gated, oidcSessionCookie+"="+sess.Value); code != http.StatusOK {
		t.Fatalf("gated request: got %d, want 200", code)
	}
	if c := got(); len(c) != 0 {
		t.Errorf("upstream Cookie = %q, want none when only the gate cookie was sent", c)
	}

	// No gate on the route: the header is forwarded byte for byte, a gpm-named
	// cookie included.
	for _, cookie := range []string{"a=1;b=2 ; c", "x=1; " + oidcSessionCookie + "=other"} {
		if code := serve(proxy, cookie); code != http.StatusOK {
			t.Fatalf("ungated request: got %d, want 200", code)
		}
		if c := got(); len(c) != 1 || c[0] != cookie {
			t.Errorf("ungated upstream Cookie = %q, want [%q]", c, cookie)
		}
	}
}
