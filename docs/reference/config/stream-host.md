# StreamHost

Raw TCP/UDP forwarding. The data plane opens a listener per `listenPort` (TCP, UDP,
or both) and forwards to the backend; listeners are reconciled on every reload
(ports added/removed, backend swapped, with no listener restart for unchanged
ports). UDP uses per-client sessions with an idle timeout.

A TCP stream can additionally be **TLS-aware** (`tls`): SNI-routed so several
hosts share one port, either passed through untouched or terminated at gpm. And
it can be **gated at L4** (`accessLists`) on the client IP, evaluated before any
backend is dialled.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="stream-host-listen-port"></span>  `listenPort` | int | yes | 1-65535. **Publish this port from the container** (compose `ports:`) so it is reachable, and avoid colliding with the data-plane 80/443 or admin port: a bind failure is logged and that one port is skipped, never fatal. |
| <span id="stream-host-protocol"></span>  `protocol` | string | yes | `tcp`\|`udp`\|`both`. |
| <span id="stream-host-target"></span>  `target` | StreamTarget | yes | The backend this port forwards to: `{host, port}`. Mirrors `upstream`'s vocabulary minus the scheme: a raw stream carries an arbitrary protocol, so `http`/`https` means nothing here. |
| <span id="stream-host-tls"></span>  `tls` | StreamTLS | no | SNI routing and/or TLS termination. **TCP only.** |
| <span id="stream-host-access-lists"></span>  `accessLists` | []string | no | L4 access lists evaluated on the client IP (below). |

```yaml
name: postgres
listenPort: 5432
protocol: tcp
target:
  host: db.internal
  port: 5432
accessLists: [lan-only]
```

## Removed fields

| Field | Status | Migration |
|---|---|---|
| <span id="stream-host-forward-host"></span>  `forwardHost` | Removed. Decoded only so the loader can reject it loudly. | Replace the pair with `target: {host, port}`. |
| <span id="stream-host-forward-port"></span>  `forwardPort` | Removed, same. | Same. |

A file still carrying either key is **rejected at load** with an error naming
the new shape: it is never silently accepted with no backend. See
[Upgrading](../../operations/upgrading.md).

## StreamTarget (`target`)

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="stream-host-target-host"></span>  `host` | string | yes | Backend host. |
| <span id="stream-host-target-port"></span>  `port` | int | yes | 1-65535. |

## L4 access lists (`accessLists`)

A stream host may reference AccessList objects, exactly like a proxy host. Only
the **IP/CIDR rules and the geo rules** are evaluated: basic auth is an HTTP
challenge/response with nowhere to live in a raw stream, so referencing a list
that has (deprecated) `basicAuth` users is **rejected at validation** rather than
silently half-applied; moving those users to an auth middleware with
`mode: basic` removes the conflict, since that middleware is HTTP-only and is
never attached to a stream host. All referenced lists must allow, and the check runs **before any
backend is dialled**, so a denied client cannot make gpm open a socket to the
backend at all.

For UDP the list is evaluated once per session (the first packet from a source);
a denied source creates no session and no upstream socket. Geo rules follow the
same fail-closed rule as HTTP: with geo configured and no GeoIP database loaded,
the port denies.

Behind an L4 balancer, enable `settings.proxyProtocol`, otherwise every
connection looks like it came from the balancer and the rules match the wrong
address.

The `maxUDPSessions` bound (4096 per listener) still caps spoofed-source UDP
memory independently of any access list.

## StreamTLS (`tls`)

Makes a TCP stream port TLS-aware: several hosts can share one port, separated by
the SNI in the ClientHello, and gpm can either forward the handshake untouched or
terminate it.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="stream-host-tls-mode"></span>  `mode` | string | yes | `passthrough` \| `terminate`. |
| <span id="stream-host-tls-sni-match"></span>  `sniMatch` | []string | see below | Server names this host claims. Exact (`db.example.com`) or a single-label wildcard (`*.example.com`). |
| <span id="stream-host-tls-certificate-ref"></span>  `certificateRef` | string | terminate only | Names a Certificate. **Required** in `terminate`, **forbidden** in `passthrough`. |

- **`passthrough`** peeks the ClientHello (a bounded, stdlib-only parse of the
  record, handshake and `server_name` extension), routes on the SNI, and replays
  the peeked bytes to the backend. gpm never decrypts and never needs the key:
  the backend terminates, end to end.
- **`terminate`** completes the handshake at gpm with `certificateRef` from the
  normal certificate store (custom or ACME-issued) and forwards **plaintext** to
  the backend. The floor is TLS 1.2 with the same AEAD cipher suites the HTTPS
  listener uses. No ALPN is offered: what rides inside a stream is an arbitrary
  TCP protocol.

**Port sharing.** Two or more enabled stream hosts may share a TCP `listenPort`
**only if every one of them sets `sniMatch`**: that is the only thing that tells
their connections apart. Validation rejects a mixed or duplicate claim, so
routing can never fall back to "whichever host compiled last". A host alone on its
port may omit `sniMatch` and take every connection. On an SNI-routed port, a
connection whose server name no host claims (or that sends no SNI) is closed
rather than handed to an arbitrary backend, and a connection that is not TLS at
all is closed too.

**UDP.** `tls` requires `protocol: tcp`. A UDP datagram carries no ClientHello, so
`udp` and `both` are rejected at validation; two hosts can never share a UDP port
either.

```yaml
# Two Postgres instances behind one public 5432, separated by SNI, never decrypted.
name: pg-blue
listenPort: 5432
protocol: tcp
target:
  host: pg-blue.internal
  port: 5432
accessLists: [lan-only]
tls:
  mode: passthrough
  sniMatch: [blue.db.example.com]
---
# TLS terminated at gpm, plaintext to a backend that speaks none.
name: mqtt
listenPort: 8883
protocol: tcp
target:
  host: mosquitto.internal
  port: 1883
tls:
  mode: terminate
  sniMatch: [mqtt.example.com]
  certificateRef: wildcard
```

---
