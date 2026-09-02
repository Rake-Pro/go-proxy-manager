# Terminology

Every term gpm uses in the docs, the API and the YAML, in one place. Object
kinds are given by their config directory name, with the UI's own nav label in
brackets where it differs.

## Host kinds

| Term | Means |
|---|---|
| **Host** | A domain name gpm serves. Four kinds, one domain claimed by at most one enabled host. |
| **Proxy host** (`proxy-hosts/`, UI: **Proxy Hosts**) | Terminates TLS for one or more domains and reverse-proxies to an upstream. Also written "reverse-proxy host" or just "host". |
| **Redirect host** (`redirect-hosts/`, UI: **Redirects**) | Answers a configured `3xx` to another domain instead of proxying. |
| **Stream host** (`stream-hosts/`, UI: **Streams**) | Raw TCP/UDP forwarding on its own listen port, optionally SNI-routed. |
| **Parked host** (`parked-hosts/`, UI: **Parked Hosts**) | Reserves a domain and answers a fixed status, default `404`. gpm's name for what Nginx Proxy Manager calls a `dead_host`; nothing about it is dead, it is a live TLS-terminating vhost doing exactly what it was told. |

## The path a request takes

| Term | Means |
|---|---|
| **Data plane** | The listeners that serve proxied traffic, `:80` and `:443` plus one per stream `listenPort`. |
| **Control plane** | The admin listener: REST API and web UI, default `:8081`. |
| **Upstream** | The backend a proxy host or location forwards to: `{scheme, host, port}`. Prose sometimes says "backend" for the same thing; the schema field is always `upstream`. A stream host's equivalent field is `target`, which carries no scheme. |
| **Upstream group** (`upstream-groups/`, UI: **Upstream Groups**) | An ordered set of interchangeable upstreams with health checks and a failover policy. |
| **Location** | A path prefix inside a proxy host with its own upstream and extra policy. Always at least as restrictive as its host. |
| **Chain** | The ordered sequence of middlewares applied to one host or location. Fixed order, not the order you listed them - see [Request pipeline](request-pipeline.md). |
| **Middleware** (`middlewares/`, UI: **Middleware**) | One reusable policy object in that chain: `auth`, `headers`, `guard`, `rate-limit`, `rewrite` or `bouncer`. |
| **Inline block** | An `auth` or `rateLimit` block written directly on a host or location instead of referencing a middleware. Same handler, same chain position. |

## Access and identity

| Term | Means |
|---|---|
| **Access list** (`access-lists/`, UI: **Access Lists**) | The gate: ordered IP/CIDR allow-deny rules plus optional GeoIP country rules over the derived client IP. Never abbreviated to "ACL" in gpm. |
| **Exemption** (`allowFrom`) | A CIDR list that lets a network skip **one** control - a guard, a rate limit, a bouncer verdict or one auth mode. Never the access list. |
| **Trusted proxy** | A peer whose `X-Forwarded-For` gpm believes when deriving the client IP. Distinct from `proxyProtocol.trustedCIDRs` (who may rewrite the connection address) and `forwardAuth.trustedProxies` (who may assert identity headers). |
| **Identity provider** (`identity-providers/`, UI: **Identity Providers**), **IdP** | An OIDC issuer, a trusted forward-auth proxy, or an `auth_request` outpost, plus its group-to-role mapping. An IdP on its own has no data-plane effect until an auth gate references it. |
| **Forward-auth** | Trusting identity headers (`Remote-User` and friends) asserted by a proxy in front of gpm. |
| **Auth-request** | An nginx `auth_request`-style subrequest to an external endpoint that answers allow or deny. Distinct from forward-auth: gpm calls out rather than reading headers. |
| **Outpost** | The external service an `auth-request` gate calls, e.g. an Authentik proxy outpost. gpm proxies its sign-in, callback and sign-out endpoints verbatim. |
| **Client CA** (`client-cas/`, UI: **Client CAs**) | The trust anchor presented client certificates are verified against, plus its optional CRL and optional signing key. Verifies peers; a `Certificate` identifies this server. |
| **API token** (`api-tokens/`, UI: **API Tokens**) | A scoped bearer credential for automation. The secret is shown once and never stored; only its SHA-256 digest is committed. |

## Configuration and automation

| Term | Means |
|---|---|
| **Object** | One typed YAML file under `config/<kind>/<name>.yaml`. The filename equals the object's `name`. |
| **Settings** | The singleton `config/settings.yaml`: fleet-wide defaults and subsystem configuration. |
| **Reference** (`...Ref`) | A field naming another object by name. A dangling reference is a load-time error, and an object cannot be deleted while another references it. |
| **Template** | The default chain a discovery reconciler applies to every host it derives. Structurally identical to a profile. |
| **Profile** | A named alternative template a discovery source may select **by name only**. Applied verbatim, never merged with the template. |
| **Reconcile** | One full-state pass of a reconciler: recompute the desired set from the whole config, compare with reality, apply the difference as one commit. |
| **Ledger** | A committed record of what gpm owns: `config/dns-ledger.yaml` for DNS records, `config/access-list-sources.yaml` for fetched IP feeds. It is what authorises a deletion. |
| **Adopted** | A record that already existed and gpm claimed rather than created. An adopted record is released, never deleted or retargeted. |
| **Apex target** | The single CNAME target every DNS-sync-managed record points at. Not an ownership marker. |
| **Freeze** | What a discovery reconciler does when it cannot read its source: no creates, no updates, no deletes, managed hosts left exactly as they are. |
| **Fail closed** | Refusing rather than admitting when a decision cannot be made. The default everywhere except the bouncer's `onError`. |

## Availability

| Term | Means |
|---|---|
| **Maintenance** | A host (or the whole edge) answering `503` with `Retry-After` and never dialling the upstream. Domains, certificate and DNS records are kept. |
| **Disabled** | An object kept in config but excluded from the running data plane. Its domain is released and its DNS records are withdrawn. |
| **Leader / follower** | The HA roles. A leader runs ACME and the discovery reconcilers and accepts writes; a follower refuses writes and pulls the leader's config repo. |

## UI-only labels

Three settings groups and two views have no config object behind them; they are
navigation labels, not kinds.

| UI label | What it edits |
|---|---|
| **Overview** | The Settings landing view: instance identity and admin authentication. |
| **Security** | The Settings group holding trusted proxies, security headers and header stripping. |
| **Integrations** | The Settings group holding DNS sync, discovery, webhooks and notifications. |
| **Advanced** | The Settings group holding maintenance, PROXY protocol and the remaining low-level keys. |
| **Operations** | The Settings group holding runtime facts, metrics and profiling status. |
| **Error Pages** | `settings.errorPages` and a host's own override. There is no `error-pages` object kind or API route. |
| **History** | `git log` over the config repo, through `GET /api/history`. Not stored config. |
| **Access Logs** | The in-memory data-plane request log and its runtime toggle (`PUT /api/logs`). Not stored config. |
