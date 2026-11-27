package model

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDNSProviderPlainKey(t *testing.T) {
	plain := map[string][]string{
		DNSProviderRFC2136: {"server", "zone", "tsigKeyName", "tsigAlgorithm", "transport", "ttl", "timeout"},
		DNSProviderACMEDNS: {"baseURL", "subdomain", "allowInsecureLocal"},
		DNSProviderWebhook: {"url", "allowInsecureLocal"},
	}
	secret := map[string][]string{
		DNSProviderRFC2136:      {"tsigSecret"},
		DNSProviderACMEDNS:      {"username", "password"},
		DNSProviderWebhook:      {"token"},
		DNSProviderCloudflare:   {"apiToken"},
		DNSProviderDigitalOcean: {"apiToken"},
		DNSProviderHetzner:      {"apiToken"},
		DNSProviderDesec:        {"apiToken"},
	}
	for _, p := range KnownDNSProviders {
		if _, ok := secret[p]; !ok {
			t.Errorf("provider %q has no row in this test; classify its keys", p)
		}
	}
	for p, keys := range plain {
		for _, k := range keys {
			if !DNSProviderPlainKey(p, k) {
				t.Errorf("%s %s: secret, want plain", p, k)
			}
		}
	}
	for p, keys := range secret {
		for _, k := range append(keys, "unknownKey", "") {
			if DNSProviderPlainKey(p, k) {
				t.Errorf("%s %q: plain, want secret", p, k)
			}
		}
		// Keys plain for another provider stay secret here.
		for _, other := range plain {
			for _, k := range other {
				if !slices.Contains(plain[p], k) && DNSProviderPlainKey(p, k) {
					t.Errorf("%s %s: plain, want secret (plain only for another provider)", p, k)
				}
			}
		}
	}
	for _, k := range []string{"server", "url", "baseURL", "apiToken"} {
		if DNSProviderPlainKey("route53", k) || DNSProviderPlainKey("", k) {
			t.Errorf("unknown provider %s: plain, want secret", k)
		}
	}
	// Case matters: a key the solver does not read is not a setting.
	if DNSProviderPlainKey(DNSProviderWebhook, "URL") {
		t.Error("webhook URL (upper case): plain, want secret")
	}
}

func literalProviders() []DNSProvider {
	return []DNSProvider{
		{ObjectMeta: ObjectMeta{Name: "ns"}, Provider: DNSProviderRFC2136, Config: map[string]Secret{
			"server": "ns1.example.com:53", "zone": "example.com", "tsigKeyName": "gpm-key", "tsigAlgorithm": "hmac-sha256",
			"transport": "tcp", "ttl": "60", "timeout": "30s", "tsigSecret": "${ENV:GPM_TSIG}",
		}},
		{ObjectMeta: ObjectMeta{Name: "adns"}, Provider: DNSProviderACMEDNS, Config: map[string]Secret{
			"baseURL": "http://127.0.0.1:8053", "subdomain": "d420c923", "allowInsecureLocal": "true",
			"username": "${ENV:ADNS_USER}", "password": "${FILE:/run/secrets/adns}",
		}},
		{ObjectMeta: ObjectMeta{Name: "hook"}, Provider: DNSProviderWebhook, Config: map[string]Secret{
			"url": "http://127.0.0.1:9000/dns", "allowInsecureLocal": "true", "token": "${ENV:HOOK_TOKEN}",
		}},
	}
}

func TestLiteralSecretsDNSProviderPlainKeys(t *testing.T) {
	providers := literalProviders()
	for _, p := range providers {
		if err := p.Validate(); err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
	}
	// Every place a DNSProvider can sit in a walked value.
	ptr := providers[2]
	for name, v := range map[string]any{
		"config":  Config{DNSProviders: providers},
		"slice":   providers,
		"value":   providers[0],
		"pointer": &ptr,
		"in any":  []any{providers[1]},
	} {
		if lits := LiteralSecrets(v); len(lits) != 0 {
			t.Errorf("%s: LiteralSecrets = %v, want none for literal plain keys", name, lits)
		}
	}

	cases := []struct {
		name string
		p    DNSProvider
		want string
	}{
		{"literal tsigSecret", DNSProvider{Provider: DNSProviderRFC2136, Config: map[string]Secret{"server": "ns1.example.com", "tsigSecret": "c2VjcmV0"}}, "DNSProviders[0].Config[tsigSecret]"},
		{"literal password", DNSProvider{Provider: DNSProviderACMEDNS, Config: map[string]Secret{"baseURL": "https://adns.example.com", "password": "pw"}}, "DNSProviders[0].Config[password]"},
		{"literal username", DNSProvider{Provider: DNSProviderACMEDNS, Config: map[string]Secret{"username": "u"}}, "DNSProviders[0].Config[username]"},
		{"literal token", DNSProvider{Provider: DNSProviderWebhook, Config: map[string]Secret{"url": "https://hook.example.com", "token": "t"}}, "DNSProviders[0].Config[token]"},
		{"literal apiToken", DNSProvider{Provider: DNSProviderCloudflare, Config: map[string]Secret{"apiToken": "t"}}, "DNSProviders[0].Config[apiToken]"},
		{"unknown key", DNSProvider{Provider: DNSProviderWebhook, Config: map[string]Secret{"url": "https://hook.example.com", "extra": "x"}}, "DNSProviders[0].Config[extra]"},
		{"key plain for another provider", DNSProvider{Provider: DNSProviderCloudflare, Config: map[string]Secret{"url": "https://hook.example.com"}}, "DNSProviders[0].Config[url]"},
		{"unknown provider", DNSProvider{Provider: "route53", Config: map[string]Secret{"server": "ns1.example.com"}}, "DNSProviders[0].Config[server]"},
		{"placeholder mixed with literal", DNSProvider{Provider: DNSProviderWebhook, Config: map[string]Secret{"token": "Bearer ${ENV:T}"}}, "DNSProviders[0].Config[token]"},
	}
	for _, tc := range cases {
		lits := LiteralSecrets(Config{DNSProviders: []DNSProvider{tc.p}})
		if len(lits) != 1 || lits[0] != tc.want {
			t.Errorf("%s: LiteralSecrets = %v, want [%s]", tc.name, lits, tc.want)
		}
	}
	// Other kinds are walked as before.
	if lits := LiteralSecrets(Config{IdentityProviders: []IdentityProvider{{ObjectMeta: ObjectMeta{Name: "idp"}, OIDC: &OIDCSpec{ClientSecret: "s"}}}}); len(lits) != 1 {
		t.Errorf("identity provider literal client secret: LiteralSecrets = %v, want it reported", lits)
	}
}

// legacyJSON is how a DNSProvider encoded before it had a MarshalJSON.
func legacyJSON(t *testing.T, p DNSProvider) string {
	t.Helper()
	type noMethod DNSProvider
	b, err := json.Marshal(noMethod(p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDNSProviderJSON(t *testing.T) {
	// Only secret keys: byte-identical to the encoding before plain keys.
	onlySecret := []DNSProvider{
		{ObjectMeta: ObjectMeta{Name: "cf", DisplayName: "Cloudflare", Labels: map[string]string{"team": "edge"}, Tags: []string{"prod"}}, Provider: DNSProviderCloudflare, Config: map[string]Secret{"apiToken": "${ENV:CF_TOKEN}"}},
		{ObjectMeta: ObjectMeta{Name: "do"}, Provider: DNSProviderDigitalOcean, Config: map[string]Secret{"apiToken": "literal-token"}},
		{ObjectMeta: ObjectMeta{Name: "empty"}, Provider: DNSProviderHetzner, Config: map[string]Secret{"apiToken": ""}},
		{ObjectMeta: ObjectMeta{Name: "nocfg"}, Provider: DNSProviderDesec},
		{ObjectMeta: ObjectMeta{Name: "emptycfg"}, Provider: DNSProviderDesec, Config: map[string]Secret{}},
		{ObjectMeta: ObjectMeta{Name: "ns", Disabled: true}, Provider: DNSProviderRFC2136, Config: map[string]Secret{"tsigSecret": "literal"}},
	}
	for _, p := range onlySecret {
		got, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if want := legacyJSON(t, p); string(got) != want {
			t.Errorf("%s: JSON = %s, want byte-identical %s", p.Name, got, want)
		}
	}
	if got, _ := json.Marshal(onlySecret[0]); string(got) != `{"name":"cf","displayName":"Cloudflare","labels":{"team":"edge"},"tags":["prod"],"provider":"cloudflare","config":{"apiToken":"${ENV:CF_TOKEN}"}}` {
		t.Errorf("JSON shape = %s", got)
	}
	if got, _ := json.Marshal(onlySecret[1]); !strings.Contains(string(got), `"apiToken":"***"`) {
		t.Errorf("literal apiToken = %s, want ***", got)
	}

	// Plain keys as written, secret keys redacted as before.
	p := DNSProvider{ObjectMeta: ObjectMeta{Name: "hook"}, Provider: DNSProviderWebhook, Config: map[string]Secret{
		"url": "https://hook.example.com/dns?zone=example", "allowInsecureLocal": "${ENV:ALLOW}", "token": "literal-token", "extra": "x",
	}}
	got, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"hook","provider":"webhook","config":{"allowInsecureLocal":"${ENV:ALLOW}","extra":"***","token":"***","url":"https://hook.example.com/dns?zone=example"}}`
	if string(got) != want {
		t.Errorf("JSON = %s\nwant   %s", got, want)
	}
	// Round trip: the plain values come back as written.
	var back DNSProvider
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatal(err)
	}
	if back.Config["url"] != p.Config["url"] || back.Config["allowInsecureLocal"] != p.Config["allowInsecureLocal"] || back.Config["token"] != "***" {
		t.Errorf("round trip = %+v", back.Config)
	}
	for _, lp := range literalProviders() {
		b, _ := json.Marshal(lp)
		var rt DNSProvider
		if err := json.Unmarshal(b, &rt); err != nil {
			t.Fatal(err)
		}
		for k, v := range lp.Config {
			if DNSProviderPlainKey(lp.Provider, k) && rt.Config[k] != v {
				t.Errorf("%s %s: round trip %q, want %q", lp.Name, k, rt.Config[k], v)
			}
		}
	}
	// A pointer and a slice element encode the same way.
	if b, _ := json.Marshal([]*DNSProvider{&p}); !strings.Contains(string(b), `"url":"https://hook.example.com/dns?zone=example"`) {
		t.Errorf("pointer in a slice: %s", b)
	}

	// YAML at rest keeps every value as written, secrets included.
	y, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "token: literal-token") || !strings.Contains(string(y), "url: https://hook.example.com/dns?zone=example") {
		t.Errorf("YAML = %s, want every value as written", y)
	}
}

// TestValidateOutboundBaseURLAllowLocal pins the rule: without the switch
// every non-destination literal is refused; with it, validation skips the
// address checks entirely (a link-local destination is still refused when the
// client dials, see the acme webhook solver test).
func TestValidateOutboundBaseURLAllowLocal(t *testing.T) {
	cases := []struct {
		url  string
		want string // refusal without allowLocal
	}{
		{"http://127.0.0.1:8053", "points at the loopback address 127.0.0.1"},
		{"http://[::1]:8053", "points at the loopback address ::1"},
		{"http://169.254.169.254/", "points at the link-local address 169.254.169.254"},
		{"http://[fe80::1%25eth0]/", "points at the link-local address fe80::1"},
		{"http://0.0.0.0/", "points at 0.0.0.0, which is not a destination"},
		{"http://[ff02::1]/", "points at the link-local address ff02::1"},
		{"http://239.1.2.3/", "points at 239.1.2.3, which is not a destination"},
	}
	for _, tc := range cases {
		if err := ValidateOutboundBaseURL("f", tc.url, false); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s without allowLocal: err = %v, want %q", tc.url, err, tc.want)
		}
		if err := ValidateOutboundBaseURL("f", tc.url, true); err != nil {
			t.Errorf("%s with allowLocal: %v, want accepted by validation", tc.url, err)
		}
	}
	for _, ok := range []string{"https://hook.example.com", "http://192.0.2.10"} {
		for _, allow := range []bool{false, true} {
			if err := ValidateOutboundBaseURL("f", ok, allow); err != nil {
				t.Errorf("%s allowLocal=%v: %v", ok, allow, err)
			}
		}
	}
	if err := ValidateOutboundBaseURL("f", "ftp://hook.example.com", true); err == nil {
		t.Error("ftp URL with allowLocal: accepted, want the scheme check to still apply")
	}
}

// TestDNSProviderLiteralLocalURL: with literals possible, the write-time URL
// checks run on a literal url / baseURL and a literal allowInsecureLocal.
func TestDNSProviderLiteralLocalURL(t *testing.T) {
	for _, tc := range []struct {
		provider, key string
		secrets       map[string]Secret
	}{
		{DNSProviderWebhook, "url", map[string]Secret{"token": "${ENV:T}"}},
		{DNSProviderACMEDNS, "baseURL", map[string]Secret{"username": "${ENV:U}", "password": "${ENV:P}", "subdomain": "s"}},
	} {
		build := func(url, flag string) DNSProvider {
			cfg := map[string]Secret{tc.key: Secret(url)}
			for k, v := range tc.secrets {
				cfg[k] = v
			}
			if flag != "" {
				cfg["allowInsecureLocal"] = Secret(flag)
			}
			return DNSProvider{ObjectMeta: ObjectMeta{Name: "dns"}, Provider: tc.provider, Config: cfg}
		}
		if err := build("http://127.0.0.1:9000/dns", "true").Validate(); err != nil {
			t.Errorf("%s: loopback with allowInsecureLocal true: %v", tc.provider, err)
		}
		if err := build("http://127.0.0.1:9000/dns", "").Validate(); err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Errorf("%s: loopback without the opt-in: err = %v, want refused", tc.provider, err)
		}
		// The switch skips the address checks at validation time; the
		// metadata address is refused when the client dials instead.
		if err := build("http://169.254.169.254/", "true").Validate(); err != nil {
			t.Errorf("%s: link-local with the opt-in: %v, want accepted by validation", tc.provider, err)
		}
		if err := build("http://169.254.169.254/", "").Validate(); err == nil || !strings.Contains(err.Error(), "link-local") {
			t.Errorf("%s: link-local without the opt-in: err = %v, want refused", tc.provider, err)
		}
	}
}

// TestValidateResolvedOutboundURL: the run-time variant applies the same
// rules with texts that never contain the value; the write-time texts are
// unchanged.
func TestValidateResolvedOutboundURL(t *testing.T) {
	cases := []struct {
		url, resolved, written string
	}{
		{"htps://hookusr:pa55@hook.example.com/dns?token=S3CRET",
			"f: is not an http or https URL (the value is not shown, it may hold a credential)",
			`f: must be an http or https URL, got "htps://hookusr:pa55@hook.example.com/dns?token=S3CRET"`},
		{"http://hookusr:pa55@127.0.0.1:9000/dns?token=S3CRET",
			`f: points at a loopback address; set config.allowInsecureLocal: "true" if the service really runs beside gpm`,
			`f: points at the loopback address 127.0.0.1; set config.allowInsecureLocal: "true" if the service really runs beside gpm`},
		{"http://hookusr:pa55@169.254.169.254/dns?token=S3CRET",
			"f: points at a link-local address, which is the cloud metadata range and never a DNS provider",
			"f: points at the link-local address 169.254.169.254, which is the cloud metadata range and never a DNS provider"},
		{"http://hookusr:pa55@0.0.0.0/dns?token=S3CRET",
			"f: points at an unspecified or multicast address, which is not a destination gpm can send credentials to",
			"f: points at 0.0.0.0, which is not a destination gpm can send credentials to"},
	}
	for _, tc := range cases {
		if err := ValidateResolvedOutboundURL("f", tc.url, false); err == nil || err.Error() != tc.resolved {
			t.Errorf("resolved %s: err = %v, want %q", tc.url, err, tc.resolved)
		}
		if err := ValidateOutboundBaseURL("f", tc.url, false); err == nil || err.Error() != tc.written {
			t.Errorf("written %s: err = %v, want %q", tc.url, err, tc.written)
		}
		if err := ValidateResolvedOutboundURL("f", tc.url, true); (err == nil) != strings.HasPrefix(tc.url, "http://") {
			t.Errorf("resolved %s with allowLocal: err = %v", tc.url, err)
		}
	}
	if err := ValidateResolvedOutboundURL("f", "https://hook.example.com/dns?token=x", false); err != nil {
		t.Errorf("public URL: %v", err)
	}
}

// TestWebhookURLUserInfoRefused: user info in a webhook url is never sent
// (the solver sets its own Authorization header), so it is refused at write
// time with a text that does not echo the URL. A placeholder is checked when
// the solver is built.
func TestWebhookURLUserInfoRefused(t *testing.T) {
	p := DNSProvider{ObjectMeta: ObjectMeta{Name: "hook"}, Provider: DNSProviderWebhook, Config: map[string]Secret{
		"url": "https://hookusr:pa55@hook.example.com/dns", "token": "${ENV:T}",
	}}
	want := `dns provider "hook": config.url: must not contain user info (user:password@); it is never sent, put the credential in config.token`
	if err := p.Validate(); err == nil || err.Error() != want {
		t.Errorf("Validate = %v, want %q", err, want)
	}
	p.Config["url"] = "https://hookusr@hook.example.com/dns"
	if err := p.Validate(); err == nil || err.Error() != want {
		t.Errorf("user name only: Validate = %v, want %q", err, want)
	}
	p.Config["url"] = "${ENV:HOOK_URL}"
	if err := p.Validate(); err != nil {
		t.Errorf("placeholder url: %v", err)
	}
	// acme-dns is unchanged.
	a := DNSProvider{ObjectMeta: ObjectMeta{Name: "a"}, Provider: DNSProviderACMEDNS, Config: map[string]Secret{
		"baseURL": "https://hookusr:pa55@acme-dns.example.com", "username": "${ENV:U}", "password": "${ENV:P}", "subdomain": "s",
	}}
	if err := a.Validate(); err != nil {
		t.Errorf("acme-dns with user info: %v, want unchanged (accepted)", err)
	}
}
