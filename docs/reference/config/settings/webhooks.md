# Settings: webhooks

Outbound lifecycle notifications: one JSON POST per successful config change.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-webhooks"></span> `webhooks` | []WebhookConfig | Outbound lifecycle notifications (below). |

## WebhookConfig (`webhooks[]`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="settings-webhooks-name"></span> `name` | string | none | yes | Name-safe identifier, used in the status and test endpoints and in logs. |
| <span id="settings-webhooks-url"></span> `url` | string | none | yes | Absolute `http`/`https` endpoint the event is POSTed to, or a value that is wholly a `${ENV:...}` / `${FILE:...}` placeholder, resolved at each delivery. See [Keeping the url out of git](#keeping-the-url-out-of-git). |
| <span id="settings-webhooks-secret"></span> `secret` | Secret | none | no | Placeholder-resolved shared secret that signs every delivery. The receiver gets `X-GPM-Webhook-Timestamp` (Unix seconds at signing) and `X-GPM-Webhook-Signature: v1=<hex HMAC-SHA256(secret, timestamp + "." + raw body)>`, never the secret itself. Verify by recomputing the MAC over the raw body, comparing in constant time, and rejecting a timestamp more than a few minutes from your clock. |
| <span id="settings-webhooks-disabled"></span> `disabled` | bool | `false` | no | Keep the target configured without firing it. A disabled target is still reachable through the test endpoint. |

## Keeping the url out of git

For a receiver whose url carries the credential (a token in the path or query),
give `url` as a secret placeholder so only the reference is committed.

- **Whole value only.** `url: ${ENV:GPM_HOOK_URL}` or
  `url: ${FILE:/run/secrets/gpm_hook_url}` is resolved; a value that mixes text
  and a placeholder (`https://hooks.example.com/${ENV:TOKEN}`) is a literal and
  is sent as written, as before.
- **Checked when sent.** A placeholder skips the url check at write time, since
  the secret may not exist where the config is edited. At each delivery the
  resolved value must be an absolute `http`/`https` URL, or that delivery fails.
- **Same resolver and guards.** Resolution follows the `${ENV:}` / `${FILE:}`
  rules of every other secret (`GPM_SECRET_ENV_PREFIXES`,
  `GPM_SECRET_FILE_ROOTS`, reserved names refused). The resolved url goes through
  the same client as a literal one: no redirects, no link-local destinations.
- **Never echoed.** The whole resolved url, host included, is hidden: the API,
  the UI, `GET /api/webhooks/status`, the test result and the logs show the
  placeholder instead.
- **Transport errors are reduced to a cause.** Because the raw error names the
  host, address or TLS server name, a failed delivery to a placeholder url
  reports only `name resolution failed`, `connection failed`,
  `TLS handshake failed`, `timed out`, `refused by the outbound policy` or
  `other transport error`. A non-2xx answer still reports its HTTP status. A
  literal url keeps the full error text.
- **Failures stay per target.** A missing env var, a file outside the allowed
  roots or a resolved value of the wrong shape fails only that delivery, with an
  error naming the webhook and the placeholder.

```yaml
webhooks:
  - name: ci
    url: ${FILE:/run/secrets/gpm_ci_hook_url}
    secret: ${ENV:GPM_CI_HOOK_SECRET}
```

## Payload and delivery

After every successful config change gpm POSTs a JSON event
`{"action","kind","name","commit","time"}` to each enabled target.

- **`action`** is one of `save` | `delete` | `restore` | `revert` | `settings` | `ingress-discovery` | `docker-discovery`.
- **Asynchronous and best-effort**, under a 10s timeout: a slow or unreachable endpoint never blocks or fails the config write, it is only logged.
- **SSRF-bounded**, as defense in depth, because targets are admin-configured URLs:
  - **Redirects are never followed.** A 3xx counts as a failed delivery, so a receiver cannot bounce gpm to a URL the admin didn't configure.
  - **Link-local destinations are refused at connect time, post-DNS**, blocking cloud-metadata pivots such as `169.254.169.254` even via a rebinding resolver.
  - **What the guard refuses:** `169.254.0.0/16`, `fe80::/10` and link-local multicast, and any destination address it cannot read. It ignores an address zone when deciding, so a literal such as `[::ffff:169.254.169.254%25eth0]` is refused too.
  - **Private/LAN targets remain allowed:** they are the normal self-hosted case.

**Delivery is bounded, and drops rather than queues.** At most 8 deliveries are
in flight at once. An event that would exceed that bound is **dropped for that
target with a `WARN`**, not queued: delivery is best-effort and must never block
or slow a config write. A target that is consistently dropping events is a
receiver that cannot keep up, not a gpm backlog to drain.

**Checking a target.** Delivery being fire-and-forget used to make a receiver
that had been failing for weeks look identical to a working one:

- `GET /api/webhooks/status`: one row per configured target with the outcome of
  its most recent attempt (`lastAttempt`, `status`, `durationMs`, `ok`, `error`).
  In-memory and per-process: it is an operational hint, not config, and it resets
  on restart.
- `POST /api/webhooks/{name}/test`: sends a synthetic
  `{"action":"test","kind":"Webhook","name":"<target>","time":"<RFC3339>"}` event
  and waits up to 5s for the answer. A **disabled** target is still tested, so a
  receiver can be proved before it is turned on. A refused or timed-out delivery
  comes back as `200` with `ok: false`; only an unknown target name is a `404`.
