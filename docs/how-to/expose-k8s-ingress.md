# Expose a Kubernetes Ingress through gpm

Turn annotated cluster `Ingress` objects into managed proxy hosts, read-only
and opt-in.

Turns annotated cluster `Ingress` objects into managed proxy hosts, which then
feed the DNS sync above. Configure it under **Settings -> Kubernetes Ingress
discovery** (full field reference in
[Settings: Kubernetes Ingress discovery](../reference/config/settings/ingress-discovery.md),
rationale in [design/ingress-discovery.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/design/ingress-discovery.md)).

## Prerequisites

- A `Certificate` that already covers the hostnames the Ingresses will publish,
  plus every `Middleware`, `AccessList` and `UpstreamGroup` the template or a
  profile names. Discovery issues no certificate and creates no object it
  references: a dangling name fails the first reconcile's whole batch.
- Cluster credentials for a ServiceAccount that can `list` `ingresses`, and
  nothing else.
- The domain suffixes the derived hostnames must fall inside
  (`allowedDomainSuffixes`), which is required whenever discovery is enabled.
- The ingress controller's address, reachable from the edge host over the LAN.

## Steps

**1. Apply the RBAC. gpm reads the cluster; it never writes to it.** The shipped role
grants `list` on `ingresses` and nothing else: the reconciler works from a full
list on a poll interval, so it never reads an object by name and never opens a
watch:

```
kubectl apply -f deploy/k8s-ingress-discovery-rbac.yaml
```

**2. Tighten it when you scope to one namespace.** If you set
`settings.ingressDiscovery.namespace`, gpm lists a single namespace, so replace
the shipped `ClusterRole`/`ClusterRoleBinding` with a `Role`/`RoleBinding` in
that namespace, same single `list` verb, without cluster-wide read on every
`Ingress` in the cluster.

**3. Extract the credential** for the (normal) off-cluster deployment, where gpm
runs on the edge host and there is no kubelet to project a token into it. Create
each file `0400` **first**: a plain redirect creates it `0644`, leaving a window
in which any local user can read the bearer token.

```
install -m 0400 /dev/null /run/secrets/gpm-k8s-token
install -m 0400 /dev/null /run/secrets/gpm-k8s-ca.crt
kubectl -n gpm-discovery get secret gpm-ingress-discovery-token \
  -o jsonpath='{.data.token}' | base64 -d > /run/secrets/gpm-k8s-token
kubectl -n gpm-discovery get secret gpm-ingress-discovery-token \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > /run/secrets/gpm-k8s-ca.crt
```

The `ca.crt` jsonpath escapes the dot **inside the key** only. `{.data\.ca\.crt}`
escapes the separator as well, matches nothing, and silently writes an empty CA
file, which surfaces later as `caFile ... contains no usable PEM certificate`.

Mount both into the container and point `tokenFile` / `caFile` at them. gpm
re-reads the token from disk every 5 minutes (and immediately after a `401`), so
replacing the file rotates the credential with no restart. If you instead run gpm
*as a pod in the cluster*, leave `apiURL`, `tokenFile` and `caFile` empty: the
projected ServiceAccount values are used automatically.

**4. Annotate the Ingresses you want published**, and only those:

```yaml
metadata:
  annotations:
    gpm.rake.pro/managed: "true"
    gpm.rake.pro/profile: "sso-internal" # optional, names a settings profile
    gpm.rake.pro/lan-direct: "true"      # optional, overrides the profile's defaultDNS
    gpm.rake.pro/public-cname: "false"   # optional
```

`gpm.rake.pro/profile` names one of `settings.ingressDiscovery.profiles`; omit it
and the default `template` applies. Naming a profile that is not defined
**skips** the Ingress (visible in the status below) rather than falling back to
the default, so a typo shows up instead of silently changing a host's middleware
or access-list chain. The annotation can only carry a *name*: there is
deliberately no annotation that names a middleware, access list, certificate or
upstream, because an Ingress author is untrusted. See
[Settings: Kubernetes Ingress discovery](../reference/config/settings/ingress-discovery.md#discovery-profiles).

**5. Before you move an existing host into discovery, diff it against the template.**
The template and every profile carry the same fields a hand-written proxy host
does (including `robotsNoIndex`, `timeouts` and `tags`) but they are *template*
fields, so anything the chosen profile does not set is not set on the derived
host either. Check the host you are retiring for `robotsNoIndex`, a `timeouts`
override and `tags`, and put them on the profile you are cutting over to.
`locations` is the one thing that has no template equivalent by design: if the
host you are moving has them, leave it hand-written (discovery never touches an
unlabelled host) or publish the paths as their own annotated `Ingress`. After the
cutover, `GET /api/ingress-discovery/status` names the profile each host
resolved to; the derived object itself is in git, so `git show` on the reconcile
commit is the authoritative diff.

```
# Reconcile on demand and read the result.
curl -s -X POST https://<admin>/api/ingress-discovery/reconcile -H 'Authorization: Bearer gpm_...' | jq
curl -s          https://<admin>/api/ingress-discovery/status    -H 'Authorization: Bearer gpm_...' | jq
# {"enabled":true,"lastRun":"...","lastSuccess":"...","discovered":7,"managed":7,
#  "created":0,"updated":0,"deleted":0,"skipped":0,"hosts":[...]}
```

**Deploy ordering.** The template's `certificateRef` must name a `Certificate`
that already exists, and any `middlewares`/`accessLists` must exist too,
otherwise the first reconcile's batch fails referential integrity and writes
nothing (reported in `status.error`). Create those objects, *then* enable
discovery.

## Verify

| Check | Expected on the first run | Expected on the second |
|---|---|---|
| `created` | The number of annotated Ingresses whose hosts pass validation | `0` |
| Every entry in `hosts[]` | `created`, each with its resolved `profile` | `unchanged` |
| `GET /api/history` | One `Ingress discovery: reconcile` commit | No new commit |
| `lastRun` vs `lastSuccess` | Equal | Equal, a gap means the run froze |
| `hosts[].action: "skipped"` | Each carries a `reason` | Same |

**Ownership covers the domain, not just the name.** A derived host is skipped
whenever any of its domains is already claimed by a host discovery does not own
(including a *disabled* one), so an annotated Ingress cannot take over the
hostname of your SSO or dashboard host by deriving a name that happens to sort
after it. Two annotated Ingresses claiming the same hostname are resolved the
same way: the first by derived name wins, the second is skipped with a reason.
The rule is also enforced one layer down: the config validator rejects any two
*enabled* hosts claiming the same domain, whatever wrote them.

**Freeze on failure is expected behaviour, not an outage.** If the API server is
unreachable, returns an error, returns something that is not an `IngressList`, or
a paginated list fails part-way, the run aborts before any write and the managed
hosts stay exactly as they are. One list is bounded to two minutes end to end, so
a hung API server fails the run instead of stalling the reconciler. `status.error`
says why and `lastSuccess` says how stale the state is; watch that pair, not
`lastRun`, when alerting. The only condition that deletes a managed host is a
*complete, successful* list that no longer derives it (which includes an Ingress
that simply lost its annotation).

**A misdirected `apiURL` cannot empty your config.** A `200` from something that
is not the Kubernetes API (another HTTPS service behind the same internal CA, a
mesh or gateway envelope) is rejected as a shape error rather than decoded as an
empty list, so it lands on the freeze path instead of deleting every managed host.

**Upstream gotcha.** `template.upstream` is the **ingress controller's** address,
not a Service: gpm is off-cluster, so `*.svc.cluster.local` cannot be resolved or
reached. Use `scheme: http` to the controller's plain port unless you have a
reason not to: with `https` the upstream host is what SNI and certificate
verification use, so a bare IP will fail the handshake.

Scopes: `ingress-discovery:read` for status, `ingress-discovery:write` for
reconcile.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Ingress skipped: hostname outside `allowedDomainSuffixes` | The name is not inside the configured bound | Add the suffix, or fix the Ingress |
| Ingress skipped: the name is already taken | A hand-written proxy host, or one from another reconciler, already owns that name | Rename, or remove the other host |
| Ingress skipped: the **domain** is already claimed | Another host (proxy, redirect or parked, enabled *or* disabled) already serves that hostname | Resolve the collision; this rule is what stops a tenant claiming your SSO hostname |
| Host disabled with `profile ... is not defined` | The profile was renamed or retired; an unresolvable profile fails closed so a revoked chain cannot be pinned | Re-add the profile, or fix the annotation |
| `lastSuccess` far behind `lastRun`, hosts unchanged | The run froze: a transport error, non-`200`, decode failure, or a body that is not an `IngressList` | Read `status.error`; nothing was deleted, and the state is stale rather than wrong |
| `caFile ... contains no usable PEM certificate` | The `ca.crt` jsonpath escaped the separator and wrote an empty file | Use `{.data.ca\.crt}`, escaping only the dot inside the key |
| The first reconcile writes nothing and reports an error | The template names a certificate, middleware or access list that does not exist | Create those objects, then enable discovery |
| A derived host serves a `502` to the controller | `template.upstream` points at a Service rather than the controller, or an `https` upstream names a bare IP | Point it at the controller's address; prefer `scheme: http` |
