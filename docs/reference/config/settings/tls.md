# Settings: TLS

The fleet-wide minimum TLS version every HTTPS and stream-terminate listener
negotiates, so an edge can be hardened in one place instead of host by host.

| Field | Type | Default | Required | Description |
|---|---|---|---|---|
| <span id="settings-tls"></span> `tls` | object | `{}` | no | Fleet-wide TLS defaults. Omitted entirely by an untouched `settings.yaml`. |
| <span id="settings-tls-min-version"></span> `tls.minVersion` | string | `"1.2"` | no | Lowest TLS version the edge accepts: `"1.2"` (the default, and what an empty value means) or `"1.3"`. Anything else is rejected at write time. |

## What it covers

- **HTTPS listener.** Every proxy, redirect and parked host that does not pin
  its own floor, plus an unknown or absent SNI.
- **Stream hosts in `terminate` mode.** A stream host has no per-host
  `minTLSVersion`, so the fleet floor is the only way to raise it.
- **Not the admin panel's own listener**, which is served by the admin server,
  and not an outbound connection gpm makes (ACME, DNS, webhooks, upstreams).

## Precedence: a host overrides the fleet, in both directions

[ProxyHost `tls.minTLSVersion`](../proxy-host.md#proxy-host-tls) is a per-host
floor selected by SNI at handshake time, and it wins over this setting whichever
way it points.

| `settings.tls.minVersion` | Host `tls.minTLSVersion` | Floor for that host |
|---|---|---|
| unset / `"1.2"` | unset | 1.2 |
| unset / `"1.2"` | `"1.3"` | 1.3 |
| `"1.3"` | unset | 1.3 |
| `"1.3"` | `"1.2"` | 1.2 |

## Raising the floor

`1.3` refuses the handshake outright for a client that cannot negotiate it:
there is no fallback, no redirect and no error page to read, because the
connection never becomes HTTP. Raise it only once every client of every host
that does not pin its own floor supports TLS 1.3, and pin `"1.2"` on the
individual hosts that still serve older embedded devices.

```yaml
# config/settings.yaml - TLS 1.3 across the fleet
tls:
  minVersion: "1.3"
```

```yaml
# config/proxy-hosts/legacy-camera.yaml - one host stays on 1.2
name: legacy-camera
domains: [camera.example.com]
upstream: {scheme: http, host: 192.0.2.20, port: 8080}
tls:
  certificateRef: wildcard
  forceSSL: true
  minTLSVersion: "1.2"
```

## Where it is in the UI

**Settings -> General -> TLS**, a single "Minimum TLS version" select. A change
takes effect on the next config reload, which the save triggers; existing
connections are unaffected.
