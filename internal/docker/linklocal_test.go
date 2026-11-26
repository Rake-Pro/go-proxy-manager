package docker

import (
	"strings"
	"testing"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

func withNetworks(nets map[string]string) func(*Container) {
	return func(c *Container) {
		c.NetworkSettings.Networks = map[string]struct {
			IPAddress string `json:"IPAddress"`
		}{}
		for name, ip := range nets {
			c.NetworkSettings.Networks[name] = struct {
				IPAddress string `json:"IPAddress"`
			}{IPAddress: ip}
		}
	}
}

// A link-local container address is refused by default, the way loopback and
// unspecified addresses are, and accepted again with allowLinkLocal.
func TestDeriveLinkLocalAddress(t *testing.T) {
	allow := func(s *model.Settings) { s.DockerDiscovery.AllowLinkLocal = true }
	onNet := func(s *model.Settings) { s.DockerDiscovery.Network = "bridge" }
	tests := []struct {
		name     string
		conf     model.DockerDiscoverySettings
		nets     map[string]string
		wantHost string
		wantErr  string
	}{
		{"ipv4 link-local on the configured network", testConf(onNet),
			map[string]string{"bridge": "169.254.10.20"}, "", "link-local IP address on network \"bridge\""},
		{"ipv6 link-local on the configured network", testConf(onNet),
			map[string]string{"bridge": "fe80::1"}, "", "link-local IP address on network \"bridge\""},
		{"only link-local with no configured network", testConf(nil),
			map[string]string{"bridge": "169.254.10.20"}, "", "only link-local IP addresses"},
		{"link-local network is passed over for a usable one", testConf(nil),
			map[string]string{"a-linklocal": "169.254.10.20", "b-lan": "192.0.2.10"}, "192.0.2.10", ""},
		{"allowLinkLocal on the configured network", testConf(func(s *model.Settings) { onNet(s); allow(s) }),
			map[string]string{"bridge": "169.254.10.20"}, "169.254.10.20", ""},
		{"allowLinkLocal with no configured network keeps the first network", testConf(allow),
			map[string]string{"a-linklocal": "169.254.10.20", "b-lan": "192.0.2.10"}, "169.254.10.20", ""},
		{"allowLinkLocal still refuses loopback", testConf(func(s *model.Settings) { onNet(s); allow(s) }),
			map[string]string{"bridge": "127.0.0.1"}, "", "unusable IP address"},
		{"an address next to the link-local range is usable", testConf(onNet),
			map[string]string{"bridge": "169.255.0.1"}, "169.255.0.1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, host, _, err := derive(container("app", enabled(nil), withNetworks(tc.nets)), tc.conf)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if host.Upstream.Host != tc.wantHost {
				t.Fatalf("upstream host %q, want %q", host.Upstream.Host, tc.wantHost)
			}
		})
	}
}

// A container reporting a link-local address is skipped with the reason in the
// status; a host already stored for it is kept as it is (the rule for any
// container that can no longer be derived), not deleted and not rewritten.
func TestPlanReconcileLinkLocalContainer(t *testing.T) {
	conf := testConf(nil)
	ct := container("app", enabled(nil), withNetworks(map[string]string{"bridge": "169.254.10.20"}))

	t.Run("new container is skipped", func(t *testing.T) {
		p := planReconcile(model.Config{}, conf, []Container{ct}, nil)
		if len(p.Upserts) != 0 || p.Skipped != 1 {
			t.Fatalf("plan %+v, want one skip and no write", p)
		}
		h := hostResults(p)[0]
		if h.Action != ActionSkipped || !strings.Contains(h.Reason, "link-local") || h.Container != "app" {
			t.Fatalf("host result %+v, want a skip naming the link-local address", h)
		}
	})

	t.Run("stored host is frozen, not deleted", func(t *testing.T) {
		stored := managedHostFixture("dkr-app", func(h *model.ProxyHost) { h.Upstream.Host = "169.254.10.20" })
		p := planReconcile(model.Config{ProxyHosts: []model.ProxyHost{stored}}, conf, []Container{ct}, nil)
		if len(p.Upserts) != 0 || len(p.Deletes) != 0 || p.Skipped != 1 {
			t.Fatalf("plan %+v, want the stored host left as it is", p)
		}
	})
}
