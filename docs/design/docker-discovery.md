# Design: Docker container discovery

Derives managed proxy hosts from labelled Docker containers, the way
[Ingress discovery](ingress-discovery.md) derives them from annotated
Kubernetes `Ingress` objects. This note records only what is **shared** and what
**differs**; every rule not listed as differing is the Ingress design's,
unchanged.

## Why

- The homelab audience gpm targets runs Docker Compose far more often than
  Kubernetes.
- Hand-writing a proxy host per container is the same repetitive step Ingress
  discovery already removed for clusters.
- The security model that makes Ingress discovery safe (opt-in, name-only
  profile selection, operator-owned templates, ownership-gated writes) is not
  Kubernetes-specific.

## Shared vs differs

| Concern | Shared with Ingress discovery | Differs |
|---------|-------------------------------|---------|
| Reconcile engine | `internal/discovery.Plan` - the whole full-state plan: ownership by name and domain, freeze-on-derive-failure, fail-closed on an unresolvable profile, operator-owned `disabled`/`maintenance` carry-forward, one commit per run | - |
| Template type | `model.IngressHostTemplate`, same fields, same validators | `upstream`/`upstreamGroupRef` are ignored (derived per container) and not required |
| Profiles | Same map type, same name-only selection, same "undefined name is a skip, never a downgrade" rule | `dockerDiscovery.profiles` empty inherits `ingressDiscovery.profiles` |
| Label/annotation prefix | `ingressDiscovery.annotationPrefix` (default `gpm.rake.pro`), including the prefix-migration refusal | - |
| Ownership marker | Key `<prefix>/managed-by`, and `<prefix>/disabled-by` for a self-placed hold | Value is `docker-discovery`, not `ingress-discovery` |
| Domain validation | `model.IsHostname` + `allowedDomainSuffixes`, per-profile narrowing included | Hostnames arrive in one comma-separated label instead of `spec.rules[].host` |
| Freeze rule | A managed host is deleted only after a complete, successful listing | A non-array body is the "not an `IngressList`" check's counterpart |
| Client | Plain `net/http` + `encoding/json`, read-only, bounded reads, no redirects, link-local refused | Unix socket (or `tcp://`/`https://`) instead of an HTTPS API server with a bearer token |
| Trigger | Full-state reconcile, poll interval with a 15s floor | Engine event stream drives the normal case, debounced 2s; the poll is the fallback |
| Derived name | `<prefix>-<source>` shape, `ValidateName`-checked | `dkr-<container name>` (lowercased), vs `ing-<name>.<namespace>` |
| HA | Leader-only writer; a follower runs neither reconciler | - |

## Decisions

- **Two label values, one key.** Both reconcilers stamp `<prefix>/managed-by`,
  and each recognises ownership by exact value. A host derived from an `Ingress`
  is invisible to the container reconciler and vice versa, so the two can run on
  one instance without ever competing for an object.
- **One shared planner, not two copies.** Every rule with a security
  consequence lives in `internal/discovery`; the per-source packages only derive
  and project results onto their own wire shape. Two copies of the ownership and
  freeze rules would be two places to get them wrong.
- **The upstream is the container's own address.** A label may name a port and
  a scheme within its own container, never an arbitrary address, so a compose
  file cannot aim gpm at somebody else's machine. Which address (container IP or
  published port) is an operator setting, not a label.
- **Events are latency, not correctness.** The stream only says "look sooner".
  A stream that is missing, blocked by a socket proxy, or broken degrades to the
  poll interval and nothing else changes.
- **Read-only by construction.** The client can issue exactly `GET /version`,
  `GET /containers/json` and `GET /events`; there is no code path that can POST
  to the Engine. That is what makes a read-only socket proxy an exact fit.

## Non-goals

- **Swarm services and Kubernetes-style path routing.** One container, one host,
  by vhost.
- **`locations` from labels.** Path-scoped chains from an untrusted label are
  the self-service privilege grant the annotation model already refuses.
- **Writing to the Engine.** gpm never starts, stops, creates or execs anything.
- **Inspecting containers individually.** `/containers/json` carries the labels,
  the ports and the per-network addresses; a per-container `/inspect` would add
  N requests per poll for nothing.
