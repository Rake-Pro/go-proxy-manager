# Discover Docker containers

Turn labelled Docker containers into managed proxy hosts, over a read-only
Engine API connection. Configure it under **Integrations -> Docker discovery**; the
full field and label reference is
[Settings: Docker container discovery](../reference/config/settings/docker-discovery.md)
and the rationale is [design/docker-discovery.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/design/docker-discovery.md).

## Prerequisites

- gpm able to reach the Docker Engine API read-only. A socket proxy is the
  recommended shape: see [Engine access](#engine-access) below.
- A certificate that already covers the hostnames the labels will publish;
  discovery never issues one.
- Every middleware, access list and upstream group the template or a profile
  names must already exist, or the first reconcile fails referential integrity
  and writes nothing.
- The domain suffixes those hostnames must fall inside
  (`allowedDomainSuffixes`).

## Engine access

gpm issues exactly three GETs and has no code path that writes to the Engine:

| Request | Used for |
|---|---|
| `GET /version` | API version negotiation only. |
| `GET /containers/json?filters={"label":["gpm.rake.pro/enabled=true"]}` | The full-state list every reconcile is computed from. |
| `GET /events?filters={"type":["container"],...}` | A lifecycle stream that only says "reconcile sooner". Optional: without it the poll interval still keeps the config correct. |

**The Docker socket is a root-equivalent credential.** Anything that can write to
it can start a privileged container and own the host. Pick one of:

| Option | Shape | Trade-off |
|---|---|---|
| **Read-only socket proxy** (recommended) | `host: tcp://socket-proxy:2375`, proxy allows `CONTAINERS` and `EVENTS` only | gpm never sees a writable socket |
| Direct socket mount | `-v /var/run/docker.sock:/var/run/docker.sock:ro` | The read-only flag bounds the file, not the API |
| Remote Engine over TLS | `host: https://docker.example.com:2376` with `tlsCA` (plus `tlsCert`/`tlsKey` for mutual TLS) | No skip-verify option; link-local destinations are refused at connect time |

**Never publish the raw socket over plain `tcp://`**: that hands the credential
to anyone who can reach the port.

```yaml
services:
  socket-proxy:
    image: ghcr.io/example/docker-socket-proxy:1.0.0   # pin a digest or semver
    environment:
      CONTAINERS: "1"     # GET /containers/json
      EVENTS: "1"         # GET /events
      POST: "0"           # refuse every write
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks: [edge]
  gpm:
    image: ghcr.io/rake-pro/go-proxy-manager:1.3.0     # pin a digest or semver
    networks: [edge]
```

## Steps

1. **Give gpm read-only Engine access** using one of the options above.
2. **Add the settings block:**

   ```yaml
   dockerDiscovery:
     enabled: true
     host: tcp://socket-proxy:2375
     network: edge
     pollInterval: 60s
     allowedDomainSuffixes: [example.com]
     template:
       tls: { certificateRef: wildcard, forceSSL: true }
       accessLists: [home-vpn]
       robotsNoIndex: true
       tags: [docker]
       defaultDNS: { lanDirect: true }
     profiles:
       public-ratelimited:
         tls: { certificateRef: wildcard, forceSSL: true }
         middlewares: [rate-limit]
         tags: [docker, public]
         defaultDNS: { lanDirect: true, publicCname: true }
   ```

3. **Label the containers.** A label can only ever name a profile the operator
   wrote, and an address inside the container's own network:

   ```yaml
   services:
     grafana:
       image: grafana/grafana:11.6.0
       networks: [edge]
       labels:
         gpm.rake.pro/enabled: "true"
         gpm.rake.pro/domains: "grafana.example.com"
         gpm.rake.pro/port: "3000"
     paste:
       image: ghcr.io/example/paste:1.2.3
       networks: [edge]
       labels:
         gpm.rake.pro/enabled: "true"
         gpm.rake.pro/domains: "paste.example.com"
         gpm.rake.pro/port: "8080"
         gpm.rake.pro/profile: "public-ratelimited"
         gpm.rake.pro/public-cname: "true"
   networks:
     edge:
       external: true
   ```

4. **Preview before the first write:** `GET /api/docker-discovery/plan`
   (**Preview changes** in the settings UI).
5. **Reconcile:** `POST /api/docker-discovery/reconcile`, or wait for the next
   event or poll.

## Verify

| Check | Expected |
|---|---|
| `GET /api/docker-discovery/status` | `reachable: true`, `endpoint` as configured, `lastSuccess` close to `lastRun`, per-host actions and the resolved `profile` |
| The host list | One `dkr-<name>` host per labelled container, chipped as docker-managed |
| `POST /api/docker-discovery/reconcile` | Runs one on demand; **409 Conflict** while one is in flight, never queued |
| A second reconcile | Every host `unchanged`, and no new commit in `GET /api/history` |

Two operational rules worth knowing before you alert on any of this:

- **An unreadable Engine never empties the config.** A managed host is deleted
  only after a complete, successful list that no longer derives it; everything
  else freezes the current state.
- **Discovery is leader-only**, like Ingress discovery. A follower runs neither
  reconciler and refuses config writes.

Scopes: `docker-discovery:read` for status and plan, `docker-discovery:write`
for reconcile.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `reachable: false`, error names the socket path | The socket is not mounted, or the path is wrong | Mount `/var/run/docker.sock` read-only, or point `host` at the socket proxy |
| `status 403` from the endpoint | A socket proxy that does not allow `CONTAINERS` / `EVENTS` | Allow exactly those two, nothing else |
| `did not return a JSON array` | `host` points at something that is not the Engine API | Check the port, and that the proxy forwards `/version` and `/containers/json`. The run freezes rather than deleting anything |
| `lastSuccess` far behind `lastRun` | Every run since then has failed | Read `error`; managed hosts are frozen, not deleted, meanwhile |
| Container skipped: `gpm.rake.pro/port is required` | Zero or several exposed TCP ports | Set the `port` label explicitly |
| Container skipped: `outside allowedDomainSuffixes` | The hostname is not inside the configured bound | Add the suffix, or fix the label |
| Container skipped: `not managed by docker discovery` | A hand-written or Ingress-derived host already has that name or domain | Rename the container, or remove the other host |
| Host disabled with `profile ... is not defined` | The profile was renamed or retired; an unresolvable profile fails closed | Re-add the profile, or fix the container's `profile` label |
| Host serves `502` after a container is recreated | The container IP changed between reconciles | Wait for the next event-driven reconcile, or use `usePublishedPorts` |
| The first reconcile writes nothing and reports an error | The template names a certificate, middleware or access list that does not exist | Create those objects, then enable discovery |
