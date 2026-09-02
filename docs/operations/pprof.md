# Profiling with pprof

On-demand CPU, memory and goroutine attribution for the running edge.

It is the tool for "why is gpm hot or slow" during a throughput investigation,
instead of guessing from `docker stats`. Off by default: it expands attack
surface and adds overhead while it is on.

## Enabling it

**Enable/disable:** set `GPM_PPROF=1` (or `-pprof`) and restart. Leave it OFF in
normal operation: profiles can expose in-memory data (secrets, session
material), and profiling itself costs CPU. Flip on for an investigation,
capture what you need, flip back off.

## Authentication

The endpoints are mounted on the **admin server** at `/debug/pprof/`, behind the
same gate as `/api/` (same-origin guard + admin-role session) **plus an `admin`
scope check**. There is no token-in-URL or basic-auth mode: you authenticate with
an admin browser session (`gpm_session` cookie), either on the LAN admin listener
(`-admin-addr`, default `:8081`) or via a proxied admin domain if you've fronted
the admin panel with a host (see "Admin OIDC" above).

An API token works too, but **only one holding the `admin` scope**: a heap dump
and `/debug/pprof/cmdline` contain resolved backend credentials (Cloudflare,
Pi-hole) in cleartext, and every token principal is admin-*role* by construction,
so the role gate alone would hand a `proxy-hosts:read` token the process memory.
A resource-scoped token gets `403` here.

## Endpoints

| Endpoint | Answers |
|----------|---------|
| `/debug/pprof/profile?seconds=30` | CPU hotspots |
| `/debug/pprof/heap` | Memory growth / allocation sources |
| `/debug/pprof/goroutine?debug=2` | Stalls / deadlocks (full goroutine dump) |
| `/debug/pprof/trace?seconds=5` | Scheduler / syscall timeline (`go tool trace`) |

## Capture workflow

A browser session cookie is required on every request:

```
# 30s CPU profile while reproducing load
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/profile?seconds=30" -o cpu.pprof
go tool pprof -http :8080 cpu.pprof

# heap snapshot
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/heap" -o heap.pprof

# goroutine dump (stalls/deadlocks)
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/goroutine?debug=2" -o goroutines.txt

# execution trace
curl -b "gpm_session=<session-id>" \
  "https://admin.example.com/debug/pprof/trace?seconds=5" -o trace.out
go tool trace trace.out
```

## Known limitation

`go tool pprof https://admin.example.com/debug/pprof/profile`
(direct remote attach) does not work. `go tool pprof` sends no session cookie,
and its symbolization step issues `POST /debug/pprof/symbol`, which fails
CSRF/auth like any other mutating admin request. Download the profile with the
cookie (as above) and run `go tool pprof` against the local file instead.
