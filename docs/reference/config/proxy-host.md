# ProxyHost

Terminates TLS for one or more domains and reverse-proxies to an upstream.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="proxy-host-domains"></span>  `domains` | []string | yes | One or more hostnames served by this host. |
| <span id="proxy-host-upstream"></span>  `upstream` | Upstream | one of | Default backend (single). Mutually exclusive with `upstreamGroupRef`. |
| <span id="proxy-host-upstream-group-ref"></span>  `upstreamGroupRef` | string | one of | Name of an [UpstreamGroup](upstream-group.md) for health-checked failover across several backends. Mutually exclusive with `upstream`. |
| <span id="proxy-host-robots-no-index"></span>  `robotsNoIndex` | bool | no | Emit `X-Robots-Tag: noindex, nofollow` (HTTP and HTTPS) to discourage search-engine indexing. A headers middleware that sets `X-Robots-Tag` explicitly still wins. |
| <span id="proxy-host-timeouts"></span>  `timeouts` | HostTimeouts | no | Per-host upstream timeout overrides (below). |
| <span id="proxy-host-tls"></span>  `tls` | TLSSettings | no | Certificate + TLS behaviour. |
| <span id="proxy-host-dns"></span>  `dns` | DNSSyncPolicy | no | Opt this host's domains into DNS record management (below). |
| <span id="proxy-host-middlewares"></span>  `middlewares` | []string | no | Host-wide middleware names, applied top-down. |
| <span id="proxy-host-access-lists"></span>  `accessLists` | []string | no | Host-wide access-list names. |
| <span id="proxy-host-auth"></span>  `auth` | AuthMiddleware | no | Gate this host through an identity provider **without a Middleware object**. Same shape and same gate as a `type: auth` middleware. See [Inline auth and rate limit](#proxy-host-inline-auth-and-rate-limit). |
| <span id="proxy-host-rate-limit"></span>  `rateLimit` | RateLimitMiddleware | no | Throttle this host **without a Middleware object**. Same shape and same limiter as a `type: rate-limit` middleware. See [Inline auth and rate limit](#proxy-host-inline-auth-and-rate-limit). |
| <span id="proxy-host-locations"></span>  `locations` | []Location | no | Path-scoped overrides (below). |
| <span id="proxy-host-maintenance"></span>  `maintenance` | bool | no | Take this host out of service without removing it: gpm answers every request with a 503 maintenance page and never dials the upstream. The host keeps its domains, certificate and DNS records. Operator-owned: Ingress discovery preserves it. See [Maintenance mode](settings/maintenance.md). |
| <span id="proxy-host-compression"></span>  `compression` | Compression | no | Gzip response compression (below). Zero value (`enabled: false`) is today's behaviour: no compression. |
| <span id="proxy-host-error-pages"></span>  `errorPages` | ErrorPagesConfig | no | Overrides [`settings.errorPages`](settings/error-pages.md) for this host's own gpm-generated errors. Unset uses the settings-level pages, if any. Same shape and same validation as the settings block: `dir`, `inline`, `interceptUpstream`. |
| <span id="proxy-host-error-pages-dir"></span>  `errorPages.dir` | string | no | Directory of `<status>.html` templates plus an optional `default.html`, relative to the cert store. Same rules as [`settings.errorPages.dir`](settings/error-pages.md#settings-error-pages-dir). |
| <span id="proxy-host-error-pages-inline"></span>  `errorPages.inline` | map[string]string | no | Status code (or `"default"`) mapped to `html/template` source, for a page kept in config rather than a mounted directory. |
| <span id="proxy-host-error-pages-intercept-upstream"></span>  `errorPages.interceptUpstream` | []int | no | Status codes for which this host also replaces the **upstream's own** error body. |
| <span id="proxy-host-security-headers"></span>  `securityHeaders` | map[string]string \| map[string]{value,scope} | no | Merges over [`settings.securityHeaders`](settings/security-headers.md) per key for this host (replacing the settings value **and its scope** for a header it names). Unset uses the settings-level default unchanged. |
| <span id="proxy-host-strip-response-headers"></span>  `stripResponseHeaders` | []string | no | Response headers removed from what this host's upstream sends, **in addition to** [`settings.stripResponseHeaders`](settings/security-headers.md#strip-response-headers-section) (union, not replacement: a host cannot opt out of a fleet-level strip). |
| <span id="proxy-host-trusted-proxies"></span>  `trustedProxies` | []string (nullable) | no | The L7 proxies whose `X-Forwarded-For` is believed when deriving this host's client IP. **Three states:** omitted or `null` inherits [`settings.trustedProxies`](settings/trusted-proxies.md); `[]` trusts nobody for this host whatever the fleet default is; a non-empty list **replaces** the fleet list (never extends it). See [Per-host override](settings/trusted-proxies.md#per-host-override-absent-is-not-the-same-as-empty) and [Client IP and the three trust tiers](../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers). |

**Upstream**: a single backend target. Used by `proxyHost.upstream`,
`location.upstream` and each member of an [UpstreamGroup](upstream-group.md).

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="proxy-host-upstream-scheme"></span>  `scheme` | string | none | yes | `http` or `https`. |
| <span id="proxy-host-upstream-host"></span>  `host` | string | none | yes | Backend hostname or IP. |
| <span id="proxy-host-upstream-port"></span>  `port` | int | none | yes | 1-65535. |
| <span id="proxy-host-upstream-path"></span>  `path` | string | `""` | no | Base path prefixed to every forwarded request: `/api` turns a client `/v1/x` into `/api/v1/x`. Must be absolute, must not contain `.`/`..` segments, `\`, `;`, a query string or a fragment. Applied **last**, after any location prefix strip and any rewrite middleware. |
| <span id="proxy-host-upstream-host-header"></span>  `hostHeader` | string | `""` | no | Host header sent upstream. `""` keeps the client's Host (today's behaviour); `upstream` sends the upstream's own `host:port`; anything else is sent literally and must be a hostname, optionally `host:port`. |

```yaml
upstream:
  scheme: http
  host: 10.0.0.40
  port: 8080
  path: /api              # backend serves the app under /api
  hostHeader: upstream    # backend keys its vhost off its own address
```

On an [UpstreamGroup](upstream-group.md) member, `path` and
`hostHeader` are **per member**, applied by the failover transport on the attempt
that actually reaches that member. `healthCheck.path` is used verbatim and is
**not** prefixed with `path`, so write the full probe path there.

**TLSSettings**: `certificateRef` (a Certificate name; see [Which certificate a
host serves](certificate.md#which-certificate-a-host-serves); it does **not** select the
certificate for an L7 host), `forceSSL` (redirect HTTP->HTTPS), `hsts` (`enabled`,
`maxAge` in seconds, default one year when unset, `includeSubdomains`, `preload`),
`minTLSVersion` (`"1.2"` default | `"1.3"`), `clientAuth` (mTLS, below).
When `hsts.enabled` is set, the data plane emits `Strict-Transport-Security` on
HTTPS responses for the host (never over plain HTTP).

**ClientAuth** (`tls.clientAuth`) opts the host into mTLS: client certificates
are verified at the TLS handshake against a [ClientCA](client-ca.md).
It is editable from the host editor's TLS section ("Client certificates (mTLS)"):
a toggle, a Client CA picker and the mode. Turning it **on** is greyed out with
the reason until its preconditions hold (`forceSSL` on, and at least one enabled
ClientCA to point at), while turning it **off** is always possible, so a host
whose stored combination is already invalid is never trapped. Turning `forceSSL`
off under a live mTLS host is refused rather than silently dropping the
certificate requirement. A `caRef` the CA list does not contain is shown flagged
and kept as-is on save rather than the picker quietly retargeting the trust
anchor, and if the CA list cannot be loaded at all the page says so and leaves
`caRef`/`mode` untouched instead of guessing.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="proxy-host-tls-client-auth-ca-ref"></span>  `caRef` | string | yes | ClientCA name. Must exist and be enabled; the host also requires `forceSSL: true`. |
| <span id="proxy-host-tls-client-auth-mode"></span>  `mode` | string | no | `require` (default) rejects the handshake without a valid certificate; `optional` verifies a presented certificate but lets certless requests through (mTLS as a fallback beside SSO). |
| <span id="proxy-host-tls-client-auth-identity-headers"></span>  `identityHeaders` | ClientCertHeaders | no | Forward the verified certificate's identity upstream. Unset = forward nothing. |

**ClientCertHeaders** (`tls.clientAuth.identityHeaders`):

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| <span id="proxy-host-tls-client-auth-identity-headers-subject-header"></span>  `subjectHeader` | string | `X-Client-Cert-Subject` | Header carrying the certificate subject in RFC 2253 form (`CN=ops,O=Corp`). Must be a valid header name and may not be an `X-Forwarded-*` header. |
| <span id="proxy-host-tls-client-auth-identity-headers-san"></span>  `san` | bool | false | Send `X-Client-Cert-SAN`: the subject alternative names (DNS, email, IP, URI), comma-separated. |
| <span id="proxy-host-tls-client-auth-identity-headers-serial"></span>  `serial` | bool | false | Send `X-Client-Cert-Serial`: the serial number in lower-case hex. |
| <span id="proxy-host-tls-client-auth-identity-headers-fingerprint"></span>  `fingerprint` | bool | false | Send `X-Client-Cert-Fingerprint`: the SHA-256 digest of the DER certificate, lower-case hex. |

These headers ride the existing identity-trust model: all four default names are
in the **baseline identity denylist**, so they are stripped from every request
whose peer is not a proxy the host trusts (whether or not the host enables
passthrough), and a custom `subjectHeader` is added to that host's own strip set.
gpm sets them *after* the strip, only from a certificate the handshake actually
**verified**; in `optional` mode a certless request reaches the upstream with no
identity headers at all.

`minTLSVersion` is a **per-host** floor selected by SNI at handshake time. The
edge already negotiates TLS 1.2 *or* 1.3 per client (1.2 is the default floor);
set `"1.3"` only on hosts where every client supports it (drops 1.2: old smart
TVs / embedded clients / legacy scripts may then fail to connect). Leave it unset
for public hosts to keep the widest client compatibility.

| Fleet default | Per-host override |
|---|---|
| [`settings.tls.minVersion`](settings/tls.md#settings-tls-min-version) sets the floor for every host that leaves `minTLSVersion` unset, plus stream hosts and an unknown SNI. | `minTLSVersion` here wins in **either** direction: `"1.3"` under the default fleet floor, and `"1.2"` under a `"1.3"` fleet floor. |

**DNSSyncPolicy** (`dns`): `lanDirect` publishes each of the host's domains as a
local CNAME on the LAN resolver (Pi-hole), so internal clients reach the edge
directly instead of hairpinning through the WAN address; `publicCname` publishes
them in the authoritative public zone (Cloudflare). Both default false: nothing
is published unless asked for, and an opted-out host omits the `dns` key from its
API responses entirely rather than returning an empty object. The backends
themselves are configured once, in
[`settings.dnsSync`](settings/dns-sync.md); a policy flag with its
backend disabled publishes nothing, and the UI says so inline while leaving the
toggle usable: setting the flag before the backend exists is legitimate staging
(the host is the declaration; the syncer publishes once it is wired), so it is
not refused.

**HostTimeouts** (`timeouts`): `connectSeconds` caps establishing the TCP/TLS
connection to the upstream; `readSeconds` caps time awaiting the upstream's
response headers (time-to-first-byte). Both are whole seconds (0-3600); `0`/unset
means no override. A host with any override uses its **own** cloned transport
(its own connection pool), so a custom timeout never affects another host's
keep-alive reuse; hosts without an override share the default pooled transport.
`readSeconds` bounds only time-to-first-byte, so it does not cut off a slow
streaming / SSE / websocket body once headers have arrived.

**Location** (`locations[]`) is a path-scoped override. Matching is
longest-prefix; the request path is forwarded unchanged unless `stripPrefix` is
set. Everything a location adds is **appended to** the host-wide chain rather
than replacing it, so a location is always at least as restrictive as its host.

| Field | Type | Default | Required | Description |
|---|---|---|---|---|
| <span id="proxy-host-locations-path"></span>  `path` | string | none | yes | Path prefix this override applies to. |
| <span id="proxy-host-locations-upstream"></span>  `upstream` | Upstream | host's backend | no | Single backend for this path. Mutually exclusive with `upstreamGroupRef`; with neither, the host's backend (including an upstream group) is inherited. |
| <span id="proxy-host-locations-upstream-group-ref"></span>  `upstreamGroupRef` | string | host's backend | no | UpstreamGroup name for this path. Mutually exclusive with `upstream`. |
| <span id="proxy-host-locations-strip-prefix"></span>  `stripPrefix` | bool | false | no | Remove this location's matched prefix before forwarding: `/app/foo` reaches the backend as `/foo`, and `/app` (or `/app/`) as `/`. A root location (`path: /`) strips nothing. |
| <span id="proxy-host-locations-middlewares"></span>  `middlewares` | []string | none | no | Middleware names appended to the host's. |
| <span id="proxy-host-locations-access-lists"></span>  `accessLists` | []string | none | no | Access-list names appended to the host's. |
| <span id="proxy-host-locations-auth"></span>  `auth` | AuthMiddleware | none | no | Inline auth gate for this path, stacked on top of the host's (below). |
| <span id="proxy-host-locations-rate-limit"></span>  `rateLimit` | RateLimitMiddleware | none | no | Inline rate limit for this path, stacked on top of the host's (below). |

`stripPrefix` is applied **inside** the whole security chain: rate limiting, the
access list, the bouncer, auth and guards all still evaluate the original client
path (the path the location itself matched on), and only the request that reaches
the backend is shortened. The query string is forwarded untouched.

```yaml
# grafana.example.com/metrics/... served by a Grafana that expects to be at /
name: tools
domains: [tools.example.com]
upstream: { scheme: http, host: 10.0.0.40, port: 80 }
locations:
  - path: /metrics
    stripPrefix: true
    upstream: { scheme: http, host: 10.0.0.41, port: 3000 }
```

**Compression** (`compression`) gzip-compresses eligible response bodies from
this host's upstream, using only the standard library's `compress/gzip` (via a
pooled `sync.Pool` of writers).

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| <span id="proxy-host-compression-enabled"></span>  `enabled` | bool | false | Off means byte-for-byte today's behaviour: no compression. |
| <span id="proxy-host-compression-min-bytes"></span>  `minBytes` | int | 1024 | Smallest response body gzip bothers with. The body is buffered up to this size before the compress/pass-through decision is made, so a response that never reaches it is sent uncompressed. |
| <span id="proxy-host-compression-types"></span>  `types` | []string | text/html, text/plain, text/css, text/csv, application/json, application/javascript, text/javascript, application/xml, text/xml, image/svg+xml | Response `Content-Type`s (media type only; `charset` etc. ignored) eligible for compression. |

Compression honours the client's `Accept-Encoding` and is skipped outright when:
the request is `HEAD`; the client sent no `Accept-Encoding: gzip`; the upstream
already set `Content-Encoding` (never double-encoded); the response
`Content-Type` doesn't match `types`; the body stays under `minBytes`; the
status is `204`, `304`, or `101` (protocol switch); or the response is
`text/event-stream` or otherwise starts **streaming** (the handler flushes
before the compress decision is made, this is also what keeps a WebSocket
upgrade, which is hijacked rather than written through, untouched). A
compressed response gets `Content-Encoding: gzip`, `Vary: Accept-Encoding`, and
has `Content-Length` stripped (the compressed size isn't known up front).
**BREACH**: compressing a response whose size depends on attacker-controlled
input reflected alongside a secret (e.g. a CSRF token) can leak that secret
through the compressed size; that trade-off is why compression is opt-in per
host rather than default-on; hosts serving that shape of response should leave
it off.

```yaml
name: app
domains: [app.example.com]
upstream: {scheme: http, host: backend, port: 8080}
tls: {certificateRef: wildcard, forceSSL: true}
dns: {lanDirect: true, publicCname: true}
middlewares: [require-sso]
compression: {enabled: true}
locations:
  - path: /metrics
    accessLists: [internal-only]      # /metrics also requires the internal CIDR
```

<span id="proxy-host-inline-auth-and-rate-limit"></span>

## Inline auth and rate limit (`auth` / `rateLimit`)

A proxy host or location may carry an `auth` or `rateLimit` block **directly**,
with no `Middleware` object and no reference to attach. The block has the same
shape, the same validation rules and compiles to the same data-plane handler as
the middleware of that kind, so behaviour, metrics and error pages are identical.

- **Direct is for a handful of hosts.** Gating one host by SSO is one block on
  that host, not three objects across three pages.
- **Middleware objects remain the reuse path.** One gate shared by a fleet is
  still one `Middleware` every host references.
- **Both may be set.** There is no mutual exclusivity: a host can run an inline
  block and reference a middleware of the same kind, and every gate must pass.
- **Inline runs first.** An inline block sits at the same chain position as its
  middleware kind, just outside the referenced ones of that kind.
- **A location stacks on its host.** The host's inline block still applies on the
  location's path; the location's own is added to it.

| Key | Type | Default | Required | Description |
|---|---|---|---|---|
| `auth` | AuthMiddleware | none | no | Identical to a `type: auth` middleware's `auth` spec: `identityProvider`, `mode`, `requiredRoles`, `allowFrom`, `clientCertRoles`, `basic`. **Every mode works inline, `basic` included**, so `auth.basic.realm` and `auth.basic.users[].username`/`.passwordHash` are valid here, see [BasicAuthSpec](middleware.md#middleware-auth-basic-users). A plaintext `password` field on a `PUT` is hashed server-side on a host or location exactly as on a middleware. See [AuthMiddleware](middleware.md#authmiddleware-auth) for every field rule. |
| `rateLimit` | RateLimitMiddleware | none | no | Identical to a `type: rate-limit` middleware's `rateLimit` spec: `requestsPerSecond` **or** `requests`+`window`, `burst`, `allowFrom`, `blockFor`. See [RateLimitMiddleware](middleware.md#ratelimitmiddleware-ratelimit). |

Validation is the middleware's, verbatim: `identityProvider` must resolve to an
existing IdentityProvider (a dangling name is a load-time error, and the data
plane fails closed with `503` if one ever reaches it), `allowFrom` is refused in
`oidc` and `forward-auth` mode, `clientCertRoles` is `client-cert` only, and a
rate limit must set exactly one of the two rate forms.

The five-host shape, with no middleware objects at all:

```yaml
# config/proxy-hosts/grafana.yaml
name: grafana
domains: [grafana.example.com]
upstream: {scheme: http, host: 192.0.2.20, port: 3000}
tls: {certificateRef: wildcard, forceSSL: true}
auth:
  identityProvider: authentik      # the only other object you need
  mode: forward-auth
  requiredRoles: [admin]
rateLimit:
  requests: 100
  window: 1m
  allowFrom: [10.0.0.0/8]          # the LAN is never throttled
locations:
  - path: /api
    rateLimit: {requestsPerSecond: 5}   # tighter on the API path only
```

## Deprecated fields

Still parsed so existing YAML keeps loading. They are gone from the UI, the
OpenAPI schema and the tables above, and set nothing at runtime.

| Field | Status | Reason |
|---|---|---|
| <span id="proxy-host-websockets-upgrade"></span>  `websocketsUpgrade` | Deprecated, ignored | WebSocket upgrades always work: the proxy forwards the `Upgrade` handshake, and the compression and header-strip layers both special-case a `101`. The field is retained only so existing YAML keeps loading. |
| <span id="proxy-host-tls-http2"></span>  `tls.http2` | Deprecated, ignored | HTTP/2 is always offered: the listener advertises `h2,http/1.1` via ALPN unconditionally, so there is nothing per-host to switch. |

---
