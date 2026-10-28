# Analytics

The Analytics view (sidebar, Overview group) is an at-a-glance picture of the traffic
the edge handled. It reads `GET /api/analytics` and renders plain HTML and inline SVG;
no third-party script is loaded.

## What the page shows

| Block | Content |
|---|---|
| Filters | **Range**, **Host** and **Source** (all traffic, external only, internal only) apply to every block below; **Auto-refresh** re-reads on a timer |
| Tiles | Requests, denied, threat hits, 5xx, p95 latency, bytes out; each with a delta against the previous period of the same length and a sparkline |
| Traffic | One column per bucket: allowed on the bottom, denied stacked on top; threat hits as a line on the same count axis. Hover for the bucket's numbers; **Table** shows them as rows |
| Small charts | Requests, denied and threat hits, each on its own scale |
| Countries | Requests, denied and threat hits per ISO country code; `--` is unknown or private |
| Denials by reason | Which tier refused the request (`access-list`, `geo`, `bouncer`, `rate-limit`, `guard`, `auth-*`); counted once, by the first tier that refused |
| Threat signatures | Observe-only labels (see below) with hits and top countries |
| Hosts | Global view only: one row per proxy host plus `no host (-)` for requests that matched none |
| Top clients | Most active client addresses in the range |

## Range

- Presets: `1h`, `6h`, `24h`, `7d`, `30d`; every tile, chart and table uses the same range.
- Bucket width follows the range: 1 min, 5 min, 30 min, 3 h, 12 h.

## Host filter

- `All hosts` is the fleet view and includes the Hosts table.
- One host narrows every number to that proxy host; the URL becomes
  `#/analytics/<name>`, so the view can be bookmarked. The Hosts table rows link there.

## Source filter

- `All traffic`, `External only` or `Internal only`; the choice is remembered in the browser
  and applies to every tile, chart and table.
- Internal is a client address in a private (RFC 1918, IPv6 ULA), loopback, link-local
  or unspecified range, or in a network listed under
  [`settings.analytics.internalNetworks`](config/settings/analytics.md#settings-analytics-internal-networks)
  (a VPN or overlay range, for example). Everything else, including a request with no
  resolvable client address, is external.
- The source is decided once, when the request is aggregated, with the client address the
  host's trusted-proxy settings resolve. Changing the internal networks applies from then
  on; data already collected keeps its source. A host whose trusted proxies include
  addresses that are not your own proxies lets an outside client assert a private
  `X-Forwarded-For` and land in the internal view, so keep `trustedProxies` tight.
- Use it to keep LAN chatter (a home automation server polling an upstream, for example)
  out of the picture when reading internet-facing traffic, or to look at the LAN alone.
- Rollups written by a pre-release build from before the source existed carry no source:
  they count in `All traffic` only.

## Auto-refresh

- Off, every 15 s, or every minute; **Refresh** does it once.
- Refreshes repaint in place (no loading screen, scroll position and the Table toggle kept)
  and pause while the tab is hidden.

## Copying values

- Country codes, client addresses and signature labels show a copy button when their
  row is hovered or focused. A value containing control characters is copied
  percent-encoded.

## Threat signatures

| Label | Source |
|---|---|
| `wp-probe`, `secret-probe`, `admin-probe`, `webshell`, `iot-rce`, `traversal`, `injection`, `scanner-ua`, `proto-abuse` | Built-in observe-only classifier over path and User-Agent |
| `crowdsec:<scenario>` | CrowdSec decision scenario, when the bouncer is in use |
| `guard:<name>` | A named guard trigger |

A label counts a request; it never blocks one. Blocking stays with access lists,
guard, rate limits and the bouncer.

## API

- `GET /api/analytics?range=24h&host=<name>&scope=external` (scope `*:read`); `scope` is
  `external`, `internal`, or omitted for both. Schema `AnalyticsReport` in the
  [OpenAPI document](../reference/api.md).
- Collector off: `200` with `enabled: false`. Bad range, unknown host, bad scope or a range
  the retention does not cover: `400`. No analytics endpoint at all (older server): `501`.

## Where the numbers come from

- The collector is switched on with [`settings.analytics.enabled`](config/settings/analytics.md);
  off, the page says so and the API answers `enabled: false`.
- Tiles and the chart are minute resolution; the countries, reasons, signatures and
  clients tables are hourly resolution (a `1h` view includes the whole running hour and
  the previous one).
- `7d` and `30d` are served from the hourly rollups and appear only when
  `analytics.retentionDays` covers them.
- A `dropped` chip appears when the collector queue overflowed since start; the counts
  then undercount by that many.

## Seed data

- `GPM_ANALYTICS_DEMO=1` serves deterministic seed data (documentation-range addresses,
  generic host names) instead of the collector, so the view can be reviewed. The API marks
  it `demo: true` and the page shows a `Seed data` chip; only the `geoip` flag reflects the
  real instance.
