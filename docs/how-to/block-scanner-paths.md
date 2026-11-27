# Block scanner paths

Answer the probes every public host receives (`/.env`, `/wp-login.php`, `/.git/config`) at the edge, with a `404` by default, so they never reach an access list, an identity provider or an upstream. It is a path shape filter, not content inspection and not a substitute for hardening the upstream.

## Prerequisites

- An admin session, or a token with the `admin` scope (a settings write needs it). Writing only middlewares and hosts needs `middlewares:write` and `proxy-hosts:write`.
- A list of what each proxy host serves. A listed path that an application uses on purpose returns `404` for real users too: an application written in PHP needs `pathBlock: off` or a list without `.php`.
- `.well-known` stays off the `extensions`, `names` and `globs` lists (certificate and discovery clients use it).
- For an `@list` exemption: an existing [access list](../reference/config/access-list.md).

## Steps

1. **Set the fleet list** in `config/settings.yaml`. This is a starting list, not enabled by default:

   ```yaml
   pathBlock:
     extensions: [".php", ".php5", ".phtml", ".asp", ".aspx", ".jsp", ".cgi", ".env", ".sql", ".bak", ".old", ".orig", ".swp", ".tar.gz", ".tgz"]
     names: [".git", ".svn", ".hg", ".env", ".aws", ".ssh", ".DS_Store", ".htaccess", ".htpasswd", "wp-admin", "wp-includes", "wp-content", "wp-login.php", "xmlrpc.php", "phpmyadmin", "cgi-bin"]
     globs: ["/server-status", "/server-info"]
   ```

   It now covers every proxy host except the synthesized admin edge host. Reference: [Path block list](../reference/config/settings/path-block.md).

   A file edit takes effect the way any config file edit does: the daemon loads it on start or reload ([Configuration model](../concepts/config-model.md)). The same change can be made in the UI (Settings, Path block list) or with `PUT /api/settings`.

2. **Opt a host out** when it serves listed paths on purpose.

   A hand-written host:

   ```yaml
   # config/proxy-hosts/blog.yaml
   pathBlock: off
   ```

   A discovery-managed host: add `pathBlock: off` to the existing template or profile, because a hand edit of a derived host is rebuilt away at the next reconcile. A profile is a complete chain that replaces the template, so never write a profile that holds only this key.

   ```yaml
   # config/settings.yaml (a fragment: add the key to a profile you already have)
   ingressDiscovery:
     profiles:
       php-app:
         # ...the profile's other keys (upstream, tls.certificateRef, ...)
         pathBlock: off
   ```

   The same key exists on `ingressDiscovery.template` and on `dockerDiscovery.template` and `dockerDiscovery.profiles`. When `dockerDiscovery.profiles` is empty the Docker reconciler uses `ingressDiscovery.profiles`, so `pathBlock: off` on an Ingress profile then also applies to Docker-derived hosts.

3. **Add a per-host or per-location list** with a middleware. It applies whether or not the host opted out of the fleet list:

   ```yaml
   # config/middlewares/admin-paths.yaml
   name: admin-paths
   type: path-block
   pathBlock:
     names: ["admin", "console"]
     globs: ["/internal/*"]
   ```

   `/internal/*` covers `/internal/` and one level below it only: not `/internal` itself and not deeper levels. To block a directory at any depth use a `names` entry instead; see the [matching rules](../reference/config/middleware.md#pathblockmiddleware-pathblock).

   Reference it from a host or a location:

   ```yaml
   middlewares: [admin-paths]
   ```

4. **Exempt a network** with `allowFrom`: CIDRs, bare IPs, or `@<access-list>`. The exemption compares the derived client IP, so declare any L7 proxy in front of gpm in `settings.trustedProxies`, or in the host's own `trustedProxies`, which replaces it for that host.

   ```yaml
   # the settings pathBlock block, or a path-block middleware's pathBlock
   pathBlock:
     extensions: [".php"]
     allowFrom: [192.0.2.0/24, "@office"]
   ```

   `@office` exempts a client only when the first rule of the `office` list that matches it is an allow rule without `paths`.

## Verify

Replace `app.example.com` with a covered host and `legacy.example.com` with one that set `pathBlock: off`.

| Check | Expected |
|---|---|
| `curl -s -o /dev/null -w '%{http_code}\n' https://app.example.com/.env` | `404` |
| `curl -s -o /dev/null -w '%{http_code}\n' https://app.example.com/index.php` | `404` |
| `curl -s -o /dev/null -w '%{http_code}\n' "https://app.example.com/.GIT/config"` | `404` (matching is case-insensitive) |
| `curl -s -o /dev/null -w '%{http_code}\n' https://app.example.com/` | The application's normal status |
| `curl -s -o /dev/null -w '%{http_code}\n' https://legacy.example.com/index.php` | The application's own status (host has `pathBlock: off`) |
| `curl -s -o /dev/null -w '%{http_code}\n' https://app.example.com/.env` from an exempt network | The application's own status |

Blocked requests are counted in `gpm_denials_total{reason="path-block"}` and carry `deny: path-block` in the access log, which records only while access logging is on (`-access-log` or `GPM_ACCESS_LOG=1` at start, or **Enable capture** on the Access Logs page). A glob also blocks the trailing-slash form (`/server-status/`); see [what the filter does not catch](../reference/config/middleware.md#path-block-not-caught). The matching rules are on [PathBlockMiddleware](../reference/config/middleware.md#pathblockmiddleware-pathblock).

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| An application's own pages return `404` after enabling | A list entry matches a path the application serves (`.php` on a PHP application, `wp-admin` on WordPress) | Set `pathBlock: off` on the host, or narrow the list |
| Sign-in loops or fails | An entry matches the auth gate's own path (the `auth-request` outpost prefix or `/__gpm/oidc/callback`); the path block runs outside authentication | Remove the matching entry, or set `pathBlock: off` on the host |
| `pathBlock: off` on a discovery-managed host disappears | The host is rebuilt from the template or profile at every reconcile | Set `pathBlock: off` on the discovery template or profile |
| A request with a very long path returns `404` on a host with globs | On a route covered by a list that has `globs`, a path over 4096 bytes counts as listed | Expected; shorten the path, exempt the client with `allowFrom`, or move the globs to a middleware on the hosts that need them |
| An `@list` exemption does not apply | The first rule of that list matching the client is a `deny` rule, or the client matches only a path-scoped rule (an `@list` exemption ignores path-scoped rules), the `defaultAction` or a `geo` rule, none of which exempt anyone; or the list does not exist | Put an allow rule for the client's network first in the list, without `paths`, or use a CIDR in `allowFrom` |
| Settings write refused: `pathBlock.allowFrom: references unknown accessList` | The `@name` list does not exist | Create the list first, or remove the reference |
| A scanner still reaches the upstream | The host opted out, the path shape is not listed, the spelling is one the filter does not catch ([what it does not catch](../reference/config/middleware.md#path-block-not-caught)), or the request went through a redirect, parked or stream host, which the list does not cover | Add the entry, or remove `pathBlock: off`; the list covers proxy hosts only |
| `.well-known` requests are blocked | An entry in `extensions`, `names` or `globs` matches it (for example `.txt` blocks `/.well-known/security.txt`) | Remove the entry or narrow it; keep `.well-known` off all three lists. An upstream that runs its own ACME client is blocked the same way |
| Write refused: `globs[0]: ... has an upper-case letter inside [...]` | A `[...]` class holds an upper-case letter, and matching is case-insensitive | Write the class in lower case (`[a-z]`) |
| Write refused: `globs[0]: "/old/" must not end with "/" (a path is compared without its trailing slash; write "/old")` | A glob ends with `/`; a path is compared without its trailing slash, so `/old` already matches `/old/` | Drop the trailing slash |
| Write refused: `globs[0]: ... must not contain an empty segment ("//")`, `... has a segment ending with a dot or a space, which a path is compared without` or `... must not contain a control character` | The glob could never match the form a path is compared in | Remove the empty segment, the trailing dot or space, or the control character |
| Write refused: `globs: are more than 2048 bytes in total` | All globs of one list together exceed the cap | Shorten the globs, or move some to a `names` entry |
| Write refused: `extensions[0]: "php" must be a dot followed by the extension (e.g. ".php")` | An extension was written without its leading dot | Add the dot (`.php`) |
| Blocked requests still count toward rate limiting | By design: the block runs inside the rate limit, and a hit is not a ban | Tune the host's rate limit ([RateLimitMiddleware](../reference/config/middleware.md#ratelimitmiddleware-ratelimit)) |
