package model

import (
	"strings"
	"testing"
)

func dockerBase() DockerDiscoverySettings {
	return DockerDiscoverySettings{
		Enabled:               true,
		AllowedDomainSuffixes: []string{"example.com"},
		Template: IngressHostTemplate{
			TLS: TLSSettings{CertificateRef: "wildcard"},
		},
	}
}

func TestDockerDiscoveryValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*DockerDiscoverySettings)
		wantErr string
	}{
		{"defaults are valid", nil, ""},
		{"disabled is never validated", func(d *DockerDiscoverySettings) {
			d.Enabled = false
			d.AllowedDomainSuffixes = nil
			d.Template = IngressHostTemplate{}
		}, ""},
		{"allowedDomainSuffixes is required", func(d *DockerDiscoverySettings) {
			d.AllowedDomainSuffixes = nil
		}, "allowedDomainSuffixes is required"},
		{"certificateRef is required", func(d *DockerDiscoverySettings) {
			d.Template.TLS.CertificateRef = ""
		}, "dockerDiscovery.template.tls.certificateRef is required"},
		{"relative socket", func(d *DockerDiscoverySettings) {
			d.Socket = "docker.sock"
		}, "socket must be an absolute path"},
		{"absolute socket is fine", func(d *DockerDiscoverySettings) {
			d.Socket = "/run/docker.sock"
		}, ""},
		{"host must be a URL", func(d *DockerDiscoverySettings) {
			d.Host = "socket-proxy:2375"
		}, "must be an absolute tcp:// or https:// URL"},
		{"tcp host is fine", func(d *DockerDiscoverySettings) {
			d.Host = "tcp://socket-proxy:2375"
		}, ""},
		{"client certs need https", func(d *DockerDiscoverySettings) {
			d.Host = "tcp://socket-proxy:2375"
			d.TLSCert, d.TLSKey = "/etc/gpm/c.pem", "/etc/gpm/k.pem"
		}, "need an https:// host"},
		{"cert without key", func(d *DockerDiscoverySettings) {
			d.Host = "https://docker.example.com:2376"
			d.TLSCert = "/etc/gpm/c.pem"
		}, "must be set together"},
		{"relative tls path", func(d *DockerDiscoverySettings) {
			d.TLSCA = "ca.pem"
		}, "tlsCA must be an absolute path"},
		{"publishedHost without usePublishedPorts", func(d *DockerDiscoverySettings) {
			d.PublishedHost = "127.0.0.1"
		}, "only applies with"},
		{"network and usePublishedPorts are exclusive", func(d *DockerDiscoverySettings) {
			d.Network = "edge"
			d.UsePublishedPorts = true
		}, "mutually exclusive"},
		{"pollInterval floor", func(d *DockerDiscoverySettings) {
			d.PollInterval = "1s"
		}, "must be at least 15s"},
		{"pollInterval must parse", func(d *DockerDiscoverySettings) {
			d.PollInterval = "soon"
		}, "must be a Go duration"},
		{"bad domain suffix", func(d *DockerDiscoverySettings) {
			d.AllowedDomainSuffixes = []string{"not a domain"}
		}, "is not a valid domain suffix"},
		{"profile may not be named template", func(d *DockerDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"template": {TLS: TLSSettings{CertificateRef: "wildcard"}}}
		}, "reserved for the default template block"},
		{"a profile may only narrow the global suffix list", func(d *DockerDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"public": {
				TLS:                   TLSSettings{CertificateRef: "wildcard"},
				AllowedDomainSuffixes: []string{"other.test"},
			}}
		}, "may only narrow the global list"},
		{"a shared profile carrying an ingress upstream is accepted", func(d *DockerDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso": {
				TLS:      TLSSettings{CertificateRef: "wildcard"},
				Upstream: Upstream{Scheme: "http", Host: "10.0.0.1", Port: 80},
			}}
		}, ""},
		{"a shared profile with a malformed upstream still fails", func(d *DockerDiscoverySettings) {
			d.Profiles = map[string]IngressHostTemplate{"sso": {
				TLS:      TLSSettings{CertificateRef: "wildcard"},
				Upstream: Upstream{Scheme: "gopher", Host: "10.0.0.1", Port: 80},
			}}
		}, "upstream scheme must be http or https"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := dockerBase()
			if tc.mutate != nil {
				tc.mutate(&d)
			}
			err := d.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v, want one containing %q", err, tc.wantErr)
			}
			// Every message must name the block the operator actually broke.
			if strings.Contains(err.Error(), "ingressDiscovery.") {
				t.Fatalf("error %q names the wrong settings block", err)
			}
		})
	}
}

// The docker block inherits the label prefix and (by default) the profile set
// from ingressDiscovery, so a deployment states each of them once.
func TestDockerDiscoveryResolved(t *testing.T) {
	shared := map[string]IngressHostTemplate{"sso": {TLS: TLSSettings{CertificateRef: "wildcard"}}}
	own := map[string]IngressHostTemplate{"containers-only": {TLS: TLSSettings{CertificateRef: "wildcard"}}}

	tests := []struct {
		name         string
		settings     Settings
		wantPrefix   string
		wantProfiles []string
	}{
		{
			name:         "default prefix and inherited profiles",
			settings:     Settings{IngressDiscovery: IngressDiscoverySettings{Profiles: shared}},
			wantPrefix:   "gpm.rake.pro",
			wantProfiles: []string{"sso"},
		},
		{
			name: "custom prefix is shared",
			settings: Settings{IngressDiscovery: IngressDiscoverySettings{
				AnnotationPrefix: "edge.example.com",
				Profiles:         shared,
			}},
			wantPrefix:   "edge.example.com",
			wantProfiles: []string{"sso"},
		},
		{
			name: "own profiles win when set",
			settings: Settings{
				IngressDiscovery: IngressDiscoverySettings{Profiles: shared},
				DockerDiscovery:  DockerDiscoverySettings{Profiles: own},
			},
			wantPrefix:   "gpm.rake.pro",
			wantProfiles: []string{"containers-only"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.settings.DockerDiscoveryResolved()
			if d.Prefix() != tc.wantPrefix {
				t.Fatalf("prefix %q, want %q", d.Prefix(), tc.wantPrefix)
			}
			if d.LabelEnabled() != tc.wantPrefix+"/enabled" || d.ManagedByLabel() != tc.wantPrefix+"/managed-by" {
				t.Fatalf("label keys %q/%q are not under the resolved prefix", d.LabelEnabled(), d.ManagedByLabel())
			}
			if strings.Join(d.ProfileNames(), ",") != strings.Join(tc.wantProfiles, ",") {
				t.Fatalf("profiles %v, want %v", d.ProfileNames(), tc.wantProfiles)
			}
		})
	}
}

// The two reconcilers share the managed-by KEY and are told apart by its value.
// Neither may recognise the other's hosts as its own, and neither may see the
// other's labels as "stale" during a prefix migration.
func TestDiscoveryOwnershipValuesDoNotOverlap(t *testing.T) {
	docker := Settings{}.DockerDiscoveryResolved()
	ingress := IngressDiscoverySettings{}

	if docker.ManagedByLabel() != ingress.ManagedByLabel() {
		t.Fatalf("managed-by keys differ (%q vs %q); they are meant to share one key",
			docker.ManagedByLabel(), ingress.ManagedByLabel())
	}
	if ManagedByDockerDiscovery == ManagedByIngressDiscovery {
		t.Fatal("the two reconcilers use the same managed-by value; they could delete each other's hosts")
	}

	dockerLabels := map[string]string{"old.example.com/managed-by": ManagedByDockerDiscovery}
	ingressLabels := map[string]string{"old.example.com/managed-by": ManagedByIngressDiscovery}
	if !docker.HasStaleManagedByLabel(dockerLabels) {
		t.Fatal("docker discovery does not recognise its own stale label")
	}
	if docker.HasStaleManagedByLabel(ingressLabels) {
		t.Fatal("docker discovery claims an ingress-discovery label as its own stale label")
	}
	if !ingress.HasStaleManagedByLabel(ingressLabels) {
		t.Fatal("ingress discovery does not recognise its own stale label")
	}
	if ingress.HasStaleManagedByLabel(dockerLabels) {
		t.Fatal("ingress discovery claims a docker-discovery label as its own stale label")
	}

	// A strip during a migration must leave the other reconciler's labels alone.
	mixed := map[string]string{
		"old.example.com/managed-by": ManagedByDockerDiscovery,
		"old.example.com/labelled":   ManagedByIngressDiscovery,
	}
	ingress.StripStaleDiscoveryLabels(mixed)
	if len(mixed) != 2 {
		t.Fatalf("labels %v, want ingress discovery to have stripped nothing of docker's", mixed)
	}
}

func TestDockerDiscoveryValidateRefs(t *testing.T) {
	cfg := Config{
		Certificates: []Certificate{{ObjectMeta: ObjectMeta{Name: "wildcard"}}},
	}
	tests := []struct {
		name    string
		mutate  func(*DockerDiscoverySettings)
		wantErr string
	}{
		{"resolvable refs", nil, ""},
		{"dangling certificate", func(d *DockerDiscoverySettings) {
			d.Template.TLS.CertificateRef = "missing"
		}, `unknown certificate "missing"`},
		{"dangling middleware", func(d *DockerDiscoverySettings) {
			d.Template.Middlewares = []string{"sso"}
		}, `unknown middleware "sso"`},
		{"disabled block is not checked", func(d *DockerDiscoverySettings) {
			d.Enabled = false
			d.Template.TLS.CertificateRef = "missing"
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := dockerBase()
			if tc.mutate != nil {
				tc.mutate(&d)
			}
			err := d.ValidateRefs(cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v, want one containing %q", err, tc.wantErr)
			}
		})
	}
}
