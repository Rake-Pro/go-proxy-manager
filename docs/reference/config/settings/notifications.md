# Settings: notifications

Outbound operational alerts to ntfy, Discord or a generic JSON webhook.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-notifications"></span> `notifications` | NotificationsSettings | Outbound alert targets - ntfy/Discord/generic (below). |

Outbound operational alerts to ntfy, Discord, or a generic JSON webhook - a
renewal failure, an approaching or actual cert expiry, an upstream health
flap, an ACME account error, a frozen discovery reconciler, or (opt-in) a
config change. Delivery is asynchronous and best-effort, mirroring `webhooks`
above but sent through the separate `internal/notify` package (own targets,
own payload shapes, own event filtering). Empty (default) sends nothing.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| <span id="settings-notifications-targets"></span> `targets` | []NotificationTarget | no | The configured receivers (below). |
| <span id="settings-notifications-expiring-threshold-days"></span> `expiringThresholdDays` | int | no | Days before ACME expiry a certificate joins the daily `cert.expiring` digest. Default `14`. |

**NotificationTarget**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| <span id="settings-notifications-targets-name"></span> `name` | string | yes | Name-safe identifier, shown in logs and status. |
| <span id="settings-notifications-targets-type"></span> `type` | string | yes | `ntfy`, `discord`, or `generic`. |
| <span id="settings-notifications-targets-url"></span> `url` | string | yes | Absolute http/https endpoint: an ntfy topic URL, a Discord webhook URL (must contain `/api/webhooks/`), or any endpoint for `generic`. |
| <span id="settings-notifications-targets-secret"></span> `secret` | Secret | no | ntfy access token or `generic` bearer token, sent as `Authorization: Bearer <value>`. Unused for `discord` - the webhook URL is itself the credential. |
| <span id="settings-notifications-targets-disabled"></span> `disabled` | bool | no | Keep the target configured without firing it. |
| <span id="settings-notifications-targets-events"></span> `events` | []string | no | Subset of event kinds this target receives. Empty selects the default set (every kind below except `config.changed`). |

**Event kinds**

| Kind | When | Default on |
|------|------|------------|
| `cert.renewal_failed` | An ACME issue/renew attempt fails, for any reason (missing DNS provider, solver build, EAB, account/directory client, the order itself). | yes |
| `cert.expiring` | Daily digest of every ACME certificate within `expiringThresholdDays` of `notAfter`. One message lists every cert, not one message per cert. | yes |
| `cert.expired` | Daily digest of every ACME certificate already past `notAfter`. | yes |
| `upstream.unhealthy` | An [upstream group](../upstream-group.md) member's active/passive health check flips it down. | yes |
| `upstream.recovered` | The same member flips back up. | yes |
| `acme.account_error` | Reserved for ACME account/directory-level failures (currently folded into `cert.renewal_failed`; the kind exists for forward compatibility and manual test sends). | yes |
| `discovery.frozen` | A Kubernetes Ingress or Docker discovery reconcile fails past the freeze boundary (managed hosts are left untouched, stale). | yes |
| `config.changed` | Any successful config write (the same events `webhooks` fires on). Noisy by default - **opt-in per target**. | no |

**Payload shapes**

- `ntfy`: POST to the topic URL with the message as the plain-text body; `Title`, `Priority` (`3`/`4`/`5` for info/warning/critical), and `Tags` headers carry the summary and severity. `secret` rides as `Authorization: Bearer <token>`.
- `discord`: POST `{"content", "embeds": [{"title", "description", "color", "fields": [{"name", "value"}]}]}` to the webhook URL. No `Authorization` header - the URL is the credential.
- `generic`: POST `{"kind", "title", "body", "severity", "fields", "time"}` as JSON, with the kind repeated in the `X-GPM-Event` header. `secret` rides as `Authorization: Bearer <token>`.

**Delivery is bounded, and drops rather than queues.** At most 8 deliveries are
in flight at once. An event that would exceed that bound is **dropped for that
target with a `WARN`**, not queued: delivery is best-effort and must never block
or slow a config write. A target that is consistently dropping events is a
receiver that cannot keep up, not a gpm backlog to drain.

**Checking a target**, mirroring the webhook endpoints:

- `GET /api/notifications/status` - one row per configured target with the outcome of its most recent send. In-memory and per-process; resets on restart.
- `POST /api/notifications/{name}/test` - sends a synthetic event and waits up to 5s for the answer, bypassing the target's `events` filter and the dedup window. A **disabled** target is still tested. A refused or timed-out delivery comes back as `200` with `ok: false`; only an unknown target name is a `404`.

```yaml
notifications:
  expiringThresholdDays: 14
  targets:
    - name: ntfy-ops
      type: ntfy
      url: https://ntfy.example.com/gpm-alerts
      secret: ${FILE:/run/secrets/gpm_ntfy_token}
    - name: discord-ops
      type: discord
      url: https://discord.com/api/webhooks/123456789012345678/example-token
      events: [cert.renewal_failed, cert.expired, discovery.frozen]
    - name: audit-log
      type: generic
      url: https://hooks.example.com/gpm-notify
      secret: ${ENV:GPM_NOTIFY_TOKEN}
      events: [config.changed]
```
