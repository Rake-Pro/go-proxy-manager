package k8s

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAPI is a hermetic stand-in for the Kubernetes API server: a TLS httptest
// server whose own certificate is written out as the CA bundle, so the client's
// real verification path (RootCAs from a PEM file) is exercised rather than
// bypassed.
type fakeAPI struct {
	srv       *httptest.Server
	caFile    string
	tokenFile string
	// requests records the query strings of every ingress LIST, so pagination and
	// selector plumbing are assertable.
	requests []string
	// handler, when set, replaces the default list handler.
	handler http.HandlerFunc
	// seenAuth records the last Authorization header.
	seenAuth atomic.Value
}

func newFakeAPI(t *testing.T, token string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.seenAuth.Store(r.Header.Get("Authorization"))
		if strings.Contains(r.URL.Path, "/ingresses") {
			f.requests = append(f.requests, r.URL.RawQuery)
		}
		if f.handler != nil {
			f.handler(w, r)
			return
		}
		writeList(w, nil, "")
	}))
	t.Cleanup(f.srv.Close)

	dir := t.TempDir()
	f.caFile = filepath.Join(dir, "ca.crt")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.srv.Certificate().Raw})
	if err := os.WriteFile(f.caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	f.tokenFile = filepath.Join(dir, "token")
	if err := os.WriteFile(f.tokenFile, []byte(token), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	return f
}

func (f *fakeAPI) config() ClientConfig {
	return ClientConfig{APIURL: f.srv.URL, TokenFile: f.tokenFile, CAFile: f.caFile}
}

func (f *fakeAPI) client(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(f.config())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// writeList renders one LIST page, exactly as a Kubernetes API server does -
// kind and apiVersion included, because the client asserts on them.
func writeList(w http.ResponseWriter, items []string, cont string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","metadata":{"continue":%q},"items":[%s]}`,
		cont, strings.Join(items, ","))
}

// ingressJSON builds one Ingress item with the given annotations and hosts.
func ingressJSON(ns, name string, annotations map[string]string, hosts ...string) string {
	return ingressJSONWithLabels(ns, name, nil, annotations, hosts...)
}

// ingressJSONWithLabels is ingressJSON plus metadata.labels, for tests that
// exercise settings.ingressDiscovery.profileRules' matchLabels.
func ingressJSONWithLabels(ns, name string, labels, annotations map[string]string, hosts ...string) string {
	ann := make([]string, 0, len(annotations))
	for k, v := range annotations {
		ann = append(ann, fmt.Sprintf("%q:%q", k, v))
	}
	lbl := make([]string, 0, len(labels))
	for k, v := range labels {
		lbl = append(lbl, fmt.Sprintf("%q:%q", k, v))
	}
	rules := make([]string, 0, len(hosts))
	for _, h := range hosts {
		rules = append(rules, fmt.Sprintf(`{"host":%q}`, h))
	}
	return fmt.Sprintf(`{"metadata":{"name":%q,"namespace":%q,"labels":{%s},"annotations":{%s}},"spec":{"rules":[%s]}}`,
		name, ns, strings.Join(lbl, ","), strings.Join(ann, ","), strings.Join(rules, ","))
}

func TestListIngressesSendsBearerAndDecodes(t *testing.T) {
	f := newFakeAPI(t, "tok-1")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{
			ingressJSON("monitoring", "grafana", map[string]string{"gpm.rake.pro/managed": "true"}, "grafana.example.com"),
			ingressJSON("default", "plain", nil, "plain.example.com"),
		}, "")
	}
	got, err := f.client(t).ListIngresses(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Metadata.Namespace != "monitoring" || got[0].Spec.Rules[0].Host != "grafana.example.com" {
		t.Fatalf("decoded badly: %+v", got[0])
	}
	if auth, _ := f.seenAuth.Load().(string); auth != "Bearer tok-1" {
		t.Fatalf("Authorization = %q", auth)
	}
}

// An empty list is a legitimate answer and must be reported as success with zero
// items - never as an error, and never confusable with one.
func TestListIngressesEmptyIsNotAnError(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) { writeList(w, nil, "") }
	got, err := f.client(t).ListIngresses(context.Background())
	if err != nil {
		t.Fatalf("an empty list must not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d items, want 0", len(got))
	}
}

func TestListIngressesFollowsPagination(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("continue") {
		case "":
			writeList(w, []string{ingressJSON("a", "one", nil, "one.example.com")}, "page2")
		case "page2":
			writeList(w, []string{ingressJSON("a", "two", nil, "two.example.com")}, "page3")
		default:
			writeList(w, []string{ingressJSON("a", "three", nil, "three.example.com")}, "")
		}
	}
	got, err := f.client(t).ListIngresses(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3 across pages", len(got))
	}
	if len(f.requests) != 3 {
		t.Fatalf("made %d requests, want 3", len(f.requests))
	}
	if !strings.Contains(f.requests[1], "continue=page2") {
		t.Fatalf("second request did not carry the continue token: %q", f.requests[1])
	}
}

// The property the reconciler's freeze rule depends on: a page that fails after
// earlier pages succeeded discards everything and returns an error. Returning
// the partial accumulation with a nil error would look exactly like "these are
// the only Ingresses left", and would delete the rest.
func TestListIngressesPartialPageIsAnErrorNotAShortList(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("continue") == "" {
			writeList(w, []string{ingressJSON("a", "one", nil, "one.example.com")}, "page2")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"etcd unavailable"}`)
	}
	got, err := f.client(t).ListIngresses(context.Background())
	if err == nil {
		t.Fatal("a failed page must fail the whole list")
	}
	if got != nil {
		t.Fatalf("a failed list must return no items, got %d", len(got))
	}
	if !strings.Contains(err.Error(), "etcd unavailable") {
		t.Fatalf("error should carry the API message, got %v", err)
	}
}

func TestListIngressesStatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"Unauthorized"}`, "401"},
		{"forbidden", http.StatusForbidden, `{"message":"ingresses is forbidden"}`, "forbidden"},
		{"server error", http.StatusInternalServerError, `{"message":"boom"}`, "boom"},
		{"not found", http.StatusNotFound, `{"message":"the server could not find the requested resource"}`, "404"},
		{"html error page", http.StatusBadGateway, `<html>gateway</html>`, "502"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}
			got, err := f.client(t).ListIngresses(context.Background())
			if err == nil {
				t.Fatalf("status %d must be an error", tc.status)
			}
			if got != nil {
				t.Fatal("an error must carry no items")
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("error %v does not mention %q", err, tc.wantText)
			}
		})
	}
}

func TestListIngressesMalformedBodyIsAnError(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"metadata":`)
	}
	got, err := f.client(t).ListIngresses(context.Background())
	if err == nil || got != nil {
		t.Fatalf("a truncated body must be an error with no items (got %v items, err %v)", len(got), err)
	}
}

func TestListIngressesTimeoutIsAnError(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	got, err := f.client(t).ListIngresses(ctx)
	if err == nil || got != nil {
		t.Fatalf("a timeout must be an error with no items (got %d items, err %v)", len(got), err)
	}
}

// Projected ServiceAccount tokens rotate on disk. A 401 drops the cached token,
// so the very next call re-reads the file and picks up the rotated value.
func TestTokenIsRereadAfterUnauthorized(t *testing.T) {
	f := newFakeAPI(t, "old-token")
	var seen []string
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		if auth != "Bearer new-token" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Unauthorized"}`)
			return
		}
		writeList(w, nil, "")
	}
	c := f.client(t)
	if _, err := c.ListIngresses(context.Background()); err == nil {
		t.Fatal("the stale token must be rejected")
	}
	if err := os.WriteFile(f.tokenFile, []byte("new-token\n"), 0o600); err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	if _, err := c.ListIngresses(context.Background()); err != nil {
		t.Fatalf("after rotation the list must succeed: %v", err)
	}
	if len(seen) != 2 || seen[0] != "Bearer old-token" || seen[1] != "Bearer new-token" {
		t.Fatalf("tokens seen = %v", seen)
	}
}

// Within the TTL the token file is read once, not per request: the cache is what
// keeps a 60s poll from stat-ing and reading the file forever.
func TestTokenIsCachedWithinTTL(t *testing.T) {
	f := newFakeAPI(t, "tok-a")
	c := f.client(t)
	if _, err := c.ListIngresses(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := os.WriteFile(f.tokenFile, []byte("tok-b"), 0o600); err != nil {
		t.Fatalf("rewrite token: %v", err)
	}
	if _, err := c.ListIngresses(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if auth, _ := f.seenAuth.Load().(string); auth != "Bearer tok-a" {
		t.Fatalf("token should still be the cached one, got %q", auth)
	}

	// Expire the cache and the next call re-reads.
	c.mu.Lock()
	c.tokenAt = time.Now().Add(-2 * tokenTTL)
	c.mu.Unlock()
	if _, err := c.ListIngresses(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if auth, _ := f.seenAuth.Load().(string); auth != "Bearer tok-b" {
		t.Fatalf("token after TTL = %q, want the rotated one", auth)
	}
}

func TestTokenFileProblemsAreErrors(t *testing.T) {
	f := newFakeAPI(t, "tok")
	c := f.client(t)

	if err := os.WriteFile(f.tokenFile, []byte("  \n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := c.ListIngresses(context.Background()); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("an empty token file must be refused, got %v", err)
	}

	// A token is carried in a header, so control characters are a header-injection
	// vector rather than a harmless typo.
	if err := os.WriteFile(f.tokenFile, []byte("abc\r\nX-Evil: 1"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	c.dropToken()
	if _, err := c.ListIngresses(context.Background()); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("a token with control characters must be refused, got %v", err)
	}

	if err := os.Remove(f.tokenFile); err != nil {
		t.Fatalf("remove: %v", err)
	}
	c.dropToken()
	if _, err := c.ListIngresses(context.Background()); err == nil {
		t.Fatal("a missing token file must be an error")
	}
}

func TestNewClientRejectsBadConfig(t *testing.T) {
	f := newFakeAPI(t, "tok")
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.pem")
	if err := os.WriteFile(junk, []byte("not a certificate"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	tests := []struct {
		name string
		cfg  ClientConfig
		want string
	}{
		{"plain http api url", ClientConfig{APIURL: "http://k8s.example.lan:6443", TokenFile: f.tokenFile, CAFile: f.caFile}, "https"},
		{"not a url", ClientConfig{APIURL: "::::", TokenFile: f.tokenFile, CAFile: f.caFile}, "https"},
		{"missing ca", ClientConfig{APIURL: f.srv.URL, TokenFile: f.tokenFile, CAFile: filepath.Join(dir, "nope.pem")}, "read caFile"},
		{"ca is not pem", ClientConfig{APIURL: f.srv.URL, TokenFile: f.tokenFile, CAFile: junk}, "no usable PEM"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewClient(tc.cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewClient = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

// An untrusted CA must fail verification: there is no skip-verify escape hatch.
func TestClientVerifiesTheServerCertificate(t *testing.T) {
	f := newFakeAPI(t, "tok")
	other := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(other, unrelatedCAPEM(t), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	c, err := NewClient(ClientConfig{APIURL: f.srv.URL, TokenFile: f.tokenFile, CAFile: other})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.ListIngresses(context.Background()); err == nil {
		t.Fatal("a server presenting a certificate outside the configured CA must be refused")
	}
}

func TestNamespaceAndLabelSelectorNarrowTheRequest(t *testing.T) {
	f := newFakeAPI(t, "tok")
	var path string
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		writeList(w, nil, "")
	}
	cfg := f.config()
	cfg.Namespace = "monitoring"
	cfg.LabelSelector = "app.kubernetes.io/part-of=platform"
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.ListIngresses(context.Background()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if path != "/apis/networking.k8s.io/v1/namespaces/monitoring/ingresses" {
		t.Fatalf("path = %q", path)
	}
	if !strings.Contains(f.requests[0], "labelSelector=app.kubernetes.io%2Fpart-of%3Dplatform") {
		t.Fatalf("query = %q", f.requests[0])
	}
}

// Pagination that never terminates must fail loudly rather than silently
// truncating: a truncated list read as complete is what would delete managed
// hosts that should have been kept.
func TestListIngressesRefusesEndlessPagination(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		writeList(w, []string{ingressJSON("a", "one", nil, "one.example.com")}, "more")
	}
	got, err := f.client(t).ListIngresses(context.Background())
	if err == nil || got != nil {
		t.Fatalf("endless pagination must be an error with no items, got %d items / %v", len(got), err)
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error should say why it refuses: %v", err)
	}
}

func TestInClusterConfigDefaults(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	got, err := InClusterConfig(ClientConfig{})
	if err != nil {
		t.Fatalf("InClusterConfig: %v", err)
	}
	if got.APIURL != "https://10.96.0.1:443" {
		t.Fatalf("apiURL = %q", got.APIURL)
	}
	if !strings.HasSuffix(got.TokenFile, "/serviceaccount/token") || !strings.HasSuffix(got.CAFile, "/serviceaccount/ca.crt") {
		t.Fatalf("projected paths = %q / %q", got.TokenFile, got.CAFile)
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := InClusterConfig(ClientConfig{}); err == nil {
		t.Fatal("with no apiURL and no in-cluster environment, config must fail")
	}
}

// unrelatedCAPEM generates a throwaway self-signed CA that has nothing to do
// with the fake API server, so certificate verification has something real to
// reject (every httptest TLS server shares one built-in certificate).
func unrelatedCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// A 200 whose body is not an IngressList is an error at the client boundary, so
// the reconciler never sees a "complete, empty" list it would act on.
func TestListIngressesRequiresAnIngressListShape(t *testing.T) {
	for name, body := range map[string]string{
		"null":       `null`,
		"empty":      `{}`,
		"status":     `{"kind":"Status","apiVersion":"v1","status":"Success"}`,
		"null items": `{"kind":"IngressList","items":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeAPI(t, "tok")
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, body)
			}
			got, err := f.client(t).ListIngresses(context.Background())
			if err == nil {
				t.Fatalf("want an error, got %d items and nil", len(got))
			}
			if got != nil {
				t.Fatalf("an error must never come with items, got %+v", got)
			}
			if !strings.Contains(err.Error(), "IngressList") {
				t.Fatalf("error should name the expected shape, got %v", err)
			}
		})
	}
}

// A response larger than the body cap is reported as exactly that. Silently
// truncating it produced a JSON syntax error instead, which reads as "the API
// server is broken" and freezes discovery until somebody works out that one
// object simply carried too many annotations.
func TestOversizedResponseIsReportedAsSuch(t *testing.T) {
	f := newFakeAPI(t, "tok")
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"kind":"IngressList","metadata":{"continue":""},"items":[`))
		chunk := strings.Repeat("x", 64<<10)
		for written := 0; written < maxRespBody; written += len(chunk) {
			w.Write([]byte(chunk))
		}
	}
	_, err := f.client(t).ListIngresses(context.Background())
	if err == nil {
		t.Fatal("an oversized response must be an error")
	}
	if !strings.Contains(err.Error(), "body cap") {
		t.Fatalf("error must distinguish an oversized body from a malformed one, got %v", err)
	}
}

// The page size has to stay well under the body cap: a real Ingress carries
// managedFields and arbitrary annotations, so a large page is one verbose tenant
// manifest away from overflowing it.
func TestListPageSizeLeavesHeadroomUnderTheBodyCap(t *testing.T) {
	const generousObjectBytes = 64 << 10
	if listPageSize*generousObjectBytes > maxRespBody {
		t.Fatalf("listPageSize %d x %d bytes exceeds the %d-byte body cap", listPageSize, generousObjectBytes, maxRespBody)
	}
}
