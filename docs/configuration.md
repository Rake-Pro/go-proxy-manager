# Configuration reference

go-proxy-manager is configured by a set of typed YAML objects in a git-backed
directory (default `/data/config`). You can edit them through the web UI / REST
API, or write the files directly and let the daemon load them on start/reload.

## Layout

```
config/
  settings.yaml            # singleton app settings
  proxy-hosts/<name>.yaml
  redirect-hosts/<name>.yaml
  stream-hosts/<name>.yaml
  dead-hosts/<name>.yaml
  certificates/<name>.yaml
  dns-providers/<name>.yaml
  identity-providers/<name>.yaml
  access-lists/<name>.yaml
  middlewares/<name>.yaml
  api-tokens/<name>.yaml
```

One object per file; the file's base name must equal the object's `name`. The
directory is a git repository — every change made through the API is a commit,
and the whole graph is validated before it is accepted (a reference to a
non-existent certificate, middleware, access list, identity provider, or DNS
provider is a load-time error, and an object cannot be deleted while another
references it).

## Common fields (every object)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | yes | Identity and filename. Lowercase alphanumeric plus `-_.`, must start and end alphanumeric, 1–254 chars. |
| `displayName` | string | no | Human label for the UI. |
| `labels` | map | no | Arbitrary key/value metadata. **`gpm.rake.pro/managed-by` is reserved** — see below. |
| `tags` | []string | no | Flat, free-form labels for grouping/filtering. On the Proxy Hosts list they render as chips and are matched by the filter box. |
| `disabled` | bool | no | Keep the object in config but exclude it from the running data plane. |

> **`gpm.rake.pro/managed-by` is a reserved label — do not set it by hand.** It
> marks an object as owned by an automated reconciler. Adding
> `gpm.rake.pro/managed-by: ingress-discovery` to a proxy host you wrote yourself
> hands it to Ingress discovery, which will **delete it** on the next poll,
> because no annotated `Ingress` derives it. Removing the label is the supported
> way to adopt a discovered host permanently; adding it is never the way to give
> one away.

## Domains are exclusive

The data plane routes by hostname, so **at most one enabled host may claim a
given domain**. Two enabled proxy, redirect or dead hosts listing the same domain
are rejected at load time (`hosts "a" and "b" both claim domain "x.example.com"`)
rather than resolved by whichever file happens to be read last. *Disabled* hosts
are exempt: they are excluded from the running data plane entirely, so staging a
replacement host beside the live one stays legal — enable the new one in the same
change that disables the old one.

## Secrets

Secret-valued fields (API tokens, client secrets, etc.) must be **placeholders**,
not literal values:

```
${ENV:CF_API_TOKEN}        # resolved from the environment variable
${FILE:/run/secrets/token} # resolved from a file (e.g. a Docker secret), trimmed
```

Placeholders resolve lazily, at the moment the secret is used. Committing a
literal secret is refused with `refusing to commit literal secret(s): ...` — on
`Save`, on `SaveSettings`, **and on `Restore`** (an uploaded backup archive cannot
smuggle a plaintext secret onto disk or into git history; a refused restore rolls
the working tree back and commits nothing). In API responses, literal secrets are
redacted to `***`; placeholders are returned verbatim.

`${FILE:...}` reads are confined to an allowlisted root, defaulting to
`/run/secrets`. A path that is relative, or outside the allowed root (including
via `..`), is refused — so a config write cannot turn a file-backed secret into
an arbitrary host-file read. Override the allowed roots with the
`GPM_SECRET_FILE_ROOTS` environment variable (a list of absolute directories,
separated by the OS path-list separator, e.g. `:` on Linux).

`${ENV:...}` resolution has two guards. gpm's own sensitive process env vars —
`GPM_SSO_SIGNING_KEY` and `GPM_LOCAL_ADMIN_PASSWORD_HASH` — are **never**
resolvable via a `${ENV:...}` placeholder, so an admin-authored config value
cannot exfiltrate them (e.g. as a webhook secret posted to an attacker URL). By
default any other env var name resolves. To lock this down further, set
`GPM_SECRET_ENV_PREFIXES` to a comma-separated list of allowed name prefixes
(e.g. `GPM_SECRET_,APP_`); then only `${ENV:...}` names carrying one of those
prefixes resolve and everything else is refused.

---

## Settings (`config/settings.yaml`)

Singleton application configuration.

| Field | Type | Notes |
|-------|------|-------|
| `schemaVersion` | int | Config schema version. |
| `appName` | string | Brand label in the UI and login page. Default `Go Proxy Manager`. |
| `externalBaseURL` | string | Canonical public URL of the admin panel. Must be an absolute URL. Used to build the OIDC `redirect_uri` so it never depends on spoofable `X-Forwarded-*` headers. |
| `adminAuth.providers` | []string | Identity-provider names allowed to log into the admin panel. |
| `adminAuth.localLoginEnabled` | bool | Keep username/password login available (anti-lockout). Default true. |
| `adminAuth.ssoOnly` | bool | Disable local login entirely. Requires at least one `providers` entry. Recovery from an SSO outage is by redeploying with local login re-enabled. |
| `webhooks` | []WebhookConfig | Outbound lifecycle notifications (below). |
| `dnsSync` | DNSSyncSettings | Optional DNS record reconcilers (below). |
| `ingressDiscovery` | IngressDiscoverySettings | Optional Kubernetes Ingress discovery (below). |

**WebhookConfig**: `name` (required, name-safe identifier), `url` (required,
absolute http/https), optional `secret` (placeholder-resolved, sent as the
`X-GPM-Webhook-Secret` header), `disabled` (keep configured but do not fire). After
every successful config change gpm POSTs a JSON event
`{"action","kind","name","commit","time"}` to each enabled target. `action` is one
of `save` | `delete` | `restore` | `revert` | `settings` | `ingress-discovery`. Delivery is asynchronous
and best-effort under a 10s timeout — a slow or unreachable endpoint never blocks
or fails the config write, it is only logged. Because targets are admin-configured
URLs, delivery is SSRF-bounded as defense in depth: **redirects are never
followed** (a 3xx counts as a failed delivery, so a receiver cannot bounce gpm to
a URL the admin didn't configure) and **link-local destinations are refused at
connect time, post-DNS** (blocking cloud-metadata pivots such as
`169.254.169.254` even via a rebinding resolver). Private/LAN targets remain
allowed — they are the normal self-hosted case.

```yaml
schemaVersion: 1
appName: Go Proxy Manager
externalBaseURL: https://gpm.example.com
adminAuth:
  providers: [authentik-oidc]
  localLoginEnabled: true
  ssoOnly: false
webhooks:
  - name: ci
    url: https://hooks.example.com/gpm
    secret: ${FILE:/run/secrets/gpm_webhook_secret}
dnsSync:
  pihole:
    enabled: true
    url: http://pihole.lan
    appPassword: ${FILE:/run/secrets/pihole_app_password}
    apexTarget: edge.example.com
  cloudflare:
    enabled: true
    dnsProviderRef: cloudflare      # an existing dns-providers entry
    zoneName: example.com
    apexTarget: edge.example.com
    proxied: false
ingressDiscovery:
  enabled: true
  apiURL: https://k8s.example.lan:6443
  tokenFile: /run/secrets/gpm-k8s-token
  caFile: /run/secrets/gpm-k8s-ca.crt
  pollInterval: 60s
  allowedDomainSuffixes: [example.com]
  template:
    upstream: { scheme: http, host: 10.0.0.40, port: 80 }   # the ingress controller
    tls:
      certificateRef: wildcard
      forceSSL: true
      http2: true
    middlewares: [sso]
    defaultDNS: { lanDirect: true }
  profiles:                         # optional named chains, selected per Ingress
    public-ratelimited:
      upstream: { scheme: http, host: 10.0.0.40, port: 80 }
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
      middlewares: [rate-limit]     # and no access list: public on purpose
```

### DNSSyncSettings (`settings.dnsSync`)

Publishes CNAME records for the proxy hosts that opted in via their
[`dns` policy](#proxyhost-configproxy-hosts). Both backends are independently
enabled; with both disabled (the default) the subsystem is inert and never
contacts anything.

| Field | Type | Notes |
|-------|------|-------|
| `pihole.enabled` | bool | Turn on local (LAN) CNAME reconciliation. |
| `pihole.url` | string | Pi-hole base URL, absolute http/https, no `/api` suffix. Required when enabled. |
| `pihole.appPassword` | Secret | Pi-hole **application password** (placeholder-resolved). Used for `POST /api/auth`. |
| `pihole.apexTarget` | string | CNAME target every managed record points at. Required when enabled. **Also the ownership marker** — see below. |
| `cloudflare.enabled` | bool | Turn on public zone reconciliation. |
| `cloudflare.dnsProviderRef` | string | Name of an existing [DNSProvider](#dnsprovider-configdns-providers) whose `config.apiToken` is reused. Required when enabled. |
| `cloudflare.zoneName` | string | Zone the records live in, e.g. `example.com`. Required when enabled. |
| `cloudflare.apexTarget` | string | CNAME content every managed record points at. Required when enabled. |
| `cloudflare.proxied` | bool | Cloudflare orange-cloud flag on created records. Default `false` (DNS only). |

**Ownership — what gpm will and will not delete.** Reconcile is *full-state*: the
desired set is recomputed from the whole config on every run, so a record deleted
out of band is recreated and a host removed while gpm was down is still cleaned
up. Deletion, however, is strictly limited to records gpm demonstrably owns:

- **Pi-hole** — only a CNAME whose target is *exactly* `pihole.apexTarget`. A
  hand-written entry pointing anywhere else is read and ignored, even when it
  names a domain gpm also serves. If a desired name already exists with a
  different target, gpm logs it and creates nothing for that name rather than
  adding a second, shadowing entry.
- **Cloudflare** — only a record carrying the comment `managed-by:gpm`, which gpm
  writes on every record it creates. If a desired name already exists as somebody
  else's record, gpm logs it and leaves both alone rather than creating a
  duplicate or removing theirs.

> **Changing `apexTarget` orphans the records created under the old one.**
> Ownership on Pi-hole is target equality, so as soon as the apex changes, every
> record gpm previously created stops matching and becomes, by its own rules,
> somebody else's — it will never be updated or deleted again, and the names now
> conflict with the desired set (so nothing is recreated either, per the
> skip-and-warn above). The same applies on Cloudflare for records that predate
> the `managed-by:gpm` comment. **Delete the stale records yourself before or
> right after changing `apexTarget`**, then run
> `POST /api/dns-sync/reconcile` to recreate them against the new target.

Wildcard domains (`*.example.com`) are skipped by both backends, as is a domain
equal to the apex target (which would be a CNAME loop). Disabled proxy hosts
contribute nothing.

`dnsProviderRef` is validated for *name shape* at settings-write time only:
settings are a separate singleton from the object graph, so a reference to a
missing DNSProvider surfaces at reconcile time in the sync status, not as a
rejected write.

A reconcile is triggered automatically after any proxy-host write, settings
change, restore or whole-config revert (non-blocking, and bursts coalesce into a
single run), and can be run on demand with `POST /api/dns-sync/reconcile`. The
manual endpoint never queues: if a reconcile is already running it answers **409
Conflict** rather than blocking, so repeated clicks cannot stack requests behind a
slow backend. `GET /api/dns-sync/status` reports the last run per backend. A
Pi-hole `403` is
surfaced as a distinct error — it means the session is read-only or the instance
was built without `webserver.api.app_sudo`, which retrying will not fix.

### IngressDiscoverySettings (`settings.ingressDiscovery`)

Discovers annotated Kubernetes `Ingress` objects and reconciles them into
template-derived proxy hosts, which then feed the DNS sync above. Disabled (the
default) means the subsystem is inert and never contacts anything. The full
rationale is in [docs/design/ingress-discovery.md](design/ingress-discovery.md).

| Field | Type | Notes |
|-------|------|-------|
| `enabled` | bool | Turn discovery on. Everything below is validated only when this is true. |
| `apiURL` | string | Kubernetes API base URL, **absolute https**. Empty uses the in-cluster endpoint (`KUBERNETES_SERVICE_HOST`/`_PORT`). |
| `tokenFile` | string | Absolute path to the read-only ServiceAccount bearer token. Empty uses the projected in-cluster path. **Re-read from disk periodically** (5 min), so a rotated projected token keeps working. |
| `caFile` | string | Absolute path to the PEM bundle that verifies the API server. Empty uses the projected in-cluster CA. There is no skip-verify option. |
| `namespace` | string | Restrict the list to one namespace. Empty lists cluster-wide (still annotation-gated). |
| `labelSelector` | string | Optional server-side label selector. The opt-in annotation is still required — the Kubernetes API cannot select on annotations, so that filter is always client-side. |
| `pollInterval` | duration | Go duration string. Default `1m`, **minimum `15s`** (a reconcile takes the store write lock). |
| `allowedDomainSuffixes` | []string | **Required when enabled.** A discovered hostname must equal one of these or end in `.` + one of them. |
| `template.upstream` | Upstream | Where every derived host forwards: the **cluster ingress controller's** address. Mutually exclusive with `template.upstreamGroupRef`. |
| `template.upstreamGroupRef` | string | Names an `upstream-groups` entry instead of a single address. **Prefer this when the ingress controller runs on more than one node** - otherwise every derived host is pinned to one node while hand-written hosts fail over. |
| `template.tls` | TLSSettings | Applied verbatim. `certificateRef` is **required**. |
| `template.websocketsUpgrade` | bool | Applied to every derived host. |
| `template.middlewares` | []string | Applied to every derived host, in order. |
| `template.accessLists` | []string | Applied to every derived host. |
| `template.defaultDNS` | DNSSyncPolicy | The `dns` policy a derived host gets when the corresponding annotation is absent. Each flag is overridden individually by its annotation. |
| `profiles` | map[string]→ same shape as `template` | Additional named chains an Ingress may **select by name** (below). Each key is a profile name (`ValidateName` shape); `template` is reserved for the default block. |

**Opt-in annotations** (on the `Ingress`, never on gpm's side):

| Annotation | Value | Meaning |
|------------|-------|---------|
| `gpm.rake.pro/managed` | `"true"` | Opt this Ingress into discovery. Absent or any other value means gpm ignores it entirely. There is no opt-out mode and no namespace sweep. |
| `gpm.rake.pro/profile` | a `profiles` key | Select one of the operator-defined profiles. Absent (or empty) uses the default `template`. An **undefined** name skips the Ingress. |
| `gpm.rake.pro/lan-direct` | `"true"` \| `"false"` | Sets `dns.lanDirect` on the derived host, overriding the resolved profile's `defaultDNS`. |
| `gpm.rake.pro/public-cname` | `"true"` \| `"false"` | Sets `dns.publicCname` on the derived host. |

#### Discovery profiles

One template only fits a uniform fleet. A real one is not: some hosts are
deliberately public, some carry `sso`, some carry a login middleware, some
rate-limit. With a single template, adopting anything but the group that happens
to match it would silently **drop** a host's `sso`/`rate-limit`/login middleware,
or **impose** an access list on a host that is public on purpose — either way the
host keeps serving, with a chain nobody chose.

`profiles` is a map of operator-defined chains, each with **exactly the same
shape and the same validation as `template`** (`upstream` XOR `upstreamGroupRef`,
required `tls.certificateRef`, name-checked `middlewares`/`accessLists`,
`websocketsUpgrade`, `defaultDNS`). An `Ingress` picks one with
`gpm.rake.pro/profile`.

**The annotation carries a name and nothing else — that is the security model.**
An Ingress author is untrusted: in a shared cluster a tenant may be able to
create or edit an `Ingress`, and gpm sits at the edge in front of everything.
There is deliberately **no** annotation that lets an Ingress name a middleware,
an access list, a certificate or an upstream, because such an annotation is a
self-service privilege grant — `access-lists: ""` on your own namespace's Ingress
would remove `home-vpn` from a hostname at the edge. Every profile is written by
you, here, in the config repo; a manifest chooses among them and can never invent
one, and can never produce a host weaker than something you explicitly
sanctioned. Full threat model in
[design/ingress-discovery.md §5a](design/ingress-discovery.md).

**Resolution rules:**

| `gpm.rake.pro/profile` | Result |
|---|---|
| absent | the default `template` block |
| present but empty or whitespace-only | treated as absent → the default `template` |
| exact match on a `profiles` key (surrounding whitespace trimmed) | that profile, **verbatim** |
| anything else | the Ingress is **skipped**, with the requested name in the status `reason` and a `warn` log |

An undefined profile is **never** downgraded to the default and never adopted
with a partial chain — falling back is exactly the silent regression profiles
exist to prevent. The skip also **protects** any existing derived host for that
Ingress from deletion, so a typo in an annotation cannot take a service offline.
Matching is exact: no prefix match, no case folding, no nearest-neighbour guess.
A profile is applied **verbatim**, never merged with the template, so the
default's access list can never leak onto a profile that is public on purpose.

Every profile validates at **settings-write time**, not at reconcile time — an
invalid one is rejected by `PUT /api/settings` where you see it, rather than
surfacing later as a skipped host.

`GET /api/ingress-discovery/status` reports the resolved `profile` per host (the
literal `template` for the default block), so you can audit what chain a given
Ingress actually got.

Worked example, modelled on a mixed fleet:

```yaml
ingressDiscovery:
  enabled: true
  allowedDomainSuffixes: [example.com]
  # The default: no middleware, everything behind the VPN access list. Any
  # Ingress that names no profile gets this - unchanged from before profiles.
  template:
    upstreamGroupRef: k8s-nodes
    tls: { certificateRef: wildcard, forceSSL: true, http2: true }
    accessLists: [home-vpn]
    defaultDNS: { lanDirect: true }
  profiles:
    # Public on purpose - rate-limited, and NO access list.
    public-ratelimited:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
      middlewares: [rate-limit]
      defaultDNS: { lanDirect: true, publicCname: true }
    # SSO-gated and VPN-restricted.
    sso-internal:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
      middlewares: [sso, rate-limit]
      accessLists: [home-vpn]
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
once, at the edge, and the edge→LB hop is a trusted LAN path). With
`scheme: https` the Go transport derives SNI and certificate verification from the
**upstream host**, not from the forwarded `Host`, so an https upstream must name a
hostname the controller's certificate actually covers — pointing it at a bare IP
will fail verification.

**Derived objects.** Each opted-in Ingress produces one proxy host named
`ing-<ingressName>.<namespace>` (e.g. `ing-grafana.monitoring`), carrying
`labels["gpm.rake.pro/managed-by"] = "ingress-discovery"`. Its `domains` are the
`spec.rules[].host` values, lowercased, de-duplicated and sorted. `spec.tls` is
read but is **not** authoritative: it selects no certificate and contributes no
domain.

**Ownership — what discovery will and will not touch.** Only objects carrying the
managed-by label are ever created, updated or deleted. A hand-written proxy host
with the same name is **skipped with a warning**, never overwritten and never
removed (the same rule the DNS backends apply to records they do not own). To
adopt a discovered host permanently, remove the label: gpm then treats it as
operator-authored and stops managing it.

Ownership covers the **domain** as well as the name. A derived host whose domains
include one already claimed by a host discovery does not own — proxy, redirect or
dead, enabled *or* disabled — is skipped with that host named in the `reason`.
Without that rule a tenant who can annotate an `Ingress` in their own namespace
could claim `sso.example.com`, and because the router fills its per-domain maps
in config load order, a derived name sorting after the operator's host would
silently replace its whole middleware/access-list chain and its TLS pinning.
`allowedDomainSuffixes` alone does not prevent this: an exact-match suffix makes
even the apex claimable. Two annotated Ingresses claiming the same hostname are
resolved the same way — first by derived name wins, the rest are skipped.

Ownership is re-checked **under the store lock at write time**, not only when the
reconcile was planned: the plan is made before a multi-second cluster list, so an
object relabelled or replaced in that window is left alone rather than written on
the strength of a stale snapshot.

**Hostname validation.** Every string from the API server is untrusted. A host
must be a valid multi-label LDH hostname of at most 253 characters and fall
within `allowedDomainSuffixes`; wildcards, single labels, underscores, URLs and
anything with whitespace or control characters are rejected. An Ingress with no
usable host is skipped. Nothing about an Ingress can supply an upstream, a
certificate, a middleware or an access list — those come only from the template
or a named profile, and a profile is selected by *name* only, so a cluster user
who can edit an Ingress can never weaken the chain you configured.

**When the cluster cannot be read, discovery freezes.** A managed host is deleted
**only** when a reconcile obtained a complete, successful, fully-paginated list of
annotated Ingresses and the derived name is absent from it. Any transport error,
timeout, non-`200` status, decode failure, a page that fails mid-pagination, or a
`200` whose body is not an `IngressList` (a mistyped `apiURL` landing on another
HTTPS service behind the same CA, a mesh or gateway envelope, a `Status` reply)
aborts the run *before any write* — no creates, no updates, no deletes. One list
is bounded to two minutes end to end, so a hung endpoint fails the run rather
than holding the reconciler for the page limit times the per-request timeout. An
annotated Ingress that cannot be derived (bad hostname, unusable name) is skipped
**and** protects its existing host from deletion, so one bad manifest edit cannot
take a host offline. An *empty successful* list is a different thing entirely: it
is a legitimate delete-all, applied and logged per host at WARN.

**Writes land as one commit per reconcile** — every create, update and delete from
one poll is a single revision (`Ingress discovery: reconcile (+N ~M -K)`, authored
by `ingress-discovery`), so history stays readable and revert is meaningful. A
reconcile that finds no drift writes nothing at all.

Discovery publishes no DNS itself: it sets the `dns` policy on the derived hosts
and asks the phase-1 reconciler for a run, so there is exactly one DNS code path.

`POST /api/ingress-discovery/reconcile` runs a reconcile on demand (**409
Conflict** while one is in flight, never queued);
`GET /api/ingress-discovery/status` reports the last run, including `lastRun` vs
`lastSuccess` — separate on purpose, so a frozen state cannot look fresh — and a
per-host list of actions (`created` / `updated` / `unchanged` / `deleted` /
`skipped`, with the resolved `profile` and a reason for each skip). The cluster-side RBAC to apply is
[`deploy/k8s-ingress-discovery-rbac.yaml`](../deploy/k8s-ingress-discovery-rbac.yaml).

---

## ProxyHost (`config/proxy-hosts/`)

Terminates TLS for one or more domains and reverse-proxies to an upstream.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | One or more hostnames served by this host. |
| `upstream` | Upstream | one of | Default backend (single). Mutually exclusive with `upstreamGroupRef`. |
| `upstreamGroupRef` | string | one of | Name of an [UpstreamGroup](#upstreamgroup-configupstream-groups) for health-checked failover across several backends. Mutually exclusive with `upstream`. |
| `websocketsUpgrade` | bool | no | Offer WebSocket upgrades. |
| `robotsNoIndex` | bool | no | Emit `X-Robots-Tag: noindex, nofollow` (HTTP and HTTPS) to discourage search-engine indexing. A headers middleware that sets `X-Robots-Tag` explicitly still wins. |
| `timeouts` | HostTimeouts | no | Per-host upstream timeout overrides (below). |
| `tls` | TLSSettings | no | Certificate + TLS behaviour. |
| `dns` | DNSSyncPolicy | no | Opt this host's domains into DNS record management (below). |
| `middlewares` | []string | no | Host-wide middleware names, applied top-down. |
| `accessLists` | []string | no | Host-wide access-list names. |
| `locations` | []Location | no | Path-scoped overrides (below). |

**Upstream**: `scheme` (`http`|`https`), `host`, `port` (1–65535) — all required.

**TLSSettings**: `certificateRef` (a Certificate name), `forceSSL` (redirect
HTTP→HTTPS), `http2`, `hsts` (`enabled`, `maxAge` — seconds, default one year when
unset, `includeSubdomains`, `preload`), `minTLSVersion` (`"1.2"` default | `"1.3"`).
When `hsts.enabled` is set, the data plane emits `Strict-Transport-Security` on
HTTPS responses for the host (never over plain HTTP).

`minTLSVersion` is a **per-host** floor selected by SNI at handshake time. The
edge already negotiates TLS 1.2 *or* 1.3 per client (1.2 is the default floor);
set `"1.3"` only on hosts where every client supports it (drops 1.2 — old smart
TVs / embedded clients / legacy scripts may then fail to connect). Leave it unset
for public hosts to keep the widest client compatibility.

**DNSSyncPolicy** (`dns`): `lanDirect` publishes each of the host's domains as a
local CNAME on the LAN resolver (Pi-hole), so internal clients reach the edge
directly instead of hairpinning through the WAN address; `publicCname` publishes
them in the authoritative public zone (Cloudflare). Both default false — nothing
is published unless asked for, and an opted-out host omits the `dns` key from its
API responses entirely rather than returning an empty object. The backends
themselves are configured once, in
[`settings.dnsSync`](#dnssyncsettings-settingsdnssync); a policy flag with its
backend disabled publishes nothing (the UI greys the toggle out).

**HostTimeouts** (`timeouts`): `connectSeconds` caps establishing the TCP/TLS
connection to the upstream; `readSeconds` caps time awaiting the upstream's
response headers (time-to-first-byte). Both are whole seconds (0–3600); `0`/unset
means no override. A host with any override uses its **own** cloned transport
(its own connection pool), so a custom timeout never affects another host's
keep-alive reuse; hosts without an override share the default pooled transport.
`readSeconds` bounds only time-to-first-byte, so it does not cut off a slow
streaming / SSE / websocket body once headers have arrived.

**Location**: a path-scoped override. `path` (required), optional `upstream`
override, plus `middlewares` / `accessLists` that are **appended to** (not
replace) the host-wide chain — so a location is always at least as restrictive as
its host. Matching is longest-prefix; the request path is forwarded unchanged.
A location may set its own single `upstream` **or** its own `upstreamGroupRef`
(mutually exclusive); with neither it inherits the host's backend, including an
upstream group.

```yaml
name: app
domains: [app.example.com]
upstream: {scheme: http, host: backend, port: 8080}
websocketsUpgrade: true
tls: {certificateRef: wildcard, forceSSL: true}
dns: {lanDirect: true, publicCname: true}
middlewares: [require-sso]
locations:
  - path: /metrics
    accessLists: [internal-only]      # /metrics also requires the internal CIDR
```

---

## RedirectHost (`config/redirect-hosts/`)

Issues HTTP redirects.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | Source hostnames. |
| `targetDomain` | string | yes | Where to redirect. |
| `targetScheme` | string | no | `http`\|`https`\|`auto`. |
| `statusCode` | int | no | `301`\|`302`\|`307`\|`308` (0 = default). |
| `preservePath` | bool | no | Keep the request path. |
| `tls` | TLSSettings | no | |

```yaml
name: apex
domains: [example.com]
targetDomain: www.example.com
statusCode: 301
preservePath: true
```

---

## StreamHost (`config/stream-hosts/`)

Raw TCP/UDP forwarding. The data plane opens a listener per `listenPort` (TCP, UDP,
or both) and forwards to the backend; listeners are reconciled on every reload
(ports added/removed, backend swapped, with no listener restart for unchanged
ports). UDP uses per-client sessions with an idle timeout.

> **No access control at L4.** Stream hosts blind-forward: access lists, geo rules,
> rate limits, and identity/SSO are HTTP-layer controls and do **not** apply here.
> The only built-in bound is `maxUDPSessions` (4096), which caps spoofed-source UDP
> memory. Expose a stream port only on a trusted network, or put IP filtering in
> front of it at the firewall / host level. Do not publish a stream port to the
> public internet expecting gpm to gate it.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `listenPort` | int | yes | 1–65535. **Publish this port from the container** (compose `ports:`) so it is reachable, and avoid colliding with the data-plane 80/443 or admin port — a bind failure is logged and that one port is skipped, never fatal. |
| `protocol` | string | yes | `tcp`\|`udp`\|`both`. |
| `forwardHost` | string | yes | Backend host. |
| `forwardPort` | int | yes | 1–65535. |

```yaml
name: postgres
listenPort: 5432
protocol: tcp
forwardHost: db.internal
forwardPort: 5432
```

---

## DeadHost (`config/dead-hosts/`)

Returns a fixed status for claimed domains — useful to absorb unmatched vhosts and
stop default-host leakage.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `domains` | []string | yes | |
| `statusCode` | int | no | Default 404. |
| `tls` | TLSSettings | no | |

---

## UpstreamGroup (`config/upstream-groups/`)

An ordered set of interchangeable backends a ProxyHost forwards to instead of a
single `upstream`, with health-checked failover. The first upstream is preferred;
the rest are backups tried in order when an earlier one is unhealthy or
unreachable. Many hosts can reference one group — its backends are probed once
per group, not once per host.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `upstreams` | []GroupUpstream | yes | Ordered backend list. Same shape as a host `upstream` plus optional `weight`; duplicates rejected. |
| `policy` | string | no | `failover` (default) \| `round-robin` \| `least-connections` \| `ip-hash`. |
| `stickiness` | Stickiness | no | Cookie-based session affinity with a TTL (below). |
| `healthCheck` | HealthCheck | no | Active probe tuning (defaults below). |

**GroupUpstream**: `scheme`/`host`/`port` (as a host upstream) plus `weight`
(1–256, default 1) — the relative share for the weighted policies; ignored by
`failover` and `ip-hash`.

**Policies** (unhealthy upstreams always drop to the end of the try-order,
whatever the policy):

- `failover` — strict list order: the first healthy upstream takes all traffic,
  the rest are backups. The right default for identical entry points.
- `round-robin` — smooth weighted round-robin (nginx's algorithm) across the
  healthy set.
- `least-connections` — the healthy upstream with the fewest in-flight requests
  relative to its weight.
- `ip-hash` — rendezvous hashing on the client IP: a client sticks to one
  upstream while it stays healthy, and when an upstream dies only its own
  clients move (no global reshuffle).

**Stickiness** (`stickiness`): `ttl` (required — a Go duration such as `30m` /
`12h`, with a whole-day `d` suffix also accepted, e.g. `3d`) and `cookie`
(optional name, default `gpm-sticky-<group>`). On a client's first request the
data plane assigns an upstream by the policy and sets an HMAC-signed cookie
(`HttpOnly`, `Path=/`, `SameSite=Lax`, `Secure` when the client came over
HTTPS) naming it; later requests honor the pin while the cookie is valid and
that upstream is healthy. Semantics worth knowing:

- **TTL is enforced server-side** — the expiry rides inside the signed value, so
  a client replaying a cookie past its `Max-Age` is still re-assigned. The
  window is fixed from assignment (not sliding), matching "stick for X".
- The cookie is signed with the same key as data-plane SSO sessions
  (`GPM_SSO_SIGNING_KEY` / the persisted `sso_signing.key`), so a client cannot
  forge a pin to steer itself onto a chosen backend. A restart with an
  ephemeral key re-assigns everyone once.
- Composes with any `policy`: the policy picks the initial upstream (and the
  re-pick when the pinned one dies or the TTL lapses); the cookie holds it.
- Only cookie-honoring clients get affinity; raw API clients silently fall back
  to the policy — use `ip-hash` when those need stickiness too.
- An honored pin adds no `Set-Cookie` noise; the cookie is (re)issued only when
  the assignment is new or moved.

**HealthCheck**: `path` (optional — an HTTP GET probe of this path; blank keeps a
plain TCP-connect probe), `intervalSeconds` (default 5), `timeoutSeconds`
(default 3), `rise` (consecutive successes to return an upstream to service,
default 2), `fall` (consecutive failures to remove it, default 2).

```yaml
name: edge-nodes
policy: round-robin
stickiness: {ttl: 12h}          # optional: pin each client to its upstream
upstreams:
  - {scheme: http, host: 192.0.2.11, port: 80}
  - {scheme: http, host: 192.0.2.12, port: 80}
  - {scheme: http, host: 192.0.2.13, port: 80, weight: 2}   # weight used by weighted policies only
healthCheck: {path: /ping, intervalSeconds: 5, fall: 2, rise: 2}
```

Semantics worth knowing:

- **Failover retries only connect-phase failures** (dial refused / no route /
  dial timeout / TLS handshake). A request that may already have been sent —
  reset mid-response, timeout awaiting headers, or any HTTP response including a
  5xx — is never replayed against another upstream, so non-idempotent requests
  cannot double-apply. An application error also fails through every entry point
  equally, so retrying it would buy nothing.
- **Probes measure the entry point, not the app**: an HTTP probe counts any
  response below 500 as alive. Point `path` at something that reflects the
  upstream proxy/node itself (e.g. a ping endpoint), not at a shared application.
- **Passive detection feeds the same counters**: live-traffic connect failures
  count toward `fall`, so a dead upstream is skipped quickly even between probes.
- **Fail-open**: with every upstream marked down, requests are still attempted in
  preference order rather than rejected outright.
- **Request bodies up to 1 MiB are buffered** so a failover retry can replay
  them; a larger body streams and gets a single attempt at the preferred
  (healthiest) upstream.
- A `disabled` group cannot be referenced by an enabled host (validation error).
- Live health is exposed at `GET /api/upstream-health`
  (`{"<group>": [{"upstream": "http://192.0.2.11:80", "healthy": true, "weight": 1, "active": 0}, ...]}`,
  where `active` is the in-flight request count) and shown in the UI editor.

---

## Certificate (`config/certificates/`)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `type` | string | yes | `custom` or `acme`. |
| `domains` | []string | yes | Domains the cert covers (`*.example.com` for wildcard). |
| `acme` | ACMESpec | when `type: acme` | |
| `custom` | CustomCertSpec | when `type: custom` | |

**ACMESpec**: `email` (required), `dnsProvider` (required — a DNSProvider name),
`directoryURL` (optional, defaults to Let's Encrypt production), `keyType`
(`ecdsa` default | `rsa`), `challenge` (only `dns-01` is supported).

**CustomCertSpec**: `certFile`, `keyFile` — paths **relative to the cert store**
(absolute paths and `..` are rejected). These are file references, not inline PEM.

```yaml
# ACME wildcard
name: wildcard
type: acme
domains: ["*.example.com", example.com]
acme:
  email: admin@example.com
  dnsProvider: cloudflare
  keyType: ecdsa
```
```yaml
# Bring-your-own
name: internal
type: custom
domains: [internal.example.com]
custom: {certFile: internal.crt, keyFile: internal.key}
```

Certificates are selected at TLS time by SNI: an exact-domain match wins, else a
wildcard match on the parent domain. An ACME certificate that has not been issued
yet is simply skipped until the manager issues it.

---

## DNSProvider (`config/dns-providers/`)

Solves ACME `dns-01` challenges.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `provider` | string | yes | `cloudflare`. |
| `config` | map[string]Secret | yes | Provider-specific, secret-valued. |

```yaml
name: cloudflare
provider: cloudflare
config:
  apiToken: ${FILE:/run/secrets/cf_token}   # scope: Zone:DNS:Edit + Zone:Read
```

---

## IdentityProvider (`config/identity-providers/`)

| Field | Type | Notes |
|-------|------|-------|
| `type` | string | `oidc` \| `forward-auth` \| `auth-request`. |
| `oidc` / `forwardAuth` / `authRequest` | spec | The spec matching `type`. |
| `roleMapping` | RoleMapping | Map IdP groups → roles. |

**OIDCSpec**: `issuerURL` (req), `clientID` (req), `clientSecret` (Secret),
`scopes` (default `openid profile email groups`), `usePKCE` (default true),
`requireVerifiedEmail`, `trustIdPMFA`.

**ForwardAuthSpec**: `trustedProxies` (req, CIDRs allowed to assert identity),
`userHeader` (req), `emailHeader`, `nameHeader`, `groupsHeader`,
`groupsDelimiter` (default `,`), `amrHeader`.

> `trustedProxies` is also the per-host source of truth for **client-IP
> resolution**. `X-Forwarded-For` is honoured (for access-list, rate-limit, geo,
> and auth-request `allowFrom`) only for proxies a host actually trusts — the
> `trustedProxies` of the forward-auth IdPs *that host* references — not a global
> union across every IdP. A host with no forward-auth IdP in front therefore trusts
> no `XFF` and keys IP controls off the connection peer. If you IP-filter a host
> that sits behind a real proxy, give that host a forward-auth IdP declaring the
> proxy CIDR so its forwarded client IP is trusted.

**AuthRequestSpec**: `outpostURL` (req), `pathPrefix` (default
`/outpost.goauthentik.io`), `authPath` (default `<pathPrefix>/auth/nginx`),
`copyHeaders` (default the Authentik `X-authentik-*` set).

**RoleMapping**: `groupsClaim` (default `groups`), `adminGroups`, `userGroups`,
`defaultRole` (`""` = deny | `user` | `admin`), `allowDefaultAdmin` (bool; must be
`true` when `defaultRole` is `"admin"` — prevents silently granting admin to every
authenticated user when no group gating is configured).

```yaml
name: authentik-oidc
type: oidc
oidc:
  issuerURL: https://auth.example.com/application/o/gpm/
  clientID: gpm
  clientSecret: ${FILE:/run/secrets/oidc_secret}
roleMapping:
  adminGroups: [proxy-admins]      # no defaultRole -> anyone not in the group is denied
```

> The OIDC client reads claims from the **ID token**, so if your provider only
> emits groups via the userinfo endpoint you must configure it to include the
> groups claim in the ID token.

> **SSO session lifetime / offboarding.** For `type: oidc` hosts, gpm mints a
> signed `__Host-gpm_sso` session cookie with a **1-hour absolute TTL** (not a sliding
> window — it is not extended by activity). On expiry the next request re-runs the
> OIDC flow against the IdP, which is silent when the IdP session is still valid
> and re-checks group membership. This bounds the offboarding window: a user
> removed from a group or disabled at the IdP loses data-plane access within an
> hour, without gpm holding server-side session state. There is no per-user
> revocation, but there is a global one: `POST /api/sso/revoke` (admin-gated;
> also a button under Settings) moves a signed revocation watermark to "now",
> invalidating every outstanding SSO session on this instance immediately —
> users re-authenticate at the IdP on their next request. The watermark
> persists next to the signing key, so it survives restarts. Scope note: the
> watermark is read at startup, so a *second* gpm instance sharing the same
> signing key only picks a revocation up on its next restart. For a
> single-user cutoff, revoke at the IdP; access ends at that user's next
> hourly re-auth.

---

## AccessList (`config/access-lists/`)

| Field | Type | Notes |
|-------|------|-------|
| `satisfyAny` | bool | false = require both auth AND IP; true = either suffices. |
| `basicAuth` | []BasicAuthUser | `username` + `passwordHash` (bcrypt). |
| `rules` | []IPRule | Ordered `action` (`allow`/`deny`) + `cidr` (CIDR or bare IP). |
| `defaultAction` | string | `allow` \| `deny` (default `deny`). |

```yaml
name: internal-only
rules:
  - {action: allow, cidr: 10.0.0.0/8}
  - {action: allow, cidr: 192.168.0.0/16}
defaultAction: deny
```

---

## Middleware (`config/middlewares/`)

| `type` | Spec | Purpose |
|--------|------|---------|
| `auth` | AuthMiddleware | Require authentication. |
| `headers` | HeadersMiddleware | Add/remove request/response headers. |
| `guard` | GuardMiddleware | Conditionally deny requests. |
| `rate-limit` | RateLimitMiddleware | Per-host rate limiting. |
| `rewrite` | RewriteMiddleware | Exact-match request-path replacement (upstream-facing). |

**AuthMiddleware**: `identityProvider` (req), `mode` (`oidc`|`forward-auth`|
`auth-request`, defaults from the IdP type), `requiredRoles` (forbidden in
`auth-request` mode — the IdP application binding does authorization),
`allowFrom` (CIDRs that bypass auth; `auth-request` mode only — e.g. let a LAN
skip SSO).

In `oidc` mode gpm is itself the OIDC relying party for the host: an
unauthenticated request is redirected to the IdP, the reserved callback path
`/__gpm/oidc/callback` exchanges the code (PKCE + nonce) and sets a signed,
stateless SSO session cookie, and `requiredRoles` (via the IdP's `roleMapping`)
is enforced; with no role mapping or required roles the gate just requires a
valid login. Register `https://<host>/__gpm/oidc/callback` as a redirect URI on
the IdP. The signing key is auto-generated and persisted at
`<cert-dir>/sso_signing.key` (0600) on first use, so sessions survive restarts
without any operator action. Set `GPM_SSO_SIGNING_KEY` explicitly to supply your
own key (useful when rotating or sharing a key across instances).

**HeadersMiddleware**: `setRequest`, `setResponse` (maps), `removeRequest`,
`removeResponse` (lists).

**GuardMiddleware**: `triggers` (≥1; each has `paths`, `methods`, `queryEquals`
and matches when all set fields match), `allowFrom` (exempt CIDRs), `denyStatus`
(default 403).

**RewriteMiddleware**: `replacePath` (a map of exact request paths to their
replacements). When the incoming request path equals a key **exactly** (no
prefix or pattern matching), the path is replaced by the mapped value before
the request is proxied. Both key and value must be absolute paths (start with
`/`), and a key may not map to itself. The rewrite is **internal**: it mutates
the proxied path in place, preserving the method and body - it is never an HTTP
redirect (the client sees no 3xx, and a `POST` body is forwarded unchanged). It
is exact-match only by design, sidestepping the path-confusion / ReDoS classes
pattern rewrites invite. It runs **innermost** (closest to the upstream), so
auth, guards and access lists all evaluate the ORIGINAL client path; a rewrite
can never move a request past a path-scoped security control.

**RateLimitMiddleware**: a rate expressed one of two ways (exactly one must be
set), plus `burst` (default `ceil(requests)`), `allowFrom` (CIDRs exempt
from rate limiting entirely - no token consumed, no 429), and `blockFor`
(optional):

- `requests` + `window`: "N requests per window", where `window` is a Go
  duration string (`"1s"`, `"10s"`, `"1m"`, `"1h"`, ...). Use this for limits
  that don't reduce cleanly to a per-second rate, e.g. `100` requests per
  `1m`, or `5` per `1h`.
- `requestsPerSecond` (req, >0): legacy shorthand, equivalent to
  `requests: <value>` with `window: "1s"`. Kept for backward compatibility;
  new configs should prefer `requests`/`window`.

Enforced as a per-host, per-client-IP token bucket (capacity = `burst`, refill
= `requests`/`window` in tokens/sec). Over-limit requests get `429 Too Many
Requests` with a `Retry-After` header computed from the refill rate (a slow
limit like `5` per `1h` can report a Retry-After of several minutes); the
request is not proxied. The client IP is resolved the same XFF-aware way as
access lists; a request whose client IP cannot be resolved falls back to a
single shared bucket (fail-safe, never unlimited, and never matches
`allowFrom`). The middleware sits **outermost** in the chain (evaluated first)
so a flood is shed before it can drive an auth subrequest or any other
per-request work: rate-limit → access-list → auth → guard → headers → rewrite →
upstream.

`blockFor` (a Go duration string, e.g. `"30s"`, `"5m"`) adds an extra,
harsher penalty on top of the token bucket: the first request that exceeds
the limit blocks that client for `blockFor`, and every request from it is
rejected (`429`, `Retry-After` counting down to the end of the block) for the
whole period - independent of token refill, so a client that merely pauses
and resumes cannot slip back through once tokens would otherwise have
refilled. The block is **fixed, not sliding**: repeat requests during the
block do not push it back out, so it always expires exactly `blockFor` after
the trip that started it. Once it expires, ordinary token-bucket rules
resume (the bucket has been refilling in the background the whole time, up
to `burst`, so the client gets a normal allotment, not an instant re-burst).
Omit `blockFor` (the default) for today's behavior: only the token bucket
governs, and a client that outwaits the refill rate is let back through
immediately.

```yaml
# Require SSO, but let the LAN through without it
name: require-sso
type: auth
auth:
  identityProvider: authentik-outpost
  mode: auth-request
  allowFrom: [10.0.0.0/8]
```
```yaml
# Block POSTs to a login path except from the LAN (break-glass guard)
name: login-lan-only
type: guard
guard:
  triggers:
    - {paths: [/login], methods: [POST]}
  allowFrom: [10.0.0.0/8]
```
```yaml
# Rate-limit the host to 100 requests/minute, but let the LAN through uncapped
name: api-rate-limit
type: rate-limit
rateLimit:
  requests: 100
  window: 1m
  burst: 150
  allowFrom: [10.0.0.0/8]
```
```yaml
# Legacy shorthand (requests per second); equivalent to requests: 10, window: 1s
name: api-rate-limit-legacy
type: rate-limit
rateLimit:
  requestsPerSecond: 10
  burst: 20
  allowFrom: [10.0.0.0/8]
```
```yaml
# Trip the limit and the client is locked out for 5 minutes, regardless of
# token refill - not just throttled back to the steady-state rate.
name: api-rate-limit-blocked
type: rate-limit
rateLimit:
  requests: 100
  window: 1m
  blockFor: 5m
```
```yaml
# Add the trailing slash a client strips off Authentik's token endpoint, so the
# request reaches Django as /application/o/token/ (POST + body preserved) instead
# of getting a 405. Exact-match, upstream-facing only.
name: authentik-token-slash
type: rewrite
rewrite:
  replacePath:
    /application/o/token: /application/o/token/
```

### Middleware ordering

Middlewares are applied in a fixed order per request regardless of the order you
list them: **rate-limit → access-list → auth → guard → headers → rewrite →
upstream**. Rate limiting is outermost (evaluated first, so floods are shed before
any work); the access-list is evaluated ahead of auth, so a denied IP never
reaches the IdP; path rewrites are innermost (closest to the backend), so every
security tier above still sees the original client path. Host-wide middlewares run
before any location-scoped ones.

---

## APIToken (`config/api-tokens/`)

A non-interactive credential for the REST API: a bearer secret with an explicit
scope list, used by scripts and CI instead of an admin session cookie.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `scopes` | []string | yes | What this token may do (below). At least one. |
| `expiresAt` | RFC3339 time | no | Token stops authenticating after this instant. Unset never expires. |
| `tokenHash` | string | **server-owned** | Lowercase SHA-256 hex digest of the secret. Written by the server; a client-supplied value is discarded, and it is **never returned by any endpoint** (`json:"-"`) — a digest is offline-crackable. It exists only in the YAML at rest. |
| `disabled` | bool | no | Keep the token in config without it authenticating. |

**The secret is never stored.** It is generated server-side (`gpm_` + 32 random
bytes, base64url) and returned **exactly once**, as the `token` field in the
response to the `PUT` that created it. Only its digest is committed, in a plain
string field rather than a `Secret` — a digest is not a value to resolve from the
environment, and the store refuses literal `Secret` values outright.

```
PUT  /api/api-tokens/ci            # create; response carries "token" once
PUT  /api/api-tokens/ci            # ordinary edit; digest carried forward, no new secret
PUT  /api/api-tokens/ci?rotate=1   # rotate; new secret returned once, old one dies
```

Use it as a bearer credential:

```
curl -H 'Authorization: Bearer gpm_...' https://gpm.example.com/api/proxy-hosts
```

### Scopes

A scope is `<subject>:read`, `<subject>:write`, `*:read`, `*:write`, or `admin`.

- **write implies read** on the same subject; read never implies write.
- `*` matches any subject, but a concrete subject never satisfies a `*`
  requirement — a whole-config read (`/api/config`, `/api/history`, `/api/logs`,
  `/api/upstream-health`) genuinely needs `*:read`.
- `admin` satisfies everything, and is the **only** scope that reaches:
  - `/api/api-tokens` (a token that could mint tokens could widen itself),
  - **`PUT /api/settings`** (see below),
  - `GET /api/backup` — the archive is the raw on-disk YAML, so unlike the JSON
    reads it carries the api-tokens' stored digests,
  - `POST /api/restore`, `POST /api/revert`, `POST /api/sso/revoke`,
  - `/debug/pprof/*` when profiling is enabled — a heap dump and the process
    command line contain resolved backend credentials in cleartext, and every
    token principal is admin-*role* by construction, so the role gate alone is
    not a boundary there.

**`settings:write` does not reach `PUT /api/settings`.** Writing settings is
admin-equivalent and takes the `admin` scope: a settings write can point
`dnsSync.pihole.url` or a webhook at an attacker-controlled URL while supplying
`${ENV:SOME_TOKEN}` as its credential, and the write itself triggers the
reconcile/dispatch that resolves that env var and sends it offsite — and it can
rewrite `adminAuth` outright. `settings:read` still grants `GET /api/settings`
(reading resolves nothing). `settings:write` remains a valid scope string for
forward compatibility but grants nothing beyond `settings:read` today; the UI
greys the box out rather than offering a grant that does nothing.

**Reverting an `APIToken` is refused** — scoped (`POST
/api/api-tokens/{name}/revert`) and whole-config (`POST /api/revert`, which
preserves the `api-tokens` directory across the restore). Restoring an older
token file would restore an older `tokenHash` and silently revive a secret the
operator rotated away, so rotation would stop meaning revocation. Create a
replacement token instead.

Valid subjects are the REST resource plurals — `proxy-hosts`, `redirect-hosts`,
`stream-hosts`, `dead-hosts`, `certificates`, `client-cas`, `dns-providers`,
`identity-providers`, `upstream-groups`, `access-lists`, `middlewares`,
`api-tokens` — plus three pseudo-resources for non-CRUD endpoint groups:
`settings`, `dns-sync` and `ingress-discovery`. An unknown subject or verb is rejected at write time.
`GET /api/capabilities` and `GET /api/me` need no scope: any authenticated caller
may ask what the instance supports.

Scopes constrain **API tokens only**. An admin session keeps full access exactly
as before.

```yaml
name: ci-deploy
scopes:
  - proxy-hosts:write
  - certificates:read
expiresAt: 2027-01-01T00:00:00Z
```

**Auth mechanics.** A request carrying `Authorization: Bearer gpm_...` is
resolved as a token *before* the cookie path and never falls through to it: a
presented-but-invalid token is a `401`, not an invitation to try a session cookie
riding along on the same request. Any other bearer scheme is left alone and the
cookie path runs as usual. Token principals are **CSRF-exempt** — the
double-submit check defends against a browser attaching ambient credentials, and
a bearer token is never attached automatically. Successful and failed token
authentications are logged (token name on success; never any secret material).
Last-use is tracked **in memory only** and surfaced as `lastUsed` on
`GET /api/api-tokens`; the config store is git-backed, so persisting a timestamp
per request would be a commit flood. It resets on restart.

---

## Kubernetes Ingress annotations

Shipped: see
[IngressDiscoverySettings](#ingressdiscoverysettings-settingsingressdiscovery)
above for the annotation contract, the derived-object rules, and the ownership
and freeze behaviour.
