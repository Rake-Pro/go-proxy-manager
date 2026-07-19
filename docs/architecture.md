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
(no shell), a repo-local identity, and prompts disabled. Rollback comes in two
scopes, both recorded as a new commit (a revert is itself revertible): a
**whole-tree revert** (`Store.Revert`) resets the entire config to a target commit
(`git read-tree --reset -u` + `clean -fd`), and a **per-object revert**
(`Store.RevertObject`) restores only one object's file from a target commit
(`git checkout <hash> -- <rel>`, the path always after `--` and derived from the
trusted object-kind directory mapping) so objects created after that commit are
left untouched. Both re-validate the whole graph before committing and roll the
working tree back to HEAD if the result does not load cleanly; a per-object revert
whose object is absent at the target commit is refused rather than deleting it.

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

**Listeners** (`internal/dataplane`). An HTTPS listener (TLS 1.2+ by default, a
fixed set of forward-secret AEAD cipher suites, ALPN `h2,http/1.1`) and an HTTP
listener. A host may pin a higher minimum TLS version (`tls.minTLSVersion: "1.3"`)
applied per-connection by SNI via `GetConfigForClient`. Certificates are chosen
per-connection by SNI: an exact-domain match wins, otherwise the left-most label
is stripped and a wildcard match is tried; an unknown SNI is an error (there is no
default certificate to leak). Custom certs load from the cert store; ACME certs
load from their issued artifacts, and an unissued ACME cert is skipped until the
manager produces it. **Stream hosts** add their own raw TCP and/or UDP listeners
(one per `listenPort`), reconciled on every reload — ports added are opened, ports
removed are closed, and a changed backend is swapped without dropping the port.

**Routing.** An HTTP(S) request is dispatched by `Host` to its compiled handler:
**proxy hosts** run the middleware chain to a reverse proxy; **redirect hosts**
return the configured 3xx to their target (scheme/status/path-preservation per
config); **dead hosts** return a fixed status (default 404). An unknown host →
404; no default-host leakage. On the HTTP listener, a host with `forceSSL` gets a
308 redirect to HTTPS. Within a proxy host, **locations** are matched
longest-prefix-first and fall back to the host default; a location carries its own
upstream and a middleware/access-list chain that is appended to the host's, so it
is always at least as restrictive.

**Upstream groups** (`internal/dataplane/upstreamgroup.go`). A proxy host (or a
single location) may reference an `UpstreamGroup` instead of one upstream: an
ordered backend list with per-group health state. A health manager living on the
data-plane server (across reloads) runs one prober per group — TCP connect or
HTTP GET, with rise/fall hysteresis — and live-traffic connect failures feed the
same counters. Reloads are serialized and staged: a new config's group state is
built first, the router compiles against it, and only a successful build commits
(an unchanged group keeps its probers and up/down state; a rejected config
disturbs nothing). Per request, a failover-aware transport orders the healthy
upstreams by the group's policy — `failover` (list order), `round-robin`
(smooth weighted), `least-connections` (in-flight/weight), `ip-hash`
(rendezvous) — optionally honors a signed sticky-session cookie with a
server-enforced TTL, and retries the next candidate **only on connect-phase
errors** (dial/TLS — the request was never transmitted, so non-idempotent
requests cannot double-apply; request bodies up to 1 MiB are buffered to make
the replay possible). With every upstream down the group fails open and attempts
them anyway. Live state is exposed at `GET /api/upstream-health`.

**Middleware chain** (`internal/dataplane/chain.go`). Each host/location compiles
to a handler that wraps the reverse proxy in a fixed order:

```
request → rate-limit → access-list → auth → guard → headers → rewrite → reverse proxy → upstream
```

Rate limiting is outermost; path rewrite is innermost (closest to the backend).
The access-list sits ahead of auth, so an IP the list would deny is dropped
before any auth work runs (no forward-auth subrequest to the IdP, no OIDC
redirect).

The **rewrite** middleware (`internal/dataplane/rewrite.go`) does exact-match
request-path replacement just before the request enters the reverse proxy. On an
exact `r.URL.Path` hit it swaps in the target path (clearing `RawPath` so Go
re-derives the escaped form) and forwards the request unchanged otherwise -
same method, same body, no HTTP redirect. Exact matching (a single map lookup,
no regex) sidesteps the path-confusion and ReDoS classes pattern rewrites
invite. Because it is wrapped innermost, the replacement is purely
upstream-facing: rate-limit, access-list, auth and guard all evaluate the
original client path, so a rewrite can never carry a request past a path-scoped
security control. Its motivating case is repairing a client that mangles an
upstream path - e.g. adding the trailing slash a mobile OIDC client strips off
Authentik's `/application/o/token` endpoint, which Django would otherwise answer
`405`. The reverse proxy sets `X-Forwarded-*`, preserves the client `Host`, and
carries WebSocket upgrades transparently. Redirects that an upstream emits to its
own address are rewritten to the public scheme/host.

## Trust model

- **Peer-rooted, per-host trust.** Access-control and identity decisions use the
  connection peer IP (`RemoteAddr`), never a forwarded header — unless that peer is
  a trusted proxy *for that host*, in which case `X-Forwarded-For` is honored
  right-to-left. The trusted-proxy set is per-host (the forward-auth
  `trustedProxies` of the IdPs the host references), not a global union across all
  hosts, so a proxy trusted by one host cannot spoof another host's client IP.
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
