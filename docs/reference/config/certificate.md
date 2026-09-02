# Certificate

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="certificate-type"></span> `type` | string | yes | `custom` or `acme`. |
| <span id="certificate-domains"></span> `domains` | []string | yes | Domains the cert covers (`*.example.com` for wildcard). |
| <span id="certificate-acme"></span> `acme` | ACMESpec | when `type: acme` | |
| <span id="certificate-custom"></span> `custom` | CustomCertSpec | when `type: custom` | |

**ACMESpec**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="certificate-acme-email"></span> `email` | string | yes | ACME account contact. |
| <span id="certificate-acme-challenge"></span> `challenge` | string | no | `dns-01` or `http-01`. Default: `dns-01` when `dnsProvider` is set (so configs written before this field existed keep their behaviour), `http-01` otherwise. |
| <span id="certificate-acme-dns-provider"></span> `dnsProvider` | string | for `dns-01` | A [DNSProvider](dns-provider.md) name. Rejected with `http-01`. |
| <span id="certificate-acme-directory-url"></span> `directoryURL` | string | no | Defaults to Let's Encrypt production. |
| <span id="certificate-acme-key-type"></span> `keyType` | string | no | `ecdsa` (default) \| `rsa`. |
| <span id="certificate-acme-eab"></span> `eab` | EABSpec | no | External Account Binding, for CAs that require it. |

**EABSpec**: `kid` (the key id the CA issued) and `hmacKey` (Secret; base64url
as the CA issued it). Both are required together. An EAB key id widens the ACME
account identity, so two external accounts on the same CA get separate account
keys under `<cert-dir>/acme/accounts/`.

Challenge selection:

- **`http-01`** - validated on the data plane's plaintext `:80` listener. The
  ACME manager parks the in-flight token in memory and the listener answers
  `/.well-known/acme-challenge/<token>` **before** host routing, the force-SSL
  redirect, and any auth, so a certificate can be issued for a host that does not
  exist yet or that redirects everything to https. A challenge path whose token
  is not in flight is routed normally, so an upstream running its own ACME client
  keeps working. Port 80 must be reachable from the internet.
- **`dns-01`** - the only challenge that can prove a wildcard. A wildcard domain
  with `http-01` (explicit or defaulted) is a validation error.

```yaml
# ZeroSSL with External Account Binding, http-01
name: zerossl
type: acme
domains: [app.example.com]
acme:
  email: admin@example.com
  challenge: http-01
  directoryURL: https://acme.zerossl.com/v2/DV90
  eab:
    kid: ${ENV:ZEROSSL_EAB_KID}
    hmacKey: ${ENV:ZEROSSL_EAB_HMAC}
```
```yaml
# Google Public CA (EAB required; kid + hmacKey come from `gcloud publicca`)
acme:
  email: admin@example.com
  challenge: http-01
  directoryURL: https://dv.acme-v02.api.pki.goog/directory
  eab:
    kid: ${ENV:GOOGLE_CA_EAB_KID}
    hmacKey: ${FILE:/run/secrets/google_ca_eab_hmac}
```

**CustomCertSpec**: `certFile`, `keyFile` - paths **relative to the cert store**
(absolute paths and `..` are rejected). These are file references, not inline PEM.

```yaml
# ACME wildcard
name: wildcard
type: acme
domains: ["*.example.com", example.com]
acme:
  email: admin@example.com
  dnsProvider: cloudflare
  keyType: ecdsa
```
```yaml
# ACME single name over http-01 (no DNS provider needed)
name: app
type: acme
domains: [app.example.com]
acme:
  email: admin@example.com
  challenge: http-01
```
```yaml
# Bring-your-own
name: internal
type: custom
domains: [internal.example.com]
custom: {certFile: internal.crt, keyFile: internal.key}
```

## Status fields (GET only)

`GET /api/certificates` and `GET /api/certificates/{name}` decorate every
stored object with these read-only fields, computed from the certificate store
on disk (`lastError`/`lastAttempt` come from the ACME manager's in-memory
state instead). None are accepted on write and none are stored in the YAML.

| Field | Type | Notes |
|-------|------|-------|
| <span id="certificate-status-not-before"></span> `notBefore` | string (RFC 3339) | Leaf certificate's validity start. Absent for an `acme` certificate with no completed order yet (`state: pending`). |
| <span id="certificate-status-not-after"></span> `notAfter` | string (RFC 3339) | Leaf certificate's expiry. Absent alongside `notBefore`. |
| <span id="certificate-status-days-remaining"></span> `daysRemaining` | int | Whole days until `notAfter`, negative once expired. Absent alongside `notAfter`. |
| <span id="certificate-status-issuer"></span> `issuer` | string | Leaf certificate's issuer common name. Absent alongside `notAfter`. |
| <span id="certificate-status-state"></span> `state` | string | `valid`, `expiring` (at or inside 30 days), `expired`, `pending` (`acme`, no order has completed yet) or `error` (last attempt failed - see `lastError`). |
| <span id="certificate-status-last-error"></span> `lastError` | string | Most recent renewal (or, for `custom`, file-read) failure. Empty or absent means the last attempt succeeded. **The text depends on the caller:** an admin gets the raw ACME or file/parse message truncated to 300 characters; any non-admin caller - the read-only `user` role, or a `certificates:read` token - gets the same short classified reason `GET /api/health` uses, e.g. `dns-01 challenge or DNS provider failure`. The raw text can embed a third party's response body, so it is not disclosed more widely than `/health` does. |
| <span id="certificate-status-last-attempt"></span> `lastAttempt` | string (RFC 3339) | When the ACME manager last attempted to issue/renew this certificate. `acme` certificates only; absent on an HA follower, which does not run the manager. |
| <span id="certificate-status-sans"></span> `sans` | []string | Leaf certificate's subject alternative names (DNS names and IPs). |

Force an immediate renewal, ignoring the 30-day renewal window, with
`POST /api/certificates/{name}/renew` (`certificates:write` scope). It
responds `{"started": true}` once the order has **started**, not once it
completes - DNS-01 propagation alone can take minutes. Poll the status fields
above for the outcome.

Two things answer `409`, and the error message says which:

| Cause | Message | Fix |
|---|---|---|
| Another order is in flight anywhere on this instance | Names the in-flight order; nothing is queued | Wait for it and retry |
| This certificate is inside its **1-hour renew cooldown**, counted from `lastAttempt` - a failed attempt starts it too | States the remaining wait, e.g. `retry in 42m10s` | Wait it out, and read `lastError` meanwhile |

`400` is a `custom` certificate (gpm does not renew those - replace the file and
`PUT` the object); `501` is an instance that is not the ACME issuer. See
[Certificate health](../../operations/certificate-health.md) for the
operational walkthrough and `GET /api/health` for the fleet-wide summary.

## Which certificate a host serves

Selection is by **SNI**, not by the host's `tls.certificateRef`:

- Every enabled `Certificate` object is loaded into one SNI map at compile time.
- At handshake, an exact match on the client's `server_name` wins; otherwise a
  `*.parent` wildcard one label up wins; otherwise the handshake fails.
- An ACME certificate that has not been issued yet is skipped until the manager
  issues it.

Where `tls.certificateRef` is honoured:

| Host kind | `tls.certificateRef` | What actually selects the certificate |
|---|---|---|
| `ProxyHost`, `RedirectHost`, `ParkedHost` | Checked for existence only. **Ignored at request time.** | SNI, across every certificate. |
| `StreamHost` with `tls.mode: terminate` | **Authoritative and required.** | The named certificate, and only it. |
| `StreamHost` with `tls.mode: passthrough` | Forbidden. | Nothing - gpm never terminates. |

For an L7 host the field is an intent record: useful documentation of which
certificate you expect to cover the host, and nothing more. gpm logs a **warning
at load time** when a proxy, redirect or parked host names a certificate that
covers none of its domains, because that config serves a *different* certificate
(or fails the handshake) than the field implies:

```
proxy host "app" sets tls.certificateRef to certificate "wildcard-other", which
covers none of its domains (app.example.com); proxy host certificates are
selected by SNI across every certificate ...
```

It is a warning, not an error: the config is legal, and the certificate that does
cover the domain still serves the host.

---
