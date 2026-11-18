# Settings: client limits

Bound how many requests one client address may have in progress at once on
the `:80`/`:443` proxy listeners.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-client-limits"></span> `clientLimits` | ClientLimitsSettings | Optional. Absent (the default): no limit, and nothing is counted. |

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="settings-client-limits-max-in-flight"></span> `maxInFlight` | int | no | Requests one client address may have in progress at a time, across every host and both listeners. `0` (the default) sets no limit. Maximum `1048576`. |
| <span id="settings-client-limits-allow-from"></span> `allowFrom` | []string | no | CIDRs or bare IPs that are never limited (monitoring, an internal load test, a NAT gateway you trust). |

- **What counts.** Every request, whoever answers it: a proxied response, an
  error page, a redirect, an auth refusal, the `404` for an unknown host. A
  request is counted from the moment the listener hands it over until its
  handler returns, including when the handler fails.
- **Upgraded connections count while open.** A WebSocket (or any other
  protocol switch) counts as one request for as long as the tunnel is open, so
  set the limit above the number of tunnels one client legitimately holds.
- **Who is a client.** The address every IP control compares: the connection
  peer, or the address taken from `X-Forwarded-For` when the peer is in
  [`trustedProxies`](trusted-proxies.md) (the host's own `trustedProxies` when
  the request names a proxy host that sets one). IPv6 addresses count per
  `/64`, the same grouping the rate limiter uses. Clients behind one NAT
  address share one budget.
- **Behind a load balancer or CDN.** Set [`trustedProxies`](trusted-proxies.md)
  or PROXY protocol for it. With neither, every request arrives from the load
  balancer's address, so all clients share one budget and the limit refuses
  them together.
- **Over the limit.** The request is refused with `429 Too Many Requests` and
  `Retry-After: 1`, through the host's error page for `429` when one is
  configured. It is counted in `gpm_denials_total` and in analytics with the
  reason `in-flight-limit`, and logged like any other response.
- **Bounded state.** An entry exists only while its client has a request in
  progress. The table holds at most 65536 client addresses at a time.
- **A full table fails open.** A new address that finds the table full is
  served without a count, and a warning is logged at most once a minute with
  the number of such requests. Addresses already in the table keep their own
  limit. The trade-off: an attacker that holds requests open from 65536
  addresses (one IPv6 `/48` is enough) escapes the limit for further
  addresses, but cannot use the full table to refuse everyone else.
- **Suggested value.** `256` for an instance that faces the internet: above
  what one browser holds open across several sites behind gpm (HTTP/2 allows
  100 streams per connection) plus a few WebSockets, while it bounds what one
  address can keep in progress. Raise it for clients behind a shared NAT
  address, or exempt them with `allowFrom`.
- A change applies to the next request; requests already in progress finish
  under the setting they started with.

```yaml
clientLimits:
  maxInFlight: 256
  allowFrom: [192.0.2.0/24, "2001:db8::/48"]
```
