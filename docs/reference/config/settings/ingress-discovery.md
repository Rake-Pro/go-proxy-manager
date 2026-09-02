# Settings: Kubernetes Ingress discovery

Derive managed proxy hosts from annotated cluster `Ingress` objects, using
operator-authored templates and named profiles.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-ingress-discovery"></span> `ingressDiscovery` | IngressDiscoverySettings | Optional Kubernetes Ingress discovery (below). |

Discovers annotated Kubernetes `Ingress` objects and reconciles them into
template-derived proxy hosts, which then feed the DNS sync above. Disabled (the
default) means the subsystem is inert and never contacts anything. The full
rationale is in [Design: Kubernetes Ingress discovery (DNS sync phase 2)](../../../design/ingress-discovery.md).

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-ingress-discovery-enabled"></span> `enabled` | bool | Turn discovery on. Everything below is validated only when this is true. |
| <span id="settings-ingress-discovery-api-url"></span> `apiURL` | string | Kubernetes API base URL, **absolute https**. Empty uses the in-cluster endpoint (`KUBERNETES_SERVICE_HOST`/`_PORT`). |
| <span id="settings-ingress-discovery-token-file"></span> `tokenFile` | string | Absolute path to the read-only ServiceAccount bearer token. Empty uses the projected in-cluster path. **Re-read from disk periodically** (5 min), so a rotated projected token keeps working. |
| <span id="settings-ingress-discovery-ca-file"></span> `caFile` | string | Absolute path to the PEM bundle that verifies the API server. Empty uses the projected in-cluster CA. There is no skip-verify option. |
| <span id="settings-ingress-discovery-namespace"></span> `namespace` | string | Restrict the list to one namespace. Empty lists cluster-wide (still annotation-gated). |
| <span id="settings-ingress-discovery-label-selector"></span> `labelSelector` | string | Optional server-side label selector. The opt-in annotation is still required - the Kubernetes API cannot select on annotations, so that filter is always client-side. |
| <span id="settings-ingress-discovery-poll-interval"></span> `pollInterval` | duration | Go duration string. Default `1m`, **minimum `15s`** (a reconcile takes the store write lock). |
| <span id="settings-ingress-discovery-allowed-domain-suffixes"></span> `allowedDomainSuffixes` | []string | **Required when enabled.** A discovered hostname must equal one of these or end in `.` + one of them. |
| <span id="settings-ingress-discovery-template-upstream"></span> `template.upstream` | Upstream | Where every derived host forwards: the **cluster ingress controller's** address. Mutually exclusive with `template.upstreamGroupRef`. |
| <span id="settings-ingress-discovery-template-upstream-group-ref"></span> `template.upstreamGroupRef` | string | Names an `upstream-groups` entry instead of a single address. **Prefer this when the ingress controller runs on more than one node** - otherwise every derived host is pinned to one node while hand-written hosts fail over. |
| <span id="settings-ingress-discovery-template-tls"></span> `template.tls` | TLSSettings | Applied verbatim. `certificateRef` is **required** - discovery never issues a certificate, so one operator-maintained (typically wildcard) certificate must already cover the discovered names. It does not *select* the certificate: derived hosts are L7, and the data plane matches by SNI. See [Which certificate a host serves](../certificate.md#which-certificate-a-host-serves). |
| <span id="settings-ingress-discovery-template-robots-no-index"></span> `template.robotsNoIndex` | bool | Applied to every derived host: sends `X-Robots-Tag: noindex, nofollow`, exactly as on a hand-written host. Omit it and derived hosts carry no such header - do **not** reach for a `headers` middleware instead. |
| <span id="settings-ingress-discovery-template-timeouts"></span> `template.timeouts` | HostTimeouts | Upstream `connectSeconds`/`readSeconds` override, applied to every derived host and validated by the **same rules as a proxy host's** (0-3600). Unset means the shared pooled transport. |
| <span id="settings-ingress-discovery-template-middlewares"></span> `template.middlewares` | []string | Applied to every derived host, in order. |
| <span id="settings-ingress-discovery-template-access-lists"></span> `template.accessLists` | []string | Applied to every derived host. |
| <span id="settings-ingress-discovery-template-tags"></span> `template.tags` | []string | Free-form grouping labels applied to every derived host, for filtering in the host list. No data-plane effect. |
| <span id="settings-ingress-discovery-template-strip-response-headers"></span> `template.stripResponseHeaders` | []string | Applied to every derived host: response headers removed from what its upstream sends, exactly as on a hand-written host, and validated by the same rules. Without it a hand-set list on a managed host is reverted on the next reconcile. See [StripResponseHeaders](security-headers.md#strip-response-headers-section). |
| <span id="settings-ingress-discovery-template-default-dns"></span> `template.defaultDNS` | DNSSyncPolicy | The `dns` policy a derived host gets when the corresponding annotation is absent. Each flag is overridden individually by its annotation. |
| <span id="settings-ingress-discovery-template-allowed-domain-suffixes"></span> `template.allowedDomainSuffixes` | []string | Optional. **Narrows** the top-level `allowedDomainSuffixes` for hosts derived from the template. Must be a **subset** of the global list (checked at settings-write time); empty means no narrowing. |
| <span id="settings-ingress-discovery-profiles"></span> `profiles` | map[string]-> same shape as `template` (including its own `allowedDomainSuffixes`) | Additional named chains an Ingress may **select by name** (below). Each key is a profile name (`ValidateName` shape); `template` is reserved for the default block. |
| <span id="settings-ingress-discovery-profile-rules"></span> `profileRules` | []IngressProfileRule | Optional, ordered. Operator-side profile selection - see [below](#operator-side-profile-selection-profilerules). |
| <span id="settings-ingress-discovery-profile-selection"></span> `profileSelection` | `"annotation-or-rules"` \| `"rules-only"` | Empty means `"annotation-or-rules"` (today's behaviour: try `profileRules` first, then the annotation). `"rules-only"` never reads `gpm.rake.pro/profile` at all. |
| <span id="settings-ingress-discovery-annotation-prefix"></span> `annotationPrefix` | string | Replaces `gpm.rake.pro` as the prefix for every annotation below and for the `managed-by`/`disabled-by` labels gpm writes on derived hosts. Empty (the default) keeps every existing deployment's keys unchanged. Must be a DNS-subdomain-shaped prefix: lowercase alphanumerics, `-` and `.`, no leading/trailing dot, no slash, at most 253 characters. See [Changing the annotation prefix](#changing-the-annotation-prefix) below - **changing this does not relabel existing hosts by itself.** |
| <span id="settings-ingress-discovery-annotation-prefix-migrate"></span> `annotationPrefixMigrate` | bool | Opt-in escape hatch for the refusal `annotationPrefix` triggers when existing hosts are still labelled under the old prefix - see below. Does not relabel anything itself; only lifts the refusal so the *next reconcile* can. |

A derived host's `displayName` is always `<namespace>/<name>` (where the Ingress
came from) and is not templatable. `locations` are **deliberately not** a
template field - see [below](#what-a-derived-host-cannot-express).

**`template.tls` and `template.upstream` carry the full ProxyHost shapes.**
Every sub-key below is settable on the template and on any profile, and is
applied verbatim to each derived host:

| Template key | Shape | Sub-keys documented at |
|---|---|---|
| `template.upstream` | `Upstream` | [ProxyHost: Upstream](../proxy-host.md#proxy-host-upstream) - `scheme`, `host`, `port`, `path`, `hostHeader` |
| `template.tls` | `TLSSettings` | [ProxyHost: `tls`](../proxy-host.md#proxy-host-tls) - `certificateRef`, `forceSSL`, `minTLSVersion`, `hsts.*`, `clientAuth.*` |
| `template.tls.hsts` | `HSTS` | [ProxyHost: `tls.hsts`](../proxy-host.md#proxy-host-tls) - `enabled`, `maxAge`, `includeSubdomains`, `preload` |
| `template.tls.clientAuth` | `ClientAuth` | [ProxyHost: `tls.clientAuth`](../proxy-host.md#proxy-host-tls) - `caRef`, `mode`, `identityHeaders.*` |
| `template.timeouts` | `HostTimeouts` | [ProxyHost: `timeouts`](../proxy-host.md#proxy-host-timeouts) - `connectSeconds`, `readSeconds` |
| `template.defaultDNS` | `DNSSyncPolicy` | [ProxyHost: `dns`](../proxy-host.md#proxy-host-dns) - `lanDirect`, `publicCname` |

The same table applies to every entry in `profiles`, which is the identical type.

**Opt-in annotations** (on the `Ingress`, never on gpm's side; prefixed with `annotationPrefix`, default `gpm.rake.pro`):

| Annotation | Value | Meaning |
|------------|-------|---------|
| `gpm.rake.pro/managed` | `"true"` | Opt this Ingress into discovery. Absent or any other value means gpm ignores it entirely. There is no opt-out mode and no namespace sweep. |
| `gpm.rake.pro/profile` | a `profiles` key | Select one of the operator-defined profiles. Absent (or empty) uses the default `template`. An **undefined** name skips the Ingress. |
| `gpm.rake.pro/lan-direct` | `"true"` \| `"false"` | Sets `dns.lanDirect` on the derived host, overriding the resolved profile's `defaultDNS`. |
| `gpm.rake.pro/public-cname` | `"true"` \| `"false"` | Sets `dns.publicCname` on the derived host. |

## Changing the annotation prefix

Ownership of a derived proxy host is recognised **only under the currently
configured prefix**: the `managed-by` label discovery stamps on every host it
owns is `<annotationPrefix>/managed-by: ingress-discovery`, and a host carrying
that pair under some *other* prefix looks exactly like a hand-written host to a
fresh reconcile - it is left alone (never deleted, never overwritten), but it
also stops being treated as discovery-managed.

Because of that, **a settings write that changes `annotationPrefix` is refused**
if any existing proxy host is still labelled `managed-by` under the *old*
prefix, naming how many hosts would be affected. To go ahead anyway, set
`annotationPrefixMigrate: true` in the same (or a follow-up) write. That flag
does not touch anything itself - it only lifts the refusal. The relabel happens
in the **next reconcile**: every such host is picked up as an ordinary update
(or, if its `Ingress` is meanwhile unresolvable, as the usual fail-closed
disable) and rewritten with the new prefix's `managed-by`/`disabled-by` labels,
in that reconcile's single commit, exactly like any other change discovery
makes. Once that reconcile has run, `annotationPrefixMigrate` can be turned back
off - it is not "sticky" state discovery depends on afterwards.

Also update the `annotationPrefix` your cluster `Ingress` manifests use for
their own annotations (`.../managed`, `.../profile`, `.../lan-direct`,
`.../public-cname`) at the same time: an `Ingress` still carrying the old
prefix's opt-in annotation becomes invisible to discovery the moment the
setting changes, the same as if it had never opted in.

## Discovery profiles

One template only fits a uniform fleet. A real one is not: some hosts are
deliberately public, some carry `sso`, some carry a login middleware, some
rate-limit. With a single template, adopting anything but the group that happens
to match it would silently **drop** a host's `sso`/`rate-limit`/login middleware,
or **impose** an access list on a host that is public on purpose - either way the
host keeps serving, with a chain nobody chose.

`profiles` is a map of operator-defined chains, each with **exactly the same
shape and the same validation as `template`** (`upstream` XOR `upstreamGroupRef`,
required `tls.certificateRef`, name-checked `middlewares`/`accessLists`,
`robotsNoIndex`, range-checked `timeouts`, `tags`,
validated `stripResponseHeaders`, `defaultDNS`). An `Ingress` picks one with `gpm.rake.pro/profile`.

**The annotation carries a name and nothing else - that is the security model.**
An Ingress author is untrusted: in a shared cluster a tenant may be able to
create or edit an `Ingress`, and gpm sits at the edge in front of everything.
There is deliberately **no** annotation that lets an Ingress name a middleware,
an access list, a certificate or an upstream, because such an annotation is a
self-service privilege grant - `access-lists: ""` on your own namespace's Ingress
would remove `home-vpn` from a hostname at the edge. Every profile is written by
you, here, in the config repo; a manifest chooses among them and can never invent
one, so a derived host is always one of a set you authored and can audit.

**The residual risk, stated plainly: every profile is selectable by every
annotating Ingress - define only profiles you are willing for any cluster tenant
to choose.** Selection is not restricted per namespace, per Ingress or per
tenant. A tenant who can annotate an Ingress in their own namespace can pick the
*most permissive* profile you defined, so a profile with no access list (like
`public-ratelimited` in the example below) is effectively an open door any tenant
may walk their own hostname through. That is a real capability - bounded by a set
you control, not unbounded, but not nothing. If it is too coarse for your cluster,
the escalation path is an operator-side selector table (deferred, see
[design/ingress-discovery.md section 5a](../../../design/ingress-discovery.md), which also holds
the full threat model).

**Resolution rules:**

| `gpm.rake.pro/profile` | Result |
|---|---|
| absent | the default `template` block |
| present but empty or whitespace-only | treated as absent -> the default `template` |
| exact match on a `profiles` key (surrounding whitespace trimmed) | that profile, **verbatim** |
| anything else | the Ingress is **skipped**, with the requested name in the status `reason` and a `warn` log |

An undefined profile is **never** downgraded to the default and never adopted
with a partial chain - falling back is exactly the silent regression profiles
exist to prevent. Matching is exact: no prefix match, no case folding, no
nearest-neighbour guess.

**An unresolvable profile fails closed on an existing host.** If the Ingress
already has a derived host, that host is not left alone: it is updated with
`disabled: true`. Nothing is deleted, nothing is rewritten, and re-adding the
profile re-enables it on the very next reconcile. This is what makes **revocation**
work. Leaving it untouched would mean a tenant could pin a chain you have just
tightened, renamed or retired - by pointing the annotation at a name that does
not exist - and the host would go on serving the revoked chain indefinitely.
Retiring a profile (or clearing the profile rows in the UI) disables the hosts
derived from it for the same reason.

Every *other* derive failure - a malformed hostname, an unusable derived name -
still **freezes** the existing host instead: your policy has not changed there,
the host on disk is the last good rendering of it, and failing closed would let
any tenant take their own service offline with a one-character manifest edit.
A profile is applied **verbatim**, never merged with the template, so the
default's access list can never leak onto a profile that is public on purpose.

Every profile validates at **settings-write time**, not at reconcile time - an
invalid one is rejected by `PUT /api/settings` where you see it, rather than
surfacing later as a skipped host. That includes **referential** validation: the
`certificateRef`, `upstreamGroupRef`, `middlewares`, `accessLists` and
`tls.clientAuth.caRef` of the template and of every profile are cross-checked
against the objects that actually exist. A dangling name there is not a
localised problem later - it is stamped onto every derived host, and the store
validates a reconcile as one batch, so the rejection would drop every *other*
tenant's create, update and delete too, on every poll, until it was fixed.
A disabled `ingressDiscovery` block is not cross-checked, so a half-filled draft
never blocks an unrelated settings write.

`GET /api/ingress-discovery/status` reports the resolved `profile` per host (the
literal `template` for the default block), so you can audit what chain a given
Ingress actually got. `GET /api/ingress-discovery/plan` (`ingress-discovery:read`,
wired into the settings UI as **Preview changes**) reports the same per-host
decisions a reconcile would take - `created`/`updated`/`deleted`/`skipped` counts
and the per-host `hosts` list - without writing anything, mirroring
`GET /api/dns-sync/plan`. Both answer `409 Conflict` while a reconcile is already
in flight.

## Operator-side profile selection (`profileRules`)

`gpm.rake.pro/profile` puts the tenant in charge of *choosing* among the profiles
you defined (never inventing one), but every profile stays selectable by every
annotating Ingress. `profileRules` is the escalation path when that is too
coarse: an ordered list of `{namespace?, matchLabels?, profile}` rules, evaluated
**before** the annotation. The **first matching rule wins** and its `profile` is
used - exactly as if the Ingress had carried that name in `gpm.rake.pro/profile`
- and the annotation is **not consulted** for that Ingress at all. A rule with no
`namespace` matches any namespace; no `matchLabels` matches any labels; both set
are AND'd together. `profile` names `template` (the default block) or a
`profiles` key, and is cross-checked at `Settings.Validate` - a rule naming an
undefined profile fails the settings write.

When no rule matches, `profileSelection` decides what happens next:

- `"annotation-or-rules"` (default, i.e. empty) - falls back to
  `gpm.rake.pro/profile`, unchanged from the no-`profileRules` behaviour above.
- `"rules-only"` - the annotation is **never read at all**. An Ingress that
  matches no rule gets the default `template`, exactly as if it carried no
  annotation, even if its own `gpm.rake.pro/profile` names a real profile. Use
  this when the Ingress author should have no say in profile selection
  whatsoever.

`profileRules` is strictly stronger than the annotation - the Ingress author
cannot see, match or influence a rule - at the cost of a settings commit per new
namespace/label combination rather than per new service. Named profiles (and the
template) remain the substrate either mode selects from; the vocabulary an
Ingress or a rule can reach is always one the operator authored in
`ingressDiscovery.template`/`.profiles`.

## Per-profile `allowedDomainSuffixes`

The top-level `allowedDomainSuffixes` bounds every derived host by default. A
profile (or the template) may set its **own** `allowedDomainSuffixes` to narrow
that further for hosts it derives - e.g. a `public` profile restricted to
`public.example.com` even though the global list also covers
`internal.example.com`. It must be a **subset** of the global list (every entry
equal to, or a dot-boundary sub-suffix of, some global entry); a profile that
would *widen* the domains a tenant could publish fails `Settings.Validate` at the
settings write, the same standard every other profile field is held to. Leaving
it unset means the profile uses the global list unchanged - this is a narrowing
knob only, never a second way to grant a wider set of names.

## What a derived host cannot express

A derived host is a full `ProxyHost` with two deliberate exceptions:

- **`locations`** - path-scoped overrides with their own upstream and chain.
  Discovery forwards **everything** to the cluster ingress controller by vhost,
  and the controller does the path routing itself from the same `Ingress`, so a
  template-wide location list would be applied to every derived host regardless
  of the service. The useful version is per-service, and reading paths/upstreams
  /chains out of a cluster manifest is exactly the self-service privilege grant
  the annotation model forbids. If you need a per-path chain on a discovered
  service, publish a second hostname (a second `Ingress`) and annotate it, or
  hand-write the host and leave it out of discovery - an unlabelled host is never
  touched. See [design/ingress-discovery.md section 5](../../../design/ingress-discovery.md).
- **`displayName`** - always the source `<namespace>/<name>`, so the host list
  says where a host came from. Not templatable, because one display name shared
  by every derived host would carry no information.

Everything else a hand-written proxy host can set - `robotsNoIndex`, `timeouts`,
`tags`, TLS/mTLS, middlewares, access lists, websockets, DNS policy - is a
template (and therefore a profile) field.

Worked example, modelled on a mixed fleet:

```yaml
ingressDiscovery:
  enabled: true
  allowedDomainSuffixes: [example.com]
  # The default: no middleware, everything behind the VPN access list. Any
  # Ingress that names no profile gets this - unchanged from before profiles.
  template:
    upstreamGroupRef: k8s-nodes
    tls: { certificateRef: wildcard, forceSSL: true }
    accessLists: [home-vpn]
    robotsNoIndex: true          # internal by default: keep it out of search
    tags: [cluster]
    defaultDNS: { lanDirect: true }
  profiles:
    # Public on purpose - rate-limited, and NO access list.
    public-ratelimited:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true }
      middlewares: [rate-limit]
      tags: [cluster, public]
      defaultDNS: { lanDirect: true, publicCname: true }
    # SSO-gated and VPN-restricted, and slow to answer on a cold start.
    sso-internal:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true }
      middlewares: [sso, rate-limit]
      accessLists: [home-vpn]
      robotsNoIndex: true
      timeouts: { connectSeconds: 5, readSeconds: 60 }
      tags: [cluster, sso]
      defaultDNS: { lanDirect: true }
```

```yaml
# paste.example.com: public, rate-limited.
metadata:
  annotations:
    gpm.rake.pro/managed: "true"
    gpm.rake.pro/profile: "public-ratelimited"
---
# radarr.example.com: sso + home-vpn.
metadata:
  annotations:
    gpm.rake.pro/managed: "true"
    gpm.rake.pro/profile: "sso-internal"
---
# grafana.example.com: no profile named, so the default template applies.
metadata:
  annotations:
    gpm.rake.pro/managed: "true"
```

**The upstream is the ingress controller, not the Ingress backend Service.** gpm
runs *outside* the cluster (on the edge host), so
`<svc>.<ns>.svc.cluster.local` is neither resolvable nor routable from it. gpm
therefore ignores `spec.rules[].http.paths[].backend` entirely and forwards every
derived host to `template.upstream`; the controller then routes to the right
workload **by vhost**, using the `Host` header the data plane preserves on the way
through. Prefer `scheme: http` to the controller's plain port (TLS is terminated
once, at the edge, and the edge->LB hop is a trusted LAN path). With
`scheme: https` the Go transport derives SNI and certificate verification from the
**upstream host**, not from the forwarded `Host`, so an https upstream must name a
hostname the controller's certificate actually covers - pointing it at a bare IP
will fail verification.

**Derived objects.** Each opted-in Ingress produces one proxy host named
`ing-<ingressName>.<namespace>` (e.g. `ing-grafana.monitoring`), carrying
`labels["gpm.rake.pro/managed-by"] = "ingress-discovery"`. Its `domains` are the
`spec.rules[].host` values, lowercased, de-duplicated and sorted. `spec.tls` is
read but is **not** authoritative: it selects no certificate and contributes no
domain.

**Ownership - what discovery will and will not touch.** Only objects carrying the
managed-by label are ever created, updated or deleted. A hand-written proxy host
with the same name is **skipped with a warning**, never overwritten and never
removed (the same rule the DNS backends apply to records they do not own). To
adopt a discovered host permanently, remove the label: gpm then treats it as
operator-authored and stops managing it.

Ownership covers the **domain** as well as the name. A derived host whose domains
include one already claimed by a host discovery does not own - proxy, redirect or
dead, enabled *or* disabled - is skipped with that host named in the `reason`.
Without that rule a tenant who can annotate an `Ingress` in their own namespace
could claim `sso.example.com`, and because the router fills its per-domain maps
in config load order, a derived name sorting after the operator's host would
silently replace its whole middleware/access-list chain and its TLS pinning.
`allowedDomainSuffixes` alone does not prevent this: an exact-match suffix makes
even the apex claimable. Two annotated Ingresses claiming the same hostname are
resolved the same way - first by derived name wins, the rest are skipped.

**What a derived host cannot express.** Cutting a hand-written host over to
discovery - annotate the `Ingress`, let the derived host appear, delete the
hand-written one - silently drops everything the template has no field for. A
derived host carries only `upstream`/`upstreamGroupRef`, `tls`,
`middlewares`, `accessLists` and the `dns` policy. It cannot carry:

- `timeouts` - per-host dial/response overrides; the host falls back to the
  shared transport defaults, which a slow backend will notice,
- `locations` - path-scoped routing, and with it any path-scoped middleware,
  access list or upstream override,
- `robotsNoIndex` - the `X-Robots-Tag: noindex, nofollow` response header,
- `tags`, and `displayName`, which is fixed to `<namespace>/<name>` (cosmetic).

So **diff each hand-written host against what the template derives before you
delete it**, field by field, and keep it hand-written if it uses any of the
above. Adding them back afterwards is not an option: manual edits to a managed
host are reverted on the next poll (see the reserved-label note near the top of
this document).

Ownership is re-checked **under the store lock at write time**, not only when the
reconcile was planned: the plan is made before a multi-second cluster list, so an
object relabelled or replaced in that window is left alone rather than written on
the strength of a stale snapshot.

**Hostname validation.** Every string from the API server is untrusted. A host
must be a valid multi-label LDH hostname of at most 253 characters and fall
within `allowedDomainSuffixes`; wildcards, single labels, underscores, URLs and
anything with whitespace or control characters are rejected. An Ingress with no
usable host is skipped. Nothing about an Ingress can supply an upstream, a
certificate, a middleware or an access list - those come only from the template
or a named profile, and a profile is selected by *name* only, so a cluster user
who can edit an Ingress can never weaken the chain you configured.

**When the cluster cannot be read, discovery freezes.** A managed host is deleted
**only** when a reconcile obtained a complete, successful, fully-paginated list of
annotated Ingresses and the derived name is absent from it. Any transport error,
timeout, non-`200` status, decode failure, a page that fails mid-pagination, or a
`200` whose body is not an `IngressList` (a mistyped `apiURL` landing on another
HTTPS service behind the same CA, a mesh or gateway envelope, a `Status` reply)
aborts the run *before any write* - no creates, no updates, no deletes. One list
is bounded to two minutes end to end, so a hung endpoint fails the run rather
than holding the reconciler for the page limit times the per-request timeout. An
annotated Ingress that cannot be derived (bad hostname, unusable name) is skipped
**and** protects its existing host from deletion, so one bad manifest edit cannot
take a host offline - the one exception being an unresolvable *profile*, which
disables the existing host rather than freezing it (see "Resolution rules"). An
*empty successful* list is a different thing entirely: it
is a legitimate delete-all, applied and logged per host at WARN.

**Writes land as one commit per reconcile** - every create, update and delete from
one poll is a single revision (`Ingress discovery: reconcile (+N ~M -K)`, authored
by `ingress-discovery`), so history stays readable and revert is meaningful. A
reconcile that finds no drift writes nothing at all.

Discovery publishes no DNS itself: it sets the `dns` policy on the derived hosts
and asks the phase-1 reconciler for a run, so there is exactly one DNS code path.

**Disabling a derived host withdraws its DNS.** The host *object* is preserved by
a disable - but a disabled host contributes nothing to the DNS desired set, so
the next reconcile treats its domains as no longer wanted: records **gpm
created** are **deleted**, records gpm had only **adopted** are **released**
(dropped from the ledger, left standing). This is deliberate and fail-closed - a
name must not keep resolving to an edge that no longer serves it - but it means
anything that disables a derived host takes its public and LAN DNS down with it,
including a discovery profile that has been retired or that no longer resolves.
Putting the profile back re-enables the host and **recreates** the records, after
up to one poll interval (default 60s) plus whatever negative-cache TTL the
resolvers on the far side are still holding, so the name does not necessarily
come back the moment the config does. A released record needs nothing recreated:
it never left, and the next reconcile simply re-adopts it.

`POST /api/ingress-discovery/reconcile` runs a reconcile on demand (**409
Conflict** while one is in flight, never queued);
`GET /api/ingress-discovery/status` reports the last run, including `lastRun` vs
`lastSuccess` - separate on purpose, so a frozen state cannot look fresh - and a
per-host list of actions (`created` / `updated` / `unchanged` / `deleted` /
`skipped`, with the resolved `profile` and a reason for each skip). The cluster-side RBAC to apply is
[`deploy/k8s-ingress-discovery-rbac.yaml`](https://github.com/Rake-Pro/go-proxy-manager/blob/main/deploy/k8s-ingress-discovery-rbac.yaml).

## Deprecated template fields

| Field | Status | Reason |
|---|---|---|
| <span id="settings-ingress-discovery-template-websockets-upgrade"></span> `template.websocketsUpgrade` (and the same key in a `profiles` entry) | Deprecated, ignored | Same as `proxyHost.websocketsUpgrade`: upgrades always work. Still stamped onto derived hosts so a template written before the deprecation keeps producing byte-identical hosts, instead of showing up as an update on every reconcile. |
| <span id="settings-ingress-discovery-template-tls-http2"></span> `template.tls.http2` | Deprecated, ignored | Same as `proxyHost.tls.http2`: HTTP/2 is always offered. |
