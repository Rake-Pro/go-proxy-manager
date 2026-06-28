# Architecture

go-proxy-manager is one process with two cooperating halves: a **control plane**
that owns configuration and a **data plane** that serves traffic. They are
decoupled by an in-memory snapshot — a config change recompiles the data plane
atomically, so the running state never drifts from the stored config.

```
   write config (UI/API/files)                 serve traffic
            |                                        ^
            v                                        |
   +------------------+   recompile + swap   +------------------+
   |  config store    |--------------------->|   data plane     |
   |  (git, validated)|   (atomic snapshot)  | (router + certs) |
   +------------------+                      +------------------+
            |                                        ^
            v                                        |
   +------------------+                      +------------------+
   |  ACME manager    |---- issues certs --->|   cert store     |
   +------------------+                      +------------------+
```

## Control plane

**Config store** (`internal/store`). Configuration lives as one YAML file per
object under a git repository (default `/data/config`). Every mutation — from the
API or the UI — merges the object into the in-memory config, validates the
**entire graph** (cross-references must resolve; a delete that would dangle a
reference is rejected), writes the file, and makes a git commit. There is no
non-git mode; history is a first-class feature. Git is invoked with a fixed argv
(no shell), a repo-local identity, and prompts disabled.

**REST API + web UI** (`internal/api`, `internal/ui`). The API is a small
JSON CRUD surface over the config objects; the UI is a vanilla-JS single-page app
embedded in the binary with `go:embed`. Both are served by the admin listener.
Mutating requests require an admin session plus a double-submit CSRF token, behind
a same-origin guard.

**Auth** (`internal/auth`, `internal/oidc`, `internal/session`). Operators
authenticate with a local bcrypt password and/or OIDC (authorization code +
PKCE). Sessions are server-side (SQLite), referenced by an opaque `gpm_session`
cookie (`HttpOnly`, `SameSite=Lax`, `Secure` by default). OIDC group claims map to
roles; there is no path by which a user outside the configured admin groups
becomes an admin.

**ACME manager** (`internal/acme`). A background loop (12h interval) issues and
renews `Certificate` objects of type `acme` via DNS-01: register/reuse an account
key per directory URL, create the order, write the `_acme-challenge` TXT record
through the DNS provider, wait for propagation against a public resolver, accept
the challenge, finalize the CSR, and write `fullchain.pem` + `privkey.pem`
atomically. Renewal triggers when a cert is unissued, its domain set changed, or
it is within 30 days of expiry. On any change it signals the data plane to reload.

## Data plane

**Listeners** (`internal/dataplane`). An HTTPS listener (TLS 1.2+, a fixed set of
forward-secret AEAD cipher suites, ALPN `h2,http/1.1`) and an HTTP listener.
Certificates are chosen per-connection by SNI: an exact-domain match wins,
otherwise the left-most label is stripped and a wildcard match is tried; an
unknown SNI is an error (there is no default certificate to leak). Custom certs
load from the cert store; ACME certs load from their issued artifacts, and an
unissued ACME cert is skipped until the manager produces it.

**Routing.** A request is dispatched by `Host` to its compiled host handler
(unknown host → 404; no default-host leakage). On the HTTP listener, a host with
`forceSSL` gets a 308 redirect to HTTPS. Within a host, **locations** are matched
longest-prefix-first and fall back to the host default; a location carries its own
upstream and a middleware/access-list chain that is appended to the host's, so it
is always at least as restrictive.

**Middleware chain** (`internal/dataplane/chain.go`). Each host/location compiles
to a handler that wraps the reverse proxy in a fixed order:

```
request → rate-limit → auth → guard → access-list → headers → reverse proxy → upstream
```

Authentication is outermost; header mutation is innermost (closest to the
backend). The reverse proxy sets `X-Forwarded-*`, preserves the client `Host`, and
carries WebSocket upgrades transparently. Redirects that an upstream emits to its
own address are rewritten to the public scheme/host.

## Trust model

- **Peer-rooted trust.** Access-control and identity decisions use the connection
  peer IP (`RemoteAddr`), never a forwarded header — unless that peer is an
  explicitly configured trusted proxy, in which case `X-Forwarded-For` is honored
  right-to-left.
- **Identity-header stripping.** Headers that carry an asserted identity are
  stripped from any untrusted peer before the request reaches a backend, so a
  direct client cannot forge an identity. Forward-auth and auth-request handlers
  re-strip their own header sets as a second layer.
- **Fail-closed.** Misconfigured or unknown auth modes deny rather than pass; a
  nil/unparseable client IP is denied; an access list with no matching rule falls
  through to its `defaultAction` (deny by default).

## Dependencies

A deliberately small, vetted set (Go 1.26, CGO disabled):

| Dependency | Use |
|------------|-----|
| `coreos/go-oidc/v3` (+ `go-jose/v4`) | OIDC discovery + ID-token verification |
| `golang.org/x/oauth2` | OAuth2 authorization-code flow |
| `golang.org/x/crypto` | bcrypt, TLS helpers |
| `rs/zerolog` | structured logging |
| `gopkg.in/yaml.v3` | config (de)serialization |
| `modernc.org/sqlite` | pure-Go session store (no CGO) |

Everything else is the Go standard library.
