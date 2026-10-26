# Changelog

All notable changes to go-proxy-manager are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Analytics view (sidebar, Overview group) and `GET /api/analytics`: traffic
  through the edge for a range, fleet-wide or per proxy host - request volume,
  denied and threat-signature counts, countries, denial reasons, hosts and top
  clients, drawn as plain HTML and inline SVG. The API answers 501 until a
  collector is wired; `GPM_ANALYTICS_DEMO=1` serves seed data for review.
- Access Logs: a Country column, `country:` filter term and CSV column, filled
  from the GeoIP database when one is loaded (`GPM_GEOIP_DB`); the JSON log
  line and `GET /api/logs` entries gain the same `country` field.
- Hover-to-copy on Access Logs and Analytics rows: host, path, client address,
  country code and signature label each get a copy button on row hover or
  keyboard focus; values with control characters are copied percent-encoded.
- Access Logs: the in-memory buffer is sizeable. `-access-log-buffer` /
  `GPM_ACCESS_LOG_BUFFER` sets the startup size (default `5000`, was a fixed
  `1000`), and the Buffer selector on the Access Logs page (or `PUT /api/logs`
  with `{"capacity": n}`) resizes it live between 100 and 100000, keeping
  the newest entries. Runtime-only, like the capture toggle.
- Rate limit `blockBackoff` and `blockMax`: a client that trips the limit
  again after its block ended is blocked `blockBackoff` times longer each
  round (`2` doubles it), capped at `blockMax` (default `24h`). The streak is
  forgotten after one block length of good behaviour. Available on
  `type: rate-limit` middlewares and inline host/location `rateLimit` blocks,
  with the matching fields in the UI form.
- Rate limit `allowFrom` accepts `@<access-list>`: the client networks that
  list allows (allow `cidr` rules plus fetched `source` sets, path-scoped
  rules excluded) bypass the limit, so the list that gates a host also exempts
  it. Validation requires the list to exist.

- List filters take alternatives: commas inside a scoped term
  (`status:4xx,5xx`, `host:a.example,b.example`) make the term hold when any
  of them does; a quoted value keeps a literal comma.
- Access Logs: the `client` field matches by address. A CIDR
  (`client:10.0.0.0/8`, `client:2001:db8::/32`) takes every client inside the
  network, a bare IP matches exactly (an IPv4-mapped IPv6 client counts as
  IPv4), and `@name` stands for the networks an access list allows (its
  `cidr` rules plus what its named sources last fetched), so
  `-client:@home-only` shows only what reached the proxy from outside your
  own networks and `-client:@uptimerobot` hides a monitor fed by its
  published IP list. The `client:` suggestions offer the `@` lists first.
- `GET /api/access-list-networks` (scope `access-lists:read`) returns the
  networks each access list allows, by name, resolved the same way.
- Saved filters: every list's filter box has a **Saved** menu that applies a
  named filter, saves the current one under a name, or deletes one. They are
  kept in settings (`savedFilters`, new routes `GET`/`PUT
  /api/settings/saved-filters`, scope `settings:read`/`settings:write`), so
  they follow you across browsers and show in History. The last filter typed
  on each list also comes back when you return to it (per browser).
- Access Logs: **Export** downloads the rows the filter shows as CSV or
  JSON, in the table's order, with spreadsheet-formula cells neutralised.

### Security

The open items of the 2026-10-18 review (issues #49, #50, #51), all shipped:

- Writes to `/api/dns-providers` and `/api/identity-providers` take the
  `admin` scope; a bouncer middleware whose `apiKey` is a placeholder is
  created, or has its `url`/`apiKey` changed, by `admin` only (403 otherwise).
  The bouncer client refuses link-local destinations like the other outbound
  clients. (#49)
- A secret is a placeholder only when it is nothing but placeholders; mixed
  values are literals and refused. `GET /settings` redacts webhook and
  notification URLs below `admin`. The SSO session cookie is bound to the
  provider that minted it, and a host with more than one OIDC gate is warned
  about. A client-cert gate refuses a certificate verified under another
  host's SNI (so on a host without its own `tls.clientAuth`, which is warned
  about, it admits only its allowFrom exemptions) and re-checks revocation per
  request, as does the host's own mTLS gate. Rate-limit buckets and the basic-auth lockout key IPv6 by /64.
  Discovery keeps an existing managed host's current domain ahead of any
  claimant instead of failing the batch. (#50)
- Explicit discovery `tokenFile`/`caFile` paths are confined to the secret
  file roots. Webhooks are signed (`X-GPM-Webhook-Signature`,
  `X-GPM-Webhook-Timestamp`) instead of carrying the secret. The `Host`
  header's port is validated; request-override headers (`X-Original-URL`,
  `X-HTTP-Method-Override`, ...) are stripped from every peer and the
  client-address headers (`True-Client-IP`, `CF-Connecting-IP`, `Forwarded`,
  ...) unless the peer is one of the host's trusted proxies; a request body
  may stall at most
  60 s between reads; the sticky
  cookie carries an opaque tag instead of the upstream label; a revert to a
  commit holding a literal secret is refused; `POST /api/middlewares/{name}/revert`
  is admin-only; the Kubernetes, DNS-sync, webhook, notification and ACME
  DNS-01 REST-provider clients ignore proxy environment variables, and every
  link-local guard refuses zoned addresses; the release workflow only
  runs from `main` and only builds a tag that sits on `main`, and the docs
  workflow grants Pages permissions per job. (#51)

## [2.5.0] - 2026-10-20

### Added

- Structured filters on every list (Proxy Hosts, Certificates, the other
  object lists, API Tokens, Access Logs, History): the filter box takes
  field-scoped terms such as `tag:lan`, `status:disabled`, `cert:wildcard`,
  `source:ingress-discovery` or `status:5xx`, with a leading `-` to exclude
  and quotes for a value with spaces. Typing suggests the field names, then
  the values present in the list; a complete term becomes a chip, and the
  matched text is highlighted in the rows. Plain words still match
  everything, and the term chips compose with the domain-group chips.
- Every table sorts by its columns: the object lists, API Tokens, Access
  Logs, the discovery and access-list source status tables and the issued
  client certificates, besides Proxy Hosts (now also by certificate,
  sign-in, status and tags) and Certificates (now also by type and issuer).
  The choice is remembered per table in the browser.
- Access Logs: **Clear** empties the buffer (`DELETE /api/logs`, admin
  scope), an auto-refresh interval (off, 2 s, 5 s, 15 s, 1 min) reloads the
  rows in place while keeping the filter, the sort and the scroll position,
  a Bytes column, a count line (shown of total, buffer capacity), and the
  filter understands `method:`, `host:`, `path:`, `status:` (a code or a
  class such as `4xx`), `client:` and `slow:yes`. `GET /api/logs` reports
  the buffer `capacity`.
- Proxy Hosts bulk bar: **Maintenance on** / **Maintenance off** beside
  Enable / Disable, one commit per host (#52).
- `gpm_maintenance_responses_total{host}`: counts the 503 a host in
  maintenance answers with, so an alert can notice a window nobody closed
  (#52).

### Changed

- The always-visible explanatory paragraphs at the top of pages and cards are
  gone (Integrations tabs, Settings cards, host editor sections, the Error
  Pages and Runtime pages, the middleware list, the Access Logs capture
  banner). The `?` next to each control and the documentation carry the same
  text; the page keeps only its one-line subtitle.
- Hovering a Tags cell (Proxy Hosts and the other object lists) shows every
  tag; the middleware check-lists in the host editor and the wizard say
  "sign-in" for an `auth` middleware, as the rail does (#57).
- The three font files no stylesheet declared (Inter, Space Grotesk,
  JetBrains Mono) are gone from the embedded bundle, about 100 KB off the
  binary; a test now refuses an embedded face nothing names (#57).

- Proxy Hosts list: the Source column is gone. A discovered host's source
  (`k8s-managed`, `docker-managed`) is the first chip in the Tags column,
  which is always shown, and a display or object name that differs from the
  domain sits beside the domain.

### Security

- Guard `queryEquals` triggers fail closed on a repeated query key:
  `?direct=0&direct=1` counts as matching the trigger instead of being judged
  on the first value only (#50).
- `GET /version` requires a signed-in session or an API token (any role), so
  the build commit, date and Go version are no longer visible to anonymous
  clients; `/healthz` stays open (#51).
- Restoring a backup archive keeps the live `api-tokens/*` and ignores the
  archive's, matching Revert; the ignored files are logged (#51).
- Local login is also locked per username: twenty failures against one
  account within 15 minutes, from any clients, lock it for the window (the
  per-IP limit stays five), so a distributed guess is bounded; a locked
  username and a locked IP get the identical response (#51).

### Fixed

- Saving an object without changing anything no longer writes a file, commits
  or restamps `updatedAt`; the API answers with an empty `X-Config-Commit` and
  the UI says "No changes" (#58).
- Access Logs: with nothing captured the page no longer shows "No request
  matches this filter".
- mTLS: the 421 that refuses a request whose connection never presented the
  client certificate now closes that connection (GOAWAY on HTTP/2), so the
  browser re-handshakes under the host's own SNI and is asked for the
  certificate. Before, a connection kept busy from before the host required a
  certificate, or one coalesced from a non-mTLS host under the same wildcard
  certificate, answered "Misdirected Request" until it went idle.

## [2.4.0] - 2026-10-20

### Added

- History: **Changes** on a commit shows every file it touched as a unified
  diff with a per-file summary, and **Compare** on two commits shows what
  changed between them, oldest to newest. Backed by `GET /api/history/diff`
  (admin scope: the patch carries whole object files).
- API tokens: **Edit** on a token changes its scopes and expiry in place. The
  secret in use keeps working; Rotate is still the only way to mint a new one.

- Admin front door (`settings.adminEdge`, Settings > General): gpm serves its
  own panel at the edge on `externalBaseURL`'s hostname, with the access
  lists, middlewares, TLS and DNS policy of a proxy host but without a proxy
  host object. It is never listed, disabled, deleted or discovery-managed; the
  name `gpm-admin` is reserved and a stored host may not claim the hostname
  while it is on (the write is refused, either side). Replaces the hand-written
  proxy host route, which remains supported.
- Fleet-wide maintenance leaves the admin front door and the hosts the admin
  sign-in providers live behind up, so the panel stays reachable to turn it
  off; the Settings page lists what stays up and `GET /api/capabilities`
  reports it under `maintenance.exempt`. A per-host maintenance flag on one of
  them is still honoured. Fixes #9.
- Integrations is tabbed like Settings: DNS sync, Ingress discovery, Docker
  discovery, Access-list sync, Webhooks and Notifications, each deep-linkable
  as `#/integrations/<tab>`, any tab's Save writing the whole settings object.

### Fixed

- Admin UI on a phone: the tab strip no longer shows its own scrollbar under
  the tabs, and the hover underline no longer sticks to the last tapped tab.
- README: the Go badge now reads `.go-version`, the toolchain the binary and
  image are built with, instead of the language minimum in `go.mod`.
- Docs: the upgrade table dated its 1.0.0 rows "Unreleased"; the UI tour
  describes the Proxy Hosts columns and API token Edit; the flags reference no
  longer claims `-version` has an environment variable.

## [2.3.0] - 2026-10-20

### Added

- Ingress and Docker discovery templates and profiles take `locations`, the
  same path-scoped overrides a hand-written proxy host has, stamped onto every
  derived host and validated by the same rules at settings-write time. They
  live on the operator's template or profile only; an Ingress annotation still
  carries a profile name and nothing else. Meant for a profile written for one
  service whose upstream is not the cluster ingress controller (a hostPort pod
  serving a sidecar under a path).

### Changed

- Admin UI list pages for identity providers, middleware, access lists,
  client CAs, DNS providers, upstream groups, streams, redirects and parked
  hosts are tables, as Proxy Hosts and Certificates already were: one row per
  object, one column per summary field, long values wrapped instead of cut
  short. The Add button sits on the filter row on every list.
- Admin UI Identity Providers list no longer shows the OIDC client ID; it is
  in the provider's editor only.
- Admin UI tables size their cells to the window: every fixed pixel cap on a
  name, upstream, issuer, path or list value is now a viewport-relative clamp,
  so a wide screen shows the full text instead of clipping it beside empty
  space, and a phone still gets the old floor.
- Admin UI lists are one line per row, everywhere. Proxy Hosts' second line
  (kind chip, source path, object name, tags) became two columns: Source,
  with the k8s/docker chip and its namespace/name, or the display or object
  name when it differs from the domain; and Tags, present only when some host
  has tags. Certificates and the generic object lists show a display or
  object name inline, muted, only when it differs from the name. The
  certificate Type cell is one line and the issuer column is no longer
  clipped at 200px.
- Admin UI frame: the masthead and sidebar stay put and only the main column
  scrolls, so the scrollbar starts under the masthead instead of running up
  its edge; the tab strip no longer shows a stray scrollbar thumb.

### Fixed

- API responses omit `createdAt`/`updatedAt` when the store never stamped
  them, instead of sending the zero time (shown as year 0 in the lists).

- Admin UI sidebar footer no longer repeats the commit hash on a dev build
  ("dev-abc1234 (abc1234)"), and the config-commit link next to it is never
  truncated.

## [2.2.0] - 2026-10-19

### Changed

- Admin UI visual refresh ("drift"): the sidebar is PatternFly's dark rail
  under the masthead in both themes; the masthead, dialog and wizard title
  bands drift from black into navy and end in a brand-to-cyan hairline; the
  logo sits in a brand tile; the canvas is a cool grey; every shadow is cast
  in the palette's blue (a hairline ring plus a faint glow in dark mode), so
  cards keep an edge on both canvases.
- Admin UI buttons are one filled system: solid blue primary, tonal (tinted,
  borderless) secondary, solid red only for a dialog's confirm, tonal red for
  the page-level Delete that opens it. Chips, segmented controls, help and
  icon buttons, step and chain numbers, bordered sub-blocks and the History
  timeline dots are filled rather than outlined. Text inputs keep their box.
- Every object editor carries Clone and Delete (and Renew now on an ACME
  certificate) in the title band, as the proxy host editor already did; save
  bars put the primary first, as dialogs do. Settings tabs sit in the title
  band like the host editor's. The bulk-action bar is a white bar with the
  brand edge so its buttons show.
- Sign-in and verification-code pages open with the app's masthead strip.
- The "managed" chip on a discovered host reads "k8s-managed" or
  "docker-managed" instead of naming the reconciler.
- Dark mode: inline alerts, attention rows and banners keep a translucent
  tint of their severity colour instead of a neutral grey fill.
- List cards (access lists, middleware, upstream groups, identity providers,
  DNS providers, client CAs, redirects, parked hosts, streams) say they open:
  a kind tile with the section's glyph and a round "go" mark at rest; on
  hover or keyboard focus the card lifts, takes the brand ring, the name turns
  brand and the hairline wire lights along its top edge.

- Dev builds report their identity: the `dev-<sha>` image is built with
  `dev-<sha>` as its version plus the commit and build date, so `/version` and
  the admin shell name the build instead of "dev" and "unknown".

### Fixed

- Admin UI: on a phone the theme button shows only its glyph, so the instance
  name no longer truncates in the masthead; the open navigation drawer in dark
  mode has a visible edge.

## [2.1.0] - 2026-10-19

### Changed

- Admin UI: the dot that marked a host editor tab holding a non-default
  value is gone; it read as an unread badge. The fold summaries inside each
  tab and the rail carry that information.
- Admin UI consistency round, from an adversarial review of every page:
  - Sign-in page on the app's design and theme; a refused sign-in (wrong
    password, wrong code, lockout, failed SSO callback) re-renders the page
    with the reason and the username kept instead of a plain-text error.
  - Delete left the list cards. Every delete, token rotate and client
    certificate renew asks in the app's own dialog, naming the hosts that
    still use the object; a refused delete is a banner, not a toast.
  - One vocabulary: Upstream (not Backend), Sign-in (not Auth), Sign out,
    Client addresses, Exempt networks, Request chain, commit (not revision).
    Pointers name the real page: Integrations > DNS sync, Error Pages,
    Settings > Operations for fleet-wide maintenance, "Public URL of this
    panel", "Admin sign-in > Single sign-on providers". History and the
    revert dialog use the UI's names for object kinds.
  - Hosts list: the TLS column is now "Certificate", resolved by domain the
    way the handshake does, with the certificate's state (amber inside the
    renewal window, red when expired, "no certificate" when nothing covers
    the domain); the rail says "N of M domains covered". A muted "disabled"
    chip marks disabled objects on every list.
  - Editors: one "Name and status" card on every kind (certificates gain
    display name, tags and disabled); "Add <kind>" on every new object, a
    create opens the object's page, an edit returns to the list; fold
    summaries and "Attached:" lines follow your edits; client-side
    validation flags the field, opens its fold and switches to its tab;
    the Access card is "Request chain" and its order text matches the rail;
    the stream editor greys out access lists with path rules; the host
    editor's Advanced tab is "More".
  - Wizards: the name follows the first domain again after Back; a dirty
    wizard keeps its page when a leave prompt is declined; the setup wizard
    checks for unsaved edits first and hands off to the host wizard only on
    an instance with no host; Force SSL defaults on only when every domain
    is covered and follows the domain list; "Open the full editor instead"
    carries the draft; the Review leaves out the timestamps; wizards and
    editors apply one rule per field (wildcard proxies warn, HSTS needs
    Force SSL).
  - Dialogs: closed on navigation; Enter confirms only from the dialog's own
    controls; natural height on phones; one footer convention; toasts above
    the scrim; focus returns to the page title when the opener is gone.
  - Text: plurals through one helper, subtitles on one line, sentence-case
    chips and buttons, failure toasts named for the action, help popovers
    that add to the inline line instead of repeating it (a test keeps it
    so), the sidebar foot without "(none)".
  - Lists: the Overview counts hosts in maintenance apart and lists them
    under Needs attention; the hosts list sorts by Domain with the header
    marked, zone chips group by the primary domain's zone and hide exactly
    what they count, tags cap at three plus "+N"; Redirects and Parked
    Hosts lead with the domain, status, tags and last change, and Parked
    Hosts moved into the Hosts group; API Tokens shows the list first with
    "Add token" and Enable/Disable per row; upstream group cards show
    health; access log times carry seconds; per-list filter hints.
  - Phone and tablet: below 1000 px the hosts table drops Upstream and
    Updated, below 720 px it shows Domain and Status with no sideways
    scroll; the sidebar becomes a drawer below 800 px, the menu button
    toggles and the page behind it is inert.
  - Keyboard and screen readers: a "Skip to content" link, the title band
    inside the main landmark, the same focus ring on fold summaries and
    tabs, help popovers reachable by Tab with "Learn more" inside, the
    theme button showing its state as text, the masthead name with a
    tooltip.
  - Typography: 15 px base, no thin antialiasing, darker secondary text in
    light mode, secondary text half a step larger, no 11 px text.
- API and model: validation errors lead with the dotted path of the field
  they are about (`proxy host "app": upstream.port: ...`), cross-object
  conflicts name the field of the object being saved, and the store's
  "rejecting save:" wrapper is gone. Scripts that matched the old text need
  updating.

### Fixed

- Admin UI: a location's "(host default)" scheme saved `http` on an https
  host; the option is gone and a new row starts on the host's scheme.
- Admin UI: bulk actions reached hosts the filter had hidden; the selection
  now follows the filter and every bulk dialog lists the targets. Tag
  changes skip discovery-managed hosts and say so.
- Admin UI: a clone kept the source's labels (including the discovery
  ownership label, so the copy was deleted on the next reconcile) and its
  timestamps; stream clones kept the listen port.
- Admin UI: the fleet-wide maintenance banner linked to a control that does
  not exist.

### Security

- Admin UI: creating an object through any editor could silently replace an
  existing object of the same name with a plain upsert; recreating an API
  token raised the existing token's scopes and kept its secret. Every create
  is now create-only (`If-None-Match: *`) and a duplicate name is refused
  with a link to the existing object.
- Sign-in: an identity provider's error code from the SSO callback is logged,
  not echoed into the page, trimmed to the RFC 6749 charset and 64 bytes.
- Local sign-in lockout: the attempt is now counted before the credential is
  checked, so a burst of parallel guesses from one address cannot exceed the
  limit; a gate full of expired entries no longer keeps every new client
  locked out until a restart; IPv6 clients are keyed by /64. The callback
  clears the state cookie and drops the pending login on every failure path.
  The sign-in and TOTP pages send `Cache-Control: no-store`.

## [2.0.1] - 2026-10-19

### Changed

- Admin UI: the proxy host editor is five tabs in the title band (Routing,
  TLS, Access, Headers, Advanced), a dot on every tab that holds a
  non-default value, a rail (status, certificate, request chain in evaluation
  order, config file) and the object actions YAML, Clone and Delete in the
  band. The "Request flow" panel and the two-column card grid are gone; a
  validation error switches to the tab of the field it names.
- Admin UI: Clone left the list rows (proxy hosts, certificates and every
  card list). It lives on the object's page and asks for the copy's name
  first, checked against the names in use, before opening the editor on the
  copy.
- Admin UI: "YAML" on a proxy host shows the config file as the server writes
  it (a dry run of the stored object; nothing is written).

## [2.0.0] - 2026-10-18

### Added

- Admin UI: a first-time setup wizard. A fresh instance (no proxy host, no
  certificate) greets the first admin on the Overview with "Set up this
  instance": instance name, public URL of the panel and the proxies to trust
  for client addresses, a review, then the host wizard. Skippable per browser
  and always reachable from the account menu.
- Admin UI: a creation wizard for proxy hosts. "Add proxy host" opens four
  steps (Service, Certificate, Access, Review); every finished step summarizes
  what was chosen in the step list, the Review step shows the exact YAML the
  server would commit (through the new dry-run route) and "Import from YAML"
  fills the steps from a pasted document (through the new parse route),
  naming any field the wizard does not cover and offering the full editor
  for it. The full editor stays at `#/hosts/_new`. Pure vanilla JS, no new
  dependency.
- API: `PUT /api/{plural}/{name}?dryRun=1` (or `dryRun=true`) runs every check
  a real save runs and returns the YAML it would commit and the file path,
  without writing, committing, reloading or firing a webhook. A failing check
  is the same `400` as the real save, and an API-token dry run never mints a
  secret. The reply also says whether the name is already stored, and
  `If-None-Match: *` on any `PUT` makes it create-only (`409` on a stored
  name), which is what the creation wizard sends.
- API: `POST /api/{plural}/_parse` turns one YAML document
  (`Content-Type: application/yaml`) into that kind's object with the same
  decode rules as a `PUT`, and reports whether it passes validation without
  refusing an incomplete object. Nothing is persisted; the body is capped at
  1 MiB and excessive YAML aliasing is refused.

### Changed

- Admin UI: restyled on the PatternFly 4 design grammar (the system
  Authentik's admin uses): a dark masthead across the top with the instance
  name, a theme button and an account menu (who is signed in, API tokens,
  History, Log out), the sidebar beneath it with text-only groups and the
  version and config commit at its foot, a title band above the content that
  carries each page's title and one summary line (no page shows its title
  twice), flat square cards on a grey canvas, form controls with a darker
  bottom edge that turns primary on focus, outline status labels, underline
  tabs and inline alerts. Dark mode follows the same tokens on neutral greys.
  The primary colour is `#0066cc` (the previous brand blue did not meet AA on
  white). Fonts are now Red Hat Text, Display and Mono (OFL 1.1), vendored
  like before, one variable file per family; the Inter, Space Grotesk and
  JetBrains Mono files are no longer referenced.

### Fixed

- HA followers answer `PUT ...?dryRun=1` and `POST .../_parse` like the leader:
  neither writes, and a warm standby should preview exactly as the leader does.
- Ingress discovery truncates the annotation value it logs when the value is
  not `true` or `false`, as Docker discovery already did.
- Admin UI: on card views (identity providers, middlewares and the other
  generic object lists) the filter box was a fixed 360px wide, a few pixels
  wider than the card column under it. The toolbar now shares the card
  grid's columns, so the box ends exactly where the first card ends at any
  window width.
- Admin UI: text typed into a chip field (domains, allow-from networks,
  trusted proxies, scopes, tags) but not committed with Enter was silently
  left out of the save. It is now committed when the field loses focus and
  included in every save.
- Admin UI: saving a certificate dropped its display name, labels, tags and
  disabled flag, and rotating an API token dropped its display name, labels
  and tags. Both now carry them through. A location's middleware and
  access-list references are also kept when their list failed to load.
- Admin UI: a zone filter remembered from an earlier visit could hide every
  proxy host with no chip left to clear it. The filter now applies only while
  the zone chips are shown, and zones that no longer exist are forgotten.
- Admin UI: issuing or renewing a client certificate, migrating an access
  list's basic auth and generating a client CA no longer throw away unsaved
  edits on the page without asking.
- Admin UI: navigating quickly between pages could leave the previous page's
  content, or its "Reference list unavailable" banner, on the new page.
- Admin UI: clicking a field's label opened its help popover instead of
  focusing the field. Most field labels are now tied to their controls
  (screen readers announce them), and the "?" sits beside the label. On a
  collapsed section the "?" now appears once the section is opened.
- Admin UI: hints, column headers, section labels and every coloured text
  (links, warnings, error and status chips) are now at least 4.5:1 against
  their background in both themes; the light-theme off switch and keyboard
  focus ring are clearly visible.
- Admin UI: layout fixes - keyboard focus no longer squares off pill switches
  and round buttons; segmented pickers have rounded corners; the issued
  client certificates table no longer overflows its card; inline field rows
  keep the same 14px spacing as other fields; the Settings SSO provider list
  matches the other check lists; discovery upstream fields line up with the
  fields around them; the menu and row-arrow icons have a fixed size; banners,
  nested blocks and Integrations save bars share one style; long domains no
  longer break mid-label in list cards; long log paths are truncated with the
  full path on hover; toasts fit a phone screen; the page title lines up with
  the page content on wide screens and the topbar stays one line on a phone.
- Admin UI: proxy host and certificate rows, and object cards, can now be
  opened from the keyboard; sortable column headers keep their header
  semantics and announce the sort order; the off-screen navigation drawer on
  narrow screens is out of the tab order and closes on Escape; Settings tabs
  support the arrow keys; the current page, segmented pickers and zone chips
  expose their state to screen readers.
- Admin UI: Enter on a card's Clone or Delete button no longer also opens the
  object, and destructive confirm dialogs now open with Cancel focused.
- Admin UI: the hosts bulk-edit bar is greyed out for read-only users, not
  only on HA followers; refreshing or re-rendering the hosts, certificates,
  tokens, access logs and history pages keeps the read-only and maintenance
  banners and gating.
- Admin UI: the "config @" commit badge now updates after a restore or
  revert, and appears after the first write on a fresh install.
- Admin UI: deleting a certificate with unsaved edits no longer raises an
  "unsaved changes" prompt; the new-token card's border uses the accent
  colour; the certificate expiry sort keeps certificates without an expiry
  last in both directions; a failed DNS provider list now also disables the
  certificate editor's Save; the client certificate download is no longer
  cancelled in browsers that start it asynchronously.
- Admin UI: a webhook or notification secret that reads `***` (a redacted
  literal) is refused before saving, and the Settings page explains where to
  fix a refused literal secret it has no field for.

### Changed

- Docs accuracy and structure pass; neutralized wording in test fixtures and
  in failure-mode comments and docs. No behavior change.
- Admin UI: blocking validation messages are shown in the sticky banner at
  the top of the page, like a failed save, instead of a 7-second toast.
- Admin UI: every Integrations save bar is labelled "Save integrations",
  since each one saves every card on the page; all of them are disabled
  while a save is in flight.
- Admin UI: the middleware rate-limit card uses the same per-window /
  per-second form as the host and location rate limits, with free-form
  durations, so a middleware stored with `requestsPerSecond` keeps that form.
  A new rate-limit middleware now defaults to a 1m window (was 1s), as host
  and location rate limits already did.
- Admin UI: the stream, DNS provider, access list and middleware editors use
  the full page width instead of the left half; the Overview "Retry now"
  action, which only opened the certificate, is labelled "Open".
- Admin UI: domains, certificate names, upstreams and other names no longer
  break mid-word in tables and cards. They stay on one line and are cut off
  with "..." only when unusually long, with the full value on hover. Every
  Proxy Hosts row now has the same two lines: the domain, then the
  ingress/docker badge, source and tags.
- Admin UI: quieter lists. Timestamps and numbers are right-aligned, row
  buttons line up in one column, the certificate type and issuance method
  share one column, API tokens show three scopes plus a count (folded lists
  open with a tap or keypress on their "+N"), a commit author's email moves
  to a tooltip, discovery counters are only coloured when non-zero, and
  filter boxes use short placeholders with the full hint on hover.

### Security

- The post-login return path refuses every control byte, not only CR, LF and
  NUL: a TAB survived `http.Redirect` and browsers collapse `/\t/evil.com`
  into an off-site `//evil.com` redirect.
- `${FILE:...}` references can no longer read gpm's own credential files (the
  paths behind `GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE`,
  `GPM_LOCAL_ADMIN_TOTP_SECRET_FILE` and `GPM_SSO_SIGNING_KEY_FILE`), by path
  or through a symlink, whichever secret root they sit under.
- mTLS: client-certificate revocation is checked on resumed TLS sessions too.
  The stdlib skips `VerifyPeerCertificate` on resumption and TLS 1.3 renews
  the ticket every time, so a certificate revoked after its first handshake
  could keep access by resuming; `VerifyConnection` now re-checks it.
- Restore counts and sizes every archive entry before deciding whether to keep
  it, so entries it skips can no longer carry unbounded data through the
  decompressor.

## [1.0.10] - 2026-09-26

### Fixed

- ACME `keyType` was documented and offered in the UI as accepting `rsa`, but
  the CSR path only ever issues ECDSA P-256 certificates; a config with
  `keyType: rsa` validated successfully and only failed at the next
  issue/renew attempt. `Validate()` now rejects an unsupported
  `acme.keyType` at config-write time, and the docs, OpenAPI enum, and UI
  placeholder (`EC256` -> `ecdsa`) were corrected to match.
- Corrected the `acme.email` hint and docs text, which still described Let's
  Encrypt using the address for expiry notices; Let's Encrypt stopped
  sending those in 2025, and the address is now used only for
  account/policy notices.

## [1.0.9] - 2026-09-24

### Security

- Alpine base image bumped from 3.24.1 to 3.24.2.

### Changed

- Dependencies: modernc.org/sqlite v1.58.0 -> v1.59.0.

## [1.0.8] - 2026-09-22

### Security

- Dependencies: golang.org/x/crypto v0.56.0 -> v0.57.0.

### Changed

- Dependencies: github.com/oschwald/maxminddb-golang/v2 v2.5.0 -> v2.6.0
  (GeoIP lookups).

## [1.0.7] - 2026-09-10

### Security

- Dependencies: github.com/go-jose/go-jose/v4 v4.1.4 -> v4.1.5,
  golang.org/x/oauth2 v0.36.0 -> v0.37.0.

## [1.0.6] - 2026-09-06

### Changed

- CI: added a weekly Trivy rescan (Monday cron) of the released image on
  GHCR, so CVEs disclosed after release surface without needing a code push.

## [1.0.5] - 2026-09-06

### Changed

- CI: release flow renamed from `main -> prod` to `dev -> main`. `dev` is the
  default working branch, `main` the protected release branch; the bot PR is
  now "Merge dev to main". Mechanics unchanged. Pushes to `dev` now also
  publish `:dev` and `:dev-<sha>` images.

## [1.0.4] - 2026-09-05

### Changed

- Dependencies: migrated from the archived `gopkg.in/yaml.v3` to the
  maintained `go.yaml.in/yaml/v3` fork (same API).

## [1.0.3] - 2026-09-05

### Fixed

- The sidebar wordmark uses the same fan-out glyph as the favicon; the two
  had diverged in 1.0.2.

## [1.0.2] - 2026-09-03

### Added

- The admin UI declares a favicon (`favicon.svg`: a two-way fan-out on the
  brand gradient tile), so browser tabs and bookmarks show the app icon
  instead of the generic page glyph.

### Changed

- Removed the unused `deploy/runbook-template.md` authoring scaffold.
- The Go toolchain is pinned once, in `.go-version`: CI reads it, `make
  bump-go VERSION=X.Y.Z` rewrites it together with both builder images (tag
  and digest), and a test fails when any of them drift or when the docs quote
  a language minimum other than go.mod's.

## [1.0.1] - 2026-09-02

### Changed

- Build on Go 1.27.1 (builder image and CI toolchain).
- Dependencies: go-oidc v3.21.0, golang.org/x/crypto v0.56.0,
  modernc.org/sqlite v1.58.0 (with its libc and memory modules).

## [1.0.0] - 2026-09-02

First public release.

### Added

- **Proxying.** SNI-based TLS termination with exact and wildcard certificate
  matching, HTTP/2 and WebSockets, HTTP-to-HTTPS redirect, path-scoped
  locations, upstream groups (failover, weighted round-robin,
  least-connections, sticky ip-hash) with active and passive health checks,
  redirect hosts, raw TCP/UDP streams with SNI routing, and parked hosts.
- **Certificates.** ACME issuance and renewal over HTTP-01 or DNS-01 against
  any CA, four named DNS providers plus RFC2136 and acme-dns solvers, custom
  PEM certificates, a fleet-wide TLS floor, and a built-in client CA with
  PKCS#12 client bundles and CRL revocation.
- **Access and auth.** Ordered allow/deny access lists with GeoIP, path and
  method scoping and remote CIDR feeds, all keyed on one derived client IP;
  OIDC, forward-auth, `auth_request`, client-certificate and basic auth as
  typed config; a composable middleware chain (rate limits, guards, rewrites,
  security headers, bouncer hooks, maintenance mode).
- **Automation.** Kubernetes Ingress and Docker label discovery with plan and
  reconcile, DNS record sync, and notifications to ntfy, Discord or a generic
  webhook.
- **Admin panel and API.** Git-backed per-object YAML config with history,
  a REST API with an OpenAPI description and scoped tokens, local login with
  optional TOTP, OIDC admin sign-in with group-to-role mapping, a read-only
  viewer role, Prometheus metrics, and an Overview that lists only what needs
  attention.

[Unreleased]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.5.0...HEAD
[2.5.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.4.0...v2.5.0
[2.4.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.3.0...v2.4.0
[2.3.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.2.0...v2.3.0
[2.2.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.1.0...v2.2.0
[2.1.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.0.1...v2.1.0
[2.0.1]: https://github.com/Rake-Pro/go-proxy-manager/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.10...v2.0.0
[1.0.10]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.9...v1.0.10
[1.0.9]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.8...v1.0.9
[1.0.8]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.7...v1.0.8
[1.0.7]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.6...v1.0.7
[1.0.6]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.5...v1.0.6
[1.0.5]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/Rake-Pro/go-proxy-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/Rake-Pro/go-proxy-manager/releases/tag/v1.0.0
