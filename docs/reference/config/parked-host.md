# ParkedHost

Returns a fixed status for claimed domains: reserve a name without serving
anything. Useful to absorb unmatched vhosts and stop default-host leakage.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="parked-host-domains"></span>  `domains` | []string | yes | Hostnames this host claims. Exclusive across enabled hosts, like every other kind. |
| <span id="parked-host-status-code"></span>  `statusCode` | int | no | Status returned for every request. Unset (or `0`) sends `404`. |
| <span id="parked-host-tls"></span>  `tls` | TLSSettings | no | Same shape as [ProxyHost `tls`](proxy-host.md#proxy-host-tls) - `certificateRef`, `forceSSL`, `minTLSVersion`, `hsts.*` and `clientAuth.*` all apply. Note `certificateRef` is an intent record here too: the served certificate is selected by SNI. |

A parked host's response renders the [settings-level error page](settings/error-pages.md)
for `statusCode`, when one is configured, falling back to the plain-text body
otherwise; it has no `errorPages` field of its own to override with.

---
