# Design: Kubernetes Ingress discovery (DNS sync phase 2)

Status: **implemented.** This document settled the design before the code
landed; it is kept as the rationale record for
`internal/k8s`, `settings.ingressDiscovery`, and the
`gpm.rake.pro/*` annotation contract.

Phase 1 (Pi-hole + Cloudflare CNAME reconciliation for opted-in proxy hosts) is
shipped: a proxy host carrying a `dns` policy gets its domains published to the
LAN resolver and/or the public zone. The manual step that remains is the one
this document removes: **a cluster service still has to be hand-entered as a
proxy host before its DNS follows.** Phase 2 discovers annotated cluster
`Ingress` objects and reconciles them into managed proxy hosts, which then feed
the phase-1 reconciler unchanged.

It resolves the three open questions BACKLOG.md left explicitly open
(certificate strategy, commit granularity, behaviour when the API server is
unreachable) plus the mechanism questions that fall out of them (poll vs watch,
Ingress→ProxyHost field mapping, derived-object naming, and what happens when an
Ingress loses its annotation).

## Context: where gpm actually runs

The deciding fact for almost every decision below is the deployment topology:

```
   internet ─▶ [ edge-host ]  ── LAN ──▶ [ k8s cluster ]
               gpm (edge)             ingress controller (LB address)
               TLS termination        Services / Pods
```

**gpm runs OFF-cluster**, on the edge host, and reaches the cluster over the
LAN. It is not a pod, it has no kube-proxy, no CoreDNS, and no cluster network
membership. Two consequences, both load-bearing:

1. `http://<svc>.<ns>.svc.cluster.local:<port>` — the obvious "backend service"
   mapping — is **not resolvable and not routable** from gpm. Any design that
   derives the upstream from `spec.rules[].http.paths[].backend.service` is
   wrong for this deployment.
2. The cluster connection is an ordinary outbound HTTPS client call to the API
   server with a bearer token, so the "in-cluster config" path
   (`KUBERNETES_SERVICE_HOST` + projected ServiceAccount files) is the
   *secondary* mode, and explicit config (API URL + token file + CA file) is the
   *primary* one. Both are supported; the explicit one is the real deployment.

## Goals

- Discover cluster `Ingress` objects that **opt in** by annotation, and keep a
  set of gpm-managed proxy hosts in step with them.
- Feed the existing DNS sync. Discovery sets the `dns` policy on the derived
  hosts and then does nothing else about DNS; phase 1 publishes the records.
  **One DNS code path, not two.**
- Never overwrite, and never delete, anything an operator authored.
- Add **no new dependency**: `net/http` + `encoding/json` against the Kubernetes
  REST API, no `client-go`.
- Read-only against the cluster. gpm must never hold a token that can write.

## Non-goals

- **No `client-go`.** Its transitive tree dwarfs this project's entire direct
  dependency set, which is the thing the project exists to avoid.
- **No watch/informer cache.** See "Poll vs watch" below.
- **No namespace sweeps.** An Ingress without the opt-in annotation is invisible
  to gpm; there is no "manage everything in namespace X" mode, ever.
- **No writes to the cluster.** No status updates, no finalizers, no events. The
  RBAC we ship cannot express a write even if the code tried.
- **No Gateway API / IngressClass-driven discovery.** Annotation opt-in only.
  Adding a second opt-in mechanism doubles the blast radius of a mistake.
- **No per-Ingress upstream override.** The upstream comes from the template
  (see "Field mapping"); an Ingress cannot aim gpm at an arbitrary address.
- **No discovery of Services, EndpointSlices, or CRDs.** Ingress only.

---

## 1. Open question (a): certificates for discovered hosts

### Options

- **A. Single wildcard certificate reference, supplied by the template.** Every
  derived host gets `tls.certificateRef: <name>` naming a `Certificate` object
  the operator already maintains. Discovery never creates a `Certificate` and
  never triggers ACME.
- **B. Per-host ACME.** Each derived host gets its own `Certificate` object,
  issued on demand.
- **C. Hybrid** — wildcard by default, per-host when the derived domain falls
  outside the wildcard's coverage.

### Decision: **A — a single certificate reference from the template.**

Four reasons, in order of weight:

1. **The homelab already runs a wildcard-capable DNS-01 issuer.** A wildcard
   `*.example.com` certificate already covers every name a cluster Ingress will
   realistically publish. Option B would issue dozens of certificates to cover
   what one already covers.
2. **Rate limits.** Let's Encrypt caps new orders per registered domain per
   week. Discovery is a *loop* driven by cluster state: a flapping Ingress, a
   reconcile bug, or a bad template could mint orders on every poll. Coupling an
   autodiscovered, externally-driven object set to an issuance path with a
   hard weekly cap is how you lock yourself out of issuance for the entire zone.
   A wildcard ref has no per-host issuance at all, so the failure mode does not
   exist.
3. **Surprise public issuance.** Every issued certificate is published to
   Certificate Transparency logs, permanently. Auto-issuing per discovered
   Ingress means an internal service name becomes a public, permanent record the
   moment someone annotates an Ingress — a disclosure decision made by whoever
   edits a manifest, not by the gpm operator. A wildcard leaks nothing beyond
   the zone apex, which is already public.
4. **Ownership discipline.** Option B forces discovery to create and delete
   `Certificate` objects, i.e. to own a second object kind with its own
   private-key lifecycle. Keeping discovery to exactly one derived kind
   (`ProxyHost`) keeps "what does the reconciler own?" answerable in one
   sentence.

Consequences, stated plainly:

- `ingressDiscovery.template.tls.certificateRef` is **required** when discovery
  is enabled, and is validated for name shape at settings-write time and for
  existence by the store's referential-integrity check on the first write.
- A discovered host whose domain is **not** covered by the referenced
  certificate will serve the wrong name. That is contained by the
  `allowedDomainSuffixes` guard (section 5): the suffix list is normally exactly
  the wildcard's zone, so an out-of-zone name is refused at derivation rather
  than served with a mismatched certificate.
- Option C is deliberately deferred, not designed out: a future
  `perHostACME: true` could add per-host `Certificate` creation for names
  outside the wildcard. It would need its own rate-limit budget and a CT
  disclosure note in the UI, and there is no demand for it today.

---

## 2. Open question (b): commit granularity

### Options

- **A. One commit per reconcile.** All creates, updates and deletes from one
  poll land as a single revision.
- **B. One commit per object.** Each derived host is its own `Store.Save` /
  `Store.Delete`, i.e. its own commit.

### Decision: **A — one commit per reconcile.**

- **The store's existing `Save` semantics make B expensive.** Every `Save` is
  load-whole-config → validate-whole-graph → write file → `git add -A` +
  `git commit` (two process spawns). A first reconcile against a cluster with 30
  annotated Ingresses would be 30 full-graph validations and 60 git processes;
  worse, each one fires the daemon's reload + webhook + DNS-trigger path. `A`
  costs one validation, one commit, one reload.
- **Reconcile bursts.** Discovery is a *poll*, and cluster changes arrive in
  clumps (a Helm release adds four Ingresses at once; a namespace teardown
  removes six). Per-object commits turn each clump into a commit storm whose
  intermediate revisions are states that never existed as a whole and that
  nobody wants to revert to.
- **History/revert UX.** "Ingress discovery: +2 ~1 -1" is a revision an operator
  can read, revert, and reason about. Per-object commits bury the operator's own
  hand-authored changes under machine noise in `GET /history` and in the History
  view, and make whole-tree revert land in the middle of a reconcile.
- **Atomicity.** A partially-applied reconcile (three hosts written, then a
  validation failure on the fourth) is a state the config should never be
  committed in. One batch write is all-or-nothing.

This required one new store primitive: `Store.ApplyBatch(upserts, deletes, …)`.
`SaveBatch` already commits many writes atomically but cannot delete;
`ApplyBatch` is `SaveBatch` plus removals, validating the merged graph once
(including the dangling-reference check that `Delete` performs) before touching
the working tree. A no-op reconcile writes nothing and commits nothing — the
common case must not produce empty revisions.

**Commit message:** `Ingress discovery: reconcile (+N ~M -K)`, authored by
`ingress-discovery <gpm@localhost>` so machine-authored revisions are trivially
distinguishable from operator ones in `git log`.

---

## 3. Open question (c): API server unreachable, or a partial/erroring list

### Decision: **freeze.** A managed host is deleted under exactly one condition.

> A gpm-managed host is deleted **only** when a reconcile obtained a
> **complete, successful, fully-paginated** list of annotated Ingresses, and the
> host's derived name is absent from the desired set computed from that list.

Everything else keeps the current managed hosts exactly as they are. Concretely,
each of the following aborts the run **before any write** — no creates, no
updates, no deletes:

| Condition | Outcome |
|---|---|
| Token file missing/unreadable | abort, status error |
| CA file missing/unparseable | abort at client construction, status error |
| TCP/TLS/dial failure, timeout, context cancellation | abort, status error |
| `401` / `403` (token expired, RBAC changed) | abort, status error, cached token dropped |
| Any non-`200` status | abort, status error |
| JSON decode failure on any page | abort, status error |
| A pagination page fails after earlier pages succeeded | abort, status error — the partial accumulation is **discarded**, never returned |
| Pagination exceeds the page/item safety cap | abort, status error (never a silent truncation) |

The last two are the ones that matter: a partial list is exactly the input that
would make freeze-vs-delete go wrong, so the client never returns partial data
with a nil error. `ListIngresses` returns `([]Ingress, error)` and the
accumulator is only returned when the `continue` token has been followed to
exhaustion.

### Making "empty list" and "error" impossible to confuse

The confusion this guards against is real and catastrophic: an API server that
returns `{"items": []}` for an authorization failure, or a client that returns
`nil, nil` on a transport error, would delete **every** managed host on the next
reconcile.

Three structural properties keep the two apart:

1. **They are different return shapes, not different values of one.** Success is
   `(items, nil)` where `items` may legitimately be empty. Failure is
   `(nil, err)` and the reconciler's very first action after listing is
   `if err != nil { record; return }` — the desired set is never computed on an
   error path, so there is no code path where an error can be *read as* an empty
   list.
2. **The reconciler never infers emptiness from a status code.** Only HTTP 200
   with a decodable `IngressList` body produces items. There is no "treat 404 as
   empty" convenience.
3. **An empty successful list is a legitimate delete-all, and says so out loud.**
   If every annotation is genuinely removed, deleting every managed host is the
   correct answer. When a reconcile deletes managed hosts because the list came
   back empty, each removal is logged at WARN naming the host, and the status
   payload carries the deletion in `hosts[]` with `action: "deleted"`. An
   operator who sees a delete-all in the log and did not expect one has the
   commit to revert and the reason recorded.

Additionally, **per-object errors freeze per-object.** If an annotated Ingress
is present but cannot be derived (invalid hostname, name too long, no usable
rules), it is recorded as `action: "skipped"` with a reason and a WARN log — and
its derived name is added to the **protected** set, so the previously-derived
host is left exactly as-is rather than deleted. Deletion follows only from
*absence* from a healthy list, never from a parse failure. One bad manifest edit
must not take a host offline.

Status distinguishes `lastRun` from `lastSuccess`, so a UI that shows "last run
2 minutes ago" cannot hide "last successful run 6 hours ago".

---

## 4. Poll vs watch

### Decision: **poll, with a configurable interval (default 60s, floor 15s).**

`?watch=1` is superficially attractive and materially more code to get right
without `client-go`:

- A watch is a long-lived chunked response that must be **restarted** on every
  idle timeout, LB reset, NAT expiry and API-server rollout. gpm reaches the
  cluster over the LAN from a different host, so all four happen.
- Correct watch semantics require tracking `resourceVersion`, handling
  `410 Gone` with a full re-list, and a backoff/jitter loop — i.e.
  reimplementing a reflector. Getting it subtly wrong produces the exact failure
  this design is most afraid of: a stale view that looks healthy.
- Discovery is **not latency-critical**. A new cluster service appearing in gpm
  60 seconds later is fine; the DNS TTLs downstream are of the same order.
- A full-state poll is **the same shape as the phase-1 DNS reconciler** —
  recompute the desired set from scratch, compare, converge — which means it
  self-heals from any missed event by construction. A watch's correctness
  depends on never missing an event; a poll's does not depend on anything.
- Cost is negligible: one `GET` of a small list every 60s.

The interval is `settings.ingressDiscovery.pollInterval`, a Go duration string
(`"60s"`, `"5m"`), matching the `rateLimit.window` convention already in the
schema. Values below 15s are refused at validation: the reconcile takes the
store write lock, and a hot loop against the config repo is a self-inflicted
denial of service.

The poll loop reads settings **live on every iteration** (the webhook
dispatcher's and DNS syncer's pattern), so enabling, disabling or re-pointing
discovery takes effect without a restart.

---

## 5. Field mapping: Ingress → ProxyHost

| ProxyHost field | Source | Notes |
|---|---|---|
| `name` | derived: `ing-<name>.<namespace>` | see "Naming" below |
| `labels["gpm.rake.pro/managed-by"]` | constant `ingress-discovery` | the ownership marker; nothing else is ever touched |
| `domains` | `spec.rules[].host` | lowercased, trailing dot stripped, de-duplicated, **sorted**, each one validated (below) |
| `upstream` | **template** | the cluster ingress controller's address — *not* the Ingress backend |
| `tls` | **template** | certificate ref, forceSSL, HTTP/2, HSTS, minTLSVersion, clientAuth |
| `middlewares` | **template** | |
| `accessLists` | **template** | |
| `websocketsUpgrade` | **template** | |
| `dns` | template default, overridden by the two DNS annotations | `nil` when both flags end up false |
| everything else | zero | |

### The upstream is the ingress controller, not the Service

This is the correction that makes the feature work at all. Because gpm is
off-cluster (see "Context"), `spec.rules[].http.paths[].backend.service` names
a Service that resolves only inside the cluster. gpm therefore **ignores the
backend entirely** and takes its upstream from
`settings.ingressDiscovery.template.upstream` — the **cluster ingress
controller's LB address** (e.g. `http://10.0.0.40:80`). Routing to the right
workload is then the controller's job, done exactly the way it already is for
every other client: **by vhost**.

For that to work the original `Host` must survive the extra hop, and it does:
the data plane's reverse-proxy rewrite sets `pr.Out.Host = pr.In.Host`
unconditionally (`internal/dataplane/proxy.go`), so a request that arrived as
`app.example.com` reaches the controller with `Host: app.example.com` and
matches the same Ingress rule the cluster already has. gpm terminates TLS at the
edge; the controller sees a plain proxied request with the browser-facing host.

**Scheme choice, and the SNI caveat.** With `scheme: https` the Go transport
derives SNI and certificate verification from the **URL host** — the LB address
— not from the forwarded `Host` header, and the data plane sets no custom
`TLSClientConfig`. Pointing an `https` upstream at a bare IP therefore fails
verification unless the controller presents a certificate with that IP in its
SANs. The two supported shapes are:

- **`scheme: http` to the controller's plain port** (recommended): the edge→LB
  hop is a trusted LAN path, TLS is terminated once at gpm, and there is no SNI
  question at all. This is the homelab configuration.
- **`scheme: https` to a *hostname*** that resolves to the LB and is covered by
  the controller's certificate. Then SNI is that hostname, verification passes,
  and the vhost routing still keys off the forwarded `Host` header.

Documented in `docs/configuration.md` so nobody discovers it via a 502.

### `spec.tls` is read but never authoritative

The typed struct decodes `spec.tls[].hosts` because it is part of the object we
consume, but it does **not** select a certificate (decision (a)) and does not
contribute domains. Its only use is a DEBUG log when an Ingress declares TLS
hosts that its rules do not mention — a manifest smell worth surfacing and
nothing more. Deriving TLS material from cluster-side manifests would put the
edge's certificate selection in the hands of whoever can edit an Ingress.

### Hostname validation (untrusted input)

Every string that arrives from the API server is untrusted. Hosts are validated
before they can become a domain:

- lowercased, surrounding whitespace and one trailing dot stripped;
- must match a strict LDH hostname shape with at least two labels
  (`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`),
  total length ≤ 253 — this alone rejects `*` wildcards, empty labels, spaces,
  URL/scheme fragments, path separators, `..`, and control characters;
- **must fall within `allowedDomainSuffixes`** (`name == suffix` or
  `name` ends in `.suffix`). The list is **required and non-empty** when
  discovery is enabled: an unrestricted discovery feature is one annotated
  manifest away from claiming `login.microsoftonline.com` at the edge.

An Ingress with no valid host is skipped (and protected from deletion, section
3). An Ingress with a mix keeps the valid ones and logs the rejects.

Wildcards are rejected deliberately, not incidentally: the phase-1 DNS
reconciler already skips wildcard domains (it cannot express one as a managed
CNAME), so a discovered wildcard host would silently publish nothing.

### Nothing else is inherited

No annotation, label, or field of an Ingress can contribute a middleware
reference, an access-list reference, an upstream, a certificate, a path, or a
header. The operator's profile set is the **only** source for everything
security-relevant. This is the core containment property: a cluster user who can
edit an Ingress can (a) publish a hostname within the allowed suffixes, (b)
toggle two DNS booleans, and (c) **name** one of the operator's profiles — and
can never weaken the auth/access chain the operator configured, nor point gpm at
an address of their choosing.

---

## 5a. Named profiles: one template does not fit a real fleet

### The problem

The original design derived every host from one `template`, which supplies the
middleware and access-list chain. A real fleet is heterogeneous. Ours looks like
this:

| host | middlewares | access lists |
|---|---|---|
| `paste` | `rate-limit` | *(none — deliberately public)* |
| `notes` | `joplin-login-lan` | `home-vpn` |
| `wiki` | `rate-limit` | `home-vpn` |
| `cloud` | `nextcloud-login-lan` | *(none — public)* |
| `plex` | *(none)* | *(none — public)* |
| `radarr` / `jackett` | `sso` / `sso-lan` | `home-vpn` |
| `alertmanager` | `sso-lan` | `home-vpn` |
| most others | *(none)* | `home-vpn` |

Only the last group matches the single template. Adopting anything else into
discovery would either **drop** its `sso` / `rate-limit` / login middleware, or
**impose** `home-vpn` on a host that is public on purpose. Both are
security-relevant regressions, and both are silent — the host keeps serving, just
with a chain nobody chose. So in practice discovery could only ever adopt the
uniform tail of the fleet, which is not where the operational cost is.

### Options

- **A. Per-Ingress fields.** Annotations that name middlewares, access lists,
  upstreams, certificates. Maximum flexibility.
- **B. Operator-defined named profiles, selected by annotation.** The operator
  writes N chains in settings; an Ingress names one.
- **C. Selector-based mapping.** Operator writes rules in settings
  (`namespace == x AND label y ⇒ profile p`). Nothing on the Ingress at all.

### Decision: **B — named profiles, selected by name only.**

#### Threat model: the Ingress author is untrusted

This is the whole design constraint, so state it plainly. A cluster tenant may be
able to create or edit an `Ingress` — that is normal RBAC in a shared cluster,
and it is *why* discovery is opt-in and suffix-bounded in the first place. gpm
sits at the edge in front of everything. Therefore an Ingress author must never
be able to:

- invent a chain that no operator ever wrote down,
- name an arbitrary middleware, access list, certificate or upstream,
- produce a host **weaker** than something the operator explicitly sanctioned.

Option A fails all three by construction. `gpm.rake.pro/access-lists: ""` on your
own namespace's Ingress is a self-service removal of `home-vpn` from a hostname
at the edge; `gpm.rake.pro/upstream: http://10.0.0.99:80` aims the edge at an
address of the tenant's choosing. There is no validation that fixes this, because
the vocabulary itself is the privilege — any middleware the operator has defined
is by definition one the operator considered acceptable *somewhere*, not
everywhere.

Option B moves the entire vocabulary into settings, which lives in the config git
repo and is written by the operator. The annotation carries **a name and nothing
else**, and every name it can carry maps to a chain the operator authored in
full. The tenant's power reduces to *choosing among sanctioned configurations* —
still a real capability (a tenant can pick the most permissive profile you
defined), but bounded by a set you control and can audit, rather than unbounded.
The mitigation for the residual is that you only define profiles you are willing
for any annotating tenant to select; if that is too coarse, option C is the
escalation path.

Option C is strictly stronger (nothing on the Ingress selects anything) but
inverts the ergonomics: the operator has to maintain a rule table that mirrors
the cluster's shape, and every new service needs a settings commit before it can
be published — which is most of the toil discovery exists to remove. Deferred,
not rejected: profiles are the substrate a selector layer would sit on.

#### Resolution rules

`gpm.rake.pro/profile` is resolved before anything else about the Ingress is
derived:

| annotation | result |
|---|---|
| absent | the default `template` block |
| present but empty or whitespace-only | treated as absent → the default `template` |
| exact match on a defined profile (surrounding whitespace trimmed) | that profile, **verbatim** |
| anything else | the Ingress is **SKIPPED**, with the requested name in the status reason and a `warn` log |

The two rules that matter:

**An unknown profile is a skip, never a fall back to the default.** Falling back
is precisely the silent downgrade the feature exists to prevent — an Ingress that
asked for `public-ratelimited` and received the default's `home-vpn`, or worse,
one that asked for `sso-internal`, hit a typo, and got a chain with no `sso` in
it. A skip is loud, visible in `GET /ingress-discovery/status`, and leaves
whatever is on disk untouched. For the same reason a skipped Ingress **protects**
its existing derived host from deletion: a typo in an annotation must not take a
service offline either.

**A profile is applied verbatim, never merged with the default.** A merge would
mean the default's access list leaks onto a profile that is public on purpose —
the exact failure the feature is meant to eliminate. Each profile is a complete,
independently-valid chain.

There is no prefix matching, no case folding and no nearest-neighbour guess:
those are the ways a junk or hostile annotation value turns into a chain nobody
chose. The name `template` is reserved for the default block, so the profile
reported in status is never ambiguous.

#### Validation

Every profile validates **exactly** as the template does — `certificateRef`
required, `upstream` XOR `upstreamGroupRef`, `ValidateName` on every middleware
and access-list reference, plus `ValidateName` on the profile's own key. It runs
at `Settings.Validate`, so an invalid profile fails the settings **write**, where
an operator sees it, rather than surfacing hours later as a skipped host in a
reconcile status nobody is watching.

#### Backwards compatibility

`template` stays exactly what it was and becomes the default profile. A config
with no `profiles` key, and Ingresses with no `profile` annotation, behaves
identically to before — covered by a regression test.

### Naming of derived objects

`ing-<ingressName>.<namespace>`, e.g. `ing-grafana.monitoring`.

- The `ing-` prefix makes machine-authored objects obvious in the UI list and on
  disk (`config/proxy-hosts/ing-grafana.monitoring.yaml`).
- The separator is `.` rather than `-` because Kubernetes namespaces are
  DNS-1123 **labels** (no dots) while names may contain `-`. `ing-a.b` is
  therefore unambiguous, whereas `ing-a-b` could be `(a, b)` or `(a-b, …)`.
  Dots are legal in `model.ValidateName`.
- Names that would exceed the 253-character limit, or that otherwise fail
  `ValidateName`, are skipped with a reason (and protected from deletion).
- The name is **stable**: it depends only on the Ingress identity, so renaming
  a host's domains updates in place rather than churning create/delete pairs.

### When an Ingress loses the annotation, or is deleted

Both are the same event to a full-state reconciler: the Ingress is no longer in
the desired set, so its derived host is **deleted** on the next successful
reconcile (one revision, revertible, logged at INFO with the host name). There
is no tombstone and no grace period — the annotation is the opt-in, and removing
it means "gpm should stop serving this", which is exactly what a delete does.

Two guards remain in force:

- Only objects carrying `labels["gpm.rake.pro/managed-by"] =
  "ingress-discovery"` are ever deleted. An operator who wants to keep a
  discovered host permanently removes that label; discovery then treats the
  object as operator-authored, refuses to touch it, and (because the name is
  taken) skips the corresponding Ingress with a warning — the same
  skip-and-warn ownership rule `dnssync/pihole.go` and `dnssync/cloudflare.go`
  already use for a record they do not own.
- A delete that would leave a dangling reference is refused by the store's
  existing referential-integrity check, aborting the batch rather than
  committing a broken graph.

---

## 6. Package layout

`internal/k8s` holds both the REST client (`client.go`) and the reconciler
(`discovery.go`).

The alternative was a separate `internal/ingressdisc` importing `internal/k8s`.
Rejected: the client exists solely to serve this reconciler, the split would
force the `Ingress` types and every list option to be exported across a package
boundary for exactly one consumer, and the two halves share the same vocabulary
(annotations, ownership, validation). One package, one doc comment stating the
two properties (full-state, ownership-gated) — mirroring how `internal/dnssync`
keeps its two backends beside its syncer.

Coupling is kept to injected function values, as `dnssync` does: the reconciler
takes a `load` func and an `apply` func and imports neither `store` nor `api`.

---

## 7. Security posture

| Requirement | Mechanism |
|---|---|
| gpm never writes to the cluster | shipped ClusterRole grants `list` on `ingresses` only (the reconciler never gets by name and never watches); no write verb exists to call |
| Opt-in only | `gpm.rake.pro/managed: "true"` exactly; absent/any other value = invisible. No namespace sweep mode exists |
| No privilege inheritance from cluster manifests | every security-relevant field comes from an operator-written profile; the Ingress contributes hostnames (validated, suffix-restricted), two booleans, and the *name* of one profile — never a chain, a middleware, an access list, a certificate or an upstream (§5a) |
| Untrusted profile selection | exact-match only against `settings.ingressDiscovery.profiles`; an undefined name **skips** the Ingress rather than falling back to the default (which would be a silent downgrade), and a profile is applied verbatim rather than merged with the default |
| Untrusted strings | strict hostname validation + allowed-suffix gate + `model.ValidateName` on the derived name; upstream is never built from Ingress input |
| Credential handling | bearer token read from a file, re-read on a TTL (projected SA tokens rotate), never logged, never returned by any endpoint, never committed to git |
| Transport | TLS with the supplied CA bundle, redirects never followed, bounded response reads, bounded timeouts — the same hardening as `internal/dnssync` |
| Ownership | writes and deletes restricted to objects carrying the managed-by label, re-verified under the store lock at write time (the plan predates the cluster list); collision on the derived NAME *or* on any DOMAIN with a host discovery does not own = skip + warn, backstopped by a duplicate-domain check in `Config.Validate` |
| API surface | `ingress-discovery:read` / `ingress-discovery:write` scopes; status carries no token, no CA, no cluster addresses beyond what settings already expose |

The token file is a path, not a `Secret` placeholder, for two reasons: projected
ServiceAccount tokens **rotate on disk** (an env-var snapshot would go stale and
start failing at ~1h), and a file path keeps the credential out of the config
repo entirely rather than relying on placeholder discipline.

---

## 8. Alternatives considered

- **`client-go` + informers.** The industry-standard answer, and the one this
  project exists to avoid: it would multiply the dependency tree by an order of
  magnitude for one `GET` of one resource type. Rejected on the same grounds as
  etcd in the HA design.
- **`external-dns` for the DNS half.** Would solve records but not proxy hosts,
  adds a second component with its own credentials, and leaves gpm's config
  ignorant of what exists — the opposite of the git-backed, whole-config model.
- **A gpm agent running *inside* the cluster** that pushes to the gpm API.
  Inverts the trust direction (the cluster gets a gpm write credential), adds a
  deployable, and needs its own auth story. Pull from the edge is strictly less
  privilege.
- **Deriving upstreams from Services / EndpointSlices** (per-pod or per-service
  IP). Requires cluster network reachability gpm does not have, and would make
  gpm re-implement kube-proxy's load balancing. The ingress controller already
  does this correctly.
- **Opt-out (manage everything unless annotated otherwise).** One forgotten
  annotation publishes an internal service at the edge. Opt-in is the only
  defensible default.
- **`IngressClass`-based selection instead of an annotation.** Ties gpm's
  discovery to a cluster-wide routing concept that already means something else,
  and an Ingress can have exactly one class — an Ingress served by the cluster
  controller *and* mirrored by gpm is the normal case here.
- **Storing discovered hosts outside git** (an in-memory overlay). Would keep
  history clean at the cost of the property the whole system is built on: what
  gpm serves is what is committed. Rejected.

---

## 9. Sequencing

1. **Settings schema + validation** (`model.IngressDiscoverySettings`,
   `IngressHostTemplate`) — no behaviour, safe to land alone.
2. **REST client** (`internal/k8s/client.go`) with in-cluster and explicit
   config, token TTL re-read, pagination, hardened transport. Hermetic
   `httptest` coverage including the partial/erroring-list cases.
3. **`Store.ApplyBatch`** — one commit for upserts + deletes.
4. **Reconciler** (`internal/k8s/discovery.go`): derive, compare, apply,
   status; ownership and freeze rules.
5. **Daemon wiring** — poll loop, reload, and the existing `dnsSyncer.Trigger()`
   after a successful write (no second DNS path).
6. **API surface** — status/reconcile endpoints, scopes, capability probe.
7. **UI** — settings block + status panel, gated on the capability.
8. **RBAC manifest + docs** — `deploy/k8s-rbac.yaml`, configuration/deployment
   docs, token extraction recipe.

**Effort:** M. No new dependency; no data-plane change; one new store primitive.

## 10. Deferred

- Per-host ACME for names outside the wildcard (option C in section 1).
- Watch-based discovery, if a sub-minute convergence requirement ever appears.
- Discovery of `Gateway`/`HTTPRoute` (Gateway API) as a second source.
- A dry-run mode (`GET /ingress-discovery/plan`) that reports the diff without
  writing. Cheap to add on top of the reconciler's existing plan/apply split if
  operators ask for it.
- **Operator-side profile selection** (option C in §5a): mapping rules in
  settings (`namespace`/label ⇒ profile) so the Ingress selects nothing at all.
  Strictly stronger than the annotation, at the cost of a settings commit per new
  service. Profiles are the substrate it would sit on.
- **Per-profile `allowedDomainSuffixes`**, so a permissive profile could be
  restricted to a subset of the published namespace. The current suffix list is
  global.
