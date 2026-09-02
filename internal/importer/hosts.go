package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Rake-Pro/go-proxy-manager/internal/model"
)

// resolveCertRef maps an NPM certificate_id to an imported cert object Name.
// It returns "" (dropping the reference) and warns when the cert was not
// imported or its PEM files were missing, so the produced config stays valid.
func (s *importState) resolveCertRef(certID int64, hostLabel string) string {
	if certID == 0 {
		return ""
	}
	name, mapped := s.certNames[certID]
	if !mapped {
		s.warn(hostLabel, "certificate_id",
			fmt.Sprintf("referenced certificate #%d was not imported; TLS certificate reference dropped", certID))
		return ""
	}
	if !s.certOK[certID] {
		s.warn(hostLabel, "certificate_id",
			fmt.Sprintf("referenced certificate #%d had missing PEM files; TLS certificate reference dropped", certID))
		return ""
	}
	return name
}

func (s *importState) tlsFor(r map[string]any, certID int64, hostLabel string) model.TLSSettings {
	return model.TLSSettings{
		CertificateRef: s.resolveCertRef(certID, hostLabel),
		ForceSSL:       asBool(r["ssl_forced"]),
		//lint:ignore SA1019 compat write of deprecated TLSSettings.HTTP2 during NPM import
		HTTP2: asBool(r["http2_support"]),
		HSTS: model.HSTS{
			Enabled:           asBool(r["hsts_enabled"]),
			IncludeSubdomains: asBool(r["hsts_subdomains"]),
		},
	}
}

// npmLocation is one entry of proxy_host.locations JSON.
type npmLocation struct {
	Path           string `json:"path"`
	ForwardScheme  string `json:"forward_scheme"`
	ForwardHost    string `json:"forward_host"`
	ForwardPort    int    `json:"forward_port"`
	AdvancedConfig string `json:"advanced_config"`
}

func (s *importState) importProxyHosts() error {
	want := []string{
		"id", "domain_names", "forward_scheme", "forward_host", "forward_port",
		"access_list_id", "certificate_id", "ssl_forced", "http2_support",
		"hsts_enabled", "hsts_subdomains", "allow_websocket_upgrade", "enabled",
		"locations", "advanced_config", "block_exploits", "caching_enabled", "is_deleted",
	}
	cols, _, ok, err := s.selectAvailable("proxy_host", want)
	if err != nil {
		return err
	}
	if !ok {
		s.warn("proxy_host", "table", "proxy_host table not present; no proxy hosts imported")
		return nil
	}
	rows, err := s.queryRows("proxy_host", cols)
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		domains, dok := parseDomains(asString(r["domain_names"]))
		label := fmt.Sprintf("proxy_host #%d (%s)", id, domainLabel(domains))
		if !dok {
			s.warn(label, "domain_names", "could not parse domain_names; proxy host skipped")
			continue
		}
		host := strings.TrimSpace(asString(r["forward_host"]))
		if host == "" {
			s.warn(label, "forward_host", "missing forward_host; proxy host skipped")
			continue
		}

		name := s.uniqueName("ProxyHost", domains[0], "proxyhost", id)

		ph := model.ProxyHost{
			ObjectMeta: model.ObjectMeta{
				Name:        name,
				DisplayName: domains[0],
				Disabled:    asInt(r["enabled"]) == 0,
			},
			Domains: domains,
			Upstream: model.Upstream{
				Scheme: schemeOrDefault(asString(r["forward_scheme"])),
				Host:   host,
				Port:   int(asInt(r["forward_port"])),
			},
			//lint:ignore SA1019 compat write of deprecated ProxyHost.WebsocketsUpgrade during NPM import
			WebsocketsUpgrade: asBool(r["allow_websocket_upgrade"]),
			TLS:               s.tlsFor(r, asInt(r["certificate_id"]), label),
		}

		if alID := asInt(r["access_list_id"]); alID != 0 {
			if alName, ok := s.alNames[alID]; ok {
				ph.AccessLists = []string{alName}
			} else {
				// Fail closed: the host was gated before, so do NOT import it wide
				// open. Synthesize a deny-all access list and attach it so the host
				// returns 403 until an operator reassigns a valid access list.
				lockName := s.uniqueName("AccessList", name+"-import-locked", "accesslist", id)
				lock := model.AccessList{
					ObjectMeta:    model.ObjectMeta{Name: lockName, DisplayName: name + " import lock"},
					DefaultAction: model.ActionDeny,
					Rules:         []model.IPRule{{Action: model.ActionDeny, CIDR: "0.0.0.0/0"}},
				}
				if s.add(label, "access_list_id", lock) {
					ph.AccessLists = []string{lockName}
				}
				s.warn(label, "access_list_id",
					fmt.Sprintf("referenced access_list #%d was not imported; host LOCKED with a deny-all access list (returns 403) until an operator reassigns a valid access list", alID))
			}
		}

		ph.Locations = s.mapLocations(r, label)

		s.lossyHostWarnings(r, label)

		s.add(label, "", ph)
	}
	return nil
}

func (s *importState) mapLocations(r map[string]any, label string) []model.Location {
	raw := strings.TrimSpace(asString(r["locations"]))
	if raw == "" || raw == "null" || raw == "[]" {
		return nil
	}
	var locs []npmLocation
	if err := json.Unmarshal([]byte(raw), &locs); err != nil {
		s.warn(label, "locations", fmt.Sprintf("could not parse locations JSON: %v; locations skipped", err))
		return nil
	}
	var out []model.Location
	for i, l := range locs {
		if strings.TrimSpace(l.Path) == "" {
			s.warn(label, "locations", fmt.Sprintf("location #%d has empty path; skipped", i))
			continue
		}
		loc := model.Location{Path: l.Path}
		if l.ForwardHost != "" {
			loc.Upstream = &model.Upstream{
				Scheme: schemeOrDefault(l.ForwardScheme),
				Host:   l.ForwardHost,
				Port:   l.ForwardPort,
			}
		}
		if strings.TrimSpace(l.AdvancedConfig) != "" {
			s.warn(label, "locations.advanced_config",
				"raw nginx advanced config not imported (go-proxy-manager uses typed middleware, not raw nginx); review and re-create as middleware/headers")
		}
		out = append(out, loc)
	}
	return out
}

// lossyHostWarnings emits warnings for proxy-host fields with no typed equivalent.
func (s *importState) lossyHostWarnings(r map[string]any, label string) {
	if strings.TrimSpace(asString(r["advanced_config"])) != "" {
		s.warn(label, "advanced_config",
			"raw nginx advanced config not imported (go-proxy-manager uses typed middleware, not raw nginx); review and re-create as middleware/headers")
	}
	if asBool(r["block_exploits"]) {
		s.warn(label, "block_exploits", "block_exploits has no direct equivalent yet")
	}
	if asBool(r["caching_enabled"]) {
		s.warn(label, "caching_enabled", "caching not supported")
	}
}

func (s *importState) importRedirectionHosts() error {
	want := []string{
		"id", "domain_names", "forward_http_code", "forward_scheme", "forward_domain_name",
		"preserve_path", "certificate_id", "ssl_forced", "hsts_enabled", "hsts_subdomains",
		"enabled", "advanced_config", "is_deleted",
	}
	cols, _, ok, err := s.selectAvailable("redirection_host", want)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := s.queryRows("redirection_host", cols)
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		domains, dok := parseDomains(asString(r["domain_names"]))
		label := fmt.Sprintf("redirection_host #%d (%s)", id, domainLabel(domains))
		if !dok {
			s.warn(label, "domain_names", "could not parse domain_names; redirect host skipped")
			continue
		}
		target := strings.TrimSpace(asString(r["forward_domain_name"]))
		if target == "" {
			s.warn(label, "forward_domain_name", "missing forward_domain_name; redirect host skipped")
			continue
		}

		name := s.uniqueName("RedirectHost", domains[0], "redirecthost", id)

		scheme := strings.ToLower(strings.TrimSpace(asString(r["forward_scheme"])))
		if scheme == "auto" {
			scheme = ""
		}

		rh := model.RedirectHost{
			ObjectMeta: model.ObjectMeta{
				Name:        name,
				DisplayName: domains[0],
				Disabled:    asInt(r["enabled"]) == 0,
			},
			Domains:      domains,
			TargetScheme: scheme,
			TargetDomain: target,
			StatusCode:   int(asInt(r["forward_http_code"])),
			PreservePath: asBool(r["preserve_path"]),
			TLS:          s.tlsFor(r, asInt(r["certificate_id"]), label),
		}

		if strings.TrimSpace(asString(r["advanced_config"])) != "" {
			s.warn(label, "advanced_config",
				"raw nginx advanced config not imported (go-proxy-manager uses typed middleware, not raw nginx); review and re-create as middleware/headers")
		}

		s.add(label, "", rh)
	}
	return nil
}

// importParkedHosts reads NPM's `dead_host` table - NPM's own name for a domain
// that answers 404 and nothing else - and emits gpm ParkedHost objects. The
// table name stays NPM's because that is the schema on disk; only the gpm-side
// vocabulary is ours.
func (s *importState) importParkedHosts() error {
	want := []string{
		"id", "domain_names", "certificate_id", "ssl_forced",
		"hsts_enabled", "hsts_subdomains", "enabled", "advanced_config", "is_deleted",
	}
	cols, _, ok, err := s.selectAvailable("dead_host", want)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := s.queryRows("dead_host", cols)
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		domains, dok := parseDomains(asString(r["domain_names"]))
		label := fmt.Sprintf("dead_host #%d (%s)", id, domainLabel(domains))
		if !dok {
			s.warn(label, "domain_names", "could not parse domain_names; parked host skipped")
			continue
		}

		name := s.uniqueName("ParkedHost", domains[0], "parkedhost", id)

		ph := model.ParkedHost{
			ObjectMeta: model.ObjectMeta{
				Name:        name,
				DisplayName: domains[0],
				Disabled:    asInt(r["enabled"]) == 0,
			},
			Domains: domains,
			TLS:     s.tlsFor(r, asInt(r["certificate_id"]), label),
		}

		if strings.TrimSpace(asString(r["advanced_config"])) != "" {
			s.warn(label, "advanced_config",
				"raw nginx advanced config not imported (go-proxy-manager uses typed middleware, not raw nginx); review and re-create as middleware/headers")
		}

		s.add(label, "", ph)
	}
	return nil
}

func (s *importState) importStreams() error {
	want := []string{
		"id", "incoming_port", "forwarding_host", "forwarding_port",
		"tcp_forwarding", "udp_forwarding", "enabled", "is_deleted",
	}
	cols, _, ok, err := s.selectAvailable("stream", want)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := s.queryRows("stream", cols)
	if err != nil {
		return err
	}

	for _, r := range rows {
		id := asInt(r["id"])
		host := strings.TrimSpace(asString(r["forwarding_host"]))
		label := fmt.Sprintf("stream #%d (:%d)", id, asInt(r["incoming_port"]))
		if host == "" {
			s.warn(label, "forwarding_host", "missing forwarding_host; stream skipped")
			continue
		}

		tcp := asBool(r["tcp_forwarding"])
		udp := asBool(r["udp_forwarding"])
		var proto string
		switch {
		case tcp && udp:
			proto = "both"
		case udp:
			proto = "udp"
		case tcp:
			proto = "tcp"
		default:
			proto = "tcp"
			s.warn(label, "protocol", "neither tcp nor udp forwarding set; defaulted to tcp")
		}

		name := s.uniqueName("StreamHost", fmt.Sprintf("stream-%d", asInt(r["incoming_port"])), "streamhost", id)

		sh := model.StreamHost{
			ObjectMeta: model.ObjectMeta{
				Name:     name,
				Disabled: asInt(r["enabled"]) == 0,
			},
			ListenPort: int(asInt(r["incoming_port"])),
			Protocol:   proto,
			Target: model.StreamTarget{
				Host: host,
				Port: int(asInt(r["forwarding_port"])),
			},
		}

		s.add(label, "", sh)
	}
	return nil
}

// schemeOrDefault normalizes a forward scheme to http/https, defaulting to http.
func schemeOrDefault(sc string) string {
	sc = strings.ToLower(strings.TrimSpace(sc))
	if sc == "https" {
		return "https"
	}
	return "http"
}

func domainLabel(domains []string) string {
	if len(domains) == 0 {
		return "no-domain"
	}
	return domains[0]
}
