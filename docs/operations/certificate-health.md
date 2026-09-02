# Certificate health

Check certificate expiry and force a renewal without waiting for the ACME
manager's 30-day window.

## Prerequisites

- An admin session or an API token scoped `certificates:read` (status fields)
  and `certificates:write` (force renew).
- `curl` and `jq`, or the admin UI's Certificates page.

## Steps

1. List every certificate's status:
   ```
   curl -s -b "<admin session cookie>" https://<admin>/api/certificates | jq
   ```
   Each item carries `state`, `daysRemaining`, `issuer`, `sans` and, for `acme`
   certificates, `lastAttempt`/`lastError`: see
   [Status fields](../reference/config/certificate.md#status-fields-get-only).
2. Check one instance's aggregate health (proxy listeners, certificate
   counts by state, upstream-group health, the ACME loop's last run):
   ```
   curl -s -b "<admin session cookie>" https://<admin>/api/health | jq
   ```
3. Force an immediate renewal of one `acme` certificate:
   ```
   curl -s -X POST -b "<admin session cookie>" -H "X-CSRF-Token: <token from /api/me>" \
     https://<admin>/api/certificates/<name>/renew
   # {"started": true}
   ```

   **A 1-hour cooldown applies per certificate**, counted from its last attempt
   whether that attempt succeeded or failed, so a retry loop cannot hammer the
   CA into a rate limit. Inside it the call answers `409` and the message states
   the remaining wait (`retry in 42m10s`). Only one order runs at a time across
   the whole instance, which is the other `409`.

## Verify

- The renew response is `{"started": true}` as soon as the order begins: it
  does **not** wait for the order to finish (DNS-01 propagation alone can take
  minutes). Poll `GET /api/certificates/<name>` afterwards: `state` moves to
  `valid` and `lastError` clears once the new certificate lands, or `state`
  becomes `error` with `lastError` set to the CA's rejection detail. An **admin**
  caller sees that raw message; a viewer-role session or a `certificates:read`
  token sees the same short classified reason `GET /api/health` gives, so
  compare like with like when reading it from an automation credential.
- `GET /api/health`'s `certificates.expiring` / `.expired` / `.error` counts
  should match what `GET /api/certificates` reports for individual items.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `POST .../renew` returns `409`, message names an order in flight | Another order is already running anywhere on this instance; only one runs at a time and nothing is queued. | Wait for it to finish (poll `state`/`lastAttempt`) and retry. |
| `POST .../renew` returns `409`, message says `retry in ...` | This certificate is inside its **1-hour cooldown**, counted from `lastAttempt`: a failed attempt starts it too. | Wait out the stated remaining time, and read `lastError` meanwhile: a repeat renew will fail the same way until the cause is fixed. |
| `POST .../renew` returns `400` | The certificate is `type: custom`, not `acme`. gpm does not renew custom certificates. | Replace the certificate/key file on disk and `PUT` the object to pick it up. |
| `POST .../renew` returns `501` | This instance is not the ACME issuer: an HA follower, or ACME is not configured. | Issue against the HA leader instead. |
| `state: pending` never clears | The first ACME order has not completed, or keeps failing before it ever succeeds. | Check `lastError`; for `dns-01`, verify the [DNSProvider](../reference/config/dns-provider.md) credentials and zone; for `http-01`, verify port 80 is reachable from the internet. |
| `state: error` on a `custom` certificate | The configured `certFile`/`keyFile` could not be read or parsed as PEM. | Check `lastError` for the exact path/parse failure and that the file exists under the cert store. |
