# Configuration reference

One page per object kind, plus the singleton settings. Each page opens with the
object's field table and closes with a complete YAML example.

For how these files are stored, named and validated, see
[Configuration model](../../concepts/config-model.md).

## Object kinds

| Kind | Directory | Page |
|---|---|---|
| ProxyHost | `config/proxy-hosts/` | [proxy-host.md](proxy-host.md) |
| RedirectHost | `config/redirect-hosts/` | [redirect-host.md](redirect-host.md) |
| StreamHost | `config/stream-hosts/` | [stream-host.md](stream-host.md) |
| ParkedHost | `config/parked-hosts/` | [parked-host.md](parked-host.md) |
| UpstreamGroup | `config/upstream-groups/` | [upstream-group.md](upstream-group.md) |
| Certificate | `config/certificates/` | [certificate.md](certificate.md) |
| ClientCA | `config/client-cas/` | [client-ca.md](client-ca.md) |
| DNSProvider | `config/dns-providers/` | [dns-provider.md](dns-provider.md) |
| IdentityProvider | `config/identity-providers/` | [identity-provider.md](identity-provider.md) |
| AccessList | `config/access-lists/` | [access-list.md](access-list.md) |
| Middleware | `config/middlewares/` | [middleware.md](middleware.md) |
| APIToken | `config/api-tokens/` | [api-token.md](api-token.md) |
| Settings | `config/settings.yaml` | [settings/](settings/index.md) |

## Anchor convention

Every **key** row in a field table on these pages carries an explicit HTML
anchor, so the in-app help registry and external documentation can deep-link to
a single key and keep working across edits. Value tables - the ones whose first
column is an enum member, a status, a mode or a provider name rather than a
config key - carry none; they are keyed by the row above them, not by a key
path.

The anchor id is `<kind>-<key>`:

- `<kind>` is the object's directory name in singular form, exactly as in the
  table above: `proxy-host`, `redirect-host`, `stream-host`, `parked-host`,
  `upstream-group`, `certificate`, `client-ca`, `dns-provider`,
  `identity-provider`, `access-list`, `middleware`, `api-token`, `settings`.
- `<key>` is the **full** config key path - not just the table's first column,
  but the path from the object's root, so a nested table's rows carry their
  parent path (`locations[].path`, `tls.clientAuth.caRef`). It is lowercased,
  `[]` and `.` become `-`, camelCase boundaries are hyphenated, and every other
  run of non-alphanumeric characters collapses to one `-`.

Worked examples:

| Page | Key column | Full key path | Anchor |
|---|---|---|---|
| `proxy-host.md` | `tls.hsts.preload` | `tls.hsts.preload` | `#proxy-host-tls-hsts-preload` |
| `proxy-host.md` | `upstreamGroupRef` | `upstreamGroupRef` | `#proxy-host-upstream-group-ref` |
| `proxy-host.md` | `stripPrefix` (Location table) | `locations[].stripPrefix` | `#proxy-host-locations-strip-prefix` |
| `middleware.md` | `passwordHash` (BasicAuthSpec table) | `auth.basic.users[].passwordHash` | `#middleware-auth-basic-users-password-hash` |
| `settings/dns-sync.md` | `cloudflare.zoneName` | `dnsSync.cloudflare.zoneName` | `#settings-dns-sync-cloudflare-zone-name` |
| `settings/proxy-protocol.md` | `trustedCIDRs` | `proxyProtocol.trustedCIDRs` | `#settings-proxy-protocol-trusted-cidrs` |

**A settings sub-page's key column is relative to its sub-block; the anchor is
not.** Every settings page owns one block of `config/settings.yaml`, and the
anchor always carries that block, so `enabled` on four different pages produces
four distinct ids. This is what the `<kind>` being `settings` for all fourteen
pages requires: the id must be unique across the whole `settings` kind, not just
within a page.

**Acronyms stay whole.** The camelCase split fires on a lowercase-or-digit
followed by an uppercase only, so `trustedCIDRs` is `trusted-cidrs`, `apiURL` is
`api-url` and `tlsCA` is `tls-ca` - never `trusted-cid-rs`.

Rules for anyone editing these pages:

- **Never renumber or reword an existing anchor.** It is a public identifier.
  If a key is renamed, add the new anchor and keep the old one as an empty
  `<span>` on the same row until the next major version.
- **One anchor per row.** A duplicate id in one page is a defect; the id is only
  required to be unique within its page. Where a table deliberately repeats keys
  documented above it (the inline `auth` / `rateLimit` table on
  [proxy-host.md](proxy-host.md)), the repeat carries no anchor: the first table
  owns the key.
- The anchor lives in the first table cell, before the key, as
  `<span id="proxy-host-tls-hsts-preload"></span>`.
- Headings may carry the same style of id with `attr_list`
  (`## Scopes { #api-token-scopes }`); both `attr_list` and `md_in_html` are
  enabled in `mkdocs.yml`.
