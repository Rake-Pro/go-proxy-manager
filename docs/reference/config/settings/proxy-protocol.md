# Settings: PROXY protocol

Accept the HAProxy PROXY protocol on the proxy listeners so gpm behind
an L4 balancer sees the real client address.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-proxy-protocol"></span> `proxyProtocol` | ProxyProtocolSettings | Optional inbound PROXY protocol (below). |

Accepts the HAProxy **PROXY protocol** (v1 text and v2 binary) on the
`:80`/`:443` proxy listeners **and on every TCP stream listener**, so gpm behind an L4
load balancer (HAProxy, an AWS NLB with proxy protocol enabled, a Kubernetes
`Service` with `externalTrafficPolicy` behind an LB that sends it) sees the real
client address instead of the balancer's.

The parsed source address replaces the connection's `RemoteAddr`, which is the
address the [client-IP derivation](../../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers) starts
from, so every IP-based control sees the real client with no per-feature wiring.
This is the **L4** tier: it decides who may rewrite the connection address, not
whose `X-Forwarded-For` is believed (that is `settings.trustedProxies`).

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="settings-proxy-protocol-enabled"></span> `enabled` | bool | no | Default false: listeners are untouched. |
| <span id="settings-proxy-protocol-trusted-cidrs"></span> `trustedCIDRs` | []string | **yes when enabled** | CIDRs or bare IPs of the balancers whose header is believed. There is no "trust everyone" mode. |
| <span id="settings-proxy-protocol-timeout"></span> `timeout` | duration | no | Deadline for reading a complete header from a trusted peer. Default `5s`, maximum `1m`. |

**The header is an unauthenticated claim.** Anyone who can open the port can
assert any source address, so gpm parses it **only** when the TCP peer is inside
`trustedCIDRs`. From any other peer the bytes are treated as ordinary payload and
the peer address stands (logged once per peer at warn), otherwise enabling this
would hand every client a free source-IP spoof past every rule above. A malformed
header from a trusted peer closes the connection; a stalled one is cut at
`timeout`. A trusted peer that sends **no** header (the usual load-balancer TCP
health check) is served normally with its own address as the client IP.

v2 TLVs are consumed and ignored. `PROXY UNKNOWN` (v1), the v2 `LOCAL` command and
the v2 `AF_UNSPEC` family assert no address, so the real peer stands. There is no
UDP support: a UDP stream listener is unaffected by this setting.

> Turning this on when your balancer does **not** send the header will not break
> HTTP (the request bytes are simply not a PROXY header), but a *server-first*
> TCP stream backend fronted by a trusted peer that never speaks first will stall
> for `timeout` before the connection is closed. Enable it on the balancer first.

```yaml
proxyProtocol:
  enabled: true
  trustedCIDRs: [10.0.0.0/8, "2001:db8::/64"]
  timeout: 5s
```
