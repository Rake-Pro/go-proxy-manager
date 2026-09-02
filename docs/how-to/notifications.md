# Send operational notifications

Alert on renewal failures, certificate expiry, upstream health flaps and a
frozen discovery reconciler, over ntfy, Discord or a generic JSON webhook.

## Prerequisites

- A receiver: an ntfy topic URL, a Discord webhook URL (it must contain
  `/api/webhooks/`), or any endpoint that accepts a JSON POST.
- For ntfy or a generic receiver that needs one, a bearer token stored as a
  `${FILE:...}` or `${ENV:...}` placeholder. Discord needs none - the webhook
  URL is itself the credential.
- The `admin` scope to write settings.

## Steps

1. **Add one or more targets** to `settings.notifications`:

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
   ```

2. **Filter per target, if wanted.** An empty `events` list selects the default
   set: every kind except `config.changed`, which is noisy and opt-in. The full
   event-kind table is in
   [Settings: notifications](../reference/config/settings/notifications.md).
3. **Prove the receiver before relying on it:**

   ```
   curl -s -X POST https://<admin>/api/notifications/ntfy-ops/test \
     -H 'Authorization: Bearer gpm_...' | jq
   ```

   A **disabled** target is still tested, so a receiver can be proved before it
   is turned on.
4. **Leave a target off** with `disabled: true` rather than deleting it, when you
   want to keep the configuration without firing it.

## Verify

| Check | Expected |
|---|---|
| `POST /api/notifications/{name}/test` | `200` with `ok: true`, and the message arrives at the receiver |
| `GET /api/notifications/status` | One row per target, with `lastAttempt`, `status`, `durationMs`, `ok` |
| No targets configured | Nothing is sent; the feature is entirely opt-in |

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Test returns `200` with `ok: false` | The delivery was refused or timed out; only an unknown target name is a `404` | Read the `error` field; check the URL, the token and that the receiver is reachable from the container |
| Discord rejects the post | The URL is not a webhook URL | It must contain `/api/webhooks/`; validation refuses anything else |
| Nothing arrives, status shows no attempt | The event kind is not in this target's `events` filter | Add the kind, or clear `events` to take the default set |
| `config.changed` never fires | It is off by default because it is noisy | Add it to that target's `events` explicitly |
| Events stop under load | At most 8 deliveries are in flight; an event over that bound is dropped with a `WARN` rather than queued | Fix the slow receiver - delivery is best-effort and must never block a config write |
| The status is empty after a restart | Delivery status is in-memory and per-process | Expected; it is an operational hint, not config |
| A secret is refused at commit | Literal secret values are never accepted | Use a `${FILE:...}` or `${ENV:...}` placeholder |

Alerts and lifecycle webhooks are separate subsystems:
[`settings.webhooks`](../reference/config/settings/webhooks.md) fires on every
config change, this one fires on operational events.
