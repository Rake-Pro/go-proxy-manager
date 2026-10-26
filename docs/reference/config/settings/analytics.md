# Settings: analytics

The traffic analytics collector behind the Analytics view and `GET /api/analytics`.

| Field | Type | Default | Required | Description |
|---|---|---|---|---|
| <span id="settings-analytics"></span> `analytics` | object | `{}` | no | Collector settings. Omitted entirely by an untouched `settings.yaml`. |
| <span id="settings-analytics-enabled"></span> `analytics.enabled` | bool | `false` | no | Switches the collector on. Off costs nothing per request (the listeners serve the plain chain) and the Analytics page says it is off. |
| <span id="settings-analytics-retention-days"></span> `analytics.retentionDays` | int | `7` | no | Days of hourly rollups kept in the analytics database (1-365; `0` means the default). The `7d` range needs 7 or more, `30d` needs 30 or more; shorter retention hides those ranges. The in-memory 24 h window is unaffected. |
| <span id="settings-analytics-top-clients"></span> `analytics.topClients` | string | `"persist"` | no | `persist` keeps a bounded top list of client addresses in memory and in the rollups; `memory` keeps it for the 24 h window only; `off` never records client addresses in analytics. |

## How it works

- The observe wrapper hands every finished request to the collector with one
  non-blocking queue send (default depth 8192). A full queue drops the event and
  counts it; the Analytics page shows the dropped count. The request is never delayed.
- The worker resolves the client's country (when a GeoIP database is loaded), runs
  the observe-only signature classifier over path, query and User-Agent, and
  increments bounded aggregates: 24 h of one-minute counters per host and 25 hourly
  slots of countries, denial reasons, signatures and clients.
- Every key set is capped: 512 hosts, 64 signature labels per host-hour (the rest
  fold into `other`), 8 countries per label, 512 client addresses per host-hour (once
  reached, later new addresses are not tracked for that hour), countries by the ISO
  list (a report shows the top 30 plus one `Other` row). Path, query and User-Agent
  are never stored.
- Closed hours are written to the rollup database (`-analytics-db` /
  `GPM_ANALYTICS_DB`, default `/data/analytics.db`, same hardening as the session
  database): totals, status classes, latency buckets, denial reasons, countries,
  signatures with their top three countries, and the top 50 client addresses per
  host-hour recorded while `topClients` was `persist`. A `7d` or `30d` client table
  therefore shows the persisted top lists, which can differ from the 24 h window's
  full in-memory list. Rows past retention are pruned within 30 s of the change,
  only while the collector is enabled (switching it off freezes the history). At
  start the last 24 h, the running hour included, are replayed into memory; hourly
  totals are spread evenly over the hour's elapsed minutes. On shutdown the running
  hour is flushed (up to 5 s).
- `topClients: off` forgets every address already held, in memory and on disk.
  `memory` keeps new addresses for the 24 h window only and leaves rows already
  on disk to age out with retention; an address recorded under `memory` is never
  written to disk even if the mode is `persist` again later.
- Tables (countries, reasons, signatures, clients) are hourly resolution: a `1h` view
  includes the whole running hour and the previous one; tiles and the chart are
  minute resolution.
- A settings reload applies the switch, the retention and the clients mode live.

## Related

- [Analytics reference](../../analytics.md) for what the page shows and the API.
- [Guard middleware](../middleware.md) `triggers[].name` and the CrowdSec bouncer
  feed the `guard:<name>` and `crowdsec:<scenario>` signature labels.
