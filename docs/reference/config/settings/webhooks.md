# Settings: webhooks

Outbound lifecycle notifications: one JSON POST per successful config change.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-webhooks"></span> `webhooks` | []WebhookConfig | Outbound lifecycle notifications (below). |

## WebhookConfig (`webhooks[]`)

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="settings-webhooks-name"></span> `name` | string | - | yes | Name-safe identifier, used in the status and test endpoints and in logs. |
| <span id="settings-webhooks-url"></span> `url` | string | - | yes | Absolute `http`/`https` endpoint the event is POSTed to. |
| <span id="settings-webhooks-secret"></span> `secret` | Secret | none | no | Placeholder-resolved shared secret, sent as the `X-GPM-Webhook-Secret` header. |
| <span id="settings-webhooks-disabled"></span> `disabled` | bool | `false` | no | Keep the target configured without firing it. A disabled target is still reachable through the test endpoint. |

## Payload and delivery

After every successful config change gpm POSTs a JSON event
`{"action","kind","name","commit","time"}` to each enabled target. `action` is one
of `save` | `delete` | `restore` | `revert` | `settings` | `ingress-discovery` | `docker-discovery`. Delivery is asynchronous
and best-effort under a 10s timeout - a slow or unreachable endpoint never blocks
or fails the config write, it is only logged. Because targets are admin-configured
URLs, delivery is SSRF-bounded as defense in depth: **redirects are never
followed** (a 3xx counts as a failed delivery, so a receiver cannot bounce gpm to
a URL the admin didn't configure) and **link-local destinations are refused at
connect time, post-DNS** (blocking cloud-metadata pivots such as
`169.254.169.254` even via a rebinding resolver). Private/LAN targets remain
allowed - they are the normal self-hosted case.

**Delivery is bounded, and drops rather than queues.** At most 8 deliveries are
in flight at once. An event that would exceed that bound is **dropped for that
target with a `WARN`**, not queued: delivery is best-effort and must never block
or slow a config write. A target that is consistently dropping events is a
receiver that cannot keep up, not a gpm backlog to drain.

**Checking a target.** Delivery being fire-and-forget used to make a receiver
that had been failing for weeks look identical to a working one:

- `GET /api/webhooks/status` - one row per configured target with the outcome of
  its most recent attempt (`lastAttempt`, `status`, `durationMs`, `ok`, `error`).
  In-memory and per-process: it is an operational hint, not config, and it resets
  on restart.
- `POST /api/webhooks/{name}/test` - sends a synthetic
  `{"action":"test","kind":"Webhook","name":"<target>","time":"<RFC3339>"}` event
  and waits up to 5s for the answer. A **disabled** target is still tested, so a
  receiver can be proved before it is turned on. A refused or timed-out delivery
  comes back as `200` with `ok: false`; only an unknown target name is a `404`.
