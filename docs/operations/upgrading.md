# Upgrading and rolling back

Pin deliberately, migrate what a release asks for, and roll back with the
config as a separate axis from the binary.

## Upgrade notes by version

Every release that needs an operator action, newest first. A version absent from
this table needs none: stop the old process, start the new one against the same
data directory.

| Version | Action required | Detail |
|---|---|---|
| Unreleased | **Only if clients send more than 64 KiB of request headers** (large cookies, long SSO or bearer tokens; the realistic case to check is an auth server or application that spreads a large session over several cookies): the data-plane listeners now refuse a request whose request line plus headers exceed 64 KiB, where they accepted up to 1 MiB. HTTP/1 clients get `431`; HTTP/2 clients get `431` when the oversized header block ends in the frame where it overflows, and a connection error (GOAWAY, PROTOCOL_ERROR, closing every stream on that connection) when the client continues the block in CONTINUATION frames after the overflow or sends a single frame several times the limit. Set `GPM_MAX_HEADER_BYTES` / `-max-header-bytes` (up to `1048576`) to raise it; a value outside `4096` to `1048576` stops startup. The admin listener's limit also drops from 1 MiB to a fixed 64 KiB. | [Environment variables](../reference/env-vars-and-flags.md) |
| Unreleased | **Only if a rate limit or path block exempts clients through `@<list>` and that list is `disabled: true`:** the list now exempts nobody, so its clients are limited or blocked like everyone else (it used to keep exempting them); remove the `@<list>` entry, or enable the list, if the exemption was wanted. **Only if `GPM_SSO_SIGNING_KEY_FILE` was already set:** it is now read. A non-empty file wins over `GPM_SSO_SIGNING_KEY`, so per-host SSO users sign in again once, unless the file holds the key gpm already used; a file that cannot be read, or is empty, stops startup. Unset, nothing changes. **Only if a script sends `dryRun` to a write other than an object `PUT /api/{plural}/{name}`** (for example `PUT /api/settings?dryRun=1` or `DELETE /api/{plural}/{name}?dryRun=1`): it now gets `400` and nothing is written, where the parameter used to be ignored and the write happened (on an HA follower it gets `503`, as any write does). **Only if a script counts commits after `PUT /api/settings` or `PUT /api/settings/saved-filters`:** a save equal to the stored settings, when the config repository has no uncommitted changes, makes no commit and answers with an empty `X-Config-Commit`. **Only if an outbound target is an IPv4-mapped link-local literal written with a zone**, for example `[::ffff:169.254.169.254%25eth0]`, in a bouncer, webhook or notification target, an acme-dns, webhook, DigitalOcean, Hetzner or deSEC DNS provider, Kubernetes or Docker discovery, or DNS sync: it is now refused at connect time like any other link-local destination (it used to get through; a plain `fe80::1%eth0` was already refused). **Two new config warnings** show on the Overview and in `GET /api/health`, and nothing behaves differently: a `forward-auth` gate whose provider maps no role (every request is refused), and a DNS provider with `allowInsecureLocal` whose `baseURL` or `url` is a link-local address (every DNS-01 challenge fails). | [Environment variables](../reference/env-vars-and-flags.md), [Access lists](../reference/config/access-list.md), [OpenAPI reference](../reference/api.md#the-specification), [Troubleshooting](troubleshooting.md) |
| Unreleased | **Only if a gate sets `requiredRoles`:** three shapes are now reported as config warnings (Overview, `GET /api/health`) and behave as before: an entry that is not `admin` or `user` (it admits no one), a provider whose `roleMapping` gives no user a role (every request is refused), and a gate that takes `auth-request` mode from its provider (`requiredRoles` is not applied; the auth server decides). Use the role a group maps to, or remove the key. | [Gate a host with SSO](../how-to/per-host-sso.md) |
| Unreleased | **Only if you have certificates that live less than 90 days, a CA that offers ARI, discovery-managed hosts you edit by hand, or API tokens that write certificates:** (1) a certificate renews when `min(30 days, lifetime / 3)` remains, so one living less than 90 days renews when a third of its lifetime remains instead of on every pass; the regular pass is still every 12 hours and the loop also wakes when the next certificate enters its window. (2) A CA that advertises ACME Renewal Information (ARI, as Let's Encrypt does) is now asked, at most one request per certificate per pass, and can bring a renewal forward but never delay it. (3) The `expiring` state (also in `GET /api/health`) is lifetime-relative for `acme` certificates (a `custom` one keeps the fixed 30 days), and the daily `cert.expiring` digest lists an `acme` certificate only when its renewal is already due. (4) A failed attempt is retried after a backoff that grows with consecutive failures (for a 90-day certificate 1 h, 2 h, 4 h, 8 h, then every 12 h, counted from when the attempt started) instead of at the next 12-hour pass, every attempt now fails after the propagation timeout plus 10 minutes (13 minutes by default), and a pass that cannot load the config is retried every 5 minutes. (5) Tags added by hand to a discovery-managed host are now kept, and a managed host's own `trustedProxies` survives while the template leaves it unset (it used to be wiped by the next reconcile); a value a template stamped is marked with a `<prefix>/trusted-proxies-by` label and is removed again when the template or profile stops setting the key. (6) **Only if an API token with `certificates:write` sets `acme.caBundle` or reverts certificates:** setting or changing `acme.caBundle` through the API needs the `admin` scope, like `acme.directoryURL` and `acme.eab`, and `POST /api/certificates/{name}/revert` needs `admin` too (a `certificates:write` token gets `403`). (7) **Only if you wrote non-secret DNS provider settings as literals by hand** (rfc2136 `server`, `zone`, `tsigKeyName`, `tsigAlgorithm`, `transport`, `ttl`, `timeout`; acme-dns `baseURL`, `subdomain`, `allowInsecureLocal`): the API now returns them as written to any caller that can read DNS providers, where they used to read back as `***`. No config change is needed. | [Certificate: Renewal](../reference/config/certificate.md#renewal), [Ingress discovery: tags](../reference/config/settings/ingress-discovery.md#tags-on-managed-hosts), [trustedProxies](../reference/config/settings/ingress-discovery.md#trustedproxies-on-managed-hosts), [Rollback](#rollback) |
| Unreleased | Remote access-list sources gain an optional `maxCoverage` that bounds the total address space a feed may cover; a source without it behaves as before, and a fetch whose feed covers more than one /8 of IPv4 or one /24 of IPv6 logs a `WARN` that the feed is broad. No action needed. **Only if `GPM_LOCAL_ADMIN_TOTP_SECRET_FILE` or `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` names an empty file:** local login is refused until the file holds the secret. Admin sessions whose sign-in method is no longer allowed (provider unlisted or deleted, local login off) are signed out. An SSO login started before the upgrade and finished after it may fail once on an `https` admin panel; sign in again. A proxy-host write that sets more than 64 plaintext basic-auth passwords at once is refused. | [Access list](../reference/config/access-list.md#access-list-sources-max-coverage), [Settings: admin authentication](../reference/config/settings/admin-auth.md) |
| Unreleased | **Only if a script relies on a write being committed although it could not be applied:** a config write that validates but that the proxy listeners cannot build (for example a custom certificate whose files do not load) is now refused with `400` and nothing is committed (it used to be committed and answered `500`). This covers save, batch, revert, restore and discovery batches. **Before restoring a backup that uses custom certificates:** put the certificate files back in `certs/` first; a restore whose config names certificate files that do not load is refused. **Only if two hosts' domains differ only by a `:port` suffix:** a write that makes two hosts route under one name is refused; a pair already stored only logs a config warning and stays editable. | [Writes must compile](../concepts/config-model.md#writes-must-compile), [Domains are exclusive](../concepts/config-model.md#domains-are-exclusive), [Backup and restore](backup-and-restore.md) |
| Unreleased | **Only if an API token with `certificates:write` sets `acme.directoryURL` or `acme.eab`:** setting or changing either field now needs the `admin` scope (`403` otherwise); a write that keeps both as stored, or clears them, works as before. **Only if your ACME CA redirects its URLs, takes over 30 s to answer or sends more than 1 MiB in one response:** ACME requests now have a 30 s timeout and a 1 MiB response cap and do not follow redirects, so such a CA fails the attempt; point `directoryURL` at the final URL. | [Certificate](../reference/config/certificate.md), [API token](../reference/config/api-token.md) |
| Unreleased | **Only if you read discovery reconcile status by its text:** a skip reason for a domain collision now reads `domain "<d>" is already claimed by <kind> "<h>", ...` and names the holder's kind. **Only if you roll back after setting `accessListsMode: any`:** an older version drops the key, so the lists are checked as `all` again and a client that only one list allows gets `403`; see [Rollback](#rollback). No action otherwise. | [Ingress discovery](../reference/config/settings/ingress-discovery.md), [Access list](../reference/config/access-list.md#access-list-composing) |
| Unreleased | **Only if a script edits, deletes, reverts or restores an object that an Ingress or Docker discovery template or profile names:** a write whose result would leave such a reference missing or unusable is now refused with the settings write's error (it used to be accepted, and that source's reconcile then failed on every poll); change the template or profile first. A config already stored that way still loads, with a config warning. **Only if you call the dry-run plan routes concurrently or in a tight loop:** concurrent callers now get one shared plan instead of `409`, and a successful plan is reused for up to 5 s unless the config changes. **Only if an upstream sends more than 16 interim `1xx` responses for one request:** the further ones are not relayed, and past 256 the request ends with `502`. **Only if a headers middleware has `setResponse` / `removeResponse` rules and the host serves WebSockets:** the rules now also apply to the `101` handshake response (never to `Connection`, `Upgrade` or the `Sec-WebSocket-*` handshake headers). No action otherwise. | [Ingress discovery](../reference/config/settings/ingress-discovery.md), [Docker discovery](../reference/config/settings/docker-discovery.md), [API](../reference/api.md), [Middleware](../reference/config/middleware.md) |
| Unreleased | **Before upgrading, only if the config directory may contain symbolic links:** a link (a file or a kind directory) whose target is outside the config directory, missing, or a link loop now makes gpm **refuse to start** (a reload keeps the running config) with an error naming the file; find such links with the command in [Symbolic links](../concepts/config-model.md#symbolic-links) and replace each with the file itself or a link to a file inside the directory. Links that resolve inside the directory, ConfigMap `..data` layouts and a linked config directory work as before. **Only if an API client reads `passwordHash` values** (to copy credentials elsewhere, or to send them back under a new username): reads now return `***`; read hashes from the config files or a `GET /api/backup` archive, and send a `password` for a new or renamed user. A write that sends `***` back for an unchanged user keeps the stored hash. **Only if a Docker-discovered container's address is link-local** (`169.254.0.0/16` or `fe80::/10`): the container is now skipped (with `network` unset, gpm forwards to its address on the next network by name that is not link-local, if it has one), and a host already stored for it is kept unchanged while its profile is unchanged (and disabled if the profile changes); set `dockerDiscovery.allowLinkLocal: true` to keep the old behaviour. **Only if a DigitalOcean account's domain listing spans more than 20 pages:** DNS-01 now fails with an error instead of matching against a partial list. **Only if you alert on a non-empty `configWarnings` in `GET /health`:** it now also lists every access-list `deny` rule whose remote source resolves to the empty set (never fetched, expired, over `maxCoverage`, or every entry refused; such a rule denies no one), so fix the feed or expect the entry. **Only if a Docker-derived host is skipped for a domain conflict:** its upstream now follows the container's current address on the next reconcile. **Only if you roll back after setting a rate limit's `ipv6Prefix`:** an older version drops the key and groups IPv6 clients per /64 again. No action otherwise. | [Symbolic links](../concepts/config-model.md#symbolic-links), [REST API](../reference/api.md#basic-auth-password-hashes), [Docker discovery](../reference/config/settings/docker-discovery.md#settings-docker-discovery-allow-link-local) |
| Unreleased | **Only if a guard trigger lists `GET` without `HEAD`:** `HEAD` requests on its paths are now denied like `GET` for clients outside `allowFrom`. **Only if a route has an auth-request or OIDC gate that runs before a basic, client-cert or forward-auth gate:** that gate's sign-in endpoints (the outpost `pathPrefix`, `/__gpm/oidc/callback`) now also require the basic, client-cert or forward-auth credential unless the client is covered by the basic, client-cert or forward-auth gate's own `allowFrom`; no ordering exempts the sign-in endpoints, so add the client networks to that gate's `allowFrom` if they must stay reachable without the credential. The run order of several gates is unchanged and now documented. **Only if gpm sits behind trusted proxies in front of an auth-request outpost:** the outpost's sign-in, callback and sign-out requests now carry the real client IP in `X-Forwarded-For` and `X-Real-Ip` instead of the proxy's, so check any IP-based policy on the outpost. **Only if an outpost endpoint under `pathPrefix` takes more than 10 s to send its response headers:** that request now gets `502`. **Only if an access-list rule is a bare IPv4-mapped IPv6 address** (`::ffff:a.b.c.d`, no prefix length): the rule now applies to the IPv4 address `a.b.c.d` (an `allow` admits it, a `deny` refuses it) and no longer to IPv6 clients in `::/32`. **Only if a `clientCertRoles` key is a bare common name containing `=`:** that key stops matching; re-key it on the certificate's full RFC 2253 subject. **Only if a rate limit's `allowFrom` names an access list (`@<list>`) that has a `deny` rule above an `allow` rule covering the same addresses:** those addresses are now rate limited instead of exempt; move the `deny` rule below the `allow` rule or list the networks in `allowFrom` directly if they should stay exempt. **Only if a TCP stream host carries a protocol where one peer half-closes, or a peer ignores the other side's close:** a connection now ends when both directions are done or on an error, instead of at the first EOF. On a host with `idleTimeout` the idle deadline is then the only bound; on a host without it, the pair is reset 60 s after the first side finished. A peer that ignores the close holds the connection and its `maxConnsPerSource` slot until that bound; set `idleTimeout` if a reply after a half-close can take longer than 60 s. No action otherwise. | [Several auth gates on one route](../concepts/request-pipeline.md#several-auth-gates-on-one-route), [Middleware](../reference/config/middleware.md), [Identity provider](../reference/config/identity-provider.md), [StreamHost](../reference/config/stream-host.md#tcp-connection-close) |
| Unreleased | **Only if you ran a pre-release dev build with analytics on:** its `analytics.db` (schema 1) is rebuilt to schema 2 on first start (one transaction, rows kept without a source). The dev build cannot open the file afterwards; keep a copy if going back is possible. No config change. | [Settings: analytics](../reference/config/settings/analytics.md) |
| Unreleased | **Only if you alert on `gpm_denials_total{reason="access-list"}`:** denials decided by a country rule now carry `reason="geo"`. Add `geo` to the alert, or alert on the sum over reasons. No config change. | [Metrics](../reference/metrics.md), [Analytics](../reference/analytics.md) |
| Unreleased | **Only if an error page uses more than the documented fields, `if`/`else`/`with`, comparisons and the print and escape functions applied to a field** (for example `range`, `{{template}}`, `{{block}}`, `{{define}}`, a variable such as `{{$x := ...}}`, a parenthesized call, `index`, `slice` or `call`), **or is over 32 KiB, prints more than 128 values, or nests `if`/`with` more than 16 deep**: a write that adds or changes such an inline template is refused; one already stored, or such a file in `errorPages.dir`, is skipped with a `WARN` and the next fallback page (or the built-in response) is served instead. Rewrite the page with the [allowed constructs](../reference/config/settings/error-pages.md#allowed-template-constructs). New optional keys, off by default: `rateLimit.scope: host` (one budget per client across a host and its locations; the default stays one per route), stream host `idleTimeout` and `maxConnsPerSource` (TCP). No action otherwise. | [Settings: error pages](../reference/config/settings/error-pages.md), [RateLimitMiddleware](../reference/config/middleware.md#ratelimitmiddleware-ratelimit), [StreamHost](../reference/config/stream-host.md) |
| Unreleased | **Only if a host or certificate domain carries a port, a trailing dot or another character that is not a hostname character:** it keeps loading and is reported as a config warning; new writes that add such a domain are refused with `400`, an edit that keeps or removes it is accepted, and a revert or restore to a state that holds one is accepted with a warning. Fix the domain when convenient. **Only if `adminEdge` is on and an enabled host lists the panel's hostname with a port:** it is reported as a config warning (the front door and other writes work as before); a write that adds such a host, or turns `adminEdge` on over one, is refused with `400`. **Only if a config file's name differs from its object name:** the next save of that object moves it to `<name>.yaml` in the same commit, and a write whose `<name>.yaml` holds another object is refused until that object is saved. | [Domains are exclusive](../concepts/config-model.md#domains-are-exclusive), [Writes must compile](../concepts/config-model.md#writes-must-compile) |
| Unreleased | **Only if you release by pushing a `vX.Y.Z` tag by hand:** the release workflow now builds a pushed tag only when its commit is the current head of `main`; a tag on an older commit stops before anything is built or signed. Releases minted by a merge to `main` are unchanged. | [Verifying the image](../getting-started/install-docker.md#verifying-the-image) |
| 1.0.0 | **Rename `config/dead-hosts` to `config/parked-hosts`** before starting the new binary. The API path becomes `/api/parked-hosts`, the token-scope subject becomes `parked-hosts`, and the whole-config key becomes `parkedHosts`. | Below, and [ParkedHost](../reference/config/parked-host.md) |
| 1.0.0 | **Replace each stream host's `forwardHost`/`forwardPort` with a single `target: {host, port}`.** A file still using the old keys is rejected at load. | Below, and [StreamHost](../reference/config/stream-host.md) |
| 1.0.0 | **Only if you relied on `forwardAuth.trustedProxies` for client-IP resolution:** copy the same CIDRs into `settings.trustedProxies`. It no longer doubles as `X-Forwarded-For` trust. gpm logs a `WARN` naming the exact block to paste. | [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers) |
| 1.0.0 | **No action now, but migrate when convenient:** `AccessList.basicAuth` and `AccessList.satisfyAny` are deprecated and still enforced (2.x included). A later major release will refuse a config that carries them. | [Migrate access-list basic auth](../how-to/migrate-basic-auth.md) |
| 2.6.0 | **Security follow-ups that can need an action.** (1) Webhook receivers: the shared secret is no longer sent as `X-GPM-Webhook-Secret`; deliveries carry `X-GPM-Webhook-Timestamp` and `X-GPM-Webhook-Signature` (`v1=` + hex HMAC-SHA256 over `timestamp.body`), so update the receiver to verify the signature. (2) API tokens: writes to `/api/dns-providers` and `/api/identity-providers`, and `POST /api/middlewares/{name}/revert`, now need the `admin` scope (the per-kind `:write` and `*:write` are refused with 403); a bouncer middleware with a placeholder `apiKey` is created, or has its `url`/`apiKey` changed, by `admin` only. (3) A client-cert auth gate on a host without its own `tls.clientAuth` still loads, but it now **admits only its `allowFrom` exemptions**: a certificate is accepted only when the handshake verified it under this host's SNI configuration, which such a host never asks for. `GET /health` lists it under `configWarnings`; add `tls.clientAuth` (mode `optional` if some networks are exempt) to that host. The same warning names a host with more than one OIDC gate, which now signs the user out of one gate at every login at the other: keep one OIDC provider per host. (4) An explicit `ingressDiscovery.tokenFile`/`caFile` other than the projected in-cluster paths must lie under `GPM_SECRET_FILE_ROOTS`, **or the config does not load** (this is validation): move the file or extend the roots before upgrading. (5) `HTTPS_PROXY`/`HTTP_PROXY` are no longer honoured by the Kubernetes, DNS-sync, webhook, notification and ACME DNS-01 REST-provider clients (they are still honoured by the upstream proxy transport, the ACME directory client and OIDC discovery), so a deployment whose webhooks, notifications or DNS-01 API calls reach the internet only through a proxy must give gpm a direct route for them. (6) A secret value that mixes literal text with a placeholder (`Bearer ${ENV:TOKEN}`) is now a literal: **every write, discovery batch and revert is refused** while one exists anywhere in the config, with the field named in the error, so replace such values before upgrading; a revert to a commit holding one is refused too. (7) Existing SSO and sticky cookies are re-issued once (a login round-trip, a re-pin). (8) A request body that makes no progress for 60 s is cut off; a streamed request body that legitimately pauses longer than that is now refused. (9) **Listeners are plain TCP.** Go 1.24+ opened Multipath TCP (MPTCP) listeners by default on kernels with `net.mptcp.enabled=1`; 2.6.0 turns that off on the data plane, stream hosts and admin server (`GPM_MPTCP=1` / `-mptcp` restores it). No client-visible change: an MPTCP-capable client falls back to TCP in the handshake. Context: a listening MPTCP socket was the precondition of the published root exploit for the Linux kernel flaw CVE-2026-98432 (IPv4 TCP request-socket use-after-free); the kernel update is still the fix, this removes the foothold meanwhile. | [Webhooks](../reference/config/settings/webhooks.md), [REST API](../reference/api.md) |
| 2.5.0 | **A backup restore no longer brings back `api-tokens/*`:** the live tokens are kept and the archive's are ignored, as Revert already did. Restoring onto a fresh instance means re-minting tokens. Also: `GET /version` now needs a signed-in session or token; use `/healthz` for anonymous checks. | [Backup and restore](backup-and-restore.md) |
| 1.0.x, all | **Roll a newer binary out before adopting config it introduces.** An older binary's kind map silently ignores a directory it does not know, so a downgrade leaves live objects invisible rather than cleanly rejected. Applies to `upstream-groups` and `api-tokens`. | Below |

## 1.0.0, in full

> **BREAKING: 1.0.0 renamed two things, and neither is migrated for
> you.** The config store is a git working tree, so gpm refuses an unmigrated
> tree at startup with the exact fix in the error rather than authoring a commit
> you never made. Do both steps in the config repo *before* starting the new
> binary:
>
> 1. **`DeadHost` is now `ParkedHost`.** Rename the directory and commit it:
>
>    ```
>    git mv config/dead-hosts config/parked-hosts
>    git commit -m "Rename config/dead-hosts to config/parked-hosts"
>    ```
>
>    Startup and every reload fail while `config/dead-hosts/` still holds
>    objects and `config/parked-hosts/` holds none. The API path becomes
>    `/api/parked-hosts`, the token scope subject becomes `parked-hosts`
>    (update any API token that names `dead-hosts:read` / `dead-hosts:write`),
>    and the whole-config `deadHosts` key becomes `parkedHosts`.
>
> 2. **A stream host's `forwardHost`/`forwardPort` become a single `target`.**
>    Edit every file under `config/stream-hosts/` and commit:
>
>    ```yaml
>    # before                    # after
>    forwardHost: db.internal    target:
>    forwardPort: 5432             host: db.internal
>                                  port: 5432
>    ```
>
>    A file still using the old keys is **rejected at load** with an error
>    naming the new shape; it is never silently accepted with no backend.
>
> Restore is the one place the old name still works: a `GET /api/backup`
> archive taken before the rename restores fine, its `dead-hosts/` entries
> mapped onto `parked-hosts/` (logged at info). That mapping is one-way and
> restore-only; nothing is ever written back under the old name.

**Pin explicitly, and verify before you deploy.** Bump the image tag/digest
(GitOps) or the binary version (bare metal) deliberately rather than tracking
`latest`, see [Verifying the image](../getting-started/install-docker.md#verifying-the-image) for the
cosign check to run against whatever you're about to pin. There is no
in-place "upgrade" operation: stop the old process/container, start the new
one against the same `/data` (or `/var/lib/gpm`) volume, and confirm
`GET /version` (signed in, or with an API token) reports what you expect;
`GET /healthz` is the anonymous check.

**Config compatibility.** `Config`/`Settings` carry an explicit
`schemaVersion` (currently `1`); a version bump would come with a documented,
reversible migration in the store layer, not a silent format break: none has
shipped yet. The two situations that matter *today*, both because an **older**
binary's kind map silently ignores a directory/field it doesn't know about
rather than erroring on it, so a downgrade can leave live objects invisible to
it instead of cleanly rejected:

- **Upstream groups**: roll the new binary out *before* the first host
  references an `upstream-groups` entry. Full reasoning under
  [Upstream-group health](../reference/config/upstream-group.md#watching-live-health-operations).
- **API tokens**: roll the new binary out *before* creating the first
  `api-tokens` object. Full reasoning under [API tokens](../reference/api.md#api-tokens-automation).

The general rule those two both follow: roll a **newer** binary out before
adopting any config it introduces; roll an **older** binary back only after
confirming your config doesn't name a kind or field that predates it (an
older loader ignoring a directory isn't a downgrade-safety net; the objects
in it are simply invisible to that instance until you roll forward again).

**`session.db` has no versioned migrations to worry about yet**: the schema
is a single idempotent `CREATE TABLE IF NOT EXISTS` with no `ALTER TABLE`
history (see `internal/session/session.go`), so every released binary to date
reads and writes the identical schema and a downgrade is safe as far as
sessions are concerned. If a future release does add a schema change, treat
`session.db` as roll-forward only for that specific hop unless its changelog
entry says otherwise; the safe fallback if you must roll back past it is
deleting `session.db` (forces every admin to log in again; does not touch
`config/` or `certs/`).

**Rollback procedure**, once you've decided you need one:

1. Stop the new version.
2. If the upgrade you're undoing crossed one of the config-compatibility
   points above, resolve that first (e.g. don't roll back past the release
   that introduced your first `upstream-groups` reference while still using
   it), `POST /api/revert` or `POST /api/restore` from a
   [pre-upgrade backup](backup-and-restore.md) roll the *config* back
   independently of the binary version, and you may need both together.
3. Start the previous version against the same data directory.
4. Confirm `GET /version`, `GET /healthz`, and that proxied traffic is still
   routing: the same checks as a fresh deploy.

Taking a [`GET /api/backup`](backup-and-restore.md) immediately before an upgrade
that changes anything nontrivial costs one `curl` and turns "roll back" into
a config restore instead of a guess.

## Rollback

Starting an **older** gpm binary against a config directory a **newer** one has
written to. Both config loaders (per-object files and `settings.yaml`) use a
non-strict YAML decode, so a key the older struct does not recognise is
silently dropped rather than rejected, and the first write after that,
including an automatic reconciler commit, makes the drop permanent. A load
that meets an unknown key warns (`GET /api/health` `configWarnings`) and the
reconciler refuses to write that file, so the drop is visible before it
becomes permanent. The [changelog](https://github.com/Rake-Pro/go-proxy-manager/blob/main/CHANGELOG.md)
lists the fields each release introduces.

**Certificates and DNS providers.** An older version differs in three ways:

- It does not know `provider: webhook`: its validation rejects a DNS provider with `provider: must be one of ..., got "webhook"`, so a config that has one fails validation.
- It ignores `acme.profile` and `acme.caBundle`: private-CA renewals fail with an unknown-authority error, and short-lived certificates go back to renewing on every pass.
- It does not keep hand-added tags: its reconcile replaces a managed host's tags with the template's again.

**Discovery `trustedProxies`.** An older version has no template `trustedProxies`: its first reconcile removes the value from every derived host, and hosts that relied on a template `[]` (trust nobody) fall back to `settings.trustedProxies`. Check that list first.

### Prerequisites

- The version you are rolling back to, and its row (if any) in
  [Upgrade notes by version](#upgrade-notes-by-version) above.
- Either a [`GET /api/backup`](backup-and-restore.md) archive taken before you
  started using the newer version, or `git log` access to `/data/config` (the
  config repo, not the gpm source checkout) to recover an earlier commit.

### Steps

1. Stop the newer binary.
2. Check the newer binary's log (or `GET /health`'s `configWarnings` field)
   for `config: <path>: unknown keys ignored: ...` lines before you stop it:
   each one names a file the older version will load incorrectly rather than
   fail on.
3. If any file has unknown keys, recover the config **first**, rather than let
   the older binary start against it:

   ```
   cd /data/config
   git log --oneline
   git checkout <commit-before-the-newer-only-feature> -- .
   git commit -m "Roll back config ahead of downgrade to <version>"
   ```

   Recover only the affected files if you know exactly which ones they are;
   `git checkout <commit> -- <path>` restores one.
4. Start the older binary against the same data directory.
5. Confirm `GET /version`, `GET /healthz`, and that proxied traffic is
   routing: the same checks as a fresh deploy.

### Verify

- `GET /version` reports the version you rolled back to.
- The process log carries no `config: ... unknown keys ignored` warning for a
  file you recovered.
- A host that had an inline `auth:`/`rateLimit:` block, or a `mode: basic`
  middleware, gates traffic the way it did before the rollback, not wide
  open, and the process is not refusing to start.

### Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| A host with an inline `auth:` or `rateLimit:` block is reachable with no login or rate limit after rollback | The older binary's first write to that file silently dropped the unknown block | Recover the file with `git checkout` above, then re-apply the block once back on a version that supports it |
| The older binary refuses to start, or a load error names `auth.mode` | A middleware or inline `auth` block uses `mode: basic`; a build that predates it rejects the value outright | Recover the config to before that middleware/block existed, or stay on a build that knows the mode |
| Derived hosts lose their `trustedProxies`, or a template `trustedProxies: []` (trust nobody) no longer applies | An older version has no template `trustedProxies`, so its first reconcile removes the value from every derived host; hosts that relied on a template `[]` fall back to `settings.trustedProxies` | Before rolling back, check `settings.trustedProxies` covers those hosts; re-add the template key once back on a version that supports it |
| `upstream.path`, `hostHeader`, `stripPrefix`, or a `trustedProxies` list reverted to empty | Same silent-drop, on those fields | Same recovery as above |
| A `rewrite` middleware stopped matching | Same silent-drop, on its prefix/regex rules | Same recovery as above |
| `settings.notifications` or `settings.dockerDiscovery` stopped firing or discovering | Same silent-drop, on `settings.yaml` | Recover `settings.yaml` from git and re-add the section once back on a version that supports it |
