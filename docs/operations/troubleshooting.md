# Troubleshooting

Every symptom documented across this site, in one lookup table. Each row links
to the page that explains the mechanism behind it.

## Startup and admin access

| Symptom | Cause | Fix |
|---|---|---|
| Login page loads, password never works, no error shown | The bcrypt hash file is unreadable by the non-root container user | `chmod 644` the secret file and restart ([Install with Docker](../getting-started/install-docker.md)) |
| Banner: no administrator login is configured | No usable local credential **and** no `oidc` provider in `adminAuth.providers` | Set `GPM_LOCAL_ADMIN_USER` plus a hash, or finish [admin OIDC](../how-to/admin-oidc-sso.md) |
| Login form answers "authentication failed" every time | `GPM_LOCAL_ADMIN_USER` is set but the hash is missing or unreadable | Generate it with `gpm hashpw` and mount it as `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` |
| Login page has no buttons and no password form | `ssoOnly: true` with a provider name that does not resolve to an `oidc` provider | Recover by editing `config/settings.yaml` and redeploying ([Admin authentication](../reference/config/settings/admin-auth.md)) |
| Login "does nothing" over `http://127.0.0.1:8081` | `GPM_COOKIE_SECURE=1` forces `Secure`, and a browser refuses to store a `Secure` cookie received over plain HTTP: the POST succeeds and the next request is anonymous | Leave `GPM_COOKIE_SECURE` at its `auto` default, which issues the cookie without `Secure` on a plain-HTTP request and turns `Secure` on by itself under TLS ([Hardening](hardening.md)) |
| The SPA shows an insecure-cookie banner | `GET /api/capabilities` reports `adminLogin.cookieSecure: insecure-public` - a session cookie was issued without `Secure` to a routable client address | Front the admin plane with TLS, or set `externalBaseURL` to its `https://` URL so `auto` marks the cookie `Secure` ([Admin UI behind gpm](../how-to/admin-ui-behind-gpm.md)) |
| Process exits at startup naming the config directory | The config dir is not writable by the `gpm` user | Fix ownership on the mounted volume |
| Startup fails on an unmigrated config tree | `config/dead-hosts/` still holds objects, or a stream host still uses `forwardHost`/`forwardPort` | Apply both renames in the config repo first ([Upgrading](upgrading.md)) |

## TOTP

| Symptom | Cause | Fix |
|---|---|---|
| Every code is rejected | Server and phone clocks differ by more than 30 seconds | Fix NTP on the host ([TOTP](../how-to/totp.md)) |
| Every code is rejected, log says the secret is not usable base32 | Stale container: gpm refuses to start on an unparseable secret | Regenerate with `gpm totp-secret` and restart |
| A code that just worked is refused | Codes are single-use; the accepted step cannot be replayed | Wait for the next 30-second step |
| `429 too many login attempts` | Five failed attempts from one client IP within 15 minutes | Wait 15 minutes, or restart (the lockout is in memory only) |
| Locked out with no authenticator | The secret is the only enrolment | Unset `GPM_LOCAL_ADMIN_TOTP_SECRET*`, restart, sign in, enrol a new secret |

## Certificates

| Symptom | Cause | Fix |
|---|---|---|
| `state: pending` never clears | The first ACME order has not completed or keeps failing | Read `lastError`; verify DNS-01 credentials, or that `:80` is reachable for HTTP-01 ([Certificate health](certificate-health.md)) |
| `state: error` on a `custom` certificate | `certFile`/`keyFile` could not be read or parsed as PEM | Check the path inside the cert store and the PEM contents |
| `POST .../renew` returns `409`, message names an order in flight | Another order is already running on this instance; nothing is queued | Wait and retry |
| `POST .../renew` returns `409`, message says `retry in ...` | The certificate is inside its 1-hour renew cooldown, counted from `lastAttempt` - failed attempts start it too | Wait out the stated time; read `lastError` meanwhile ([Certificate health](certificate-health.md)) |
| `POST .../renew` returns `400` | The certificate is `type: custom`; gpm does not renew those | Replace the file and `PUT` the object |
| `POST .../renew` returns `501` | This instance is not the ACME issuer (an HA follower) | Issue on the leader ([High availability](high-availability.md)) |
| TLS handshake fails with an unknown SNI | No enabled certificate covers the requested name | Add the domain to a certificate, or issue one |
| The wrong certificate is served | Selection is by SNI across every certificate, not by `tls.certificateRef` | Check which certificate's `domains` cover the name ([Certificate](../reference/config/certificate.md)) |
| Wildcard issuance refused at validation | A wildcard cannot be proven over HTTP-01 | Use DNS-01 ([DNS-01 with Cloudflare](../how-to/dns-01-cloudflare.md)) |
| ACME fails with a `dns-01` provider error | Wrong credential, missing zone scope, or a `dnsProvider` name typo | Check the provider object and test the token directly ([DNSProvider](../reference/config/dns-provider.md)) |
| Client certificate expired and nobody noticed | There is no client-side renewal; a `.p12` lives in a keychain | Watch the expiry banner and re-issue before `notAfter` ([mTLS](../how-to/mtls-client-certs.md)) |

## Routing and requests

| Symptom | Cause | Fix |
|---|---|---|
| `404` for a domain you configured | No enabled host claims it, or another host claims it first | Check `domains` and `disabled`; two enabled hosts cannot share a domain |
| Config write refused: two hosts claim one domain | Domains are exclusive across enabled hosts | Disable the old host in the same change ([Configuration model](../concepts/config-model.md)) |
| `503` on every request to one host | The host references a middleware or access list that does not exist | Fix the reference; the data plane fails that one host closed |
| The backend receives the wrong path | Location match, `stripPrefix`, rewrite rules and the upstream base `path` compose in a fixed order | See [Path composition](../concepts/request-pipeline.md#path-composition) |
| A guard answers `400` on a URL containing `;` | A guard with a `queryEquals` trigger fails closed on legacy `;` separators | Remove the `;`, or drop the `queryEquals` trigger |
| `502` reaching the admin UI through gpm | The proxy host's upstream does not match `GPM_ADMIN_ADDR` | Align both ([Admin UI behind gpm](../how-to/admin-ui-behind-gpm.md)) |
| Login redirects in a loop through gpm's own host | `externalBaseURL` does not match the browser's scheme and host | Set it to exactly that value |
| A stream backend stalls for seconds per connection | `proxyProtocol.enabled` is on but the relay does not send a header | Enable it on the relay first ([PROXY protocol](../reference/config/settings/proxy-protocol.md)) |

## Access control and identity

| Symptom | Cause | Fix |
|---|---|---|
| Access lists allow or deny the wrong clients | The compared address is the proxy's, not the client's | Declare the proxy in `settings.trustedProxies` ([Client IP](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers)) |
| An `allowFrom` network exempts everyone | An undeclared L7 proxy's own address falls inside it; on a `client-cert` host this is a total mTLS bypass | Declare the proxy, or move the rule into an access list ([Which IP `allowFrom` compares](../concepts/request-pipeline.md#which-ip-allowfrom-compares)) |
| `403` on every request to a host | The client IP is not covered by an attached access list | Check `defaultAction` and the CIDRs ([AccessList](../reference/config/access-list.md)) |
| Geo rules deny everything | Geo is configured but no GeoIP database is loaded; geo fails closed | Set `GPM_GEOIP_DB`, or remove the geo rules |
| SSO users are all denied `403` | Group claims are not in the ID token | gpm reads claims from the ID token; configure the IdP accordingly |
| Config write refused: `allowFrom` not permitted | `allowFrom` in `oidc` or `forward-auth` mode, including via an unset `mode` | Set `mode` explicitly, or use an access list |
| A LAN client is prompted for a password after migrating basic auth | The old list required both the IP and the password, so no `allowFrom` was copied | Add the CIDR to the middleware's `allowFrom` deliberately ([Migrate basic auth](../how-to/migrate-basic-auth.md)) |
| `409` from `migrate-basic-auth` | A middleware named `<list>-basic` already exists | Rename or delete it, or migrate by hand |
| SSO sessions end after an hour | The data-plane SSO cookie has a 1-hour absolute TTL | Expected; re-auth is silent while the IdP session is valid |

## DNS sync

| Symptom | Cause | Fix |
|---|---|---|
| `skipped > 0` in the sync status | A name a host wants is held by a record gpm does not own | Resolve it by hand; gpm never shadows a foreign record ([DNS sync](../how-to/dns-sync.md)) |
| A record gpm published disappeared | The host was disabled, which withdraws its DNS | Re-enable the host; created records are recreated, adopted ones were released |
| Pi-hole answers `403` | The session is read-only, or the instance lacks `webserver.api.app_sudo` | Fix the Pi-hole side; retrying will not help |
| A reconcile deleted a record after a revert | A whole-tree revert restored an ownership claim reality had moved past | Run `GET /api/dns-sync/plan` before letting a reconcile proceed after any revert |
| `409 Conflict` from `/reconcile` or `/plan` | A reconcile is already in flight; the endpoint never queues | Retry after it finishes |

## Discovery (Kubernetes and Docker)

| Symptom | Cause | Fix |
|---|---|---|
| `reachable: false` naming the socket | The Docker socket is not mounted, or the path is wrong | Mount it read-only, or point `host` at a socket proxy ([Docker discovery](../how-to/docker-discovery.md)) |
| `status 403` from the Engine endpoint | The socket proxy does not allow `CONTAINERS`/`EVENTS` | Allow exactly those two |
| `did not return a JSON array` | `host` points at something that is not the Engine API | Fix the endpoint; the run freezes rather than deleting anything |
| Container skipped: `port is required` | Zero or several exposed TCP ports | Set the `port` label explicitly |
| Container or Ingress skipped: outside `allowedDomainSuffixes` | The hostname is not inside the configured bound | Add the suffix, or fix the label/annotation |
| Skipped: not managed by this reconciler | A hand-written or other-reconciler host already owns that name or domain | Rename, or remove the other host |
| Host disabled with "profile ... is not defined" | The profile was renamed or retired; an unresolvable profile fails closed | Re-add the profile, or fix the label/annotation |
| `502` after a container is recreated | The container IP changed between reconciles | Wait for the next event-driven reconcile, or use `usePublishedPorts` |
| `lastSuccess` far behind `lastRun` | Every run since then has failed; managed hosts are frozen, not deleted | Read `status.error` ([Expose a Kubernetes Ingress](../how-to/expose-k8s-ingress.md)) |
| `caFile ... contains no usable PEM certificate` | The `ca.crt` jsonpath escaped the separator and wrote an empty file | Use `{.data.ca\.crt}`, escaping only the dot inside the key |
| The first reconcile writes nothing and reports an error | The template names a certificate, middleware or access list that does not exist | Create those objects, then enable discovery |

## Access-list remote sources

| Symptom | Cause | Fix |
|---|---|---|
| `refused > 0` in the source status | A feed answered non-`200`, was empty, over `maxEntries`, or held one unparseable line | Fix the feed; the previously fetched set keeps serving ([Remote IP feeds](../how-to/access-list-sources.md)) |
| A source rule matches nothing | The source has never been fetched, so it resolves to the empty set | Trigger a reconcile and check the status |
| A source rule grants more than intended | A `source` rule without `paths` is an ordinary allow rule | Pair every `source` rule with `paths` |

## Tunnels and relays

| Symptom | Cause | Fix |
|---|---|---|
| Access lists see the tunnel's address, not the client | `cloudflared` does not pass a client IP gpm reads | Use Cloudflare Access or an auth middleware instead ([Tunnels](../how-to/tunnels.md)) |
| `cloudflared` logs a TLS verification error | The served certificate does not match `originServerName` | Align the certificate's `domains` with the ingress rule's hostname |
| A tailnet peer gets connection refused | The listener is not reachable on the tailnet interface, or MagicDNS has not propagated | Check `tailscale status` and curl the tailnet IP directly |

## Profiling and metrics

| Symptom | Cause | Fix |
|---|---|---|
| `/metrics` answers `404` | Metrics are off by default | Set `GPM_METRICS=1` and restart ([Metrics](../reference/metrics.md)) |
| A scrape token gets `403` on `/metrics` | The token lacks `metrics:read` | Mint a token with exactly that scope |
| `go tool pprof` against a remote URL fails | It sends no session cookie and its symbolization step is a mutating request | Download the profile with the cookie and analyse the local file ([pprof](pprof.md)) |
| A resource-scoped token gets `403` on `/debug/pprof/` | Those endpoints need the `admin` scope | Use an admin-scope token or an admin browser session |
