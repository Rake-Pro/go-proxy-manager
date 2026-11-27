# Issue certificates from a private step-ca

Get gpm to order certificates from a private ACME CA (step-ca) whose root is not in the system trust store, including short-lived ones.

## Prerequisites

- A step-ca with an ACME provisioner, reachable from gpm over https. Its ACME directory URL has the shape `https://<ca-host>/acme/<provisioner>/directory`; check the provisioner name on your own step-ca.
- The CA's root certificate as a PEM file. Get it from your step-ca (for example with the `step ca root` command, or by copying the root file the CA was created with); check the step documentation for your version.
- A challenge gpm can complete: `http-01` needs the CA to reach gpm's plaintext `:80` listener, `dns-01` needs a [DNS provider](../reference/config/dns-provider.md) the CA can resolve.
- An `admin` session or an API token with the `admin` scope if you write the certificate through the API. Files in the config repo need no scope.

## Steps

1. Put the root certificate where gpm can read it:
   - Use a file such as `/run/secrets/internal_root_ca.pem`, or paste the PEM inline in the certificate.
   - A `${FILE:}` path must lie under the secret file roots: `/run/secrets` by default, or the directories in `GPM_SECRET_FILE_ROOTS` (changing it needs a restart).
   - A path outside the roots fails the renewal attempt.
2. Add the certificate:

   ```yaml
   # config/certificates/internal.yaml
   name: internal
   type: acme
   domains: [app.example.com]
   acme:
     email: admin@example.com
     challenge: http-01
     directoryURL: https://ca.example.com/acme/acme/directory
     caBundle: ${FILE:/run/secrets/internal_root_ca.pem}
   ```

   The inline form puts the PEM in the key (at most 65536 bytes, `CERTIFICATE` blocks only):

   ```yaml
   acme:
     caBundle: |
       -----BEGIN CERTIFICATE-----
       <base64 of the root certificate>
       -----END CERTIFICATE-----
   ```

3. For short-lived certificates, set the provisioner's certificate lifetime on the step-ca side (check your step-ca documentation).
   - gpm needs no setting for it: it renews at `min(30 days, lifetime / 3)`, so a 24-hour certificate renews with 8 hours left and a 6-day certificate with 2 days left.
   - A failed attempt is retried after a backoff that grows with consecutive failures, starting at a quarter of that window (at least 5 minutes, at most 1 hour: hourly for a 24-hour certificate) and doubling up to 12 hours. See [Renewal](../reference/config/certificate.md#renewal).
4. If the CA offers issuance profiles, name one with `acme.profile`. Leave it out for the CA's default.
5. Reference the certificate from a host (`tls.certificateRef: internal`).
6. Order it now instead of waiting for the next pass: use **Renew now** in the admin UI, or `POST /api/certificates/internal/renew`.
7. Know how the bundle is used:
   - **Only the bundle is trusted.** Every exchange with that certificate's ACME directory (account, order, renewal information) trusts the bundle, not the system roots. It does not change how gpm trusts anything else.
   - **No fallback.** A bundle that cannot be read or parsed fails the renewal attempt; gpm never retries with the system roots.
   - **Scope.** Through the API, setting or changing `directoryURL`, `eab` or `caBundle` needs the `admin` scope; a `certificates:write` caller gets `403`. A write that keeps them as stored, or clears them, stays on `certificates:write`.
   - **Placeholder.** A `${FILE:}` or `${ENV:}` value is not checked when written; it is read and parsed at every renewal attempt.
   - **Challenge.** `dns-01` needs the TXT record to be visible to the public resolver gpm checks (`1.1.1.1`) before it asks the CA, so a zone that only your internal DNS serves times out with `timed out after 3m0s waiting for DNS-01 TXT records to propagate`. Use `http-01` for internal names, with the CA able to reach gpm's `:80` listener.

## Verify

| Check | Expected |
|---|---|
| `GET /api/certificates/internal` after the order has run (a new certificate is ordered at the next pass, or when you use Renew now) | `state` moves from `pending` to `valid`, `issuer` is your CA, `lastError` is empty |
| `renewAt` and `renewReason` on the same response | `renewAt` is `notAfter` minus `min(30 days, lifetime / 3)`, `renewReason` is `lifetime` (or `ari` if your CA offers renewal information) |
| `curl -sI https://app.example.com/` from a client that trusts your root | A certificate issued by your CA |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Write refused: `refusing to commit literal secret(s): [...]; use ${ENV:...} or ${FILE:...} placeholders` | An `eab.hmacKey` (or another secret) was written as a literal | Use a `${ENV:}` or `${FILE:}` placeholder; `caBundle` itself may be literal PEM |
| Write refused: `acme.caBundle: contains text that is not a PEM CERTIFICATE block` | Text outside the PEM blocks, or a malformed block | Paste only the `-----BEGIN CERTIFICATE-----` blocks, nothing before, between or after |
| Write refused: `acme.caBundle: contains a "PRIVATE KEY" PEM block; only CERTIFICATE blocks are allowed` | A different PEM type was pasted (the block type is named in the message) | Use the root certificate, never a key |
| Write refused: `acme.caBundle: certificate 1 does not parse: ...` | A truncated or corrupt certificate | Copy the file again from your CA |
| Write refused: `acme.caBundle: contains no PEM CERTIFICATE block` | The value has no certificate in it | Check the file or paste |
| Write refused: `acme.caBundle: must be at most 65536 bytes, got <n>` | The value is too large | Keep only the root (and intermediates if needed) |
| `403` with `this write needs the admin scope: acme.directoryURL, acme.eab and acme.caBundle may only be set or changed by an admin` | The token has `certificates:write` but not `admin` | Use an admin session or an `admin` token, or edit the file in the config repo |
| `lastError` contains `x509: certificate signed by unknown authority` | No `caBundle` is set, or it holds a different root than the one that signed the CA's own TLS certificate | Set `caBundle` to the root that signs the CA's server certificate |
| `lastError` contains `acme.caBundle:` | A `${FILE:}` or `${ENV:}` placeholder that does not resolve (for a file: `is outside the allowed secret roots`, missing or unreadable), or whose content does not parse | Check the path or variable, that the file is under `/run/secrets` or `GPM_SECRET_FILE_ROOTS`, and that gpm can read it |
| `authorize order with profile "<p>": ... does not advertise a profile with name <p>` | The CA does not offer that profile; the message lists the ones it supports | Use a listed name, or remove `acme.profile` |
| `authorize order with profile "<p>": acme: certificate authority does not support profiles` | The CA advertises no profiles | Remove `acme.profile` |
| Write refused: `acme.profile: must be 1 to 64 characters of letters, digits, "_" or "-", got "<v>"` | The name has another character or is too long | Use the profile name exactly as the CA lists it |
