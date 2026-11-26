# Send operational notifications

Alert on renewal failures, certificate expiry, upstream health flaps and a
frozen discovery reconciler, over ntfy, Discord or a generic JSON webhook.

## Prerequisites

- A receiver: an ntfy topic URL, a Discord webhook URL (it must contain
  `/api/webhooks/`), or any endpoint that accepts a JSON POST.
- For ntfy or a generic receiver that needs one, a bearer token stored as a
  `${FILE:...}` or `${ENV:...}` placeholder. Discord needs none: the webhook
  URL is itself the credential, so give `url` itself as a placeholder to keep
  it out of git.
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
         url: ${FILE:/run/secrets/gpm_discord_url}
         events: [cert.renewal_failed, cert.expired, discovery.frozen]
   ```

   A `url` that is wholly a placeholder is resolved at each send and checked
   then. The resolved url, host included, never appears in the API, the UI,
   the status, the test result or the logs. A literal url such as
   `https://discord.com/api/webhooks/123456789012345678/example-token` works as
   well, but is committed to git as written.

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
| Events stop under load | The delivery queue holds at most 256 pending jobs; once full, a new one is dropped with a `WARN` rather than blocking | Fix the slow receiver: delivery is best-effort and must never block a config write |
| The status is empty after a restart | Delivery status is in-memory and per-process | Expected; it is an operational hint, not config |
| Status `error` names a `url` placeholder and says it is not set, or outside the allowed secret roots | The `url` placeholder cannot be resolved where gpm runs | Set the env var, or mount the file under a root in `GPM_SECRET_FILE_ROOTS` (default `/run/secrets`) |
| Status `error` says a `url` placeholder does not resolve to an absolute http(s) URL, or lacks `/api/webhooks/` | The resolved value fails the url rules, which run at send time for a placeholder | Fix the env var or file content; for `discord` it must be the full webhook URL |
| Status `error` reads only `name resolution failed`, `connection failed`, `TLS handshake failed`, `timed out`, `refused by the outbound policy` or `other transport error` | The `url` is a placeholder, so the transport detail (which names the host) is withheld | Test the resolved url from the gpm host directly, or temporarily use it as a literal url on a test target |
| A secret is refused at commit | Literal secret values are never accepted | Use a `${FILE:...}` or `${ENV:...}` placeholder |

Alerts and lifecycle webhooks are separate subsystems:
[`settings.webhooks`](../reference/config/settings/webhooks.md) fires on every
config change, this one fires on operational events.
