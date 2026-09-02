# Using gpm behind CGNAT: tunnels and relays

gpm has no native tunnel integration - these are recipes, not integrations.
Each pattern puts a third-party tool in front of gpm's own `:443`/`:80`
listeners; gpm still owns TLS termination, routing, ACME and access lists.
No roadmap item currently tracks native tunnel support (see
[FEATURES.md](https://github.com/Rake-Pro/go-proxy-manager/blob/main/FEATURES.md) "Not planned at this time" for what is explicitly
out of scope today).

## Prerequisites (all patterns)

- gpm running with the data-plane listeners (`:80`/`:443`) reachable from
  whichever tunnel/relay component you pick - not necessarily from the
  public internet.
- A DNS-01-capable [DNSProvider](../reference/config/dns-provider.md)
  (`cloudflare`, `digitalocean`, `hetzner`, `desec`, `rfc2136` or
  `acme-dns`), since CGNAT means none of these patterns can expose a public
  `:80` for ACME HTTP-01.
- Read [Client IP and the three trust tiers](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers)
  before writing access lists behind any of these. Every pattern below puts a
  relay in front of gpm, so its address must be listed in
  `settings.trustedProxies` (or the host's own) or every request is attributed
  to the relay.

## Pattern A: Cloudflare Tunnel (cloudflared)

A `cloudflared` sidecar holds an outbound connection to Cloudflare's edge and
relays HTTP(S) to gpm - no inbound port at all, on any protocol.

### Steps

1. **Point cloudflared at gpm's HTTPS listener**, not plaintext `:80`:

   ```yaml
   # cloudflared/config.yml
   tunnel: <tunnel-id>
   credentials-file: /etc/cloudflared/credentials.json
   ingress:
     - hostname: app.example.com
       service: https://gpm:443
       originRequest:
         originServerName: app.example.com   # SNI for gpm's cert
     - service: http_status:404
   ```

2. **Issue gpm's certificate over DNS-01** (port 80 is never reached from the
   internet in this pattern):

   ```yaml
   # config/certificates/app.yaml
   name: app
   type: acme
   domains: [app.example.com]
   acme: {email: admin@example.com, dnsProvider: cloudflare}
   ```

   With a real, publicly trusted cert on gpm, `cloudflared` verifies TLS
   normally - no `noTLSVerify` needed. Only add
   `originRequest: {noTLSVerify: true}` if gpm is deliberately presenting a
   private/self-signed certificate for this host, and understand that turns
   off certificate verification between `cloudflared` and gpm entirely.

3. **Run the sidecar next to gpm** (both on one Docker network, no published
   ports needed for `80`/`443`):

   ```yaml
   # docker-compose.yml
   services:
     gpm:
       image: ghcr.io/rake-pro/go-proxy-manager
       volumes: [gpm-data:/data]
       networks: [edge]
     cloudflared:
       image: cloudflare/cloudflared:latest
       command: tunnel run
       volumes:
         - ./cloudflared/config.yml:/etc/cloudflared/config.yml:ro
         - ./secrets/cf_tunnel_credentials.json:/etc/cloudflared/credentials.json:ro
       networks: [edge]
   networks:
     edge: {}
   volumes:
     gpm-data:
   ```

   Tunnel token/credentials setup is Cloudflare's own flow (`cloudflared
   tunnel create`, `cloudflared tunnel route dns`) - not covered here.

### Client-IP limitation (read before writing access lists)

| What | State today |
|------|--------------|
| TCP peer gpm sees | `cloudflared`'s own address on the shared Docker network - not the real client. |
| `CF-Connecting-IP` header | Not read by gpm. No code path resolves it (verified against `internal/dataplane`). |
| `X-Forwarded-For` trust | Only from a proxy named in a **forward-auth** IdP's `trustedProxies` **on that host** - and that mode requires the proxy to also assert identity headers (`userHeader` etc.), which plain `cloudflared` does not send. There is no passive "just trust this CIDR" option. |
| PROXY protocol | `settings.proxyProtocol` exists (see Pattern C), but `cloudflared` does not speak it to the origin - it does not apply here. |

**Consequence:** access lists, rate limiting and geo rules on a
`cloudflared`-fronted host see the tunnel's local peer address, not the
visitor's real IP, with no supported workaround today. Do not rely on an
IP-based `AccessList` to gate a `cloudflared`-fronted host; use
[Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/) or an
OIDC/forward-auth middleware for access control on that host instead.

### Verify

- `curl -fsS https://app.example.com` returns from outside your network via
  the tunnel, with no port forward configured on your router.
- `gpm`'s access log (`-access-log`) shows the request's `RemoteAddr` as the
  `cloudflared` container's address, confirming the limitation above rather
  than assuming it away.

## Pattern B: Tailscale

gpm joins the tailnet directly - Tailscale is a network layer (WireGuard),
not an HTTP proxy, so tailnet peers reach gpm's real listeners with no proxy
hop in between.

### Steps

1. **Put gpm on the tailnet.** Run `tailscale` in the same network namespace
   as gpm (a sidecar with `network_mode: service:gpm` in Compose, or
   `tailscale` installed on the host directly for bare-metal). gpm's existing
   `:443`/`:80` binds (`GPM_HTTPS_ADDR`, `GPM_HTTP_ADDR`) already answer on
   every interface, including the new tailnet one - no gpm config change.

2. **Resolve the hostname to the tailnet IP.** Either:
   - Tailscale MagicDNS / Split DNS: add a custom DNS record for
     `app.example.com` -> gpm's tailnet IP in the Tailscale admin console, or
   - gpm's own [DNS sync](dns-sync.md)
     (`dns.lanDirect`) publishing that record to a Pi-hole reachable on the
     same tailnet.

3. **Issue the certificate over DNS-01** - a tailnet-only host is never
   publicly reachable on `:80` for HTTP-01:

   ```yaml
   # config/certificates/app.yaml
   name: app
   type: acme
   domains: [app.example.com]
   acme: {email: admin@example.com, dnsProvider: cloudflare}
   ```

4. **Restrict the host to the tailnet** with an ordinary access list - the
   Tailscale CGNAT range is `100.64.0.0/10`:

   ```yaml
   # config/access-lists/tailnet-only.yaml
   name: tailnet-only
   defaultAction: deny
   rules:
     - {action: allow, cidr: 100.64.0.0/10}
   ```

   Attach it to the proxy host's `accessLists`. No forward-auth IdP or
   `trustedProxies` wiring is needed - the peer address gpm sees **is** the
   real tailnet client, because nothing L7-proxies the connection.

### Verify

- `curl -fsS https://app.example.com` succeeds from a tailnet peer and times
  out from off-tailnet.
- gpm's access log shows the request's `RemoteAddr` as the actual tailnet
  peer's `100.x.x.x` address - real client IP preserved with no extra config.

## Pattern C: WireGuard/VPS relay with PROXY protocol

A small VPS with a public IP terminates `:80`/`:443` and relays the raw TCP
connection to gpm over WireGuard, tagging it with a PROXY protocol v2 header
so gpm recovers the real client IP.

### Steps

1. **WireGuard tunnel between the VPS and gpm's host:**

   ```
   # VPS: /etc/wireguard/wg0.conf
   [Interface]
   Address = 10.66.0.1/24
   PrivateKey = <vps-private-key>
   ListenPort = 51820

   [Peer]
   PublicKey = <gpm-host-public-key>
   AllowedIPs = 10.66.0.2/32
   ```

   ```
   # gpm host: /etc/wireguard/wg0.conf
   [Interface]
   Address = 10.66.0.2/24
   PrivateKey = <gpm-host-private-key>

   [Peer]
   PublicKey = <vps-public-key>
   Endpoint = vps.example.com:51820
   AllowedIPs = 10.66.0.1/32
   PersistentKeepalive = 25
   ```

2. **On the VPS, relay `:80`/`:443` as a TCP passthrough** (L4 only - TLS
   still terminates at gpm), tagging each connection with PROXY protocol v2.
   HAProxy:

   ```
   # /etc/haproxy/haproxy.cfg
   frontend fe_https
       bind *:443
       mode tcp
       default_backend be_gpm_https

   backend be_gpm_https
       mode tcp
       server gpm 10.66.0.2:443 send-proxy-v2
   ```

   or nginx `stream`:

   ```
   # /etc/nginx/nginx.conf (stream block)
   stream {
       server {
           listen 443;
           proxy_pass 10.66.0.2:443;
           proxy_protocol on;
       }
   }
   ```

   Repeat for `:80` (needed for ACME HTTP-01 - this pattern is the only one
   of the three where port 80 reaches gpm from the internet).

3. **Trust the VPS's WireGuard address on gpm** via
   [`settings.proxyProtocol`](../reference/config/settings/proxy-protocol.md):

   ```yaml
   # config/settings.yaml
   proxyProtocol:
     enabled: true
     trustedCIDRs: [10.66.0.1/32]
     timeout: 5s
   ```

   This replaces the connection's `RemoteAddr` before any other control
   runs, so access lists, rate limits, geo rules and the access log all see
   the real client with no per-feature wiring - see
   [ProxyProtocolSettings](../reference/config/settings/proxy-protocol.md).

### Verify

- `curl -fsS https://app.example.com` succeeds from the public internet via
  the VPS.
- gpm's access log shows the request's `RemoteAddr` as the real visitor IP,
  not `10.66.0.1` (the VPS) - confirms the PROXY protocol header is being
  read, not just tolerated as payload.
- Enabling `proxyProtocol` on gpm before HAProxy/nginx sends the header
  breaks nothing (plain HTTP is not a PROXY header, so it's read as an
  ordinary request); enabling it on the relay side first, before gpm trusts
  the CIDR, would - see the warning in
  [ProxyProtocolSettings](../reference/config/settings/proxy-protocol.md).

## Comparison

| Pattern | Real client IP preserved? | Port 80 for HTTP-01? | Extra cost | TLS terminated by |
|---------|---------------------------|-----------------------|------------|--------------------|
| A: Cloudflare Tunnel | No - no supported path today (see limitation above) | No - DNS-01 required | Cloudflare account (tunnels are free-tier) | gpm |
| B: Tailscale | Yes - native, no proxy hop | No - DNS-01 required (tailnet is not publicly reachable) | Tailscale account (free tier covers this) | gpm |
| C: WireGuard/VPS relay | Yes - via PROXY protocol v2 | Yes | A rented VPS | gpm (VPS is L4 passthrough only) |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| ACME issuance fails with a `dns-01` provider error | `DNSProvider` credential wrong/missing scope, or a `dnsProvider` name typo on the `Certificate` | Check `config/dns-providers/*.yaml` against the [DNSProvider reference](../reference/config/dns-provider.md); test the API token against the provider directly |
| Access list allows/denies the wrong clients (Patterns A/C) | `RemoteAddr` isn't what you assumed - check the access log | Pattern A: no fix today, see the [client-IP limitation](#client-ip-limitation-read-before-writing-access-lists); Pattern C: confirm `proxyProtocol.trustedCIDRs` names the relay's WireGuard address and the relay actually sends `send-proxy-v2`/`proxy_protocol on` |
| Stream backend behind Pattern C stalls for several seconds per connection | `proxyProtocol.enabled: true` on gpm but the relay isn't actually sending a PROXY header (or the reverse: relay sends it but gpm hasn't enabled it) | Enable on the relay first, then on gpm - see the ordering warning in [ProxyProtocolSettings](../reference/config/settings/proxy-protocol.md) |
| `cloudflared` logs a TLS verification error to gpm | gpm is presenting a cert whose SNI/CN doesn't match `originServerName`, or a private cert without `noTLSVerify` | Confirm the `Certificate` object's `domains` matches the `hostname` in `cloudflared`'s ingress rule, or add `noTLSVerify: true` only if the cert is deliberately private |
| Tailnet peer gets connection refused | gpm's data-plane listener isn't actually reachable on the tailnet interface, or MagicDNS/Split DNS hasn't propagated | Confirm `tailscale status` shows gpm as a peer and `tailscale ip` resolves the hostname; `curl` gpm's tailnet IP directly to isolate DNS from connectivity |
