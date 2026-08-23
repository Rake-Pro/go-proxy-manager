# Architecture

## Relationship to Nginx Proxy Manager

go-proxy-manager is a clean-room reimplementation of the ideas behind
[Nginx Proxy Manager](https://github.com/NginxProxyManager/nginx-proxy-manager)
(MIT) and [NPMplus](https://github.com/ZoeyVid/NPMplus) (AGPLv3); no code or
configuration was copied from either. NPMplus is referenced as behavioural
inspiration only, given its AGPLv3 license. The one place either project's
format is read directly is `internal/importer/`, which parses Nginx Proxy
Manager's export schema to migrate hosts, certificates, and access lists into
go-proxy-manager's own config model.

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
`APIToken` is exempt from both: its file carries a credential digest, so a
per-object revert is refused (`ErrNotRevertible`) and the whole-tree revert
preserves the `api-tokens` directory verbatim across the restore.

**REST API + web UI** (`internal/api`, `internal/ui`). The API is a small
JSON CRUD surface over the config objects; the UI is a vanilla-JS single-page app
embedded in the binary with `go:embed`. Both are served by the admin listener.
Mutating requests require an admin session plus a double-submit CSRF token, behind
a same-origin guard. Every admin response also carries hardening headers: a strict
CSP (`script-src 'self'`, with carve-outs only for inline style attributes and
Google Fonts) as an XSS backstop behind the UI's output escaping, `nosniff`,
`X-Frame-Options: DENY`, and `Referrer-Policy: same-origin`. HSTS is deliberately
left to the data plane, which is the TLS edge for the proxied admin host.

**Auth** (`internal/auth`, `internal/oidc`, `internal/session`). Operators
authenticate with a local bcrypt password and/or OIDC (authorization code +
PKCE). Sessions are server-side (SQLite), referenced by an opaque `gpm_session`
cookie (`HttpOnly`, `SameSite=Lax`, `Secure` by default). OIDC group claims map to
roles; there is no path by which a user outside the configured admin groups
becomes an admin.

**API tokens** (`internal/auth`, `internal/model`). Besides session cookies the
admin API accepts scoped bearer tokens (`Authorization: Bearer gpm_...`). The
secret is minted server-side and returned exactly once; only its SHA-256 digest is
committed as an `APIToken` object, so the git config never holds a usable
credential. Bearer resolution runs **before** the cookie path and never falls
through to it, and matching is a constant-time digest compare across the enabled,
unexpired tokens. The token set is **cached in the authenticator and invalidated
from the single config-reload path**, so a revoked token stops working
immediately while an unauthenticated bearer flood cannot force a full config load
(walk + parse + whole-graph validate) per request — a failed bearer auth never
reaches the login rate gate, so re-reading per request was an internet-facing DoS
lever. A token principal is admin-role (satisfying the coarse gate) but
scope-limited per route: `register()` wraps each resource route in a
`<plural>:read` / `<plural>:write` check, with `admin` reserved for token
management, **writing settings**, backup, restore, whole-config revert and the
pprof endpoints. Scope enforcement is injected into the API as a closure, so the
API package stays independent of `auth` for testing; the daemon's implementation
**denies when no principal is on the request** (which the mounted route makes
impossible — reaching it means broken wiring, and a broken authorization check
must fail closed). A nil closure means "allow" and exists only for the unwired
case, i.e. tests. `/debug/pprof/` is gated the same way plus its own admin-scope
check, because a heap dump and the process command line carry resolved backend
credentials in cleartext and every token principal is admin-*role*. Token
principals are CSRF-exempt by
construction: the double-submit token defends against *ambient* browser
credentials, and a bearer header is never attached automatically. Last-use is
tracked in memory only — persisting it would turn every API call into a git commit.

**DNS sync** (`internal/dnssync`). An optional reconciler that publishes CNAMEs
for the proxy hosts that opted in (`dns.lanDirect` / `dns.publicCname`), into a
local Pi-hole v6 resolver and/or the authoritative Cloudflare zone. It follows the
webhook dispatcher's shape — live settings read on every run, non-blocking
triggers, a hardened HTTP client that never follows redirects and refuses
link-local destinations at connect time — with two additions. Reconcile is
**full-state**: the desired set is recomputed from the whole config and compared
against what the backend actually holds, so out-of-band drift is repaired in both
directions. And deletion is **ledger-gated**: gpm removes only records it recorded
creating.

That ledger (`model.DNSLedger`, persisted as the singleton `config/dns-ledger.yaml`
next to `settings.yaml`) is the subsystem's load-bearing piece, and it exists
because the thing it replaced was not ownership at all. Pi-hole/dnsmasq CNAMEs
carry no comment field, so the original backend inferred ownership from target
equality — "this CNAME points at `apexTarget`, therefore gpm made it". On a shared
apex that is simply false, and on 2026-08-01 it cost an operator 19 hand-written
LAN CNAMEs: they enabled the backend for the first time, no host carried
`dns.lanDirect` yet, so the desired set was empty, every one of those records
looked managed, and the first reconcile deleted the lot. Ownership is now recorded
rather than inferred: `decide()` (in `dnssync.go`) is the single place the rules
live, and both backends plus the dry-run planner call it, so a preview cannot
disagree with the run it previews. Per desired name it **creates** what is absent,
**adopts** what is already correct but not yet in the ledger (the migration path —
an empty ledger makes a first reconcile adopt-only, never a purge), **retargets** a
record it *created* that still holds exactly what gpm wrote after `apexTarget`
moved, and **skips and warns** on a name held by a record it does not own rather than
shadowing or replacing it. It **deletes** only ledger entries the config no longer
wants, and only while the record still matches what the ledger says gpm left there
— re-pointed out of band, it is disowned instead. A name absent from the ledger is
never in a delete list, whatever it points at.

Each entry also records **how** the claim was acquired (`adopted`), because
adoption is a claim on a record somebody else made and must not become permission
to destroy it: an adopted entry the config no longer wants is **released** (dropped
from the ledger, record left standing), never deleted. The same applies when
`apexTarget` moves - a retarget is a delete plus a create, so an adopted record is
released there too rather than replaced, which also stops the claim being quietly
upgraded to "created" and arming a later deletion. Without that distinction
adoption was a one-way trap — turn `dns.lanDirect` on for a hand-written name, turn
it off again, and the next reconcile deleted the operator's record, which is the
2026-08-01 incident deferred by one config edit. An entry with no recorded
provenance (a ledger written before the field existed) reads as adopted, the only
reading of a missing field that cannot destroy anything on upgrade. Deletions are
logged at warn together with the ledger revision that authorised them, since a
whole-tree revert can restore a claim that reality has moved past. A reconcile
hands the revision it read the ledger at back to the store on write; if the repo
has moved since (a revert landing mid-run) the write is refused, and the run
re-reads and rewrites without the claims the revert withdrew rather than
resurrecting them. Cloudflare keeps its
`managed-by:gpm` record comment as an independent second condition on both
adoption and deletion (re-checked inside the delete call itself, so it cannot
become an arbitrary-delete primitive); the ledger is authoritative, the comment is
additive. The ledger lives in the config repo rather than beside it so it is
committed, diffable and reverted with everything else — rolling the config back to
before a host existed also rolls back gpm's claim on the record that host
published. It is written by the reconciler alone (there is no CRUD route onto it,
which would amount to an "authorise a DNS deletion" API), and an unchanged ledger
produces no commit. `GET /dns-sync/plan` renders the same decisions as a read-only
dry run, so enabling a backend is checkable before it is done. Runs are serialised by
a single-flight mutex. The event-triggered path *waits* for an in-flight run (that
is what makes trigger coalescing correct: the follow-up must see the config that
caused it), so a bulk restore costs one reconcile rather than one per object; the
HTTP-triggered `ReconcileNow` instead refuses with `ErrReconcileInProgress` →
**409**, so repeated manual runs cannot pile blocked goroutines up behind a slow
backend. The Cloudflare client is separate
from the ACME solver on purpose: record lifecycle management and certificate
issuance should not be able to break each other.

Both backends read defensively and write reversibly. A Pi-hole listing that does
not carry a `config.dns.cnameRecords` list is an **error**, never an empty
resolver: a nil slice would read as "everything gpm owns has been deleted out of
band", which a full-state reconciler answers by emptying the ledger and reporting a
clean run. Cloudflare pagination terminates on a short page and treats
`result_info` as advisory, so a response without it cannot truncate the listing
into a partial view of the zone. Neither backend can update a record in place, so a
retarget is a delete followed by a create; if the create fails the **original is
restored** and the run fails loudly, rather than leaving the name unresolved until
some later reconcile heals it, and the counter is incremented as soon as the delete
lands so a destructive half-step can never be reported as a run that changed
nothing. The Pi-hole session is closed on a context detached from the caller's,
because logout is reached by `defer` and the commonest reason to reach it is the
caller having gone away — cancelling the logout with it leaks one of Pi-hole's few
session slots per aborted run.

**Kubernetes Ingress discovery** (`internal/k8s`). An optional, read-only poll
loop that turns annotated cluster `Ingress` objects into gpm-managed proxy hosts,
which then feed the DNS reconciler above — one DNS code path, not two. The client
is plain `net/http` + `encoding/json` against `/apis/networking.k8s.io/v1`
(no `client-go`: its transitive tree would dwarf this project's entire direct
dependency set), with in-cluster *or* explicit `apiURL`/`tokenFile`/`caFile`
config — the latter is the real deployment, because gpm runs on the edge host
rather than as a pod. The bearer token is re-read from disk on a TTL, and dropped
immediately on a `401`, so a rotated projected ServiceAccount token keeps working
unattended. Transport hardening matches `dnssync`: TLS verified against the
supplied CA with no skip-verify, redirects never followed, link-local
destinations refused at connect time, bounded reads and bounded pagination.

A derived host is a **full** `ProxyHost`, not a reduced one: the operator's
template (or the profile an `Ingress` selected by name) supplies the upstream,
TLS/mTLS, middleware and access-list chains, websockets, `robotsNoIndex`,
`timeouts` and `tags`, and every reference type among them is deep-copied per
host so two derived hosts never share backing memory with each other or with the
loaded settings. Parity matters because the alternative is silent: a service
moved into discovery that loses its no-index header or its upstream timeout keeps
serving, just differently, and the workaround is a second mechanism (a `headers`
middleware) for something the model already expresses. The one field a derived
host deliberately cannot carry is `locations` — path routing belongs to the
cluster ingress controller, which does it from the same `Ingress` gpm read.

Three properties define the reconciler. It is **full-state** — the desired set is
recomputed from a complete list on every poll and compared with the config, so a
missed event is impossible by construction. It is **ownership-gated** — only
proxy hosts labelled `gpm.rake.pro/managed-by: ingress-discovery` are ever
written or deleted, and a collision with a hand-written host is skipped with a
warning, exactly as the DNS backends treat a record they do not own. Ownership is
gated on the **domain** as well as the name, because the router keys its
per-domain maps by hostname and fills them in config load order: without it a
derived host whose name merely sorts after the operator's could take over
`sso.example.com` and serve it with the template's chain instead of the
operator's. The same rule is enforced one layer down — `Config.Validate` rejects
any two *enabled* hosts claiming one domain, whatever wrote them — and re-checked
under the store lock at write time, since the plan is computed before a
multi-second network list. And it **freezes on error** — a managed host is deleted
only after a complete, successful, fully-paginated list; any transport error,
non-`200`, decode failure, mid-pagination failure, over-cap body, exceeded
per-reconcile deadline, or a `200` whose body is not a `kind: IngressList` aborts
before any write, and the client never returns a partial list with a nil error,
so "empty" and "failed" are different return shapes rather than different values
of one. Everything security-relevant on a
derived host (upstream, certificate, middleware, access lists) comes from an
operator-authored chain — the default `template`, or one of the named
`profiles` an Ingress may **select by name** with `gpm.rake.pro/profile`. The
Ingress contributes only strictly-validated, suffix-restricted hostnames, two DNS
booleans, and that one name. Profiles exist because a real fleet is
heterogeneous (public-and-rate-limited, SSO-and-VPN, login-middleware) and a
single template can only adopt the group that happens to match it; the annotation
carries a *name* and never a chain, so a cluster tenant chooses among
configurations the operator sanctioned but can never invent one, and an undefined
profile name skips the Ingress rather than silently downgrading it to the
default. Every profile validates exactly as the template does, at settings-write
time. Because gpm is off-cluster,
in-cluster Service DNS is unusable, so the upstream is the ingress controller's
address and the data plane's preserved `Host` header is what routes the request
to the right workload. A whole reconcile lands as **one commit**
(`Store.ApplyBatch`), and a no-drift run writes nothing at all. `ApplyBatch` is
transactional in both directions: it takes an ownership guard it re-evaluates
against freshly loaded state under the store lock, and it snapshots every file it
touches so a failed write, removal or commit rolls the working tree back — a tree
left mutated but uncommitted would be read as live config by the next load and
swept into the next unrelated commit.

**ACME manager** (`internal/acme`). A background loop (12h interval) issues and
renews `Certificate` objects of type `acme`: register/reuse an account key per
directory URL (per external account when EAB is configured), create the order,
solve every authorization, finalize the CSR, and write `fullchain.pem` +
`privkey.pem` atomically. Renewal triggers when a cert is unissued, its domain
set changed, or it is within 30 days of expiry. On any change it signals the data
plane to reload.

Two challenge types are solved. **DNS-01** writes the `_acme-challenge` TXT
record through the referenced DNS provider (Cloudflare, DigitalOcean, Hetzner,
deSEC - each a plain REST client behind one `DNSSolver` interface) and waits for
propagation against a public resolver before accepting; it is the only way to
prove a wildcard. **HTTP-01** needs no provider: the manager parks the key
authorization in an in-memory token store (`HTTP01Store`, entries expiring with
the order) which the data plane reads through the one-method
`dataplane.ACMEChallengeStore` interface - the two packages stay decoupled, the
manager owns the map, the listener only reads it. The plaintext `:80` handler
answers `/.well-known/acme-challenge/<token>` for an in-flight token before host
routing, the force-SSL redirect, and auth run, so a name that has no host yet (or
that redirects everything to https) can still be validated; an unknown token
falls through to normal routing so a proxied upstream's own ACME client keeps
working. Only the HA leader runs the manager, so only it holds tokens.

**HA role gate** (`internal/ha`, phase 1 of [design/ha.md](design/ha.md)). A
two-node pair designates its single writer statically: `GPM_HA_ROLE=leader`
(default) runs the ACME and Ingress-discovery loops and accepts admin/API
writes; `GPM_HA_ROLE=follower` disables both loops and wraps the API mux in a
gate that refuses every mutating method with `503`, naming the leader. The
follower's config arrives instead from `Store.FollowRemote`, which polls
`git pull --ff-only` against the leader's repo and calls the same `reload()` path
as any other change, but only when HEAD actually moved; a pull that is not a
clean fast-forward is refused and logged, never merged or reset, so the two repos
cannot diverge. Independently of the role, every instance re-reads the persisted
SSO revocation watermark (`<cert-dir>/sso_not_before`) on a ticker and advances
it monotonically, so a revoke - or any out-of-band edit - takes effect within one
interval rather than at the next restart. Traffic-side failover is out of
process (keepalived VIP, see [ha.md](ha.md)).

**Metrics** (`internal/metrics`). An optional Prometheus exposition at
`GET /metrics` on the admin listener (`-metrics` / `GPM_METRICS=1`; `404` when
off), gated by admin role plus a dedicated `metrics:read` token scope. It is a
small in-tree implementation of the exposition format - integer counters and
gauges, fixed-bucket histograms, a text writer - rather than
`prometheus/client_golang`, whose transitive tree would have several times
outweighed this project's entire direct dependency set for one read-only route.

Two properties matter more than the metric list. **Cardinality is bounded by
config, not by clients**: every `host` label is the operator's
`ProxyHost`/`StreamHost` *name*, resolved once per request in the observe
wrapper, never the client's `Host` header - a header is attacker-chosen, so
labelling by it would hand any client an unbounded series generator pointed at
this process's memory. A request matching no host collapses onto one `-` label,
and each metric independently caps its series count and folds the rest into a
single `__overflow__` series, so the bound survives a mistake upstream of that
rule. And **the data plane does not know about metrics**: it declares a
`MetricsHook` interface (`internal/dataplane/metricshook.go`) that
`internal/metrics` happens to satisfy structurally, exactly as the ACME manager
reaches the plaintext listener through `ACMEChallengeStore`. With no hook
installed the observe wrapper returns the handler unwrapped and every call site
is one atomic load, so an instance without `-metrics` pays nothing. Subsystems
that already track their own state (ACME expiry and renewal failures, the DNS
and Ingress reconcilers' run/success timestamps and counts) are read at scrape
time through pull collectors wired in `cmd/gpm`, so nothing is mirrored into a
second source of truth and no package gains a metrics import.

## Data plane

**Listeners** (`internal/dataplane`). An HTTPS listener (TLS 1.2+ by default, a
fixed set of forward-secret AEAD cipher suites, ALPN `h2,http/1.1`) and an HTTP
listener. A host may pin a higher minimum TLS version (`tls.minTLSVersion: "1.3"`)
applied per-connection by SNI via `GetConfigForClient`. Certificates are chosen
per-connection by SNI: an exact-domain match wins, otherwise the left-most label
is stripped and a wildcard match is tried; an unknown SNI is an error (there is no
default certificate to leak). Custom certs load from the cert store; ACME certs
load from their issued artifacts, and an unissued ACME cert is skipped until the
manager produces it. Both listeners bind a bare `:port` by default, which in Go is
the IPv6 wildcard with v4-mapped addresses enabled — one socket serving both
families, so an inbound IPv6 client is routed, gated and logged as its own v6
address with no second listener and no toggle. **Stream hosts** add their own raw
TCP and/or UDP listeners (one per `listenPort`), reconciled on every reload —
ports added are opened, ports removed are closed, and a changed route table is
swapped without dropping the port.

**Inbound PROXY protocol** (`internal/dataplane/proxyproto.go`). When
`settings.proxyProtocol` is on, every data-plane listener (HTTP, HTTPS, and each
TCP stream listener) is wrapped so an accepted connection parses a HAProxy PROXY
header — v1 text and v2 binary, hand-written against the spec with no dependency,
v2 TLVs consumed and ignored — and the asserted source replaces the connection's
`RemoteAddr`. That is deliberately the *only* integration point: every IP-based
control in the system already derives from `RemoteAddr`, so access lists, geo,
guards, rate limits, the basic-auth lockout, `X-Forwarded-For`, the access log,
the OIDC gate and the metrics labels all follow with no per-feature wiring. The
header is an unauthenticated claim, so it is parsed **only** when the real TCP
peer is inside `trustedCIDRs`; from any other peer the bytes are left untouched as
payload and the peer address stands (warned once per peer). Parsing is lazy — on
the first `Read`/`RemoteAddr`, not in `Accept` — so one stalled sender cannot hold
up the accept loop; a malformed header closes the connection with a sticky error,
and a stalled one is cut at the configured deadline. The compiled config is read
from an atomic pointer per connection, so a settings change applies without
rebinding a listener.

**Stream TLS and L4 access control** (`internal/dataplane/streamhost.go`,
`clienthello.go`). A TCP stream host may declare `tls.mode`. In `passthrough`, the
forwarder peeks the ClientHello with a hand-written, bounded, stdlib-only parser
(record header → handshake message, reassembled across records → the
`server_name` extension), routes on the SNI, and replays the peeked bytes
verbatim to the backend — gpm never decrypts and never holds the key. In
`terminate`, the same peeked bytes are replayed into `crypto/tls` with the
referenced certificate and plaintext is forwarded on. Routing is compiled per
listen port into an exact map plus single-label wildcard suffixes; a name no host
claims (or a connection that is not TLS) is closed rather than handed to an
arbitrary backend. Several hosts share a port only when every one of them is
SNI-routed, which config validation enforces so routing can never depend on
compile order. Independently, a stream host may reference access lists: only the
IP/CIDR and geo dimensions are evaluated (basic auth has no challenge/response on
a raw stream and is rejected at validation), and the gate runs **before the
backend dial** — for UDP, once per session — so a denied client never causes an
upstream socket to exist.

**Routing.** An HTTP(S) request is dispatched by `Host` to its compiled handler:
**proxy hosts** run the middleware chain to a reverse proxy; **redirect hosts**
return the configured 3xx to their target (scheme/status/path-preservation per
config); **parked hosts** return a fixed status (default 404). An unknown host →
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
request → rate-limit → access-list → bouncer → auth → guard → headers → rewrite → reverse proxy → upstream
```

Rate limiting is outermost; path rewrite is innermost (closest to the backend).
The access-list sits ahead of auth, so an IP the list would deny is dropped
before any auth work runs (no forward-auth subrequest to the IdP, no OIDC
redirect).

The **bouncer** middleware (`internal/dataplane/bouncer.go`) is a WAF/CrowdSec
**deny hook, not a bundled WAF**: gpm ships no rules, no signatures and no
detection engine, it asks an operator-run bouncer whether the client IP is
currently banned and acts on that verdict. The `crowdsec` provider speaks the
LAPI bouncer flow (`GET /v1/decisions?ip=` with `X-Api-Key`; `ban` and `captcha`
both deny, since gpm has no captcha flow to hand the client and serving anyway
would silently downgrade the operator's decision); the `http` provider is a
generic `2xx`=allow / `403`=deny endpoint any custom bouncer can implement.
Optional `stream` mode pulls the whole decision set once and then deltas on the
cache interval, so the hot path is a local IP/CIDR lookup with no per-request
LAPI call — refreshes run in the background off a request that finds the set
stale (rather than a per-chain ticker, which would leak one poller per config
reload), and a failed refresh keeps serving the previous set. Verdicts are
cached per client IP with a bounded LRU; a verdict derived from an *error* is
capped at 5s so an outage cannot pin a long TTL of guesses. `onError` decides
what an unanswerable lookup means and defaults to **fail-open**: an unreachable
threat feed must not take the site down, which is the opposite of the right
default for auth. It sits between the access list and auth deliberately — an
explicit operator allow-list (`allowFrom`, or the access list itself) still wins
outright over an external feed, and a banned IP never reaches the IdP. Denials
go through the shared per-host denial counter.

**A reference that resolves to nothing fails the host closed.** If a host (or one
of its locations) names a middleware or access list that does not exist in the
compiled registry, the chain is replaced by a handler answering `503` and the
mismatch is logged at `error`. `Config.Validate` already rejects dangling
references and is the primary guard, but it must not be the *only* thing between
a typo and an unauthenticated route: skipping an unresolvable access list would
turn a restricted host into an open one, which is the opposite of what the
reference was written for. The blast radius is deliberately one host - a config
that cannot pass validation anyway must not take unrelated hosts down as a side
effect of this defence in depth.

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
- **Least privilege for automation.** API tokens are scope-limited, not
  role-limited: a CI token can hold `proxy-hosts:write` without being able to read
  the whole config, mint another token, or restore an archive. Escalation paths are
  closed deliberately — token management, **writing settings**, `backup`,
  `restore`, whole-config `revert` and `/debug/pprof/` are `admin`-scope only, and
  a client-supplied `tokenHash` is discarded so nobody can install a digest whose
  preimage only they know. Writing settings counts as admin because a settings
  write can aim DNS sync or a webhook at an attacker-controlled URL with a
  `${ENV:...}` placeholder as its credential — and the write itself triggers the
  delivery that resolves and sends it — as well as rewrite `adminAuth`.
- **The stored digest never leaves the process.** `APIToken.TokenHash` is
  `json:"-"`, so no endpoint returns it (not `GET /api-tokens`, not the config
  dump); only the YAML at rest carries it. The backup archive *is* that raw YAML,
  which is why downloading it is `admin`-scope rather than `*:read`.
- **Rotation means revocation.** Reverting an `APIToken` — scoped or whole-tree —
  would restore an older `tokenHash` and silently revive a secret the operator
  rotated away. `Store.RevertObject` refuses the kind outright
  (`ErrNotRevertible`), and `Store.Revert` snapshots the `api-tokens` directory
  before the tree restore and writes it back over the result, so a whole-config
  revert neither revives a rotated digest nor resurrects a deleted token.
- **Ownership-gated external writes.** The DNS reconciler only ever deletes
  records it *recorded creating*, in the git-backed ownership ledger
  (`config/dns-ledger.yaml`); Cloudflare additionally requires its
  `managed-by:gpm` comment. Nothing is inferred from a record's target, because
  inferring it deleted 19 of an operator's hand-written CNAMEs on 2026-08-01. A
  name owned by a foreign record is left alone rather than replaced, so a
  misconfiguration cannot take an operator's zone
  apart. Ingress discovery applies the same rule inward: only proxy hosts carrying
  its managed-by label are written or deleted — and only when neither the derived
  name nor any of its domains is already claimed by a host it does not own — and it
  deletes at all only on the strength of a complete, successful cluster list
  (freeze on error).
- **Discovery is opt-in and read-only.** gpm never writes to the cluster — the
  shipped RBAC grants `list` on `ingresses` and nothing else — and
  an `Ingress` without `gpm.rake.pro/managed: "true"` is invisible. There is no
  namespace-sweep mode, and no field of an Ingress can supply an upstream, a
  certificate, a middleware or an access list — only the *name* of an
  operator-authored discovery profile, which is why there is no annotation that
  names those objects directly.

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
