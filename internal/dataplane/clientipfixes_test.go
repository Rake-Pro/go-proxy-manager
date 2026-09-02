package dataplane

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/clientip"
	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// TestEmptyHostTrustedProxiesTrustsNobody is the regression for the documented
// "trust nobody" override: a host whose trustedProxies is present but EMPTY must
// not fall back to the fleet-wide list. The failure direction was trusting more
// than the operator asked for, through the exact instruction the field
// documents, so it is asserted end to end against an access list.
func TestEmptyHostTrustedProxiesTrustsNobody(t *testing.T) {
	withGlobalTrustedProxies(t, "192.0.2.10/32")

	up, closeFn := backendUpstream(t, okHandler())
	defer closeFn()

	host := func(trusted *[]string) model.ProxyHost {
		return model.ProxyHost{
			ObjectMeta:     model.ObjectMeta{Name: "app"},
			Domains:        []string{"app.example.com"},
			Upstream:       up,
			AccessLists:    []string{"lan"},
			TrustedProxies: trusted,
		}
	}
	cfgFor := func(h model.ProxyHost) model.Config {
		return model.Config{
			AccessLists: []model.AccessList{{
				ObjectMeta:    model.ObjectMeta{Name: "lan"},
				DefaultAction: model.ActionDeny,
				Rules:         []model.IPRule{{Action: model.ActionAllow, CIDR: "10.0.0.0/8"}},
			}},
			ProxyHosts: []model.ProxyHost{h},
		}
	}

	tests := []struct {
		name    string
		trusted *[]string
		want    int
	}{
		{
			name:    "present but empty trusts nobody, so the forwarded client is ignored",
			trusted: &[]string{},
			want:    http.StatusForbidden,
		},
		{
			name:    "absent inherits the fleet list, so the forwarded client is believed",
			trusted: nil,
			want:    http.StatusOK,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := buildRouter(cfgFor(host(tc.trusted)), "", nil)
			if err != nil {
				t.Fatalf("buildRouter: %v", err)
			}
			// The peer is the fleet-wide trusted proxy; it claims a LAN client.
			if got := serveApp(rt, "192.0.2.10:4444", "10.1.2.3"); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestUnparseableForwardedEntryEndsTheWalk: a token the rightmost-untrusted walk
// cannot read is evidence the chain is not the one gpm expects, not a licence to
// believe the entry to its left. The client would otherwise pick its own address
// by putting it immediately left of an obfuscated node identifier ("_hidden"),
// the literal "unknown", or a "unix:" marker - all of which real proxies emit.
func TestUnparseableForwardedEntryEndsTheWalk(t *testing.T) {
	trusted := mustNets("192.0.2.10/32")
	tests := []struct {
		name, xff, want string
	}{
		{"obfuscated node identifier", "10.1.2.3, _hidden", "192.0.2.10"},
		{"literal unknown", "10.1.2.3, unknown", "192.0.2.10"},
		{"unix socket marker", "10.1.2.3, unix:", "192.0.2.10"},
		{"clean chain still walks", "10.1.2.3, 192.0.2.10", "10.1.2.3"},
		{"unspecified address is not an address", "10.1.2.3, 0.0.0.0", "192.0.2.10"},
		{"broadcast address is not an address", "10.1.2.3, 255.255.255.255", "192.0.2.10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = "192.0.2.10:443"
			r.Header.Set("X-Forwarded-For", tc.xff)
			got := deriveClientIP(r, trusted).ip
			if got == nil || got.String() != tc.want {
				t.Fatalf("derived %v, want %s", got, tc.want)
			}
		})
	}
}

// TestForwardedChainIsBounded: the walk and the rebuilt chain look at the
// rightmost entries only, so a client padding X-Forwarded-For to thousands of
// entries buys bounded work rather than linear work per request.
func TestForwardedChainIsBounded(t *testing.T) {
	var parts []string
	for i := 0; i < clientip.MaxForwardedEntries*4; i++ {
		parts = append(parts, "192.0.2.10")
	}
	parts = append(parts, "10.1.2.3") // rightmost, untrusted
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.10:443"
	r.Header.Set("X-Forwarded-For", strings.Join(parts, ", "))

	if got := len(clientip.ForwardedChain(r)); got != clientip.MaxForwardedEntries {
		t.Fatalf("chain length = %d, want it capped at %d", got, clientip.MaxForwardedEntries)
	}
	if got := deriveClientIP(r, mustNets("192.0.2.10/32")).ip; got == nil || got.String() != "10.1.2.3" {
		t.Fatalf("derived %v, want the rightmost untrusted entry", got)
	}
}

// TestUpstreamForwardedChainDropsGarbage: what gpm re-emits upstream is rebuilt
// from parsed addresses only, so a backend reading the LEFTMOST X-Forwarded-For
// entry can never be handed attacker-chosen text, and the chain it sees is
// bounded.
func TestUpstreamForwardedChainDropsGarbage(t *testing.T) {
	withGlobalTrustedProxies(t, "192.0.2.10/32")

	var gotXFF, gotReal string
	up, closeFn := backendUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		gotReal = r.Header.Get("X-Real-Ip")
		w.WriteHeader(http.StatusOK)
	}))
	defer closeFn()

	cfg := model.Config{ProxyHosts: []model.ProxyHost{{
		ObjectMeta: model.ObjectMeta{Name: "app"},
		Domains:    []string{"app.example.com"},
		Upstream:   up,
	}}}
	rt, err := buildRouter(cfg, "", nil)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	if got := serveApp(rt, "192.0.2.10:4444", "not-an-ip, 10.1.2.3"); got != http.StatusOK {
		t.Fatalf("status = %d", got)
	}
	if strings.Contains(gotXFF, "not-an-ip") {
		t.Fatalf("X-Forwarded-For = %q, want the unparseable entry dropped", gotXFF)
	}
	if !strings.HasPrefix(gotXFF, "10.1.2.3") {
		t.Fatalf("X-Forwarded-For = %q, want it to start with the genuine client entry", gotXFF)
	}
	if gotReal != "10.1.2.3" {
		t.Fatalf("X-Real-Ip = %q, want 10.1.2.3", gotReal)
	}
}

// TestClientKeyUsesDerivedIP: ip-hash stickiness must key on the derived client,
// not on the peer - behind a trusted proxy every request shares one peer, which
// would pin the entire fleet to a single upstream.
func TestClientKeyUsesDerivedIP(t *testing.T) {
	withGlobalTrustedProxies(t, "192.0.2.10/32")

	keys := map[string]bool{}
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.10:443"
		r.Header.Set("X-Forwarded-For", "10.1.2."+strconv.Itoa(i))
		info := deriveClientIP(r, currentTrustedProxies())
		keys[clientKey(withClientIP(r, info))] = true
	}
	if len(keys) != 3 {
		t.Fatalf("clientKey produced %d distinct keys for 3 forwarded clients, want 3", len(keys))
	}
}
