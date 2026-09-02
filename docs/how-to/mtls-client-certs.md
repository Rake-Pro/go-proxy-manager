# Set up mTLS with client certificates

Generate a client CA in gpm, issue PKCS#12 client certificates from it, and
track what it issued.

## Generating a CA

`POST /api/client-cas/{name}/generate` creates a complete, issuance-capable CA
from nothing. Body (every field optional, an empty body is valid):

| Field | Default | Notes |
|-------|---------|-------|
| `commonName` | the ClientCA name | Subject CN of the generated CA, at most 64 characters. |
| `validityDays` | `3650` | 1-7300 (`0` means the default). Ten years by default because a CA is a trust anchor pinned into device configuration: rotating it means re-provisioning every device that trusts it. |
| `organization` | none | Optional subject `O`, at most 64 characters. |

It produces an **RSA-4096** self-signed certificate with `CA:TRUE, pathlen:0`,
`keyUsage certSign, cRLSign`, a 128-bit random serial and the same small backdate
issuance uses. RSA (not ECDSA) for the same reason the leaves are RSA (see the
issuance section below), and 4096 rather than 2048 because this key outlives the
certificates it signs by a decade. `pathlen:0` means it can never mint a
subordinate CA: it exists to sign client leaves, and a device trusting it should
not be trusting a tree underneath it.

The private key is written to **`<certDir>/client-cas/{name}.key`** at mode
`0600`. The path is derived from the object name, never supplied by the caller,
and is confined to the certificate store like any other `caKeyFile`. The key is
**never returned in any response and never logged**; the response is the created
ClientCA object, exactly as a `PUT` would return it.

Unlike issuance, this **is** a config mutation: the object goes through the normal
store save, so it is validated against the whole config graph, committed, and
appears in object history like any other write. An HA follower refuses it with
`503`, like every other write.

Two things it will not do:

- **It never replaces an existing ClientCA.** A name already in the config is a
  `409`; nothing is generated.
- **It never overwrites a key file that is still in use.** If
  `<certDir>/client-cas/{name}.key` exists *and* any ClientCA names that path as
  its `caKeyFile`, the request is a `409` naming that CA, and the file is left
  byte-for-byte alone. It may still be the signing key behind certificates already
  deployed to devices, so replacing it would invalidate every one of them.

An **unreferenced** key file at that path is different: no object points at it, so
it can only be residue: from a crash between writing the key and saving the
object, or from a ClientCA someone deleted (a delete deliberately keeps the key).
gpm reclaims it, logging a warning, and generates over it. Refusing forever would
otherwise make that name permanently unusable from the UI with no way to fix it.
If the config save fails after the key was written, gpm removes the key it just
wrote for the same reason.

A leftover `.tmp-*` file in `client-cas/` is swept on the next generate once it is
over an hour old. Those hold a raw private key from an interrupted generate; the
hour is there so a sweep can never delete the temp file of a generate running at
that moment.

> **Deleting a ClientCA does not delete its key file.** The object goes; the file
> at `<certDir>/client-cas/{name}.key` stays. This is deliberate and matches how
> a deleted [Certificate](../reference/config/certificate.md) leaves its ACME
> artifacts in the cert store. A delete is a *config* action and is revertible
> from git history: if the key were deleted with it, restoring the object would
> resurrect a CA pointing at a missing key, silently breaking issuance and CRL
> verification with no way back. Meanwhile every certificate already issued from
> that CA stays valid on the devices holding it, and keeping the key is what lets
> you sign a CRL for them after a mistaken delete. The cost is an orphan file: if
> you genuinely want the CA gone, remove it yourself after deleting the object.
> Until you do, re-generating under the same name **reclaims** that orphan (it is
> referenced by nothing) rather than refusing, so a deleted CA never blocks its
> own name.

## Issuing client certificates

A ClientCA with **no** signing key is a complete, normal object: it verifies
presented certificates and nothing else. Adding `caKeyFile` or `caKeyPEM` turns it
into an **issuing** CA, and the UI (ClientCA editor -> "Issue client certificate")
and `POST /api/client-cas/{name}/issue` can then mint client certificates from it.
Without a key the UI control is greyed out with the reason and the API answers
`422`; the button is never offered in a state where it can only fail.

The key must be the private key of one certificate in `caPEM`: that certificate
becomes the issuer. A key that parses but matches nothing in the bundle, or matches
a certificate that is not a CA, is **refused at config validation** (for an inline
`caKeyPEM`; a `caKeyFile` is checked the first time it is used, since only the data
plane knows the cert store path).

Issuance mints an **RSA-2048** key and a certificate with `ExtKeyUsage: clientAuth`,
`KeyUsage: digitalSignature`, `CA:false`, a 128-bit `crypto/rand` serial and a small
backdate for clock skew, then returns it as a **password-protected PKCS#12**
(`.p12`) download. RSA rather than ECDSA is deliberate: iOS rejects ECDSA client
certificates during the handshake, and RSA-2048 is the one key type the whole client
fleet handles. The bundle uses the **legacy** PKCS#12 encoder (SHA-1/3DES) rather
than modern PBES2 for the same reason: PBES2 bundles fail to import into the iOS
keychain and into several Android/Wear OS releases.

Request body: `commonName` (required, at most 64 characters), `password` (required,
**at least 12 characters**, it encrypts the bundle), `validityDays` (optional,
omit or `0` for the default `365`, otherwise 1-3650), and `sans` (optional list, at
most 32; an entry that parses as an IP becomes an IP SAN, one containing `@` an
email SAN, anything else a DNS SAN).

SANs must be **printable ASCII**: a certificate stores them as `IA5String`, so an
internationalised domain has to be supplied already punycoded (`xn--...`). A
non-ASCII or control character is refused with `400` rather than failing inside
ASN.1 encoding.

> **The bundle's at-rest protection is only as strong as its password.** The legacy
> PKCS#12 encoder gpm uses for device compatibility derives the bundle's integrity
> MAC with a *single* KDF iteration: there is essentially no work factor between
> the password and the key inside. A `.p12` travels by email, chat and shared
> folder and then lives on a phone, so anyone who obtains the file can attack a
> short password offline at line rate. The encoder cannot be hardened without
> breaking the iOS and Android imports this feature exists for, which is why gpm
> enforces a 12-character floor and why the password should be sent over a
> different channel than the file.

The issued **private key is never persisted, logged or recoverable**: it exists
only inside the response body. gpm logs the CA name, subject, serial and validity
window of every issuance and nothing else. The endpoint changes no config object, so
unlike every other write it creates **no revision and no history entry**; it is
gated by the same `client-cas:write` scope (and admin session, CSRF and same-origin
guard) as editing the CA, because minting a credential with the CA's key is at least
as privileged. An HA **follower** refuses it like every other POST; issue on the
leader.

## Issuance records, expiry warnings and renewal

Every issuance is remembered: `GET /api/client-cas/{name}/certificates` lists what
this CA has issued, newest first, and the ClientCA page in the UI shows the same
list. A record carries the CA name, common name, SANs, serial, `notBefore`,
`notAfter` and `issuedAt`, and **never** key material, the bundle, or its
password. gpm keeps no copy of the issued certificate either; the download was the
only one.

These records are **runtime state, not configuration**. They live in the
certificate store (`<certDir>/client-certs/<ca>.json`, written atomically at
`0600`, the same shape as the ACME issued-certificate metadata beside it), never in
the git-backed config repo, so they survive a restart but produce no config
revision and appear in no history. A record whose certificate expired more than a
year ago is pruned on the next issuance for that CA; nothing else is dropped.

Each record carries a derived `status`:

| `status` | Meaning |
|----------|---------|
| `ok` | More than `expiryWarningDays` of life left. |
| `expiring` | At or below `expiryWarningDays` days remaining (30 days by default, so exactly 30 days left is `expiring` and 31 is `ok`). |
| `expired` | Past `notAfter`. |

While any current record is `expiring` or `expired`, the ClientCA page shows a
banner naming each certificate and its remaining days. It says the thing that is
easy to forget: **there is no client-side renewal.** A client certificate lives in
a keychain on someone's device, so every renewal ends with a human importing the
new `.p12` there; plan the re-import before the expiry date, not after it.

`POST /api/client-cas/{name}/certificates/{serial}/renew` (the per-row **Renew**
button, behind an explicit confirmation) reissues the identity already on record
(the **same** common name and SANs, which are deliberately not accepted from the
request) with a **new private key and a new serial**, and returns a new PKCS#12
bundle exactly like issuance. Body: `password` (required) and optional
`validityDays` (same bounds and default as issuance). The old record is marked
`supersededBy` the new serial and stays listed.

Renewing an **already superseded** record is refused with `409`, naming the
certificate that replaced it. A second renewal of the same record would mint a
second live certificate for one identity and rewrite the supersede link, leaving
the first renewal current with nothing pointing at it; renew the current
certificate instead. The UI shows superseded rows as historical, with no renew
action.

> **Renewing does not revoke.** The superseded certificate remains valid until its
> own `notAfter`, and every device still holding it keeps working. Revocation is
> CRL-only: to actually kill the old certificate, add its serial to this CA's
> `crlFile` / `crlPEM`. That is exactly why the superseded record stays visible:
> it is the reminder that old copies are still installed somewhere.
