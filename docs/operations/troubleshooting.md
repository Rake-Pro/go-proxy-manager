# Troubleshooting

Every symptom documented across this site, in one lookup table. Each row links
to the page that explains the mechanism behind it.

## Start at the Overview

- **Panels.** The Overview opens with three panels (this instance, hosts by
  status and source, certificates by type and state with the next expiry);
  every non-zero count opens the list filtered to what it counts.
- **Needs attention.** Below them, refreshed on every visit or with its Refresh
  button, a list of problems only: problems sort before warnings, then hosts in
  maintenance, and every row links to the object or page that fixes it.

| Row | Severity | Source | Link |
|---|---|---|---|
| Certificate failed to renew, or has expired | problem | `GET /api/certificates` state `error` / `expired` | The certificate |
| Certificate expires in N days | warning | `GET /api/certificates` state `expiring` (inside `min(30 days, lifetime / 3)` of expiry) | The certificate |
| Upstream group has unhealthy members | problem | `GET /api/health` `upstreamGroups[]` | The group |
| Ingress or Docker discovery failed | problem | `GET /api/ingress-discovery/status`, `GET /api/docker-discovery/status` `error` | Integrations |
| Discovery skipped a host | warning | The same status routes, `hosts[]` with `action: skipped` and a `reason` | Integrations |
| Config warning | warning | `GET /api/health` `configWarnings[]` (an unknown YAML key, see [Upgrading](upgrading.md)) | History |
| Could not read a source | warning | The certificates, health or discovery status request failed | The page that owns it |
| Proxy host is in maintenance | info | `GET /api/proxy-hosts`, `maintenance: true` on an enabled host | The host |

A disabled integration (its status route answers 501) adds no row. A failed
proxy hosts read adds no row either: the Hosts panel says so instead. The tables
below map each row's wording to its cause and fix.

## Startup and admin access

| Symptom | Cause | Fix |
|---|---|---|
| Login page loads, password never works, no error shown | The bcrypt hash file is unreadable by the non-root container user | Give the file to the image's `gpm` user, mode `0400`, and restart ([Install with Docker](../getting-started/install-docker.md#secret-files)) |
| Banner: no administrator login is configured | No usable local credential **and** no `oidc` provider in `adminAuth.providers` | Set `GPM_LOCAL_ADMIN_USER` plus a hash, or finish [admin OIDC](../how-to/admin-oidc-sso.md) |
| Login page answers "Wrong username or password." every time | `GPM_LOCAL_ADMIN_USER` is set but the hash is missing or unreadable | Generate it with `gpm hashpw` and mount it as `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE` |
| Login page has no buttons and no password form | `ssoOnly: true` with a provider name that does not resolve to an `oidc` provider | Recover by editing `config/settings.yaml` and restarting ([Break-glass recovery](../reference/config/settings/admin-auth.md#break-glass-recovery)) |
| Login "does nothing" over `http://127.0.0.1:8081` | `GPM_COOKIE_SECURE=1` forces `Secure`, and a browser refuses to store a `Secure` cookie received over plain HTTP: the POST succeeds and the next request is anonymous | Leave `GPM_COOKIE_SECURE` at its `auto` default, which issues the cookie without `Secure` on a plain-HTTP request and turns `Secure` on by itself under TLS ([Hardening](hardening.md)) |
| Login over the plain-HTTP admin port does not stick while `externalBaseURL` is `https://...` | In `auto` mode an `https` `externalBaseURL` marks the session and login-state cookies `Secure`, which a browser does not store over plain HTTP, and the OIDC callback returns to the `externalBaseURL` host | Sign in through the HTTPS admin hostname; for break-glass on the plain-HTTP port, restart with `GPM_COOKIE_SECURE=0` for the repair and restore `auto` afterwards ([Break-glass recovery](../reference/config/settings/admin-auth.md#break-glass-recovery), [Environment variables and flags](../reference/env-vars-and-flags.md)) |
| The SPA shows an insecure-cookie banner | `GET /api/capabilities` reports `adminLogin.cookieSecure: insecure-public`: a session cookie was issued without `Secure` to a routable client address | Front the admin panel with TLS, or set `externalBaseURL` to its `https://` URL so `auto` marks the cookie `Secure` ([Admin UI behind gpm](../how-to/admin-ui-behind-gpm.md)) |
| Process exits at startup naming the config directory | The config dir is not writable by the `gpm` user | Fix ownership on the mounted volume |
| Startup fails on an unmigrated config tree | `config/dead-hosts/` still holds objects, or a stream host still uses `forwardHost`/`forwardPort` | Apply both renames in the config repo first ([Upgrading](upgrading.md)) |
| Load, reload, backup, revert or restore fails with `symbolic link does not resolve to a file inside the config directory` | A file named in the error (or its kind directory) is a symbolic link whose target is outside the config directory, missing, or a link loop | Replace the link with the file itself, or point it at a file inside the config directory ([Symbolic links](../concepts/config-model.md#symbolic-links)) |

## TOTP

| Symptom | Cause | Fix |
|---|---|---|
| Every code is rejected | Server and phone clocks differ by more than 30 seconds | Fix NTP on the host ([TOTP](../how-to/totp.md)) |
| Every code is rejected, log says the secret is not usable base32 | Stale container: gpm refuses to start on an unparseable secret | Regenerate with `gpm totp-secret` and restart |
| A code that just worked is refused | Codes are single-use; the accepted step cannot be replayed | Wait for the next 30-second step |
| Login page says "Too many attempts. Try again in N minutes." (HTTP 429) | Five failed attempts from one client IP, or twenty against that username from any clients, within 15 minutes | Wait the minutes shown, or restart (the lockout is in memory only) |
| Locked out with no authenticator | The secret is the only enrolment | Unset `GPM_LOCAL_ADMIN_TOTP_SECRET*`, restart, sign in, enrol a new secret |
| Login says too many attempts, but this client never failed | The username is locked by guesses from other clients (twenty in 15 minutes) | Wait out the window, or sign in through an SSO provider; the log names the locked username |
| A guarded path answers 403 on a request that repeats a query key (`?k=0&k=1`) | A repeated `queryEquals` key counts as matching the trigger (fail closed) | Send the key once, or come from an `allowFrom` network |
| A guarded path answers 403 on a key spelled differently (`?%20k=1`, `?k[]=1`, `?k.x=1` for `k_x`) | A `queryEquals` key also matches the spellings a lenient backend reads as that key (fail closed) | Send the key as the trigger names it, or come from an `allowFrom` network |
| Restore did not bring back the API tokens in the archive | Restore keeps the live tokens and ignores the archive's, like Revert | Re-create the token you need; the ignored files are listed in the log |

## Certificates

| Symptom | Cause | Fix |
|---|---|---|
| `state: pending` never clears | The first ACME order has not completed or keeps failing | Read `lastError`; verify DNS-01 credentials, or that `:80` is reachable for HTTP-01 ([Certificate health](certificate-health.md)) |
| `state: error` on a `custom` certificate | `certFile`/`keyFile` could not be read or parsed as PEM | Check the path inside the cert store and the PEM contents |
| DNS-01 fails with `refusing to use a partial listing` | A Hetzner or deSEC listing ran past 100 pages, so the solver stopped instead of acting on part of the account | Use a token scoped to fewer zones or domains |
| `POST .../renew` returns `409`, message names an order in flight | Another order is already running on this instance; nothing is queued | Wait and retry |
| `POST .../renew` returns `409`, message says `retry in ...` | The certificate is inside its 1-hour renew cooldown, counted from `lastAttempt`: failed attempts start it too | Wait out the stated time; read `lastError` meanwhile ([Certificate health](certificate-health.md)) |
| `POST .../renew` returns `400` | The certificate is `type: custom`; gpm does not renew those | Replace the file and `PUT` the object |
| `POST .../renew` returns `501` | This instance is not the ACME issuer (an HA follower) | Issue on the leader ([High availability](high-availability.md)) |
| TLS handshake fails with an unknown SNI | No enabled certificate covers the requested name | Add the domain to a certificate, or issue one |
| The wrong certificate is served | Selection is by SNI across every certificate, not by `tls.certificateRef` | Check which certificate's `domains` cover the name ([Certificate](../reference/config/certificate.md)) |
| Wildcard issuance refused at validation | A wildcard cannot be proven over HTTP-01 | Use DNS-01 ([DNS-01 with Cloudflare](../how-to/dns-01-cloudflare.md)) |
| Certificate wizard: `dns-01` is greyed out, or a wildcard stops at `dns-01 needs a DNS provider` | No DNS provider is configured | Add one under DNS Providers ([DNSProvider](../reference/config/dns-provider.md)) and open the wizard again, or use `http-01` for names without a wildcard |
| Certificate wizard: `Enter the CA's directory URL, or pick a CA from the list.`; editor: `Directory URL required` | Custom is picked with a blank URL, and a blank `directoryURL` means Let's Encrypt | Enter the CA's directory URL, or pick a CA from the list ([CA directories](../reference/config/certificate.md#certificate-ca-directories)) |
| Certificate wizard: `ZeroSSL requires external account binding` (or Google Public CA) | The directory URL to be written belongs to a CA that requires EAB | Enter the EAB key ID and HMAC key, the key as a `${ENV:...}` or `${FILE:...}` placeholder |
| Import from YAML: `A literal EAB HMAC key is not carried over by an import` | The API never returns a literal secret, so the parsed document reads `***` | Enter the key as a `${ENV:...}` or `${FILE:...}` placeholder on the Authority step |
| ACME fails with a `dns-01` provider error | Wrong credential, missing zone scope, or a `dnsProvider` name typo | Check the provider object and test the token directly ([DNSProvider](../reference/config/dns-provider.md)) |
| `lastError` contains `x509: certificate signed by unknown authority` for a private CA | The CA's own TLS certificate is not signed by a root gpm trusts for that certificate | Set `acme.caBundle` to the CA's root ([Private step-ca](../how-to/step-ca.md)) |
| `lastError` starts with `the attempt did not finish within 13m0s` | The attempt (order, DNS propagation wait and validation) ran past the propagation timeout plus 10 minutes, so it was cancelled | Find the slow step (the CA, the DNS provider or the endpoint); the retry backoff applies and the TXT records are still cleaned up ([Renewal](../reference/config/certificate.md#renewal)) |
| `lastError` says the CA does not support profiles, or does not advertise the profile | `acme.profile` names a profile the CA does not offer | Use a name from the CA's directory, or remove `acme.profile` |
| A short-lived certificate renews with less than 30 days left (when `lifetime / 3` remains), or earlier than that rule when `renewReason` is `ari` or `changed` | Renewal happens at `min(30 days, lifetime / 3)`, and a CA that offers ARI can bring it forward, and `changed` means its domains or `acme.profile` changed | Nothing to fix; read `renewAt` and `renewReason` ([Renewal](../reference/config/certificate.md#renewal)) |
| `lastError` from the `webhook` DNS provider shows a non-2xx status, or a redirect | The endpoint refused the request, or redirected (redirects are never followed) | [Solve DNS-01 through your own endpoint](../how-to/dns-webhook.md#troubleshooting) |
| Client certificate expired and nobody noticed | There is no client-side renewal; a `.p12` lives in a keychain | Watch the expiry banner and re-issue before `notAfter` ([mTLS](../how-to/mtls-client-certs.md)) |

## Routing and requests

| Symptom | Cause | Fix |
|---|---|---|
| `404` for a domain you configured | No enabled host claims it, or another host claims it first | Check `domains` and `disabled`; two enabled hosts cannot share a domain |
| Config write refused: `domains: <name> is already claimed by proxy host "<other>"` | Domains are exclusive across enabled hosts | Disable the old host in the same change ([Configuration model](../concepts/config-model.md)) |
| Config write refused, or a config warning: `domain <name> is claimed by ...: the router serves it from only one of them` | Two hosts list spellings of one name, such as `<name>` and `<name>:8443`; the router drops the port | Remove the port (or the duplicate) from one host ([Domains are exclusive](../concepts/config-model.md#domains-are-exclusive)) |
| Config write refused: `the data plane cannot build the resulting config: ...` | The object validates but the proxy listeners could not load it (certificate files, `errorPages.dir`, a client CA bundle) | Fix what the message names; see [Writes must compile](../concepts/config-model.md#writes-must-compile) |
| Config warning: `object name "<n>" does not match the file name` | A file in the config repo was renamed or restored under another name | Save the object once (it moves to `<n>.yaml` in the same commit), or rename the file in the config repo |
| `503` on every request to one host | The host references a middleware or access list that does not exist | Fix the reference; the proxy listeners fail that one host closed |
| The backend receives the wrong path | Location match, `stripPrefix`, rewrite rules and the upstream base `path` compose in a fixed order | See [Path composition](../concepts/request-pipeline.md#path-composition) |
| An application's own pages return `404` after `settings.pathBlock` was set | A list entry matches the application's own paths (`.php` on a PHP application, `wp-admin` on a WordPress site); `gpm_denials_total` counts it with the reason `path-block` | Set `pathBlock: off` on the host, or narrow the list ([Block scanner paths](../how-to/block-scanner-paths.md#troubleshooting)) |
| Sign-in loops or fails on a host after `settings.pathBlock` was set | An entry matches the auth gate's own path (the `auth-request` outpost prefix or `/__gpm/oidc/callback`); the path block runs outside authentication | Remove the matching entry, or set `pathBlock: off` on the host |
| A guard answers `400` on a URL containing `;` | A guard with a `queryEquals` trigger fails closed on legacy `;` separators | Remove the `;`, or drop the `queryEquals` trigger |
| `502` reaching the admin UI through gpm | The proxy host's upstream does not match `GPM_ADMIN_ADDR` | Align both ([Admin UI behind gpm](../how-to/admin-ui-behind-gpm.md)) |
| Login redirects in a loop through gpm's own host | `externalBaseURL` does not match the browser's scheme and host | Set it to exactly that value |
| A stream backend stalls for seconds per connection | `proxyProtocol.enabled` is on but the relay does not send a header | Enable it on the relay first ([PROXY protocol](../reference/config/settings/proxy-protocol.md)) |
| `502` on one path, log line `upstream sent too many interim (1xx) responses` | The upstream sent more than 256 interim responses for one request | Fix the upstream; gpm relays the first 16 interim responses and ends the exchange past 256 |

## Access control and identity

| Symptom | Cause | Fix |
|---|---|---|
| Access lists allow or deny the wrong clients | The compared address is the proxy's, not the client's | Declare the proxy in `settings.trustedProxies` ([Client IP](../concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers)) |
| An `allowFrom` network exempts everyone | An undeclared L7 proxy's own address falls inside it; on a `client-cert` host this is a total mTLS bypass | Declare the proxy, or move the rule into an access list ([Which IP `allowFrom` compares](../concepts/request-pipeline.md#which-ip-allowfrom-compares)) |
| `403` on every request to a host | The client IP is not covered by an attached access list | Check `defaultAction` and the CIDRs ([AccessList](../reference/config/access-list.md)) |
| Adding a second access list to a host locks clients out that the first list allows | Lists combine as `accessListsMode: all` by default, so every list must allow | If both lists are allow-lists (`defaultAction: deny`, no `geo.countryDeny`), set `accessListsMode: any` on the host. A blocklist-style list cannot join an `any` group; keep `all` and widen the base list instead ([Composing access lists](../reference/config/access-list.md#access-list-composing)) |
| Config write refused: `... has defaultAction: allow` or `... has geo.countryDeny ... under accessListsMode: any` | `any` composes allow-lists only; a blocklist would admit everyone it does not block | Set the list's `defaultAction: deny` (or use `geo.countryAllow`), or keep `accessListsMode: all` |
| `403` with `reason="access-list-any"` | No list in an `any` group allowed the request. A `source` rule whose feed is not fetched yet matches nothing, and a `disabled` member is skipped | Turn on `debug` logging to see the host and the lists consulted, then check each list on its own |
| `403` with `reason="geo"` on an `any` host | No member allowed, and a member has `geo` rules while no GeoIP database is loaded, so that member fails closed | Load the database (`GPM_GEOIP_DB`) or remove the geo rules |
| Geo rules deny everything | Geo is configured but no GeoIP database is loaded; geo fails closed | Set `GPM_GEOIP_DB`, or remove the geo rules |
| SSO users are all denied `403` | Group claims are not in the ID token | gpm reads claims from the ID token; configure the IdP accordingly |
| Config write refused: `allowFrom` not permitted | `allowFrom` in `oidc` or `forward-auth` mode, including via an unset `mode` | Set `mode` explicitly, or use an access list |
| A LAN client is prompted for a password after migrating basic auth | The old list required both the IP and the password, so no `allowFrom` was copied | Add the CIDR to the middleware's `allowFrom` deliberately ([Migrate basic auth](../how-to/migrate-basic-auth.md)) |
| `409` from `migrate-basic-auth` | A middleware named `<list>-basic` already exists | Rename or delete it, or migrate by hand |
| Write refused: `user "<name>": passwordHash is the redacted placeholder "***", but ...` | API reads return every `passwordHash` as `***`, and `***` only keeps a hash gpm can place without guessing. `this user list has no stored user of that name`: the list exists and never held that user. `no stored user of that name`: the user is new or was renamed. `stored with different hashes in ...`: the username has several hashes in the object and the list is new | Send a `password` for that user ([Basic-auth password hashes](../reference/api.md#basic-auth-password-hashes)) |
| A script that copied `passwordHash` from an API read gets `***` | Reads no longer return stored hashes | Read the hashes from the config files or a `GET /api/backup` archive, or set passwords with `password` |
| SSO sessions end after an hour | The per-host SSO cookie has a 1-hour absolute TTL | Expected; re-auth is silent while the IdP session is valid |

## DNS sync

| Symptom | Cause | Fix |
|---|---|---|
| `skipped > 0` in the sync status | A name a host wants is held by a record gpm does not own | Resolve it by hand; gpm never shadows a foreign record ([DNS sync](../how-to/dns-sync.md)) |
| A record gpm published disappeared | The host was disabled, which withdraws its DNS | Re-enable the host; created records are recreated, adopted ones were released |
| Pi-hole answers `403` | The session is read-only, or the instance lacks `webserver.api.app_sudo` | Fix the Pi-hole side; retrying will not help |
| A reconcile deleted a record after a revert | A whole-tree revert restored an ownership claim reality had moved past | Run `GET /api/dns-sync/plan` before letting a reconcile proceed after any revert |
| `409 Conflict` from `/reconcile` or `/plan` | A reconcile is already in flight; the endpoint never queues | Retry after it finishes |
| `ok: false` after an `apexTarget` change, other names still retargeted | The backend refused the delete or the create of one retarget; the run went on with the rest | Read `error`: the original record still stands (or was restored) and the next run retries, or `restoring the original record also failed` and the next run recreates the name ([Settings: DNS sync](../reference/config/settings/dns-sync.md)) |

## Discovery (Kubernetes and Docker)

| Symptom | Cause | Fix |
|---|---|---|
| `reachable: false` naming the socket | The Docker socket is not mounted, or the path is wrong | Mount it read-only, or point `host` at a socket proxy ([Docker discovery](../how-to/docker-discovery.md)) |
| `status 403` from the Engine endpoint | The socket proxy does not allow `CONTAINERS`/`EVENTS` | Allow exactly those two |
| `did not return a JSON array` | `host` points at something that is not the Engine API | Fix the endpoint; the run freezes rather than deleting anything |
| Container skipped: `port is required` | Zero or several exposed TCP ports | Set the `port` label explicitly |
| Container or Ingress skipped: outside `allowedDomainSuffixes` | The hostname is not inside the configured bound | Add the suffix, or fix the label/annotation |
| Skipped: `a proxy host with this name exists and is not managed by <subsystem>` | A hand-written or other-reconciler host already has that name | Rename, or remove the other host |
| Skipped: `a <kind> with this name exists; host names are shared by proxy, redirect, parked and stream hosts` | A redirect, parked or stream host already has the derived name | Rename that host, or rename the source object |
| Host disabled with `fails closed rather than keep serving a chain or domain its profile no longer renders` | The source cannot be derived, and the stored host differs from its profile in a stamped field (upstream for Ingress discovery, TLS, `timeouts`, `robotsNoIndex`, chain or header policy) or a domain | Fix the source object; the next clean derive re-enables the host |
| A save, delete, revert or restore is refused with `ingressDiscovery.<path>` or `dockerDiscovery.<path>` | The result would leave a discovery template or profile naming an object that is missing or unusable | Change the template or profile first, then retry |
| Config warning naming `ingressDiscovery.<path>` or `dockerDiscovery.<path>` at load | The stored config already has such a reference (a hand edit or a `git pull`) | Fix the reference; hosts derived from that template or profile fail to reconcile until then |
| Skipped: `domain "<d>" is already claimed by <kind> "<h>", which <subsystem> does not manage` | A hand-written or other-reconciler proxy host, a redirect or a parked host serves `<d>` | Remove `<d>` from the source object, or from `<h>` |
| Skipped: `domain "<d>" is already claimed by proxy host "<h>", also managed by <subsystem>, which holds it after this reconcile` | Another discovered host keeps or takes `<d>`, or cannot release it because its own move is blocked | Follow the `reason` of `<h>` to the host that cannot move and resolve that; the dependent moves then land in one reconcile |
| Skipped: `the stored host's file has keys this gpm does not recognise` | The host's file was written by a newer gpm; the store refuses automatic rewrites, so the host keeps its domains and nothing may take them | Upgrade again, or rewrite the host through the API ([Rollback](upgrading.md#rollback)) |
| Host disabled with "profile ... is not defined" | The profile was renamed or retired; an unresolvable profile fails closed | Re-add the profile, or fix the label/annotation |
| `502` after a container is recreated | The container IP changed between reconciles | Wait for the next event-driven reconcile, or use `usePublishedPorts` |
| `lastSuccess` far behind `lastRun` | Every run since then has failed; managed hosts are frozen, not deleted | Read `status.error` ([Expose a Kubernetes Ingress](../how-to/expose-k8s-ingress.md)) |
| `caFile ... contains no usable PEM certificate` | The `ca.crt` jsonpath escaped the separator and wrote an empty file | Use `{.data.ca\.crt}`, escaping only the dot inside the key |
| The first reconcile writes nothing and reports an error | The template names a certificate, middleware or access list that does not exist | Create those objects, then enable discovery |

## Access-list remote sources

| Symptom | Cause | Fix |
|---|---|---|
| `refused > 0` in the source status | A feed answered non-`200`, was empty, over `maxEntries` or `maxCoverage`, or held one unparseable line | Fix the feed; the previously fetched set keeps serving ([Remote IP feeds](../how-to/access-list-sources.md)) |
| A source rule matches nothing | The source has never been fetched, so it resolves to the empty set | Trigger a reconcile and check the status |
| Needs attention: `deny rule source "<name>" has never been fetched successfully` (or `expired at`, `... resolves to no networks`, `... over its maxCoverage budget`) | The data plane resolves the deny rule's source to the empty set, so it denies no one; the addresses it should block reach the later rules and `defaultAction` | Turn on `settings.accessListSync`, fix the feed and trigger a reconcile; the warning clears once a fetch succeeds ([Unfetched sources](../reference/config/access-list.md#access-list-sources-unfetched)) |
| Source status shows `expired: true` and its rules match nothing | The source's `maxAge` passed without a successful fetch, so its stored set was emptied | Fix the feed; the next successful fetch restores the set ([Source expiry](../reference/config/access-list.md#access-list-sources-expiry)) |
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

## Analytics

| Symptom | Cause | Fix |
|---|---|---|
| The Analytics page says it is off | `settings.analytics.enabled` is false (the default) | Switch it on under Settings, General, **Analytics**; the page fills in from that moment ([Settings: analytics](../reference/config/settings/analytics.md)) |
| The `7d` or `30d` range is missing from the Range select | Retention does not cover it, or `analytics.db` could not be opened | Set `analytics.retentionDays` to 7 or 30 and check the start log for `rollup store unavailable` (path from `GPM_ANALYTICS_DB`, directory must be writable) |
| Every country is `--` | No GeoIP database is loaded, or the traffic is from private addresses | Set `GPM_GEOIP_DB` ([Environment and flags](../reference/env-vars-and-flags.md)); switch the Source filter to `External only` to leave LAN clients out |
| LAN traffic dominates the numbers | Clients on the LAN reach the proxy directly (split DNS, `lanDirect`) | Set the Source filter to `External only`; add VPN or overlay ranges to `analytics.internalNetworks` so they count as internal too |
| A `dropped` chip shows on the page | The collector queue overflowed; the counts undercount by that many | A burst that outran one worker. The chip is informational; if it keeps growing, raise the issue with the traffic profile |
| Numbers differ between the 24h view and the 7d view for the same day | 24h is served from minute counters in memory, 7d from hourly rollups: the running hour and the clients list have different resolution and caps | Expected; see [Where the numbers come from](../reference/analytics.md#where-the-numbers-come-from) |
| The Access Logs page has no **Country** column | Country needs a GeoIP database | Same `GPM_GEOIP_DB` setting; the column, the `country` filter and the CSV column appear once it loads |
