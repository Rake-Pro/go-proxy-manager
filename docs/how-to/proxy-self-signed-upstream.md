# Proxy to an upstream with a self-signed certificate

Proxy over HTTPS to an appliance (a BMC, a switch, a hypervisor UI) whose built-in certificate no public or private CA signed, by pinning that exact certificate.

## Prerequisites

- The appliance answers HTTPS at a fixed address, for example `192.0.2.30:443`.
- `openssl` on a machine that can reach the appliance (to read the fingerprint).
- An `admin` session, an API token with `proxy-hosts:write`, or write access to the config repo.
- Know what a pin checks before you rely on it:

| A pin checks | A pin does not check |
|---|---|
| That the SHA-256 of the leaf certificate the server presents equals one of the pins, on every handshake (a resumed session included) | The issuer, the chain or any CA |
| | The certificate's names (CN or subjectAltName) against the upstream address |
| | The validity dates: an expired pinned certificate is still accepted |
| | Key usage, extended key usage or `CA:TRUE` |

Everything else about the connection is unchanged: TLS versions, ALPN and HTTP/2, and SNI (from `upstream.host`). `hostHeader` does not affect the pin.

## Steps

1. Read the fingerprint of the certificate the appliance presents, from a network path you trust:

   ```sh
   openssl s_client -connect 192.0.2.30:443 </dev/null 2>/dev/null \
     | openssl x509 -noout -fingerprint -sha256
   ```

   The output looks like `sha256 Fingerprint=3A:7F:0C:...:1D`. Copy the part after `=`. Better still, read it from the appliance's own console or certificate file, so the value cannot come from a machine in the middle.
2. Set the pin on the host's upstream (any case, with or without `:`; gpm stores 64 lower-case hex characters):

   ```yaml
   # config/proxy-hosts/bmc.yaml
   name: bmc
   domains: [bmc.example.com]
   upstream:
     scheme: https
     host: 192.0.2.30
     port: 443
     tls:
       pinnedCertSHA256:
         - 3A:7F:0C:55:91:DE:2B:48:6E:A1:C4:09:F3:8D:72:B6:5E:10:AA:C7:34:9B:E2:61:0F:D8:4C:93:27:B5:E8:1D
   tls:
     certificateRef: wildcard
     forceSSL: true
   ```

   In the admin UI: open the host, keep **Scheme** on `https`, and paste the fingerprint into **Pinned certificate SHA-256** (one per line or comma separated). The field is greyed out while the scheme is `http`.
3. For an [UpstreamGroup](../reference/config/upstream-group.md), set `tls.pinnedCertSHA256` on each member: members are different devices with different certificates, and each member's pin applies to that member only. For a location, set it on the location's own `upstream`. A [Kubernetes Ingress discovery](../reference/config/settings/ingress-discovery.md) template takes it on `template.upstream`; an enabled [Docker discovery](../reference/config/settings/docker-discovery.md) template refuses it (each container has its own address and certificate), so pin a hand-written host instead.
4. Save. A malformed value is refused with the field named, for example `proxy host "bmc": upstream.tls.pinnedCertSHA256[0]: "3A:7F" is not a SHA-256 fingerprint (want 64 hex characters, optionally as 32 ":"-separated bytes)`.
5. Later changes apply at the next reload: changing or removing a pin closes the open WebSocket tunnels of every host that routes to that upstream and stops reuse of its pooled connections.

## Verify

| Check | Expected |
|---|---|
| `curl -sI https://bmc.example.com/` | The appliance's own response, not `502` |
| `GET /api/proxy-hosts/bmc` | `upstream.tls.pinnedCertSHA256` holds the lower-case form of your pin |
| Logs after a request | No `upstream error` line for host `bmc` |
| A group member with an HTTP `healthCheck.path` | `GET /api/upstream-health` shows the member `healthy: true` (its probe uses the same pin) |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: `proxy host "bmc": upstream.tls: requires scheme https (a certificate pin has no meaning for plain http), got "http"` | A pin on an `http` upstream | Set `scheme: https`, or remove `tls` |
| Write refused: `proxy host "bmc": upstream.tls.pinnedCertSHA256[0]: "3A:7F" is not a SHA-256 fingerprint (want 64 hex characters, optionally as 32 ":"-separated bytes)` | Not 64 hex characters, or not 32 `:`-separated bytes; a SHA-1 fingerprint is 40 characters | Use `-sha256`, and copy only the part after `Fingerprint=` |
| Write refused: `proxy host "bmc": upstream.tls.pinnedCertSHA256[1]: duplicate fingerprint <hex>` | The same certificate pinned twice, perhaps in two spellings | Remove one |
| Write refused: `proxy host "bmc": upstream.tls.pinnedCertSHA256: at most 4 fingerprints, got 5` | More than four pins | Keep the current and the next certificate only |
| Settings write refused: `dockerDiscovery.template.upstream.tls.pinnedCertSHA256: certificate pins are not supported on Docker discovery templates ...` | A pin on the Docker discovery template (or a Docker profile) | Remove it there; pin the upstream on a hand-written proxy host |
| Log warning `docker discovery: dockerDiscovery inherits ingressDiscovery profile "<name>", whose upstream.tls.pinnedCertSHA256 does not apply to hosts derived from containers ...` | Docker discovery has no `profiles` of its own and inherits a pinned `ingressDiscovery` profile; a container that selects it gets its own address without the pin | Expected for Ingress-only pins; define `dockerDiscovery.profiles`, or pin a hand-written host for the container |
| An open WebSocket to the host drops after a config change | The change added, removed or replaced a pin of an upstream the host routes to; the tunnel was admitted under the old pin set | Expected; the client reconnects under the new pins |
| `502`, log line `upstream error` with `presentedCertSHA256: <hex>` and `does not match any pinned fingerprint` | The appliance presents a different certificate: it was regenerated, replaced, or something else answers at that address | Confirm on the appliance itself that `<hex>` is its certificate before you pin it |
| `502` with `x509: certificate signed by unknown authority` or `certificate is not valid for any names` | No pin is set (or it was removed), so normal verification applies | Add the pin |
| Rotate the certificate without downtime | Only the pinned certificates are accepted | Add the new fingerprint next to the old one, save, replace the certificate on the appliance, then remove the old pin |
| `tls: {}` or an empty `pinnedCertSHA256` list disappears after saving | An empty object or list means "no pin" and is not stored | Expected; normal verification applies |
