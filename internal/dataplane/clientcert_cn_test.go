package dataplane

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// The bare-common-name fallback of clientCertRoles is skipped for a CN that
// contains '=': such a CN can spell another certificate's full RFC 2253
// subject, and must not pick up the role mapped to that subject.
func TestClientCertRoleFallbackSkipsCNWithEquals(t *testing.T) {
	_, caCert, caKey := crlCA(t, "corp-ca")
	cases := []struct {
		name  string
		cn    string
		roles map[string]string
		want  int
	}{
		{"CN spelling another subject does not match it", "CN=admin,O=Corp", map[string]string{"CN=admin,O=Corp": "admin"}, http.StatusForbidden},
		{"the real subject still matches", "admin", map[string]string{"CN=admin,O=Corp": "admin"}, http.StatusOK},
		{"plain bare CN still matches", "ops", map[string]string{"ops": "admin"}, http.StatusOK},
		{"bare CN containing = no longer matches", "a=b", map[string]string{"a=b": "admin"}, http.StatusForbidden},
		{"full subject of a CN containing = matches", "a=b", map[string]string{"CN=a=b,O=Corp": "admin"}, http.StatusOK},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, leaf := clientCertSerial(t, caCert, caKey, int64(100+i), tc.cn)
			spec := model.AuthMiddleware{Mode: model.AuthModeClientCert, ClientCertRoles: tc.roles, RequiredRoles: []string{"admin"}}
			h := clientCertGate(spec, peerIP, nil, "app", nil, okHandler())
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "https://app.example.com/", nil)
			req.RemoteAddr = "203.0.113.5:5000"
			req.TLS = &tls.ConnectionState{
				ServerName:       "app.example.com",
				PeerCertificates: []*x509.Certificate{leaf},
				VerifiedChains:   [][]*x509.Certificate{{leaf, caCert}},
			}
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("subject %q: got %d, want %d", leaf.Subject.String(), rec.Code, tc.want)
			}
		})
	}
}
