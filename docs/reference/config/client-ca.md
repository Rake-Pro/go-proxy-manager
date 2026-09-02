# ClientCA

The trust anchor for per-host mTLS: the CA bundle presented client certificates
are verified against (referenced by `tls.clientAuth.caRef`), plus its optional
revocation list. It is kept distinct from [Certificate](certificate.md)
because it verifies peers rather than identifying this server.

There are two ways to get one, and the UI presents them as the either/or they are:

- **Generate** (`POST /api/client-cas/{name}/generate`, or "Generate new CA" on
  the new-ClientCA page): gpm creates a self-signed CA, stores its private key,
  and saves the object ready to issue. No openssl, no files to place by hand.
- **Bring your own**: paste an existing CA certificate into `caPEM`. Add
  `caKeyFile`/`caKeyPEM` only if you also want to issue from it; a verify-only CA
  needs no key at all.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| <span id="client-ca-ca-pem"></span>  `caPEM` | string | yes | PEM CA bundle (one or more certificates). Public material, so it may be inline; a `${FILE:...}` / `${ENV:...}` placeholder also works. Must parse to at least one certificate at load. |
| <span id="client-ca-crl-file"></span>  `crlFile` | string | no | Certificate revocation list, PEM or DER, **relative to the cert store** (absolute paths and `..` are rejected, like a custom certificate's files). Re-read on every config reload and within 5 minutes of the file's mtime changing. |
| <span id="client-ca-crl-pem"></span>  `crlPEM` | string | no | Inline PEM CRL, for a small list kept in git. Mutually exclusive with `crlFile`; changes only on a config reload. |
| <span id="client-ca-crl-policy"></span>  `crlPolicy` | string | no | `fail-closed` (default) or `fail-open`: what happens when a configured CRL is unusable. Only valid alongside `crlFile`/`crlPEM`. |
| <span id="client-ca-ca-key-file"></span>  `caKeyFile` | string | no | Signing key for one certificate in `caPEM`, **relative to the cert store** (absolute paths and `..` are rejected, exactly like `crlFile`). Set it to let this CA **issue** client certificates. Mutually exclusive with `caKeyPEM`. |
| <span id="client-ca-ca-key-pem"></span>  `caKeyPEM` | Secret | no | The same signing key inline. A CA private key is a secret, so it must be a `${FILE:...}` / `${ENV:...}` placeholder: a literal value is refused at commit. Mutually exclusive with `caKeyFile`. |
| <span id="client-ca-expiry-warning-days"></span>  `expiryWarningDays` | int | no | How far ahead a certificate issued by this CA is reported as **expiring** (0-3650; `0` or unset uses the default `30`). Advisory only: nothing renews on its own. |

With no CRL configured, certificates are verified against the CA only: a revoked
but unexpired certificate still passes (the Go standard library's chain
verification checks no revocation). With one configured, the proxy listeners
reject any presented certificate whose serial is listed, after checking that the CRL is
**signed by this CA**, so dropping a file into the cert store cannot un-revoke
or mass-revoke anything.

`crlPolicy` governs the failure modes: a CRL that is missing, unparseable,
foreign-signed, or past its `nextUpdate`. `fail-closed` (the default) then
rejects **every** client certificate verified against this CA: an operator who
configured revocation asked for it to be enforced; `fail-open` accepts them and
logs a warning, for a host where availability outranks revocation. Either way an
unusable CRL never fails the config reload: unrelated hosts keep serving, exactly
like the GeoIP database's live fail-closed evaluation.

```yaml
name: corp
caPEM: ${FILE:/run/secrets/corp_client_ca.pem}
crlFile: corp.crl          # <certDir>/corp.crl, PEM or DER
crlPolicy: fail-closed     # default; fail-open accepts when the CRL is unusable
caKeyFile: client-cas/corp.key  # optional, enables issuance; what generate writes
expiryWarningDays: 30      # default; warn this far ahead of an issued cert expiring
```
