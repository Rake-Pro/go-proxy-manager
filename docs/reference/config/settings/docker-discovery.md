# Settings: Docker container discovery

Derive managed proxy hosts from labelled Docker containers, on the same
reconciler and the same profile contract as Ingress discovery.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-docker-discovery"></span> `dockerDiscovery` | DockerDiscoverySettings | Optional Docker container discovery (below). Shares `ingressDiscovery`'s label prefix and profiles. |

Discovers labelled Docker containers and reconciles them into template-derived
proxy hosts, which then feed the DNS sync above. Disabled (the default) means
the subsystem is inert and never opens the socket.

It is the same machinery as
[`ingressDiscovery`](ingress-discovery.md) with a
different source: the same `template` shape, the same profiles, the same
ownership rules, the same freeze-on-unreadable behaviour. Only the four
Docker-specific items below differ. Design note:
[Design: Docker container discovery](https://github.com/Rake-Pro/go-proxy-manager/blob/main/design/docker-discovery.md).

- **Same label prefix.** Keys come from `ingressDiscovery.annotationPrefix`
  (default `gpm.rake.pro`); there is no second prefix knob.
- **Same profiles.** `dockerDiscovery.profiles` empty means
  `ingressDiscovery.profiles`; a profile's `upstream`/`upstreamGroupRef` is
  ignored here because the upstream is the container's own address.
- **Different ownership value.** Derived hosts carry
  `gpm.rake.pro/managed-by: docker-discovery`, so the two reconcilers can run
  side by side and neither can update or delete the other's hosts.
- **Event-driven.** Reconciles run on the Engine event stream (debounced 2s),
  with the poll interval as the fallback that guarantees correctness.

## Settings

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="settings-docker-discovery-enabled"></span> `enabled` | bool | `false` | no | Turn container discovery on. Everything below is validated only when this is true. |
| <span id="settings-docker-discovery-socket"></span> `socket` | string | `/var/run/docker.sock` | no | Docker Engine API unix socket, absolute path. Ignored when `host` is set. |
| <span id="settings-docker-discovery-host"></span> `host` | string | none | no | `tcp://` or `https://` Engine endpoint used instead of the socket, e.g. `tcp://socket-proxy:2375`. Recommended: see [Discover Docker containers](../../../how-to/docker-discovery.md). |
| <span id="settings-docker-discovery-tls-cert"></span> `tlsCert` | string | none | no | Absolute path to a client certificate for an `https://` `host`. Set with `tlsKey`. |
| <span id="settings-docker-discovery-tls-key"></span> `tlsKey` | string | none | no | Absolute path to the client key. Set with `tlsCert`. |
| <span id="settings-docker-discovery-tls-ca"></span> `tlsCA` | string | none | no | Absolute path to the PEM bundle verifying the endpoint. There is no skip-verify option. |
| <span id="settings-docker-discovery-network"></span> `network` | string | first non-`host` network | no | Docker network whose per-container IP becomes the upstream host. Mutually exclusive with `usePublishedPorts`. |
| <span id="settings-docker-discovery-use-published-ports"></span> `usePublishedPorts` | bool | `false` | no | Forward to the host-published port instead of the container IP. |
| <span id="settings-docker-discovery-published-host"></span> `publishedHost` | string | `127.0.0.1` | no | Address a published port is reached on. Only valid with `usePublishedPorts`. |
| <span id="settings-docker-discovery-include-stopped"></span> `includeStopped` | bool | `false` | no | List non-running containers too. A stopped container has no address, so its host is skipped and frozen rather than derived. |
| <span id="settings-docker-discovery-poll-interval"></span> `pollInterval` | duration | `1m` | no | Fallback loop interval, minimum `15s`. Events drive the normal case. |
| <span id="settings-docker-discovery-allowed-domain-suffixes"></span> `allowedDomainSuffixes` | []string | none | **yes** | Bounds every hostname a label can publish. A derived domain must equal one of these or end in `.` + one of them. |
| <span id="settings-docker-discovery-template"></span> `template` | IngressHostTemplate | none | **yes** | The default chain every derived host takes. Same shape and validation as `ingressDiscovery.template`, except `upstream`/`upstreamGroupRef`, which are derived per container. |
| <span id="settings-docker-discovery-profiles"></span> `profiles` | map[string]IngressHostTemplate | `ingressDiscovery.profiles` | no | Named chains a container may select by name. Empty inherits the Ingress block's set. `template` is reserved. |

**`template` and every `profiles` entry carry the full ProxyHost shapes.** The
`upstream` is derived per container rather than templated, but every other
sub-key is settable and applied verbatim:

| Template key | Shape | Sub-keys documented at |
|---|---|---|
| `template.tls` | `TLSSettings` | [ProxyHost: `tls`](../proxy-host.md#proxy-host-tls): `certificateRef`, `forceSSL`, `minTLSVersion`, `hsts.*`, `clientAuth.*` |
| `template.tls.hsts` | `HSTS` | [ProxyHost: `tls.hsts`](../proxy-host.md#proxy-host-tls): `enabled`, `maxAge`, `includeSubdomains`, `preload` |
| `template.tls.clientAuth` | `ClientAuth` | [ProxyHost: `tls.clientAuth`](../proxy-host.md#proxy-host-tls): `caRef`, `mode`, `identityHeaders.*` |
| `template.timeouts` | `HostTimeouts` | [ProxyHost: `timeouts`](../proxy-host.md#proxy-host-timeouts): `connectSeconds`, `readSeconds` |
| `template.defaultDNS` | `DNSSyncPolicy` | [ProxyHost: `dns`](../proxy-host.md#proxy-host-dns): `lanDirect`, `publicCname` |

Every other template key (`middlewares`, `accessLists`, `robotsNoIndex`, `tags`,
`stripResponseHeaders`, `securityHeaders`, `allowedDomainSuffixes`) is documented on
[Kubernetes Ingress discovery](ingress-discovery.md), which uses the identical
type.

## Container labels

Read off the container; prefixed with `ingressDiscovery.annotationPrefix`
(default `gpm.rake.pro`).

| Label | Required | Meaning | Example |
|-------|----------|---------|---------|
| `gpm.rake.pro/enabled` | **yes** | Opt in. Exactly `"true"`; anything else (or absent) is invisible to discovery. | `"true"` |
| `gpm.rake.pro/domains` | **yes** | Comma-separated hostnames to serve. Lowercased, de-duplicated, sorted; each must be a valid LDH hostname inside `allowedDomainSuffixes`. Wildcards are rejected. | `"grafana.example.com,stats.example.com"` |
| `gpm.rake.pro/port` | conditional | Container port to forward to. Required unless the container exposes exactly one TCP port. | `"3000"` |
| `gpm.rake.pro/scheme` | no | Upstream scheme, `http` (default) or `https`. | `"http"` |
| `gpm.rake.pro/profile` | no | Name of one `profiles` entry. Absent uses `template`; an undefined name **skips** the container. | `"public-ratelimited"` |
| `gpm.rake.pro/lan-direct` | no | Sets `dns.lanDirect` on the derived host, overriding the profile's `defaultDNS`. | `"true"` |
| `gpm.rake.pro/public-cname` | no | Sets `dns.publicCname` on the derived host. | `"false"` |

A label can never carry a middleware, an access list, a certificate or an
upstream address of its own: it selects a chain the operator wrote and names an
address **within its own container**. That containment is the same one the
Kubernetes annotation contract has, and for the same reason.

## Where a derived host forwards

Two modes, one choice, decided by where gpm itself runs.

| Mode | Setting | Upstream becomes | Use when |
|------|---------|------------------|----------|
| Container IP (default) | `network: <name>` or unset | `<scheme>://<container IP on that network>:<port label>` | gpm runs **in Docker** and shares a network with the containers. |
| Published port | `usePublishedPorts: true` | `<scheme>://<publishedHost>:<host port>` | gpm runs **on the Docker host** (or the containers publish ports and share no network with gpm). |

- Container IP needs no published ports at all, which keeps the services off the
  host's interfaces entirely.
- Published ports need every discovered service to publish a host port, which is
  a wider exposure but works without gpm joining each stack's network.
- Docker's per-container IPs are not stable across recreation: a reconcile
  refreshes them, and between reconciles a recreated container is briefly
  unreachable. The event stream keeps that window at seconds.
- `network: host` containers have no per-container address; use
  `usePublishedPorts` for them.

## Derived objects and ownership

- Each opted-in container produces one proxy host named `dkr-<container name>`
  (lowercased), e.g. `dkr-grafana`, with `displayName` set to the container name.
- Every derived host carries
  `labels["gpm.rake.pro/managed-by"] = "docker-discovery"`.
- Only objects carrying that exact label pair are created, updated or deleted. A
  hand-written host (or an Ingress-derived one) with the same name is skipped
  with a warning, never overwritten.
- Ownership covers the **domain** as well as the name: a derived host whose
  domains include one already claimed by a host this reconciler does not own is
  skipped, with the owner named in the status `reason`.
- `disabled` and `maintenance` are operator-owned and carried forward across a
  reconcile, exactly as for Ingress discovery.
- Two containers deriving the same name: first by derived name wins, the rest
  are skipped.

## Freeze, fail-closed and commits

- **Unreadable Engine means freeze.** A managed host is deleted only when a
  reconcile obtained a complete, successful list. Any transport error, non-`200`,
  decode failure, or a `200` whose body is not a JSON array aborts the run
  before any write.
- **A bad label freezes one host.** A container that cannot be derived (bad
  hostname, ambiguous port) is skipped **and** protects its existing host from
  deletion.
- **An unresolvable profile fails closed.** The existing host is updated with
  `disabled: true` and `gpm.rake.pro/disabled-by: docker-discovery`, so a
  retired chain cannot be pinned by naming a profile that no longer exists.
  Re-adding the profile re-enables it on the next reconcile.
- **One commit per reconcile**, authored by `docker-discovery`
  (`Docker discovery: reconcile (+N ~M -K)`). A reconcile that finds no drift
  writes nothing.
- Discovery publishes no DNS itself: it sets the `dns` policy on derived hosts
  and asks the phase-1 reconciler for a run.
