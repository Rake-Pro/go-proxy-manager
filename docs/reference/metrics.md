# Prometheus metrics

The opt-in `/metrics` exposition on the admin listener: how to enable it, how
to scrape it, and every series it publishes.

## Enabling it

Off by default. Set `GPM_METRICS=1` (or `-metrics`) and restart; with it off,
`/metrics` answers `404`.

The endpoint is on the **admin server** (`-admin-addr`, default `:8081`), not
the data plane, because the payload is admin data: it names every proxy host,
stream host and certificate you have configured. It is gated by an admin-role
principal **plus**, for an API token, an explicit `metrics:read` scope, so the
credential you park in a Prometheus config can scrape and nothing else. Mint one
with a single scope:

```
curl -sS -X PUT https://gpm.example.com/api/api-tokens/prometheus \
  -H 'Content-Type: application/json' -b "$COOKIE" -H "X-CSRF-Token: $CSRF" \
  -d '{"scopes":["metrics:read"]}'
```

then scrape with `Authorization: Bearer gpm_...`. An admin browser session works
too and needs no scope.

## Cardinality

**Series cardinality is bounded by design.** Every `host` label is the
ProxyHost/StreamHost **name** from committed config, never the client's `Host`
header: a header is attacker-chosen, and using it would let one client mint
unbounded series and exhaust the process. A request matching no host is labelled
`-`. Each metric additionally caps its series count and folds the rest into a
single `__overflow__` series, so no bug downstream of that rule can grow memory
without limit.

There is no `prometheus/client_golang` dependency: the exposition is a small
internal implementation (`internal/metrics`), matching this project's rule that
every third-party dependency has to earn its place.

## Series

| Metric | Type | Labels |
|--------|------|--------|
| `gpm_build_info` | gauge | `version`, `commit`, `go` |
| `gpm_http_requests_total` | counter | `host`, `method`, `status` (class, e.g. `2xx`) |
| `gpm_http_request_duration_seconds` | histogram | `host` |
| `gpm_http_requests_in_flight` | gauge | none |
| `gpm_http_request_bytes_total` | counter | `host` |
| `gpm_http_response_bytes_total` | counter | `host` |
| `gpm_http_upstream_errors_total` | counter | `host` |
| `gpm_http_websocket_upgrades_total` | counter | `host` |
| `gpm_denials_total` | counter | `host`, `reason` (`rate-limit`, `access-list`, `access-list-auth`, `guard`, `geo`, `bouncer`) |
| `gpm_stream_connections_active` | gauge | `host` |
| `gpm_stream_connections_total` | counter | `host` |
| `gpm_acme_certificate_expiry_timestamp_seconds` | gauge | `certificate` |
| `gpm_acme_renew_failures_total` | counter | `certificate` |
| `gpm_dns_sync_last_run_timestamp_seconds` | gauge | none |
| `gpm_dns_sync_last_success_timestamp_seconds` | gauge | none |
| `gpm_dns_sync_backend_up` | gauge | `backend` |
| `gpm_dns_sync_records_desired` | gauge | `backend` |
| `gpm_dns_sync_records_managed` | gauge | `backend` |
| `gpm_ingress_discovery_enabled` | gauge | none |
| `gpm_ingress_discovery_last_run_timestamp_seconds` | gauge | none |
| `gpm_ingress_discovery_last_success_timestamp_seconds` | gauge | none |
| `gpm_ingress_discovery_discovered_ingresses` | gauge | none |
| `gpm_ingress_discovery_managed_hosts` | gauge | none |
| `gpm_ha_role` | gauge | `role` (1 for this instance's role, 0 for the other) |
| `gpm_go_goroutines` | gauge | none |
| `gpm_go_memstats_alloc_bytes` | gauge | none |
| `gpm_go_memstats_sys_bytes` | gauge | none |

## Alerting

The ACME series exist only on the **leader** (it is the only issuer; a zero
expiry on a follower would read as "expired" to any sane alert). `LastRun` and
`LastSuccess` are separate on both reconcilers on purpose: freeze-on-error is
precisely the state where they diverge, so alert on the gap between them:

```
time() - gpm_ingress_discovery_last_success_timestamp_seconds > 3600
gpm_acme_certificate_expiry_timestamp_seconds - time() < 7 * 86400
```
