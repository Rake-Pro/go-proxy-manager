# Settings

Singleton application configuration in `config/settings.yaml`. Each key below
has its own reference page; the complete example at the end shows them together.

## Sections

| Section | Keys | Page |
|---|---|---|
| General | `schemaVersion`, `appName`, `externalBaseURL` | [General](general.md) |
| Admin authentication | `adminAuth.*` | [Admin authentication](admin-auth.md) |
| Trusted proxies | `trustedProxies` | [Trusted proxies](trusted-proxies.md) |
| Security headers | `securityHeaders`, `stripResponseHeaders` | [Security headers](security-headers.md) |
| Error pages | `errorPages` | [Error pages](error-pages.md) |
| Maintenance | `maintenance` | [Maintenance mode](maintenance.md) |
| DNS sync | `dnsSync` | [DNS sync](dns-sync.md) |
| Ingress discovery | `ingressDiscovery` | [Kubernetes Ingress discovery](ingress-discovery.md) |
| Docker discovery | `dockerDiscovery` | [Docker container discovery](docker-discovery.md) |
| Access-list sync | `accessListSync` | [Access-list source sync](access-list-sync.md) |
| Webhooks | `webhooks` | [Webhooks](webhooks.md) |
| Notifications | `notifications` | [Notifications](notifications.md) |
| PROXY protocol | `proxyProtocol` | [PROXY protocol](proxy-protocol.md) |

## Complete example

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
accessListSync:
  enabled: true
  pollInterval: 15m
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
    middlewares: [sso]
    robotsNoIndex: true             # same field a hand-written host has
    timeouts: { connectSeconds: 5, readSeconds: 60 }
    tags: [cluster]
    defaultDNS: { lanDirect: true }
  profiles:                         # optional named chains, selected per Ingress
    public-ratelimited:
      upstream: { scheme: http, host: 10.0.0.40, port: 80 }
      tls: { certificateRef: wildcard, forceSSL: true }
      middlewares: [rate-limit]     # and no access list: public on purpose
```
