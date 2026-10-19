package model

import (
	"regexp"
	"testing"
)

// uiErrorPathRe is the expression the admin UI (internal/ui/static/app.js,
// ERROR_PATH_RE) applies to a validation message to find the field it names.
// A message that does not match it cannot be jumped to.
var uiErrorPathRe = regexp.MustCompile(`^(?:[a-z][a-z ]* "[^"]*":\s*)?([A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_[\]-]+)*)\s*:\s*`)

// TestValidationErrorsLeadWithFieldPath pins the shape the UI relies on: a
// message about one field starts with the field's dotted path, optionally
// behind the object ("proxy host \"app\": ").
func TestValidationErrorsLeadWithFieldPath(t *testing.T) {
	stripped := proxyHost("app", func(h *ProxyHost) { h.StripResponseHeaders = []string{"Content-Length"} })
	badPort := proxyHost("app", func(h *ProxyHost) { h.Upstream.Port = 99999 })
	badTLS := proxyHost("app", func(h *ProxyHost) { h.TLS.MinTLSVersion = "1.1" })

	tests := []struct {
		name     string
		err      error
		wantMsg  string // exact message
		wantPath string // what the UI regex captures
	}{
		{
			name:     "upstream port out of range",
			err:      badPort.Validate(),
			wantMsg:  `proxy host "app": upstream.port: 99999 is out of range (1-65535)`,
			wantPath: "upstream.port",
		},
		{
			name:     "stripResponseHeaders entry",
			err:      stripped.Validate(),
			wantMsg:  `proxy host "app": stripResponseHeaders: "Content-Length" carries the response's own semantics and cannot be stripped`,
			wantPath: "stripResponseHeaders",
		},
		{
			name:     "nested tls field",
			err:      badTLS.Validate(),
			wantMsg:  `proxy host "app": tls.minTLSVersion: must be "1.2" or "1.3", got "1.1"`,
			wantPath: "tls.minTLSVersion",
		},
		{
			name: "domain claimed by another host",
			err: Config{ProxyHosts: []ProxyHost{
				proxyHost("notes", func(h *ProxyHost) { h.Domains = []string{"notes.example.com"} }),
				proxyHost("r2", func(h *ProxyHost) { h.Domains = []string{"notes.example.com"} }),
			}}.Validate(),
			wantMsg:  `proxy host "r2": domains: notes.example.com is already claimed by proxy host "notes" (an enabled domain may be served by exactly one host; disable one of them)`,
			wantPath: "domains",
		},
		{
			name: "stream host shares a tcp port without sniMatch",
			err: Config{StreamHosts: []StreamHost{
				streamHost("x", 5353, "tcp"),
				func() StreamHost {
					h := streamHost("y", 5353, "tcp")
					h.TLS = &StreamTLS{Mode: StreamTLSPassthrough, SNIMatch: []string{"y.example.com"}}
					return h
				}(),
			}}.Validate(),
			wantMsg:  `stream host "x": listenPort: tcp 5353 is also used by stream host "y"; set tls.sniMatch on both or pick another port`,
			wantPath: "listenPort",
		},
		{
			name: "stream host references a path-scoped access list",
			err: Config{
				AccessLists: []AccessList{{
					ObjectMeta: ObjectMeta{Name: "monitoring-probes"},
					Rules:      []IPRule{{Action: ActionAllow, CIDR: "10.0.0.0/8", Paths: []string{"/health"}}},
				}},
				StreamHosts: []StreamHost{func() StreamHost {
					h := streamHost("db", 5432, "tcp")
					h.AccessLists = []string{"monitoring-probes"}
					return h
				}()},
			}.Validate(),
			wantMsg:  `stream host "db": accessLists: "monitoring-probes" has path rules, which do not apply at L4 (path-scoped and source-backed rules need an HTTP request; use a list with only literal cidr rules and/or geo, or attach this list to a proxy host)`,
			wantPath: "accessLists",
		},
		{
			name:     "dangling reference names the referring field",
			err:      Config{ProxyHosts: []ProxyHost{proxyHost("app", func(h *ProxyHost) { h.TLS.CertificateRef = "ghost" })}}.Validate(),
			wantMsg:  `proxy host "app": tls.certificateRef: references unknown certificate "ghost"`,
			wantPath: "tls.certificateRef",
		},
		{
			name:     "settings error has no object prefix",
			err:      Settings{ExternalBaseURL: "not a url", AdminAuth: AdminAuthSettings{LocalLoginEnabled: true}}.Validate(),
			wantMsg:  `externalBaseURL: must be an absolute URL, got "not a url"`,
			wantPath: "externalBaseURL",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("expected a validation error")
			}
			if got := tc.err.Error(); got != tc.wantMsg {
				t.Fatalf("message = %q\nwant      %q", got, tc.wantMsg)
			}
			m := uiErrorPathRe.FindStringSubmatch(tc.err.Error())
			if m == nil || m[1] != tc.wantPath {
				t.Fatalf("UI path regex captured %v, want %q", m, tc.wantPath)
			}
		})
	}
}

// TestUIErrorPathRegexRejectsObjectLevelMessages documents the messages the
// UI deliberately cannot jump from: they are about the object or about a
// relationship, not one field.
func TestUIErrorPathRegexRejectsObjectLevelMessages(t *testing.T) {
	for _, msg := range []string{
		`proxy host "zzz": upstream port 99999 out of range`,
		`hosts "notes" and "r2" both claim domain notes.example.com`,
		`stream host "x" shares tcp port 5353 with "y" and sets no tls.sniMatch`,
		`access list "monitoring-probes" has path rules, which do not apply to a stream host`,
	} {
		if m := uiErrorPathRe.FindStringSubmatch(msg); m != nil {
			t.Errorf("%q unexpectedly matched with path %q", msg, m[1])
		}
	}
}
