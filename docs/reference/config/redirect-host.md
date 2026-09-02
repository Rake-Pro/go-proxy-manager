# RedirectHost

Issues HTTP redirects.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="redirect-host-domains"></span>  `domains` | []string | yes | Source hostnames. |
| <span id="redirect-host-target-domain"></span>  `targetDomain` | string | yes | Where to redirect. |
| <span id="redirect-host-target-scheme"></span>  `targetScheme` | string | no | `http` \| `https` \| `auto`. Unset behaves as `auto`, which mirrors the scheme the request arrived on. |
| <span id="redirect-host-status-code"></span>  `statusCode` | int | no | `301` \| `302` \| `307` \| `308`. Unset (or `0`) sends `301`. |
| <span id="redirect-host-preserve-path"></span>  `preservePath` | bool | no | Keep the request path. |
| <span id="redirect-host-tls"></span>  `tls` | TLSSettings | no | Same shape as [ProxyHost `tls`](proxy-host.md#proxy-host-tls) - `certificateRef`, `forceSSL`, `minTLSVersion`, `hsts.*` and `clientAuth.*` all apply. Note `certificateRef` is an intent record here too: the served certificate is selected by SNI. |

```yaml
name: apex
domains: [example.com]
targetDomain: www.example.com
statusCode: 301
preservePath: true
```

---
