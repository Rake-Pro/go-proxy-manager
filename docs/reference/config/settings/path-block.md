# Settings: path block list

Answer scanner paths (`/.env`, `/wp-login.php`, `/.git/config`) at the edge, with a `404` by default, before an access list, authentication or the upstream sees them. Off until you set it.

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| <span id="settings-path-block"></span> `pathBlock` | PathBlockMiddleware | unset | Optional. Unset blocks nothing. Applies to every proxy host that has not set `pathBlock: off`. |

`pathBlock` has exactly the shape and validation of a `type: path-block` middleware. The fields:

| Key | Type | Default | Required | Description |
|-----|------|---------|----------|-------------|
| <span id="settings-path-block-extensions"></span> `extensions` | []string | none | one of `extensions`, `names`, `globs` | File extensions with the leading dot (`.php`, `.env`, `.tar.gz`). Matches when any path segment ends with one, so `/a.php/extra` is blocked too, and when a segment is the extension alone (`/.env`). At most 256 entries. |
| <span id="settings-path-block-names"></span> `names` | []string | none | one of `extensions`, `names`, `globs` | Whole path segments (`.git`, `wp-admin`, `wp-login.php`). Matches when any segment equals one, at any depth. At most 256 entries. |
| <span id="settings-path-block-globs"></span> `globs` | []string | none | one of `extensions`, `names`, `globs` | `path.Match` patterns over the whole path, each starting with `/`, compared lower-cased, both as the path is and segment-folded without a trailing slash (`/server-status` also blocks `/server-status/`). A pattern that ends with `/` (other than `/` itself), has an empty segment, a segment ending in a dot or a space, or a control character is refused. `*` and `?` never match `/`; a `[...]` class must be in lower case; there is no `**`. At most 64 entries, 128 bytes each, 2048 bytes in total. |
| <span id="settings-path-block-allow-from"></span> `allowFrom` | []string | none | no | Clients the list does not apply to: CIDRs, bare IPs and `@<access-list>` references. `@name` exempts a client only when the first rule of access list `name` that matches it is an allow rule (`cidr` or fetched `source` set) without `paths`: a client a `deny` rule above that allow rule matches stays blocked, and a path-scoped rule, the `defaultAction` and the `geo` rules exempt nobody. A `disabled` list exempts nobody. The list must exist: a settings write naming a missing list is always refused, and deleting a list this key names is refused too unless the stored state already fails the check (then it is accepted with a warning so it can be repaired). |
| <span id="settings-path-block-deny-status"></span> `denyStatus` | int | `404` | no | Status of a blocked request: `400` to `599` when set, unset or `0` means `404`. |
| <span id="settings-path-block-deny-with"></span> `denyWith` | string | `error-page` | no | `error-page` renders the host's custom error page for the status when one exists; `plain` always writes the bare status text. |

Matching rules, limits (entries must be valid UTF-8; with globs, a path over 4096 bytes counts as listed), what the filter does not catch and the full field semantics are on
[PathBlockMiddleware](../middleware.md#pathblockmiddleware-pathblock).

## Which hosts it covers

- **Covered:** every proxy host, including discovery-managed hosts, unless the host sets [`pathBlock: off`](../proxy-host.md#proxy-host-path-block).
- **Not covered:** the synthesized admin edge host (`settings.adminEdge`). A `type: path-block` middleware named in `adminEdge.middlewares` does apply to it.
- **Covered:** a hand-written proxy host that fronts the admin panel ([Reach the admin UI through gpm itself](../../../how-to/admin-ui-behind-gpm.md)), like any other proxy host. A list that matches the panel's own paths can block it, so check the list against the panel first.
- **Not covered:** redirect hosts, parked hosts, stream hosts and the admin listener, which are not served by the proxy-host chain.
- **ACME HTTP-01:** gpm's own in-flight challenge tokens are answered before any host chain, so a list never affects them. Any other challenge path falls through to normal routing, so an upstream that runs its own ACME client is blocked if a list matches `/.well-known/acme-challenge/...`. Keep `.well-known` off the list.

## Per-host switch

- `pathBlock: inherit` (the default, also when unset) applies the fleet list.
- `pathBlock: off` takes that host out of the fleet list only. A `type: path-block` middleware the host or one of its locations references always applies.
- On a discovery-managed host, set the switch on the discovery `template` or profile ([Ingress discovery](ingress-discovery.md#settings-ingress-discovery-template-path-block)); a hand edit is rebuilt away at the next reconcile.

## Behaviour

- **Position.** The fleet list runs outermost among the path-block steps, just inside the rate limit and ahead of access lists, the bouncer and authentication. See [Request pipeline](../../../concepts/request-pipeline.md#how-the-chain-is-compiled).
- **Denial reason.** Blocked requests are counted as `path-block` in `gpm_denials_total`, in analytics and in the access log.
- **Scope of the filter.** It matches path shape only: not content, and not a substitute for hardening the upstream.
- **Check each host first.** An application written in PHP needs `pathBlock: off` or a list without `.php`. Keep `.well-known` off the `extensions`, `names` and `globs` lists.
- **Open connections.** A change to `settings.pathBlock` closes the open WebSocket and other upgraded connections of every host it covers at the reload, and so does a change to a host's `pathBlock` switch. See [Request pipeline](../../../concepts/request-pipeline.md#path-composition).
- **A change** applies at the next router build, like the other settings-level defaults.

```yaml
# config/settings.yaml
pathBlock:
  extensions: [".php", ".php5", ".phtml", ".asp", ".aspx", ".jsp", ".cgi", ".env", ".sql", ".bak", ".old", ".orig", ".swp", ".tar.gz", ".tgz"]
  names: [".git", ".svn", ".hg", ".env", ".aws", ".ssh", ".DS_Store", ".htaccess", ".htpasswd", "wp-admin", "wp-includes", "wp-content", "wp-login.php", "xmlrpc.php", "phpmyadmin", "cgi-bin"]
  globs: ["/server-status", "/server-info"]
  allowFrom: [192.0.2.0/24]   # for example the office network
  denyStatus: 404
  denyWith: error-page
```

For a walkthrough see [Block scanner paths](../../../how-to/block-scanner-paths.md).
