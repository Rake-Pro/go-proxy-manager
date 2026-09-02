# Install with Docker Compose

Run gpm as a container: verify the image, mount one data volume, publish the
two proxy listener ports, and keep the admin panel off the internet.

## Verifying the image

Every image pushed by the release workflow (including the `latest` tag, which
shares the same manifest digest as the version tags built alongside it) is
signed keylessly with [cosign](https://docs.sigstore.dev/cosign/) via GitHub
Actions OIDC: no key material to manage or leak. Verify before you deploy:

```
cosign verify \
  --certificate-identity-regexp '^https://github.com/Rake-Pro/go-proxy-manager/\.github/workflows/release\.yml@refs/(heads/prod|tags/v.*)$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/rake-pro/go-proxy-manager:latest
```

A successful verification prints the signature payload and confirms the image
was built by the `release.yml` workflow in this repository from a `v*` tag,
not from a fork, a different workflow, or a hand-pushed image.

## Compose file

```yaml
services:
  gpm:
    image: ghcr.io/rake-pro/go-proxy-manager
    restart: unless-stopped
    ports:
      - "443:443"
      - "80:80"
      - "127.0.0.1:8081:8081"     # admin: loopback only
    environment:
      GPM_LOCAL_ADMIN_USER: admin
      GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE: /run/secrets/admin_hash
    volumes:
      - gpm-data:/data
    secrets:
      - admin_hash
      - cf_token                  # for ACME (optional)
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]

secrets:
  admin_hash:
    file: ./secrets/admin_hash    # from `gpm hashpw`; chmod 644
  cf_token:
    file: ./secrets/cf_token      # Cloudflare API token; chmod 644

volumes:
  gpm-data:
```

> **Docker secret file permissions:** plain `docker compose` mounts file-based
> secrets with the host file's own permissions. The non-root `gpm` user must be
> able to read them, so `chmod 644` the secret files (the `uid`/`gid`/`mode`
> secret options only apply in Swarm).

> **First login needs no cookie setting.** `GPM_COOKIE_SECURE` defaults to
> `auto`: the admin session cookie is issued without `Secure` over
> `http://127.0.0.1:8081`, and becomes `Secure` (with the `__Host-` name)
> automatically once the request reaches gpm over TLS, arrives through a trusted
> proxy sending `X-Forwarded-Proto: https`, or `externalBaseURL` is an
> `https://` URL.

## Data directory

A single volume mounted at `/data`:

```
/data/config       git-backed config repo (see reference/config/)
/data/certs        certificate store (custom certs + ACME-issued artifacts,
                   client-CA CRLs, client-CA signing keys under client-cas/,
                   and client-certificate issuance records under client-certs/)
/data/session.db   SQLite session store (pure-Go, no CGO)
```

The container runs as a non-root user; make sure the mounted volume is writable by
it (the image's `gpm` user). The object model stored under `/data/config` is
documented in the [configuration reference](../reference/config/README.md).

`client-cas/<name>.key` is where a **generated** ClientCA's signing key lands
(`POST /api/client-cas/{name}/generate`, or "Generate new CA" in the UI): gpm
writes it at `0600` itself, so nothing has to be provisioned externally to get a
working mTLS setup. A key you place here yourself (`caKeyFile` pointing anywhere
under the cert store, for a bring-your-own CA) is the same thing by hand: give it
`0600` and owner `gpm`. The alternative is `caKeyPEM` with a
`${FILE:/run/secrets/...}` placeholder, which keeps the key in the secret mount
instead.

Either way it is a **CA private key**: back it up with the rest of `/data/certs`,
and remember it is *not* in the git config repo, so a config-only backup does not
carry it: restoring config alone gives you a ClientCA object pointing at a key
that is not there. Deleting a ClientCA does **not** delete its key file (see
[ClientCA](../reference/config/client-ca.md)), so removing a CA
for good is a config delete plus an `rm` here. CA generation, certificate issuance
and renewal are all `POST`s, so an HA **follower** refuses them with `503` like
every other write: do them on the leader.

`client-certs/<ca>.json` holds the issuance records that drive the expiry warning
and the renew action. They are runtime state, not config, so a config-only backup
does not carry them: back them up with the rest of `/data/certs`, and share the
cert dir between HA peers (which the [HA recipe](../operations/high-availability.md) already calls for) if you
want the follower to show the same list. Losing them loses only gpm's *memory* of
what was issued: the certificates themselves keep working, and the CA keeps
verifying them.

## IPv6

gpm listens dual-stack out of the box: a bare `:80` / `:443` bind accepts IPv4 and
IPv6 on the same socket, the router and every middleware are address-family
agnostic, and the client IP an access list, geo rule, rate limit or
`X-Forwarded-For` sees is whichever address the client actually used: a v6 client
appears as its v6 address, not a v4 stand-in. There is no IPv6 toggle to set.

Pinning a bind to one family is still possible and is a deliberate choice:
`GPM_HTTPS_ADDR=0.0.0.0:443` is v4-only, `GPM_HTTPS_ADDR=[::]:443` is v6 (with
v4-mapped addresses, so still both on Linux unless the host has
`net.ipv6.bindv6only=1`).

What usually blocks inbound IPv6 is Docker, not gpm. Standard Docker behaviour:

- **The daemon needs IPv6 enabled.** Set `"ip6tables": true` and (for a
  user-defined bridge) `"experimental": true` where your Docker version still
  requires it in `/etc/docker/daemon.json`, then restart the daemon. Without
  `ip6tables` the daemon does not program v6 NAT/forwarding rules and published
  ports are not reachable over v6.
- **The network needs `enable_ipv6`.** A compose network must declare it (and, on
  older Compose file versions, a v6 subnet):

  ```yaml
  networks:
    edge:
      enable_ipv6: true
      ipam:
        config:
          - subnet: fd00:dead:beef::/64   # ULA is fine; a routed /64 is better
  ```

- **The container needs a real IPv6 address**, not just a v4 port mapping. A
  published port on a v4-only network is unreachable over v6 no matter what gpm
  binds.
- **`userland-proxy` changes what the app sees.** With the default
  `"userland-proxy": true` the daemon's `docker-proxy` relays the connection, so
  the container sees the *proxy's* address as the peer: the real client IP is
  lost for both families. Setting `"userland-proxy": false` in
  `/etc/docker/daemon.json` keeps the connection on the kernel path (iptables /
  ip6tables DNAT) so the original client address arrives intact.
- **Host networking is the simplest alternative.** `network_mode: host` gives the
  container the host's addresses directly: no NAT, no userland proxy, real client
  IPs in both families, at the cost of losing port isolation (gpm then binds the
  host's `:80`/`:443` and every stream port directly).

If the edge in front of gpm is an L4 load balancer rather than the client, the
client IP problem is not a Docker one: turn on
[`settings.proxyProtocol`](../reference/config/settings/proxy-protocol.md)
so gpm reads the real client address (of either family) out of the PROXY header.
