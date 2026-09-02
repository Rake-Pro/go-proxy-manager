# Settings: security headers and response-header stripping

The two halves of one mechanism: response headers gpm adds, and response
headers gpm removes from what an upstream sent.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-security-headers"></span> `securityHeaders` | map[string]string \| map[string]{value,scope} | Fleet-default response headers, each with a per-header `scope` (`all` default / `generated-only` / `proxied-only`) selecting whether it lands on gpm-generated responses, proxied upstream responses, or both. Value is a bare string (scope `all`) or a `{value, scope}` object. A [ProxyHost](../proxy-host.md)'s own `securityHeaders` merges over this per key. Empty (default) ships nothing. Full reference below. |
| <span id="settings-strip-response-headers"></span> `stripResponseHeaders` | []string | Fleet-default list of response headers removed from what an upstream sends (`Server`, `X-Powered-By`, ...). Case-insensitive; a [ProxyHost](../proxy-host.md)'s own list is the **union** with this one. Empty (default) strips nothing. See [StripResponseHeaders](#strip-response-headers-section) below. |

A configurable set of response headers gpm emits on the responses **it**
generates, at the same response layer HSTS is emitted. It closes an audit gap:
gpm's own denial responses (the 401 from an auth gate, the 302 sign-in redirect,
error pages, the 503 fail-closed, the 400 path-rejection, the 404 no-such-host)
previously carried only HSTS, because the auth chain runs outside any headers
middleware, so nothing but the per-host HSTS/robots emission could reach them.

Both levels are a map of header name -> value. Each value is **either** a bare
string (the legacy form, scope defaults to `all`) **or** an object carrying a
`value` and a `scope`:

```yaml
securityHeaders:
  X-Frame-Options: DENY                 # bare string  -> scope: all
  Content-Security-Policy:
    value: "frame-ancestors 'none'"
    scope: generated-only               # object form  -> explicit scope
```

- `settings.securityHeaders` is the fleet default.
- A `ProxyHost`'s own `securityHeaders` **merges over** the settings default
  **per key**: a header the host names replaces the settings value **and its
  scope** for that header, and a header it omits still falls through to the
  settings default (the same "host override wins per key" resolution `errorPages`
  templates use).

### Scope

`scope` declares **where** a header is applied. It is the point of this feature:
some headers are safe on gpm's own pages but break a backed app if injected onto
its proxied responses. The three values:

| `scope` | Applies to gpm-generated responses | Applies to proxied upstream responses |
|---------|:----------------------------------:|:-------------------------------------:|
| `all` (default) | yes | yes |
| `generated-only` | yes | **no** |
| `proxied-only` | **no** | yes |

Every scope is injected **set-if-absent** (a value already on the response
always wins). A header is declared **once**, at one scope: the same name cannot
appear at two scopes (it is a case-insensitive duplicate, which is rejected).

- **gpm-generated responses** are the ones gpm writes itself: auth-gate refusals
  (401/403/503), the OIDC / forward-auth sign-in redirects (302), rendered error
  pages (including the 502/504 upstream-unreachable page), the path-rejection
  400, the no-such-host 404, the misdirected-request 421, and parked / redirect
  hosts. `generated-only` and `all` headers land here.
- **proxied upstream responses** are the ones an upstream actually answered
  (any status, including the app's own errors). `proxied-only` and `all` headers
  land here, always set-if-absent, so an app that sets its own
  `X-Frame-Options: SAMEORIGIN` / `Referrer-Policy` / `X-Content-Type-Options`
  keeps it untouched.

The data plane distinguishes the two at inject time: the reverse proxy marks the
response as proxied only when an upstream actually responds (its `ModifyResponse`
hook), so the upstream-unreachable 502/504 (written by the proxy's error handler,
not the upstream) correctly counts as gpm-generated.

> **Why the scope matters: the real case.** `Content-Security-Policy:
> frame-ancestors 'none'` and a restrictive `Permissions-Policy` are exactly what
> you want on gpm's own error/denial pages, but they **break** a proxied app that
> ships none of its own: Home Assistant, for one, sets no CSP and relies on
> same-origin iframes for add-on ingress, so a `frame-ancestors 'none'` injected
> set-if-absent (the app set nothing, so gpm's value lands) breaks it. Declaring
> those two at `scope: generated-only` keeps them on gpm's pages and off every
> proxied app. That is the placement in the recommended set below.

`Strict-Transport-Security` is **not** settable here: the per-host
[`tls.hsts`](../proxy-host.md) setting owns it, and this feature is
additive: HSTS emission is unchanged on every path but one, a `101` WebSocket
upgrade, where the stdlib hijacks the connection and writes the `101` without
going through the dispatch writer, so HSTS/`X-Robots-Tag` (and these headers) are
absent on that response. This is immaterial: a `wss://` upgrade is always
preceded by the `https://` document load that already delivered HSTS, and an
indexing directive on a WebSocket has no meaning.

Interim responses are handled: an upstream `1xx` (a `103 Early Hints`, or a
`100 Continue` when a client sends `Expect: 100-continue`) does **not** drop the
configured headers, they are injected on the final response, after the reverse
proxy has forwarded and cleared the interim one. (HSTS and `X-Robots-Tag` ride
the same mechanism, for the same reason.)

### Validation

- Header names must be valid RFC 7230 field-name tokens (no CR/LF, no separator
  characters); keys are de-duplicated case-insensitively (so a header is declared
  once, at one scope: it cannot be set at two scopes).
- `scope`, when set, must be one of `all`, `generated-only` or `proxied-only`;
  an unknown scope is rejected. An omitted scope means `all`.
- `Strict-Transport-Security` is refused (HSTS owns it).
- Hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`,
  `Proxy-Authorization`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`) are
  refused.
- Values must contain no CR, LF or control bytes (only horizontal tab is
  allowed), so a value cannot inject an extra response header.

### Recommended set

Ships **nothing** by default (empty map = today's behaviour, opt-in, so no
existing deployment is surprised). A safe paste-ready set, with the two
app-breaking headers scoped to `generated-only`:

```yaml
# settings.yaml
securityHeaders:
  X-Content-Type-Options: nosniff                 # scope all
  Referrer-Policy: strict-origin-when-cross-origin # scope all
  X-Frame-Options: DENY                            # scope all
  Permissions-Policy:
    value: "geolocation=(), camera=(), microphone=()"
    scope: generated-only                          # unsafe on a proxied app
  Content-Security-Policy:
    value: "frame-ancestors 'none'"
    scope: generated-only                          # unsafe on a proxied app
```

`X-Content-Type-Options`, `Referrer-Policy` and `X-Frame-Options` are safe at
`all`: a proxied app that sets its own keeps it (set-if-absent), and gpm's
value is a reasonable default for one that doesn't. `Content-Security-Policy`
(`frame-ancestors`) and `Permissions-Policy` are at `generated-only` because
injecting them onto a proxied app that ships none of its own can break it (see
the scope note above), this keeps them on gpm's own pages, where they are safe.

---

<span id="strip-response-headers-section"></span>

## StripResponseHeaders (`settings.stripResponseHeaders` / `proxyHost.stripResponseHeaders`)

The removal half of the same mechanism: a list of response headers gpm **deletes
from what an upstream sends**, so a backend's `Server: Apache/2.4.1 (Unix)`,
`X-Powered-By: PHP/8.1.0` or `X-AspNet-Version: 4.0.30319` never reaches a
client. It is applied in the data plane's reverse proxy, on the upstream's own
response, so it covers **every** response that upstream returns for the host
regardless of which middlewares that host wires up.

```yaml
# settings.yaml
stripResponseHeaders:
  - Server
  - X-Powered-By
  - X-AspNet-Version
```

```yaml
# config/proxy-hosts/app.yaml - adds to the fleet list for this host
stripResponseHeaders:
  - X-Drupal-Cache
```

### Settings vs per-host: union, not replacement

- `settings.stripResponseHeaders` is the fleet baseline.
- A `ProxyHost`'s own `stripResponseHeaders` is the **union** with that baseline:
  the host's names are stripped **in addition to** the fleet's.

This differs deliberately from `securityHeaders`, which merges per key with the
host value winning. A map has a per-key *value* for a host to override; a list
does not, so the only two possible semantics are "host replaces the fleet
baseline" and "host adds to it". Union is the safe one: a strip list is a
hardening baseline, and a host must not be able to silently re-expose a header
the fleet strips just by naming an unrelated one. **A host cannot opt out of a
fleet-level strip**: remove the name from `settings.stripResponseHeaders` if a
host needs the header through.

### What it can and cannot reach

Stripping operates on the **upstream's own response headers**, before they are
copied onto the response the client sees. That is a structural boundary, not an
ordering convention:

- **Reached**: the headers the backend sent on its final response, whatever the
  status, including a `101 Switching Protocols` WebSocket handshake, so an
  upgrade does not leak the fingerprint an ordinary response hides. The one
  exception is an **interim `1xx`** (a `103 Early Hints`, or a `100 Continue`):
  those are forwarded on a separate path that the strip does not sit on, so
  their headers pass through unstripped. In practice this leaks nothing an
  operator is hiding: an upstream's early-hints headers are `Link` preloads,
  and the stdlib clears the interim header map before the final response is
  written, but a backend that sets `Server` on a `103` would send it there.
- **Never reached**: anything **gpm** adds. Injected
  [`securityHeaders`](#settings-security-headers),
  HSTS, `X-Robots-Tag`, the `Set-Cookie` forward-auth copies back when the IdP
  refreshes a session, the `Content-Encoding`/`Vary` the gzip handler sets, and a
  headers middleware's `setResponse` values all survive regardless of the strip
  list.
- **Nothing to reach**: gpm's own generated responses (auth-gate denials, sign-in
  redirects, error pages, the path-rejection 400, the no-such-host 404,
  parked/redirect hosts, and the upstream-unreachable 502/504) never involve an
  upstream response, so no stripping happens on them at all.

A header named in **both** the strip list and `securityHeaders` therefore ends up
**present with gpm's configured value**: the upstream's copy is removed on the way
in, and gpm's is injected on the way out. Listing `X-Frame-Options`,
`Strict-Transport-Security` or `X-Robots-Tag` is safe and does exactly that:
replace the backend's value with gpm's.

### Sharp edges

Two allowed names deserve care. Both are permitted because they only ever remove
what the **backend** sent, which is a legitimate operator choice, but both change
application behaviour:

- `Set-Cookie`: removes the backend's own cookies, which breaks that app's
  sessions. (gpm's forward-auth session cookie is unaffected; it is not an
  upstream header.)
- `WWW-Authenticate`: suppresses the backend's auth challenge, so a browser
  never prompts for its basic-auth credentials.

### Relationship to the headers middleware

The [headers middleware](../middleware.md)'s `removeResponse` does
the same removal, but inside the **auth tier of a host's middleware chain**: it
only applies where the middleware is attached, and responses generated by the
auth layer itself (denials, sign-in redirects) never pass through it.
`stripResponseHeaders` is the recommended edge-wide mechanism: one fleet list,
applied in the reverse proxy on the upstream's own response, with no per-host
wiring to forget.
`removeResponse` remains for per-middleware, per-location removal (a rule that
should apply to one path prefix, or only alongside that middleware's other
mutations).

### Validation

- Names must be valid RFC 7230 field-name tokens; an empty or malformed name is
  rejected at config write (`400`) rather than silently ignored at runtime.
- Names are de-duplicated case-insensitively (a header is listed once; matching
  against a response is case-insensitive regardless).
- Hop-by-hop headers (`Connection`, `Keep-Alive`, `Proxy-Authenticate`,
  `Proxy-Authorization`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`) are
  refused: they carry the response's own connection and framing semantics, so
  removing them breaks the response instead of hiding a backend detail. (This is
  also what keeps a `101` handshake intact while its `Server`/`X-Powered-By` are
  stripped.)
- `Content-Type`, `Content-Length`, `Content-Encoding`, `Vary` and `Location` are
  refused for the same reason: they are the response's own semantics.
  `Content-Type` is the sharpest: with no `Content-Type`, Go falls back to
  content sniffing, so a JSON or text body whose first bytes look like markup
  would be re-labelled `text/html`, turning a config typo into stored XSS.
  `Content-Encoding`/`Vary` are body encoding and the cache key, and `Location`
  is the entire meaning of a 3xx.
- `Sec-WebSocket-Accept`, `Sec-WebSocket-Protocol` and `Sec-WebSocket-Extensions`
  are refused for the same reason, and matter because `101` responses **are** in
  scope: `Sec-WebSocket-Accept` is the server's proof it understood the
  handshake, and a browser aborts the connection without it, so stripping one
  would break WebSockets on the host outright.
- Empty (the default) strips nothing, so an existing deployment is unchanged.

---
