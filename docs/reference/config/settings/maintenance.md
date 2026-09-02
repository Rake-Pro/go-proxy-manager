# Settings: maintenance mode

Take one proxy host, or the whole edge, out of service without deleting it or
touching DNS.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-maintenance"></span> `maintenance` | MaintenanceSettings | Fleet-wide downtime switch and the `Retry-After` every maintenance response carries (below). Zero value is off: each proxy host is governed by its own `maintenance` flag alone. |

Takes a proxy host (or the whole edge) out of service for a downtime window
without deleting it, disabling it, or touching DNS. While a host is in
maintenance gpm answers every request to it **itself** - HTTP 503 with a
`Retry-After` header - and never dials the upstream. The host keeps its domains,
its certificate and its DNS records, so flipping the switch back restores
service with no other change.

There are two switches:

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-proxy-host-maintenance"></span> `proxyHost.maintenance` | bool | Per host. Operator-owned state: Ingress discovery carries it forward across a reconcile instead of deriving it. |
| <span id="settings-maintenance-enabled"></span> `settings.maintenance.enabled` | bool | Fleet-wide. Puts **every** proxy host into maintenance whatever its own flag says - the global switch wins over a per-host `false`. Turning it off returns each host to its own flag. |
| <span id="settings-maintenance-retry-after-seconds"></span> `settings.maintenance.retryAfterSeconds` | int | `Retry-After` on every maintenance response, per-host ones included. `0` (default) selects 300s. Range 0-86400. |

```yaml
# settings.yaml - the whole edge goes down
maintenance:
  enabled: true
  retryAfterSeconds: 900
```

```yaml
# config/proxy-hosts/grafana.yaml - just this one host
maintenance: true
```

Both switches take effect **without a restart**. A settings write applies on the
very next request (the switch is read live); a per-host write applies on the
reload that every config write already triggers.

**Status and the page served.** 503 is the correct code for a deliberate,
temporary outage: it is what `Retry-After` is defined against, and it is the one
5xx a search engine treats as "come back later" rather than as a broken or
removed site. The body is the [error page](error-pages.md)
configured for `503` - the host's own `errorPages` override first, then the
settings-level pages, the same resolution every other gpm-generated error uses,
so a custom maintenance page needs no second mechanism. With none configured,
gpm's built-in maintenance page is served, negotiated on `Accept`: JSON to a
client asking for `application/json`, HTML to a browser, plain text to anything
else (including `*/*`). **Every** maintenance response carries a `Content-Type`
and a body - a bodyless error response from this proxy has broken real API
clients.

**What is not affected.** Redirect, parked and stream hosts proxy nothing to
take out of service and keep serving. ACME HTTP-01 challenges are answered
before host routing, so certificates still renew during a window. A host that
requires mTLS still refuses an uncertified request with a 421 rather than
disclosing a maintenance window to it. There is no scheduling: a window opens
and closes when an operator flips the switch.
