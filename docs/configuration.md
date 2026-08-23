# Configuration reference

go-proxy-manager is configured by a set of typed YAML objects in a git-backed
directory (default `/data/config`). You can edit them through the web UI / REST
API, or write the files directly and let the daemon load them on start/reload.

## Layout

```
config/
  settings.yaml            # singleton app settings
  dns-ledger.yaml          # singleton DNS record-ownership ledger (reconciler-written)
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
| `labels` | map | no | Arbitrary key/value metadata. **`gpm.rake.pro/managed-by` is reserved** — see below. (The exact key follows `ingressDiscovery.annotationPrefix`; `gpm.rake.pro` is the default and what every example below uses.) |
| `tags` | []string | no | Flat, free-form labels for grouping/filtering. On the Proxy Hosts list they render as chips and are matched by the filter box. |
| `disabled` | bool | no | Keep the object in config but exclude it from the running data plane. |

> **`gpm.rake.pro/managed-by` is a reserved label — do not set it by hand.** It
> marks an object as owned by an automated reconciler. Adding
> `gpm.rake.pro/managed-by: ingress-discovery` to a proxy host you wrote yourself
> hands it to Ingress discovery, which will **delete it** on the next poll,
> because no annotated `Ingress` derives it. Removing the label is the supported
> way to adopt a discovered host permanently; adding it is never the way to give
> one away.

> **A managed host is not editable by hand - every edit besides `disabled` is
> reverted on the next poll.** Discovery derives the whole object from the
> template and the `Ingress` and writes it back whenever it differs from what is
> stored, so an edited `displayName`, added `tags`, `timeouts`, `locations` or
> `robotsNoIndex` all survive at most until the next reconcile (default 60s).
>
> **`disabled: true` is the exception - it is operator-owned state.** Discovery
> honours an operator-set `disabled` and never clears it itself: hand-disabling a
> managed host (in the UI or by editing
> `config/proxy-hosts/<name>.yaml`) survives every subsequent poll, keeps the
> object out of the running data plane, and withdraws its DNS records exactly
> like disabling a hand-written host does. Editing the Ingress cannot undo this -
> a cluster user has no way to re-enable a host you disabled. The one case where
> a `disabled: true` a poll wrote itself IS cleared automatically is the
> fail-closed hold below (an unresolvable profile): that disable is discovery's
> own, and the very next reconcile that resolves the profile again lifts it. The
> label `gpm.rake.pro/disabled-by: ingress-discovery` on the stored object is how
> the two are told apart - never set or remove it by hand; it exists only for
> discovery to recognise a hold it placed itself.
>
> With that, there is no longer only an "emergency" off-switch - disabling in the
> UI now works - but the other two routes remain useful:
>
> - **Remove `gpm.rake.pro/managed` from the `Ingress`** (or delete the
>   `Ingress`). Discovery stops deriving the host and deletes it on the next
>   successful reconcile. This is the clean route for taking a service out of
>   discovery for good, but it needs cluster access.
> - **Remove the `gpm.rake.pro/managed-by` label from the proxy host.** The
>   object becomes operator-authored, discovery refuses to touch it ever again,
>   and you can then edit it freely. This still needs no cluster access, and is
>   still **permanent**: the corresponding `Ingress` is skipped with a warning
>   from then on, and putting the host back under discovery means deleting it and
>   letting the next poll recreate it.
>
> Preview what a reconcile would do to a specific host with `GET
> /api/ingress-discovery/plan` (or **Preview changes** in the settings UI) before
> disabling by hand, if in doubt.

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
| `proxyProtocol` | ProxyProtocolSettings | Optional inbound PROXY protocol (below). |
| `ingressDiscovery` | IngressDiscoverySettings | Optional Kubernetes Ingress discovery (below). |
| `errorPages` | ErrorPagesConfig | Default custom error pages for every host (below). A [ProxyHost](#proxyhost-configproxy-hosts)'s own `errorPages` overrides this. Zero value keeps gpm's built-in plain-text error output. |

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
    robotsNoIndex: true             # same field a hand-written host has
    timeouts: { connectSeconds: 5, readSeconds: 60 }
    tags: [cluster]
    defaultDNS: { lanDirect: true }
  profiles:                         # optional named chains, selected per Ingress
    public-ratelimited:
      upstream: { scheme: http, host: 10.0.0.40, port: 80 }
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
      middlewares: [rate-limit]     # and no access list: public on purpose
```

### ProxyProtocolSettings (`settings.proxyProtocol`)

Accepts the HAProxy **PROXY protocol** (v1 text and v2 binary) on the data-plane
`:80`/`:443` listeners **and on every TCP stream listener**, so gpm behind an L4
load balancer (HAProxy, an AWS NLB with proxy protocol enabled, a Kubernetes
`Service` with `externalTrafficPolicy` behind an LB that sends it) sees the real
client address instead of the balancer's.

The parsed source address replaces the connection's `RemoteAddr`, which is the
single value every IP-based control derives from — so access lists, geo rules,
guards, rate limits, the basic-auth lockout, `X-Forwarded-For`, the access log,
the OIDC gate and the metrics labels all see the real client with no per-feature
wiring.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `enabled` | bool | no | Default false — listeners are untouched. |
| `trustedCIDRs` | []string | **yes when enabled** | CIDRs or bare IPs of the balancers whose header is believed. There is no "trust everyone" mode. |
| `timeout` | duration | no | Deadline for reading a complete header from a trusted peer. Default `5s`, maximum `1m`. |

**The header is an unauthenticated claim.** Anyone who can open the port can
assert any source address, so gpm parses it **only** when the TCP peer is inside
`trustedCIDRs`. From any other peer the bytes are treated as ordinary payload and
the peer address stands (logged once per peer at warn) — otherwise enabling this
would hand every client a free source-IP spoof past every rule above. A malformed
header from a trusted peer closes the connection; a stalled one is cut at
`timeout`. A trusted peer that sends **no** header (the usual load-balancer TCP
health check) is served normally with its own address as the client IP.

v2 TLVs are consumed and ignored. `PROXY UNKNOWN` (v1), the v2 `LOCAL` command and
the v2 `AF_UNSPEC` family assert no address, so the real peer stands. There is no
UDP support: a UDP stream listener is unaffected by this setting.

> Turning this on when your balancer does **not** send the header will not break
> HTTP (the request bytes are simply not a PROXY header), but a *server-first*
> TCP stream backend fronted by a trusted peer that never speaks first will stall
> for `timeout` before the connection is closed. Enable it on the balancer first.

```yaml
proxyProtocol:
  enabled: true
  trustedCIDRs: [10.0.0.0/8, "2001:db8::/64"]
  timeout: 5s
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
| `pihole.apexTarget` | string | CNAME target every managed record points at. Required when enabled. **Not an ownership marker** — see below. |
| `cloudflare.enabled` | bool | Turn on public zone reconciliation. |
| `cloudflare.dnsProviderRef` | string | Name of an existing [DNSProvider](#dnsprovider-configdns-providers) whose `config.apiToken` is reused. Required when enabled. |
| `cloudflare.zoneName` | string | Zone the records live in, e.g. `example.com`. Required when enabled. |
| `cloudflare.apexTarget` | string | CNAME content every managed record points at. Required when enabled. |
| `cloudflare.proxied` | bool | Cloudflare orange-cloud flag on created records. Default `false` (DNS only). |

**Ownership — what gpm will and will not delete.** Reconcile is *full-state*: the
desired set is recomputed from the whole config on every run, so a record deleted
out of band is recreated and a host removed while gpm was down is still cleaned
up. Deletion, however, is limited to records **gpm created itself**, recorded
explicitly in the ownership ledger at `config/dns-ledger.yaml`:

```yaml
# config/dns-ledger.yaml - written by the reconciler, committed like everything else
schemaVersion: 1
pihole:
  - domain: app.example.com
    target: edge.example.com
    adopted: false      # gpm created this record, so gpm may delete it
cloudflare:
  - domain: www.example.com
    target: edge.example.com
    adopted: true       # the record was already there; gpm only claimed it
```

`adopted` records **how** the claim was acquired, and it is what decides whether
the record can ever be deleted. An entry with no `adopted` key at all (a ledger
written before this field existed) is read as **adopted**, deliberately: it is the
only reading of a missing field that cannot destroy a record on upgrade.

Per desired record, on every run:

| Backend state | What gpm does |
|---------------|---------------|
| absent | **create**, and record ownership (`adopted: false`) |
| present, right target, **not** in the ledger | **adopt** — record ownership (`adopted: true`), do not recreate (logged at info) |
| present, right target, already owned | nothing |
| **created by gpm**, still holding the target gpm wrote, but `apexTarget` has since changed | **retarget** - replace it and update the ledger (the replacement is again a record gpm created) |
| **adopted**, and `apexTarget` has since changed | **released, not retargeted** - the claim is dropped and the record left exactly where it is (logged at warn, counted as a skip) |
| present, different target, not owned | **skip and warn** — never shadowed, never replaced |
| in the ledger as **created by gpm**, no longer desired | **delete**, and drop from the ledger (logged at warn, with the ledger revision that authorised it) |
| in the ledger as **adopted**, no longer desired | **released, not deleted** — the claim is dropped and the record left exactly where it is (logged at warn) |
| **not in the ledger** | **never deleted**, whatever it points at |

**A record gpm adopted is never deleted *or* retargeted.** Adoption claims a
record somebody else made; it is not, and must never become, permission to
destroy it. So turning `dns.lanDirect` on for a name an operator had
hand-written, and then turning it off again, leaves their record untouched - gpm
simply stops managing it. A retarget is a delete followed by a create, so the
same rule applies when `apexTarget` moves: an adopted record is **released**
rather than re-pointed, both because re-pointing would destroy the operator's
record and because the replacement would be recorded as gpm-created, leaving a
later host removal free to delete the name outright. To move an adopted name to
a new apex, either delete the record and let the next reconcile create it (gpm
then owns it, and may later remove it), or re-point it yourself and let gpm
re-adopt it. The flip side is that gpm will not clean up an
adopted record for you: once released it is yours to remove by hand.

`apexTarget` is *not* an ownership marker. It says where managed records point,
nothing more. A hand-written CNAME aimed at the same host is adopted only if a
proxy host asks for that exact name, and is otherwise left completely alone.

**Cloudflare keeps a second marker.** Every record gpm creates there also carries
the comment `managed-by:gpm`, and deletion needs **both** the ledger entry and
the comment (re-checked inside the delete call itself). Adoption likewise only
claims records that already carry the comment; a record with the right content
but no comment is somebody else's and is skipped. Pi-hole/dnsmasq CNAMEs have no
comment field at all, which is exactly why the ledger exists.

**Enabling a backend for the first time is safe, and previewable.** With an empty
ledger gpm owns nothing, so it can only create and adopt: records matching the
desired set are adopted, and every other record on the backend is left untouched
and counted in the `untouched` field of the status. Run
`GET /api/dns-sync/plan` (or **Preview changes** in the settings UI) first — it
reads the backends and the ledger and reports exactly what a reconcile would
create, adopt, retarget and delete, without writing anything.

> **Do not hand-edit `config/dns-ledger.yaml`.** It is what authorises a DNS
> deletion. An entry with a missing domain or target, or a duplicate domain
> (compared case-insensitively, as the reconciler indexes it), is rejected at load
> and stops the reconcile rather than being acted on. To make gpm forget a record,
> delete the entry — that disowns it, it is not a deletion of the record itself.
> It is reverted along with the rest of the config by `POST /api/restore` / a
> whole-tree revert, which is deliberate: rolling the config back to before a host
> existed also rolls back gpm's claim on the record that host published.

> **Caveat: a revert can restore an ownership claim that is no longer true.** The
> ledger reverts with the tree, and history does not know what happened at the DNS
> backend in the meantime. If gpm created `x.example.com`, later deleted it, and an
> operator then recreated that name by hand, a revert to a commit from before the
> deletion restores gpm's claim on it — and the next reconcile, finding the name
> unwanted and the record matching what the claim says gpm left there, deletes the
> operator's record. The `adopted` rule above does **not** cover this case: the
> restored entry is one gpm genuinely created at the time it was written.
>
> Two things limit the damage. A reconcile whose ledger write is refused because
> the repo moved under it (which is what a concurrent revert looks like) re-reads
> and rewrites *without* the claims the revert withdrew, so a revert cannot be
> silently undone by a run already in flight. And every deletion is logged at
> **warn** with the ledger revision that authorised it (`ledgerRev`), so a record
> removed on the strength of a stale claim is identifiable after the fact. After
> reverting a config that ever contained DNS-synced hosts, run
> `GET /api/dns-sync/plan` before letting a reconcile proceed.

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
slow backend. `GET /api/dns-sync/status` reports the last run per backend
(`desired`, `managed`, `created`, `adopted`, `retargeted`, `deleted`, `skipped`,
`untouched`), and `GET /api/dns-sync/plan` returns the same decisions as a dry
run without touching anything (`409` while a reconcile is in flight, for the same
reason). Both take `dns-sync:read`. A Pi-hole `403` is
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
| `template.robotsNoIndex` | bool | Applied to every derived host: sends `X-Robots-Tag: noindex, nofollow`, exactly as on a hand-written host. Omit it and derived hosts carry no such header - do **not** reach for a `headers` middleware instead. |
| `template.timeouts` | HostTimeouts | Upstream `connectSeconds`/`readSeconds` override, applied to every derived host and validated by the **same rules as a proxy host's** (0-3600). Unset means the shared pooled transport. |
| `template.middlewares` | []string | Applied to every derived host, in order. |
| `template.accessLists` | []string | Applied to every derived host. |
| `template.tags` | []string | Free-form grouping labels applied to every derived host, for filtering in the host list. No data-plane effect. |
| `template.defaultDNS` | DNSSyncPolicy | The `dns` policy a derived host gets when the corresponding annotation is absent. Each flag is overridden individually by its annotation. |
| `template.allowedDomainSuffixes` | []string | Optional. **Narrows** the top-level `allowedDomainSuffixes` for hosts derived from the template. Must be a **subset** of the global list (checked at settings-write time); empty means no narrowing. |

A derived host's `displayName` is always `<namespace>/<name>` (where the Ingress
came from) and is not templatable. `locations` are **deliberately not** a
template field - see [below](#what-a-derived-host-cannot-express).
| `profiles` | map[string]→ same shape as `template` (including its own `allowedDomainSuffixes`) | Additional named chains an Ingress may **select by name** (below). Each key is a profile name (`ValidateName` shape); `template` is reserved for the default block. |
| `profileRules` | []IngressProfileRule | Optional, ordered. Operator-side profile selection - see [below](#operator-side-profile-selection-profilerules). |
| `profileSelection` | `"annotation-or-rules"` \| `"rules-only"` | Empty means `"annotation-or-rules"` (today's behaviour: try `profileRules` first, then the annotation). `"rules-only"` never reads `gpm.rake.pro/profile` at all. |
| `annotationPrefix` | string | Replaces `gpm.rake.pro` as the prefix for every annotation below and for the `managed-by`/`disabled-by` labels gpm writes on derived hosts. Empty (the default) keeps every existing deployment's keys unchanged. Must be a DNS-subdomain-shaped prefix: lowercase alphanumerics, `-` and `.`, no leading/trailing dot, no slash, at most 253 characters. See [Changing the annotation prefix](#changing-the-annotation-prefix) below - **changing this does not relabel existing hosts by itself.** |
| `annotationPrefixMigrate` | bool | Opt-in escape hatch for the refusal `annotationPrefix` triggers when existing hosts are still labelled under the old prefix - see below. Does not relabel anything itself; only lifts the refusal so the *next reconcile* can. |

**Opt-in annotations** (on the `Ingress`, never on gpm's side; prefixed with `annotationPrefix`, default `gpm.rake.pro`):

| Annotation | Value | Meaning |
|------------|-------|---------|
| `gpm.rake.pro/managed` | `"true"` | Opt this Ingress into discovery. Absent or any other value means gpm ignores it entirely. There is no opt-out mode and no namespace sweep. |
| `gpm.rake.pro/profile` | a `profiles` key | Select one of the operator-defined profiles. Absent (or empty) uses the default `template`. An **undefined** name skips the Ingress. |
| `gpm.rake.pro/lan-direct` | `"true"` \| `"false"` | Sets `dns.lanDirect` on the derived host, overriding the resolved profile's `defaultDNS`. |
| `gpm.rake.pro/public-cname` | `"true"` \| `"false"` | Sets `dns.publicCname` on the derived host. |

#### Changing the annotation prefix

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
`websocketsUpgrade`, `robotsNoIndex`, range-checked `timeouts`, `tags`,
`defaultDNS`). An `Ingress` picks one with `gpm.rake.pro/profile`.

**The annotation carries a name and nothing else — that is the security model.**
An Ingress author is untrusted: in a shared cluster a tenant may be able to
create or edit an `Ingress`, and gpm sits at the edge in front of everything.
There is deliberately **no** annotation that lets an Ingress name a middleware,
an access list, a certificate or an upstream, because such an annotation is a
self-service privilege grant — `access-lists: ""` on your own namespace's Ingress
would remove `home-vpn` from a hostname at the edge. Every profile is written by
you, here, in the config repo; a manifest chooses among them and can never invent
one, so a derived host is always one of a set you authored and can audit.

**The residual risk, stated plainly: every profile is selectable by every
annotating Ingress — define only profiles you are willing for any cluster tenant
to choose.** Selection is not restricted per namespace, per Ingress or per
tenant. A tenant who can annotate an Ingress in their own namespace can pick the
*most permissive* profile you defined, so a profile with no access list (like
`public-ratelimited` in the example below) is effectively an open door any tenant
may walk their own hostname through. That is a real capability — bounded by a set
you control, not unbounded, but not nothing. If it is too coarse for your cluster,
the escalation path is an operator-side selector table (deferred, see
[design/ingress-discovery.md §5a](design/ingress-discovery.md), which also holds
the full threat model).

**Resolution rules:**

| `gpm.rake.pro/profile` | Result |
|---|---|
| absent | the default `template` block |
| present but empty or whitespace-only | treated as absent → the default `template` |
| exact match on a `profiles` key (surrounding whitespace trimmed) | that profile, **verbatim** |
| anything else | the Ingress is **skipped**, with the requested name in the status `reason` and a `warn` log |

An undefined profile is **never** downgraded to the default and never adopted
with a partial chain — falling back is exactly the silent regression profiles
exist to prevent. Matching is exact: no prefix match, no case folding, no
nearest-neighbour guess.

**An unresolvable profile fails closed on an existing host.** If the Ingress
already has a derived host, that host is not left alone: it is updated with
`disabled: true`. Nothing is deleted, nothing is rewritten, and re-adding the
profile re-enables it on the very next reconcile. This is what makes **revocation**
work. Leaving it untouched would mean a tenant could pin a chain you have just
tightened, renamed or retired — by pointing the annotation at a name that does
not exist — and the host would go on serving the revoked chain indefinitely.
Retiring a profile (or clearing the profile rows in the UI) disables the hosts
derived from it for the same reason.

Every *other* derive failure — a malformed hostname, an unusable derived name —
still **freezes** the existing host instead: your policy has not changed there,
the host on disk is the last good rendering of it, and failing closed would let
any tenant take their own service offline with a one-character manifest edit.
A profile is applied **verbatim**, never merged with the template, so the
default's access list can never leak onto a profile that is public on purpose.

Every profile validates at **settings-write time**, not at reconcile time — an
invalid one is rejected by `PUT /api/settings` where you see it, rather than
surfacing later as a skipped host. That includes **referential** validation: the
`certificateRef`, `upstreamGroupRef`, `middlewares`, `accessLists` and
`tls.clientAuth.caRef` of the template and of every profile are cross-checked
against the objects that actually exist. A dangling name there is not a
localised problem later — it is stamped onto every derived host, and the store
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

#### Operator-side profile selection (`profileRules`)

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

#### Per-profile `allowedDomainSuffixes`

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

#### What a derived host cannot express

A derived host is a full `ProxyHost` with two deliberate exceptions:

- **`locations`** — path-scoped overrides with their own upstream and chain.
  Discovery forwards **everything** to the cluster ingress controller by vhost,
  and the controller does the path routing itself from the same `Ingress`, so a
  template-wide location list would be applied to every derived host regardless
  of the service. The useful version is per-service, and reading paths/upstreams
  /chains out of a cluster manifest is exactly the self-service privilege grant
  the annotation model forbids. If you need a per-path chain on a discovered
  service, publish a second hostname (a second `Ingress`) and annotate it, or
  hand-write the host and leave it out of discovery — an unlabelled host is never
  touched. See [design/ingress-discovery.md §5](design/ingress-discovery.md).
- **`displayName`** — always the source `<namespace>/<name>`, so the host list
  says where a host came from. Not templatable, because one display name shared
  by every derived host would carry no information.

Everything else a hand-written proxy host can set — `robotsNoIndex`, `timeouts`,
`tags`, TLS/mTLS, middlewares, access lists, websockets, DNS policy — is a
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
    tls: { certificateRef: wildcard, forceSSL: true, http2: true }
    accessLists: [home-vpn]
    robotsNoIndex: true          # internal by default: keep it out of search
    tags: [cluster]
    defaultDNS: { lanDirect: true }
  profiles:
    # Public on purpose - rate-limited, and NO access list.
    public-ratelimited:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
      middlewares: [rate-limit]
      tags: [cluster, public]
      defaultDNS: { lanDirect: true, publicCname: true }
    # SSO-gated and VPN-restricted, and slow to answer on a cold start.
    sso-internal:
      upstreamGroupRef: k8s-nodes
      tls: { certificateRef: wildcard, forceSSL: true, http2: true }
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

**What a derived host cannot express.** Cutting a hand-written host over to
discovery - annotate the `Ingress`, let the derived host appear, delete the
hand-written one - silently drops everything the template has no field for. A
derived host carries only `upstream`/`upstreamGroupRef`, `tls`,
`websocketsUpgrade`, `middlewares`, `accessLists` and the `dns` policy. It cannot
carry:

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
take a host offline — the one exception being an unresolvable *profile*, which
disables the existing host rather than freezing it (see "Resolution rules"). An
*empty successful* list is a different thing entirely: it
is a legitimate delete-all, applied and logged per host at WARN.

**Writes land as one commit per reconcile** — every create, update and delete from
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
`lastSuccess` — separate on purpose, so a frozen state cannot look fresh — and a
per-host list of actions (`created` / `updated` / `unchanged` / `deleted` /
`skipped`, with the resolved `profile` and a reason for each skip). The cluster-side RBAC to apply is
[`deploy/k8s-ingress-discovery-rbac.yaml`](../deploy/k8s-ingress-discovery-rbac.yaml).

### ErrorPagesConfig (`settings.errorPages` / `proxyHost.errorPages`)

Custom HTML pages for errors **gpm itself generates**: upstream unreachable
(502 connect/handshake failure, 504 a timeout awaiting the upstream), access
denied (403 from an access list, a guard middleware, or a geo rule), rate
limited (429), a dangling middleware/access-list reference (503), and a dead
host. The upstream's own error response (its own 500, its own 404 page) is left
untouched **unless** its status is also listed in `interceptUpstream`. A status
with no matching template — and unconfigured settings/host entirely — falls
back to gpm's historical plain-text output, unchanged from before this feature.

| Field | Type | Notes |
|-------|------|-------|
| `dir` | string | Directory of `html/template` files named `<status>.html` (e.g. `502.html`), plus an optional `default.html` fallback used when no status-specific template matches. Relative to the managed cert store — confined exactly like a custom certificate's files: no absolute path, no `..`. |
| `inline` | map[string]string | Status code (as a string, e.g. `"502"`) — or the literal `"default"` — mapped directly to `html/template` source, for a handful of pages an operator would rather keep in config than mount a directory. |
| `interceptUpstream` | []int | Status codes (4xx/5xx) for which the **upstream's own** error response body is also replaced by the configured page. Default: only gpm-generated errors are replaced, never the upstream's own. |

Templates execute with `{{.Status}}` (int), `{{.StatusText}}` (e.g. `"Bad
Gateway"`), `{{.Host}}` (the matched ProxyHost name), and `{{.RequestID}}` (the
`X-GPM-Request-Id` value when `-debug-headers`/`GPM_DEBUG_HEADERS=1` is on,
empty otherwise) — `html/template`, so all four are contextually escaped. A
**ProxyHost's own `errorPages`** is resolved first for a given status (falling
back to its own `default` template, then to the settings-level pages) so a host
override always wins; a host with no override of its own uses the
settings-level pages outright. Templates are parsed at config reload — a parse
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
| `compression` | Compression | no | Gzip response compression (below). Zero value (`enabled: false`) is today's behaviour: no compression. |
| `errorPages` | ErrorPagesConfig | no | Overrides [`settings.errorPages`](#errorpagesconfig-settingserrorpages--proxyhosterrorpages) for this host's own gpm-generated errors. Unset uses the settings-level pages, if any. |

**Upstream**: `scheme` (`http`|`https`), `host`, `port` (1–65535) — all required.

**TLSSettings**: `certificateRef` (a Certificate name), `forceSSL` (redirect
HTTP→HTTPS), `http2`, `hsts` (`enabled`, `maxAge` — seconds, default one year when
unset, `includeSubdomains`, `preload`), `minTLSVersion` (`"1.2"` default | `"1.3"`),
`clientAuth` (mTLS — below).
When `hsts.enabled` is set, the data plane emits `Strict-Transport-Security` on
HTTPS responses for the host (never over plain HTTP).

**ClientAuth** (`tls.clientAuth`) opts the host into mTLS: client certificates
are verified at the TLS handshake against a [ClientCA](#clientca-configclient-cas).

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `caRef` | string | yes | ClientCA name. Must exist and be enabled; the host also requires `forceSSL: true`. |
| `mode` | string | no | `require` (default) rejects the handshake without a valid certificate; `optional` verifies a presented certificate but lets certless requests through (mTLS as a fallback beside SSO). |
| `identityHeaders` | ClientCertHeaders | no | Forward the verified certificate's identity upstream. Unset = forward nothing. |

**ClientCertHeaders** (`tls.clientAuth.identityHeaders`):

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `subjectHeader` | string | `X-Client-Cert-Subject` | Header carrying the certificate subject in RFC 2253 form (`CN=ops,O=Corp`). Must be a valid header name and may not be an `X-Forwarded-*` header. |
| `san` | bool | false | Send `X-Client-Cert-SAN`: the subject alternative names (DNS, email, IP, URI), comma-separated. |
| `serial` | bool | false | Send `X-Client-Cert-Serial`: the serial number in lower-case hex. |
| `fingerprint` | bool | false | Send `X-Client-Cert-Fingerprint`: the SHA-256 digest of the DER certificate, lower-case hex. |

These headers ride the existing identity-trust model: all four default names are
in the **baseline identity denylist**, so they are stripped from every request
whose peer is not a proxy the host trusts — whether or not the host enables
passthrough — and a custom `subjectHeader` is added to that host's own strip set.
gpm sets them *after* the strip, only from a certificate the handshake actually
**verified**; in `optional` mode a certless request reaches the upstream with no
identity headers at all.

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
backend disabled publishes nothing, and the UI says so inline while leaving the
toggle usable — setting the flag before the backend exists is legitimate staging
(the host is the declaration; the syncer publishes once it is wired), so it is
not refused.

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

**Compression** (`compression`) gzip-compresses eligible response bodies from
this host's upstream, using only the standard library's `compress/gzip` (via a
pooled `sync.Pool` of writers).

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `enabled` | bool | false | Off means byte-for-byte today's behaviour: no compression. |
| `minBytes` | int | 1024 | Smallest response body gzip bothers with. The body is buffered up to this size before the compress/pass-through decision is made, so a response that never reaches it is sent uncompressed. |
| `types` | []string | text/html, text/plain, text/css, text/csv, application/json, application/javascript, text/javascript, application/xml, text/xml, image/svg+xml | Response `Content-Type`s (media type only; `charset` etc. ignored) eligible for compression. |

Compression honours the client's `Accept-Encoding` and is skipped outright when:
the request is `HEAD`; the client sent no `Accept-Encoding: gzip`; the upstream
already set `Content-Encoding` (never double-encoded); the response
`Content-Type` doesn't match `types`; the body stays under `minBytes`; the
status is `204`, `304`, or `101` (protocol switch); or the response is
`text/event-stream` or otherwise starts **streaming** (the handler flushes
before the compress decision is made — this is also what keeps a WebSocket
upgrade, which is hijacked rather than written through, untouched). A
compressed response gets `Content-Encoding: gzip`, `Vary: Accept-Encoding`, and
has `Content-Length` stripped (the compressed size isn't known up front).
**BREACH**: compressing a response whose size depends on attacker-controlled
input reflected alongside a secret (e.g. a CSRF token) can leak that secret
through the compressed size — that trade-off is why compression is opt-in per
host rather than default-on; hosts serving that shape of response should leave
it off.

```yaml
name: app
domains: [app.example.com]
upstream: {scheme: http, host: backend, port: 8080}
websocketsUpgrade: true
tls: {certificateRef: wildcard, forceSSL: true}
dns: {lanDirect: true, publicCname: true}
middlewares: [require-sso]
compression: {enabled: true}
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

A TCP stream can additionally be **TLS-aware** (`tls`): SNI-routed so several
hosts share one port, either passed through untouched or terminated at gpm. And
it can be **gated at L4** (`accessLists`) on the client IP, evaluated before any
backend is dialled.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `listenPort` | int | yes | 1–65535. **Publish this port from the container** (compose `ports:`) so it is reachable, and avoid colliding with the data-plane 80/443 or admin port — a bind failure is logged and that one port is skipped, never fatal. |
| `protocol` | string | yes | `tcp`\|`udp`\|`both`. |
| `forwardHost` | string | yes | Backend host. |
| `forwardPort` | int | yes | 1–65535. |
| `tls` | StreamTLS | no | SNI routing and/or TLS termination. **TCP only.** |
| `accessLists` | []string | no | L4 access lists evaluated on the client IP (below). |

```yaml
name: postgres
listenPort: 5432
protocol: tcp
forwardHost: db.internal
forwardPort: 5432
accessLists: [lan-only]
```

### L4 access lists (`accessLists`)

A stream host may reference AccessList objects, exactly like a proxy host. Only
the **IP/CIDR rules and the geo rules** are evaluated — basic auth is an HTTP
challenge/response with nowhere to live in a raw stream, so referencing a list
that has `basicAuth` users is **rejected at validation** rather than silently
half-applied. All referenced lists must allow, and the check runs **before any
backend is dialled**, so a denied client cannot make gpm open a socket to the
backend at all.

For UDP the list is evaluated once per session (the first packet from a source);
a denied source creates no session and no upstream socket. Geo rules follow the
same fail-closed rule as HTTP: with geo configured and no GeoIP database loaded,
the port denies.

Behind an L4 balancer, enable `settings.proxyProtocol` — otherwise every
connection looks like it came from the balancer and the rules match the wrong
address.

The `maxUDPSessions` bound (4096 per listener) still caps spoofed-source UDP
memory independently of any access list.

### StreamTLS (`tls`)

Makes a TCP stream port TLS-aware: several hosts can share one port, separated by
the SNI in the ClientHello, and gpm can either forward the handshake untouched or
terminate it.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `mode` | string | yes | `passthrough` \| `terminate`. |
| `sniMatch` | []string | see below | Server names this host claims. Exact (`db.example.com`) or a single-label wildcard (`*.example.com`). |
| `certificateRef` | string | terminate only | Names a Certificate. **Required** in `terminate`, **forbidden** in `passthrough`. |

- **`passthrough`** peeks the ClientHello (a bounded, stdlib-only parse of the
  record, handshake and `server_name` extension), routes on the SNI, and replays
  the peeked bytes to the backend. gpm never decrypts and never needs the key —
  the backend terminates, end to end.
- **`terminate`** completes the handshake at gpm with `certificateRef` from the
  normal certificate store (custom or ACME-issued) and forwards **plaintext** to
  the backend. The floor is TLS 1.2 with the same AEAD cipher suites the HTTPS
  listener uses. No ALPN is offered: what rides inside a stream is an arbitrary
  TCP protocol.

**Port sharing.** Two or more enabled stream hosts may share a TCP `listenPort`
**only if every one of them sets `sniMatch`** — that is the only thing that tells
their connections apart. Validation rejects a mixed or duplicate claim, so
routing can never fall back to "whichever host compiled last". A host alone on its
port may omit `sniMatch` and take every connection. On an SNI-routed port, a
connection whose server name no host claims (or that sends no SNI) is closed
rather than handed to an arbitrary backend, and a connection that is not TLS at
all is closed too.

**UDP.** `tls` requires `protocol: tcp`. A UDP datagram carries no ClientHello, so
`udp` and `both` are rejected at validation; two hosts can never share a UDP port
either.

```yaml
# Two Postgres instances behind one public 5432, separated by SNI, never decrypted.
name: pg-blue
listenPort: 5432
protocol: tcp
forwardHost: pg-blue.internal
forwardPort: 5432
accessLists: [lan-only]
tls:
  mode: passthrough
  sniMatch: [blue.db.example.com]
---
# TLS terminated at gpm, plaintext to a backend that speaks none.
name: mqtt
listenPort: 8883
protocol: tcp
forwardHost: mosquitto.internal
forwardPort: 1883
tls:
  mode: terminate
  sniMatch: [mqtt.example.com]
  certificateRef: wildcard
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

A dead host's response renders the [settings-level error page](#errorpagesconfig-settingserrorpages--proxyhosterrorpages)
for `statusCode`, when one is configured, falling back to the plain-text body
otherwise; it has no `errorPages` field of its own to override with.

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

**ACMESpec**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `email` | string | yes | ACME account contact. |
| `challenge` | string | no | `dns-01` or `http-01`. Default: `dns-01` when `dnsProvider` is set (so configs written before this field existed keep their behaviour), `http-01` otherwise. |
| `dnsProvider` | string | for `dns-01` | A [DNSProvider](#dnsprovider-configdns-providers) name. Rejected with `http-01`. |
| `directoryURL` | string | no | Defaults to Let's Encrypt production. |
| `keyType` | string | no | `ecdsa` (default) \| `rsa`. |
| `eab` | EABSpec | no | External Account Binding, for CAs that require it. |

**EABSpec**: `kid` (the key id the CA issued) and `hmacKey` (Secret; base64url
as the CA issued it). Both are required together. An EAB key id widens the ACME
account identity, so two external accounts on the same CA get separate account
keys under `<cert-dir>/acme/accounts/`.

Challenge selection:

- **`http-01`** - validated on the data plane's plaintext `:80` listener. The
  ACME manager parks the in-flight token in memory and the listener answers
  `/.well-known/acme-challenge/<token>` **before** host routing, the force-SSL
  redirect, and any auth, so a certificate can be issued for a host that does not
  exist yet or that redirects everything to https. A challenge path whose token
  is not in flight is routed normally, so an upstream running its own ACME client
  keeps working. Port 80 must be reachable from the internet.
- **`dns-01`** - the only challenge that can prove a wildcard. A wildcard domain
  with `http-01` (explicit or defaulted) is a validation error.

```yaml
# ZeroSSL with External Account Binding, http-01
name: zerossl
type: acme
domains: [app.example.com]
acme:
  email: admin@example.com
  challenge: http-01
  directoryURL: https://acme.zerossl.com/v2/DV90
  eab:
    kid: ${ENV:ZEROSSL_EAB_KID}
    hmacKey: ${ENV:ZEROSSL_EAB_HMAC}
```
```yaml
# Google Public CA (EAB required; kid + hmacKey come from `gcloud publicca`)
acme:
  email: admin@example.com
  challenge: http-01
  directoryURL: https://dv.acme-v02.api.pki.goog/directory
  eab:
    kid: ${ENV:GOOGLE_CA_EAB_KID}
    hmacKey: ${FILE:/run/secrets/google_ca_eab_hmac}
```

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
# ACME single name over http-01 (no DNS provider needed)
name: app
type: acme
domains: [app.example.com]
acme:
  email: admin@example.com
  challenge: http-01
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

## ClientCA (`config/client-cas/`)

The trust anchor for per-host mTLS: the CA bundle presented client certificates
are verified against (referenced by `tls.clientAuth.caRef`), plus its optional
revocation list. It is kept distinct from [Certificate](#certificate-configcertificates)
because it verifies peers rather than identifying this server, and it never
carries a private key.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `caPEM` | string | yes | PEM CA bundle (one or more certificates). Public material, so it may be inline; a `${FILE:...}` / `${ENV:...}` placeholder also works. Must parse to at least one certificate at load. |
| `crlFile` | string | no | Certificate revocation list, PEM or DER, **relative to the cert store** (absolute paths and `..` are rejected, like a custom certificate's files). Re-read on every config reload and within 5 minutes of the file's mtime changing. |
| `crlPEM` | string | no | Inline PEM CRL, for a small list kept in git. Mutually exclusive with `crlFile`; changes only on a config reload. |
| `crlPolicy` | string | no | `fail-closed` (default) or `fail-open` — what happens when a configured CRL is unusable. Only valid alongside `crlFile`/`crlPEM`. |

With no CRL configured, certificates are verified against the CA only: a revoked
but unexpired certificate still passes (the Go standard library's chain
verification checks no revocation). With one configured, the data plane rejects
any presented certificate whose serial is listed, after checking that the CRL is
**signed by this CA** — so dropping a file into the cert store cannot un-revoke
or mass-revoke anything.

`crlPolicy` governs the failure modes: a CRL that is missing, unparseable,
foreign-signed, or past its `nextUpdate`. `fail-closed` (the default) then
rejects **every** client certificate verified against this CA — an operator who
configured revocation asked for it to be enforced; `fail-open` accepts them and
logs a warning, for a host where availability outranks revocation. Either way an
unusable CRL never fails the config reload: unrelated hosts keep serving, exactly
like the GeoIP database's live fail-closed evaluation.

```yaml
name: corp
caPEM: ${FILE:/run/secrets/corp_client_ca.pem}
crlFile: corp.crl          # <certDir>/corp.crl, PEM or DER
crlPolicy: fail-closed     # default; fail-open accepts when the CRL is unusable
```

---

## DNSProvider (`config/dns-providers/`)

Solves ACME `dns-01` challenges. Not needed for `http-01`.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `provider` | string | yes | `cloudflare` \| `digitalocean` \| `hetzner` \| `desec`. Anything else is rejected at write time. |
| `config` | map[string]Secret | yes | Provider-specific, secret-valued. Every shipped provider needs `apiToken`. |

Each solver talks to the provider's REST API directly (no SDK), finds the zone
owning the challenge name by **longest-suffix match** over the account's zones
(so a delegated `sub.example.com` wins over `example.com`), and adds rather than
replaces the TXT value, so an apex + wildcard order that shares
`_acme-challenge.example.com` validates.

| `provider` | `config` keys | Credential |
|------------|---------------|------------|
| `cloudflare` | `apiToken` | API token with `Zone:DNS:Edit` + `Zone:Read` on the zone (`Authorization: Bearer`). |
| `digitalocean` | `apiToken` | Personal access token with write scope on domains (`Authorization: Bearer`). |
| `hetzner` | `apiToken` | Hetzner DNS API token, from the DNS console (`Auth-API-Token` header). |
| `desec` | `apiToken` | deSEC API token (`Authorization: Token`). RRsets are read-modify-written; TTL is 3600, deSEC's minimum. |

```yaml
name: cloudflare
provider: cloudflare
config:
  apiToken: ${FILE:/run/secrets/cf_token}   # scope: Zone:DNS:Edit + Zone:Read
```
```yaml
name: hetzner
provider: hetzner
config:
  apiToken: ${ENV:HETZNER_DNS_TOKEN}
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
| `geo` | AccessListGeo | Country allow/deny over the same resolved client IP (requires `GPM_GEOIP_DB`). |

A list can also be attached to a **StreamHost**, where only the `rules` and `geo`
dimensions are evaluated (there is no request to carry basic auth). A list with
`basicAuth` users is rejected for a stream host at validation — see
[StreamHost](#streamhost-configstream-hosts).

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
| `bouncer` | BouncerMiddleware | Deny hook: ask an external bouncer (CrowdSec LAPI or any HTTP endpoint) whether the client IP is banned. |

**AuthMiddleware**: `identityProvider` (required except in `client-cert` mode),
`mode` (`oidc`|`forward-auth`|`auth-request`|`client-cert`, defaults from the IdP
type), `requiredRoles` (forbidden in `auth-request` mode — the IdP application
binding does authorization), `allowFrom` (CIDRs that bypass auth; `auth-request`
mode only — e.g. let a LAN skip SSO), `clientCertRoles` (`client-cert` mode only).

In `client-cert` mode the identity comes from the TLS handshake, so no
`identityProvider` is named (setting one is an error). The gate admits a request
only when the handshake **verified** a client certificate for this host — i.e.
the host runs `tls.clientAuth` and the trust anchor (and its CRL, if configured)
accepted the certificate; otherwise it replies `401`. `clientCertRoles` maps a
certificate subject to a role: the key is the RFC 2253 subject (`CN=ops,O=Corp`)
or its bare common name, the value is the role `requiredRoles` is checked
against. With no mapping, any verified certificate passes; with a mapping, an
unmapped subject or an insufficient role gets `403`. `requiredRoles` without a
mapping is refused at validation (it could never match).

Pair it with `tls.clientAuth.mode: optional` so certless clients still reach the
chain and this middleware is what refuses them, leaving an SSO middleware free to
cover other hosts or locations.

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

> **`queryEquals` and `;`.** A guard carrying any `queryEquals` trigger rejects a
> request whose raw query string contains a `;`, with **400** (before the
> allow/deny decision, so `allowFrom` does not exempt it). gpm parses the query
> the modern way, where only `&` separates parameters, so `?a=1;direct=1` is one
> parameter `a` with the value `1;direct=1` and a `direct: "1"` trigger would not
> fire - but the raw query is forwarded to the upstream unchanged, and a backend
> still honouring the legacy `;` separator would read `direct=1` and act on it.
> Rather than evaluate a query it cannot read the same way the upstream will, the
> guard fails closed. This mirrors the same rule for `;` in request paths. Guards
> with no `queryEquals` trigger are unaffected, as is every other middleware.

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
per-request work: rate-limit → access-list → bouncer → auth → guard → headers →
rewrite → upstream.

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

**BouncerMiddleware**: `provider` (`crowdsec`|`http`, default `crowdsec`), `url`
(required), `apiKey` (Secret — `${ENV:...}` / `${FILE:...}`), `timeout`
(default `2s`), `cacheTTL` (default `60s`), `cacheMaxEntries` (default `10000`),
`onError` (`fail-open`|`fail-closed`, default `fail-open`), `denyStatus`
(default `403`), `denyWith` (`error-page`|`plain`, default `error-page`),
`stream` (crowdsec only, default off).

This is a **hook, not a WAF**. gpm ships no rules, no signatures and no
detection engine: it asks an operator-run bouncer whether the client IP is
currently banned and acts on that verdict. What "banned" means lives entirely
outside gpm.

It sits **after the access list and before auth**: an operator allow-list still
wins outright (it is evaluated first, so an explicitly allowed IP is never
overridden by an external feed), and a banned IP never reaches the IdP — no
forward-auth subrequest, no OIDC redirect. A denial is reported to the per-host
denial counter.

**`crowdsec` provider.** Per uncached client IP, gpm calls the LAPI bouncer
endpoint `GET {url}/v1/decisions?ip=<client>` with `X-Api-Key: <apiKey>`. A
`null` or empty body means "no decisions" (allow); any decision of type `ban`
or `captcha` denies. **`captcha` is treated as a deny**: it is the LAPI telling
the bouncer this client must prove it is human, and gpm has no captcha flow to
hand it — serving the request anyway would silently downgrade the operator's
decision to an allow. Decision types gpm does not implement are ignored rather
than guessed at. The LAPI resolves range (CIDR) decisions itself, so one `ip=`
query covers those too.

**`stream: true`** (crowdsec only) swaps the per-IP lookup for a local one: gpm
pulls the whole decision set once
(`GET {url}/v1/decisions/stream?startup=true`) and then deltas every `cacheTTL`,
keeping the banned IPs and CIDR ranges in memory, so the request hot path never
calls the LAPI at all. Only the very first request waits on the startup pull;
refreshes happen in the background while the current set keeps serving, and a
failed refresh logs and keeps the previous set rather than dropping it. Use it
on a busy edge or with a large decision set; leave it off for the simpler
live-lookup mode.

**`http` provider** is a generic deny hook so any custom bouncer can plug in:

```
GET {url}?ip=<client>&host=<host>&path=<path>
X-Forwarded-For: <client>          # the RESOLVED client IP, not the inbound header
X-Original-URL: <absolute request URL>
X-Api-Key: <apiKey>                # only when apiKey is set
```

`2xx` = allow, `403` = deny, **anything else** = no usable answer, so `onError`
governs. The contract is deliberately trivial: a shell script, a fail2ban shim
or a corporate threat feed can implement it in a few lines.

`onError` covers a timeout, a connection failure, an unexpected status, an
undecodable body, an unresolvable `apiKey` secret, and a client IP that cannot
be resolved at all. It defaults to **`fail-open`** (allow): an unreachable
threat feed must not take the site down, which is the opposite of the right
default for auth. Choose `fail-closed` when the bouncer is a hard requirement
and you would rather serve `403` than serve an unvetted client.

Verdicts are cached per middleware, keyed by client IP, for `cacheTTL`, bounded
at `cacheMaxEntries` with LRU eviction (so a rotating-source-IP flood cannot
grow it without bound). A verdict derived from an **error** rather than a real
answer is capped at **5s** regardless of `cacheTTL`, so an outage cannot pin a
minute of guessed verdicts and keep guessing long after the bouncer recovered.

`denyWith: error-page` (the default) renders the host's configured custom error
page for `denyStatus`, falling back to the plain status body when none is
configured; `plain` opts out of the custom page deliberately —
a bare status body gives a scanner nothing to fingerprint.

**CrowdSec quickstart.** On the host running the CrowdSec LAPI:

```
cscli bouncers add gpm
```

Copy the printed key into the environment gpm runs with (e.g.
`CROWDSEC_BOUNCER_KEY`) and reference it as a placeholder — never commit the
literal:

```yaml
name: crowdsec
type: bouncer
bouncer:
  provider: crowdsec
  url: http://crowdsec:8080
  apiKey: ${ENV:CROWDSEC_BOUNCER_KEY}
  stream: true          # local lookups; deltas pulled every cacheTTL
  cacheTTL: 60s
  onError: fail-open    # an unreachable LAPI must not take the site down
```

Verify with `cscli decisions add --ip <your-test-ip> --duration 1m` and confirm
the host answers `403`, then `cscli decisions delete --ip <your-test-ip>`.

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
# Generic deny hook: any endpoint answering 2xx/403. Fail closed - this bouncer
# is a hard requirement, so an outage denies rather than admits.
name: threat-feed
type: bouncer
bouncer:
  provider: http
  url: https://bouncer.internal/check
  apiKey: ${FILE:/run/secrets/bouncer_key}
  timeout: 1s
  cacheTTL: 30s
  onError: fail-closed
  denyStatus: 403
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
list them: **rate-limit → access-list → bouncer → auth → guard → headers →
rewrite → upstream**. Rate limiting is outermost (evaluated first, so floods are
shed before any work); the access-list is evaluated ahead of the bouncer, so an
explicit operator allow-list is never overridden by an external feed's verdict,
and both are ahead of auth, so a denied or banned IP never reaches the IdP; path
rewrites are innermost (closest to the backend), so every security tier above
still sees the original client path. Host-wide middlewares run before any
location-scoped ones.

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

  `GET /metrics` is gated the same way but on its own, narrower scope
  (`metrics:read`) rather than `admin` — an exposition is not a credential dump.

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
`api-tokens` — plus four pseudo-resources for non-CRUD endpoint groups:
`settings`, `dns-sync`, `ingress-discovery` and `metrics`. An unknown subject or verb is rejected at write time.

**`metrics:read`** is what a Prometheus scrape credential needs for
`GET /metrics` (mounted only with `GPM_METRICS=1`, see
[deployment.md](deployment.md#metrics-prometheus)). It is its own subject rather
than `*:read` so a token that lives in a monitoring config forever buys exactly
one thing: the exposition names hosts and certificates but carries no field
values, so it is not a config read.
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

## Users, roles and audit

This is a deliberate stance, not a gap: gpm has **no local user table** and no
per-user permission system. Two things authenticate to the admin panel:

- **One local break-glass admin.** A single username/password pair
  (`GPM_LOCAL_ADMIN_USER` + a bcrypt hash - see
  [deployment.md](deployment.md#configuration-flags--environment)), always
  role `admin`, always IdP `"local"`. There is exactly one of these; it is not
  a config object, cannot be listed, and does not appear in git history. Its
  purpose is recovery when SSO is unreachable, not day-to-day login - see
  `adminAuth.ssoOnly` above.
- **OIDC group-to-role mapping.** Every other admin-panel login is an
  [IdentityProvider](#identityprovider-configidentity-providers) with a
  `roleMapping`: the IdP's group claim resolves to exactly one of two local
  roles, `admin` or `user` (`RoleNone` - no access - otherwise). There is no
  third role and no per-object permission grant; a mapped `admin` can do
  everything the local break-glass admin can, and `user` can only reach
  `GET /api/me` (it exists to prove a session is authenticated, not to run a
  reduced admin panel). **Individual identity is not gpm's to manage** - who
  is in `adminGroups`/`userGroups` is a decision made and audited at the IdP,
  the same way it already has to be for every other application behind SSO.

**Not planned: multi-user local accounts, or per-user permissions finer than
the two roles above.** Local accounts don't scale past the one break-glass
credential they exist for - a second local user would need its own storage,
its own rotation story, and its own audit trail, all of which OIDC already
provides for free once an IdP is configured. If you need more than one named
human with admin access, put an IdentityProvider in front of gpm; if you need
per-person restriction to a subset of hosts or actions, that is out of scope
for the same reason the two-role model is: gpm's authorization boundary is
role (admin/user) plus, for automation, [API-token scope](#scopes) - not identity.

**Delegation is API tokens, not accounts.** A script, CI pipeline, or
integration that needs its own credential - distinct from "is logged in as
the admin" - gets a scoped [APIToken](#apitoken-configapi-tokens), not a
second local login. Tokens are named, individually revocable, expirable, and
restricted to exactly the resources they touch (`proxy-hosts:write` and
nothing else, for example), which is the delegation model NPM's shared local
accounts don't have.

**Audit is git history plus webhooks, not an audit-log table.** Every write
through the API or UI - create, update, delete, settings, restore, revert -
is one commit to the config repo (`config/`), carrying whatever
[`Author`](architecture.md) the caller resolved to: the local admin's
username, the SSO subject/email from the session, or the token's own
`name` (its principal has no email, so the commit's email falls back to
`gpm@localhost`) for a token-authenticated write. `git log`
and `GET /api/history` / `GET /api/{plural}/{name}/history` are the audit
trail - who changed what, when, and (via `git show`) exactly what the diff
was, and it is tamper-evident by construction: rewriting it means rewriting
git history, not flipping a row in a database. `webhooks` above add a
real-time feed of the same events to an external system (SIEM, chat,
ticketing) if you want change notifications outside `git log`.
**What this does *not* cover:** authentication events themselves (successful
and failed logins) are logged to gpm's structured process log
(`GPM_LOG_LEVEL`/`GPM_LOG_CONSOLE`), not to git - a failed login attempt
changes nothing in config, so there is no commit for it. Ship the process log
to your log aggregator if you need a durable record of login attempts
alongside the config-change history.

---

## High availability (`GPM_HA_ROLE`)

Two instances can run as an active/standby pair with no clustering dependency.
The role is environment-only - it is not a config object, because it describes
*this process*, not the shared configuration.

| Env | Default | Effect |
|-----|---------|--------|
| `GPM_HA_ROLE` | `leader` | `leader`: runs the ACME renewal loop and Kubernetes Ingress discovery, accepts admin/API writes. `follower`: both loops off, every write refused with `503`, reads unaffected. An unrecognised value is a startup error |
| `GPM_HA_POLL_INTERVAL` | `20s` | How often a follower runs `git pull --ff-only` on the config repo and reloads if HEAD moved. Ignored on a leader |

A follower's config arrives only by pulling the leader's repo, so it never
commits and the two repos cannot diverge; a pull that is not a clean
fast-forward is logged and refused, never merged or reset.

`GET /api/capabilities` reports `"ha": {"role": "...", "readOnly": true|false}`,
which is what the admin UI uses to grey out write controls on a follower.

Both nodes must share `GPM_SSO_SIGNING_KEY` (identical value) and the ACME
material under `<cert-dir>/acme`, and the SSO revocation watermark
(`<cert-dir>/sso_not_before`) is re-read every 30s so a revoke propagates
without a restart. Full recipe - keepalived VIP, shared cert dir, promotion,
stream failover-with-reconnect - in [ha.md](ha.md).

---

## Kubernetes Ingress annotations

Shipped: see
[IngressDiscoverySettings](#ingressdiscoverysettings-settingsingressdiscovery)
above for the annotation contract, the derived-object rules, and the ownership
and freeze behaviour.
