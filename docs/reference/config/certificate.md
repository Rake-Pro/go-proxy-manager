# Certificate

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="certificate-type"></span> `type` | string | yes | `custom` or `acme`. |
| <span id="certificate-domains"></span> `domains` | []string | yes | Domains the cert covers (`*.example.com` for wildcard). Each must be a hostname, optionally with a leading `*.`: no port, no trailing dot. A write that adds another spelling is refused; a stored one loads with a config warning. |
| <span id="certificate-acme"></span> `acme` | ACMESpec | when `type: acme` | |
| <span id="certificate-custom"></span> `custom` | CustomCertSpec | when `type: custom` | |

**ACMESpec**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="certificate-acme-email"></span> `email` | string | yes | ACME account contact. Let's Encrypt stopped sending expiration reminders in 2025; this is used for account/policy notices only. |
| <span id="certificate-acme-challenge"></span> `challenge` | string | no | `dns-01` or `http-01`. Default: `dns-01` when `dnsProvider` is set (so configs written before this field existed keep their behaviour), `http-01` otherwise. |
| <span id="certificate-acme-dns-provider"></span> `dnsProvider` | string | for `dns-01` | A [DNSProvider](dns-provider.md) name. Rejected with `http-01`. |
| <span id="certificate-acme-directory-url"></span> `directoryURL` | string | no | Defaults to Let's Encrypt production. Setting or changing it through the API needs the `admin` scope ([API token scopes](api-token.md#scopes)). |
| <span id="certificate-acme-key-type"></span> `keyType` | string | no | `ecdsa` only (default when empty). `rsa` is not implemented and is rejected at config validation time. |
| <span id="certificate-acme-eab"></span> `eab` | EABSpec | no | External Account Binding, for CAs that require it. Setting or changing it through the API needs the `admin` scope. |
| <span id="certificate-acme-profile"></span> `profile` | string | no | The CA's issuance profile for every order of this certificate (for example a short-lived one); the CA lists its profiles in its directory. 1 to 64 characters of letters, digits, `_` or `-`. Empty uses the CA's default. A CA that advertises no profiles, or not this one, fails the order (`acme: certificate authority does not support profiles`, or `does not advertise a profile with name <p>` followed by the profiles it supports). Changing it on an issued certificate makes the certificate due for renewal at the next pass, like changing its domains. |
| <span id="certificate-acme-ca-bundle"></span> `caBundle` | string | no | Trust anchors for a private CA's ACME directory: inline PEM (one or more `CERTIFICATE` blocks) or a `${FILE:/path}` / `${ENV:NAME}` placeholder. At most 65536 bytes. **Not a secret:** inline PEM is stored and returned by the API as written. A `${FILE:}` bundle must lie under the secret file roots (`/run/secrets` by default, or the directories in `GPM_SECRET_FILE_ROOTS`); a path outside them fails the renewal attempt. When set, every exchange with this certificate's ACME directory (account, order, renewal information) trusts **only** this bundle, not the system roots. A bundle that cannot be resolved or parsed fails the renewal attempt; there is no fallback to system trust. A placeholder is not checked at write time. Setting or changing it through the API needs the `admin` scope. |

Validation errors for the two new keys (each prefixed `certificate "<name>": `):

| Key | Error |
|---|---|
| `acme.profile` | `acme.profile: must be 1 to 64 characters of letters, digits, "_" or "-", got "<value>"` |
| `acme.caBundle` | `acme.caBundle: must be at most 65536 bytes, got <n>` |
| `acme.caBundle` | `acme.caBundle: contains text that is not a PEM CERTIFICATE block` |
| `acme.caBundle` | `acme.caBundle: contains a "<TYPE>" PEM block; only CERTIFICATE blocks are allowed` |
| `acme.caBundle` | `acme.caBundle: certificate <i> does not parse: <error>` |
| `acme.caBundle` | `acme.caBundle: contains no PEM CERTIFICATE block` |

**EABSpec**: `kid` (the key id the CA issued) and `hmacKey` (Secret; base64url
as the CA issued it). Both are required together. An EAB key id widens the ACME
account identity, so two external accounts on the same CA get separate account
keys under `<cert-dir>/acme/accounts/`.

<span id="certificate-ca-directories"></span>
**CA directories in the admin UI**

The CA picker in the certificate wizard and editor knows these directories. It
only fills `directoryURL` (and turns EAB on where it is required); the stored
object is the plain `acme.directoryURL` key, so nothing else changes.

| Picker label | `directoryURL` | EAB |
|---|---|---|
| Let's Encrypt | `https://acme-v02.api.letsencrypt.org/directory` (the default; the key is omitted) | Not used |
| Let's Encrypt (staging) | `https://acme-staging-v02.api.letsencrypt.org/directory` | Not used |
| ZeroSSL | `https://acme.zerossl.com/v2/DV90` | Required |
| Google Public CA | `https://dv.acme-v02.api.pki.goog/directory` | Required |
| SSL.com | `https://acme.ssl.com/sslcom-dv-ecc` | Optional |
| Custom | Any URL you enter | Only when the CA asks for it |

- **Any other directory works.** Pick Custom (or write the key by hand) and
  enter its URL; the preset list is a convenience, not a restriction.
- **Custom needs its URL.** A blank `directoryURL` means Let's Encrypt, so the
  wizard and the editor refuse Custom with a blank **Directory URL**.
- **EAB follows the URL.** The wizard requires EAB whenever the URL it will
  write is a ZeroSSL or Google Public CA directory, whichever option is picked.
- **SSL.com uses the ECC directory** because gpm issues ECDSA keys only
  ([`keyType`](#certificate-acme-key-type)). A stored `https://acme.ssl.com/sslcom-dv-rsa` is
  shown as SSL.com but never written by the picker.
- **A URL that matches a preset selects it**, so opening a certificate does not
  rewrite its `directoryURL`.

Challenge selection:

- **`http-01`**: validated on gpm's own plaintext `:80` proxy listener. The
  ACME manager parks the in-flight token in memory and the listener answers
  `/.well-known/acme-challenge/<token>` **before** host routing, the force-SSL
  redirect, and any auth, so a certificate can be issued for a host that does not
  exist yet or that redirects everything to https. A challenge path whose token
  is not in flight is routed normally, so an upstream running its own ACME client
  keeps working. Port 80 must be reachable from the internet.
- **`dns-01`**: the only challenge that can prove a wildcard. A wildcard domain
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

**CustomCertSpec**: `certFile`, `keyFile`: paths **relative to the cert store**
(absolute paths and `..` are rejected). These are file references, not inline PEM.

```yaml
# Private CA (step-ca) with its root certificate, short-lived profile
name: internal-ca
type: acme
domains: [app.example.com]
acme:
  email: admin@example.com
  challenge: http-01
  directoryURL: https://ca.example.com/acme/acme/directory
  caBundle: ${FILE:/run/secrets/internal_root_ca.pem}
```
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
| <span id="certificate-status-state"></span> `state` | string | `valid`, `expiring` (at or inside `min(30 days, lifetime / 3)` of `notAfter`, lifetime being `notAfter - notBefore`; for a `custom` certificate it stays a fixed 30 days, because nothing renews it), `expired`, `pending` (`acme`, no order has completed yet) or `error` (a never-issued `acme` certificate whose attempt failed, or a leaf that cannot be read; see `lastError`). An issued `acme` certificate whose renewal fails keeps `valid` or `expiring` with `lastError` set. |
| <span id="certificate-status-last-error"></span> `lastError` | string | Most recent renewal (or, for `custom`, file-read) failure. Empty or absent means the last attempt succeeded. **The text depends on the caller:** an admin gets the raw ACME or file/parse message truncated to 300 characters; any non-admin caller (the read-only `user` role, or a `certificates:read` token) gets the same short classified reason `GET /api/health` uses, e.g. `dns-01 challenge or DNS provider failure`. The raw text can embed a third party's response body, so it is not disclosed more widely than `/health` does. |
| <span id="certificate-status-last-attempt"></span> `lastAttempt` | string (RFC 3339) | When the ACME manager last attempted to issue/renew this certificate. `acme` certificates only; absent on an HA follower, which does not run the manager. |
| <span id="certificate-status-sans"></span> `sans` | []string | Leaf certificate's subject alternative names (DNS names and IPs). |
| <span id="certificate-status-renew-at"></span> `renewAt` | string (RFC 3339) | When the ACME manager next renews this certificate, same format as `notAfter`. A time in the past means the renewal is due and has not succeeded yet. Issued `acme` certificates only; absent for `custom`, pending and disabled certificates, on an HA follower, and before the manager's first pass. |
| <span id="certificate-status-renew-reason"></span> `renewReason` | string | `lifetime` (the renewal rule set `renewAt`), `ari` (the CA's renewal information set an earlier time) or `changed` (the certificate's domains or `acme.profile` changed, so it is due now; `renewAt` is then the time of the pass). Absent alongside `renewAt`. |

Force an immediate renewal, ignoring every renewal window, with
`POST /api/certificates/{name}/renew` (`certificates:write` scope). It
responds `{"started": true}` once the order has **started**, not once it
completes: DNS-01 propagation alone can take minutes. Poll the status fields
above for the outcome.

Two things answer `409`, and the error message says which:

| Cause | Message | Fix |
|---|---|---|
| Another order is in flight anywhere on this instance | Names the in-flight order; nothing is queued | Wait for it and retry |
| This certificate is inside its **1-hour renew cooldown**, counted from `lastAttempt` (a failed attempt starts it too) | States the remaining wait, e.g. `retry in 42m10s` | Wait it out, and read `lastError` meanwhile |

`400` is a `custom` certificate (gpm does not renew those, replace the file and
`PUT` the object); `501` is an instance that is not the ACME issuer. See
[Certificate health](../../operations/certificate-health.md) for the
operational walkthrough and `GET /api/health` for the fleet-wide summary.

## Renewal

- **Rule.** An `acme` certificate is renewed when `min(30 days, lifetime / 3)` remains, lifetime being the leaf's `notAfter - notBefore`. A 90-day certificate keeps the 30-day window, a longer one keeps 30 days, and a 6-day certificate renews with 2 days left.
- **Never issued, or changed.** A certificate that was never issued, whose domain set changed or whose `acme.profile` changed is due at the next pass regardless of the window. For a changed one, `renewReason` is `changed` and `renewAt` is the time of the pass.
- **When that pass runs.** Nothing triggers a pass when a certificate is added or changed: it is ordered at the next pass, which is the next regular 12-hour pass, an early wake for another certificate, or a restart. To order at once, use **Renew now** in the admin UI or `POST /api/certificates/{name}/renew`.
- **Regular pass.** The manager runs a pass at start and then every 12 hours. After each pass it also wakes early when the next issued certificate enters its window, or when a due certificate's retry backoff ends (never sooner than one minute), so a short window is not stepped over.
- **Retries.**
  - The base wait is a quarter of that certificate's renewal window, at least 5 minutes and at most 1 hour (1 hour for a 90-day certificate and for one never issued; 5 minutes for a window of 20 minutes or less).
  - The n-th consecutive failure waits `min(base * 2^(n-1), 12 hours)`, counted from when the failed attempt started. For a 90-day certificate that is 1 h, 2 h, 4 h, 8 h, then every 12 h.
  - A success resets the count. The count is in memory, so a restart starts again at the base, and a failed manual renew counts as a failure too.
  - A pass inside the backoff skips the certificate.
  - A pass that cannot load the config is retried every 5 minutes until a load succeeds.
- **Attempt deadline.** Every attempt fails after the DNS propagation timeout plus 10 minutes (13 minutes with the defaults) with `the attempt did not finish within 13m0s: ...`. The backoff then applies, and the TXT records are still cleaned up.
- **ARI.** When the CA's directory advertises `renewalInfo` (RFC 9773, ACME Renewal Information; Let's Encrypt does), every pass (early wakes included) asks the CA when it wants each issued certificate renewed. It needs no key and no config, and the CA's `Retry-After` (seconds, clamped to 1 hour to 7 days, 6 hours by default) sets how often a certificate is asked again. A poll is at most one request per certificate per pass, plus one directory fetch per directory.
- **ARI only brings a renewal forward.** The certificate renews at the earlier of the lifetime rule and a time gpm picks inside the CA's suggested window (once per window); a CA cannot delay a renewal past the lifetime rule.
- **ARI guards.**
  - The `renewalInfo` URL is used only when its scheme and host, port included, equal the directory URL's.
  - The CA's suggested window is cut to the certificate's own validity; an answer with nothing left counts as a failed poll.
  - An ARI time cannot trigger a renewal while the certificate is younger than `min(72 hours, lifetime / 3)`.
  - Each renewal-information request times out after 10 seconds, and after one transport error the remaining polls against that directory are skipped for the pass.
  - A failed renewal-information request keeps the previous answer and is retried an hour later. A failed directory fetch, a client that cannot be built, or a leaf without an authority key identifier is retried on the next pass. None of them counts as a renewal failure.
- **`replaces` is not sent.** gpm polls only; a new order does not carry the ARI `replaces` field, so a CA's rate-limit exemption for ARI renewals does not apply.
- **State.** The ARI answer is kept in the certificate's `meta.json` under the cert directory, not in config.
- **Manual renewal.** `POST /api/certificates/{name}/renew` bypasses every window, ARI included, and is limited by the 1-hour cooldown below.
- **Status.** `renewAt` and `renewReason` show the next renewal time and which rule set it. The `cert.expiring` digest lists an `acme` certificate only when it is inside `notifications.expiringThresholdDays` **and** its renewal is already due, so with a threshold above 30 days a 90-day certificate is listed only once its renewal is due.

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
| `StreamHost` with `tls.mode: passthrough` | Forbidden. | Nothing, gpm never terminates. |

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
