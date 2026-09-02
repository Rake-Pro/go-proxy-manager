# Settings: trusted proxies

Which L7 proxies' `X-Forwarded-For` gpm believes when it derives the client
IP every access list, rate limit and exemption compares.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-trusted-proxies"></span> `trustedProxies` | []string | CIDRs or bare IPs of the L7 proxies whose `X-Forwarded-For` is believed. Empty (default) trusts nobody: the connection peer is the client. A [ProxyHost](../proxy-host.md)'s own `trustedProxies` **replaces** this list for that host. See [Client IP and the three trust tiers](../../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers). |

## Per-host override: absent is not the same as empty

`proxyHost.trustedProxies` is a **nullable** list, and the three states differ:

| Value | Meaning |
|---|---|
| key omitted (or `null`) | Inherit `settings.trustedProxies`. |
| `trustedProxies: []` | Trust nobody for this host, whatever the fleet default is. `X-Forwarded-For` is not read and the connection peer is the client. |
| `trustedProxies: [10.0.0.0/8]` | Replace the fleet list for this host (never extend it). |

```yaml
# config/settings.yaml - the fleet sits behind one edge proxy
trustedProxies: [192.0.2.10/32]
```

```yaml
# config/proxy-hosts/direct.yaml - this one host is reached directly
name: direct
domains: [direct.example.com]
upstream: {scheme: http, host: 192.0.2.20, port: 8080}
trustedProxies: []            # trust nobody here, despite the fleet default
```
