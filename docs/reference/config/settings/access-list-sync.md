# Settings: access-list source sync

Keep the remote IP feeds an access list declares fetched into the committed
source ledger.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-access-list-sync"></span> `accessListSync` | AccessListSyncSettings | Remote [AccessList source](../access-list.md) fetching (below). Enabled by default. |

Keeps the remote IP feeds an [AccessList](../access-list.md)
declares in `sources` fetched into the committed ledger
`config/access-list-sources.yaml`.

| Field | Type | Notes |
|-------|------|-------|
| <span id="settings-access-list-sync-enabled"></span> `enabled` | bool | Turn the fetcher off. **Absent means enabled**: a deployment that declares a source wants it fetched. |
| <span id="settings-access-list-sync-poll-interval"></span> `pollInterval` | string | Go duration; how often the loop asks whether any source is *due*. Empty = `15m`; below `1m` is refused. |

```yaml
accessListSync:
  enabled: true
  pollInterval: 15m
```

`pollInterval` is not how often a feed is downloaded: each source's own
`interval` (default `24h`) decides that, so polling often is cheap. A fetch is
**refused whole**, keeping the previously fetched set, when the source answers
anything but `200`, returns no valid entries, returns more than `maxEntries`, or
contains a single line that is neither a comment nor a valid IP/CIDR. Refusals
are counted and named in `GET /api/access-list-sources/status`.

Only the **leader** fetches (the ledger is a config-repo commit); a follower
still serves the sets, since it replicates the ledger with the rest of the
config. See [Remote IP feeds](../../../how-to/access-list-sources.md) for the endpoints, the SSRF
guard, and the no-op/commit-churn behaviour.
