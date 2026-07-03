# Stream-host test harness

A self-contained harness that proves go-proxy-manager's raw **TCP/UDP stream
forwarding** works end to end, using the same image you deploy.

It runs:

- **echo** — a tiny static TCP+UDP echo backend (echoes whatever it receives).
- **gpm** — the `ghcr.io/rake-pro/go-proxy-manager:main` image with one
  `StreamHost` (`config/stream-hosts/echo.yaml`) forwarding published port
  **15432** (TCP **and** UDP) to `echo:9000`.

## Run

```sh
cd test/stream
docker compose up --build -d
./test.sh                 # asserts TCP + UDP echo through gpm's published port
docker compose logs gpm   # expect: "stream: tcp listener started" / "udp listener started"
docker compose down -v
```

`test.sh` opens a TCP connection and a UDP datagram to `127.0.0.1:15432`, sends a
marker, and asserts the echo comes back through gpm. Override the target with
`HOST=... PORT=... ./test.sh` (e.g. to hit a remote homelab host that has the
port published).

## Notes

- The gpm service runs as **root** here only so the bind-mounted `./config` is
  writable. The production image runs as the non-root `gpm` user — do not copy
  the `user: "0:0"` line into production.
- The data-plane HTTP/S/admin ports are parked on 8080/8443/8081 so the harness
  needs no privileged ports; only the stream port 15432 is published.
- To point at a **real backend** instead of the echo, edit
  `config/stream-hosts/echo.yaml` (`forwardHost` / `forwardPort` / `protocol`)
  and the published `ports:` in `docker-compose.yaml`, then `up -d` again — gpm
  reconciles the listeners on reload.
- `protocol` accepts `tcp`, `udp`, or `both`. A `listenPort` that fails to bind
  (already in use, or colliding with 80/443/admin) is logged and skipped, never
  fatal.
