# Settings: error pages

Custom HTML pages for the errors gpm generates itself, fleet-wide or per host.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-error-pages"></span> `errorPages` | ErrorPagesConfig | Default custom error pages for every host (below). A [ProxyHost](../proxy-host.md)'s own `errorPages` overrides this. Zero value keeps gpm's built-in plain-text error output. |

> In the admin UI these live in their own **Error pages** section (not under
> Settings); a host's override stays in that host's editor. The config schema is
> unchanged either way - the section edits `settings.errorPages`.

Custom HTML pages for errors **gpm itself generates**: upstream unreachable
(502 connect/handshake failure, 504 a timeout awaiting the upstream), access
denied (403 from an access list, a guard middleware, or a geo rule), rate
limited (429), a dangling middleware/access-list reference (503), a dead
host, and every terminal refusal an [auth middleware](../middleware.md)
writes. The upstream's own error response (its own 500, its own 404 page) is left
untouched **unless** its status is also listed in `interceptUpstream`. A status
with no matching template - and unconfigured settings/host entirely - falls
back to gpm's historical plain-text output, unchanged from before this feature.

**Auth-middleware refusals** covered, by gate:

| Gate | Statuses |
|------|----------|
| `forward-auth` | `401` (untrusted peer or no identity asserted), `403` (role not permitted) |
| `client-cert` | `401` (no handshake-verified certificate), `403` (subject unmapped or role not permitted) |
| `auth-request` | `403` (auth server said forbidden), `502` (auth server unreachable or an unexpected status) |
| `oidc` | `403` (role not permitted), `401` (IdP returned an error, or the code exchange failed), `400` (invalid login state), `404` (request Host is not a domain of this host), `502`/`500` (discovery or state generation failed) |
| any auth middleware that cannot be compiled | `503` ("authentication not available") |

The per-host override applies to these exactly as it does to an access-list
denial: the host's own `errorPages` is resolved first, then the settings-level
pages, then the plain-text default.

Two things are deliberately **not** error pages:

- **Redirects into a sign-in flow.** The OIDC gate's `302` to the IdP and the
  `auth-request` gate's `302` into the outpost sign-in are flows, not errors.
- **Anything the identity provider itself served.** In `auth-request` mode the
  IdP owns its sign-in, callback and sign-out endpoints, and gpm proxies those
  responses verbatim. **The IdP response always wins there** - gpm never
  overwrites it with an error page, because that page would replace a working
  login screen.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-error-pages-dir"></span> `dir` | string | Directory of `html/template` files named `<status>.html` (e.g. `502.html`), plus an optional `default.html` fallback used when no status-specific template matches. Relative to the managed cert store - confined exactly like a custom certificate's files: no absolute path, no `..`. |
| <span id="settings-error-pages-inline"></span> `inline` | map[string]string | Status code (as a string, e.g. `"502"`) - or the literal `"default"` - mapped directly to `html/template` source, for a handful of pages an operator would rather keep in config than mount a directory. |
| <span id="settings-error-pages-intercept-upstream"></span> `interceptUpstream` | []int | Status codes (4xx/5xx) for which the **upstream's own** error response body is also replaced by the configured page. Default: only gpm-generated errors are replaced, never the upstream's own. |

Templates execute with `{{.Status}}` (int), `{{.StatusText}}` (e.g. `"Bad
Gateway"`), `{{.Host}}` (the matched ProxyHost name), and `{{.RequestID}}` (the
`X-GPM-Request-Id` value when `-debug-headers`/`GPM_DEBUG_HEADERS=1` is on,
empty otherwise) - `html/template`, so all four are contextually escaped. A
**ProxyHost's own `errorPages`** is resolved first for a given status (falling
back to its own `default` template, then to the settings-level pages) so a host
override always wins; a host with no override of its own uses the
settings-level pages outright. Templates are parsed at config reload - a parse
error (or an unreadable `dir`) **fails the reload** with a clear message rather
than installing a half-broken set.

```yaml
# settings.yaml
errorPages:
  dir: errorpages                    # relative to GPM_CERT_DIR
  interceptUpstream: [502, 503]

# a proxy host overriding just its own 429 page
errorPages:
  inline:
    "429": |
      <html><body><h1>Slow down</h1><p>{{.Host}} is rate-limiting you.</p></body></html>
```

---
