package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestClientCertGateAllowFrom covers the AllowFrom network exemption in
// client-cert mode: it is exactly the auth-request semantic, so a client on an
// exempt network is proxied straight through with no certificate requirement and
// no role check, while everyone else still needs a handshake-verified
// certificate.
func TestClientCertGateAllowFrom(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp-ca")
	_, ops := clientCertSerial(t, caCert, caKey, 90, "ops")

	req := func(remote string, leaf *x509.Certificate) *http.Request {
		r := httptest.NewRequest("GET", "https://m.example/", nil)
		r.RemoteAddr = remote
		if leaf != nil {
			r.TLS = &tls.ConnectionState{
				ServerName:       "m.example",
				PeerCertificates: []*x509.Certificate{leaf},
				VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
			}
		}
		return r
	}
	lan := model.AuthMiddleware{Mode: model.AuthModeClientCert, AllowFrom: []string{"10.0.0.0/8"}}
	roled := model.AuthMiddleware{
		Mode:            model.AuthModeClientCert,
		AllowFrom:       []string{"10.0.0.0/8"},
		RequiredRoles:   []string{"admin"},
		ClientCertRoles: map[string]string{"ops": "admin"},
	}

	cases := []struct {
		name string
		spec model.AuthMiddleware
		req  *http.Request
		want int
	}{
		{"exempt IP, no certificate", lan, req("10.1.2.3:5000", nil), http.StatusOK},
		{"non-exempt IP, no certificate", lan, req("203.0.113.5:5000", nil), http.StatusUnauthorized},
		{"non-exempt IP, verified certificate", lan, req("203.0.113.5:5000", ops), http.StatusOK},
		// The exemption is decided before the role check, so a certless LAN client
		// is admitted even though it could never map to a role.
		{"exempt IP is not role-checked", roled, req("10.1.2.3:5000", nil), http.StatusOK},
		// A certificate whose subject does not map still fails off the LAN.
		{"non-exempt IP, unmapped subject", model.AuthMiddleware{
			Mode: model.AuthModeClientCert, AllowFrom: []string{"10.0.0.0/8"},
			RequiredRoles:   []string{"admin"},
			ClientCertRoles: map[string]string{"someone-else": "admin"},
		}, req("203.0.113.5:5000", ops), http.StatusForbidden},
		// With no AllowFrom the gate behaves exactly as before.
		{"no exemption configured", model.AuthMiddleware{Mode: model.AuthModeClientCert},
			req("10.1.2.3:5000", nil), http.StatusUnauthorized},
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			clientCertGate(tc.spec, peerIP, allowFromNets(tc.spec.AllowFrom), ok).ServeHTTP(w, tc.req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// TestClientCertGateAllowFromTrustedProxy proves the exemption is evaluated
// against the resolved client IP, not the connection peer: behind a trusted
// proxy the X-Forwarded-For entry decides, and an untrusted peer's forged header
// is ignored.
func TestClientCertGateAllowFromTrustedProxy(t *testing.T) {
	resolve := clientIPResolver(mustNets("192.0.2.10/32"))
	spec := model.AuthMiddleware{Mode: model.AuthModeClientCert, AllowFrom: []string{"10.0.0.0/8"}}
	nets := allowFromNets(spec.AllowFrom)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	cases := []struct {
		name   string
		remote string
		xff    string
		want   int
	}{
		{"trusted proxy forwards a LAN client", "192.0.2.10:443", "10.1.2.3", http.StatusOK},
		{"trusted proxy forwards a WAN client", "192.0.2.10:443", "203.0.113.5", http.StatusUnauthorized},
		{"untrusted peer cannot forge the LAN", "203.0.113.5:443", "10.1.2.3", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://m.example/", nil)
			r.RemoteAddr = tc.remote
			r.Header.Set("X-Forwarded-For", tc.xff)
			w := httptest.NewRecorder()
			clientCertGate(spec, resolve, nets, ok).ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// TestClientCertGateAllowFromCarriesNoIdentity proves a certless exempt request
// reaches the upstream with no client-certificate identity headers: the
// passthrough only ever fires from a handshake-VERIFIED certificate, and an
// exempt client has none, so a forged header is stripped and nothing replaces it.
func TestClientCertGateAllowFromCarriesNoIdentity(t *testing.T) {
	rt, got := certPassthroughRouter(t, &model.ClientCertHeaders{SAN: true, Serial: true, Fingerprint: true})
	r := httptest.NewRequest("GET", "https://m.example/", nil)
	r.Host = "m.example"
	r.RemoteAddr = "10.1.2.3:5000"
	for _, h := range []string{
		model.DefaultClientCertSubjectHeader, model.ClientCertSANHeader,
		model.ClientCertSerialHeader, model.ClientCertFingerprintHeader,
	} {
		r.Header.Set(h, "forged")
	}
	// No r.TLS peer certificate: an exempt, certless client.
	w := httptest.NewRecorder()
	spec := model.AuthMiddleware{Mode: model.AuthModeClientCert, AllowFrom: []string{"10.0.0.0/8"}}
	gate := clientCertGate(spec, peerIP, allowFromNets(spec.AllowFrom), http.HandlerFunc(rt.serveHTTPS))
	gate.ServeHTTP(w, r)

	for _, h := range []string{
		model.DefaultClientCertSubjectHeader, model.ClientCertSANHeader,
		model.ClientCertSerialHeader, model.ClientCertFingerprintHeader,
	} {
		if v := got.Get(h); v != "" {
			t.Fatalf("%s reached the upstream as %q; a certless exempt request must carry no identity", h, v)
		}
	}
}

// TestClientCertGateAllowFromWiredFromMiddleware proves the exemption survives
// the normal auth-middleware dispatch, i.e. authMiddlewareHandler threads both
// the client-IP resolver and the parsed AllowFrom nets into the gate.
func TestClientCertGateAllowFromWiredFromMiddleware(t *testing.T) {
	mw := model.Middleware{
		ObjectMeta: model.ObjectMeta{Name: "cert"},
		Type:       model.MWTypeAuth,
		Auth: &model.AuthMiddleware{
			Mode:      model.AuthModeClientCert,
			AllowFrom: []string{"10.0.0.0/8"},
		},
	}
	h := authMiddlewareHandler(mw, buildRegistry(model.Config{}), "m", []string{"m.example"},
		clientIPResolver(nil),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	for _, tc := range []struct {
		remote string
		want   int
	}{
		{"10.1.2.3:5000", http.StatusOK},
		{"203.0.113.5:5000", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest("GET", "https://m.example/", nil)
		r.RemoteAddr = tc.remote
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: status %d, want %d", tc.remote, w.Code, tc.want)
		}
	}
}
