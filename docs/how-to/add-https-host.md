# Add an HTTPS host

Add one more proxied domain to a running instance that already has a
certificate strategy. For the very first host, start at
[Your first host with HTTPS](../getting-started/first-host-with-https.md).

## Prerequisites

- An admin session, or an API token holding `proxy-hosts:write` (plus
  `certificates:write` if you also issue a certificate).
- A DNS record for the domain pointing at the edge, or
  [DNS sync](dns-sync.md) configured so gpm publishes it.
- A certificate that already covers the domain, or a
  [DNSProvider](../reference/config/dns-provider.md) to issue one.

## Steps

1. **Pick the certificate.** A wildcard already covering the name needs nothing
   new. Otherwise create a `Certificate` object first: selection is by SNI, so
   the certificate must exist before the host serves HTTPS.
2. **Create the proxy host:**

   ```yaml
   # config/proxy-hosts/grafana.yaml
   name: grafana
   domains: [grafana.example.com]
   upstream: {scheme: http, host: 10.0.0.40, port: 3000}
   tls: {certificateRef: wildcard, forceSSL: true}
   ```

3. **Attach access control**, if the host is not meant to be public:

   ```yaml
   accessLists: [home-vpn]
   ```

   See [Which mechanism do I use?](../concepts/which-mechanism.md) to choose
   between an access list, an exemption and an auth gate.

4. **Attach authentication**, if the host needs a login. An inline block avoids
   creating a middleware object for a single host:

   ```yaml
   auth:
     identityProvider: authentik
     mode: forward-auth
     requiredRoles: [staff]
   ```

   See [Per-host SSO](per-host-sso.md) and [Basic auth](basic-auth.md).

5. **Add path-scoped overrides**, if one prefix needs a different backend or a
   tighter chain:

   ```yaml
   locations:
     - path: /metrics
       accessLists: [internal-only]
   ```

6. **Publish DNS**, if [DNS sync](dns-sync.md) is enabled:

   ```yaml
   dns: {lanDirect: true, publicCname: true}
   ```

## Verify

| Check | Expected |
|---|---|
| `curl -sI https://grafana.example.com/` | `200`/`3xx`, certificate covers the name |
| `curl -s -o /dev/null -w '%{http_code}\n' http://grafana.example.com/` | `308` |
| `GET /api/history` | One new commit naming the host |
| A denied client | The status your access list or auth gate defines, rendered through the configured error page |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: two hosts claim the same domain | Another **enabled** host already lists this domain | Disable or edit the other host in the same change |
| `503` on every request to the host | The host references a middleware or access list that does not exist | Fix the reference; the data plane fails one host closed rather than serving it open |
| `404` for the domain | No enabled host claims it, or the request's `Host` header does not match | Check `domains` and that the host is not `disabled` |
| Requests reach the backend at the wrong path | Path composition order | See [Path composition](../concepts/request-pipeline.md#path-composition) |
| An access list allows or denies the wrong clients | The compared address is not what you assumed | See [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers) |
