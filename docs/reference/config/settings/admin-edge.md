# Settings: admin front door

Serve the admin panel at the edge, through gpm's own proxy listeners, without a
proxy host object.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-admin-edge"></span> `adminEdge` | AdminEdgeSettings | The panel's own front door (below). Zero value is off: the panel is reachable only on the admin listener (`GPM_ADMIN_ADDR`) or through whatever you put in front of it yourself. |

With `adminEdge.enabled`, gpm serves the panel on the hostname of
`externalBaseURL` at its HTTPS and HTTP listeners: the certificate is picked by
domain at the handshake like every other host's, the chain below runs in front,
and the request then hops over loopback to the admin listener. It replaces the
hand-written proxy host that used to do this (see
[Admin UI behind gpm](../../../how-to/admin-ui-behind-gpm.md)).

It is **not a proxy host**:

- it is never listed under Proxy Hosts, and cannot be disabled, deleted,
  cloned or discovery-managed;
- the fleet-wide [maintenance switch](maintenance.md) never applies to it, so
  the panel stays reachable to turn maintenance back off;
- while it is on, the name `gpm-admin` is reserved and no stored proxy,
  redirect or parked host may claim the panel's hostname: a settings write or
  an object write that would collide is refused with the other side named;
- a middleware or access list it names cannot be deleted from under it.

The direct admin listener keeps working unchanged: this is a second door, not a
replacement for the break-glass one.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-admin-edge-enabled"></span> `adminEdge.enabled` | bool | Turn the front door on. Needs `externalBaseURL`, whose hostname is the one served. |
| <span id="settings-admin-edge-tls"></span> `adminEdge.tls` | TLSSettings | Applied exactly as on a proxy host: `forceSSL`, `hsts`, `minTLSVersion`, `clientAuth`. `certificateRef` is optional and informational, since the handshake picks a certificate by domain. |
| <span id="settings-admin-edge-middlewares"></span> `adminEdge.middlewares` | []string | The middleware chain in front of the panel, in order, with proxy-host semantics. The panel has its own login; a sign-in middleware here gates it a second time. |
| <span id="settings-admin-edge-access-lists"></span> `adminEdge.accessLists` | []string | Access lists in front of the panel. A LAN/VPN list is the usual choice. |
| <span id="settings-admin-edge-dns"></span> `adminEdge.dns` | DNSSyncPolicy | [DNS sync](dns-sync.md) policy for the panel's hostname (`lanDirect`, `publicCname`), as on a proxy host. |
| <span id="settings-admin-edge-robots-no-index"></span> `adminEdge.robotsNoIndex` | bool | Send `X-Robots-Tag: noindex, nofollow` on the panel. |

```yaml
# settings.yaml
externalBaseURL: https://gpm.example.com
adminEdge:
  enabled: true
  accessLists: [admin-lan-only]
  tls:
    forceSSL: true
    hsts: {enabled: true, maxAge: 31536000, includeSubdomains: true}
    minTLSVersion: "1.3"
  dns: {lanDirect: true, publicCname: true}
  robotsNoIndex: true
```

**Client address.** The hop to the admin listener arrives from gpm's own proxy
listener, so `settings.trustedProxies` must list that address (`127.0.0.1/32`
and `::1/128` for a loopback bind) or every admin login attempt shares one
lockout bucket. See
[Client IP and the three trust tiers](../../../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers).

**Migrating from a proxy host.** Turn the front door on with the same access
lists and TLS settings the host had, then delete the host; or delete the host
first, then turn it on. The save that would leave both claiming the hostname is
refused either way, with the host named. The panel keeps answering on the admin
listener throughout.

**What stays up in fleet maintenance.** The front door, and the host each admin
sign-in provider's issuer lives behind (an OIDC login bounces through it, so
with it down the login would be too). A per-host maintenance flag on one of
those hosts is still honoured; only the fleet-wide switch skips them. The
Settings page lists them next to the switch, and `GET /api/capabilities`
reports them under `maintenance.exempt`.
