# Settings: error pages

Custom HTML pages for the errors gpm generates itself, fleet-wide or per host.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-error-pages"></span> `errorPages` | ErrorPagesConfig | Default custom error pages for every host (below). A [ProxyHost](../proxy-host.md)'s own `errorPages` overrides this. Zero value keeps gpm's built-in plain-text error output. |

> In the admin UI these live in their own **Error pages** section (not under
> Settings); a host's override stays in that host's editor. The config schema is
> unchanged either way: the section edits `settings.errorPages`.

Custom HTML pages for errors **gpm itself generates**: upstream unreachable
(502 connect/handshake failure, 504 a timeout awaiting the upstream), access
denied (403 from an access list, a guard middleware, or a geo rule), rate
limited (429), a dangling middleware/access-list reference (503), a dead
host, and every terminal refusal an [auth middleware](../middleware.md)
writes. The upstream's own error response (its own 500, its own 404 page) is left
untouched **unless** its status is also listed in `interceptUpstream`. A status
with no matching template, and unconfigured settings/host entirely, falls
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
  responses verbatim. **The IdP response always wins there**: gpm never
  overwrites it with an error page, because that page would replace a working
  login screen.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-error-pages-dir"></span> `dir` | string | Directory of `html/template` files named `<status>.html` (e.g. `502.html`), plus an optional `default.html` fallback used when no status-specific template matches. Relative to the managed cert store, confined exactly like a custom certificate's files: no absolute path, no `..`. |
| <span id="settings-error-pages-inline"></span> `inline` | map[string]string | Status code (as a string, e.g. `"502"`), or the literal `"default"`, mapped directly to `html/template` source, for a handful of pages an operator would rather keep in config than mount a directory. |
| <span id="settings-error-pages-intercept-upstream"></span> `interceptUpstream` | []int | Status codes (4xx/5xx) for which the **upstream's own** error response body is also replaced by the configured page. Default: only gpm-generated errors are replaced, never the upstream's own. |

Templates execute with `{{.Status}}` (int), `{{.StatusText}}` (e.g. `"Bad
Gateway"`), `{{.Host}}` (the matched ProxyHost name), and `{{.RequestID}}` (the
`X-GPM-Request-Id` value when `-debug-headers`/`GPM_DEBUG_HEADERS=1` is on,
empty otherwise), `html/template`, so all four are contextually escaped. A
**ProxyHost's own `errorPages`** is resolved first for a given status (falling
back to its own `default` template, then to the settings-level pages) so a host
override always wins; a host with no override of its own uses the
settings-level pages outright.

## Allowed template constructs

An error page prints a few short scalar fields, so a template may use only the
constructs below, each with a fixed limit. Anything else is refused on write
and skipped at load (see the next table).

| Construct | Limit | Example |
|---|---|---|
| The whole template (text and actions) | At most 32 KiB, for an `inline` page and a `dir` file alike | |
| Text and the four fields | | `<h1>{{.Status}} {{.StatusText}}</h1>` |
| `if`, `else`, `else if` on a field or a comparison | At most 16 levels of `if`/`with` nesting (each `else if` or `else with` is one level deeper) and at most 512 conditions per template | `{{if .RequestID}}<p>request {{.RequestID}}</p>{{end}}` |
| `with`, `else with` on a field or a comparison | Inside a `with` on a field, `.` is that field. Anywhere else `.` (the whole page data) is refused. | `{{with .Host}}{{.}}{{end}}` |
| `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `and`, `or`, `not` | At most 4 arguments, each a field or a literal of at most 64 bytes | `{{if eq .Status 502 504}}...{{end}}`, `{{or .RequestID "none"}}` |
| `html`, `js`, `urlquery`, `print`, `len` | Exactly one argument, a field | `{{urlquery .Host}}`, `{{len .RequestID}}` |
| `printf` | A literal format of at most 64 bytes, width and precision at most 3 digits, no `*`, no `[n]`, no `#` or space flag; at most 4 arguments, each a field | `{{printf "%s (%d)" .Host .Status}}` |
| Actions that print a value | At most 128 per template (conditions do not count) | |
| Comments and trim markers | | `{{/* note */}}`, `{{- .Host -}}` |

A field may also be piped into one of these functions: `{{.Host | html}}`
counts as `{{html .Host}}`.

Refused: `range`, `template`, `block`, `define`, variables (`{{$x := ...}}`,
`{{$x = ...}}`, `$`), parenthesized calls (`{{len (print .Host)}}`), a
pipeline with more than one function (`{{.Host | html | js}}`), a printing
function inside a condition (`{{if len .Host}}`), a literal printed on its own
(`{{"x"}}`), `index`, `slice`, `call` and any other function. Combine
conditions by nesting `if` blocks instead of `and (eq ...) (ne ...)`.

What these limits bound: the fields are short (`Host` is an object name of at
most 253 bytes, the others are shorter), so one action builds a few KB at
most, a template has at most 128 printing actions, and the nesting limit keeps
the render's stack small. Measured on the largest templates that pass, in every
HTML, CSS, JS and comment context: one render allocates at most about 7 MiB of
heap (the pages that reach the 1 MiB output cap; the rest stay under 4 MiB) and
grows its stack by at most 32 KiB. Each template is also executed once when the
config is built, so html/template's one-time escaping work (up to about half a
second for 32 KiB of text full of `<`) is paid at reload, never by a request.

**When a broken template is caught:**

| Moment | Behaviour |
|---|---|
| Settings write | An `inline` template that does not parse, or whose source is over 256 KiB, is **rejected**, so it never commits. |
| Settings or proxy-host write | A new or changed `inline` template that uses a construct outside the [allowed constructs](#allowed-template-constructs) is **rejected**. A template stored before this rule, kept unchanged, does not block the write. |
| Reload and startup | A stored `inline` template or a `dir` file that uses a construct outside the allowed ones, or is over 32 KiB, is **skipped** with a `WARN` naming it, as if it were not configured: the next fallback (the host's `default`, the settings-level pages, then the built-in response) is served. Every accepted template is executed once with sample values; one that fails to execute is skipped the same way. None of this fails a reload, the pre-write build check or the start. |
| Request | A page whose rendered output would pass 1 MiB stops rendering at that size; gpm serves its built-in response instead (for `interceptUpstream`, the upstream's own body passes through) and logs the template error. |
| Config reload | A parse error (or an unreadable `dir`) **fails the reload** with a clear message; the previously compiled pages stay installed. |
| Startup | A `dir` template that broke out of band is logged as a **warning** and gpm starts with its built-in error output; it never refuses to boot over an error page. |

A rendered page carries `X-Content-Type-Options: nosniff`, matching the
built-in plain-text response, so configuring a page never drops a header the
default one had.

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
