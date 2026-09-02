# Install the binary with systemd

Run gpm without a container runtime: one static CGO-free binary, a system
user, a data directory and a hardened unit file.

No container runtime is required. `git` must be on `PATH` at **runtime**, not
just to build: the config store shells out to it for every commit.

## Build

```
git clone https://github.com/Rake-Pro/go-proxy-manager
cd go-proxy-manager
make build              # -> bin/gpm, with VERSION/COMMIT/DATE stamped via ldflags
```

`go build -trimpath -o /usr/local/bin/gpm ./cmd/gpm` works too, but skips the
version stamping `make build` does - fine for a quick local test, not for
something you will run `gpm version` against later. The container image installs
`git` explicitly for the same reason.

## User, data directory and secrets

```
sudo useradd --system --no-create-home --shell /usr/sbin/nologin gpm
sudo install -d -o gpm -g gpm -m 0750 /var/lib/gpm /var/lib/gpm/config /var/lib/gpm/certs
sudo install -d -o root -g gpm -m 0750 /etc/gpm

sudo install -m 0755 bin/gpm /usr/local/bin/gpm

# Password hash: create the file 0600 FIRST, the same reasoning as the
# Ingress-discovery token below - a plain redirect creates it world-readable
# for the window between creation and chmod.
sudo install -m 0600 -o gpm -g gpm /dev/null /etc/gpm/admin_hash
/usr/local/bin/gpm hashpw 'your-password' | sudo tee /etc/gpm/admin_hash >/dev/null
```

## Environment file

`/etc/gpm/gpm.env`, mode `0640 root:gpm` so only the service can read it:

```
GPM_CONFIG_DIR=/var/lib/gpm/config
GPM_CERT_DIR=/var/lib/gpm/certs
GPM_SESSION_DB=/var/lib/gpm/session.db
GPM_LOCAL_ADMIN_USER=admin
GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE=/etc/gpm/admin_hash
GPM_LOG_LEVEL=info
```

Add any other flags from the [table above](../reference/env-vars-and-flags.md#flags-and-environment-variables)
as `GPM_*` lines here - there is no separate bare-metal flag surface.

## Unit file

`/etc/systemd/system/gpm.service`:

```
[Unit]
Description=go-proxy-manager
Documentation=https://github.com/Rake-Pro/go-proxy-manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=gpm
Group=gpm
EnvironmentFile=/etc/gpm/gpm.env
ExecStart=/usr/local/bin/gpm
Restart=on-failure
RestartSec=2s

# Bind :80/:443 as the non-root gpm user, with no setuid binary and no root
# process - the systemd equivalent of the container image's non-root USER.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true

# Filesystem hardening. A container gets most of this from its own root
# filesystem being throwaway; state it explicitly here instead.
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/gpm
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## Start and verify

```
sudo systemctl daemon-reload
sudo systemctl enable --now gpm
sudo systemctl status gpm
curl -fsS http://127.0.0.1:8081/healthz && echo   # -> ok
journalctl -u gpm -f                              # structured JSON logs; add GPM_LOG_CONSOLE=1 for human-readable
```

| Check | Expected |
|---|---|
| `systemctl status gpm` | `active (running)` |
| `curl -fsS http://127.0.0.1:8081/healthz` | `ok` |
| `curl -fsS http://127.0.0.1:8081/version` | The version you built |
| `journalctl -u gpm` | No `error` lines about the config directory or the admin credential |
