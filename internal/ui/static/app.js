// go-proxy-manager admin SPA. Dependency-free vanilla ES module.

// ---------- icons ----------
const ICON = {
  arrow: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h16M13 6l7 6-7 6"/></svg>',
  grid: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>',
  globe: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3a9 9 0 100 18 9 9 0 000-18z"/><path d="M3 12h18M12 3c2.5 2.5 2.5 15 0 18M12 3c-2.5 2.5-2.5 15 0 18"/></svg>',
  redirect: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 7h11a4 4 0 014 4v0a4 4 0 01-4 4H4"/><path d="M8 3 4 7l4 4"/></svg>',
  stream: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 8h18M3 16h18M7 4v16M17 4v16"/></svg>',
  parked: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3.5" y="3.5" width="17" height="17" rx="3.5"/><path d="M9.5 17V7h3.2a3 3 0 010 6H9.5"/></svg>',
  lock: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg>',
  user: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0116 0"/></svg>',
  shield: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z"/></svg>',
  shieldCheck: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z"/><path d="M9.5 12l1.8 1.8L15 10"/></svg>',
  list: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 6h16M4 12h10M4 18h7"/></svg>',
  layers: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="5" rx="1.5"/><rect x="3" y="15" width="18" height="5" rx="1.5"/><path d="M8 9v6M16 9v6"/></svg>',
  headers: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 7h16M4 12h16M4 17h10"/></svg>',
  gauge: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 18a8 8 0 1116 0"/><path d="M12 18l4-5"/></svg>',
  history: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 12a9 9 0 109-9 9 9 0 00-7 3.3M3 3v4h4"/><path d="M12 7v5l3 2"/></svg>',
  cert: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg>',
  cog: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 00-.1-1.3l2-1.5-2-3.4-2.3 1a7 7 0 00-2.2-1.3L14 2h-4l-.4 2.2a7 7 0 00-2.2 1.3l-2.3-1-2 3.4 2 1.5A7 7 0 005 12a7 7 0 00.1 1.3l-2 1.5 2 3.4 2.3-1a7 7 0 002.2 1.3L10 22h4l.4-2.2a7 7 0 002.2-1.3l2.3 1 2-3.4-2-1.5A7 7 0 0019 12z"/></svg>',
  plus: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>',
  x: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  search: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/></svg>',
  server: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><path d="M7 8h.01M7 17h.01"/></svg>',
  clientUser: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="8" r="4"/><path d="M5 20a7 7 0 0114 0"/></svg>',
  trash: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  commit: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="3"/><path d="M12 3v6M12 15v6M3 12h6M15 12h6"/></svg>',
  menu: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/></svg>',
  // Access Logs and History both used to render ICON.history, which made two
  // unrelated sections read as one. Logs is a request tape (a page of entries);
  // history stays the clock-with-arrow.
  logbook: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M5 3h11l3 3v15H5z"/><path d="M8 9h8M8 13h8M8 17h5"/></svg>',
  // Proxy Hosts keeps ICON.globe; DNS Providers gets its own record-lookup mark
  // so the two are not the same glyph in one sidebar.
  dns: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3v6"/><circle cx="12" cy="11" r="2"/><path d="M12 13v3M6 21v-3a2 2 0 012-2h8a2 2 0 012 2v3"/><circle cx="12" cy="4" r="1.6"/></svg>',
  plug: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M9 3v6M15 3v6"/><path d="M6 9h12v3a6 6 0 01-12 0z"/><path d="M12 18v3"/></svg>',
  chevron: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg>',
};

// ---------- nav ----------
// Grouped rather than flat: seventeen equal-weight entries gave a daily task
// (add a host) the same visual rank as a once-a-year one (client CAs). The
// groups are the operator's mental model - what is served, what guards it, what
// runs it - and everything a small install never touches lives under a
// collapsed ADVANCED group. Route ids are unchanged, so every existing deep
// link (#/parked, #/identity, ...) still resolves; only labels moved.
const NAV_GROUPS = [
  {
    label: '', items: [
      { id: 'overview', label: 'Overview', icon: ICON.grid },
    ],
  },
  {
    label: 'Hosts', items: [
      { id: 'hosts', label: 'Proxy Hosts', icon: ICON.globe },
      { id: 'redirects', label: 'Redirects', icon: ICON.redirect },
    ],
  },
  {
    label: 'Security', items: [
      { id: 'certs', label: 'Certificates', icon: ICON.cert },
      { id: 'access', label: 'Access Lists', icon: ICON.shield },
      { id: 'identity', label: 'Identity Providers', icon: ICON.user },
      { id: 'middleware', label: 'Middleware', icon: ICON.layers },
    ],
  },
  {
    label: 'Operations', items: [
      { id: 'integrations', label: 'Integrations', icon: ICON.plug },
      { id: 'logs', label: 'Access Logs', icon: ICON.logbook },
      { id: 'history', label: 'History', icon: ICON.history },
      { id: 'settings', label: 'Settings', icon: ICON.cog },
    ],
  },
  {
    label: 'Advanced', collapsible: true, items: [
      { id: 'streams', label: 'Streams', icon: ICON.stream, count: 'streamHosts' },
      { id: 'parked', label: 'Parked Hosts', icon: ICON.parked, count: 'parkedHosts' },
      { id: 'upstreams', label: 'Upstream Groups', icon: ICON.server, count: 'upstreamGroups' },
      { id: 'clientcas', label: 'Client CAs', icon: ICON.clientUser, count: 'clientCAs' },
      { id: 'dns', label: 'DNS Providers', icon: ICON.dns, count: 'dnsProviders' },
      { id: 'errorpages', label: 'Error Pages', icon: ICON.headers },
      { id: 'tokens', label: 'API Tokens', icon: ICON.lock, count: 'apiTokens' },
    ],
  },
];

// Flat view of the grouped nav, for everything that only cares about the set of
// routes (active-item marking, capability gating, the route-coverage test).
const NAV = NAV_GROUPS.reduce((acc, g) => acc.concat(g.items), []);

const TITLES = {
  overview: 'Overview', hosts: 'Proxy Hosts', redirects: 'Redirects', streams: 'Streams',
  parked: 'Parked Hosts', certs: 'Certificates', clientcas: 'Client CAs', identity: 'Identity Providers', access: 'Access Lists',
  middleware: 'Middleware', upstreams: 'Upstream Groups', dns: 'DNS Providers',
  errorpages: 'Error Pages', integrations: 'Integrations',
  tokens: 'API Tokens', logs: 'Access Logs', history: 'History', settings: 'Settings',
};

// Registry id of the "About this page" block each section's list view renders.
const PAGE_HINT = {
  overview: 'page.overview', hosts: 'page.proxyHosts', redirects: 'page.redirects',
  streams: 'page.streams', parked: 'page.parkedHosts', certs: 'page.certificates',
  clientcas: 'page.clientCAs', identity: 'page.identityProviders', access: 'page.accessLists',
  middleware: 'page.middleware', upstreams: 'page.upstreamGroups', dns: 'page.dnsProviders',
  errorpages: 'page.errorPages', integrations: 'page.integrations', tokens: 'page.apiTokens',
  logs: 'page.accessLogs', history: 'page.history', settings: 'page.settings',
};

// plural API paths per section
const PLURAL = {
  hosts: 'proxy-hosts', redirects: 'redirect-hosts', streams: 'stream-hosts',
  parked: 'parked-hosts', certs: 'certificates', clientcas: 'client-cas', identity: 'identity-providers',
  access: 'access-lists', middleware: 'middlewares', upstreams: 'upstream-groups',
  dns: 'dns-providers', tokens: 'api-tokens',
};

// Maps a store object Kind() (as it appears in a commit message) to its plural
// API path, for offering a per-object revert straight from the history feed.
const KIND_PLURAL = {
  ProxyHost: 'proxy-hosts', RedirectHost: 'redirect-hosts', StreamHost: 'stream-hosts',
  ParkedHost: 'parked-hosts', Certificate: 'certificates', ClientCA: 'client-cas',
  DNSProvider: 'dns-providers', IdentityProvider: 'identity-providers',
  UpstreamGroup: 'upstream-groups', AccessList: 'access-lists', Middleware: 'middlewares',
  APIToken: 'api-tokens',
};

// parseObjectCommit recognises a single-object update commit (e.g.
// `ProxyHost "app": update`) and returns its kind/name/plural so the history
// view can offer a scoped revert. Non-object commits (import, restore, revert,
// settings, delete) return null and get only the whole-config revert.
function parseObjectCommit(msg) {
  const m = /^(\w+) "(.+)": update$/.exec(msg || '');
  if (!m) return null;
  const plural = KIND_PLURAL[m[1]];
  if (!plural) return null;
  return { kind: m[1], name: m[2], plural };
}

// ---------- helpers ----------
function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
function shortSha(s) { return (s || '').slice(0, 7); }
function fmtTime(s) {
  const d = new Date(s);
  if (isNaN(d.getTime())) return s || '';
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}
// Date only, no time - for a compact reading next to a relative-label chip.
// The full timestamp still goes in that chip's title tooltip via fmtTime.
function fmtDate(s) {
  const d = new Date(s);
  if (isNaN(d.getTime())) return '';
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
function arr(v) { return Array.isArray(v) ? v : []; }

// A label can be long enough to need to wrap, but only at a separator a reader
// expects (a dot between two real labels), never mid-word. <wbr> after such a
// "." gives the browser that break point explicitly; paired with
// overflow-wrap:normal and word-break:keep-all in CSS, that stops e.g.
// "acme-test.rake.pro" from splitting mid-label the way a bare
// word-break:break-all would. The dot must be preceded by a label character -
// not "*" - so a wildcard's "*." never breaks on its own, orphaned, from the
// label it prefixes.
function wbrLabel(s) {
  return esc(s).replace(/([A-Za-z0-9])\./g, '$1.<wbr>');
}
// Renders up to `max` items (default 2) comma-joined with <wbr> break points,
// folding the rest into "+N"; the full list is always in the title tooltip.
// Shared by any column that can hold a long domain/SAN list.
function domainListHtml(domains, max) {
  domains = arr(domains);
  if (!domains.length) return '';
  const shown = domains.slice(0, max || 2);
  const rest = domains.length - shown.length;
  const html = shown.map(wbrLabel).join(', <wbr>') + (rest > 0 ? ` <span class="faint">+${rest}</span>` : '');
  return `<span class="domain-list" title="${esc(domains.join(', '))}">${html}</span>`;
}

// ObjectMeta keys no editor renders a control for. Every PUT is a whole-object
// replacement, so a save body built from the form alone DELETES them: labels
// carries the discovery ownership marker (<prefix>/managed-by: ingress-discovery
// / docker-discovery) that makes the reconciler keep owning a host it created,
// and tags is the operator's own grouping. Both are seeded from the loaded
// object before the form's own fields are applied, so an editor that does own a
// control (the proxy-host tags chips) still overwrites its own key.
// createdAt/updatedAt are deliberately NOT carried: the store maintains them.
function metaCarryForward(o) {
  const out = {};
  if (!o) return out;
  if (o.labels && typeof o.labels === 'object' && Object.keys(o.labels).length) out.labels = o.labels;
  if (arr(o.tags).length) out.tags = arr(o.tags);
  return out;
}
function $(sel, root) { return (root || document).querySelector(sel); }
function $$(sel, root) { return Array.from((root || document).querySelectorAll(sel)); }

// Maps a domain to its "zone" (group) for the hosts list filter chips.
// A leading "*." is stripped first; a wildcard's remainder already names the
// zone (*.iot.example.com -> iot.example.com), so the label-drop below only runs
// for non-wildcard domains with more than 2 labels (sensor.iot.example.com
// -> iot.example.com, app.example.com -> example.com, example.com -> example.com).
function domainZone(d) {
  let s = String(d || '');
  const wildcard = s.startsWith('*.');
  if (wildcard) s = s.slice(2);
  if (!wildcard) {
    const parts = s.split('.').filter(Boolean);
    if (parts.length > 2) return parts.slice(1).join('.');
  }
  return s;
}

// ---------- enum labels ----------
// One map for every raw enum token this UI puts in front of an operator, so a
// select never reads as a list of internal identifiers ("forward-auth",
// "generated-only", "fail-open") that only make sense once you have read the
// config reference.
//
// The raw token is kept ALONGSIDE the human label, never replaced by it: it is
// what the YAML, the API and the docs use, so hiding it would leave the UI and
// the config file speaking different languages. Inside a <select> that has to
// be plain text ("Forward-auth (trusted headers) - forward-auth"), because HTML
// allows no markup in an <option>; enumChip() is the variant for everywhere
// else, where the token gets its own muted element.
//
// An empty-string key is the "not set" option, whose label says what the
// effective default is rather than appending a meaningless "- ".
const ENUM_LABELS = {
  middlewareType: {
    'auth': 'Authentication',
    'headers': 'Headers',
    'guard': 'Guard (block matching requests)',
    'rate-limit': 'Rate limit',
    'rewrite': 'Path rewrite',
    'bouncer': 'Bouncer (external deny hook)',
  },
  authMode: {
    '': 'Take the mode from the identity provider',
    'oidc': 'OIDC (redirect to the provider)',
    'forward-auth': 'Forward-auth (trusted headers)',
    'auth-request': 'Auth-request (external subrequest)',
    'client-cert': 'Client certificate (mTLS)',
    'basic': 'Basic (username/password)',
  },
  notificationType: {
    'ntfy': 'ntfy topic',
    'discord': 'Discord webhook',
    'generic': 'Generic JSON webhook',
  },
  hostHeader: {
    '': 'Keep client Host',
    'upstream': 'Use upstream host',
    'custom': 'Custom...',
  },
  tsigAlgorithm: {
    'hmac-sha1': 'HMAC-SHA1',
    'hmac-sha224': 'HMAC-SHA224',
    'hmac-sha256': 'HMAC-SHA256 (default)',
    'hmac-sha384': 'HMAC-SHA384',
    'hmac-sha512': 'HMAC-SHA512',
  },
  dnsTransport: {
    'tcp': 'TCP (default)',
    'udp': 'UDP - retried over TCP when truncated',
  },
  idpType: {
    'oidc': 'OIDC',
    'forward-auth': 'Forward-auth (trusted headers)',
    'auth-request': 'Auth-request (external outpost)',
  },
  role: {
    '': 'Deny when no group matches',
    'user': 'User',
    'admin': 'Admin',
  },
  headerScope: {
    'all': 'Every response',
    'generated-only': 'Only responses gpm writes itself',
    'proxied-only': 'Only proxied upstream responses',
  },
  loadBalance: {
    '': 'Failover - ordered, first healthy upstream wins (default)',
    'round-robin': 'Round robin (weighted, smooth)',
    'least-connections': 'Least connections (fewest in flight per weight)',
    'ip-hash': 'Sticky per client IP',
  },
  crlPolicy: {
    '': 'Fail closed - reject every client certificate (default)',
    'fail-open': 'Fail open - accept and log',
  },
  // Settings -> Ingress discovery. Listed here so the Settings pass has the
  // labels; the control itself still lives on that page.
  profileSelection: {
    '': 'Annotation or rules (default)',
    'rules-only': 'Rules only - ignore the annotation',
  },
  acmeChallenge: {
    'http-01': 'HTTP-01 (validated on port 80)',
    'dns-01': 'DNS-01 (via a DNS provider; the only way to get a wildcard)',
  },
  certType: {
    'acme': 'ACME (Let\'s Encrypt and compatible CAs)',
    'custom': 'Custom (PEM files already on the server)',
  },
  mtlsMode: {
    'require': 'Require - the handshake rejects certless clients',
    'optional': 'Optional - certless clients still reach the chain',
  },
  streamProtocol: {
    'tcp': 'TCP',
    'udp': 'UDP',
    'both': 'TCP and UDP',
  },
  streamTLSMode: {
    '': 'None - forward the bytes blind',
    'passthrough': 'Passthrough - route on SNI, never decrypt',
    'terminate': 'Terminate - handshake at gpm, plaintext upstream',
  },
  redirectScheme: {
    'auto': 'Auto - keep the scheme the client used',
    'http': 'HTTP',
    'https': 'HTTPS',
  },
  redirectStatus: {
    '301': 'Moved permanently (cached by browsers)',
    '302': 'Found - temporary',
    '307': 'Temporary redirect (keeps the method)',
    '308': 'Permanent redirect (keeps the method)',
  },
  ruleAction: {
    'allow': 'Allow',
    'deny': 'Deny',
  },
  ruleMatch: {
    'cidr': 'CIDR or IP',
    'source': 'Named source feed',
  },
  geoUnknown: {
    '': 'Allow (default)',
    'allow': 'Allow',
    'deny': 'Deny',
  },
  bouncerProvider: {
    'crowdsec': 'CrowdSec local API',
    'http': 'Generic HTTP hook',
  },
  bouncerOnError: {
    'fail-open': 'Fail open - allow the request',
    'fail-closed': 'Fail closed - deny the request',
  },
  bouncerDenyWith: {
    'error-page': 'The configured error page',
    'plain': 'A plain-text body',
  },
};

// enumLabel is the single place a raw token becomes operator-facing text.
function enumLabel(group, token) {
  const map = ENUM_LABELS[group] || {};
  const key = token == null ? '' : String(token);
  const human = map[key];
  if (key === '') return human || '(default)';
  return human ? `${human} - ${key}` : key;
}
// enumOptions builds a whole <select> body for one enum, in the order given.
function enumOptions(group, tokens, selected) {
  const sel = selected == null ? '' : String(selected);
  return tokens.map((t) => {
    const v = t == null ? '' : String(t);
    return `<option value="${esc(v)}"${v === sel ? ' selected' : ''}>${esc(enumLabel(group, v))}</option>`;
  }).join('');
}
// enumChip renders an enum outside a <select>, where the raw token can have its
// own muted element instead of being glued to the label with a dash.
function enumChip(group, token) {
  const map = ENUM_LABELS[group] || {};
  const key = token == null ? '' : String(token);
  const human = map[key];
  if (!human) return `<span class="mono">${esc(key)}</span>`;
  if (key === '') return esc(human);
  return `${esc(human)} <span class="enum-raw mono">${esc(key)}</span>`;
}

// ---------- api ----------
let csrfToken = '';

// One place decides what an expired session looks like. Every request this SPA
// makes - api(), and the two raw fetch() calls that need a binary body or a
// multipart upload - funnels its response through here, so a 401 always ends in
// a re-authentication rather than in a "Restore failed (401)" the operator
// answers by picking the archive again.
function redirectOn401(res) {
  if (res.status !== 401) return;
  location.href = '/auth/login?return=' + encodeURIComponent(location.pathname + location.hash);
  throw new Error('Unauthorized');
}

async function api(path, opts) {
  opts = opts || {};
  const init = { method: opts.method || 'GET', headers: {}, credentials: 'same-origin' };
  if (opts.body !== undefined && opts.body !== null) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(opts.body);
  }
  if (init.method !== 'GET' && init.method !== 'HEAD') {
    init.headers['X-CSRF-Token'] = csrfToken;
  }
  let res;
  try {
    res = await fetch(path, init);
  } catch (e) {
    throw new Error('Network error: ' + (e && e.message ? e.message : 'request failed'));
  }
  redirectOn401(res);
  const commit = res.headers.get('X-Config-Commit');
  let data = null;
  const text = await res.text();
  if (text) {
    try { data = JSON.parse(text); } catch (e) { data = text; }
  }
  if (!res.ok) {
    let msg;
    if (data && typeof data === 'object' && data.error) msg = data.error;
    else if (typeof data === 'string' && data) msg = data;
    else msg = `Request failed (${res.status})`;
    const err = new Error(msg);
    err.status = res.status;
    throw err;
  }
  return { data, commit };
}

// ---------- toasts ----------
function toast(title, msg, type, opts) {
  const wrap = document.getElementById('toasts');
  if (!wrap) return;
  const t = document.createElement('div');
  t.className = 'toast ' + (type || '');
  // msg is escaped as text by default; pass {html:true} only for deliberately
  // built markup (the caller stays responsible for escaping any data in it).
  t.innerHTML = `<div class="t-title">${esc(title)}</div>` + (msg ? `<div class="t-msg">${(opts && opts.html) ? msg : esc(msg)}</div>` : '');
  wrap.appendChild(t);
  setTimeout(() => {
    t.style.transition = 'opacity .3s';
    t.style.opacity = '0';
    setTimeout(() => t.remove(), 300);
  }, type === 'err' ? 7000 : 4500);
}
function toastSaved(commit) {
  const sha = shortSha(commit);
  toast('Saved', sha ? `committed <span class="sha">${esc(sha)}</span>` : 'changes committed to git', 'ok', { html: true });
}
function toastErr(e) {
  toast('Something went wrong', e && e.message ? e.message : String(e), 'err');
}

// A toast that stays put and is rewritten as work proceeds, for the bulk
// actions that issue one request per selected object. Returns {step, done}.
function progressToast(title) {
  const wrap = document.getElementById('toasts');
  const t = document.createElement('div');
  t.className = 'toast';
  t.innerHTML = `<div class="t-title">${esc(title)}</div><div class="t-msg">starting...</div>`;
  if (wrap) wrap.appendChild(t);
  const msg = t.querySelector('.t-msg');
  return {
    step(text) { if (msg) msg.textContent = text; },
    done(doneTitle, text, type) {
      const ti = t.querySelector('.t-title');
      if (ti) ti.textContent = doneTitle;
      if (msg) msg.textContent = text;
      t.className = 'toast ' + (type || 'ok');
      setTimeout(() => {
        t.style.transition = 'opacity .3s';
        t.style.opacity = '0';
        setTimeout(() => t.remove(), 300);
      }, type === 'err' ? 9000 : 4500);
    },
  };
}

// ---------- save-failure banner ----------
// A save failure is not a notification, it is the state the page is now in: the
// operator has to read the message, fix a field and try again. A 7-second toast
// gave them none of that, so a failed save now writes a banner at the top of the
// open editor that stays until the next save attempt (or until the view is
// replaced, which re-renders #content and takes the banner with it).
//
// FIELD PATHS: the API's validation errors are prefixed with the dotted path of
// the offending field ("tls.hsts.maxAge: must be ..."). Inputs carry that same
// path in data-path, so the banner can flag the field, scroll to it and offer a
// "Go to field" jump. Longest-prefix matching, so "tls.hsts.maxAge" still finds
// a control registered as "tls.hsts" when the leaf has no control of its own.
const ERROR_PATH_RE = /^([A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_[\]-]+)+)\s*:\s*/;

function clearEditorError() {
  $$('#content .editor-error').forEach((el) => el.remove());
  $$('#content .field-flagged').forEach((el) => el.classList.remove('field-flagged'));
}
function errorFieldFor(msg) {
  const m = ERROR_PATH_RE.exec(String(msg || ''));
  if (!m) return null;
  const parts = m[1].split('.');
  for (let n = parts.length; n > 0; n--) {
    const path = parts.slice(0, n).join('.');
    const el = document.querySelector(`#content [data-path="${path}"]`);
    if (el) return { el, path: m[1] };
  }
  return null;
}
function focusErrorField(target) {
  if (!target || !target.el) return;
  target.el.classList.add('field-flagged');
  try { target.el.scrollIntoView({ block: 'center', behavior: 'smooth' }); } catch (e) { target.el.scrollIntoView(); }
  // A field inside a collapsed <details> is worth nothing until it is open.
  let d = target.el.closest && target.el.closest('details');
  while (d) { d.open = true; d = d.parentElement && d.parentElement.closest('details'); }
  if (target.el.focus) { try { target.el.focus({ preventScroll: true }); } catch (e) { /* ignore */ } }
}
// showSaveError(e, title) renders the banner and returns it. Call clearEditorError()
// at the top of every save handler so a stale failure never outlives its cause.
function showSaveError(e, title) {
  const c = $('#content');
  if (!c) { toastErr(e); return null; }
  clearEditorError();
  const msg = (e && e.message) ? e.message : String(e);
  const target = errorFieldFor(msg);
  const banner = document.createElement('div');
  banner.className = 'editor-error';
  banner.setAttribute('role', 'alert');
  banner.innerHTML = `<div class="ee-body">
      <div class="ee-title">${esc(title || 'Could not save')}</div>
      <div class="ee-msg">${esc(msg)}</div>
      ${target ? `<div class="ee-jump"><button class="btn ghost sm" type="button" id="ee-jump">Go to <span class="mono">${esc(target.path)}</span></button></div>` : ''}
    </div>
    <button class="ee-close" type="button" aria-label="Dismiss">&times;</button>`;
  c.insertBefore(banner, c.firstChild);
  banner.querySelector('.ee-close').addEventListener('click', () => clearEditorError());
  const jump = banner.querySelector('#ee-jump');
  if (jump) jump.addEventListener('click', () => focusErrorField(target));
  if (target) focusErrorField(target); else window.scrollTo(0, 0);
  return banner;
}

// ---------- inline row validation ----------
// The row editors (locations, access-list rules, sources, basic-auth users)
// used to DROP a half-filled row silently at save time - the operator typed an
// upstream, hit save, got "Saved", and the row was gone. These two put the
// complaint on the row itself, in the makeSecurityHeaderRows.error() shape:
// clear everything, then mark what is wrong and block the save.
function clearRowErrors(wrap) {
  if (!wrap) return;
  wrap.querySelectorAll('.row-error').forEach((el) => el.remove());
  wrap.querySelectorAll('.row-bad').forEach((el) => el.classList.remove('row-bad'));
}
function markRowError(row, msg) {
  if (!row) return msg;
  row.classList.add('row-bad');
  const d = document.createElement('div');
  d.className = 'row-error';
  d.textContent = msg;
  row.appendChild(d);
  return msg;
}

// ---------- confirm modal ----------
// The one confirmation primitive in this app. window.confirm() is a browser
// chrome string with no room to explain a consequence, and it reads identically
// for "delete this row" and "reset every object in the config" - so the
// dangerous actions get this instead: a titled dialog with real body copy and,
// for anything wholesale or hard to undo, a typed confirmation.
//
// confirmModal({title, body, confirmLabel, danger, typed}) -> Promise<boolean>.
//   body         markup (the CALLER escapes any data it interpolates).
//   typed        a word the operator must type exactly before Confirm enables.
//   prompt       {label, placeholder} - asks for a value instead of a bare yes.
//                Resolves with the trimmed STRING on confirm (never empty:
//                Confirm stays disabled until something is typed), so callers
//                that want a value and callers that want a boolean both read
//                the result as truthy-or-not.
//   danger       styles Confirm as destructive.
// Resolves false on Cancel, Escape and a scrim click, so a dismissed dialog is
// never a confirmation.
function confirmModal(opts) {
  opts = opts || {};
  return new Promise((resolve) => {
    const prevFocus = document.activeElement;
    const wrap = document.createElement('div');
    wrap.className = 'modal-scrim';
    wrap.innerHTML = `
      <div class="modal" role="dialog" aria-modal="true" aria-labelledby="cm-title">
        <div class="modal-title" id="cm-title">${esc(opts.title || 'Are you sure?')}</div>
        <div class="modal-body">${opts.body || ''}</div>
        ${opts.typed ? `<div class="field-group modal-typed">
          <label for="cm-typed">Type <span class="mono">${esc(opts.typed)}</span> to confirm</label>
          <input class="field mono" id="cm-typed" autocomplete="off" spellcheck="false" aria-label="Type ${esc(opts.typed)} to confirm" />
        </div>` : ''}
        ${opts.prompt ? `<div class="field-group modal-typed">
          <label for="cm-prompt">${esc(opts.prompt.label || 'Value')}</label>
          <input class="field mono" id="cm-prompt" autocomplete="off" spellcheck="false" placeholder="${esc(opts.prompt.placeholder || '')}" aria-label="${esc(opts.prompt.label || 'Value')}" />
        </div>` : ''}
        <div class="modal-actions">
          <button class="btn ghost" id="cm-cancel" type="button">Cancel</button>
          <button class="btn ${opts.danger === false ? 'primary' : 'danger'}" id="cm-ok" type="button">${esc(opts.confirmLabel || 'Confirm')}</button>
        </div>
      </div>`;
    document.body.appendChild(wrap);

    const okBtn = wrap.querySelector('#cm-ok');
    const typedInput = wrap.querySelector('#cm-typed');
    const promptInput = wrap.querySelector('#cm-prompt');
    let done = false;
    function finish(v) {
      if (done) return;
      done = true;
      document.removeEventListener('keydown', onKey, true);
      wrap.remove();
      if (prevFocus && prevFocus.focus) { try { prevFocus.focus(); } catch (e) { /* ignore */ } }
      resolve(v);
    }
    function typedOK() {
      if (opts.typed && !(typedInput && typedInput.value.trim() === opts.typed)) return false;
      if (opts.prompt && !(promptInput && promptInput.value.trim())) return false;
      return true;
    }
    // Confirming a prompt dialog answers with the value, not with `true`.
    function answer() { return promptInput ? promptInput.value.trim() : true; }
    function refresh() { okBtn.disabled = !typedOK(); }
    function onKey(e) {
      if (e.key === 'Escape') { e.preventDefault(); finish(false); return; }
      if (e.key === 'Enter' && typedOK() && e.target !== wrap.querySelector('#cm-cancel')) { e.preventDefault(); finish(answer()); }
      // Crude focus trap: Tab out of the dialog wraps back into it.
      if (e.key === 'Tab') {
        const f = Array.from(wrap.querySelectorAll('button:not([disabled]), input'));
        if (!f.length) return;
        const first = f[0], last = f[f.length - 1];
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      }
    }
    document.addEventListener('keydown', onKey, true);
    wrap.addEventListener('click', (e) => { if (e.target === wrap) finish(false); });
    wrap.querySelector('#cm-cancel').addEventListener('click', () => finish(false));
    okBtn.addEventListener('click', () => { if (typedOK()) finish(answer()); });
    if (typedInput) typedInput.addEventListener('input', refresh);
    if (promptInput) promptInput.addEventListener('input', refresh);
    if (typedInput || promptInput) { refresh(); (promptInput || typedInput).focus(); }
    else okBtn.focus();
  });
}

// ---------- unsaved-changes tracking ----------
// Armed for any view that renders a save bar (every editor, Settings, Error
// pages) and disarmed everywhere else, so list and status views never prompt.
// Any input/change/switch event inside the content area marks the view dirty; a
// successful save clears it.
let dirtyArmed = false;
let dirtyFlag = false;
function armDirtyTracking(container) {
  dirtyArmed = !!(container && container.querySelector('.save-bar'));
  dirtyFlag = false;
}
function markDirty(e) {
  if (!dirtyArmed) return;
  const t = e && e.target;
  if (t && t.closest && t.closest('#content')) dirtyFlag = true;
}
function clearDirty() { dirtyFlag = false; }
document.addEventListener('input', markDirty);
document.addEventListener('change', markDirty);
document.addEventListener('switchchange', markDirty);
window.addEventListener('beforeunload', (e) => {
  if (!dirtyFlag) return;
  e.preventDefault();
  e.returnValue = '';
  return '';
});

// ---------- shell ----------
const state = { me: null, version: null, headSha: null, instance: 'go-proxy-manager', appName: 'Go Proxy Manager', capabilities: null, counts: {}, summary: null, runtime: null, refListFailed: [], routeMemo: null };

// ---------- per-route request memo ----------
// loadTopbar and the view that follows it both want /api/settings,
// /api/history and /api/config/summary, and neither cached them: a cold
// #/hosts/<name> load issued /api/settings twice, #/overview issued all three
// twice. The memo lives for exactly ONE route() render - reset before the view
// function runs, filled by whichever of the shell or the view asks first - so a
// Save that changes settings is still re-read on the next render and no stale
// value can outlive the page it was fetched for.
function resetRouteMemo() { state.routeMemo = {}; }
function memoGet(path) {
  if (!state.routeMemo) state.routeMemo = {};
  if (!state.routeMemo[path]) {
    // A rejected promise is evicted so the next caller retries rather than
    // inheriting the failure for the rest of the render.
    state.routeMemo[path] = api(path).catch((e) => { delete state.routeMemo[path]; throw e; });
  }
  return state.routeMemo[path];
}
// Degrading accessors for the views that render fine without the value.
// settings keeps the { data } envelope its call sites already destructure.
function routeSettings() {
  return memoGet('/api/settings').catch(() => null).then((r) => ({ data: (r && r.data) || {} }));
}
function routeHistory() {
  return memoGet('/api/history').catch(() => null).then((r) => arr(r && r.data));
}

// GET /api/runtime, cached like the capability probe. Read-only startup facts
// (listeners, paths, HA role, whether a local admin credential exists), so a
// 403 for a token without settings:read is a normal outcome, not an error.
async function loadRuntime() {
  if (state.runtime !== null) return state.runtime;
  try { state.runtime = (await api('/api/runtime')).data || {}; }
  catch (e) { state.runtime = { _error: (e && e.message) || 'unavailable' }; }
  return state.runtime;
}

// Object counts per kind, from GET /api/config/summary - a counts-only endpoint,
// so the sidebar no longer pulls the whole config just to len() a dozen slices.
// Only the sidebar and the Overview checklist read them, so a failed fetch
// degrades to "everything empty" rather than an error. The response keys are the
// kebab-case API plurals; state.counts keeps its camelCase names because other
// code (navGroupDefaultOpen, the get-started checklist) reads them.
async function loadCounts() {
  try {
    const summary = (await memoGet('/api/config/summary')).data || {};
    const cc = summary.counts || {};
    state.summary = summary;
    state.counts = {
      proxyHosts: cc['proxy-hosts'] || 0,
      redirectHosts: cc['redirect-hosts'] || 0,
      streamHosts: cc['stream-hosts'] || 0,
      parkedHosts: cc['parked-hosts'] || 0,
      certificates: cc['certificates'] || 0,
      clientCAs: cc['client-cas'] || 0,
      dnsProviders: cc['dns-providers'] || 0,
      identityProviders: cc['identity-providers'] || 0,
      upstreamGroups: cc['upstream-groups'] || 0,
      accessLists: cc['access-lists'] || 0,
      middlewares: cc['middlewares'] || 0,
      apiTokens: cc['api-tokens'] || 0,
    };
  } catch (e) { state.counts = {}; }
  return state.counts;
}

// GET /api/capabilities probe (e.g. { geoip: { dbLoaded } }), cached on state the
// same way loadTopbar caches /api/me and /api/settings. Call from anywhere that
// needs a capability check; the network fetch only happens once.
async function loadCapabilities() {
  if (state.capabilities) return state.capabilities;
  try { state.capabilities = (await api('/api/capabilities')).data || {}; }
  catch (e) { state.capabilities = {}; }
  return state.capabilities;
}

// ---------- theme ----------
// gpm.theme in localStorage is 'light' | 'dark' | absent (auto, follows the OS).
// index.html's inline head script applies the saved choice as documentElement's
// data-theme before first paint (see it for why); everything here just keeps
// the topbar toggle and that attribute in sync after the SPA boots.
const THEME_KEY = 'gpm.theme';
function getTheme() {
  try {
    const t = localStorage.getItem(THEME_KEY);
    if (t === 'light' || t === 'dark') return t;
  } catch (e) { /* ignore */ }
  return 'auto';
}
function applyTheme(t) {
  if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  else document.documentElement.removeAttribute('data-theme');
}
function themeLabel(t) { return t === 'light' ? 'Light' : t === 'dark' ? 'Dark' : 'Auto'; }
function updateThemeBtn() {
  const btn = $('#themeBtn');
  if (!btn) return;
  const t = getTheme();
  btn.textContent = 'Theme: ' + themeLabel(t);
  btn.title = 'Click to change (auto follows your OS)';
}
function setTheme(t) {
  try {
    if (t === 'auto') localStorage.removeItem(THEME_KEY); else localStorage.setItem(THEME_KEY, t);
  } catch (e) { /* ignore */ }
  applyTheme(t);
  updateThemeBtn();
}

// ---------- grouped sidebar ----------
// A collapsible group remembers its own state per browser (gpm.nav.<group>).
// With no stored preference it starts collapsed UNLESS the install actually
// uses something in it: state.counts comes from the /api/config summary loaded
// at boot, so an operator who has stream hosts or client CAs never has to
// discover that the group holding them exists.
const NAV_GROUP_KEY = 'gpm.nav.';
function navGroupId(label) { return 'nav-grp-' + label.toLowerCase().replace(/[^a-z0-9]+/g, '-'); }
function navGroupDefaultOpen(group) {
  return group.items.some((n) => n.count && (state.counts[n.count] || 0) > 0);
}
function navGroupOpen(group) {
  try {
    const v = localStorage.getItem(NAV_GROUP_KEY + group.label);
    if (v === 'open') return true;
    if (v === 'closed') return false;
  } catch (e) { /* ignore */ }
  return navGroupDefaultOpen(group);
}
function navItemHtml(n) {
  return `<button class="nav-item" data-view="${n.id}">${n.icon}${esc(n.label)}</button>`;
}
function navGroupHtml(group) {
  const items = group.items.map(navItemHtml).join('');
  if (!group.label) return `<div class="nav-group">${items}</div>`;
  const id = navGroupId(group.label);
  if (!group.collapsible) {
    return `<div class="nav-group"><p class="nav-group-label" id="${id}-lbl">${esc(group.label)}</p>
      <div class="nav-group-items" role="group" aria-labelledby="${id}-lbl">${items}</div></div>`;
  }
  const open = navGroupOpen(group);
  return `<div class="nav-group nav-collapsible">
    <button type="button" class="nav-group-head" data-group="${esc(group.label)}" aria-expanded="${open ? 'true' : 'false'}" aria-controls="${id}">
      <span class="nav-chev" aria-hidden="true">${ICON.chevron}</span>${esc(group.label)}
    </button>
    <div class="nav-group-items" id="${id}" role="group"${open ? '' : ' hidden'}>${items}</div>
  </div>`;
}
// persist=false is used when the app opens a group on the operator's behalf
// (a deep link into a collapsed group), so it does not overwrite their choice.
function setNavGroupOpen(label, open, persist) {
  const head = document.querySelector(`#nav .nav-group-head[data-group="${label}"]`);
  if (!head) return;
  head.setAttribute('aria-expanded', open ? 'true' : 'false');
  const body = document.getElementById(head.getAttribute('aria-controls'));
  if (body) body.hidden = !open;
  if (persist) {
    try { localStorage.setItem(NAV_GROUP_KEY + label, open ? 'open' : 'closed'); } catch (e) { /* ignore */ }
  }
}

function buildShell() {
  const app = document.getElementById('app');
  app.innerHTML = `
    <div class="scrim" id="scrim"></div>
    <div class="app">
      <aside class="sidebar">
        <div class="wordmark">
          <span class="logo" aria-hidden="true">${ICON.arrow}</span>
          <span class="name">${esc(state.appName)}</span>
        </div>
        <nav class="nav" id="nav" aria-label="Primary">
          ${NAV_GROUPS.map(navGroupHtml).join('')}
        </nav>
      </aside>
      <div class="main">
        <header class="topbar">
          <button class="menu-btn" id="menuBtn" aria-label="Open navigation">${ICON.menu}</button>
          <h1 class="page-title" id="pageTitle">${esc(state.appName)}</h1>
          <div class="spacer"></div>
          <button class="btn ghost sm" id="themeBtn" type="button"></button>
          <div class="ident" id="ident"></div>
        </header>
        <main class="content" id="content"></main>
      </div>
    </div>`;

  $$('#nav .nav-item').forEach((b) => {
    b.addEventListener('click', () => { location.hash = '#/' + b.dataset.view; closeNav(); });
  });
  // The collapsible group header is a real <button>, so Enter/Space and the tab
  // order come from the platform rather than from a keydown handler here.
  $$('#nav .nav-group-head').forEach((b) => {
    b.addEventListener('click', () => setNavGroupOpen(b.dataset.group, b.getAttribute('aria-expanded') !== 'true', true));
  });
  $('#menuBtn').addEventListener('click', () => document.body.classList.add('nav-open'));
  $('#scrim').addEventListener('click', closeNav);
  updateThemeBtn();
  $('#themeBtn').addEventListener('click', () => {
    const order = ['auto', 'light', 'dark'];
    setTheme(order[(order.indexOf(getTheme()) + 1) % order.length]);
  });
}
function closeNav() { document.body.classList.remove('nav-open'); }

async function loadTopbar() {
  // Best-effort and each independent, so they are issued together instead of
  // one round trip after another: the shell used to wait for six sequential
  // GETs before the first view could render.
  const ident = $('#ident');
  let verStr = '', cfgSha = '', principal = '';
  const ok = (p) => p.then((r) => r, () => null);
  const [vR, hR, meR, , sR] = await Promise.all([
    ok(api('/version')),
    ok(memoGet('/api/history')),
    ok(api('/api/me')),
    ok(loadCapabilities()),
    ok(memoGet('/api/settings')),
    ok(loadCounts()),
  ]);
  const v = vR && vR.data;
  state.version = v;
  if (v) verStr = `${v.version || ''}${v.commit ? ' - ' + shortSha(v.commit) : ''}`;

  const h = hR && hR.data;
  if (Array.isArray(h) && h.length) { state.headSha = h[0].hash; cfgSha = shortSha(h[0].hash); }

  const me = meR && meR.data;
  state.me = me;
  if (me) {
    csrfToken = me.csrfToken || '';
    const name = me.Name || me.Subject || me.Email || 'user';
    const role = me.Role || '';
    const idp = me.IdP || '';
    principal = `<b>${esc(name)}</b>${role ? ' &middot; ' + esc(role) : ''}${idp ? ' &middot; via ' + esc(idp) : ''}`;
    state.avatarChar = (name[0] || '?').toLowerCase();
  }

  const s = sR && sR.data;
  if (s && s.externalBaseURL) {
    try { state.instance = new URL(s.externalBaseURL).host || s.externalBaseURL; }
    catch (e) { state.instance = s.externalBaseURL; }
  }
  if (s && s.appName) {
    state.appName = s.appName;
    const nm = document.querySelector('.wordmark .name');
    if (nm) nm.textContent = s.appName;
    document.title = s.appName;
  }

  ident.innerHTML = `
    <span class="instance">${esc(state.instance)}</span>
    ${verStr ? `<span class="badge ver" title="Build version">${esc(verStr)}</span>` : ''}
    ${cfgSha ? `<a class="badge" href="#/history" title="Current config commit">config @ ${esc(cfgSha)}</a>` : ''}
    <span class="principal">
      <span class="avatar">${esc(state.avatarChar || 'g')}</span>
      <span>${principal || 'not signed in'}</span>
    </span>
    <button class="btn ghost sm" id="logoutBtn">Log out</button>`;
  $('#logoutBtn').addEventListener('click', logout);
}

async function logout() {
  try { await api('/auth/logout', { method: 'POST' }); } catch (e) { /* ignore */ }
  // select=1 keeps gpm on the login page instead of auto-redirecting back into a
  // still-live SSO session under ssoOnly (which would silently re-log-in).
  location.href = '/auth/login?select=1';
}

// ---------- small UI builders ----------
// The value the HSTS max-age input renders when the stored object has none.
// Kept as a constant so the save path can tell "the operator left the default
// showing" apart from "the operator typed one year", and omit the key in the
// first case (absent stays absent).
const HSTS_DEFAULT_MAX_AGE = 31536000;
function hstsMaxAgeFor(stored, el) {
  const typed = parseInt(el ? el.value : '', 10);
  if (isNaN(typed) || typed < 0) return null;
  if (stored != null) return typed;
  return typed === HSTS_DEFAULT_MAX_AGE ? null : typed;
}
function switchHtml(id, checked, label, hintId) {
  return `<button class="switch" type="button" role="switch" id="${id}" aria-checked="${checked ? 'true' : 'false'}"`
    + `${label ? ` aria-label="${esc(label)}"` : ''}${hintId ? ` data-hint="${esc(hintId)}"` : ''}></button>`;
}
function isOn(id) { const el = document.getElementById(id); return el && el.getAttribute('aria-checked') === 'true'; }

// ---------- capability gating ----------
// Reads a dotted path out of the cached /api/capabilities probe, e.g.
// hasCapability('geoip.dbLoaded'). Missing/unloaded capabilities read as false.
function hasCapability(path) {
  let v = state.capabilities;
  for (const k of path.split('.')) { if (v == null) return false; v = v[k]; }
  return !!v;
}
// Greys out el (a field/select/button, or a composite wrapper such as a
// chip-input div) and attaches a tooltip explaining why, when a runtime
// capability is unavailable. Reusable for any future capability-gated control -
// just pass the element(s), the capability's boolean, and the reason text.
// No-ops (and clears the disabled state) when available.
function gateControl(el, available, reason) {
  if (!el) return;
  el.classList.toggle('cap-disabled', !available);
  el.setAttribute('aria-disabled', available ? 'false' : 'true');
  if ('disabled' in el) el.disabled = !available;
  // An <a> has no disabled property, so CSS pointer-events is all that stops a
  // gated link - and that stops the mouse only. Take it out of the tab order
  // and drop the href so Tab+Enter cannot follow it either.
  if (el.tagName === 'A') {
    if (!available) {
      if (el.hasAttribute('href')) { el.dataset.gatedHref = el.getAttribute('href'); el.removeAttribute('href'); }
      el.setAttribute('tabindex', '-1');
      el.setAttribute('role', 'link');
    } else if (el.dataset.gatedHref) {
      el.setAttribute('href', el.dataset.gatedHref);
      delete el.dataset.gatedHref;
      el.removeAttribute('tabindex');
    }
  }
  if (!available) el.title = reason; else el.removeAttribute('title');
  el.querySelectorAll('input, select, button').forEach((child) => { child.disabled = !available; });
}
// ---------- reference lists ----------
// A list of objects that another object points at by name: middlewares, access
// lists, upstream groups, identity providers, DNS providers, certificates.
// Three states, and they MUST stay distinguishable: loaded with entries, loaded
// and empty, or failed. Collapsing the last two into an empty picker is a
// data-loss bug rather than a cosmetic one - every editor PUTs a whole object,
// so a save built from an empty picker strips the references the object already
// holds, and an unrelated 500 on GET /api/middlewares silently takes a live
// host's auth middleware and its IP allowlist away while the toast says
// "Saved". So a failed fetch yields null, the view renders an explicit
// unavailable state instead of an empty one, and Save is refused until the
// operator reloads. Same three-state contract the client CA picker has had.
function refList(path, label) {
  return api(path).then((r) => arr(r.data)).catch(() => {
    if (arr(state.refListFailed).indexOf(label) === -1) state.refListFailed.push(label);
    return null;
  });
}
function resetRefListFailures() { state.refListFailed = []; }
// Rendered by route() after the view: one banner naming every list that failed,
// and every Save on the page disabled with the same reason as its tooltip.
function applyRefListGuard(container) {
  const failed = arr(state.refListFailed);
  if (!container || !failed.length) return;
  const msg = `Could not load ${failed.join(', ')}; save is disabled to avoid stripping references. Reload the page to try again.`;
  const banner = document.createElement('div');
  banner.className = 'ro-banner warn';
  banner.innerHTML = `<b>Reference list unavailable.</b> ${esc(msg)}`;
  container.prepend(banner);
  // Save only: deleting an object does not depend on a reference list, and
  // greying Delete out under a message about saving would just read as a bug.
  container.querySelectorAll('#saveBtn, #ed-save, #set-save, .set-save, .int-save').forEach((b) => gateControl(b, false, msg));
}
// The in-card note that replaces an empty picker when its list did not load.
function refListUnavailableHtml(what) {
  return `<div class="check-empty">Could not load ${esc(what)}. Reload the page - saving is disabled so the references stored on this object are left exactly as they are.</div>`;
}

// ---------- HA read-only mode ----------
// A follower instance (capabilities.ha.readOnly) refuses every config write with
// a 503, so its write controls are greyed out up front instead of accepting a
// change the API will reject. Runs after each view renders; the server enforces
// this independently.
const RO_WRITE_CONTROLS = [
  '.btn.primary', '.btn.danger', '[data-revert]', '[data-revert-obj]',
  '.tok-rotate', '#restoreBtn', '#backupBtn', '#set-save', '.set-save', '#errp-save',
  '#set-sso-revoke', '#set-dns-run', '#set-id-run', '#set-dkr-run', '#int-als-run',
  '#ed-src-reconcile', '#logsToggle', '.wh-test', '.ntf-test', '.ct-renew', '#ed-renew',
  '.al-migrate', '#set-metrics-link',
  // The Proxy Hosts bulk bar issues ordinary PUTs, so it is a write control
  // like any other even though its buttons are styled ghost.
  '[data-bulk]',
].join(', ');

// One predicate for both read-only modes. An HA follower refuses every config
// write with a 503; a caller whose role is "user" is refused with a 403. The
// consequence for the UI is identical, so the disabling logic lives in one
// place and only the banner copy differs.
function isRoleReadOnly() { return !!(state.me && state.me.readOnly); }
function isReadOnly() { return isRoleReadOnly() || hasCapability('ha.readOnly'); }
function readOnlyReason() {
  if (isRoleReadOnly()) return 'Read-only: your role cannot make changes.';
  const role = (state.capabilities && state.capabilities.ha && state.capabilities.ha.role) || 'follower';
  return `This instance runs as an HA ${role} and is read-only. Make config changes on the leader.`;
}
function applyReadOnlyGating(container) {
  if (!container || !isReadOnly()) return;
  const reason = readOnlyReason();
  container.querySelectorAll(RO_WRITE_CONTROLS).forEach((el) => gateControl(el, false, reason));
  if (hasCapability('ha.readOnly')) {
    const role = (state.capabilities && state.capabilities.ha && state.capabilities.ha.role) || 'follower';
    const banner = document.createElement('div');
    banner.className = 'ro-banner';
    banner.innerHTML = `<b>Read-only (HA ${esc(role)}).</b> This instance does not accept config writes; make changes on the leader and they replicate here.`;
    container.prepend(banner);
  }
  if (isRoleReadOnly()) {
    const banner = document.createElement('div');
    banner.className = 'ro-banner';
    banner.innerHTML = '<b>Read-only access.</b> Your role is "user": you can view everything and change nothing.';
    container.prepend(banner);
  }
}

// The bootstrap failure state: no local admin credential AND no admin SSO
// provider that renders a sign-in button, so the login page cannot succeed for
// anyone. Same wording as the server-rendered login page.
function applyNoAdminLoginBanner(container) {
  if (!container || !state.capabilities || !state.capabilities.adminLogin) return;
  if (state.capabilities.adminLogin.configured !== false) return;
  const banner = document.createElement('div');
  banner.className = 'ro-banner warn';
  banner.innerHTML = '<b>No administrator login is configured.</b>'
    + '<ul><li>Set <span class="mono">GPM_LOCAL_ADMIN_USER</span> and <span class="mono">GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE</span> on the container, then restart it.</li>'
    + '<li>Or add an <span class="mono">oidc</span> identity provider and list its name under <span class="mono">adminAuth.providers</span> in <a href="#/settings/general">Settings</a>.</li></ul>'
    + '<pre class="mono">gpm hashpw \'your-password\' &gt; /run/secrets/gpm_admin_hash</pre>';
  container.prepend(banner);
}

// capabilities.adminLogin.cookieSecure is evaluated per request, not at
// startup: "insecure-public" means this session's cookie went out WITHOUT the
// Secure flag over plain HTTP to a client that is not on a private network, so
// anything on the path can read it. "insecure-private" (a LAN-only panel) and
// "secure" are normal and say nothing. Rendered on every route because the
// value can change without a restart - a new externalBaseURL, or the same panel
// reached from a different address.
function applyInsecureCookieBanner(container) {
  if (!container || !state.capabilities || !state.capabilities.adminLogin) return;
  if (state.capabilities.adminLogin.cookieSecure !== 'insecure-public') return;
  const banner = document.createElement('div');
  banner.className = 'ro-banner warn';
  banner.innerHTML = '<b>This admin session cookie is sent without the Secure flag.</b> '
    + 'It travels over plain HTTP from a public address, so it can be read in transit. '
    + 'Serve the panel over HTTPS, or set <a href="#/settings/general">External base URL</a> to an '
    + '<span class="mono">https://</span> URL, and sign in again. '
    + `<a href="${docHref('operations/hardening.md#admin-session-cookie')}" target="_blank" rel="noopener">Learn more</a>.`;
  container.prepend(banner);
}

// Re-evaluate every shell-level banner against the CURRENT capability probe and
// re-render it on the view already on screen. A Settings save changes what they
// say (fleet-wide maintenance, the HA role, the insecure-cookie state) but does
// not re-render the view, so without this the operator who just turned
// maintenance on sees nothing until they navigate away and back.
function refreshShellBanners() {
  const c = $('#content');
  if (!c) return;
  Array.from(c.children).forEach((el) => { if (el.classList && el.classList.contains('ro-banner')) el.remove(); });
  applyRefListGuard(c);
  applyMaintenanceBanner(c);
  applyReadOnlyGating(c);
  applyNoAdminLoginBanner(c);
  applyInsecureCookieBanner(c);
}

function loadingHtml(label) {
  return `<div class="loading"><span class="spinner"></span>${esc(label || 'Loading...')}</div>`;
}
function emptyState(title, sub, btnLabel, btnHref) {
  return `<div class="empty-state">
    <div class="es-title">${esc(title)}</div>
    <div class="es-sub">${esc(sub)}</div>
    ${btnLabel ? `<a class="btn primary" href="${esc(btnHref)}">${ICON.plus}${esc(btnLabel)}</a>` : ''}
  </div>`;
}
function inlineError(msg) {
  return `<div class="inline-error"><b>Could not load this view.</b><br/>${esc(msg)}</div>`;
}
function viewHead(title, sub, actionHtml) {
  return `<div class="row-between view-head">
    <div><h2>${esc(title)}</h2><p>${glossaryize(esc(sub))}</p></div>
    ${actionHtml || ''}
  </div>`;
}

// ---------- in-app help ----------
// Every form control carries data-hint="<id>"; the sentence and the docs anchor
// for that id live in internal/ui/hints/hints.json, served next to this file by
// internal/ui. internal/ui/hints_test.go keeps the two in step: every id used
// here exists in the registry, every registry entry is used by the UI, and every
// doc target resolves to a real anchor under docs/.
const DOCS_BASE = 'https://rake-pro.github.io/go-proxy-manager/';
let HINTS = {};
async function loadHints() {
  try {
    const r = await fetch('hints.json', { headers: { Accept: 'application/json' } });
    if (r.ok) HINTS = await r.json();
  } catch (e) { HINTS = {}; }
}
// "reference/config/proxy-host.md#x" -> "<base>/reference/config/proxy-host/#x".
// MkDocs serves a page at its path without the extension and with a trailing
// slash; index.md and README.md are the directory itself.
function docHref(doc) {
  const parts = String(doc || '').split('#');
  let p = parts[0].replace(/\.md$/, '').replace(/(^|\/)(index|README)$/, '$1');
  if (p && !p.endsWith('/')) p += '/';
  return DOCS_BASE + p + (parts[1] ? '#' + parts[1] : '');
}

// ---------- glossary ----------
// The terms docs/concepts/terminology.md defines, mapped to their registry id.
// A term in a page intro or a fold summary gets a dotted underline and its
// definition on hover or focus.
const GLOSSARY = {
  'identity provider': 'glossary.identity-provider',
  'access list': 'glossary.access-list',
  'apex target': 'glossary.apex-target',
  'middleware': 'glossary.middleware',
  'maintenance': 'glossary.maintenance',
  'reconcile': 'glossary.reconcile',
  'upstream': 'glossary.upstream',
  'location': 'glossary.location',
  'outpost': 'glossary.outpost',
  'ledger': 'glossary.ledger',
  'parked': 'glossary.parked',
};
const GLOSSARY_RE = new RegExp('\\b(' + Object.keys(GLOSSARY).join('|') + ')(s?)\\b', 'gi');
function glossaryTerm(word) {
  const id = GLOSSARY[String(word).toLowerCase()];
  if (!id || !HINTS[id]) return esc(word);
  return `<span class="gloss" data-hint="${esc(id)}" tabindex="0" role="button" aria-label="Glossary: ${esc(word)}">${esc(word)}</span>`;
}
// Runs over text that is ALREADY html-escaped (page intros, fold summaries), so
// the spans it injects are the only markup in the result. Only the first
// occurrence of each term is marked, so a summary does not turn into a rash of
// dotted underlines.
function glossaryize(escaped) {
  if (!escaped) return escaped;
  const seen = Object.create(null);
  return String(escaped).replace(GLOSSARY_RE, (m, term, plural) => {
    const key = term.toLowerCase();
    if (seen[key] || !GLOSSARY[key] || !HINTS[GLOSSARY[key]]) return m;
    seen[key] = true;
    return glossaryTerm(term) + plural;
  });
}

// ---------- the "?" affordance ----------
function hintHtml(id, label) {
  if (!HINTS[id]) return '';
  return `<button type="button" class="hint-btn" data-hint-id="${esc(id)}" aria-expanded="false"`
    + ` aria-label="Help for ${esc(label || id)}">?</button>`;
}
// A collapsible "About this page" block for a list view's intro.
function aboutPageHtml(id) {
  const e = HINTS[id];
  if (!e) return '';
  const bullets = Array.isArray(e.bullets) ? e.bullets : [];
  return `<details class="about-page" data-hint="${esc(id)}">
    <summary>About this page</summary>
    <p>${glossaryize(esc(e.text))}</p>
    ${bullets.length ? `<ul>${bullets.map((b) => `<li>${glossaryize(esc(b))}</li>`).join('')}</ul>` : ''}
    <a class="about-more" href="${esc(docHref(e.doc))}" target="_blank" rel="noopener">Learn more</a>
  </details>`;
}
// Where the "?" for one control goes: its field-group's label, or the name line
// of a toggle row. A control that lives in a repeater row has no label of its
// own - the row's columns are identified by aria-label - so it gets the hint as
// a native tooltip instead of a button that would triple the row's width.
function hintAnchorFor(el) {
  const fg = el.closest('.field-group');
  if (fg) {
    const lab = fg.querySelector(':scope > label');
    if (lab) return lab;
  }
  const tl = el.closest('.toggle-line');
  if (tl) {
    const nm = tl.querySelector('.tl-text .nm');
    if (nm) return nm;
  }
  // A repeater or check-list that IS the section (rules, sources, upstreams,
  // webhooks, the middleware chain) has no label of its own; its card's section
  // heading is the label an operator reads, so the "?" goes there.
  const card = el.closest('.card.form-section, details.fold');
  if (card) {
    const sl = card.querySelector('.section-label');
    if (sl) return sl;
  }
  return null;
}
let hintDecorating = false;
const hintObserver = (typeof MutationObserver === 'function')
  ? new MutationObserver(() => { if (!hintDecorating) decorateHints(document.getElementById('content')); })
  : null;
// Adds the "?" next to every labelled control that has a hint. Idempotent, and
// re-run on every DOM insertion so repeater rows added after render (locations,
// rules, upstreams, profiles) are decorated too.
function decorateHints(root) {
  if (!root) return;
  hintDecorating = true;
  try {
    root.querySelectorAll('[data-hint]').forEach((el) => {
      if (el.dataset.hinted === '1') return;
      el.dataset.hinted = '1';
      const id = el.getAttribute('data-hint');
      const entry = HINTS[id];
      if (!entry) return;
      if (el.classList.contains('gloss') || el.classList.contains('about-page')) return;
      const target = hintAnchorFor(el);
      if (!target) { if (!el.getAttribute('title')) el.setAttribute('title', entry.text); return; }
      if (target.querySelector('.hint-btn')) return;
      const label = (target.textContent || el.getAttribute('aria-label') || '').trim();
      target.insertAdjacentHTML('beforeend', hintHtml(id, label));
    });
  } finally {
    if (hintObserver) hintObserver.takeRecords();
    hintDecorating = false;
  }
}

// ---------- help popover ----------
// One popover element for the whole app: opening a second closes the first, and
// Escape or a click anywhere else dismisses it.
let hintPopOwner = null;
function hintPopEl() {
  let el = document.getElementById('hint-pop');
  if (!el) {
    el = document.createElement('div');
    el.id = 'hint-pop';
    el.className = 'hint-pop';
    el.setAttribute('role', 'dialog');
    el.hidden = true;
    document.body.appendChild(el);
  }
  return el;
}
function closeHintPop() {
  const el = document.getElementById('hint-pop');
  if (el) el.hidden = true;
  if (hintPopOwner) {
    if (hintPopOwner.classList.contains('hint-btn')) hintPopOwner.setAttribute('aria-expanded', 'false');
    hintPopOwner = null;
  }
}
function openHintPop(owner, id) {
  const e = HINTS[id];
  if (!e) return;
  const el = hintPopEl();
  const bullets = Array.isArray(e.bullets) ? e.bullets : [];
  el.innerHTML = `<p>${esc(e.text)}</p>`
    + (bullets.length ? `<ul>${bullets.map((b) => `<li>${esc(b)}</li>`).join('')}</ul>` : '')
    + (e.doc ? `<a href="${esc(docHref(e.doc))}" target="_blank" rel="noopener">Learn more</a>` : '');
  el.hidden = false;
  hintPopOwner = owner;
  if (owner.classList.contains('hint-btn')) owner.setAttribute('aria-expanded', 'true');
  const r = owner.getBoundingClientRect();
  const w = el.offsetWidth;
  let left = r.left + window.scrollX;
  const maxLeft = window.scrollX + document.documentElement.clientWidth - w - 12;
  if (left > maxLeft) left = Math.max(window.scrollX + 12, maxLeft);
  el.style.left = left + 'px';
  el.style.top = (r.bottom + window.scrollY + 6) + 'px';
}
document.addEventListener('click', (e) => {
  const btn = e.target.closest && e.target.closest('.hint-btn');
  if (btn) {
    e.preventDefault();
    e.stopPropagation();
    const id = btn.dataset.hintId;
    if (hintPopOwner === btn) closeHintPop(); else { closeHintPop(); openHintPop(btn, id); }
    return;
  }
  if (!(e.target.closest && e.target.closest('#hint-pop'))) closeHintPop();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeHintPop();
});
// Glossary terms are hover/focus, not click: they explain a word in a sentence
// rather than opening a panel about a control.
document.addEventListener('mouseover', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g && hintPopOwner !== g) { closeHintPop(); openHintPop(g, g.getAttribute('data-hint')); }
});
document.addEventListener('mouseout', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g && hintPopOwner === g) closeHintPop();
});
document.addEventListener('focusin', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g) { closeHintPop(); openHintPop(g, g.getAttribute('data-hint')); }
});
document.addEventListener('focusout', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g && hintPopOwner === g) closeHintPop();
});

// global switch toggle + keyboard
document.addEventListener('click', (e) => {
  const sw = e.target.closest && e.target.closest('.switch');
  if (sw && !sw.classList.contains('disabled') && sw.getAttribute('aria-disabled') !== 'true') {
    sw.setAttribute('aria-checked', sw.getAttribute('aria-checked') === 'true' ? 'false' : 'true');
    sw.dispatchEvent(new CustomEvent('switchchange', { bubbles: true }));
  }
});
document.addEventListener('keydown', (e) => {
  const t = e.target;
  if (t && t.classList && t.classList.contains('switch') && (e.key === ' ' || e.key === 'Enter')) {
    e.preventDefault();
    t.click();
  }
});

// chip-input controller. Returns {get()}; mutates a token list in DOM.
// onChange, when given, fires after every add/remove - used where another
// control is derived from the list (e.g. the certificate a host's domains
// resolve to).
function makeChipInput(container, initial, placeholder, onChange) {
  const tokens = (initial || []).slice();
  function changed() { if (onChange) onChange(tokens.slice()); }
  function render() {
    container.innerHTML = tokens.map((t, i) =>
      `<span class="chip-tok">${esc(t)} <button type="button" aria-label="Remove ${esc(t)}" data-i="${i}">&times;</button></span>`
    ).join('') + `<input class="mono" placeholder="${esc(placeholder || 'add...')}" aria-label="${esc(placeholder || 'add')}" />`;
    const input = container.querySelector('input');
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ',') {
        e.preventDefault();
        const v = input.value.trim().replace(/,$/, '');
        if (v && tokens.indexOf(v) === -1) { tokens.push(v); render(); container.querySelector('input').focus(); changed(); }
      } else if (e.key === 'Backspace' && !input.value && tokens.length) {
        tokens.pop(); render(); container.querySelector('input').focus(); changed();
      }
    });
    container.querySelectorAll('.chip-tok button').forEach((b) => {
      b.addEventListener('click', () => { tokens.splice(parseInt(b.dataset.i, 10), 1); render(); changed(); });
    });
  }
  render();
  return { get: () => tokens.slice() };
}

// ---------- shared value helpers ----------
// Relative time for the delivery/status chips. Deliberately coarse: these are
// "did it happen recently" readouts, and the absolute timestamp goes in the
// tooltip next to them.
function relTime(iso) {
  const d = new Date(iso);
  if (!iso || isNaN(d.getTime())) return '';
  const s = Math.round((Date.now() - d.getTime()) / 1000);
  if (s < 0) return 'just now';
  if (s < 60) return s <= 1 ? 'just now' : s + ' sec ago';
  const m = Math.round(s / 60);
  if (m < 60) return m + ' min ago';
  const h = Math.round(m / 60);
  if (h < 48) return h + ' h ago';
  return Math.round(h / 24) + ' days ago';
}

// isCidrOrIP mirrors the model's allowFrom/trustedProxies parser closely enough
// to catch a typo before the round trip. The server is authoritative; this only
// decides whether to block the save with the message the API would return.
function isCidrOrIP(v) {
  const s = String(v == null ? '' : v).trim();
  if (!s) return false;
  const slash = s.indexOf('/');
  const addr = slash === -1 ? s : s.slice(0, slash);
  if (slash !== -1) {
    const bits = s.slice(slash + 1);
    if (!/^\d{1,3}$/.test(bits)) return false;
    const n = parseInt(bits, 10);
    if (n > (addr.indexOf(':') !== -1 ? 128 : 32)) return false;
  }
  if (addr.indexOf(':') !== -1) return /^[0-9A-Fa-f:.]+$/.test(addr);
  const parts = addr.split('.');
  if (parts.length !== 4) return false;
  return parts.every((p) => /^\d{1,3}$/.test(p) && parseInt(p, 10) <= 255);
}
// The first entry in list that is not a CIDR or bare IP, or ''.
function firstBadCidr(list) {
  for (const v of arr(list)) { if (!isCidrOrIP(v)) return v; }
  return '';
}
// True when the list trusts every peer, which makes every IP rule spoofable.
function hasWildcardProxy(list) {
  return arr(list).some((v) => v === '0.0.0.0/0' || v === '::/0');
}
// Warning, not an error: a wildcard is a legitimate (if bad) configuration, so
// it is surfaced permanently in the card and the save stays enabled.
const TRUSTED_WILDCARD_WARNING = '0.0.0.0/0 trusts every peer, so any client can set its own address and every IP rule below becomes spoofable. List your proxies\' real addresses instead.';

// A hostname, optionally with a port - what upstream.hostHeader accepts.
const HOSTHEADER_RE = /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:\d{1,5})?$/;

// upstreamPathError mirrors the server's checks on upstream.path. Order matters
// only for which message an operator sees first.
function upstreamPathError(p) {
  const s = String(p || '').trim();
  if (!s) return '';
  if (s[0] !== '/') return 'Base path must be absolute (start with "/").';
  if (s.indexOf('?') !== -1 || s.indexOf('#') !== -1) return 'Base path must not contain a query string or fragment.';
  if (s.indexOf('\\') !== -1 || s.indexOf(';') !== -1) return 'Base path must not contain "\\" or ";".';
  if (s.split('/').some((seg) => seg === '.' || seg === '..')) return 'Base path must not contain "." or ".." segments.';
  return '';
}

// ---------- delivery status (webhooks and notification targets) ----------
// GET /api/webhooks/status and GET /api/notifications/status answer the same
// shape (internal/notify reuses webhook.Delivery), so one renderer serves both.
// The state is in-memory and per-process; the copy under each repeater says so.
function deliveryChipHtml(st, disabled) {
  const pre = disabled ? 'Disabled - ' : '';
  if (!st || !st.lastAttempt) {
    return `<span class="chip" title="This target has not fired since gpm started.">${esc(pre)}Never delivered</span>`;
  }
  const when = relTime(st.lastAttempt);
  const tip = [st.error || '', fmtTime(st.lastAttempt)].filter(Boolean).join(' - ');
  let cls = st.ok ? 'ok' : 'err';
  let text;
  if (st.ok) text = `OK ${st.status} - ${st.durationMs} ms - ${when}`;
  else if (st.status) text = `HTTP ${st.status} - ${when}`;
  else text = `Failed - ${when}`;
  if (disabled) cls = '';
  return `<span class="chip ${cls}" title="${esc(tip)}">${esc(pre + text)}</span>`;
}

// ---------- basic-auth credential rows ----------
// Shared by the auth middleware editor and every inline auth block (host and
// location). The UI posts the PLAINTEXT password; the API bcrypts it and stores
// only passwordHash, so nothing here ever hashes and nothing ever renders a
// stored hash into a visible input.
const BASIC_MAX_USERS = 64;
function makeBasicUserRows(wrap, users) {
  function addRow(u) {
    u = u || {};
    const existing = !!u.passwordHash;
    const div = document.createElement('div');
    div.className = 'loc-row';
    div._hash = u.passwordHash || '';
    div._new = !existing;
    div.innerHTML = `<input class="field mono bu-user" data-hint="middleware.auth.basic.users.username" style="flex:1 1 130px" value="${esc(u.username || '')}" placeholder="username" aria-label="Username" />
      <input class="field mono bu-pw" data-hint="middleware.auth.basic.users.password" type="password" autocomplete="new-password" style="flex:2 1 170px" placeholder="${existing ? 'Unchanged' : 'Set a password'}" aria-label="Password" />
      ${existing ? '<span class="chip ok" title="A password is set for this user. Leave the field blank to keep it.">set</span>' : ''}
      <button class="icon-btn bu-del" type="button" aria-label="Remove user">${ICON.x}</button>`;
    div.querySelector('.bu-del').addEventListener('click', () => { div.remove(); sync(); });
    wrap.appendChild(div);
    sync();
    return div;
  }
  function rows() { return Array.from(wrap.querySelectorAll(':scope > .loc-row')); }
  // At least one user is required and at most 64, so the controls that would
  // break either bound are disabled rather than left to the API to refuse.
  function sync() {
    const rs = rows();
    rs.forEach((r) => { r.querySelector('.bu-del').disabled = rs.length <= 1; });
    const add = wrap.parentElement && wrap.parentElement.querySelector('.bu-add');
    if (add) gateControl(add, rs.length < BASIC_MAX_USERS, `At most ${BASIC_MAX_USERS} users.`);
  }
  arr(users).forEach(addRow);
  if (!rows().length) addRow({});
  return {
    addRow, sync,
    // Positional: the API applies password to users[i], so the array is sent in
    // exactly the order it was built.
    get() {
      return rows().map((r) => {
        const u = { username: r.querySelector('.bu-user').value.trim() };
        const pw = r.querySelector('.bu-pw').value;
        if (pw) u.password = pw; else if (r._hash) u.passwordHash = r._hash;
        return u;
      });
    },
    error() {
      clearRowErrors(wrap);
      const seen = Object.create(null);
      let err = '';
      rows().forEach((r) => {
        if (err) return;
        const name = r.querySelector('.bu-user').value.trim();
        const pw = r.querySelector('.bu-pw').value;
        if (!name) { err = markRowError(r, 'Every credential needs a username.'); return; }
        if (name.indexOf(':') !== -1 || /[\r\n]/.test(name)) { err = markRowError(r, 'A username cannot contain ":" or a line break.'); return; }
        if (seen[name]) { err = markRowError(r, `Duplicate username "${name}".`); return; }
        seen[name] = true;
        if (!pw && !r._hash) { err = markRowError(r, 'A password is required for a new user.'); return; }
      });
      if (!err && !rows().length) err = 'Add at least one user.';
      return err;
    },
  };
}

// ---------- inline auth block (host, location, middleware) ----------
// One implementation of the auth editor, rendered in three places: the auth
// middleware, a proxy host's inline `auth`, and a location's inline `auth`. The
// JSON shape is identical (model.AuthMiddleware), so the control is too - only
// the surrounding fold and its help text differ.
const AUTH_MODES = ['', 'oidc', 'forward-auth', 'auth-request', 'client-cert', 'basic'];
function authBlockHtml(p, auth, idps, opts) {
  auth = auth || {};
  opts = opts || {};
  const mode = auth.mode || '';
  const idpKnown = idps.some((x) => x.name === auth.identityProvider);
  const basic = auth.basic || {};
  return `
    <div class="field-group"><label>Identity provider</label>
      <select class="field mono" id="${p}-idp" data-hint="middleware.auth.identityProvider" data-path="${esc(opts.path || 'auth')}.identityProvider">
        <option value="">select provider...</option>
        ${auth.identityProvider && !idpKnown ? `<option value="${esc(auth.identityProvider)}" selected>${esc(auth.identityProvider)} (not found)</option>` : ''}
        ${idps.map((x) => `<option value="${esc(x.name)}"${auth.identityProvider === x.name ? ' selected' : ''}>${esc(x.name)}${x.type ? ' (' + esc(x.type) + ')' : ''}</option>`).join('')}
      </select>
      <div class="hint" id="${p}-idp-hint">${idps.length ? 'Where people are sent to prove who they are.' : 'No identity providers yet. Add one under <a href="#/identity">Identity Providers</a>.'}</div>
    </div>
    <div class="field-group"><label>Mode</label>
      <select class="field mono" id="${p}-mode" data-hint="middleware.auth.mode" data-path="${esc(opts.path || 'auth')}.mode">${enumOptions('authMode', AUTH_MODES, mode)}</select>
      <div class="hint" id="${p}-mode-hint"></div>
    </div>
    <div class="field-group" id="${p}-roles-wrap"><label>Required roles</label>
      <div class="chip-input" id="${p}-roles" data-hint="middleware.auth.requiredRoles"></div>
      <div class="hint">Roles the signed-in identity must carry. Not supported in auth-request or basic mode.</div>
    </div>
    <div class="field-group" id="${p}-allow-wrap"><label>Exempt networks</label>
      <div class="chip-input" id="${p}-allow" data-hint="middleware.auth.allowFrom"></div>
      <div class="hint">CIDRs or bare IPs that skip this gate. Matched against the client IP this host resolves - <span class="mono">X-Forwarded-For</span> is honoured only from this host's own trusted proxies.</div>
    </div>
    <div class="field-group" id="${p}-certroles-wrap"><label>Certificate subject -&gt; role</label>
      <div id="${p}-certroles" data-hint="middleware.auth.clientCertRoles"></div>
      <button class="btn ghost sm" id="${p}-certroles-add" type="button" style="margin-top:6px">${ICON.plus}Add</button>
      <div class="hint">Key: the certificate subject (<span class="mono">CN=ops,O=Corp</span>) or its bare common name. Leave empty to admit any verified certificate.</div>
    </div>
    <div class="field-group" id="${p}-basic-wrap">
      <p class="section-label" style="margin-top:4px">Credentials</p>
      <div class="field-group"><label>Realm</label>
        <input class="field mono" id="${p}-realm" data-hint="middleware.auth.basic.realm" value="${esc(basic.realm || '')}" placeholder="the host name" />
        <div class="hint">Shown in the browser's password prompt. Defaults to the host name.</div>
      </div>
      <label>Users</label>
      <div id="${p}-users" data-hint="middleware.auth.basic.users"></div>
      <button class="btn ghost sm bu-add" id="${p}-users-add" type="button" style="margin-top:6px">${ICON.plus}Add user</button>
      <div class="hint" style="margin-top:6px">The password is sent in plaintext over this session and hashed with bcrypt server-side; gpm stores only the hash and can never show it again.</div>
    </div>`;
}
const AUTH_MODE_HINTS = {
  '': 'The identity provider\'s own type decides how sign-in happens.',
  'oidc': 'gpm redirects to the provider and keeps the session cookie itself.',
  'forward-auth': 'Identity is read from headers asserted by a trusted proxy in front of gpm.',
  'auth-request': 'Each request is authorized by a subrequest to an external outpost, which does the authorization too.',
  'client-cert': 'The TLS handshake is the identity source. Pair with TLS &gt; Client certificates (mTLS) in optional mode so certless clients still reach this gate.',
  'basic': 'Checks a username and password against the credentials below. No identity provider is involved.',
};
function wireAuthBlock(p, auth, idps) {
  auth = auth || {};
  const rolesCtl = makeChipInput($('#' + p + '-roles'), arr(auth.requiredRoles), 'add role...');
  const allowCtl = makeChipInput($('#' + p + '-allow'), arr(auth.allowFrom), 'add CIDR...');
  const certRolesCtl = makeKVRows($('#' + p + '-certroles'), auth.clientCertRoles || {}, 'CN=ops,O=Corp', 'role', false);
  $('#' + p + '-certroles-add').addEventListener('click', () => certRolesCtl.addRow('', ''));
  const usersCtl = makeBasicUserRows($('#' + p + '-users'), (auth.basic && auth.basic.users) || []);
  $('#' + p + '-users-add').addEventListener('click', () => usersCtl.addRow({}));
  const modeSel = $('#' + p + '-mode');
  const idpSel = $('#' + p + '-idp');
  // Mode-driven visibility is the point of the block: a field the backend would
  // reject is hidden, and the one control that stays present but unusable (the
  // provider select in client-cert / basic mode) is greyed with the reason.
  function sync() {
    const m = modeSel.value;
    $('#' + p + '-mode-hint').innerHTML = AUTH_MODE_HINTS[m] || '';
    const noIdP = m === 'client-cert' || m === 'basic';
    gateControl(idpSel, !noIdP, m === 'basic'
      ? 'Basic mode checks the credentials below, so no identity provider applies.'
      : 'The TLS handshake is the identity source in client-cert mode.');
    $('#' + p + '-roles-wrap').hidden = m === 'auth-request' || m === 'basic';
    $('#' + p + '-allow-wrap').hidden = !(m === 'auth-request' || m === 'client-cert' || m === 'basic');
    $('#' + p + '-certroles-wrap').hidden = m !== 'client-cert';
    $('#' + p + '-basic-wrap').hidden = m !== 'basic';
    if (m === 'basic') usersCtl.sync();
  }
  modeSel.addEventListener('change', sync);
  sync();
  return {
    mode() { return modeSel.value; },
    // Returns the auth spec, or null after showing the first blocking message.
    get(label) {
      const m = modeSel.value;
      const idp = (m === 'client-cert' || m === 'basic') ? '' : idpSel.value;
      const roles = rolesCtl.get();
      const allow = allowCtl.get();
      const certRoles = m === 'client-cert' ? certRolesCtl.get() : {};
      const where = label ? label + ': ' : '';
      if (m !== 'client-cert' && m !== 'basic' && !idp) {
        toast('Identity provider required', where + 'Select an identity provider.', 'err'); return null;
      }
      if (m === 'auth-request' && roles.length) {
        toast('Roles unsupported', where + 'Required roles are not supported in auth-request mode - the auth server does authorization.', 'err'); return null;
      }
      if (m === 'client-cert' && roles.length && !Object.keys(certRoles).length) {
        toast('Mapping required', where + 'Add at least one certificate subject role, or clear the required roles.', 'err'); return null;
      }
      if (allow.length) {
        const bad = firstBadCidr(allow);
        if (bad) { toast('Invalid network', where + `"${bad}" is not a CIDR or IP address.`, 'err'); return null; }
        if (m === 'oidc' || m === 'forward-auth') {
          toast('Exemption not applicable', where + 'Exempt networks apply only to auth-request, client-cert and basic modes.', 'err'); return null;
        }
        if (!m) {
          const t = (idps.find((x) => x.name === idp) || {}).type || '';
          if (t !== 'auth-request') {
            toast('Set a mode explicitly', where + `With no mode this uses ${idp}'s type (${t || 'unknown'}), where the exemption would be ignored.`, 'err'); return null;
          }
        }
      }
      const spec = {};
      if (idp) spec.identityProvider = idp;
      if (m) spec.mode = m;
      if (roles.length) spec.requiredRoles = roles;
      if (allow.length) spec.allowFrom = allow;
      if (Object.keys(certRoles).length) spec.clientCertRoles = certRoles;
      if (m === 'basic') {
        const uerr = usersCtl.error();
        if (uerr) { toast('Credentials incomplete', where + uerr, 'err'); return null; }
        const realm = $('#' + p + '-realm').value.trim();
        if (realm && (realm.length > 128 || /["\\]/.test(realm) || /[^\x20-\x7e]/.test(realm))) {
          toast('Invalid realm', where + 'Realm must be printable ASCII without " or \\ (it is sent verbatim in the WWW-Authenticate header).', 'err'); return null;
        }
        const b = { users: usersCtl.get() };
        if (realm) b.realm = realm;
        spec.basic = b;
      }
      return spec;
    },
  };
}

// ---------- inline rate-limit block (host, location) ----------
// model.RateLimitMiddleware accepts EITHER requestsPerSecond OR requests+window,
// never both, so the form is a radio pair and only the selected side is sent.
const RL_WINDOW_PRESETS = ['1s', '10s', '30s', '1m', '5m', '15m', '1h'];
function rateLimitBlockHtml(p, rl) {
  rl = rl || {};
  const perSecond = rl.requestsPerSecond > 0;
  const windows = RL_WINDOW_PRESETS.slice();
  const win = rl.window || '1m';
  if (!windows.includes(win)) windows.unshift(win);
  return `
    <div class="field-group"><label>Rate form</label>
      <div class="seg" data-hint="middleware.rateLimit.form">
        <button type="button" class="seg-btn${perSecond ? '' : ' on'}" id="${p}-rf-win">Per window</button>
        <button type="button" class="seg-btn${perSecond ? ' on' : ''}" id="${p}-rf-sec">Per second</button>
      </div>
    </div>
    <div class="inline-fields" id="${p}-win-fields"${perSecond ? ' hidden' : ''}>
      <div class="field-group"><label>Requests</label><input class="field mono" id="${p}-req" data-hint="middleware.rateLimit.requests" type="number" step="0.1" min="0" value="${esc(rl.requests != null && rl.requests > 0 ? rl.requests : '')}" placeholder="100" /></div>
      <div class="field-group"><label>Window</label><input class="field mono" id="${p}-win" data-hint="middleware.rateLimit.window" value="${esc(win)}" placeholder="1m" /><div class="hint">Go duration: <span class="mono">10s</span>, <span class="mono">1m</span>, <span class="mono">1h</span>.</div></div>
    </div>
    <div class="inline-fields" id="${p}-sec-fields"${perSecond ? '' : ' hidden'}>
      <div class="field-group"><label>Requests per second</label><input class="field mono" id="${p}-rps" data-hint="middleware.rateLimit.requestsPerSecond" type="number" step="0.1" min="0" value="${esc(rl.requestsPerSecond > 0 ? rl.requestsPerSecond : '')}" placeholder="5" /></div>
    </div>
    <div class="inline-fields">
      <div class="field-group"><label>Burst</label><input class="field mono" id="${p}-burst" data-hint="middleware.rateLimit.burst" type="number" min="0" value="${esc(rl.burst > 0 ? rl.burst : '')}" placeholder="ceil(requests)" /></div>
      <div class="field-group"><label>Block for</label><input class="field mono" id="${p}-block" data-hint="middleware.rateLimit.blockFor" value="${esc(rl.blockFor || '')}" placeholder="10m (optional)" /></div>
    </div>
    <div class="field-group"><label>Exempt networks</label><div class="chip-input" id="${p}-allow" data-hint="middleware.rateLimit.allowFrom"></div>
      <div class="hint">CIDRs or bare IPs that bypass the limit entirely.</div>
    </div>
    <div class="hint">Once a client exceeds the limit, Block for refuses it for that long regardless of token refill.</div>`;
}
const GO_DURATION_RE = /^(\d+(\.\d+)?(ns|us|ms|s|m|h))+$/;
function wireRateLimitBlock(p, rl) {
  rl = rl || {};
  const allowCtl = makeChipInput($('#' + p + '-allow'), arr(rl.allowFrom), 'add CIDR...');
  const winBtn = $('#' + p + '-rf-win');
  const secBtn = $('#' + p + '-rf-sec');
  function pick(perSecond) {
    winBtn.classList.toggle('on', !perSecond);
    secBtn.classList.toggle('on', perSecond);
    $('#' + p + '-win-fields').hidden = perSecond;
    $('#' + p + '-sec-fields').hidden = !perSecond;
  }
  winBtn.addEventListener('click', () => pick(false));
  secBtn.addEventListener('click', () => pick(true));
  return {
    get(label) {
      const where = label ? label + ': ' : '';
      const perSecond = secBtn.classList.contains('on');
      const spec = {};
      if (perSecond) {
        const rps = parseFloat($('#' + p + '-rps').value);
        if (isNaN(rps) || rps <= 0) { toast('Rate required', where + 'Set a rate: requests per second, or requests plus a window.', 'err'); return null; }
        spec.requestsPerSecond = rps;
      } else {
        const req = parseFloat($('#' + p + '-req').value);
        const win = $('#' + p + '-win').value.trim();
        if (isNaN(req) || req <= 0) { toast('Rate required', where + 'Set a rate: requests per second, or requests plus a window.', 'err'); return null; }
        if (!GO_DURATION_RE.test(win)) { toast('Invalid window', where + 'Window must be a duration such as 10s, 1m or 1h.', 'err'); return null; }
        spec.requests = req;
        spec.window = win;
      }
      const burst = parseInt($('#' + p + '-burst').value, 10);
      if (!isNaN(burst) && burst > 0) spec.burst = burst;
      const allow = allowCtl.get();
      if (allow.length) {
        const bad = firstBadCidr(allow);
        if (bad) { toast('Invalid network', where + `"${bad}" is not a CIDR or IP address.`, 'err'); return null; }
        spec.allowFrom = allow;
      }
      const block = $('#' + p + '-block').value.trim();
      if (block) {
        if (!GO_DURATION_RE.test(block)) { toast('Invalid block duration', where + 'Block for must be a duration greater than zero, such as 10m.', 'err'); return null; }
        spec.blockFor = block;
      }
      return spec;
    },
  };
}

// ---------- inline auth / rate-limit folds ----------
// The fold wrapper both inline blocks share: an on/off switch that owns the
// whole key (off omits it from the body entirely), a one-line summary of what
// the block currently holds, and the block itself. Used by the proxy-host
// editor and by every location row.
function inlineFoldHtml(p, title, help, on, summary, body, hintId) {
  return foldHtml(p + '-card', title, summary, on, `
    <div class="toggle-line">
      <div class="tl-text"><div class="nm">${esc(title)}</div><div class="ds">${help}</div></div>
      ${switchHtml(p + '-on', on, title, hintId)}
    </div>
    <div id="${p}-fields" style="margin-top:10px;${on ? '' : 'display:none'}">${body}</div>`);
}
function wireInlineFold(p) {
  const sw = document.getElementById(p + '-on');
  if (!sw) return;
  sw.addEventListener('switchchange', () => {
    const f = document.getElementById(p + '-fields');
    if (f) f.style.display = isOn(p + '-on') ? '' : 'none';
    const card = document.getElementById(p + '-card');
    if (card && isOn(p + '-on')) card.open = true;
  });
}
function authFoldSummary(auth) {
  if (!auth) return 'off - this host is not gated by a sign-in';
  const parts = [];
  if (auth.mode) parts.push(enumLabel('authMode', auth.mode).split(' - ')[0]);
  if (auth.identityProvider) parts.push('via ' + auth.identityProvider);
  if (arr(auth.requiredRoles).length) parts.push(arr(auth.requiredRoles).length + ' required role(s)');
  return parts.length ? parts.join(', ') : 'on';
}
function rateFoldSummary(rl) {
  if (!rl) return 'off - no throttle';
  if (rl.requestsPerSecond > 0) return rl.requestsPerSecond + ' req/s';
  if (rl.requests > 0) return `${rl.requests} per ${rl.window || '1m'}`;
  return 'on';
}

// ---------- upstream escape hatches ----------
// upstream.path and upstream.hostHeader, rendered wherever a scheme/host/port
// group is. Both are optional and omitempty: the key is dropped entirely when
// unset, so a host that never used them round-trips byte-identically.
function upstreamExtraHtml(p, up) {
  up = up || {};
  const hh = up.hostHeader || '';
  const preset = (hh === '' || hh === 'upstream') ? hh : 'custom';
  return `
    <div class="inline-fields" style="margin-top:8px">
      <div class="field-group"><label>Base path</label>
        <input class="field mono" id="${p}-uppath" data-hint="proxyHost.upstream.path" value="${esc(up.path || '')}" placeholder="/api" />
        <div class="hint">Prefixed to every request sent to this backend: with <span class="mono">/api</span>, a request for <span class="mono">/v1/x</span> arrives as <span class="mono">/api/v1/x</span>. Leave empty to forward the path unchanged.</div>
      </div>
      <div class="field-group"><label>Host header</label>
        <select class="field mono" id="${p}-uphh" data-hint="proxyHost.upstream.hostHeader">${enumOptions('hostHeader', ['', 'upstream', 'custom'], preset)}</select>
        <div class="hint" id="${p}-uphh-hint"></div>
      </div>
      <div class="field-group" id="${p}-uphh-custom-wrap"${preset === 'custom' ? '' : ' hidden'}><label>Custom Host header</label>
        <input class="field mono" id="${p}-uphh-custom" data-hint="proxyHost.upstream.hostHeaderCustom" value="${esc(preset === 'custom' ? hh : '')}" placeholder="backend.example.com" />
      </div>
    </div>`;
}
const HOSTHEADER_HINTS = {
  '': 'The backend sees the hostname the client asked for. This is the default and suits almost every backend.',
  'upstream': 'The backend sees its own address (host:port). Use it for a backend that keys its virtual host off its own address.',
  'custom': 'The backend sees exactly this hostname. Must be a hostname, optionally with a port.',
};
function wireUpstreamExtra(p) {
  const sel = $('#' + p + '-uphh');
  if (!sel) return { get() { return {}; } };
  function sync() {
    $('#' + p + '-uphh-hint').textContent = HOSTHEADER_HINTS[sel.value] || '';
    const custom = sel.value === 'custom';
    $('#' + p + '-uphh-custom-wrap').hidden = !custom;
    if (!custom) $('#' + p + '-uphh-custom').value = '';
  }
  sel.addEventListener('change', sync);
  sync();
  return {
    // Returns {path?, hostHeader?} to merge onto an upstream object, or null
    // after toasting the first problem.
    get(label) {
      const where = label ? label + ': ' : '';
      const out = {};
      const path = $('#' + p + '-uppath').value.trim();
      const perr = upstreamPathError(path);
      if (perr) { toast('Invalid base path', where + perr, 'err'); return null; }
      if (path) out.path = path;
      if (sel.value === 'upstream') out.hostHeader = 'upstream';
      else if (sel.value === 'custom') {
        const v = $('#' + p + '-uphh-custom').value.trim();
        if (!v) { toast('Host header required', where + 'Enter a hostname, or choose another option.', 'err'); return null; }
        if (v.length > 253 || !HOSTHEADER_RE.test(v)) { toast('Invalid host header', where + 'Host header must be a hostname, optionally "host:port".', 'err'); return null; }
        out.hostHeader = v;
      }
      return out;
    },
  };
}

// ---------- certificate health ----------
// GET /api/certificates decorates every certificate with the material's own
// state (notAfter, daysRemaining, issuer, sans, state, lastError). Absent - not
// null - on a pending or failed ACME certificate that has never been issued, so
// every read is guarded.
// Short relative label for the chip text - "in 84 days", "expired 3 days ago",
// "pending", "error" - never the absolute timestamp: that used to be the chip
// text itself, which wrapped onto two lines inside a tall pill for anything
// with a time-of-day component. The absolute date, if a caller wants it, is a
// separate plain-text element next to the chip (see certExpiryCellHtml).
function certExpiryLabel(state, days) {
  if (state === 'pending') return 'pending';
  if (state === 'error') return 'error';
  if (days == null) return '';
  if (days >= 0) return `in ${days} day${days === 1 ? '' : 's'}`;
  const ago = -days;
  return `expired ${ago} day${ago === 1 ? '' : 's'} ago`;
}
// valid/pending render as a small coloured dot plus text (no border or fill),
// so a healthy or not-yet-issued certificate stays quiet; only expiring,
// expired and error states get the filled/bordered pill, so attention goes
// where it matters. The state word is always in the text too, never colour
// alone.
function certStateChip(ct) {
  ct = ct || {};
  if (!ct.state) return '<span class="faint" title="This build of the API does not report certificate state.">-</span>';
  const state = ct.state;
  const cls = { valid: 'ok', expiring: 'warn', expired: 'err', pending: 'muted', error: 'err' }[state] || '';
  const flat = state === 'valid' || state === 'pending';
  const days = ct.daysRemaining;
  const label = certExpiryLabel(state, days);
  const titleBits = [];
  if (state === 'error' || state === 'pending') {
    if (ct.lastError) titleBits.push(ct.lastError);
  } else if (days != null) {
    titleBits.push(days >= 0 ? `${days} days remaining` : `Expired ${-days} days ago`);
  }
  if (ct.notAfter) titleBits.push(fmtTime(ct.notAfter));
  const title = titleBits.join('; ');
  const chipCls = flat ? `chip flat${cls ? ' ' + cls : ''}` : `chip ${cls}`;
  return `<span class="${chipCls}" title="${esc(title)}"><span class="dot ${cls}"></span>${esc(label)}</span>`;
}
// Expiry column cell: the state chip plus the plain absolute date beside it
// (no time - the full timestamp is the chip's tooltip already). Used only
// where there is room for both; certStateChip alone covers every other spot
// (status card, overview widget) where the absolute date has its own field.
function certExpiryCellHtml(ct) {
  ct = ct || {};
  const date = ct.notAfter ? fmtDate(ct.notAfter) : '';
  return `<span class="cert-expiry">${certStateChip(ct)}${date ? `<span class="expiry-date" title="${esc(fmtTime(ct.notAfter))}">${esc(date)}</span>` : ''}</span>`;
}
// POST /api/certificates/{name}/renew starts an order and returns immediately -
// DNS-01 propagation alone can take minutes, so there is no spinner and no poll:
// the toast says it started and the next natural refresh picks up the result.
async function renewCertificate(name) {
  const ok = await confirmModal({
    title: 'Renew this certificate now?',
    body: `<p>Forces an immediate renewal of <b>${esc(name)}</b>. This starts a new ACME order now, ignoring the 30-day schedule.</p>`
      + '<p>The order runs in the background; DNS-01 validation can take several minutes.</p>',
    confirmLabel: 'Renew now',
    danger: false,
  });
  if (!ok) return false;
  try {
    await api('/api/certificates/' + encodeURIComponent(name) + '/renew', { method: 'POST' });
    toast('Renewal started', `${name} is being renewed. This can take a few minutes for DNS-01.`, 'ok');
    return true;
  } catch (e) {
    if (e && e.status === 409) {
      toast('Renewal already running', 'Another certificate order is already in progress on this instance. Wait for it to finish and try again.', 'err');
    } else {
      toastErr(e);
    }
    return false;
  }
}

// ---------- clone ----------
// Kinds whose objects carry a flat .domains list that collides across the
// store (proxy/redirect/parked hosts all share one domain namespace) - cleared
// on clone so the copy doesn't fail the duplicate-domain check on save.
const CLONE_CLEAR_DOMAINS = new Set(['hosts', 'redirects', 'parked']);

// Recursively blanks any string value the API masked as "***" - the exact
// sentinel model.Secret.MarshalJSON uses for a literal (non-placeholder)
// secret, and nothing else ever produces. Structural, not a per-kind
// field-path list, so it stays correct as editors grow new Secret-typed
// fields without this needing an update. Deliberately narrower than "any
// ${ENV:...}/${FILE:...}-looking string": several non-secret fields (e.g. a
// client CA's caPEM) legitimately use that same placeholder syntax to load
// public material from a file, and must survive a clone untouched.
function stripSecrets(v) {
  if (Array.isArray(v)) { v.forEach(stripSecrets); return; }
  if (v && typeof v === 'object') {
    Object.keys(v).forEach((k) => {
      const val = v[k];
      if (val === '***') v[k] = '';
      else stripSecrets(val);
    });
  }
}
// Deep-copies a stored object for the "Clone" action (JSON round-trip - none of
// these carry functions/undefined), blanks its name so the editor prompts for a
// new one, strips every secret-bearing field, and clears .domains for the kinds
// that need it. tags and disabled are left as-is (both should survive a clone).
function cloneObject(obj, clearDomains) {
  const copy = JSON.parse(JSON.stringify(obj || {}));
  copy.name = '';
  if (clearDomains) copy.domains = [];
  stripSecrets(copy);
  return copy;
}
// Stashes a clone seed and navigates to that section's "new" editor, which
// picks it up via takeCloneSeed instead of starting from a blank object.
let cloneSeed = null;
function startClone(section, obj) {
  cloneSeed = { section, origName: (obj && obj.name) || '', data: cloneObject(obj, CLONE_CLEAR_DOMAINS.has(section)) };
  location.hash = '#/' + section + '/_new';
}
// Consumes (one-shot) the pending clone seed for section, or returns null.
function takeCloneSeed(section) {
  if (cloneSeed && cloneSeed.section === section) {
    const seed = cloneSeed;
    cloneSeed = null;
    return seed;
  }
  return null;
}
// Wires the editor's optional #ed-clone / btnId save-bar button (rendered only
// for an existing object) to clone the object currently loaded in that editor.
function wireCloneButton(section, orig, btnId) {
  const btn = $('#' + (btnId || 'ed-clone'));
  if (btn) btn.addEventListener('click', () => startClone(section, orig));
}

// ---------- router ----------
async function route() {
  const raw = location.hash.replace(/^#/, '');
  const parts = raw.split('/').filter(Boolean);
  if (!parts.length) { location.hash = '#/overview'; return; }
  const section = parts[0];
  const sub = parts[1] ? decodeURIComponent(parts[1]) : null;

  setActiveNav(section);
  $('#pageTitle').textContent = TITLES[section] || state.appName;
  const c = $('#content');
  c.innerHTML = loadingHtml();
  window.scrollTo(0, 0);
  // Per-render bookkeeping: the shell and the view share one memo for the three
  // GETs they both want, and each render starts with a clean reference-list
  // failure list so a banner never outlives the view that produced it.
  resetRouteMemo();
  resetRefListFailures();
  // Capabilities gate the maintenance banner, the read-only banner and several
  // controls, and a Settings save drops the cache. Only some views reloaded it,
  // so on every other route every capability read as false right after a save -
  // fleet-wide maintenance was invisible on the hosts list the moment it was
  // turned on. Refresh here, once, before anything reads it.
  await loadCapabilities();

  try {
    switch (section) {
      case 'overview': await viewOverview(c); break;
      case 'hosts':
        if (sub === '_new') await hostEditor(c, null);
        else if (sub) await hostEditor(c, sub);
        else await listHosts(c);
        break;
      case 'certs':
        if (sub === '_new') await certEditor(c, null);
        else if (sub) await certEditor(c, sub);
        else await listCerts(c);
        break;
      case 'clientcas': await genericSection(c, 'clientcas', sub); break;
      case 'identity': await genericSection(c, 'identity', sub); break;
      case 'access': await genericSection(c, 'access', sub); break;
      case 'middleware': await genericSection(c, 'middleware', sub); break;
      case 'upstreams': await genericSection(c, 'upstreams', sub); break;
      case 'dns': await genericSection(c, 'dns', sub); break;
      case 'redirects': await genericSection(c, 'redirects', sub); break;
      case 'streams': await genericSection(c, 'streams', sub); break;
      case 'parked': await genericSection(c, 'parked', sub); break;
      case 'errorpages': await viewErrorPages(c); break;
      case 'integrations': await viewIntegrations(c); break;
      case 'tokens': await viewTokens(c); break;
      case 'logs': await viewLogs(c); break;
      case 'history': await viewHistory(c); break;
      case 'settings': await viewSettings(c, sub); break;
      default: location.hash = '#/overview'; return;
    }
    c.classList.add('view');
    applyRefListGuard(c);
    applyMaintenanceBanner(c);
    applyReadOnlyGating(c);
    applyNoAdminLoginBanner(c);
    applyInsecureCookieBanner(c);
    armDirtyTracking(c);
    decorateHints(c);
    if (hintObserver) hintObserver.observe(c, { childList: true, subtree: true });
  } catch (e) {
    if (e && e.message === 'Unauthorized') return;
    c.innerHTML = inlineError(e && e.message ? e.message : String(e));
  }
}
function setActiveNav(section) {
  $$('#nav .nav-item').forEach((n) => n.classList.toggle('active', n.dataset.view === section));
  // A deep link (or the back button) into a collapsed group opens it, so the
  // active item is never highlighted inside a section the operator cannot see.
  // Not persisted: this is the app following a link, not a preference.
  const group = NAV_GROUPS.find((g) => g.collapsible && g.items.some((n) => n.id === section));
  if (group) setNavGroupOpen(group.label, true, false);
}

// In-app navigation guard. Every nav control in this app (sidebar buttons,
// Cancel links, the breadcrumb) moves by setting location.hash, so guarding the
// hashchange covers all of them AND the browser back button, which intercepting
// clicks would not. On a declined prompt the hash is put back, and the
// resulting second hashchange is swallowed.
let currentHash = location.hash;
let revertingHash = false;
// A Settings tab is a deep link into the SAME rendered view, not a navigation:
// every tab's controls are in the DOM at once (that is what lets any tab's Save
// send the whole settings object), so switching one re-renders nothing, keeps
// unsaved edits and must not raise the discard prompt.
function settingsTabOf(hash) {
  const m = /^#\/settings(?:\/([^/]+))?$/.exec(hash || '');
  return m ? (m[1] || SETTINGS_TABS[0].id) : null;
}
async function onHashChange() {
  if (revertingHash) { revertingHash = false; currentHash = location.hash; return; }
  const nextTab = settingsTabOf(location.hash);
  if (nextTab && settingsTabOf(currentHash) && document.getElementById('set-tabs')) {
    currentHash = location.hash;
    showSettingsTab(nextTab);
    return;
  }
  if (dirtyFlag && location.hash !== currentHash) {
    const leave = await confirmModal({
      title: 'You have unsaved changes',
      body: '<p>Changes on this page are not committed yet. Leaving discards them.</p>',
      confirmLabel: 'Discard and leave',
    });
    if (!leave) { revertingHash = true; location.hash = currentHash; return; }
  }
  currentHash = location.hash;
  clearDirty();
  await route();
}

// Fleet-wide maintenance is invisible from every page except Settings, yet it
// makes every proxy host answer 503 - so while it is on, say so everywhere.
// The probe is the cached /api/capabilities response; a Settings save that
// changes it invalidates that cache, so the next view re-reads it.
function applyMaintenanceBanner(container) {
  if (!container || !hasCapability('maintenance.globalEnabled')) return;
  const banner = document.createElement('div');
  banner.className = 'ro-banner warn';
  banner.innerHTML = '<b>Maintenance mode is ON for all hosts.</b> Every proxy host answers 503 instead of dialling its upstream. '
    + 'Turn it off under <a href="#/settings">Settings</a> -> Maintenance mode.';
  container.prepend(banner);
}

// ---------- INTEGRATIONS ----------
// Everything gpm talks to on your behalf, on one page: the DNS zones it
// publishes into, the orchestrators it derives hosts from, the IP feeds it
// pulls, and the endpoints it notifies. All six cards edit `settings`, and all
// six save through ONE helper that starts from the settings object as loaded
// and overlays only what this page renders - PUT /api/settings is a full
// replacement, so rebuilding it from the form would wipe everything Settings
// owns (appName, adminAuth, trustedProxies, securityHeaders, ...).

// The eight event kinds a notification target can subscribe to. `on` is the
// state a BRAND-NEW row starts in, matching notify.DefaultNotificationEvents().
const NOTIFY_EVENTS = [
  { k: 'cert.renewal_failed', label: 'Renewal failed', on: true },
  { k: 'cert.expiring', label: 'Expiring soon', on: true },
  { k: 'cert.expired', label: 'Expired', on: true },
  { k: 'upstream.unhealthy', label: 'Upstream unhealthy', on: true },
  { k: 'upstream.recovered', label: 'Upstream recovered', on: true },
  { k: 'acme.account_error', label: 'ACME account error', on: true },
  { k: 'discovery.frozen', label: 'Discovery frozen', on: true },
  { k: 'config.changed', label: 'Config changed', on: false },
];
function defaultNotifyEvents() { return NOTIFY_EVENTS.filter((e) => e.on).map((e) => e.k); }
function sameEventSet(a, b) {
  const x = arr(a).slice().sort().join('|');
  const y = arr(b).slice().sort().join('|');
  return x === y;
}
// Toggleable chips, one per event kind. An absent/empty stored list means the
// server-side default, so that is what the chips show - rendering "nothing
// selected" would misrepresent what the target is actually subscribed to.
function makeEventChips(container, selected) {
  const on = new Set(arr(selected).length ? arr(selected) : defaultNotifyEvents());
  function render() {
    container.innerHTML = NOTIFY_EVENTS.map((e) =>
      `<button type="button" class="chip toggle${on.has(e.k) ? ' on' : ''}" data-k="${esc(e.k)}" aria-pressed="${on.has(e.k) ? 'true' : 'false'}" title="${esc(e.k)}">${esc(e.label)}</button>`).join('');
    container.querySelectorAll('.chip.toggle').forEach((b) => b.addEventListener('click', () => {
      if (on.has(b.dataset.k)) on.delete(b.dataset.k); else on.add(b.dataset.k);
      render();
      container.dispatchEvent(new CustomEvent('switchchange', { bubbles: true }));
    }));
  }
  render();
  return { get: () => NOTIFY_EVENTS.map((e) => e.k).filter((k) => on.has(k)) };
}

function intSaveBar(label) {
  return `<div class="panel save-bar int-save-bar">
    <div class="save-note">${ICON.commit}Saves the whole settings object as one revision.</div>
    <div style="display:flex;gap:10px"><button class="btn primary int-save" type="button">${esc(label || 'Save changes')}</button></div>
  </div>`;
}

// Shared status renderers. The Ingress and Docker reconcilers report the same
// shape (counters, a hosts table, lastRun/lastSuccess), so they render through
// one pair of helpers rather than two near-identical copies.
const DISCOVERY_ACTION_CLASS = { created: 'ok', updated: 'cyan', unchanged: '', deleted: 'err', skipped: 'warn' };
function discoveryCounters(st) {
  const n = (k) => (st[k] || 0);
  return `<div class="chip-row" style="margin:6px 0">
    <span class="chip">discovered ${n('discovered')}</span>
    <span class="chip">managed ${n('managed')}</span>
    <span class="chip ok">created ${n('created')}</span>
    <span class="chip cyan">updated ${n('updated')}</span>
    <span class="chip err">deleted ${n('deleted')}</span>
    <span class="chip warn">skipped ${n('skipped')}</span>
  </div>`;
}
function discoveryHostsTable(hosts, secondCol, secondKey) {
  if (!arr(hosts).length) return '<div class="hint">No hosts in the last run.</div>';
  return `<div class="table-wrap"><table class="mini-table">
    <thead><tr><th>Host</th><th>${esc(secondCol)}</th><th>Domains</th><th>Action</th><th>Profile</th><th>Reason</th></tr></thead>
    <tbody>${arr(hosts).map((h) => `<tr>
      <td><a href="#/hosts/${encodeURIComponent(h.name || '')}" class="mono">${esc(h.name || '')}</a></td>
      <td class="mono faint">${esc(h[secondKey] || '')}</td>
      <td class="mono faint">${esc(arr(h.domains).join(', '))}</td>
      <td><span class="chip ${DISCOVERY_ACTION_CLASS[h.action] || ''}">${esc(h.action || '')}</span></td>
      <td class="mono faint">${esc(h.profile || '')}</td>
      <td class="faint">${esc(h.reason || '')}</td>
    </tr>`).join('')}</tbody>
  </table></div>`;
}
function discoveryRunLine(st) {
  const stale = st.lastSuccess && st.lastRun && new Date(st.lastSuccess) < new Date(st.lastRun);
  const tip = 'A failed run freezes the managed hosts; it never deletes them.';
  return `<div class="chip-row"${stale ? ` title="${esc(tip)}"` : ''}>
    <span class="chip${stale ? ' warn' : ''}">Last run ${esc(st.lastRun ? relTime(st.lastRun) : 'never')}</span>
    <span class="chip${stale ? ' warn' : ''}">Last success ${esc(st.lastSuccess ? relTime(st.lastSuccess) : 'never')}</span>
  </div>`;
}

async function viewIntegrations(c) {
  const [setR, , ] = await Promise.all([
    api('/api/settings'),
    loadCapabilities(),
    loadCounts(),
  ]);
  const s = setR.data || {};
  const ds = Object.assign({ pihole: {}, cloudflare: {} }, s.dnsSync || {});
  ds.pihole = ds.pihole || {};
  ds.cloudflare = ds.cloudflare || {};
  const alsEnabled = !(s.accessListSync && s.accessListSync.enabled === false);
  const alsPoll = (s.accessListSync && s.accessListSync.pollInterval) || '';
  const idc = Object.assign({ template: {} }, s.ingressDiscovery || {});
  const idt = Object.assign({ upstream: {}, tls: {}, defaultDNS: {} }, idc.template || {});
  idt.upstream = idt.upstream || {};
  idt.tls = idt.tls || {};
  idt.defaultDNS = idt.defaultDNS || {};
  // Left as a plain object for reading only: `timeoutsPayload` returns undefined
  // when both inputs are blank, so an unset block never round-trips a
  // `timeouts: {}` onto the template (and from there onto every derived host).
  const idTo = idt.timeouts || {};
  const idProfiles = (idc.profiles && typeof idc.profiles === 'object') ? idc.profiles : {};
  const dd = Object.assign({ template: {} }, s.dockerDiscovery || {});
  const ddt = Object.assign({ tls: {}, defaultDNS: {} }, dd.template || {});
  ddt.tls = ddt.tls || {};
  ddt.defaultDNS = ddt.defaultDNS || {};
  const ddTo = ddt.timeouts || {};
  const ddProfiles = (dd.profiles && typeof dd.profiles === 'object') ? dd.profiles : {};
  const resolvedPrefix = idc.annotationPrefix || 'gpm.rake.pro';

  c.innerHTML = viewHead('Integrations',
    'Systems gpm talks to on your behalf: DNS zones it publishes into, orchestrators it derives hosts from, IP feeds it pulls, and the endpoints it notifies.')
    + aboutPageHtml('page.integrations')
    + `
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">DNS sync</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Publishes CNAMEs for proxy hosts that opted in (per-host, under DNS sync in the host editor). Reconcile is full-state, and gpm only ever deletes records it created itself, recorded in the ownership ledger at <span class="mono">config/dns-ledger.yaml</span>. A record that is not in that ledger is never deleted, whatever it points at. Turning a backend on for the first time is therefore safe: matching records are adopted, everything else is left exactly as it is. Use <b>Preview changes</b> first to see it.</p>
      <div class="grid-2">
        <div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Pi-hole (LAN)</div><div class="ds">Local CNAMEs on the LAN resolver</div></div>${switchHtml('set-ph-on', !!ds.pihole.enabled, 'Pi-hole DNS sync', 'settings.dnsSync.pihole.enabled')}</div>
          <div class="field-group" style="margin-top:8px"><label>Pi-hole URL</label><input class="field mono" id="set-ph-url" data-hint="settings.dnsSync.pihole.url" data-path="dnsSync.pihole.url" value="${esc(ds.pihole.url || '')}" placeholder="http://pihole.lan" /></div>
          <div class="field-group"><label>App password</label><input class="field mono" id="set-ph-pw" data-hint="settings.dnsSync.pihole.appPassword" data-path="dnsSync.pihole.appPassword" value="${esc(ds.pihole.appPassword || '')}" placeholder="\${ENV:PIHOLE_APP_PASSWORD}" /></div>
          <div class="field-group"><label>Apex target</label><input class="field mono" id="set-ph-apex" data-hint="settings.dnsSync.pihole.apexTarget" data-path="dnsSync.pihole.apexTarget" value="${esc(ds.pihole.apexTarget || '')}" placeholder="edge.example.com" /><div class="hint">Where managed CNAMEs point. It is not an ownership marker: hand-written records aimed here are adopted only if a host asks for that exact name, and are otherwise left alone.</div></div>
        </div>
        <div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Cloudflare (public)</div><div class="ds">Records in the authoritative zone</div></div>${switchHtml('set-cf-on', !!ds.cloudflare.enabled, 'Cloudflare DNS sync', 'settings.dnsSync.cloudflare.enabled')}</div>
          <div class="field-group" style="margin-top:8px"><label>DNS provider</label><input class="field mono" id="set-cf-ref" data-hint="settings.dnsSync.cloudflare.dnsProviderRef" data-path="dnsSync.cloudflare.dnsProviderRef" value="${esc(ds.cloudflare.dnsProviderRef || '')}" placeholder="cloudflare" /><div class="hint">Names a <a href="#/dns">DNS Providers</a> entry; its <span class="mono">apiToken</span> is reused.</div></div>
          <div class="field-group"><label>Zone name</label><input class="field mono" id="set-cf-zone" data-hint="settings.dnsSync.cloudflare.zoneName" data-path="dnsSync.cloudflare.zoneName" value="${esc(ds.cloudflare.zoneName || '')}" placeholder="example.com" /></div>
          <div class="field-group"><label>Apex target</label><input class="field mono" id="set-cf-apex" data-hint="settings.dnsSync.cloudflare.apexTarget" data-path="dnsSync.cloudflare.apexTarget" value="${esc(ds.cloudflare.apexTarget || '')}" placeholder="edge.example.com" /></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Proxied</div><div class="ds">Cloudflare orange cloud (off = DNS only)</div></div>${switchHtml('set-cf-proxied', !!ds.cloudflare.proxied, 'Proxied', 'settings.dnsSync.cloudflare.proxied')}</div>
        </div>
      </div>
      <div id="set-dns-status" class="hint" style="margin-top:12px"></div>
      <div id="set-dns-plan" class="hint" style="margin-top:6px"></div>
      <div style="margin-top:6px;display:flex;gap:6px;flex-wrap:wrap">
        <button class="btn ghost sm" id="set-dns-preview" type="button" title="Read the backends and report what a reconcile would change. Nothing is written.">Preview changes</button>
        <button class="btn ghost sm" id="set-dns-run" type="button">Reconcile now</button>
      </div>
      ${intSaveBar('Save DNS sync')}
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Kubernetes Ingress discovery</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Polls the cluster read-only for Ingresses annotated <span class="mono">${esc(resolvedPrefix)}/managed: "true"</span> and reconciles them into proxy hosts labelled <span class="mono">${esc(resolvedPrefix)}/managed-by: ingress-discovery</span>. Only those labelled hosts are ever written or removed; a host you wrote by hand is never touched. Everything except the hostnames and the two DNS annotations comes from the template or a named profile below, so a cluster manifest can never supply an upstream, a certificate or a middleware - only the <em>name</em> of a chain you wrote here. The prefix is set under <a href="#/settings/advanced">Settings -&gt; Advanced</a>.</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Ingress discovery</div><div class="ds">Reconcile annotated cluster Ingresses into managed proxy hosts</div></div>${switchHtml('set-id-on', !!idc.enabled, 'Ingress discovery', 'settings.ingressDiscovery.enabled')}</div>
      ${foldHtml('id-conn-card', 'Connection', idc.apiURL || 'in-cluster endpoint', !!idc.enabled, `
        <div class="grid-2">
          <div>
            <div class="field-group"><label>API server URL</label><input class="field mono" id="set-id-url" data-hint="settings.ingressDiscovery.apiURL" data-path="ingressDiscovery.apiURL" value="${esc(idc.apiURL || '')}" placeholder="https://k8s.example.lan:6443" /></div>
            <div class="field-group"><label>Token file</label><input class="field mono" id="set-id-token" data-hint="settings.ingressDiscovery.tokenFile" data-path="ingressDiscovery.tokenFile" value="${esc(idc.tokenFile || '')}" placeholder="/run/secrets/gpm-k8s-token" /><div class="hint">Path to a read-only ServiceAccount token. Re-read periodically, so a rotated token is picked up.</div></div>
          </div>
          <div>
            <div class="field-group"><label>CA file</label><input class="field mono" id="set-id-ca" data-hint="settings.ingressDiscovery.caFile" data-path="ingressDiscovery.caFile" value="${esc(idc.caFile || '')}" placeholder="/run/secrets/gpm-k8s-ca.crt" /></div>
            <div class="field-group"><label>Poll interval</label><input class="field mono" id="set-id-poll" data-hint="settings.ingressDiscovery.pollInterval" data-path="ingressDiscovery.pollInterval" value="${esc(idc.pollInterval || '')}" placeholder="60s" /><div class="hint">Go duration, minimum 15s. Empty means 1m.</div></div>
          </div>
        </div>
      `)}
      ${foldHtml('id-scope-card', 'Scope and safety', arr(idc.allowedDomainSuffixes).length ? arr(idc.allowedDomainSuffixes).join(', ') : 'no allowed domain suffixes yet', !!idc.enabled, `
        <div class="grid-2">
          <div>
            <div class="field-group"><label>Namespace</label><input class="field mono" id="set-id-ns" data-hint="settings.ingressDiscovery.namespace" data-path="ingressDiscovery.namespace" value="${esc(idc.namespace || '')}" placeholder="(all namespaces)" /></div>
            <div class="field-group"><label>Label selector</label><input class="field mono" id="set-id-sel" data-hint="settings.ingressDiscovery.labelSelector" data-path="ingressDiscovery.labelSelector" value="${esc(idc.labelSelector || '')}" placeholder="app.kubernetes.io/part-of=platform" /></div>
          </div>
          <div>
            <div class="field-group">
              <label>Allowed domain suffixes</label>
              <div class="chip-input" id="set-id-suffixes" data-hint="settings.ingressDiscovery.allowedDomainSuffixes" data-path="ingressDiscovery.allowedDomainSuffixes"></div>
              <div class="hint">Required. A discovered hostname must equal or end in one of these, so a cluster manifest cannot publish an arbitrary name at the edge.</div>
            </div>
          </div>
        </div>
      `)}
      ${foldHtml('id-tmpl-card', 'Default host template', idt.tls.certificateRef ? 'certificate ' + idt.tls.certificateRef : 'no certificate set', !!idc.enabled, `
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">gpm runs outside the cluster, so in-cluster Service DNS is not resolvable: the upstream is the <strong>ingress controller's</strong> address, and the controller routes by vhost using the Host header gpm forwards. Prefer <span class="mono">http</span> to its plain port; with <span class="mono">https</span> the upstream host is what SNI and certificate verification use.</p>
        <div class="grid-2">
          <div>
            <div class="field-group">
              <label>Upstream (ingress controller)</label>
              <div class="loc-row">
                <select class="field mono" id="set-id-up-scheme" data-hint="settings.ingressDiscovery.template.upstream" style="flex:0 0 90px" aria-label="Upstream scheme">
                  <option value="http"${(idt.upstream.scheme || 'http') === 'http' ? ' selected' : ''}>http</option>
                  <option value="https"${idt.upstream.scheme === 'https' ? ' selected' : ''}>https</option>
                </select>
                <input class="field mono" id="set-id-up-host" data-hint="settings.ingressDiscovery.template.upstream" style="flex:2 1 160px" value="${esc(idt.upstream.host || '')}" placeholder="10.0.0.40" aria-label="Upstream host" />
                <input class="field mono" id="set-id-up-port" data-hint="settings.ingressDiscovery.template.upstream" style="flex:0 0 90px" value="${esc(idt.upstream.port || '')}" placeholder="80" aria-label="Upstream port" />
              </div>
              <div class="hint">Leave blank and name an upstream group below instead when the controller runs on more than one node.</div>
            </div>
            <div class="field-group"><label>Upstream group (instead of the address above)</label><input class="field mono" id="set-id-up-group" data-hint="settings.ingressDiscovery.template.upstreamGroupRef" value="${esc(idt.upstreamGroupRef || '')}" placeholder="k8s-nodes" /><div class="hint">Mutually exclusive with the upstream above. Preferred when the ingress controller runs on every node: derived hosts then fail over exactly like hand-written ones.</div></div>
            <div class="field-group"><label>Certificate</label><input class="field mono" id="set-id-cert" data-hint="settings.ingressDiscovery.template.tls.certificateRef" data-path="ingressDiscovery.template.tls.certificateRef" value="${esc(idt.tls.certificateRef || '')}" placeholder="wildcard" /><div class="hint">Required. Discovery never issues per-host certificates: point this at your wildcard certificate.</div></div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect http to https on derived hosts</div></div>${switchHtml('set-id-forcessl', !!idt.tls.forceSSL, 'Force SSL', 'settings.ingressDiscovery.template.tls.forceSSL')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send X-Robots-Tag on derived hosts</div></div>${switchHtml('set-id-robots', !!idt.robotsNoIndex, 'Discourage indexing', 'settings.ingressDiscovery.template.robotsNoIndex')}</div>
          </div>
          <div>
            <div class="field-group"><label>Middlewares</label><div class="chip-input" id="set-id-mw" data-hint="settings.ingressDiscovery.template.middlewares"></div><div class="hint">Applied to every derived host, in order.</div></div>
            <div class="field-group"><label>Access lists</label><div class="chip-input" id="set-id-al" data-hint="settings.ingressDiscovery.template.accessLists"></div></div>
            <div class="field-group">
              <label>Upstream timeouts (seconds)</label>
              <div class="loc-row">
                <input class="field mono" id="set-id-to-connect" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(idTo.connectSeconds || '')}" placeholder="connect" aria-label="Connect timeout seconds" />
                <input class="field mono" id="set-id-to-read" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(idTo.readSeconds || '')}" placeholder="read" aria-label="Read timeout seconds" />
              </div>
              <div class="hint">Leave both blank for the shared pooled transport. Read caps time-to-first-byte only, so streaming upstreams stay safe.</div>
            </div>
            <div class="field-group"><label>Tags</label><div class="chip-input" id="set-id-tags" data-hint="settings.ingressDiscovery.template.tags"></div><div class="hint">Applied to every derived host, for grouping in the host list.</div></div>
            <div class="field-group">
              <label>Allowed domain suffixes override (optional)</label>
              <div class="chip-input" id="set-id-suffixes-override" data-hint="settings.ingressDiscovery.template.allowedDomainSuffixes"></div>
              <div class="hint">Narrows the global list above for hosts derived from the template. Must be a subset. Leave empty to use the global list.</div>
            </div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Default LAN direct</div><div class="ds">When the lan-direct annotation is absent</div></div>${switchHtml('set-id-dns-lan', !!idt.defaultDNS.lanDirect, 'Default LAN direct', 'settings.ingressDiscovery.template.defaultDNS.lanDirect')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Default public CNAME</div><div class="ds">When the public-cname annotation is absent</div></div>${switchHtml('set-id-dns-pub', !!idt.defaultDNS.publicCname, 'Default public CNAME', 'settings.ingressDiscovery.template.defaultDNS.publicCname')}</div>
          </div>
        </div>
      `)}
      ${foldHtml('id-profiles-card', 'Named profiles', Object.keys(idProfiles).length ? Object.keys(idProfiles).sort().join(', ') : 'none - every Ingress gets the template', Object.keys(idProfiles).length > 0, `
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">One template only fits a uniform fleet. Define a profile per chain you actually run - a deliberately public one with a rate limit and no access list, an SSO-gated one behind the VPN list - and an Ingress picks one with <span class="mono">${esc(resolvedPrefix)}/profile: "&lt;name&gt;"</span>. <strong>The annotation carries a name and nothing else.</strong> <strong>Every profile is selectable by every annotating Ingress - define only profiles you are willing for any cluster tenant to choose.</strong> An Ingress naming a profile that does not exist is <strong>skipped</strong>, and if it already had a derived host, that host is <strong>disabled</strong>.</p>
        <div id="set-id-profiles" data-hint="settings.ingressDiscovery.profiles"></div>
        <button class="btn ghost sm" id="addIdProfile" type="button" style="margin-top:6px">${ICON.plus}Add profile</button>
      `)}
      ${foldHtml('id-rules-card', 'Profile selection rules', arr(idc.profileRules).length ? arr(idc.profileRules).length + ' rule(s)' : 'none - the annotation decides', arr(idc.profileRules).length > 0, `
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Rules let YOU route an Ingress to a profile by namespace and/or label, with no say from the Ingress author at all - stronger than the annotation. Evaluated in order; the first match wins.</p>
        <div class="field-group" style="max-width:380px"><label>Mode</label>
          <select class="field mono" id="set-id-profile-selection" data-hint="settings.ingressDiscovery.profileSelection" data-path="ingressDiscovery.profileSelection">
            ${enumOptions('profileSelection', ['', 'rules-only'], idc.profileSelection === 'rules-only' ? 'rules-only' : '')}
          </select>
        </div>
        <div id="set-id-rules" data-hint="settings.ingressDiscovery.profileRules"></div>
        <button class="btn ghost sm" id="addIdRule" type="button" style="margin-top:6px">${ICON.plus}Add rule</button>
      `)}
      <div id="set-id-status" class="hint" style="margin-top:12px"></div>
      <div id="set-id-plan" class="hint" style="margin-top:6px"></div>
      <div id="set-id-hosts" style="margin-top:8px"></div>
      <div style="margin-top:6px;display:flex;gap:6px;flex-wrap:wrap">
        <button class="btn ghost sm" id="set-id-preview" type="button" title="Read the cluster and report what a reconcile would create, update, delete and skip. Nothing is written.">Preview changes</button>
        <button class="btn ghost sm" id="set-id-run" type="button">Reconcile now</button>
      </div>
      ${intSaveBar('Save Ingress discovery')}
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Docker discovery</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Derives proxy hosts from labelled Docker containers. Uses the same templates, profiles and label prefix as Kubernetes Ingress discovery - only the source differs. Label prefix: <span class="mono">${esc(resolvedPrefix)}</span> (set under <a href="#/settings/advanced">Settings -&gt; Advanced</a>).</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Docker discovery</div><div class="ds">Turn container discovery on. Everything below is only validated when this is on.</div></div>${switchHtml('set-dkr-on', !!dd.enabled, 'Docker discovery', 'settings.dockerDiscovery.enabled')}</div>
      <div id="dkr-fields">
        ${foldHtml('dkr-conn-card', 'Engine connection', dd.host || dd.socket || '/var/run/docker.sock', !!dd.enabled, `
          <div class="grid-2">
            <div>
              <div class="field-group"><label>Socket</label><input class="field mono" id="set-dkr-socket" data-hint="settings.dockerDiscovery.socket" data-path="dockerDiscovery.socket" value="${esc(dd.socket || '')}" placeholder="/var/run/docker.sock" /><div class="hint">Docker Engine unix socket. Absolute path. Ignored when a host URL is set.</div></div>
              <div class="field-group"><label>Host URL</label><input class="field mono" id="set-dkr-host" data-hint="settings.dockerDiscovery.host" data-path="dockerDiscovery.host" value="${esc(dd.host || '')}" placeholder="tcp://socket-proxy:2375" /><div class="hint">Engine endpoint used instead of the socket. A read-only socket proxy is the recommended shape.</div></div>
              <div class="field-group"><label>Poll interval</label><input class="field mono" id="set-dkr-poll" data-hint="settings.dockerDiscovery.pollInterval" data-path="dockerDiscovery.pollInterval" value="${esc(dd.pollInterval || '')}" placeholder="60s" /><div class="hint">Fallback loop interval (Go duration, minimum 15s). Container events drive the normal case.</div></div>
            </div>
            <div>
              <div class="field-group"><label>TLS client certificate</label><input class="field mono" id="set-dkr-tlscert" data-hint="settings.dockerDiscovery.tlsCert" data-path="dockerDiscovery.tlsCert" value="${esc(dd.tlsCert || '')}" placeholder="/run/secrets/docker-client.crt" /><div class="hint">Absolute path. Only for an <span class="mono">https://</span> host URL. Set with the key.</div></div>
              <div class="field-group"><label>TLS client key</label><input class="field mono" id="set-dkr-tlskey" data-hint="settings.dockerDiscovery.tlsKey" data-path="dockerDiscovery.tlsKey" value="${esc(dd.tlsKey || '')}" placeholder="/run/secrets/docker-client.key" /></div>
              <div class="field-group"><label>TLS CA bundle</label><input class="field mono" id="set-dkr-tlsca" data-hint="settings.dockerDiscovery.tlsCA" data-path="dockerDiscovery.tlsCA" value="${esc(dd.tlsCA || '')}" placeholder="/run/secrets/docker-ca.crt" /><div class="hint">Absolute path to the PEM bundle that verifies the endpoint. There is no skip-verify option.</div></div>
            </div>
          </div>
        `)}
        ${foldHtml('dkr-addr-card', 'Container addressing', dd.usePublishedPorts ? 'published ports on ' + (dd.publishedHost || '127.0.0.1') : (dd.network || 'first non-host network'), !!dd.enabled, `
          <div class="grid-2">
            <div>
              <div class="field-group"><label>Network</label><input class="field mono" id="set-dkr-network" data-hint="settings.dockerDiscovery.network" data-path="dockerDiscovery.network" value="${esc(dd.network || '')}" placeholder="first non-host network" /></div>
              <div class="field-group"><label>Published host</label><input class="field mono" id="set-dkr-pubhost" data-hint="settings.dockerDiscovery.publishedHost" data-path="dockerDiscovery.publishedHost" value="${esc(dd.publishedHost || '')}" placeholder="127.0.0.1" /></div>
            </div>
            <div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Use published ports</div><div class="ds">Forward to the host-published port instead of the container IP. Use when gpm runs on the Docker host rather than in a shared network.</div></div>${switchHtml('set-dkr-pubports', !!dd.usePublishedPorts, 'Use published ports', 'settings.dockerDiscovery.usePublishedPorts')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Include stopped containers</div><div class="ds">List non-running containers too. A stopped container has no address, so its host is skipped and frozen rather than derived.</div></div>${switchHtml('set-dkr-stopped', !!dd.includeStopped, 'Include stopped containers', 'settings.dockerDiscovery.includeStopped')}</div>
              <div class="field-group" style="margin-top:10px">
                <label>Allowed domain suffixes</label>
                <div class="chip-input" id="set-dkr-suffixes" data-hint="settings.dockerDiscovery.allowedDomainSuffixes" data-path="dockerDiscovery.allowedDomainSuffixes"></div>
                <div class="hint">Required. A derived hostname must equal one of these or end in "." plus one of them.</div>
              </div>
            </div>
          </div>
        `)}
        ${foldHtml('dkr-tmpl-card', 'Default host template', ddt.tls.certificateRef ? 'certificate ' + ddt.tls.certificateRef : 'no certificate set', !!dd.enabled, `
          <p class="muted" style="font-size:11.5px;margin:0 0 10px">The default chain every derived host takes. The upstream is the container's own address, so there is nothing to set here.</p>
          <div class="grid-2">
            <div>
              <div class="field-group"><label>Certificate</label><input class="field mono" id="set-dkr-cert" data-hint="settings.dockerDiscovery.template.tls.certificateRef" data-path="dockerDiscovery.template.tls.certificateRef" value="${esc(ddt.tls.certificateRef || '')}" placeholder="wildcard" /></div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect http to https on derived hosts</div></div>${switchHtml('set-dkr-forcessl', !!ddt.tls.forceSSL, 'Force SSL', 'settings.dockerDiscovery.template.tls.forceSSL')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send X-Robots-Tag on derived hosts</div></div>${switchHtml('set-dkr-robots', !!ddt.robotsNoIndex, 'Discourage indexing', 'settings.dockerDiscovery.template.robotsNoIndex')}</div>
              <div class="field-group" style="margin-top:10px">
                <label>Upstream timeouts (seconds)</label>
                <div class="loc-row">
                  <input class="field mono" id="set-dkr-to-connect" data-hint="settings.dockerDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(ddTo.connectSeconds || '')}" placeholder="connect" aria-label="Connect timeout seconds" />
                  <input class="field mono" id="set-dkr-to-read" data-hint="settings.dockerDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(ddTo.readSeconds || '')}" placeholder="read" aria-label="Read timeout seconds" />
                </div>
              </div>
            </div>
            <div>
              <div class="field-group"><label>Middlewares</label><div class="chip-input" id="set-dkr-mw" data-hint="settings.dockerDiscovery.template.middlewares"></div></div>
              <div class="field-group"><label>Access lists</label><div class="chip-input" id="set-dkr-al" data-hint="settings.dockerDiscovery.template.accessLists"></div></div>
              <div class="field-group"><label>Tags</label><div class="chip-input" id="set-dkr-tags" data-hint="settings.dockerDiscovery.template.tags"></div></div>
              <div class="field-group"><label>Allowed domain suffixes override (optional)</label><div class="chip-input" id="set-dkr-suffixes-override" data-hint="settings.dockerDiscovery.template.allowedDomainSuffixes"></div><div class="hint">Must be a subset of the list above.</div></div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Default LAN direct</div></div>${switchHtml('set-dkr-dns-lan', !!ddt.defaultDNS.lanDirect, 'Default LAN direct', 'settings.dockerDiscovery.template.defaultDNS.lanDirect')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Default public CNAME</div></div>${switchHtml('set-dkr-dns-pub', !!ddt.defaultDNS.publicCname, 'Default public CNAME', 'settings.dockerDiscovery.template.defaultDNS.publicCname')}</div>
            </div>
          </div>
        `)}
        ${foldHtml('dkr-profiles-card', 'Named profiles', Object.keys(ddProfiles).length ? Object.keys(ddProfiles).sort().join(', ') : 'none - containers use the Ingress discovery profiles', Object.keys(ddProfiles).length > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 10px">Leave empty to use the Ingress discovery profiles. Add rows only to give containers a different set.</p>
          <div id="set-dkr-profiles" data-hint="settings.dockerDiscovery.profiles"></div>
          <button class="btn ghost sm" id="addDkrProfile" type="button" style="margin-top:6px">${ICON.plus}Add profile</button>
        `)}
      </div>
      <div id="set-dkr-status" style="margin-top:12px"></div>
      <div style="margin-top:6px;display:flex;gap:6px;flex-wrap:wrap">
        <button class="btn ghost sm" id="set-dkr-preview" type="button" title="Read the Engine and report what a reconcile would change. Nothing is written.">Preview changes</button>
        <button class="btn ghost sm" id="set-dkr-run" type="button">Reconcile now</button>
      </div>
      ${intSaveBar('Save Docker discovery')}
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Access-list sync</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Keeps access lists that declare a remote <b>source</b> (an IP feed fetched over http(s)) up to date. The interval below is only how often the loop asks whether any source is <i>due</i>; each source carries its own fetch interval, so polling often is cheap. A list with no source costs nothing either way.</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Fetch remote sources</div><div class="ds">On by default; turning it off freezes every list at its last fetch</div></div>${switchHtml('set-als-on', alsEnabled, 'Fetch remote access-list sources', 'settings.accessListSync.enabled')}</div>
      <div class="field-group" style="margin-top:10px;max-width:220px">
        <label>Poll interval</label>
        <input class="field mono" id="set-als-poll" data-hint="settings.accessListSync.pollInterval" data-path="accessListSync.pollInterval" value="${esc(alsPoll)}" placeholder="15m" />
        <div class="hint">Go duration, minimum 1m, default 15m.</div>
      </div>
      <div id="int-als-status" style="margin-top:12px"></div>
      <div style="margin-top:6px"><button class="btn ghost sm" id="int-als-run" type="button">Reconcile now</button></div>
      ${intSaveBar('Save access-list sync')}
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Lifecycle webhooks</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">POST a JSON event to each URL after every config change (create/update/delete, restore, revert, settings). Delivery is async and best-effort, so a slow endpoint never blocks a save.</p>
      <div id="set-webhooks" data-hint="settings.webhooks" data-path="webhooks"></div>
      <button class="btn ghost sm" id="addWebhook" type="button" style="margin-top:6px">${ICON.plus}Add webhook</button>
      <div class="hint" style="margin-top:8px">Delivery state is kept in memory and resets when gpm restarts. Test sends <span class="mono">{"action":"test","kind":"Webhook","name":"&lt;target&gt;","time":"&lt;RFC3339&gt;"}</span> to the target URL, with the same <span class="mono">X-GPM-Webhook-Secret</span> header a real event carries.</div>
      ${intSaveBar('Save webhooks')}
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Notifications</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Alerts to ntfy, Discord, or a generic webhook on ACME renewal failure, certificate expiry, upstream health flaps, and a frozen discovery reconciler. Delivery is async and best-effort, so a slow receiver never blocks a save.</p>
      <div class="field-group" style="max-width:260px">
        <label>Expiry digest threshold (days)</label>
        <input class="field mono" id="set-ntf-days" data-hint="settings.notifications.expiringThresholdDays" data-path="notifications.expiringThresholdDays" type="number" min="1" max="365" value="${esc((s.notifications && s.notifications.expiringThresholdDays) || '')}" placeholder="14" />
      </div>
      <div id="set-notifications" data-hint="settings.notifications.targets" data-path="notifications.targets" style="margin-top:10px"></div>
      <button class="btn ghost sm" id="addNotification" type="button" style="margin-top:6px">${ICON.plus}Add notification target</button>
      <div class="hint" style="margin-top:8px">Delivery state is kept in memory and resets when gpm restarts. Test sends a synthetic event to the target immediately, using the same payload shape a real alert would (plain text for ntfy, an embed for Discord, a JSON envelope for generic), bypassing this target's Events filter.</div>
      ${intSaveBar('Save notifications')}
    </div>`;

  // ----- DNS sync -----
  async function renderDNSStatus() {
    const el = $('#set-dns-status');
    if (!el) return;
    try {
      const st = (await api('/api/dns-sync/status')).data || {};
      const line = (label, b) => {
        if (!b || !b.enabled) return `${label}: off`;
        if (b.error) return `${label}: FAILED - ${b.error}`;
        const extra = [];
        if (b.adopted) extra.push(`${b.adopted} adopted`);
        if (b.retargeted) extra.push(`${b.retargeted} retargeted`);
        if (b.skipped) extra.push(`${b.skipped} skipped`);
        if (b.untouched) extra.push(`${b.untouched} left alone`);
        return `${label}: ok (${b.desired} desired, ${b.managed} owned, +${b.created} / -${b.deleted}` +
          (extra.length ? ', ' + extra.join(', ') : '') + ')';
      };
      el.textContent = (st.lastRun ? 'Last run ' + fmtTime(st.lastRun) + '. ' : 'Never run. ') +
        line('Pi-hole', st.pihole) + ' | ' + line('Cloudflare', st.cloudflare) +
        (st.error ? ' | ' + st.error : '');
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'DNS sync is not wired in this deployment.' : 'Status unavailable: ' + (e.message || e);
    }
  }
  renderDNSStatus();

  // Dry run. gpm only ever deletes records its ownership ledger says it created,
  // so the number that matters before a first reconcile is "left alone" - it
  // should equal everything the operator wrote by hand on that backend.
  async function renderDNSPlan() {
    const el = $('#set-dns-plan');
    if (!el) return;
    el.textContent = 'Reading backends...';
    try {
      const p = (await api('/api/dns-sync/plan')).data || {};
      const names = (a) => arr(a).join(', ');
      const line = (label, b) => {
        if (!b || !b.enabled) return `${label}: off`;
        if (b.error) return `${label}: FAILED - ${b.error}`;
        const parts = [];
        if (arr(b.create).length) parts.push(`create ${names(b.create)}`);
        if (arr(b.adopt).length) parts.push(`adopt ${names(b.adopt)}`);
        if (arr(b.retarget).length) parts.push(`retarget ${names(b.retarget)}`);
        if (arr(b.delete).length) parts.push(`DELETE ${names(b.delete)}`);
        if (arr(b.skip).length) parts.push(`skip (not ours) ${names(b.skip)}`);
        parts.push(`${b.untouched || 0} record(s) gpm does not own, left alone`);
        return `${label}: ` + parts.join('; ');
      };
      el.textContent = 'Dry run, nothing was changed. ' + line('Pi-hole', p.pihole) + ' | ' + line('Cloudflare', p.cloudflare);
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'DNS sync is not wired in this deployment.' : 'Preview unavailable: ' + (e.message || e);
    }
  }
  $('#set-dns-preview').addEventListener('click', async () => {
    const btn = $('#set-dns-preview'); btn.disabled = true;
    await renderDNSPlan();
    btn.disabled = false;
  });
  $('#set-dns-run').addEventListener('click', async () => {
    const btn = $('#set-dns-run'); btn.disabled = true;
    try {
      await api('/api/dns-sync/reconcile', { method: 'POST' });
      toast('Reconciled', 'DNS records are back in step with the config.', 'ok');
    } catch (e) { toastErr(e); }
    await renderDNSStatus();
    btn.disabled = false;
  });

  // ----- Ingress discovery -----
  const idSuffixCtl = makeChipInput($('#set-id-suffixes'), arr(idc.allowedDomainSuffixes), 'add suffix...');
  const idMwCtl = makeChipInput($('#set-id-mw'), arr(idt.middlewares), 'add middleware...');
  const idAlCtl = makeChipInput($('#set-id-al'), arr(idt.accessLists), 'add access list...');
  const idTagCtl = makeChipInput($('#set-id-tags'), arr(idt.tags), 'add tag...');
  const idTemplateSuffixCtl = makeChipInput($('#set-id-suffixes-override'), arr(idt.allowedDomainSuffixes), 'add suffix...');

  // Same MERGE invariant as the tls object in the save handler: `timeouts` is
  // rebuilt from the two inputs this form renders, over whatever was loaded, so a
  // field added to HostTimeouts later is not stripped by an unrelated save. Both
  // inputs empty means "no override" and drops the key entirely, so the derived
  // hosts carry no `timeouts` at all rather than a zero-valued one.
  // A discovery template's `upstream` is a struct VALUE in the model with no
  // per-field omitempty, so sending `{scheme: 'http', host: '', port: 0}` writes
  // a whole upstream block into settings.yaml for a template that never had one.
  // Nothing to express -> send nothing, and the zero struct is omitted on write.
  function upstreamPayload(schemeEl, hostEl, portEl) {
    const host = hostEl.value.trim();
    const port = parseInt(portEl.value, 10) || 0;
    if (!host && !port) return undefined;
    return { scheme: schemeEl.value, host, port };
  }
  function timeoutsPayload(orig, connect, read) {
    const c = parseInt(connect, 10) || 0;
    const r = parseInt(read, 10) || 0;
    if (!c && !r) return undefined;
    return Object.assign({}, orig, { connectSeconds: c, readSeconds: r });
  }

  // Named discovery profiles. Same fields as the template above, because a
  // profile IS a template - one an Ingress may select by name. The chip
  // controllers are hung off the row so the save handler can read them back.
  const pfWrap = $('#set-id-profiles');
  let pfSeq = 0;
  function profileRow(name, p) {
    p = p || {};
    const up = p.upstream || {};
    const tls = p.tls || {};
    const dns = p.defaultDNS || {};
    const to = p.timeouts || {};
    const i = ++pfSeq;
    const div = document.createElement('div');
    div.className = 'panel id-profile';
    div.style.cssText = 'padding:12px;margin-bottom:10px';
    div.innerHTML = `
      <div class="loc-row" style="margin-bottom:8px">
        <input class="field mono pf-name" data-hint="settings.ingressDiscovery.profiles.name" style="flex:1 1 180px" value="${esc(name || '')}" placeholder="profile name (e.g. sso-internal)" aria-label="Profile name" />
        <button class="icon-btn pf-del" type="button" aria-label="Remove profile">${ICON.x}</button>
      </div>
      <div class="grid-2">
        <div>
          <div class="field-group">
            <label>Upstream (ingress controller)</label>
            <div class="loc-row">
              <select class="field mono pf-up-scheme" data-hint="settings.ingressDiscovery.template.upstream" style="flex:0 0 90px" aria-label="Upstream scheme">
                <option value="http"${(up.scheme || 'http') === 'http' ? ' selected' : ''}>http</option>
                <option value="https"${up.scheme === 'https' ? ' selected' : ''}>https</option>
              </select>
              <input class="field mono pf-up-host" data-hint="settings.ingressDiscovery.template.upstream" style="flex:2 1 160px" value="${esc(up.host || '')}" placeholder="10.0.0.40" aria-label="Upstream host" />
              <input class="field mono pf-up-port" data-hint="settings.ingressDiscovery.template.upstream" style="flex:0 0 90px" value="${esc(up.port || '')}" placeholder="80" aria-label="Upstream port" />
            </div>
          </div>
          <div class="field-group"><label>Upstream group (instead of the address above)</label><input class="field mono pf-up-group" data-hint="settings.ingressDiscovery.template.upstreamGroupRef" value="${esc(p.upstreamGroupRef || '')}" placeholder="k8s-nodes" aria-label="Upstream group" /><div class="hint">Mutually exclusive with the upstream above.</div></div>
          <div class="field-group"><label>Certificate</label><input class="field mono pf-cert" data-hint="settings.ingressDiscovery.template.tls.certificateRef" value="${esc(tls.certificateRef || '')}" placeholder="wildcard" aria-label="Certificate" /><div class="hint">Required, exactly as for the template.</div></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div></div>${switchHtml('pf-forcessl-' + i, !!tls.forceSSL, 'Force SSL', 'settings.ingressDiscovery.template.tls.forceSSL')}</div>
        </div>
        <div>
          <div class="field-group"><label>Middlewares</label><div class="chip-input pf-mw" data-hint="settings.ingressDiscovery.template.middlewares"></div><div class="hint">Applied in order to every host that selects this profile.</div></div>
          <div class="field-group"><label>Access lists</label><div class="chip-input pf-al" data-hint="settings.ingressDiscovery.template.accessLists"></div><div class="hint">Leave empty for a profile that is public on purpose.</div></div>
          <div class="field-group">
            <label>Upstream timeouts (seconds)</label>
            <div class="loc-row">
              <input class="field mono pf-to-connect" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(to.connectSeconds || '')}" placeholder="connect" aria-label="Connect timeout seconds" />
              <input class="field mono pf-to-read" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(to.readSeconds || '')}" placeholder="read" aria-label="Read timeout seconds" />
            </div>
            <div class="hint">Blank for the shared pooled transport.</div>
          </div>
          <div class="field-group"><label>Tags</label><div class="chip-input pf-tags" data-hint="settings.ingressDiscovery.template.tags"></div><div class="hint">Applied to every host that selects this profile.</div></div>
          <div class="field-group"><label>Allowed domain suffixes override (optional)</label><div class="chip-input pf-suffixes" data-hint="settings.ingressDiscovery.template.allowedDomainSuffixes"></div><div class="hint">Narrows the global list for hosts using this profile. Must be a subset.</div></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div></div>${switchHtml('pf-robots-' + i, !!p.robotsNoIndex, 'Discourage indexing', 'settings.ingressDiscovery.template.robotsNoIndex')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Default LAN direct</div></div>${switchHtml('pf-dns-lan-' + i, !!dns.lanDirect, 'Default LAN direct', 'settings.ingressDiscovery.template.defaultDNS.lanDirect')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Default public CNAME</div></div>${switchHtml('pf-dns-pub-' + i, !!dns.publicCname, 'Default public CNAME', 'settings.ingressDiscovery.template.defaultDNS.publicCname')}</div>
        </div>
      </div>`;
    div.dataset.uid = String(i);
    // The loaded profile, kept so the save handler can MERGE rather than rebuild.
    // See the tls merge in the save handler for why this is load-bearing.
    div._orig = p;
    div.querySelector('.pf-del').addEventListener('click', () => div.remove());
    pfWrap.appendChild(div);
    div._mw = makeChipInput(div.querySelector('.pf-mw'), arr(p.middlewares), 'add middleware...');
    div._al = makeChipInput(div.querySelector('.pf-al'), arr(p.accessLists), 'add access list...');
    div._tags = makeChipInput(div.querySelector('.pf-tags'), arr(p.tags), 'add tag...');
    div._suffixes = makeChipInput(div.querySelector('.pf-suffixes'), arr(p.allowedDomainSuffixes), 'add suffix...');
  }
  Object.keys(idProfiles).sort().forEach((n) => profileRow(n, idProfiles[n]));
  $('#addIdProfile').addEventListener('click', () => profileRow('', {}));

  // Profile rules: operator-side selection, strictly stronger than the
  // annotation. Each row is namespace + comma-separated key=value labels +
  // target profile (a profiles key, or "template" for the default block).
  const ruleWrap = $('#set-id-rules');
  function ruleRow(rule) {
    rule = rule || {};
    const div = document.createElement('div');
    div.className = 'loc-row';
    div.style.marginBottom = '8px';
    const labelsStr = Object.entries(rule.matchLabels || {}).map(([k, v]) => k + '=' + v).join(',');
    div.innerHTML = `
      <input class="field mono rule-ns" data-hint="settings.ingressDiscovery.profileRules.namespace" style="flex:1 1 140px" value="${esc(rule.namespace || '')}" placeholder="namespace (any)" aria-label="Rule namespace" />
      <input class="field mono rule-labels" data-hint="settings.ingressDiscovery.profileRules.matchLabels" style="flex:1 1 200px" value="${esc(labelsStr)}" placeholder="key=value,key2=value2 (any)" aria-label="Rule match labels" />
      <input class="field mono rule-profile" data-hint="settings.ingressDiscovery.profileRules.profile" style="flex:1 1 160px" value="${esc(rule.profile || '')}" placeholder="profile name or template" aria-label="Rule target profile" />
      <button class="icon-btn rule-del" type="button" aria-label="Remove rule">${ICON.x}</button>`;
    div.querySelector('.rule-del').addEventListener('click', () => div.remove());
    ruleWrap.appendChild(div);
  }
  arr(idc.profileRules).forEach(ruleRow);
  $('#addIdRule').addEventListener('click', () => ruleRow({}));

  const idAvailable = hasCapability('ingressDiscovery.enabled');
  gateControl($('#set-id-run'), idAvailable, 'Ingress discovery is turned off (enable it above and save).');
  gateControl($('#set-id-preview'), idAvailable, 'Ingress discovery is turned off (enable it above and save).');

  async function renderIngressStatus() {
    const el = $('#set-id-status');
    const list = $('#set-id-hosts');
    if (!el) return;
    if (!idAvailable) {
      el.textContent = 'Ingress discovery is turned off.';
      if (list) list.innerHTML = '';
      return;
    }
    try {
      const st = (await api('/api/ingress-discovery/status')).data || {};
      const parts = [];
      parts.push(st.lastRun ? 'Last run ' + fmtTime(st.lastRun) : 'Never run');
      // A failed run freezes the managed hosts, so how stale the good state is
      // matters more than when the last attempt happened.
      if (st.error) parts.push('FAILED - ' + st.error + (st.lastSuccess ? ' (last success ' + fmtTime(st.lastSuccess) + ', managed hosts frozen as they were)' : ' (no successful run yet)'));
      else parts.push(`${st.discovered || 0} annotated Ingresses, ${st.managed || 0} managed hosts (+${st.created || 0} / ~${st.updated || 0} / -${st.deleted || 0}${st.skipped ? ', ' + st.skipped + ' skipped' : ''})`);
      el.textContent = parts.join('. ') + '.';
      if (list) list.innerHTML = discoveryRunLine(st) + discoveryCounters(st) + discoveryHostsTable(st.hosts, 'Ingress', 'ingress');
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'Ingress discovery is not wired in this deployment.' : 'Status unavailable: ' + (e.message || e);
      if (list) list.innerHTML = '';
    }
  }
  renderIngressStatus();

  // Dry run: the exact per-host decisions a reconcile would take, without
  // writing anything - mirrors renderDNSPlan above.
  async function renderIngressPlan() {
    const el = $('#set-id-plan');
    if (!el) return;
    if (!idAvailable) { el.textContent = 'Ingress discovery is turned off.'; return; }
    el.textContent = 'Reading the cluster...';
    try {
      const p = (await api('/api/ingress-discovery/plan')).data || {};
      if (!p.enabled) { el.textContent = 'Ingress discovery is turned off.'; return; }
      if (p.error) { el.textContent = 'FAILED - ' + p.error; return; }
      const byAction = {};
      arr(p.hosts).forEach((h) => { (byAction[h.action] = byAction[h.action] || []).push(h.name); });
      const line = (label, names) => (names && names.length ? `${label} ${names.join(', ')}` : '');
      const parts = [
        line('create', byAction.created), line('update', byAction.updated),
        line('DELETE', byAction.deleted), line('skip', byAction.skipped),
      ].filter(Boolean);
      el.textContent = 'Dry run, nothing was changed. ' + (parts.length ? parts.join('; ') : 'no changes') +
        ` (${p.discovered || 0} annotated Ingresses, ${p.managed || 0} managed hosts after).`;
      const list = $('#set-id-hosts');
      if (list) list.innerHTML = discoveryCounters(p) + discoveryHostsTable(p.hosts, 'Ingress', 'ingress');
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'Ingress discovery is not wired in this deployment.' :
        (e && e.status === 409 ? 'A reconcile is already in progress; try again shortly.' : 'Preview unavailable: ' + (e.message || e));
    }
  }
  $('#set-id-preview').addEventListener('click', async () => {
    const btn = $('#set-id-preview'); btn.disabled = true;
    await renderIngressPlan();
    btn.disabled = false;
  });
  $('#set-id-run').addEventListener('click', async () => {
    const btn = $('#set-id-run'); btn.disabled = true;
    try {
      await api('/api/ingress-discovery/reconcile', { method: 'POST' });
      toast('Reconciled', 'Managed proxy hosts are back in step with the cluster.', 'ok');
    } catch (e) { toastErr(e); }
    await renderIngressStatus();
    btn.disabled = false;
  });

  // ----- Docker discovery -----
  const dkrSuffixCtl = makeChipInput($('#set-dkr-suffixes'), arr(dd.allowedDomainSuffixes), 'add suffix...');
  const dkrMwCtl = makeChipInput($('#set-dkr-mw'), arr(ddt.middlewares), 'add middleware...');
  const dkrAlCtl = makeChipInput($('#set-dkr-al'), arr(ddt.accessLists), 'add access list...');
  const dkrTagCtl = makeChipInput($('#set-dkr-tags'), arr(ddt.tags), 'add tag...');
  const dkrTemplateSuffixCtl = makeChipInput($('#set-dkr-suffixes-override'), arr(ddt.allowedDomainSuffixes), 'add suffix...');

  const dpfWrap = $('#set-dkr-profiles');
  let dpfSeq = 0;
  // A docker profile is the same object as an ingress profile minus the
  // upstream: the address comes from the container, so rendering upstream rows
  // here would offer fields the reconciler ignores.
  function dockerProfileRow(name, p) {
    p = p || {};
    const tls = p.tls || {};
    const dns = p.defaultDNS || {};
    const to = p.timeouts || {};
    const i = ++dpfSeq;
    const div = document.createElement('div');
    div.className = 'panel dkr-profile';
    div.style.cssText = 'padding:12px;margin-bottom:10px';
    div.innerHTML = `
      <div class="loc-row" style="margin-bottom:8px">
        <input class="field mono dpf-name" data-hint="settings.dockerDiscovery.profiles.name" style="flex:1 1 180px" value="${esc(name || '')}" placeholder="profile name (e.g. public-ratelimited)" aria-label="Profile name" />
        <button class="icon-btn dpf-del" type="button" aria-label="Remove profile">${ICON.x}</button>
      </div>
      <div class="grid-2">
        <div>
          <div class="field-group"><label>Certificate</label><input class="field mono dpf-cert" data-hint="settings.dockerDiscovery.template.tls.certificateRef" value="${esc(tls.certificateRef || '')}" placeholder="wildcard" aria-label="Certificate" /></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div></div>${switchHtml('dpf-forcessl-' + i, !!tls.forceSSL, 'Force SSL', 'settings.dockerDiscovery.template.tls.forceSSL')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div></div>${switchHtml('dpf-robots-' + i, !!p.robotsNoIndex, 'Discourage indexing', 'settings.dockerDiscovery.template.robotsNoIndex')}</div>
          <div class="field-group" style="margin-top:10px">
            <label>Upstream timeouts (seconds)</label>
            <div class="loc-row">
              <input class="field mono dpf-to-connect" data-hint="settings.dockerDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(to.connectSeconds || '')}" placeholder="connect" aria-label="Connect timeout seconds" />
              <input class="field mono dpf-to-read" data-hint="settings.dockerDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(to.readSeconds || '')}" placeholder="read" aria-label="Read timeout seconds" />
            </div>
          </div>
        </div>
        <div>
          <div class="field-group"><label>Middlewares</label><div class="chip-input dpf-mw" data-hint="settings.dockerDiscovery.template.middlewares"></div></div>
          <div class="field-group"><label>Access lists</label><div class="chip-input dpf-al" data-hint="settings.dockerDiscovery.template.accessLists"></div></div>
          <div class="field-group"><label>Tags</label><div class="chip-input dpf-tags" data-hint="settings.dockerDiscovery.template.tags"></div></div>
          <div class="field-group"><label>Allowed domain suffixes override (optional)</label><div class="chip-input dpf-suffixes" data-hint="settings.dockerDiscovery.template.allowedDomainSuffixes"></div></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Default LAN direct</div></div>${switchHtml('dpf-dns-lan-' + i, !!dns.lanDirect, 'Default LAN direct', 'settings.dockerDiscovery.template.defaultDNS.lanDirect')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Default public CNAME</div></div>${switchHtml('dpf-dns-pub-' + i, !!dns.publicCname, 'Default public CNAME', 'settings.dockerDiscovery.template.defaultDNS.publicCname')}</div>
        </div>
      </div>`;
    div.dataset.uid = String(i);
    div._orig = p;
    div.querySelector('.dpf-del').addEventListener('click', () => div.remove());
    dpfWrap.appendChild(div);
    div._mw = makeChipInput(div.querySelector('.dpf-mw'), arr(p.middlewares), 'add middleware...');
    div._al = makeChipInput(div.querySelector('.dpf-al'), arr(p.accessLists), 'add access list...');
    div._tags = makeChipInput(div.querySelector('.dpf-tags'), arr(p.tags), 'add tag...');
    div._suffixes = makeChipInput(div.querySelector('.dpf-suffixes'), arr(p.allowedDomainSuffixes), 'add suffix...');
  }
  Object.keys(ddProfiles).sort().forEach((n) => dockerProfileRow(n, ddProfiles[n]));
  $('#addDkrProfile').addEventListener('click', () => dockerProfileRow('', {}));

  // Mutually exclusive in the model as well as here, so the control that cannot
  // apply is greyed with the reason rather than accepted and then refused.
  function syncDockerAddressing() {
    const pub = isOn('set-dkr-pubports');
    gateControl($('#set-dkr-network'), !pub, 'network and usePublishedPorts are mutually exclusive.');
    gateControl($('#set-dkr-pubhost'), pub, 'publishedHost only applies with usePublishedPorts: true.');
    gateControl($('#dkr-fields'), isOn('set-dkr-on'), 'Turn Docker discovery on to configure it.');
    if (isOn('set-dkr-on')) {
      // gateControl re-enables every descendant control, so the pair above has
      // to be re-applied after the card-wide gate.
      gateControl($('#set-dkr-network'), !pub, 'network and usePublishedPorts are mutually exclusive.');
      gateControl($('#set-dkr-pubhost'), pub, 'publishedHost only applies with usePublishedPorts: true.');
    }
  }
  $('#set-dkr-pubports').addEventListener('switchchange', syncDockerAddressing);
  $('#set-dkr-on').addEventListener('switchchange', syncDockerAddressing);
  syncDockerAddressing();

  const dkrAvailable = hasCapability('dockerDiscovery.enabled');
  gateControl($('#set-dkr-run'), dkrAvailable && !isReadOnly(), dkrAvailable ? readOnlyReason() : 'Docker discovery is turned off (enable it above and save).');
  gateControl($('#set-dkr-preview'), dkrAvailable, 'Docker discovery is turned off (enable it above and save).');

  function renderDockerPayload(st, dry) {
    const el = $('#set-dkr-status');
    if (!el) return;
    const pill = st.reachable === false
      ? `<span class="chip err">Engine unreachable${st.endpoint ? ' - ' + esc(st.endpoint) : ''}</span>`
      : (st.endpoint ? `<span class="chip mono">${esc(st.endpoint)}</span>` : '');
    el.innerHTML = (dry ? '<div class="hint">Dry run, nothing was changed.</div>' : discoveryRunLine(st))
      + (pill ? `<div class="chip-row" style="margin-top:6px">${pill}</div>` : '')
      + (st.error ? `<div class="inline-error">${esc(st.error)}</div>` : '')
      + discoveryCounters(st)
      + discoveryHostsTable(st.hosts, 'Container', 'container');
  }
  async function renderDockerStatus() {
    const el = $('#set-dkr-status');
    if (!el) return;
    if (!dkrAvailable) { el.innerHTML = '<div class="hint">Docker discovery is turned off.</div>'; return; }
    try {
      renderDockerPayload((await api('/api/docker-discovery/status')).data || {}, false);
    } catch (e) {
      el.innerHTML = `<div class="hint">${e && e.status === 501 ? 'Docker discovery is not wired in this build.' : esc('Status unavailable: ' + (e.message || e))}</div>`;
    }
  }
  renderDockerStatus();
  $('#set-dkr-preview').addEventListener('click', async () => {
    const btn = $('#set-dkr-preview'); btn.disabled = true;
    try {
      renderDockerPayload((await api('/api/docker-discovery/plan')).data || {}, true);
    } catch (e) { dockerActionError(e); }
    btn.disabled = false;
  });
  $('#set-dkr-run').addEventListener('click', async () => {
    const btn = $('#set-dkr-run'); btn.disabled = true;
    try {
      renderDockerPayload((await api('/api/docker-discovery/reconcile', { method: 'POST' })).data || {}, false);
      toast('Reconciled', 'Managed proxy hosts are back in step with the Engine.', 'ok');
    } catch (e) { dockerActionError(e); }
    btn.disabled = false;
  });
  function dockerActionError(e) {
    const st = e && e.status;
    if (st === 409) { toast('Already running', 'A reconcile is already running.', 'err'); return; }
    if (st === 501) { toast('Not wired', 'Docker discovery is not wired in this build.', 'err'); return; }
    if (st === 403) { toast('Scope missing', 'This token lacks the docker-discovery scope.', 'err'); return; }
    if (st === 502) {
      const el = $('#set-dkr-status');
      if (el) el.innerHTML = `<div class="inline-error">${esc((e && e.message) || 'Engine error')}</div>`;
      return;
    }
    toastErr(e);
  }

  // ----- Access-list sync -----
  async function renderAlsStatus() {
    const el = $('#int-als-status');
    if (!el) return;
    try {
      const st = (await api('/api/access-list-sources/status')).data || {};
      const rows = arr(st.sources);
      el.innerHTML = rows.length ? `<div class="table-wrap"><table class="mini-table">
        <thead><tr><th>Access list</th><th>Source</th><th>Fetched</th><th>Entries</th><th>Error</th></tr></thead>
        <tbody>${rows.map((r) => `<tr>
          <td><a class="mono" href="#/access/${encodeURIComponent(r.list || '')}">${esc(r.list || '')}</a></td>
          <td class="mono">${esc(r.name || '')}</td>
          <td class="faint">${esc(r.fetchedAt ? relTime(r.fetchedAt) : 'never')}</td>
          <td class="mono">${esc(r.entryCount || 0)}</td>
          <td class="warn-text">${esc(r.lastError || '')}</td>
        </tr>`).join('')}</tbody></table></div>` : '<div class="hint">No access list declares a remote source yet.</div>';
    } catch (e) {
      el.innerHTML = `<div class="hint">${e && e.status === 501 ? 'Access-list source sync is not wired in this deployment.' : esc('Status unavailable: ' + (e.message || e))}</div>`;
    }
  }
  renderAlsStatus();
  $('#int-als-run').addEventListener('click', async () => {
    const btn = $('#int-als-run'); btn.disabled = true;
    try {
      await api('/api/access-list-sources/reconcile', { method: 'POST' });
      toast('Reconciled', 'Access-list sources are back in sync.', 'ok');
    } catch (e) { toastErr(e); }
    await renderAlsStatus();
    btn.disabled = false;
  });

  // ----- Webhooks -----
  // Delivery state is in-memory and per-process; a row that has no entry in the
  // last status response cannot be tested (it is not in the saved config yet),
  // so its Test button is disabled with that reason rather than left to 404.
  let whStatus = [];
  const whWrap = $('#set-webhooks');
  function webhookRow(w) {
    w = w || {};
    const div = document.createElement('div');
    div.className = 'loc-row';
    div.style.marginBottom = '8px';
    div.innerHTML = `
      <input class="field mono wh-name" data-hint="settings.webhooks.name" style="flex:0 0 140px" value="${esc(w.name || '')}" placeholder="name" aria-label="Webhook name" />
      <input class="field mono wh-url" data-hint="settings.webhooks.url" style="flex:2 1 220px" value="${esc(w.url || '')}" placeholder="https://hooks.example.com/x" aria-label="Webhook URL" />
      <input class="field mono wh-secret" data-hint="settings.webhooks.secret" style="flex:1 1 140px" value="${esc(w.secret || '')}" placeholder="secret (\${ENV:...} optional)" aria-label="Webhook secret" />
      <label class="check-item" style="flex:0 0 auto" title="Keep configured but do not fire"><input type="checkbox" class="wh-disabled" data-hint="settings.webhooks.disabled"${w.disabled ? ' checked' : ''}/>off</label>
      <span class="wh-status"></span>
      <button class="btn ghost sm wh-test" type="button">Test</button>
      <button class="icon-btn wh-del" type="button" aria-label="Remove webhook">${ICON.x}</button>`;
    div.querySelector('.wh-del').addEventListener('click', () => div.remove());
    div.querySelector('.wh-test').addEventListener('click', () => testDelivery(div, 'wh', 'Webhook', '/api/webhooks/'));
    whWrap.appendChild(div);
    paintDeliveryRow(div, 'wh', whStatus);
  }
  function paintDeliveryRow(row, pfx, statuses) {
    const name = row.querySelector('.' + pfx + '-name').value.trim();
    const st = arr(statuses).find((x) => x.name === name);
    const disabled = row.querySelector('.' + pfx + '-disabled').checked;
    row.querySelector('.' + pfx + '-status').innerHTML = deliveryChipHtml(st, disabled);
    const btn = row.querySelector('.' + pfx + '-test');
    if (isReadOnly()) gateControl(btn, false, readOnlyReason());
    else gateControl(btn, !!st, st ? '' : 'Save first.');
    if (st) btn.title = 'Send a test now. A disabled target is still tested.';
  }
  function repaintAll(pfx, statuses) {
    $$('#' + (pfx === 'wh' ? 'set-webhooks' : 'set-notifications') + ' .loc-row').forEach((r) => paintDeliveryRow(r, pfx, statuses));
  }
  async function loadWebhookStatus() {
    try { whStatus = arr((await api('/api/webhooks/status')).data); } catch (e) { whStatus = []; }
    repaintAll('wh', whStatus);
  }
  async function testDelivery(row, pfx, kind, base) {
    const name = row.querySelector('.' + pfx + '-name').value.trim();
    const btn = row.querySelector('.' + pfx + '-test');
    btn.disabled = true;
    try {
      const r = (await api(base + encodeURIComponent(name) + '/test', { method: 'POST' })).data || {};
      if (r.ok) toast(kind + ' delivered', `${name} answered HTTP ${r.status} in ${r.durationMs} ms.`, 'ok');
      else if (r.status) toast(kind + ' rejected', `${name} answered HTTP ${r.status}. ${r.error || ''}`, 'err');
      else toast(kind + ' unreachable', `${name} did not answer: ${r.error || 'no response'}`, 'err');
    } catch (e) {
      if (e && e.status === 404) toast('Not saved yet', `Save first - gpm can only test a ${kind.toLowerCase()} that is in the saved configuration.`, 'err');
      else toastErr(e);
    }
    btn.disabled = false;
    if (pfx === 'wh') await loadWebhookStatus(); else await loadNotifyStatus();
  }
  arr(s.webhooks).forEach(webhookRow);
  $('#addWebhook').addEventListener('click', () => webhookRow({}));
  loadWebhookStatus();

  // ----- Notifications -----
  let ntfStatus = [];
  const ntfWrap = $('#set-notifications');
  const NTF_URL_HINTS = {
    ntfy: { ph: 'https://ntfy.example.com/gpm-alerts', hint: 'The topic URL.', secret: 'Access token (optional), sent as Authorization: Bearer <token>.' },
    discord: { ph: 'https://discord.com/api/webhooks/123.../abc...', hint: 'Must contain /api/webhooks/ - the server rejects a URL that does not.', secret: 'Not used for Discord - the webhook URL itself is the credential.' },
    generic: { ph: 'https://hooks.example.com/gpm-notify', hint: 'Any http(s) endpoint. Receives a JSON envelope.', secret: 'Bearer token (optional), sent as Authorization: Bearer <token>.' },
  };
  function notificationRow(t) {
    t = t || {};
    const type = t.type || 'ntfy';
    const div = document.createElement('div');
    div.className = 'loc-row ntf-row';
    div.style.cssText = 'margin-bottom:8px;flex-wrap:wrap';
    div.innerHTML = `
      <input class="field mono ntf-name" data-hint="settings.notifications.targets.name" style="flex:0 0 140px" value="${esc(t.name || '')}" placeholder="name" aria-label="Notification target name" />
      <select class="field ntf-type" data-hint="settings.notifications.targets.type" style="flex:0 0 150px" aria-label="Type">${enumOptions('notificationType', ['ntfy', 'discord', 'generic'], type)}</select>
      <input class="field mono ntf-url" data-hint="settings.notifications.targets.url" style="flex:2 1 220px" value="${esc(t.url || '')}" aria-label="Notification URL" />
      <input class="field mono ntf-secret" data-hint="settings.notifications.targets.secret" style="flex:1 1 160px" value="${esc(t.secret || '')}" placeholder="secret (optional, \${ENV:...} or \${FILE:...})" aria-label="Notification secret" />
      <label class="check-item" style="flex:0 0 auto" title="Keep configured but do not fire"><input type="checkbox" class="ntf-disabled" data-hint="settings.notifications.targets.disabled"${t.disabled ? ' checked' : ''}/>off</label>
      <div class="ntf-actions">
        <span class="ntf-status"></span>
        <button class="btn ghost sm ntf-test" type="button" title="Send a test notification now. A disabled target is still tested.">Test</button>
        <button class="icon-btn ntf-del" type="button" aria-label="Remove notification target">${ICON.x}</button>
      </div>
      <div class="chip-row ntf-events" data-hint="settings.notifications.targets.events" style="flex:1 1 100%"></div>
      <div class="hint ntf-hint" style="flex:1 1 100%"></div>`;
    div.querySelector('.ntf-del').addEventListener('click', () => div.remove());
    div.querySelector('.ntf-test').addEventListener('click', () => testDelivery(div, 'ntf', 'Notification', '/api/notifications/'));
    ntfWrap.appendChild(div);
    div._events = makeEventChips(div.querySelector('.ntf-events'), t.events);
    const typeSel = div.querySelector('.ntf-type');
    const urlIn = div.querySelector('.ntf-url');
    const secretIn = div.querySelector('.ntf-secret');
    function syncType() {
      const h = NTF_URL_HINTS[typeSel.value] || NTF_URL_HINTS.ntfy;
      urlIn.placeholder = h.ph;
      // Discord's webhook URL IS the credential, so the secret field is greyed
      // out AND cleared - a stale value from switching type back and forth must
      // not be sent.
      if (typeSel.value === 'discord') secretIn.value = '';
      gateControl(secretIn, typeSel.value !== 'discord', h.secret);
      div.querySelector('.ntf-hint').innerHTML = `${esc(h.hint)} ${esc(h.secret)} Config changed fires on every save - off by default, opt in per target.`;
    }
    typeSel.addEventListener('change', syncType);
    syncType();
    paintDeliveryRow(div, 'ntf', ntfStatus);
  }
  async function loadNotifyStatus() {
    try { ntfStatus = arr((await api('/api/notifications/status')).data); } catch (e) { ntfStatus = []; }
    repaintAll('ntf', ntfStatus);
  }
  arr(s.notifications && s.notifications.targets).forEach(notificationRow);
  $('#addNotification').addEventListener('click', () => notificationRow({}));
  loadNotifyStatus();

  // ----- shared save -----
  // ONE body builder for all six cards. It starts from the settings object as
  // loaded and overlays only what this page renders, so a save here can never
  // wipe appName, adminAuth, trustedProxies, securityHeaders, maintenance,
  // proxyProtocol or errorPages - none of which have a control on this page.
  $$('.int-save').forEach((b) => b.addEventListener('click', () => saveIntegrations(b)));

  async function saveIntegrations(btn) {
    clearEditorError();
    const body = Object.assign({}, s);

    // accessListSync.enabled defaults to ON server-side (it is a *bool so that
    // "absent" is distinguishable from "false"). It is sent explicitly when the
    // switch is OFF (omitting it there would silently turn the fetcher back on)
    // and when the stored config already carried the key. It is NOT sent for an
    // untouched ON switch on a config that never had the key: writing it would
    // materialise `accessListSync.enabled: true` on a plain save, which is the
    // absent-stays-absent rule this whole builder follows.
    const alsOn = isOn('set-als-on');
    const alsStored = s.accessListSync && s.accessListSync.enabled;
    const alsBody = {};
    if (!alsOn || alsStored === true || alsStored === false) alsBody.enabled = alsOn;
    const alsPollVal = $('#set-als-poll').value.trim();
    if (alsPollVal) alsBody.pollInterval = alsPollVal;
    if (Object.keys(alsBody).length) body.accessListSync = alsBody; else delete body.accessListSync;

    const webhooks = [];
    let whErr = '';
    clearRowErrors(whWrap);
    $$('#set-webhooks .loc-row').forEach((row) => {
      if (whErr) return;
      const nm = row.querySelector('.wh-name').value.trim();
      const url = row.querySelector('.wh-url').value.trim();
      if (!nm && !url) return;
      // A half-filled webhook row used to be sent with an empty name or URL and
      // bounced by the API with no indication of which row; now the row says so.
      if (!nm) { whErr = markRowError(row, 'Name is required.'); return; }
      if (!url) { whErr = markRowError(row, 'URL is required.'); return; }
      if (!/^https?:\/\//.test(url)) { whErr = markRowError(row, 'URL must start with http:// or https://.'); return; }
      const wh = { name: nm, url };
      const secret = row.querySelector('.wh-secret').value.trim();
      if (secret) wh.secret = secret;
      if (row.querySelector('.wh-disabled').checked) wh.disabled = true;
      webhooks.push(wh);
    });
    if (whErr) { toast('Webhook invalid', whErr, 'err'); return; }
    if (webhooks.length) body.webhooks = webhooks; else delete body.webhooks;

    const targets = [];
    let ntfErr = '';
    const ntfSeen = Object.create(null);
    clearRowErrors(ntfWrap);
    $$('#set-notifications .ntf-row').forEach((row) => {
      if (ntfErr) return;
      const nm = row.querySelector('.ntf-name').value.trim();
      const url = row.querySelector('.ntf-url').value.trim();
      const type = row.querySelector('.ntf-type').value;
      if (!nm && !url) return;
      if (!nm) { ntfErr = markRowError(row, 'Name is required.'); return; }
      if (!/^[A-Za-z0-9_-]+$/.test(nm)) { ntfErr = markRowError(row, 'Name may only contain letters, digits, hyphens and underscores.'); return; }
      if (ntfSeen[nm]) { ntfErr = markRowError(row, `Duplicate name "${nm}" - target names must be unique.`); return; }
      ntfSeen[nm] = true;
      if (!url) { ntfErr = markRowError(row, 'URL is required.'); return; }
      if (!/^https?:\/\//.test(url)) { ntfErr = markRowError(row, 'URL must start with http:// or https://.'); return; }
      if (type === 'discord' && url.indexOf('/api/webhooks/') === -1) {
        ntfErr = markRowError(row, 'A Discord webhook URL must contain /api/webhooks/.'); return;
      }
      const t = { name: nm, type, url };
      const secret = type === 'discord' ? '' : row.querySelector('.ntf-secret').value.trim();
      if (secret) t.secret = secret;
      if (row.querySelector('.ntf-disabled').checked) t.disabled = true;
      // Omitted when the chips are exactly the server-side default, so an
      // untouched row does not gain a redundant explicit list in git.
      const ev = row._events.get();
      if (!sameEventSet(ev, defaultNotifyEvents())) t.events = ev;
      targets.push(t);
    });
    if (ntfErr) { toast('Notification target invalid', ntfErr, 'err'); return; }
    const ntfDays = parseInt($('#set-ntf-days').value, 10);
    const notifications = {};
    if (targets.length) notifications.targets = targets;
    if (!isNaN(ntfDays) && ntfDays > 0 && ntfDays !== 14) notifications.expiringThresholdDays = ntfDays;
    if (Object.keys(notifications).length) body.notifications = notifications; else delete body.notifications;

    const dnsSync = { pihole: {}, cloudflare: {} };
    const phPw = $('#set-ph-pw').value.trim();
    if (phPw === '***') {
      // A literal secret is redacted on read; saving it back would commit "***".
      toast('Secret masked', 'The Pi-hole app password reads ***. Replace it with a ${ENV:...} or ${FILE:...} placeholder before saving.', 'err');
      return;
    }
    if (isOn('set-ph-on')) dnsSync.pihole.enabled = true;
    dnsSync.pihole.url = $('#set-ph-url').value.trim();
    dnsSync.pihole.appPassword = phPw;
    dnsSync.pihole.apexTarget = $('#set-ph-apex').value.trim();
    if (isOn('set-cf-on')) dnsSync.cloudflare.enabled = true;
    dnsSync.cloudflare.dnsProviderRef = $('#set-cf-ref').value.trim();
    dnsSync.cloudflare.zoneName = $('#set-cf-zone').value.trim();
    dnsSync.cloudflare.apexTarget = $('#set-cf-apex').value.trim();
    if (isOn('set-cf-proxied')) dnsSync.cloudflare.proxied = true;
    body.dnsSync = dnsSync;

    // annotationPrefix and annotationPrefixMigrate belong to Settings ->
    // Advanced, so they are carried forward from the loaded block rather than
    // rebuilt here; everything else on the ingressDiscovery block is this card's.
    const ingressDiscovery = {
      annotationPrefix: idc.annotationPrefix || '',
      apiURL: $('#set-id-url').value.trim(),
      tokenFile: $('#set-id-token').value.trim(),
      caFile: $('#set-id-ca').value.trim(),
      namespace: $('#set-id-ns').value.trim(),
      labelSelector: $('#set-id-sel').value.trim(),
      pollInterval: $('#set-id-poll').value.trim(),
      allowedDomainSuffixes: idSuffixCtl.get(),
      template: {
        // Mutually exclusive server-side, so send only the one that is filled
        // in: an empty upstream object alongside a group ref would be rejected.
        upstream: $('#set-id-up-group').value.trim() ? undefined
          : upstreamPayload($('#set-id-up-scheme'), $('#set-id-up-host'), $('#set-id-up-port')),
        upstreamGroupRef: $('#set-id-up-group').value.trim() || undefined,
        // INVARIANT: tls is MERGED over what was loaded, never rebuilt from the
        // two fields this form renders. A settings write is a full replacement
        // server-side, and TLSSettings also carries clientAuth (mTLS),
        // minTLSVersion and hsts - none of which have a control here. Rebuilding
        // would strip a GitOps-authored `clientAuth: {caRef: corp-ca, mode:
        // require}` on any unrelated save, and the next reconcile would push that
        // silent downgrade onto every derived host. Any TLS field added to this
        // form must be added to the overlay below, not left to the merge.
        tls: Object.assign({}, idt.tls, {
          certificateRef: $('#set-id-cert').value.trim(),
          forceSSL: isOn('set-id-forcessl'),
        }),
        // http2 and websocketsUpgrade lost their switches (nothing reads either
        // on a derived host: h2 is negotiated by ALPN and WebsocketsUpgrade is
        // unread). http2 rides the tls merge above; websocketsUpgrade sits on a
        // REBUILT literal, so it has to be carried forward by hand or an
        // unrelated save strips it and the next reconcile pushes that onto every
        // derived host.
        websocketsUpgrade: idt.websocketsUpgrade !== undefined ? idt.websocketsUpgrade : undefined,
        robotsNoIndex: isOn('set-id-robots'),
        timeouts: timeoutsPayload(idt.timeouts, $('#set-id-to-connect').value, $('#set-id-to-read').value),
        middlewares: idMwCtl.get(),
        accessLists: idAlCtl.get(),
        tags: idTagCtl.get(),
        // No control on this form yet, and this literal is a REBUILD - so the
        // loaded values have to be sent back or an unrelated settings save
        // strips the template's strip list / security headers and the next
        // reconcile pushes that onto every derived host.
        stripResponseHeaders: arr(idt.stripResponseHeaders).length ? idt.stripResponseHeaders : undefined,
        securityHeaders: Object.keys(idt.securityHeaders || {}).length ? idt.securityHeaders : undefined,
        allowedDomainSuffixes: idTemplateSuffixCtl.get().length ? idTemplateSuffixCtl.get() : undefined,
      },
    };
    if (idc.annotationPrefixMigrate) ingressDiscovery.annotationPrefixMigrate = true;
    if (isOn('set-id-on')) ingressDiscovery.enabled = true;
    if (isOn('set-id-dns-lan') || isOn('set-id-dns-pub')) {
      ingressDiscovery.template.defaultDNS = { lanDirect: isOn('set-id-dns-lan'), publicCname: isOn('set-id-dns-pub') };
    }

    // Null-prototype: the keys are operator-typed, and a name like __proto__ on a
    // plain object would set the prototype instead of a key, silently dropping
    // the profile. Here it becomes a normal key the server rejects by name.
    const profiles = Object.create(null);
    let pfDup = '';
    $$('#set-id-profiles .id-profile').forEach((row) => {
      const pname = row.querySelector('.pf-name').value.trim();
      if (!pname) return; // an untouched blank row is not a profile
      if (pname in profiles) pfDup = pname;
      const uid = row.dataset.uid;
      const group = row.querySelector('.pf-up-group').value.trim();
      const prof = {
        upstream: group ? undefined
          : upstreamPayload(row.querySelector('.pf-up-scheme'), row.querySelector('.pf-up-host'), row.querySelector('.pf-up-port')),
        upstreamGroupRef: group || undefined,
        // Same invariant as the template block above: merge over the loaded
        // profile so an unrelated save cannot strip a clientAuth / minTLSVersion /
        // hsts this form does not render.
        tls: Object.assign({}, (row._orig || {}).tls, {
          certificateRef: row.querySelector('.pf-cert').value.trim(),
          forceSSL: isOn('pf-forcessl-' + uid),
        }),
        // Carried forward like the template's above, and for the same reason.
        websocketsUpgrade: (row._orig || {}).websocketsUpgrade !== undefined ? (row._orig || {}).websocketsUpgrade : undefined,
        robotsNoIndex: isOn('pf-robots-' + uid),
        timeouts: timeoutsPayload((row._orig || {}).timeouts, row.querySelector('.pf-to-connect').value, row.querySelector('.pf-to-read').value),
        middlewares: row._mw.get(),
        accessLists: row._al.get(),
        tags: row._tags.get(),
        // Carried forward from the loaded profile, like the template's above.
        stripResponseHeaders: arr((row._orig || {}).stripResponseHeaders).length ? (row._orig || {}).stripResponseHeaders : undefined,
        securityHeaders: Object.keys((row._orig || {}).securityHeaders || {}).length ? (row._orig || {}).securityHeaders : undefined,
        allowedDomainSuffixes: row._suffixes.get().length ? row._suffixes.get() : undefined,
      };
      if (isOn('pf-dns-lan-' + uid) || isOn('pf-dns-pub-' + uid)) {
        prof.defaultDNS = { lanDirect: isOn('pf-dns-lan-' + uid), publicCname: isOn('pf-dns-pub-' + uid) };
      }
      profiles[pname] = prof;
    });
    if (pfDup) {
      toast('Duplicate profile', `Two discovery profiles are both called "${pfDup}". Profile names must be unique - an Ingress selects one by name.`, 'err');
      return;
    }
    if (Object.keys(profiles).length) ingressDiscovery.profiles = profiles;

    const profSel = $('#set-id-profile-selection').value;
    if (profSel) ingressDiscovery.profileSelection = profSel;
    const profileRules = [];
    $$('#set-id-rules .loc-row').forEach((row) => {
      const ns = row.querySelector('.rule-ns').value.trim();
      const labelsRaw = row.querySelector('.rule-labels').value.trim();
      const profile = row.querySelector('.rule-profile').value.trim();
      if (!ns && !labelsRaw && !profile) return; // an untouched blank row is not a rule
      const rule = {};
      if (ns) rule.namespace = ns;
      if (labelsRaw) {
        const ml = Object.create(null);
        labelsRaw.split(',').forEach((pair) => {
          const idx = pair.indexOf('=');
          if (idx <= 0) return;
          ml[pair.slice(0, idx).trim()] = pair.slice(idx + 1).trim();
        });
        if (Object.keys(ml).length) rule.matchLabels = ml;
      }
      if (profile) rule.profile = profile;
      profileRules.push(rule);
    });
    if (profileRules.length) ingressDiscovery.profileRules = profileRules;
    body.ingressDiscovery = ingressDiscovery;

    // Docker discovery. Mirrors the ingress block, including both merge
    // invariants; the template carries no upstream because a container's address
    // is its own.
    const dkrOn = isOn('set-dkr-on');
    const dkrSocket = $('#set-dkr-socket').value.trim();
    const dkrHost = $('#set-dkr-host').value.trim();
    const dkrCert = $('#set-dkr-tlscert').value.trim();
    const dkrKey = $('#set-dkr-tlskey').value.trim();
    const dkrSuffixes = dkrSuffixCtl.get();
    if (dkrOn && !dkrSuffixes.length) { toast('Docker discovery', 'allowedDomainSuffixes is required when discovery is enabled', 'err'); return; }
    if (dkrSocket && dkrSocket[0] !== '/') { toast('Docker discovery', 'socket must be an absolute path', 'err'); return; }
    if (dkrHost && !/^(tcp|https):\/\//.test(dkrHost)) { toast('Docker discovery', 'host must be an absolute tcp:// or https:// URL (e.g. tcp://socket-proxy:2375)', 'err'); return; }
    if (!!dkrCert !== !!dkrKey) { toast('Docker discovery', 'tlsCert and tlsKey must be set together', 'err'); return; }
    if (dkrCert && !/^https:\/\//.test(dkrHost)) { toast('Docker discovery', 'tlsCert/tlsKey need an https:// host', 'err'); return; }
    if ($('#set-dkr-pubhost').value.trim() && !isOn('set-dkr-pubports')) { toast('Docker discovery', 'publishedHost only applies with usePublishedPorts: true', 'err'); return; }
    if ($('#set-dkr-network').value.trim() && isOn('set-dkr-pubports')) { toast('Docker discovery', 'network and usePublishedPorts are mutually exclusive', 'err'); return; }
    if (dkrOn && !$('#set-dkr-cert').value.trim()) { toast('Docker discovery', 'template.tls.certificateRef is required when discovery is enabled', 'err'); return; }
    const dockerDiscovery = {
      socket: dkrSocket || undefined,
      host: dkrHost || undefined,
      tlsCert: dkrCert || undefined,
      tlsKey: dkrKey || undefined,
      tlsCA: $('#set-dkr-tlsca').value.trim() || undefined,
      network: $('#set-dkr-network').value.trim() || undefined,
      publishedHost: $('#set-dkr-pubhost').value.trim() || undefined,
      pollInterval: $('#set-dkr-poll').value.trim() || undefined,
      allowedDomainSuffixes: dkrSuffixes.length ? dkrSuffixes : undefined,
      template: {
        tls: Object.assign({}, ddt.tls, {
          certificateRef: $('#set-dkr-cert').value.trim(),
          forceSSL: isOn('set-dkr-forcessl'),
        }),
        websocketsUpgrade: ddt.websocketsUpgrade !== undefined ? ddt.websocketsUpgrade : undefined,
        robotsNoIndex: isOn('set-dkr-robots'),
        timeouts: timeoutsPayload(ddt.timeouts, $('#set-dkr-to-connect').value, $('#set-dkr-to-read').value),
        middlewares: dkrMwCtl.get(),
        accessLists: dkrAlCtl.get(),
        tags: dkrTagCtl.get(),
        stripResponseHeaders: arr(ddt.stripResponseHeaders).length ? ddt.stripResponseHeaders : undefined,
        securityHeaders: Object.keys(ddt.securityHeaders || {}).length ? ddt.securityHeaders : undefined,
        allowedDomainSuffixes: dkrTemplateSuffixCtl.get().length ? dkrTemplateSuffixCtl.get() : undefined,
      },
    };
    if (dkrOn) dockerDiscovery.enabled = true;
    if (isOn('set-dkr-pubports')) dockerDiscovery.usePublishedPorts = true;
    if (isOn('set-dkr-stopped')) dockerDiscovery.includeStopped = true;
    if (isOn('set-dkr-dns-lan') || isOn('set-dkr-dns-pub')) {
      dockerDiscovery.template.defaultDNS = { lanDirect: isOn('set-dkr-dns-lan'), publicCname: isOn('set-dkr-dns-pub') };
    }
    const dkrProfiles = Object.create(null);
    let dpfDup = '';
    $$('#set-dkr-profiles .dkr-profile').forEach((row) => {
      const pname = row.querySelector('.dpf-name').value.trim();
      if (!pname) return;
      if (pname in dkrProfiles) dpfDup = pname;
      const uid = row.dataset.uid;
      const prof = {
        tls: Object.assign({}, (row._orig || {}).tls, {
          certificateRef: row.querySelector('.dpf-cert').value.trim(),
          forceSSL: isOn('dpf-forcessl-' + uid),
        }),
        websocketsUpgrade: (row._orig || {}).websocketsUpgrade !== undefined ? (row._orig || {}).websocketsUpgrade : undefined,
        robotsNoIndex: isOn('dpf-robots-' + uid),
        timeouts: timeoutsPayload((row._orig || {}).timeouts, row.querySelector('.dpf-to-connect').value, row.querySelector('.dpf-to-read').value),
        middlewares: row._mw.get(),
        accessLists: row._al.get(),
        tags: row._tags.get(),
        stripResponseHeaders: arr((row._orig || {}).stripResponseHeaders).length ? (row._orig || {}).stripResponseHeaders : undefined,
        securityHeaders: Object.keys((row._orig || {}).securityHeaders || {}).length ? (row._orig || {}).securityHeaders : undefined,
        allowedDomainSuffixes: row._suffixes.get().length ? row._suffixes.get() : undefined,
      };
      if (isOn('dpf-dns-lan-' + uid) || isOn('dpf-dns-pub-' + uid)) {
        prof.defaultDNS = { lanDirect: isOn('dpf-dns-lan-' + uid), publicCname: isOn('dpf-dns-pub-' + uid) };
      }
      dkrProfiles[pname] = prof;
    });
    if (dpfDup) {
      toast('Duplicate profile', `Two Docker discovery profiles are both called "${dpfDup}". Profile names must be unique.`, 'err');
      return;
    }
    if (Object.keys(dkrProfiles).length) dockerDiscovery.profiles = dkrProfiles;
    body.dockerDiscovery = dockerDiscovery;

    btn.disabled = true;
    try {
      const r = await api('/api/settings', { method: 'PUT', body });
      // The per-route memo now holds a settings object this save superseded.
      resetRouteMemo();
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      // ingressDiscovery.enabled and dockerDiscovery.enabled are capability
      // probe values this save can flip. Dropping the cache is not enough on
      // its own: a null cache reads as "every capability false" for every view
      // that does not reload it, so re-read it here and now.
      state.capabilities = null;
      await loadCapabilities();
      refreshShellBanners();
      await loadWebhookStatus();
      await loadNotifyStatus();
    } catch (e) { showSaveError(e, 'Could not save these integrations'); }
    btn.disabled = false;
  }
}

// ---------- OVERVIEW ----------
// The get-started checklist. Five steps, in the order a first deployment
// actually needs them, each a deep link into the editor that closes it. It is
// derived state only (GET /api/config/summary plus the settings object), so it
// never disagrees with what the config holds, and it disappears on its own once
// every step is done - a checklist that outlives its usefulness is clutter.
const CHECKLIST_KEY = 'gpm.overview.checklist';
function checklistDismissed() {
  try { return localStorage.getItem(CHECKLIST_KEY) === 'dismissed'; } catch (e) { return false; }
}
function getStartedSteps(counts, settings, cfg) {
  const hosts = arr(cfg.proxyHosts);
  const mwAuth = {};
  arr(cfg.middlewares).forEach((m) => { if (m.type === 'auth') mwAuth[m.name] = true; });
  const guarded = hosts.some((h) => h.auth || arr(h.accessLists).length || arr(h.middlewares).some((n) => mwAuth[n]));
  return [
    {
      id: 'url', label: 'Set the public URL of this panel',
      done: !!(settings && settings.externalBaseURL),
      href: '#/settings/general',
      why: 'OIDC builds its redirect_uri from it, so admin sign-in needs it before anything else.',
    },
    {
      id: 'dns', label: 'Add a DNS provider, or plan on HTTP-01',
      done: (counts.dnsProviders || 0) > 0 || (counts.certificates || 0) > 0,
      href: '#/dns/_new',
      why: 'DNS-01 is the only way to get a wildcard. HTTP-01 needs no provider, just port 80 reachable.',
    },
    {
      id: 'cert', label: 'Add your first certificate',
      done: (counts.certificates || 0) > 0,
      href: '#/certs/_new',
      why: 'A host never picks a certificate: the handshake selects by domain, so one covering the name is all it needs.',
    },
    {
      id: 'host', label: 'Add your first proxy host',
      done: (counts.proxyHosts || 0) > 0,
      href: '#/hosts/_new',
      why: 'One public domain, one internal upstream. This is where most day-to-day work happens.',
    },
    {
      id: 'guard', label: 'Protect it with an access list or a sign-in',
      done: guarded,
      href: '#/access/_new',
      why: 'An access list gates by IP or country; a Sign-in fold on the host gates by identity.',
    },
  ];
}
function getStartedCardHtml(steps) {
  const done = steps.filter((s) => s.done).length;
  return `<div class="card form-section" id="getstarted" style="margin-bottom:16px">
    <div class="card-head">
      <div>
        <p class="section-label" style="margin:0 0 2px">Get started</p>
        <h3 style="margin:0">${done} of ${steps.length} done</h3>
      </div>
      <button class="btn ghost sm" id="gs-dismiss" type="button" title="Hide this card. It stays hidden in this browser.">Dismiss</button>
    </div>
    <ol class="checklist">
      ${steps.map((s) => `<li class="${s.done ? 'done' : ''}">
        <span class="ck-mark" aria-hidden="true">${s.done ? '&#10003;' : ''}</span>
        <div class="ck-body">
          <a href="${esc(s.href)}">${esc(s.label)}</a>
          <div class="muted" style="font-size:11.5px">${esc(s.why)}</div>
        </div>
      </li>`).join('')}
    </ol>
  </div>`;
}

async function viewOverview(c) {
  // Five independent reads, issued together. /api/health is the only real
  // liveness signal the SPA has, so a failure there degrades this page's Data
  // plane tile rather than failing the whole view. /api/certificates (not
  // /api/config's plain stored objects) is what carries notAfter/state/
  // daysRemaining, so the certificates card below can show real expiry instead
  // of "-".
  const [cfgR, , histR, healthR, setR, certsR] = await Promise.all([
    api('/api/config'),
    loadCapabilities(),
    routeHistory(),
    api('/api/health').catch(() => null),
    routeSettings(),
    api('/api/certificates'),
  ]);
  const cfg = cfgR.data || {};
  const history = arr(histR);
  const health = (healthR && healthR.data) || null;
  const settings = setR.data || {};
  const certsWithStatus = arr(certsR.data);
  await loadCounts();

  const hosts = arr(cfg.proxyHosts);
  const liveHosts = hosts.filter((h) => !h.disabled).length;
  const certs = arr(cfg.certificates);
  const idps = arr(cfg.identityProviders);
  const idpSub = idps.length ? idps.map((p) => p.name).join(', ') : 'none configured';

  // Upstreams tile. If the process is down the admin panel is down too, so a
  // tile just claiming "listening" told the operator nothing; this one
  // carries information instead, from GET /api/health's upstreamGroups: the
  // healthy/unhealthy member counts across every configured group.
  const upGroups = arr(health && health.upstreamGroups);
  const upHealthy = upGroups.reduce((n, g) => n + (g.healthy || 0), 0);
  const upUnhealthy = upGroups.reduce((n, g) => n + (g.unhealthy || 0), 0);
  const upColor = !upGroups.length ? 'var(--faint)' : (upUnhealthy ? 'var(--err)' : 'var(--ok)');
  const upTitle = !upGroups.length
    ? 'No upstream groups are configured.'
    : `${upHealthy} of ${upHealthy + upUnhealthy} upstream group members are healthy.`
      + (upUnhealthy ? ` Unhealthy in: ${upGroups.filter((g) => g.unhealthy > 0).map((g) => g.name).join(', ')}.` : '');

  const hc = (health && health.certificates) || null;
  const certProblems = hc ? ((hc.expiring || 0) + (hc.expired || 0) + (hc.error || 0)) : 0;
  const certProblemText = hc ? [
    hc.expiring ? `<b>${hc.expiring}</b> expiring soon` : '',
    hc.expired ? `<b>${hc.expired}</b> expired` : '',
    hc.error ? `<b>${hc.error}</b> renewal error` : '',
  ].filter(Boolean).join(', ') : '';

  const feed = arr(history).slice(0, 6).map((h) => `
    <div class="feed-row">
      <span class="feed-tick"></span>
      <div class="feed-body">
        <div class="feed-meta">${esc(fmtTime(h.when))} &middot; ${esc(h.author || 'unknown')}</div>
        <div class="feed-msg">${esc(h.message || '(no message)')}</div>
        <div class="feed-actions"><span class="sha">${esc(shortSha(h.hash))}</span></div>
      </div>
    </div>`).join('') || `<div class="muted" style="font-size:13px">No commits yet.</div>`;

  // Soonest-expiring first: ascending daysRemaining (negative - already
  // expired - sorts to the very top, since that is the most urgent). A cert
  // with no daysRemaining yet (pending, or an older API build) has no expiry
  // to sort by, so it drops to the end, ordered by name for a stable list.
  const certsSoonest = certsWithStatus.slice().sort((a, b) => {
    const ad = a.daysRemaining, bd = b.daysRemaining;
    if (ad == null && bd == null) return (a.name || '').localeCompare(b.name || '');
    if (ad == null) return 1;
    if (bd == null) return -1;
    return ad - bd;
  });
  const certRows = certsSoonest.length ? certsSoonest.slice(0, 5).map((ct) => {
    const domains = arr(ct.domains).join(', ');
    const typ = ct.type === 'acme' ? 'ACME' : (ct.type === 'custom' ? 'Custom' : (ct.type || 'cert'));
    const detail = ct.type === 'acme'
      ? (ct.acme && ct.acme.dnsProvider ? `DNS-01 via ${ct.acme.dnsProvider}` : 'ACME')
      : 'Custom certificate';
    return `<div class="cert-row">
      <span class="cert-ico">${ICON.cert.replace('stroke="currentColor"', 'stroke="var(--ok)"')}</span>
      <div style="flex:1;min-width:0">
        <div class="host" style="font-size:14px">${esc(ct.name)} <span class="mono muted" style="font-weight:400;font-size:12px">${esc(domains)}</span></div>
        <div class="muted" style="font-size:11.5px">${esc(typ)} &middot; ${esc(detail)}</div>
      </div>
      <div>${certExpiryCellHtml(ct)}</div>
    </div>`;
  }).join('') : `<div class="muted" style="font-size:13px">No certificates yet.</div>`;

  const steps = getStartedSteps(state.counts, settings, cfg);
  const showChecklist = !checklistDismissed() && steps.some((s) => !s.done);

  c.innerHTML = `
    <div class="view-head">
      <h2>Overview</h2>
      <p>Status of <span class="mono">${esc(state.instance)}</span>: hosts, certificates and recent changes.</p>
    </div>
    ${aboutPageHtml('page.overview')}
    ${showChecklist ? getStartedCardHtml(steps) : ''}
    <div class="stat-grid">
      <div class="stat s-ok">
        <div class="k">Proxy hosts</div>
        <div class="v">${hosts.length}</div>
        <div class="sub"><b>${liveHosts}</b> live &middot; ${hosts.length - liveHosts} disabled</div>
      </div>
      <${certProblems > 0 ? 'a href="#/certs"' : 'div'} class="stat s-warn">
        <div class="k">Certificates</div>
        <div class="v">${certs.length}</div>
        <div class="sub"><b>${certs.filter((x) => x.type === 'acme').length}</b> ACME &middot; ${certs.filter((x) => x.type === 'custom').length} custom</div>
        ${certProblems > 0 ? `<div class="sub warn-text">${certProblemText}</div>` : ''}
      </${certProblems > 0 ? 'a' : 'div'}>
      <div class="stat s-cyan">
        <div class="k">Identity providers</div>
        <div class="v">${idps.length}</div>
        <div class="sub">${esc(idpSub)}</div>
      </div>
      <div class="stat" title="${esc(upTitle)}">
        <div class="k">Upstreams</div>
        <div class="v" style="color:${upColor}">${upGroups.length ? (upHealthy + upUnhealthy) : '-'}</div>
        <div class="sub">${upGroups.length
          ? `<b>${upHealthy}</b> healthy${upUnhealthy ? ` &middot; <span class="err-text">${upUnhealthy}</span> unhealthy` : ''}`
          : 'no groups'}</div>
      </div>
    </div>
    <div class="grid-2-1">
      <div class="card">
        <div class="card-head">
          <div>
            <p class="section-label" style="margin:0 0 2px">Recent config changes</p>
            <h3 style="margin:0">Git-backed history</h3>
          </div>
          <a class="btn ghost sm" href="#/history">View all</a>
        </div>
        <div>${feed}</div>
      </div>
      <div class="card">
        <div class="card-head">
          <p class="section-label" style="margin:0">Certificates</p>
          <a class="btn ghost sm" href="#/certs">View all</a>
        </div>
        <div>${certRows}</div>
      </div>
    </div>`;

  const dismiss = $('#gs-dismiss');
  if (dismiss) {
    dismiss.addEventListener('click', () => {
      try { localStorage.setItem(CHECKLIST_KEY, 'dismissed'); } catch (e) { /* ignore */ }
      const card = $('#getstarted');
      if (card) card.remove();
    });
  }
}

// ---------- PROXY HOSTS LIST ----------
// The list an operator lives in. Three things it now does that it did not:
//   - the text filter matches the UNDERLYING values (name, display name,
//     domains, upstream, certificate, tags, status), not the rendered chip
//     text - matching the rendering meant typing "live" hid every disabled host
//     for the wrong reason, and a chip label change silently changed what a
//     search found;
//   - the columns sort;
//   - rows select, so the four bulk edits an operator otherwise does by opening
//     twenty editors in turn (enable, disable, add a tag, remove a tag) are one
//     confirmed action with a progress toast.
const HOSTS_SORT_KEY = 'gpm.hosts.sort';

// managedBy reads the discovery ownership label off a host, whatever the
// deployment's annotation prefix is: the key always ends in "/managed-by" and
// only the VALUE distinguishes the two reconcilers.
function managedBy(h) {
  const labels = (h && h.labels) || {};
  for (const k of Object.keys(labels)) {
    if (k === 'managed-by' || k.endsWith('/managed-by')) return labels[k];
  }
  return '';
}
const MANAGED_CHIPS = {
  'ingress-discovery': { text: 'ingress', title: 'Derived from an annotated Kubernetes Ingress. Edits other than disable and maintenance are reverted on the next reconcile.' },
  'docker-discovery': { text: 'docker', title: 'Derived from a labelled Docker container. Edits other than disable and maintenance are reverted on the next reconcile.' },
};

async function listHosts(c) {
  const [hostsR, mwR] = await Promise.all([
    api('/api/proxy-hosts'),
    refList('/api/middlewares', 'middlewares'),
  ]);
  const hosts = arr(hostsR.data);
  const mwType = {};
  arr(mwR).forEach((m) => { mwType[m.name] = m.type; });

  const head = viewHead('Proxy Hosts',
    'One public domain routed to one internal upstream, with the TLS, access and middleware chain that sits in between. This is where most day-to-day work happens.',
    `<a class="btn primary" href="#/hosts/_new">${ICON.plus}Add proxy host</a>`)
    + aboutPageHtml('page.proxyHosts');

  if (!hosts.length) {
    c.innerHTML = head + emptyState('No proxy hosts yet',
      'Add one with the domain your visitors type and the address of the service behind it - gpm picks the certificate by domain, so TLS needs no wiring here.',
      'Add proxy host', '#/hosts/_new');
    return;
  }

  // zone (domain-group) filter chips: excluded zones persist across reloads.
  const ZONES_OFF_KEY = 'gpm.hosts.zonesOff';
  let zonesOff = new Set();
  try {
    const raw = localStorage.getItem(ZONES_OFF_KEY);
    if (raw) zonesOff = new Set(JSON.parse(raw));
  } catch (e) { /* ignore */ }
  function saveZonesOff() {
    try { localStorage.setItem(ZONES_OFF_KEY, JSON.stringify(Array.from(zonesOff))); } catch (e) { /* ignore */ }
  }

  const zoneCounts = {};
  // One record per host, holding the values the filter, the sort and the row
  // renderer all read - so all three agree on what a column means.
  const recs = hosts.map((h) => {
    const domains = arr(h.domains);
    const zones = Array.from(new Set(domains.map(domainZone).filter(Boolean)));
    zones.forEach((z) => { zoneCounts[z] = (zoneCounts[z] || 0) + 1; });
    const up = h.upstream || {};
    const upStr = h.upstreamGroupRef ? `group:${h.upstreamGroupRef}` : `${up.host || '?'}:${up.port != null ? up.port : '?'}`;
    const certRef = (h.tls && h.tls.certificateRef) || '';
    const authMw = arr(h.middlewares).find((m) => mwType[m] === 'auth') || '';
    // "This host requires sign-in" is ONE fact whichever way it was configured,
    // so an inline auth block reads the same in this column as an attached auth
    // middleware does.
    const authInline = !!h.auth;
    const status = h.disabled ? 'disabled' : (h.maintenance ? 'maintenance' : 'live');
    const tags = arr(h.tags);
    const managed = managedBy(h);
    return {
      h, domains, zones, upStr, certRef, authMw, authInline, managed, status, tags,
      name: h.name || '',
      display: h.displayName || '',
      updated: h.updatedAt || '',
      // The filter haystack: every value the row is BUILT from, never the
      // markup it renders to.
      blob: [h.name, h.displayName, domains.join(' '), upStr, certRef, authMw, authInline ? 'sign-in' : '', managed, status, tags.join(' ')]
        .join(' ').toLowerCase(),
    };
  });
  const zoneEntries = Object.keys(zoneCounts)
    .map((z) => [z, zoneCounts[z]])
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const showZones = zoneEntries.length >= 2;

  let sort = { key: 'name', dir: 1 };
  try {
    const raw = localStorage.getItem(HOSTS_SORT_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (parsed && parsed.key) sort = { key: parsed.key, dir: parsed.dir === -1 ? -1 : 1 };
    }
  } catch (e) { /* ignore */ }

  const SORTS = {
    name: (r) => (r.display || r.name).toLowerCase(),
    domains: (r) => (r.domains[0] || r.name).toLowerCase(),
    upstream: (r) => r.upStr.toLowerCase(),
    updated: (r) => r.updated || '',
  };
  const selected = new Set();

  const COLUMNS = [
    { key: 'domains', label: 'Domain' },
    { key: 'upstream', label: 'Upstream' },
    { key: null, label: 'TLS' },
    { key: null, label: 'Auth' },
    { key: null, label: 'Status' },
    { key: 'updated', label: 'Updated' },
    { key: null, label: '' },
  ];
  function headHtml() {
    return `<tr><th class="sel-cell"><input type="checkbox" id="hostSelAll" aria-label="Select all listed hosts" /></th>`
      + COLUMNS.map((col) => {
        if (!col.key) return `<th>${esc(col.label)}</th>`;
        const active = sort.key === col.key;
        const aria = active ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none';
        return `<th class="sortable" data-sort="${col.key}" aria-sort="${aria}" role="button" tabindex="0">${esc(col.label)}<span class="sort-mark">${active ? (sort.dir === 1 ? '&#9650;' : '&#9660;') : '&#9650;'}</span></th>`;
      }).join('')
      + `</tr>`;
  }

  function visible() {
    const q = (($('#hostFilter') && $('#hostFilter').value) || '').trim().toLowerCase();
    const out = recs.filter((r) => {
      const zoneOk = r.zones.length === 0 || r.zones.some((z) => !zonesOff.has(z));
      return zoneOk && (!q || r.blob.indexOf(q) !== -1);
    });
    const get = SORTS[sort.key] || SORTS.name;
    return out.sort((a, b) => {
      const av = get(a), bv = get(b);
      if (av === bv) return a.name.localeCompare(b.name);
      return (av < bv ? -1 : 1) * sort.dir;
    });
  }

  function rowHtml(r) {
    const h = r.h;
    const extra = r.domains.length > 1 ? ` +${r.domains.length - 1}` : '';
    const tls = r.certRef
      ? `<span class="lock ok">${ICON.lock}${esc(r.certRef)}</span>`
      : `<span class="chip">none</span>`;
    const auth = r.authMw
      ? `<span class="chip brand">${esc(r.authMw)}</span>`
      : (r.authInline ? `<span class="chip brand" title="Configured inline on this host, under Sign-in.">sign-in</span>` : `<span class="chip">none</span>`);
    // live/disabled are the quiet states (dot + text, no pill); maintenance is
    // the one that should draw the eye, so it keeps the filled/bordered pill.
    const status = r.status === 'disabled'
      ? `<span class="chip flat muted"><span class="dot muted"></span>disabled</span>`
      : (r.status === 'maintenance'
        ? `<span class="chip warn"><span class="dot warn"></span>maintenance</span>`
        : `<span class="chip flat ok"><span class="dot ok"></span>live</span>`);
    const tagChips = r.tags.map((t) => `<span class="chip tag">${esc(t)}</span>`).join(' ');
    const mc = MANAGED_CHIPS[r.managed];
    const managedChip = mc ? `<span class="chip managed" title="${esc(mc.title)}">${esc(mc.text)}</span>` : '';
    return `<tr class="clickable" data-name="${esc(h.name)}">
      <td class="sel-cell"><input type="checkbox" class="host-sel" data-name="${esc(h.name)}" aria-label="Select ${esc(h.name)}"${selected.has(h.name) ? ' checked' : ''} /></td>
      <td><span class="host">${esc(r.domains[0] || h.name)}${esc(extra)}</span> ${managedChip}${r.display ? `<div class="faint" style="font-size:11px">${esc(r.display)}</div>` : ''}${tagChips ? `<div style="margin-top:3px;display:flex;gap:4px;flex-wrap:wrap">${tagChips}</div>` : ''}</td>
      <td class="mono">${esc(r.upStr)}</td>
      <td>${tls}</td>
      <td>${auth}</td>
      <td>${status}</td>
      <td class="mono faint" style="white-space:nowrap">${esc(r.updated ? fmtTime(r.updated) : '')}</td>
      <td><button class="btn ghost sm host-clone" data-name="${esc(h.name)}" type="button">Clone</button></td>
    </tr>`;
  }

  function zoneChipsInner() {
    return (zonesOff.size ? `<button type="button" class="chip zone all" data-zone-all="1">all</button>` : '')
      + zoneEntries.map(([z, n]) => `<button type="button" class="chip zone${zonesOff.has(z) ? ' off' : ''}" data-zone="${esc(z)}">${esc(z)} (${n})</button>`).join('');
  }
  const zoneChipsHtml = showZones ? `<div class="chip-row" id="zoneChips">${zoneChipsInner()}</div>` : '';

  c.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="hostFilter" placeholder="filter: domain, upstream, certificate, tag, status..." aria-label="Filter hosts" /></div>
      ${zoneChipsHtml}
    </div>
    <div id="hostBulk"></div>
    <div class="table-wrap">
      <table>
        <thead id="hostHead">${headHtml()}</thead>
        <tbody id="hostRows"></tbody>
      </table>
    </div>`;

  const bulkWrap = $('#hostBulk');
  function renderBulk() {
    if (!selected.size) { bulkWrap.innerHTML = ''; return; }
    bulkWrap.innerHTML = `<div class="bulk-bar">
      <span class="bulk-count"><b>${selected.size}</b> selected</span>
      <button class="btn ghost sm" data-bulk="enable" type="button">Enable</button>
      <button class="btn ghost sm" data-bulk="disable" type="button">Disable</button>
      <button class="btn ghost sm" data-bulk="tag-add" type="button">Add tag</button>
      <button class="btn ghost sm" data-bulk="tag-remove" type="button">Remove tag</button>
      <button class="btn ghost sm" data-bulk="clear" type="button">Clear</button>
    </div>`;
    bulkWrap.querySelectorAll('[data-bulk]').forEach((b) => {
      b.addEventListener('click', () => runBulk(b.dataset.bulk));
    });
    // The bar is built on selection, after applyReadOnlyGating has already run
    // over this view, so a follower's gating is re-applied here (the banner is
    // already on the page; only the controls need it). "Clear" only deselects,
    // so it stays live.
    if (hasCapability('ha.readOnly')) {
      const role = (state.capabilities && state.capabilities.ha && state.capabilities.ha.role) || 'follower';
      bulkWrap.querySelectorAll('[data-bulk]:not([data-bulk="clear"])').forEach((el) => {
        gateControl(el, false, `This instance runs as an HA ${role} and is read-only. Make config changes on the leader.`);
      });
    }
  }

  function render() {
    const rows = visible();
    $('#hostHead').innerHTML = headHtml();
    $('#hostRows').innerHTML = rows.length
      ? rows.map(rowHtml).join('')
      : `<tr><td colspan="8" class="list-empty">No host matches this filter.</td></tr>`;
    wireRows();
    wireHead(rows);
    renderBulk();
  }

  function wireHead(rows) {
    const all = $('#hostSelAll');
    if (all) {
      all.checked = rows.length > 0 && rows.every((r) => selected.has(r.name));
      all.addEventListener('change', () => {
        rows.forEach((r) => { if (all.checked) selected.add(r.name); else selected.delete(r.name); });
        render();
      });
    }
    $$('#hostHead th.sortable').forEach((th) => {
      const apply = () => {
        const key = th.dataset.sort;
        sort = { key, dir: sort.key === key ? -sort.dir : 1 };
        try { localStorage.setItem(HOSTS_SORT_KEY, JSON.stringify(sort)); } catch (e) { /* ignore */ }
        render();
      };
      th.addEventListener('click', apply);
      th.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); apply(); } });
    });
  }

  function wireRows() {
    $$('#hostRows tr.clickable').forEach((tr) => {
      tr.addEventListener('click', (e) => {
        if (e.target.closest('.host-clone') || e.target.closest('.sel-cell')) return;
        location.hash = '#/hosts/' + encodeURIComponent(tr.dataset.name);
      });
    });
    $$('#hostRows .host-sel').forEach((box) => {
      box.addEventListener('change', () => {
        if (box.checked) selected.add(box.dataset.name); else selected.delete(box.dataset.name);
        const all = $('#hostSelAll');
        if (all) all.checked = $$('#hostRows .host-sel').every((b) => b.checked);
        renderBulk();
      });
    });
    $$('#hostRows .host-clone').forEach((b) => {
      b.addEventListener('click', (e) => {
        e.stopPropagation();
        const h = hosts.find((x) => x.name === b.dataset.name);
        if (h) startClone('hosts', h);
      });
    });
  }

  // Bulk edits are ordinary whole-object PUTs, one per host and in order: the
  // API has no batch endpoint, each write is its own git commit, and a partial
  // run has to leave the hosts it already changed changed. So the progress
  // toast counts them, and a failure stops and says which host it stopped on
  // rather than carrying on and reporting a tidy total.
  async function runBulk(action) {
    if (action === 'clear') { selected.clear(); render(); return; }
    const names = Array.from(selected);
    const targets = names.map((n) => hosts.find((h) => h.name === n)).filter(Boolean);
    if (!targets.length) return;
    const many = `<b>${targets.length}</b> proxy host${targets.length === 1 ? '' : 's'}`;
    let mutate, title, confirmLabel, body, tag = '';

    if (action === 'enable' || action === 'disable') {
      const off = action === 'disable';
      title = off ? 'Disable the selected hosts?' : 'Enable the selected hosts?';
      confirmLabel = off ? 'Disable hosts' : 'Enable hosts';
      body = off
        ? `<p>${many} stop being served: gpm answers on their domains as if they were not configured. Their config is kept.</p><p>Each host is committed to git separately.</p>`
        : `<p>${many} start being served again with the config they already hold.</p><p>Each host is committed to git separately.</p>`;
      mutate = (h) => { const o = Object.assign({}, h); if (off) o.disabled = true; else delete o.disabled; return o; };
    } else if (action === 'tag-add' || action === 'tag-remove') {
      const add = action === 'tag-add';
      const answered = await confirmModal({
        title: add ? 'Add a tag to the selected hosts?' : 'Remove a tag from the selected hosts?',
        body: `<p>Applied to ${many}. Tags are free-form labels used for grouping and filtering; they change nothing about how a host is served.</p>`,
        confirmLabel: add ? 'Add tag' : 'Remove tag',
        danger: false,
        prompt: { label: add ? 'Tag to add' : 'Tag to remove', placeholder: 'e.g. internal' },
      });
      if (!answered) return;
      tag = String(answered);
      mutate = (h) => {
        const o = Object.assign({}, h);
        const tags = arr(h.tags).slice();
        if (add) { if (tags.indexOf(tag) === -1) tags.push(tag); } else {
          const i = tags.indexOf(tag);
          if (i !== -1) tags.splice(i, 1);
        }
        if (tags.length) o.tags = tags; else delete o.tags;
        return o;
      };
    } else {
      return;
    }

    if (action === 'enable' || action === 'disable') {
      const ok = await confirmModal({ title, body, confirmLabel, danger: action === 'disable' });
      if (!ok) return;
    }

    const prog = progressToast(action === 'tag-add' || action === 'tag-remove' ? 'Updating tags' : 'Updating hosts');
    let done = 0;
    for (const h of targets) {
      prog.step(`${done}/${targets.length} - ${h.name}`);
      try {
        await api('/api/proxy-hosts/' + encodeURIComponent(h.name), { method: 'PUT', body: mutate(h) });
        done++;
      } catch (e) {
        prog.done('Stopped', `${done} of ${targets.length} updated. ${h.name}: ${e && e.message ? e.message : e}`, 'err');
        refreshHeadSha();
        await listHosts(c);
        return;
      }
    }
    prog.done('Done', `${done} host${done === 1 ? '' : 's'} updated and committed.`, 'ok');
    refreshHeadSha();
    await listHosts(c);
  }

  const filter = $('#hostFilter');
  filter.addEventListener('input', render);
  const zoneChips = $('#zoneChips');
  if (zoneChips) {
    zoneChips.addEventListener('click', (e) => {
      const btn = e.target.closest('.chip.zone');
      if (!btn) return;
      if (btn.dataset.zoneAll) {
        zonesOff.clear();
      } else {
        const z = btn.dataset.zone;
        if (zonesOff.has(z)) zonesOff.delete(z); else zonesOff.add(z);
      }
      saveZonesOff();
      zoneChips.innerHTML = zoneChipsInner();
      render();
    });
  }
  render();
}

// ---------- HOST EDITOR ----------
function flowNode(type, name, sub, icon, cap) {
  if (cap) {
    return `<div class="node cap">
      <span class="ico">${icon}</span>
      <span class="cap-label">${esc(name)}</span>
      <span class="cap-sub">${esc(sub || '')}</span>
    </div>`;
  }
  return `<div class="node">
    <span class="ico">${icon}</span>
    <span class="type">${esc(type)}</span>
    <span class="nm">${esc(name)}${sub ? `<small>${esc(sub)}</small>` : ''}</span>
  </div>`;
}
const connector = `<div class="connector"><span class="line"></span><span class="flow-dash"></span></div>`;

function mwIcon(type) {
  if (type === 'auth') return ICON.shieldCheck;
  if (type === 'headers') return ICON.headers;
  if (type === 'rate-limit') return ICON.gauge;
  if (type === 'rewrite') return ICON.redirect;
  if (type === 'bouncer') return ICON.shield;
  return ICON.layers;
}

function renderHostFlow(rootEl, ctx) {
  // ctx: { certRef, certDomains, upstreamStr, mwSelected:[name], mwType:{}, alSelected:[name] }
  const nodes = [];
  nodes.push(flowNode('', 'CLIENT', ':443', ICON.clientUser, true));
  if (ctx.certRef) {
    nodes.push(flowNode('TLS - termination', 'TLS termination', ctx.certDomains || ctx.certRef, ICON.lock, false));
  }
  // An inline block sits at its kind's chain position, just outside the
  // referenced middlewares of that kind - so the diagram has to show it, or a
  // host gated only inline reads as ungated here.
  if (ctx.inlineRateLimit) nodes.push(flowNode('rate-limit - inline', 'Rate limit', 'on this host', ICON.gauge, false));
  ctx.mwSelected.forEach((m) => {
    const ty = ctx.mwType[m] || 'middleware';
    nodes.push(flowNode(`${ty}`, m, ty, mwIcon(ty), false));
  });
  if (ctx.inlineAuth) nodes.push(flowNode('auth - inline', 'Sign-in', ctx.inlineAuth, ICON.shieldCheck, false));
  ctx.alSelected.forEach((a) => {
    nodes.push(flowNode('access-list - ip', a, 'rules', ICON.list, false));
  });
  nodes.push(flowNode('', 'UPSTREAM', ctx.upstreamStr, ICON.server, true));
  rootEl.innerHTML = nodes.join(connector);
}

async function hostEditor(c, name) {
  const isNew = !name;
  const seed = isNew ? takeCloneSeed('hosts') : null;
  // Every one of these is a reference list: null means "did not load", which is
  // not the same as "empty" and must never be saved back as an empty picker.
  const [certList, mwList, alList, ugList, clientCAs, hostR, idpList, setR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    refList('/api/middlewares', 'middlewares'),
    refList('/api/access-lists', 'access lists'),
    refList('/api/upstream-groups', 'upstream groups'),
    refList('/api/client-cas', 'client CAs'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/proxy-hosts/' + encodeURIComponent(name)),
    refList('/api/identity-providers', 'identity providers'),
    routeSettings(),
  ]);
  const idps = arr(idpList);
  const fleetProxies = arr((setR.data || {}).trustedProxies);
  const certs = arr(certList);
  const mwListOK = mwList !== null;
  const alListOK = alList !== null;
  const ugListOK = ugList !== null;
  const middlewares = arr(mwList);
  const accessLists = arr(alList);
  const upstreamGroups = arr(ugList);
  const mwType = {}; middlewares.forEach((m) => { mwType[m.name] = m.type; });
  const h = seed ? seed.data : (hostR.data || {});
  const up = h.upstream || {};
  const tls = h.tls || {};
  const hsts = tls.hsts || {};
  // Per-host mTLS, and the identity passthrough nested inside it.
  const mtlsOn = !!tls.clientAuth;
  const caRef = (tls.clientAuth && tls.clientAuth.caRef) || '';
  const caMode = (tls.clientAuth && tls.clientAuth.mode) || 'require';
  const certIDOn = !!(tls.clientAuth && tls.clientAuth.identityHeaders);
  const certID = (tls.clientAuth && tls.clientAuth.identityHeaders) || {};
  // The Client CA picker has THREE states and they must stay distinguishable:
  // the list loaded with CAs, the list loaded and empty, or the list failed to
  // load. Collapsing the last two into "no client CAs defined" would state a
  // falsehood, gate the toggle for the wrong reason, and - worst - make every
  // save of an mTLS host bail on a CA it can see perfectly well in its own
  // config. When the list is unavailable this page reports that, changes
  // nothing about the stored trust anchor, and stays out of the way.
  const caListOK = clientCAs !== null;
  const caList = caListOK ? clientCAs : [];
  const caKnown = caList.some((ca) => ca.name === caRef);
  const caUsable = caList.some((ca) => !ca.disabled);
  // A caRef the list does not contain is shown as itself, flagged and selected,
  // so saving round-trips it. Silently letting the select fall through to the
  // first option would retarget the host's trust anchor without a word.
  const caOptions = !caListOK
    ? `<option value="${esc(caRef)}" selected>${esc(caRef || '(unknown)')}</option>`
    : (caList.length
      ? (caRef && !caKnown ? `<option value="${esc(caRef)}" selected>${esc(caRef)} (not found)</option>` : '')
        + caList.map((ca) => `<option value="${esc(ca.name)}"${caRef === ca.name ? ' selected' : ''}${ca.disabled ? ' disabled' : ''}>${esc(ca.name)}${ca.disabled ? ' (disabled)' : ''}</option>`).join('')
      : '<option value="">no client CAs defined</option>');
  const caHint = !caListOK
    ? 'Client CA list unavailable - reload the page. Saving leaves this host\'s trust anchor exactly as it is.'
    : (!caList.length
      ? 'Create one under <a href="#/clientcas">Client CAs</a> first - it is the trust anchor client certificates are verified against.'
      : (caRef && !caKnown
        ? `<b>${esc(caRef)}</b> is not in the client CA list. It is kept as-is on save - fix it under <a href="#/clientcas">Client CAs</a>.`
        : 'The trust anchor presented certificates are verified against.'));

  const comp = h.compression || {};
  const ep = h.errorPages || {};

  // Fleet-wide maintenance overrides every per-host switch, so the probe is
  // read BEFORE the form renders: showing a host's own toggle as "off" while
  // settings.maintenance.enabled has it serving the maintenance page would be a
  // lie the operator acts on.
  await loadCapabilities();
  const globalMaint = hasCapability('maintenance.globalEnabled');

  const selMw = arr(h.middlewares);
  const selAl = arr(h.accessLists);
  // Attached entries render first, in the host's stored (chain) order, so the
  // active chain is visible without scrolling; the save path reads checkboxes
  // in DOM order, so this also preserves that order on re-save.
  const mwSorted = selMw.map((n) => middlewares.find((m) => m.name === n)).filter(Boolean)
    .concat(middlewares.filter((m) => selMw.indexOf(m.name) === -1));
  const alSorted = selAl.map((n) => accessLists.find((a) => a.name === n)).filter(Boolean)
    .concat(accessLists.filter((a) => selAl.indexOf(a.name) === -1));

  const statusChip = h.disabled
    ? `<span class="chip"><span class="dot" style="background:var(--faint)"></span>disabled</span>`
    : ((h.maintenance || globalMaint) && !isNew
      ? `<span class="chip warn"><span class="dot warn"></span>maintenance</span>`
      : `<span class="chip ok"><span class="dot ok"></span>${isNew ? 'new' : 'live'}</span>`);

  // Progressive disclosure. A proxy host has thirteen sections and a normal one
  // uses four of them, so everything optional collapses to a single line that
  // says what it currently holds ("Locations (2)", "Timeouts (defaults)",
  // "Middleware (sso, rate-limit)"). A section that holds a non-default value
  // opens itself, so nothing configured is ever hidden from someone scanning
  // the page - the fold saves scrolling, it never hides state.
  //
  // The four that stay open are the ones every host needs: domains, the
  // upstream, TLS, and the access/middleware chain.
  const idParts = [];
  if (h.displayName) idParts.push(h.displayName);
  if (arr(h.tags).length) idParts.push(`${arr(h.tags).length} tag${arr(h.tags).length === 1 ? '' : 's'}`);
  if (h.disabled) idParts.push('disabled');
  if (h.maintenance) idParts.push('maintenance');
  const idSummary = idParts.length ? idParts.join(' - ') : (h.name || 'name, tags, status');

  const locCount = arr(h.locations).length;
  const locSummary = locCount ? `${locCount} path override${locCount === 1 ? '' : 's'}` : 'none - every path goes to the upstream above';

  const tmo = h.timeouts || {};
  const tmoParts = [];
  if (tmo.connectSeconds) tmoParts.push(`connect ${tmo.connectSeconds}s`);
  if (tmo.readSeconds) tmoParts.push(`read ${tmo.readSeconds}s`);
  if (h.robotsNoIndex) tmoParts.push('indexing discouraged');
  const tmoSummary = tmoParts.length ? tmoParts.join(', ') : 'defaults';

  const compSummary = comp.enabled
    ? ['gzip on', comp.minBytes ? `min ${comp.minBytes} bytes` : '', arr(comp.types).length ? `${arr(comp.types).length} content types` : ''].filter(Boolean).join(', ')
    : 'off';

  const epParts = [];
  if (ep.dir) epParts.push(`directory ${ep.dir}`);
  if (ep.inline && Object.keys(ep.inline).length) epParts.push(`${Object.keys(ep.inline).length} inline`);
  if (arr(ep.interceptUpstream).length) epParts.push(`intercepts ${arr(ep.interceptUpstream).join(', ')}`);
  const epSummary = epParts.length ? epParts.join(', ') : 'inherited from Settings';

  const secHdrCount = (h.securityHeaders && typeof h.securityHeaders === 'object') ? Object.keys(h.securityHeaders).length : 0;
  const secHdrSummary = secHdrCount ? `${secHdrCount} set` : 'fleet defaults';

  const stripCount = arr(h.stripResponseHeaders).length;
  const stripSummary = stripCount ? `${stripCount} header${stripCount === 1 ? '' : 's'}` : 'none';

  const dnsParts = [];
  if (h.dns && h.dns.lanDirect) dnsParts.push('LAN direct');
  if (h.dns && h.dns.publicCname) dnsParts.push('public CNAME');
  const dnsSummary = dnsParts.length ? dnsParts.join(', ') : 'off';

  // trustedProxies is a THREE-state field, and the states are not the same
  // thing: absent means "inherit the fleet list", present-and-empty means
  // "trust nobody on this host" (which is how you opt a single host out of a
  // non-empty fleet list), and a non-empty list replaces it. A two-state control
  // could only ever express two of the three, so the editor picks between them
  // explicitly and NEVER sends [] unless the operator chose to.
  const tpList = arr(h.trustedProxies);
  const tpMode = (h.trustedProxies == null) ? 'inherit' : (tpList.length ? 'custom' : 'none');
  const tpSummary = tpMode === 'custom' ? `trusted proxies: ${tpList.join(', ')}`
    : (tpMode === 'none' ? 'trusted proxies: none (connection address only)' : 'trusted proxies inherited from Settings');

  c.innerHTML = `
    <div class="row-between view-head">
      <div>
        <div class="muted" style="font-size:12px;margin-bottom:3px"><a href="#/hosts">Proxy Hosts</a> / ${isNew ? 'new' : 'edit'}</div>
        <h2 style="font-family:var(--display)">${esc(isNew ? 'New proxy host' : (arr(h.domains)[0] || h.name))}</h2>
        <p>Edit routing, TLS, and the middleware chain for this host.</p>
      </div>
      ${statusChip}
    </div>

    <div class="panel flow-panel">
      <div class="flow-head"><h3>Request flow</h3><span class="chip cyan">left to right &middot; pipeline order</span></div>
      <div class="flow" id="flow"></div>
    </div>

    <div class="form-grid">
      <div class="stack">
        ${foldHtml('f-id-card', 'Identity', idSummary, isNew || idParts.length > 0, `
          <div class="inline-fields">
            <div class="field-group">
              <label>Name</label>
              <input class="field mono" id="f-name" data-hint="common.name" data-path="name" value="${esc(h.name || '')}" ${isNew ? '' : 'disabled'} placeholder="${esc(seed ? seed.origName + '-copy' : 'internal-name')}" />
              <div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div>
            </div>
            <div class="field-group">
              <label>Display name</label>
              <input class="field" id="f-display" data-hint="common.displayName" data-path="displayName" value="${esc(h.displayName || '')}" placeholder="optional label" />
            </div>
          </div>
          <div class="field-group" style="margin-top:10px">
            <label>Tags</label>
            <div class="chip-input" id="f-tags" data-hint="proxyHost.tags" data-path="tags"></div>
            <div class="hint">Free-form labels for grouping and filtering. Press Enter to add.</div>
          </div>
          <div class="toggle-line" style="margin-top:6px">
            <div class="tl-text"><div class="nm">Disabled</div><div class="ds">Stop serving this host</div></div>
            ${switchHtml('f-disabled', !!h.disabled, 'Disabled', 'common.disabled')}
          </div>
          <div class="toggle-line" style="margin-top:6px">
            <div class="tl-text"><div class="nm">Maintenance</div><div class="ds">${globalMaint
              ? 'Fleet-wide maintenance is on in <a href="#/settings">Settings</a>: every host serves the maintenance page regardless of this switch.'
              : 'Serve a 503 maintenance page instead of proxying. The host keeps its domains and certificate.'}</div></div>
            ${switchHtml('f-maintenance', !!h.maintenance, 'Maintenance', 'proxyHost.maintenance')}
          </div>
        `)}

        <div class="card form-section">
          <p class="section-label">Domains</p>
          <div class="chip-input" id="f-domains" data-hint="proxyHost.domains" data-path="domains"></div>
          <div class="hint">Press Enter to add. At least one domain is required.</div>
        </div>

        <div class="card form-section">
          <p class="section-label">Upstream</p>
          ${upstreamGroups.length ? `<div class="field-group" style="margin-bottom:10px">
            <label>Upstream group</label>
            <select class="field mono" id="f-upgroup" data-hint="proxyHost.upstreamGroupRef" data-path="upstreamGroupRef">
              <option value="">(single upstream)</option>
              ${upstreamGroups.map((g) => `<option value="${esc(g.name)}"${h.upstreamGroupRef === g.name ? ' selected' : ''}>${esc(g.name)} (${arr(g.upstreams).length} upstreams)</option>`).join('')}
            </select>
          </div>` : ''}
          <div class="inline-fields">
            <div class="field-group">
              <label>Scheme</label>
              <select class="field mono" id="f-scheme" data-hint="proxyHost.upstream.scheme" data-path="upstream.scheme">
                <option value="http"${up.scheme === 'https' ? '' : ' selected'}>http</option>
                <option value="https"${up.scheme === 'https' ? ' selected' : ''}>https</option>
              </select>
            </div>
            <div class="field-group" style="flex:2">
              <label>Host</label>
              <input class="field mono" id="f-uphost" data-hint="proxyHost.upstream.host" data-path="upstream.host" value="${esc(up.host || '')}" placeholder="10.0.0.5" />
            </div>
            <div class="field-group">
              <label>Port</label>
              <input class="field mono" id="f-upport" data-hint="proxyHost.upstream.port" data-path="upstream.port" type="number" value="${esc(up.port != null ? up.port : '')}" placeholder="8080" />
            </div>
          </div>
          ${upstreamExtraHtml('f', up)}
        </div>

        ${foldHtml('f-locs-card', 'Locations', locSummary, locCount > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 8px">A path prefix that overrides the host's upstream and chain. Path order: location match, then Strip prefix, then rewrite rules, then the upstream base path. Security controls always evaluate the ORIGINAL client path.</p>
          <div id="f-locs" data-path="locations"></div>
          <button class="btn ghost sm" id="addLoc" type="button" style="margin-top:6px">${ICON.plus}Add path override</button>
        `)}

        ${foldHtml('f-crawl-card', 'Crawling and timeouts', tmoSummary, tmoParts.length > 0, `
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send <span class="mono">X-Robots-Tag: noindex, nofollow</span></div></div>
            ${switchHtml('f-robots', !!h.robotsNoIndex, 'Discourage indexing', 'proxyHost.robotsNoIndex')}
          </div>
          <div class="inline-fields" style="margin-top:10px">
            <div class="field-group">
              <label>Connect timeout (s)</label>
              <input class="field mono" id="f-to-connect" data-hint="proxyHost.timeouts.connectSeconds" data-path="timeouts.connectSeconds" type="number" min="0" max="3600" value="${esc(h.timeouts && h.timeouts.connectSeconds ? h.timeouts.connectSeconds : '')}" placeholder="default" />
            </div>
            <div class="field-group">
              <label>Read timeout (s)</label>
              <input class="field mono" id="f-to-read" data-hint="proxyHost.timeouts.readSeconds" data-path="timeouts.readSeconds" type="number" min="0" max="3600" value="${esc(h.timeouts && h.timeouts.readSeconds ? h.timeouts.readSeconds : '')}" placeholder="default" />
            </div>
          </div>
          <div class="hint">Blank keeps the shared pooled transport. Read timeout caps time-to-first-byte; it does not cut off slow streaming/websocket bodies.</div>
        `)}

        ${foldHtml('f-comp-card', 'Compression', compSummary, !!comp.enabled, `
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">Gzip responses</div><div class="ds">Compress eligible upstream responses honouring Accept-Encoding</div></div>
            ${switchHtml('f-gzip', !!comp.enabled, 'Gzip responses', 'proxyHost.compression.enabled')}
          </div>
          <div id="gzip-fields" style="margin-top:10px;${comp.enabled ? '' : 'display:none'}">
            <div class="inline-fields">
              <div class="field-group"><label>Minimum bytes</label><input class="field mono" id="f-gzip-min" data-hint="proxyHost.compression.minBytes" data-path="compression.minBytes" type="number" min="0" value="${esc(comp.minBytes || '')}" placeholder="1024" /></div>
            </div>
            <div class="field-group" style="margin-top:10px">
              <label>Content types</label>
              <textarea class="field mono" id="f-gzip-types" data-hint="proxyHost.compression.types" data-path="compression.types" rows="2" placeholder="text/html, application/json, ...">${esc(arr(comp.types).join(', '))}</textarea>
              <div class="hint">Comma-separated media types. Blank uses the built-in text/JSON/JS/CSS/SVG/XML list. Never applied to websocket upgrades, streaming, or event-stream responses.</div>
            </div>
          </div>
        `)}

        ${foldHtml('f-errp-card', 'Error pages', epSummary, epParts.length > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 8px">Override the global error pages (Settings) for this host's own errors - upstream unreachable, access denied, rate limited, an auth refusal (401/403/503), a dangling middleware reference. Blank leaves the settings-level pages (or gpm's default output) in effect.</p>
          <div class="field-group">
            <label>Templates directory</label>
            <input class="field mono" id="f-errp-dir" data-hint="proxyHost.errorPages.dir" data-path="errorPages.dir" value="${esc(ep.dir || '')}" placeholder="relative to the cert store, e.g. errorpages/app" />
            <div class="hint">html/template files named "&lt;status&gt;.html" (e.g. 502.html) plus an optional default.html.</div>
          </div>
          <div class="field-group" style="margin-top:10px">
            <label>Inline templates (JSON)</label>
            <textarea class="field mono" id="f-errp-inline" data-hint="proxyHost.errorPages.inline" data-path="errorPages.inline" rows="4" placeholder='{"502": "&lt;h1&gt;...&lt;/h1&gt;", "default": "&lt;h1&gt;...&lt;/h1&gt;"}'>${esc(ep.inline ? JSON.stringify(ep.inline, null, 2) : '')}</textarea>
            <div class="hint">Status code (or "default") to HTML source. Template vars: {{.Status}} {{.StatusText}} {{.Host}} {{.RequestID}}.</div>
          </div>
          <div class="field-group" style="margin-top:10px">
            <label>Also replace upstream errors for</label>
            <input class="field mono" id="f-errp-intercept" data-hint="proxyHost.errorPages.interceptUpstream" data-path="errorPages.interceptUpstream" value="${esc(arr(ep.interceptUpstream).join(', '))}" placeholder="502, 503" />
            <div class="hint">Comma-separated status codes. Normally only gpm's own errors get the custom page; the upstream's own error body passes through untouched.</div>
          </div>
        `)}

        ${foldHtml('f-sech-card', 'Response security headers', secHdrSummary, secHdrCount > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 8px">Overrides the fleet defaults (Settings) for this host, per header name - a name listed here replaces the settings value <b>and</b> its scope. Names not listed keep the settings-level header. Applied set-if-absent, so an upstream's own header is never clobbered.</p>
          <div id="f-secheaders" data-hint="proxyHost.securityHeaders" data-path="securityHeaders"></div>
          <button class="btn ghost sm" id="f-secheaders-add" type="button" style="margin-top:6px">${ICON.plus}Add header</button>
          <div class="hint" style="margin-top:6px">Scope: <span class="mono">all</span>, <span class="mono">generated-only</span> (only responses gpm writes itself - denials, sign-in redirects, error pages) or <span class="mono">proxied-only</span>. Strict-Transport-Security is not settable here; HSTS lives in the TLS section.</div>
        `)}

        ${foldHtml('f-strip-card', 'Strip response headers', stripSummary, stripCount > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 8px">This host's additions to the fleet strip list (Settings) - header names removed from proxied upstream responses before they reach the client. Only what the upstream sent is touched, never a header gpm adds.</p>
          <div class="field-group"><label>Header names</label><div class="chip-input" id="f-striphdrs" data-hint="proxyHost.stripResponseHeaders" data-path="stripResponseHeaders"></div></div>
          <div class="hint">Hop-by-hop and response-semantic names (Content-Type, Content-Length, Content-Encoding, Location, Vary, the WebSocket handshake) are refused on save.</div>
        `)}

        ${foldHtml('f-adv-card', 'Advanced', tpSummary, tpMode !== 'inherit', `
          <div class="field-group">
            <label>Trusted proxies (override)</label>
            <div class="seg" id="f-tp-seg" data-hint="proxyHost.trustedProxies.mode">
              <button type="button" class="seg-btn${tpMode === 'inherit' ? ' on' : ''}" data-tp="inherit">Inherit fleet setting</button>
              <button type="button" class="seg-btn${tpMode === 'none' ? ' on' : ''}" data-tp="none">Trust nobody on this host</button>
              <button type="button" class="seg-btn${tpMode === 'custom' ? ' on' : ''}" data-tp="custom">Custom list</button>
            </div>
            <div id="f-tp-custom"${tpMode === 'custom' ? '' : ' hidden'} style="margin-top:8px">
              <div class="chip-input" id="f-trustedproxies" data-hint="proxyHost.trustedProxies" data-path="trustedProxies"></div>
            </div>
            <div class="hint"><b>Replaces</b> the fleet-wide trusted proxies for this host - it does not add to them. Set a custom list when this host sits behind a different proxy, or trust nobody to make this host read the connection address only while the fleet list stays non-empty.</div>
            <div class="hint" id="f-trustedproxies-inherit"${tpMode === 'inherit' ? '' : ' hidden'}>${esc(fleetProxies.length ? 'Inheriting: ' + fleetProxies.join(', ') : 'Inheriting: nothing (the connection address is the client).')}</div>
            <div class="hint" id="f-trustedproxies-none"${tpMode === 'none' ? '' : ' hidden'}>This host reads the client address from the connection only. <span class="mono">X-Forwarded-For</span> is ignored however the fleet list is set.</div>
            <div class="hint warn" id="f-trustedproxies-warn" hidden>${esc(TRUSTED_WILDCARD_WARNING)}</div>
          </div>
        `)}

        ${foldHtml('f-dns-card', 'DNS sync', dnsSummary, dnsParts.length > 0, `
          <p class="muted" style="font-size:11.5px;margin:0 0 8px">Publish this host's domains as CNAMEs pointing at the edge. Configure the backends under Settings.</p>
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">LAN direct</div><div class="ds">Local CNAME on the LAN resolver (Pi-hole)</div></div>
            ${switchHtml('f-dns-lan', !!(h.dns && h.dns.lanDirect), 'LAN direct', 'proxyHost.dns.lanDirect')}
          </div>
          <div class="hint" id="f-dns-lan-hint" style="display:none">Pi-hole DNS sync is not configured yet - this will take effect when it is (Settings -> DNS sync).</div>
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">Public CNAME</div><div class="ds">Record in the authoritative public zone (Cloudflare)</div></div>
            ${switchHtml('f-dns-public', !!(h.dns && h.dns.publicCname), 'Public CNAME', 'proxyHost.dns.publicCname')}
          </div>
          <div class="hint" id="f-dns-public-hint" style="display:none">Cloudflare DNS sync is not configured yet - this will take effect when it is (Settings -> DNS sync).</div>
        `)}
      </div>

      <div class="stack">
        <div class="card form-section">
          <p class="section-label">TLS</p>
          <div class="field-group">
            <label>Certificate</label>
            <div id="f-cert-auto">${certCoverageHtml(certs, arr(h.domains))}</div>
            <div class="hint">${CERT_AUTO_HINT}</div>
          </div>
          <div class="field-group">
            <label>Minimum TLS version</label>
            <select class="field mono" id="f-mintls" data-hint="proxyHost.tls.minTLSVersion" data-path="tls.minTLSVersion">
              <option value="1.2"${(tls.minTLSVersion || '1.2') === '1.2' ? ' selected' : ''}>1.2 (default - negotiates 1.2/1.3)</option>
              <option value="1.3"${tls.minTLSVersion === '1.3' ? ' selected' : ''}>1.3 only</option>
            </select>
            <div class="hint">1.3-only drops clients that can't do TLS 1.3 (old smart TVs / embedded). Keep 1.2 for public hosts.</div>
          </div>
          <div style="margin-top:6px">
            <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS</div></div>${switchHtml('f-forcessl', !!tls.forceSSL, 'Force SSL', 'proxyHost.tls.forceSSL')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Send <span class="mono">Strict-Transport-Security</span></div></div>${switchHtml('f-hsts', !!hsts.enabled, 'HSTS', 'proxyHost.tls.hsts.enabled')}</div>
          </div>
          <div id="hsts-fields" style="margin-top:12px;${hsts.enabled ? '' : 'display:none'}">
            <div class="inline-fields">
              <div class="field-group"><label>Max age (s)</label><input class="field mono" id="f-hsts-max" data-hint="proxyHost.tls.hsts.maxAge" data-path="tls.hsts.maxAge" type="number" value="${esc(hsts.maxAge != null ? hsts.maxAge : HSTS_DEFAULT_MAX_AGE)}" /></div>
            </div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Include subdomains</div></div>${switchHtml('f-hsts-sub', !!hsts.includeSubdomains, 'Include subdomains', 'proxyHost.tls.hsts.includeSubdomains')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Preload</div><div class="ds">Ask browsers to hard-code this domain as HTTPS-only</div></div>${switchHtml('f-hsts-preload', !!hsts.preload, 'Preload', 'proxyHost.tls.hsts.preload')}</div>
          </div>
          <div style="margin-top:14px;padding-top:12px;border-top:1px solid var(--border)">
            <p class="section-label">Client certificates (mTLS)</p>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Client certificates</div><div class="ds">Verify the client's certificate at the handshake against a Client CA</div></div>${switchHtml('f-mtls', mtlsOn, 'Client certificates', 'proxyHost.tls.clientAuth')}</div>
            <div class="hint" id="f-mtls-hint" style="display:none"></div>
            <div id="mtls-fields" style="margin-top:10px;${mtlsOn ? '' : 'display:none'}">
            <div class="field-group"><label>Client CA</label>
              <select class="field mono" id="f-mtls-ca" data-hint="proxyHost.tls.clientAuth.caRef" data-path="tls.clientAuth.caRef"${caListOK ? '' : ' disabled'}>
                ${caOptions}
              </select>
              <div class="hint">${caHint}</div>
            </div>
            <div class="field-group"><label>Mode</label>
              <select class="field mono" id="f-mtls-mode" data-hint="proxyHost.tls.clientAuth.mode" data-path="tls.clientAuth.mode">
                ${enumOptions('mtlsMode', ['require', 'optional'], caMode === 'optional' ? 'optional' : 'require')}
              </select>
              <div class="hint">Pair <span class="mono">optional</span> with a client-cert auth middleware for LAN-exempt enforcement.</div>
            </div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Identity passthrough</div><div class="ds">Send the verified certificate's subject upstream</div></div>${switchHtml('f-certid', certIDOn, 'Identity passthrough', 'proxyHost.tls.clientAuth.identityHeaders')}</div>
            <div id="certid-fields" style="${certIDOn ? '' : 'display:none'}">
              <div class="toggle-line"><div class="tl-text"><div class="nm">SAN</div><div class="ds"><span class="mono">X-Client-Cert-SAN</span></div></div>${switchHtml('f-certid-san', !!certID.san, 'SAN header', 'proxyHost.tls.clientAuth.identityHeaders.san')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Serial</div><div class="ds"><span class="mono">X-Client-Cert-Serial</span></div></div>${switchHtml('f-certid-serial', !!certID.serial, 'Serial header', 'proxyHost.tls.clientAuth.identityHeaders.serial')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Fingerprint</div><div class="ds"><span class="mono">X-Client-Cert-Fingerprint</span></div></div>${switchHtml('f-certid-fp', !!certID.fingerprint, 'Fingerprint header', 'proxyHost.tls.clientAuth.identityHeaders.fingerprint')}</div>
              <div class="field-group" style="margin-top:10px"><label>Subject header name</label>
                <input class="field mono" id="f-certid-subject" data-hint="proxyHost.tls.clientAuth.identityHeaders.subjectHeader" data-path="tls.clientAuth.identityHeaders.subjectHeader" value="${esc(certID.subjectHeader || '')}" placeholder="X-Client-Cert-Subject" />
                <div class="hint">These headers are always stripped from untrusted peers; only gpm asserts them.</div>
              </div>
            </div>
            </div>
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Middleware chain</p>
          <p class="muted" style="font-size:11.5px;margin:0 0 10px">Attached: <b>${esc(selMw.length ? selMw.join(', ') : 'none')}</b>. Evaluation order is fixed whatever order they are ticked in: rate limit, access list, bouncer, auth, guard, headers, rewrite, then the upstream.</p>
          <div class="check-list" id="f-mw" data-hint="proxyHost.middlewares" data-path="middlewares">
            ${!mwListOK ? refListUnavailableHtml('the middleware list') : (mwSorted.length ? mwSorted.map((m) => `
              <label class="check-item"><input type="checkbox" value="${esc(m.name)}"${selMw.indexOf(m.name) !== -1 ? ' checked' : ''}/>${esc(m.name)}<span class="ci-ty">${esc(m.type || '')}</span></label>
            `).join('') : '<div class="check-empty">No middleware defined yet.</div>')}
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Access lists</p>
          <p class="muted" style="font-size:11.5px;margin:0 0 10px">Attached: <b>${esc(selAl.length ? selAl.join(', ') : 'none')}</b>. Who may reach this host, by IP range or country.</p>
          <div class="check-list" id="f-al" data-hint="proxyHost.accessLists" data-path="accessLists">
            ${!alListOK ? refListUnavailableHtml('the access list index') : (alSorted.length ? alSorted.map((a) => `
              <label class="check-item"><input type="checkbox" value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 ? ' checked' : ''}/>${esc(a.name)}<span class="ci-ty">access-list</span></label>
            `).join('') : '<div class="check-empty">No access lists defined yet.</div>')}
          </div>
          <div class="hint" id="f-inline-order"${(h.auth || h.rateLimit) ? '' : ' hidden'} style="margin-top:8px">Sign-in and rate limit set below run before the middlewares listed above.</div>
        </div>

        ${inlineFoldHtml('f-auth', 'Sign-in', 'Gate this host through an identity provider. For a gate shared by several hosts, use a middleware instead.',
    !!h.auth, authFoldSummary(h.auth), authBlockHtml('f-auth', h.auth, idps), 'proxyHost.auth')}

        ${inlineFoldHtml('f-rl', 'Rate limit', 'Throttle this host per client IP. For a limit shared by several hosts, use a middleware instead.',
    !!h.rateLimit, rateFoldSummary(h.rateLimit), rateLimitBlockHtml('f-rl', h.rateLimit), 'proxyHost.rateLimit')}
      </div>
    </div>

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        ${isNew ? '' : `<button class="btn ghost" id="hostCloneBtn" type="button">Clone</button>`}
        ${isNew ? '' : `<button class="btn danger" id="deleteBtn" type="button">${ICON.trash}Delete</button>`}
        <a class="btn ghost" href="#/hosts">Cancel</a>
        <button class="btn primary" id="saveBtn" type="button">Save changes</button>
      </div>
    </div>`;

  // domains chip input
  const domainsCtl = makeChipInput($('#f-domains'), arr(h.domains), 'add domain...', (d) => {
    $('#f-cert-auto').innerHTML = certCoverageHtml(certs, d);
    refreshFlow();
  });
  const tagsCtl = makeChipInput($('#f-tags'), arr(h.tags), 'add tag...');

  const secHdrCtl = makeSecurityHeaderRows($('#f-secheaders'), h.securityHeaders);
  $('#f-secheaders-add').addEventListener('click', () => secHdrCtl.addRow('', ''));
  const stripCtl = makeChipInput($('#f-striphdrs'), arr(h.stripResponseHeaders), 'add header...');

  // Trusted proxies (host override), three-state. The wildcard warning is
  // permanent while the chip is present - it does not block the save.
  const trustedCtl = makeChipInput($('#f-trustedproxies'), tpList, '10.0.0.0/8 or 192.0.2.10', (list) => {
    $('#f-trustedproxies-warn').hidden = !hasWildcardProxy(list);
  });
  $('#f-trustedproxies-warn').hidden = !hasWildcardProxy(tpList);
  function tpChoice() {
    const on = $('#f-tp-seg .seg-btn.on');
    return on ? on.dataset.tp : 'inherit';
  }
  $$('#f-tp-seg .seg-btn').forEach((b) => b.addEventListener('click', () => {
    $$('#f-tp-seg .seg-btn').forEach((x) => x.classList.toggle('on', x === b));
    const mode = b.dataset.tp;
    $('#f-tp-custom').hidden = mode !== 'custom';
    $('#f-trustedproxies-inherit').hidden = mode !== 'inherit';
    $('#f-trustedproxies-none').hidden = mode !== 'none';
    $('#f-trustedproxies-warn').hidden = mode !== 'custom' || !hasWildcardProxy(trustedCtl.get());
    b.dispatchEvent(new CustomEvent('switchchange', { bubbles: true }));
  }));

  // The two inline blocks. Their folds own the whole key: off omits it.
  const hostAuthCtl = wireAuthBlock('f-auth', h.auth, idps);
  const hostRlCtl = wireRateLimitBlock('f-rl', h.rateLimit);
  const hostUpExtra = wireUpstreamExtra('f');
  wireInlineFold('f-auth');
  wireInlineFold('f-rl');
  const syncInlineOrderHint = () => {
    const hint = $('#f-inline-order');
    if (hint) hint.hidden = !(isOn('f-auth-on') || isOn('f-rl-on'));
  };
  $('#f-auth-on').addEventListener('switchchange', () => { syncInlineOrderHint(); refreshFlow(); });
  $('#f-rl-on').addEventListener('switchchange', () => { syncInlineOrderHint(); refreshFlow(); });
  $('#f-auth-idp').addEventListener('change', refreshFlow);

  // These two DNS toggles stay USABLE when their backend is not configured, and
  // say so inline instead. Deliberately unlike every other capability-gated
  // control (which gateControl still greys out): setting the flag before the
  // backend exists is legitimate staging - the host is the declaration, the
  // syncer publishes whenever it is wired - not an error to be refused. And the
  // capability probe is cached per page load, so a stale "not configured" would
  // otherwise outlive the fact and block a perfectly valid edit.
  await loadCapabilities();
  const dnsHint = (id, available) => {
    const el = $(id);
    if (el) el.style.display = available ? 'none' : '';
  };
  dnsHint('#f-dns-lan-hint', hasCapability('dnsSync.piholeEnabled'));
  dnsHint('#f-dns-public-hint', hasCapability('dnsSync.cloudflareEnabled'));

  // locations. A location is a whole host in miniature now - its own upstream
  // (with the same base-path and Host-header escape hatches), its own middleware
  // and access-list picks, and its own inline sign-in and rate limit - so each
  // one is a block with folds rather than a single flat row.
  const locsWrap = $('#f-locs');
  const locCtls = [];
  let locSeq = 0;
  function locRow(loc) {
    loc = loc || {};
    const lu = loc.upstream || {};
    const p = 'loc' + (++locSeq);
    const div = document.createElement('div');
    div.className = 'panel loc-block';
    div.style.cssText = 'padding:12px;margin-bottom:10px';
    div.dataset.uid = p;
    div._orig = loc;
    const groupSel = upstreamGroups.length ? `
      <select class="field mono loc-group" data-hint="proxyHost.locations.upstreamGroupRef" style="flex:0 0 130px" aria-label="Upstream group">
        <option value="">(no group)</option>
        ${upstreamGroups.map((g) => `<option value="${esc(g.name)}"${loc.upstreamGroupRef === g.name ? ' selected' : ''}>group:${esc(g.name)}</option>`).join('')}
      </select>` : '';
    div.innerHTML = `
      <div class="loc-row">
        <input class="field mono loc-path" data-hint="proxyHost.locations.path" style="flex:1 1 120px" value="${esc(loc.path || '')}" placeholder="/api" aria-label="Path" />
        <span class="arrow">${ICON.arrow}</span>${groupSel}
        <select class="field mono loc-scheme" data-hint="proxyHost.locations.upstream.scheme" style="flex:0 0 90px" aria-label="Upstream scheme"><option value="">(host default)</option><option value="http"${lu.scheme === 'http' ? ' selected' : ''}>http</option><option value="https"${lu.scheme === 'https' ? ' selected' : ''}>https</option></select>
        <input class="field mono loc-host" data-hint="proxyHost.locations.upstream.host" style="flex:1 1 110px" value="${esc(lu.host || '')}" placeholder="host (optional)" aria-label="Upstream host" />
        <input class="field mono loc-port" data-hint="proxyHost.locations.upstream.port" type="number" style="flex:0 0 80px" value="${esc(lu.port != null ? lu.port : '')}" placeholder="port" aria-label="Upstream port" />
        <button class="icon-btn loc-del" type="button" aria-label="Remove location">${ICON.x}</button>
      </div>
      <div class="toggle-line" style="margin-top:6px">
        <div class="tl-text"><div class="nm">Strip prefix</div><div class="ds">Removes the location path from the request before it reaches the backend: <span class="mono">/app/foo</span> arrives as <span class="mono">/foo</span>, and <span class="mono">/app</span> as <span class="mono">/</span>. Use it when the backend expects to live at the root.</div></div>
        ${switchHtml(p + '-strip', !!loc.stripPrefix, 'Strip prefix', 'proxyHost.locations.stripPrefix')}
      </div>
      <div class="hint loc-strip-preview" hidden></div>
      ${upstreamExtraHtml(p, lu)}
      <div class="inline-fields" style="margin-top:8px">
        <div class="field-group"><label>Middlewares</label>
          <select multiple class="field mono loc-mw" data-hint="proxyHost.locations.middlewares" style="height:56px" aria-label="Location middlewares" title="Middlewares for this path only (blank = host chain applies)">${middlewares.map((m) => `<option value="${esc(m.name)}"${arr(loc.middlewares).indexOf(m.name) !== -1 ? ' selected' : ''}>${esc(m.name)}</option>`).join('')}</select>
        </div>
        <div class="field-group"><label>Access lists</label>
          <select multiple class="field mono loc-al" data-hint="proxyHost.locations.accessLists" style="height:56px" aria-label="Location access lists" title="Access lists for this path only (blank = host lists apply)">${accessLists.map((a) => `<option value="${esc(a.name)}"${arr(loc.accessLists).indexOf(a.name) !== -1 ? ' selected' : ''}>${esc(a.name)}</option>`).join('')}</select>
        </div>
      </div>
      ${inlineFoldHtml(p + '-auth', 'Sign-in', 'Gate this path. The host\'s own sign-in still applies here.',
    !!loc.auth, authFoldSummary(loc.auth), authBlockHtml(p + '-auth', loc.auth, idps), 'proxyHost.locations.auth')}
      ${inlineFoldHtml(p + '-rl', 'Rate limit', 'Throttle this path. The host\'s own rate limit still applies here.',
    !!loc.rateLimit, rateFoldSummary(loc.rateLimit), rateLimitBlockHtml(p + '-rl', loc.rateLimit), 'proxyHost.locations.rateLimit')}`;
    locsWrap.appendChild(div);

    const ctl = {
      div, p, _orig: loc,
      up: wireUpstreamExtra(p),
      auth: wireAuthBlock(p + '-auth', loc.auth, idps),
      rl: wireRateLimitBlock(p + '-rl', loc.rateLimit),
    };
    wireInlineFold(p + '-auth');
    wireInlineFold(p + '-rl');
    locCtls.push(ctl);

    div.querySelector('.loc-del').addEventListener('click', () => {
      div.remove();
      const i = locCtls.indexOf(ctl);
      if (i >= 0) locCtls.splice(i, 1);
    });
    // A root location has no prefix to strip, so the switch is greyed out with
    // the reason rather than accepted and refused by the API. The preview line
    // follows the path as it is typed.
    const pathInput = div.querySelector('.loc-path');
    function syncStrip() {
      const path = pathInput.value.trim();
      const root = path === '/' || !path;
      gateControl(document.getElementById(p + '-strip'), !root, 'A root location has no prefix to strip.');
      const prev = div.querySelector('.loc-strip-preview');
      const on = isOn(p + '-strip');
      prev.hidden = !on || root;
      if (on && !root) prev.textContent = `Example: ${path}/foo -> /foo`;
    }
    pathInput.addEventListener('input', syncStrip);
    const stripSw = document.getElementById(p + '-strip');
    if (stripSw) stripSw.addEventListener('switchchange', syncStrip);
    syncStrip();
    // A selected group replaces (and greys out) the row's single-upstream fields.
    const gsel = div.querySelector('.loc-group');
    if (gsel) {
      const sync = () => {
        const grouped = !!gsel.value;
        ['.loc-scheme', '.loc-host', '.loc-port'].forEach((s) => { div.querySelector(s).disabled = grouped; });
      };
      gsel.addEventListener('change', sync);
      sync();
    }
    return ctl;
  }
  arr(h.locations).forEach(locRow);
  $('#addLoc').addEventListener('click', () => locRow({}));

  // HSTS show/hide
  $('#f-hsts').addEventListener('switchchange', () => {
    $('#hsts-fields').style.display = isOn('f-hsts') ? '' : 'none';
  });

  // Compression fields show/hide
  $('#f-gzip').addEventListener('switchchange', () => {
    $('#gzip-fields').style.display = isOn('f-gzip') ? '' : 'none';
  });

  // Client-certificate identity passthrough show/hide. It lives inside the mTLS
  // block, so it is only reachable while mTLS is on.
  const certIdSw = $('#f-certid');
  if (certIdSw) certIdSw.addEventListener('switchchange', () => {
    $('#certid-fields').style.display = isOn('f-certid') ? '' : 'none';
  });

  // mTLS preconditions. The model refuses tls.clientAuth without forceSSL and
  // without a caRef that resolves to an ENABLED ClientCA, so the enable switch is
  // greyed with the reason instead of accepting a host the API will reject.
  function refreshMtls() {
    const ssl = isOn('f-forcessl');
    let reason = '';
    if (!caListOK) reason = 'Client CA list unavailable - reload the page before turning client certificates on.';
    else if (!caUsable) reason = 'No enabled client CA defined yet - create one under Client CAs first.';
    else if (!ssl) reason = 'Client certificates require Force SSL: mTLS must never be servable in the clear.';
    // The gate blocks turning mTLS ON, never turning it OFF. A host whose stored
    // combination is already invalid (git-authored clientAuth with forceSSL
    // false, or a CA since removed) would otherwise be trapped in the broken
    // state with the only exit - the toggle - greyed out.
    gateControl($('#f-mtls'), !reason || isOn('f-mtls'), reason);
    const hint = $('#f-mtls-hint');
    hint.style.display = reason ? '' : 'none';
    hint.textContent = reason;
    $('#mtls-fields').style.display = isOn('f-mtls') ? '' : 'none';
  }
  $('#f-mtls').addEventListener('switchchange', refreshMtls);
  // Turning Force SSL off under a live mTLS host is blocked rather than silently
  // dropping clientAuth: the combination cannot be saved, and quietly disabling
  // the host's client-certificate requirement is the wrong way to resolve that.
  $('#f-forcessl').addEventListener('switchchange', () => {
    if (!isOn('f-forcessl') && isOn('f-mtls')) {
      $('#f-forcessl').setAttribute('aria-checked', 'true');
      toast('Force SSL is required', 'Turn client certificates off first - mTLS must never be servable in the clear.', 'err');
      return;
    }
    refreshMtls();
  });
  refreshMtls();

  // upstream group vs single upstream: a selected group replaces (and greys
  // out) the single-upstream fields.
  function curGroup() { const g = $('#f-upgroup'); return g ? g.value : ''; }
  function refreshUpstreamMode() {
    const grouped = !!curGroup();
    ['#f-scheme', '#f-uphost', '#f-upport'].forEach((sel) => { $(sel).disabled = grouped; });
  }

  // flow
  const flowEl = $('#flow');
  function curMw() { return $$('#f-mw input:checked').map((i) => i.value); }
  function curAl() { return $$('#f-al input:checked').map((i) => i.value); }
  function refreshFlow() {
    // The flow diagram shows the certificate the handshake will actually
    // present, resolved from the first domain, not a stored certificateRef the
    // L7 data plane never reads.
    const autoCert = certForDomain(certs, domainsCtl.get()[0]);
    const cert = autoCert ? autoCert.name : '';
    const uphost = $('#f-uphost').value || '?';
    const upport = $('#f-upport').value || '?';
    renderHostFlow(flowEl, {
      certRef: cert, certDomains: autoCert ? arr(autoCert.domains).join(', ') : cert,
      upstreamStr: curGroup() ? `group:${curGroup()}` : `${uphost}:${upport}`,
      mwSelected: curMw(), mwType, alSelected: curAl(),
      inlineAuth: isOn('f-auth-on') ? (($('#f-auth-idp') && $('#f-auth-idp').value) || 'credentials') : '',
      inlineRateLimit: isOn('f-rl-on'),
    });
  }
  refreshUpstreamMode();
  refreshFlow();
  wireHstsPreload('f-hsts-preload');
  $('#f-uphost').addEventListener('input', refreshFlow);
  $('#f-upport').addEventListener('input', refreshFlow);
  const upGroupSel = $('#f-upgroup');
  if (upGroupSel) upGroupSel.addEventListener('change', () => { refreshUpstreamMode(); refreshFlow(); });
  $$('#f-mw input, #f-al input').forEach((i) => i.addEventListener('change', refreshFlow));

  // save
  $('#saveBtn').addEventListener('click', async () => {
    // A previous failure's banner belongs to the previous attempt.
    clearEditorError();
    const nm = isNew ? $('#f-name').value.trim() : h.name;
    if (!nm) { toast('Name required', 'Enter an internal name for this host.', 'err'); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return; }
    // A PUT is a whole-object replacement, so every ObjectMeta key this form
    // does NOT render has to be seeded from the loaded host or the save deletes
    // it. labels is the one that bites: it carries the discovery ownership
    // marker (<prefix>/managed-by), and dropping it orphans a host the ingress/
    // docker reconciler created. createdAt/updatedAt are excluded on purpose -
    // the store owns those and preserves them itself.
    const obj = Object.assign({}, metaCarryForward(h), { name: nm, domains: domains });
    if (!ugListOK && h.upstreamGroupRef) {
      // Same rule as the middleware/access-list pickers: no upstream-group list
      // means no picker rendered, which must not read as "unset the group".
      obj.upstreamGroupRef = h.upstreamGroupRef;
    } else if (curGroup()) {
      obj.upstreamGroupRef = curGroup();
    } else {
      const portVal = parseInt($('#f-upport').value, 10);
      if (!$('#f-uphost').value.trim() || isNaN(portVal)) { toast('Upstream incomplete', 'Set the upstream host and port, or select an upstream group.', 'err'); return; }
      // MERGED over the stored upstream, never rebuilt from the three inputs:
      // Upstream also carries path and hostHeader, and a rebuild dropped both on
      // every save (silently undoing a git-authored escape hatch). The two
      // fields this form renders are then set explicitly, including their
      // absence, so clearing one in the UI actually clears it.
      const upObj = Object.assign({}, up, { scheme: $('#f-scheme').value, host: $('#f-uphost').value.trim(), port: portVal });
      const extra = hostUpExtra.get();
      if (!extra) return;
      delete upObj.path; delete upObj.hostHeader;
      Object.assign(upObj, extra);
      obj.upstream = upObj;
    }
    const display = $('#f-display').value.trim();
    if (display) obj.displayName = display;
    const tags = tagsCtl.get(); if (tags.length) obj.tags = tags;
    if (isOn('f-disabled')) obj.disabled = true;
    if (isOn('f-maintenance')) obj.maintenance = true;
    // websocketsUpgrade has no control any more: nothing in the data plane reads
    // ProxyHost.WebsocketsUpgrade, so the switch promised a behaviour change it
    // could not deliver. The stored value is still carried forward verbatim (a
    // host PUT is a whole-object replacement) so a git-authored one is not
    // dropped by an unrelated save here.
    if (h.websocketsUpgrade !== undefined) obj.websocketsUpgrade = h.websocketsUpgrade;
    if (isOn('f-robots')) obj.robotsNoIndex = true;
    const dns = {};
    if (isOn('f-dns-lan')) dns.lanDirect = true;
    if (isOn('f-dns-public')) dns.publicCname = true;
    if (Object.keys(dns).length) obj.dns = dns;
    const toConnect = parseInt($('#f-to-connect').value, 10);
    const toRead = parseInt($('#f-to-read').value, 10);
    const timeouts = {};
    if (!isNaN(toConnect) && toConnect > 0) timeouts.connectSeconds = toConnect;
    if (!isNaN(toRead) && toRead > 0) timeouts.readSeconds = toRead;
    if (Object.keys(timeouts).length) obj.timeouts = timeouts;

    if (isOn('f-gzip')) {
      const compObj = { enabled: true };
      const minB = parseInt($('#f-gzip-min').value, 10);
      if (!isNaN(minB) && minB > 0) compObj.minBytes = minB;
      const types = $('#f-gzip-types').value.split(',').map((s) => s.trim()).filter(Boolean);
      if (types.length) compObj.types = types;
      obj.compression = compObj;
    }

    const errpDir = $('#f-errp-dir').value.trim();
    const errpInlineRaw = $('#f-errp-inline').value.trim();
    const errpIntercept = $('#f-errp-intercept').value.split(',').map((s) => s.trim()).filter(Boolean)
      .map((s) => parseInt(s, 10)).filter((n) => !isNaN(n));
    if (errpDir || errpInlineRaw || errpIntercept.length) {
      const errp = {};
      if (errpDir) errp.dir = errpDir;
      if (errpInlineRaw) {
        try { errp.inline = JSON.parse(errpInlineRaw); }
        catch (e) { toast('Invalid error pages JSON', 'Inline templates must be valid JSON (status code or "default" -> HTML).', 'err'); return; }
      }
      if (errpIntercept.length) errp.interceptUpstream = errpIntercept;
      obj.errorPages = errp;
    }

    // securityHeaders: this host's per-key override of settings.securityHeaders,
    // owned by the row editor above (which replaced the old carry-forward guard).
    // A host PUT is a whole-object replacement, so the editor's output IS the
    // field: it emits an untouched map byte-equivalently, keeps a shape it does
    // not understand verbatim, and returns null when there are no rows - which
    // leaves the key off the body entirely rather than committing an empty map.
    const secHdrErr = secHdrCtl.error();
    if (secHdrErr) { toast('Invalid security header', secHdrErr, 'err'); return; }
    const secHdrs = secHdrCtl.get();
    if (secHdrs) obj.securityHeaders = secHdrs;

    // stripResponseHeaders (this host's additions to the fleet strip list) is
    // owned by the chip editor above, which replaced the old carry-forward
    // guard. An empty list leaves the key off the body entirely rather than
    // committing an empty array.
    const stripHdrs = stripCtl.get();
    const stripErr = stripHeaderListError(stripHdrs);
    if (stripErr) { toast('Invalid strip header', stripErr, 'err'); return; }
    if (stripHdrs.length) obj.stripResponseHeaders = stripHdrs;

    // certificateRef and http2 have no control on this form any more - the L7
    // listener picks the certificate by SNI and negotiates h2 over ALPN, so
    // neither field is read for a proxy host. Both are carried forward from the
    // stored object rather than rebuilt, so a value written in git survives an
    // unrelated save here.
    const tlsObj = {};
    if (tls.certificateRef) tlsObj.certificateRef = tls.certificateRef;
    if (tls.http2 !== undefined) tlsObj.http2 = tls.http2;
    if (isOn('f-forcessl')) tlsObj.forceSSL = true;
    const minTLS = $('#f-mintls') && $('#f-mintls').value;
    // absent-stays-absent, present-stays-present: 1.2 is the default so the
    // key is normally omitted, but a file that spells it out keeps it.
    if (minTLS && (minTLS !== '1.2' || tls.minTLSVersion === '1.2')) tlsObj.minTLSVersion = minTLS;
    if (isOn('f-hsts')) {
      const hstsObj = { enabled: true, includeSubdomains: isOn('f-hsts-sub'), preload: isOn('f-hsts-preload') };
      const maxAge = hstsMaxAgeFor(hsts.maxAge, $('#f-hsts-max'));
      if (maxAge != null) hstsObj.maxAge = maxAge;
      tlsObj.hsts = hstsObj;
    }
    // mTLS. The enable switch owns tls.clientAuth: off omits the object entirely,
    // which drops identityHeaders with it - correct, they are nested inside
    // clientAuth in the model and mean nothing without a verified certificate.
    //
    // INVARIANT, two levels and they are NOT the same:
    //   - clientAuth itself is merged over the stored object and only the fields
    //     this form renders are overwritten, so an unrendered ClientAuth field
    //     (a GitOps-authored one, or one added to the API later) survives.
    //   - identityHeaders is ALSO merged over the stored block, and then all four
    //     fields this form renders are set explicitly - including the false ones,
    //     since a cleared switch has to be able to clear the stored value. Any
    //     ClientCertHeaders field added to the model later therefore survives too,
    //     but a NEW control added to this form must be added to that explicit set
    //     or it will read as unchangeable.
    // When the client-CA list could not be loaded, caRef/mode are left exactly as
    // stored: the controls are showing placeholder state, so writing them back
    // would retarget the trust anchor on the strength of a failed request.
    if (isOn('f-mtls')) {
      const ca = Object.assign({}, tls.clientAuth);
      delete ca.identityHeaders;
      if (caListOK) {
        const caRefSel = $('#f-mtls-ca').value;
        if (!caRefSel) { toast('Client CA required', 'Select the client CA to verify certificates against, or turn client certificates off.', 'err'); return; }
        ca.caRef = caRefSel;
        ca.mode = $('#f-mtls-mode').value;
      }
      if (isOn('f-certid')) {
        const ih = Object.assign({}, (tls.clientAuth || {}).identityHeaders);
        const sh = $('#f-certid-subject').value.trim();
        if (sh) ih.subjectHeader = sh; else delete ih.subjectHeader;
        ih.san = isOn('f-certid-san');
        ih.serial = isOn('f-certid-serial');
        ih.fingerprint = isOn('f-certid-fp');
        ca.identityHeaders = ih;
      }
      tlsObj.clientAuth = ca;
    }
    if (Object.keys(tlsObj).length) obj.tls = tlsObj;

    // A picker whose list did not load is showing placeholder state, so its
    // checkboxes say nothing about what this host references. Save is already
    // disabled in that case (applyRefListGuard), but the builder carries the
    // stored value through verbatim as well so no code path can turn a failed
    // GET into a stripped middleware chain or a stripped IP allowlist.
    const mws = mwListOK ? curMw() : arr(h.middlewares);
    if (mws.length) obj.middlewares = mws;
    const als = alListOK ? curAl() : arr(h.accessLists);
    if (als.length) obj.accessLists = als;

    // trustedProxies: a host-level REPLACEMENT of settings.trustedProxies, with
    // three distinct outcomes. "Inherit" omits the key; "Trust nobody" sends an
    // explicit [] (the only way to opt one host out of a non-empty fleet list);
    // "Custom list" sends the chips. An empty custom list is a mistake, not a
    // third way of saying "nobody", so it is refused rather than sent as [].
    const tpPick = tpChoice();
    if (tpPick === 'none') {
      obj.trustedProxies = [];
    } else if (tpPick === 'custom') {
      const trusted = trustedCtl.get();
      if (!trusted.length) { toast('Trusted proxies empty', 'Add at least one CIDR or IP, or choose "Inherit fleet setting" or "Trust nobody on this host".', 'err'); return; }
      const badTp = firstBadCidr(trusted);
      if (badTp) { toast('Invalid trusted proxy', `"${badTp}" is not a CIDR or IP address. Use 10.0.0.0/8 or 192.0.2.10.`, 'err'); return; }
      obj.trustedProxies = trusted;
    }

    // Inline auth / rateLimit. The fold switch owns the key entirely: off drops
    // it, never sends null and never sends an empty object.
    if (isOn('f-auth-on')) {
      const authSpec = hostAuthCtl.get();
      if (!authSpec) return;
      obj.auth = authSpec;
    }
    if (isOn('f-rl-on')) {
      const rlSpec = hostRlCtl.get();
      if (!rlSpec) return;
      obj.rateLimit = rlSpec;
    }

    // A half-filled location row used to be DROPPED here without a word: type
    // an upstream, forget the path, hit save, get "Saved" - and the override is
    // gone. Now the row says what is missing and the save is blocked. A row
    // with nothing in it at all is still discarded silently; that is the
    // "Add path override" button's empty row, not lost work.
    const locs = [];
    let locErr = '';
    let locAborted = false;
    clearRowErrors($('#f-locs'));
    for (const ctl of locCtls) {
      if (locErr || locAborted) break;
      const row = ctl.div;
      const path = row.querySelector('.loc-path').value.trim();
      const gsel = row.querySelector('.loc-group');
      const grouped = !!(gsel && gsel.value);
      const lh = row.querySelector('.loc-host').value.trim();
      const lpRaw = row.querySelector('.loc-port').value.trim();
      const mwSel = Array.from(row.querySelector('.loc-mw').selectedOptions).map((o) => o.value);
      const alSel = Array.from(row.querySelector('.loc-al').selectedOptions).map((o) => o.value);
      const inlineOn = isOn(ctl.p + '-auth-on') || isOn(ctl.p + '-rl-on');
      if (!path) {
        if (grouped || lh || lpRaw || mwSel.length || alSel.length || inlineOn) {
          locErr = markRowError(row, 'This path override needs a path (for example /api). Fill it in, or remove the row.');
        }
        continue;
      }
      if (path[0] !== '/') {
        locErr = markRowError(row, 'A location path must start with "/".');
        continue;
      }
      const loc = { path };
      const origUp = (ctl._orig && ctl._orig.upstream) || (row._orig && row._orig.upstream) || {};
      if (grouped) {
        loc.upstreamGroupRef = gsel.value;
      } else if (lh || lpRaw) {
        const lp = parseInt(lpRaw, 10);
        const ls = row.querySelector('.loc-scheme').value;
        if (!lh || isNaN(lp)) {
          locErr = markRowError(row, 'A per-path upstream needs both a host and a port. Clear both to fall back to the host upstream.');
          continue;
        }
        // Merged over the stored upstream for the same reason as the host's:
        // path and hostHeader have to survive a save that does not touch them.
        const lup = Object.assign({}, origUp, { scheme: ls || 'http', host: lh, port: lp });
        const lextra = ctl.up.get('location ' + path);
        if (!lextra) { locAborted = true; break; }
        delete lup.path; delete lup.hostHeader;
        Object.assign(lup, lextra);
        loc.upstream = lup;
      }
      loc.middlewares = mwSel;
      loc.accessLists = alSel;
      if (isOn(ctl.p + '-strip') && path !== '/') loc.stripPrefix = true;
      if (isOn(ctl.p + '-auth-on')) {
        const la = ctl.auth.get('location ' + path);
        if (!la) { locAborted = true; break; }
        loc.auth = la;
      }
      if (isOn(ctl.p + '-rl-on')) {
        const lr = ctl.rl.get('location ' + path);
        if (!lr) { locAborted = true; break; }
        loc.rateLimit = lr;
      }
      // INVARIANT (same merge-over-stored rule as tls.clientAuth above): merge
      // over the original location instead of sending only what this form
      // renders, or any Location field this editor doesn't expose (present or
      // future) would be silently dropped on every save. Identity is the ROW,
      // not the path, so renaming a path keeps that row's other fields.
      // upstream/upstreamGroupRef are mutually exclusive, and so are the
      // fold-owned keys: an off fold must actually clear the stored block.
      const orig = Object.assign({}, ctl._orig);
      if (loc.upstreamGroupRef) delete orig.upstream; else if (loc.upstream) delete orig.upstreamGroupRef;
      delete orig.auth; delete orig.rateLimit; delete orig.stripPrefix;
      locs.push(Object.assign(orig, loc));
    }
    if (locAborted) return;
    if (locErr) { toast('Location incomplete', locErr, 'err'); return; }
    if (locs.length) obj.locations = locs;

    const btn = $('#saveBtn'); btn.disabled = true;
    try {
      const r = await api('/api/proxy-hosts/' + encodeURIComponent(nm), { method: 'PUT', body: obj });
      toastSaved(r.commit);
      refreshHeadSha(); clearDirty();
      if (isNew) location.hash = '#/hosts/' + encodeURIComponent(nm);
      else location.hash = '#/hosts';
    } catch (e) { showSaveError(e, 'Could not save this proxy host'); btn.disabled = false; }
  });

  // delete
  const del = $('#deleteBtn');
  if (del) {
    del.addEventListener('click', async () => {
      if (!confirm(`Delete proxy host "${h.name}"? This is committed to git.`)) return;
      del.disabled = true;
      try {
        const r = await api('/api/proxy-hosts/' + encodeURIComponent(h.name), { method: 'DELETE' });
        toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'host removed', 'ok', { html: true });
        refreshHeadSha(); clearDirty();
        location.hash = '#/hosts';
      } catch (e) { toastErr(e); del.disabled = false; }
    });
  }
  wireCloneButton('hosts', h, 'hostCloneBtn');
}

// Effective ACME challenge for a certificate: the explicit value, else dns-01
// when a DNS provider is referenced (back-compat) and http-01 otherwise. Mirrors
// ACMESpec.EffectiveChallenge in the model.
function certChallenge(ct) {
  const a = (ct && ct.acme) || {};
  if (a.challenge) return a.challenge;
  return a.dnsProvider ? 'dns-01' : 'http-01';
}

// ---------- CERTIFICATES LIST ----------
// A table rather than a card grid, because the questions asked of this page are
// comparative ("which of these expires first", "which one covers this domain")
// and cards cannot be sorted or scanned column-wise.
const CERTS_SORT_KEY = 'gpm.certs.sort';

function certExpiry(ct) {
  if (!ct) return '';
  return ct.notAfter || ct.expiresAt || (ct.status && ct.status.notAfter) || '';
}

async function listCerts(c) {
  const certs = arr((await api('/api/certificates')).data);
  const head = viewHead('Certificates',
    'The TLS certificates gpm presents at the edge, issued by ACME or brought as PEM files. A host never picks one: the handshake selects by domain, so a certificate covering the name is all a host needs.',
    `<a class="btn primary" href="#/certs/_new">${ICON.plus}Add certificate</a>`)
    + aboutPageHtml('page.certificates');
  if (!certs.length) {
    c.innerHTML = head + emptyState('No certificates yet',
      'Request an ACME certificate for the domains you serve - dns-01 if you need a wildcard, http-01 otherwise - or point gpm at PEM files already on the server.',
      'Add certificate', '#/certs/_new');
    return;
  }

  const recs = certs.map((ct) => {
    const domains = arr(ct.domains);
    const type = ct.type || 'cert';
    const challenge = type === 'acme' ? certChallenge(ct) : '';
    const detail = type === 'acme'
      ? ((ct.acme && ct.acme.email) || '')
      : [(ct.custom && ct.custom.certFile) || '', (ct.custom && ct.custom.keyFile) || ''].filter(Boolean).join(' ');
    const provider = (ct.acme && ct.acme.dnsProvider) || '';
    return {
      ct, domains, type, challenge, provider,
      name: ct.name || '',
      expiry: certExpiry(ct),
      state: ct.state || '',
      issuer: ct.issuer || '',
      blob: [ct.name, ct.displayName, domains.join(' '), type, challenge, provider, detail, ct.issuer, ct.state].join(' ').toLowerCase(),
    };
  });

  let sort = { key: 'name', dir: 1 };
  try {
    const raw = localStorage.getItem(CERTS_SORT_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (parsed && parsed.key) sort = { key: parsed.key, dir: parsed.dir === -1 ? -1 : 1 };
    }
  } catch (e) { /* ignore */ }
  const SORTS = {
    name: (r) => r.name.toLowerCase(),
    domains: (r) => (r.domains[0] || '').toLowerCase(),
    // Blank expiry sorts last in either direction rather than pretending to be
    // the earliest date, which is what an empty string would do.
    expiry: (r) => r.expiry || '9999',
  };
  const COLUMNS = [
    { key: 'name', label: 'Name' },
    { key: 'domains', label: 'Domains', cls: 'col-domains' },
    { key: null, label: 'Type' },
    { key: null, label: 'Issuance', cls: 'col-issuance' },
    { key: null, label: 'Issuer' },
    { key: 'expiry', label: 'Expiry', cls: 'col-expiry' },
    { key: null, label: '', cls: 'col-actions' },
  ];

  c.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="certFilter" placeholder="filter: name, domain, provider, issuer, state..." aria-label="Filter certificates" /></div>
    </div>
    <div class="table-wrap">
      <table class="certs-table">
        <thead id="certHead"></thead>
        <tbody id="certRows"></tbody>
      </table>
    </div>`;

  function headHtml() {
    return '<tr>' + COLUMNS.map((col) => {
      const cls = col.cls ? ` class="${col.key ? 'sortable ' : ''}${col.cls}"` : (col.key ? ' class="sortable"' : '');
      if (!col.key) return `<th${cls}>${esc(col.label)}</th>`;
      const active = sort.key === col.key;
      const aria = active ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none';
      return `<th${cls} data-sort="${col.key}" aria-sort="${aria}" role="button" tabindex="0">${esc(col.label)}<span class="sort-mark">${active ? (sort.dir === 1 ? '&#9650;' : '&#9660;') : '&#9650;'}</span></th>`;
    }).join('') + '</tr>';
  }

  function rowHtml(r) {
    const issuance = r.type === 'acme'
      ? `${esc(r.challenge)}${r.provider ? ' via ' + esc(r.provider) : ''}`
      : 'PEM files on the server';
    return `<tr class="clickable" data-name="${esc(r.ct.name)}">
      <td><span class="host">${esc(r.name)}</span></td>
      <td class="mono col-domains">${domainListHtml(r.domains)}</td>
      <td><span class="chip ${r.type === 'acme' ? 'cyan' : ''}">${esc(r.type)}</span></td>
      <td class="mono faint col-issuance">${issuance}</td>
      <td class="mono faint">${esc(r.issuer || '-')}</td>
      <td class="col-expiry">${certExpiryCellHtml(r.ct)}</td>
      <td class="col-actions">
        ${r.type === 'acme' ? `<button class="btn ghost sm ct-renew" data-name="${esc(r.ct.name)}" type="button">Renew now</button>` : ''}
        <button class="btn ghost sm ct-clone" data-name="${esc(r.ct.name)}" type="button">Clone</button>
      </td>
    </tr>`;
  }

  function render() {
    const q = (($('#certFilter') && $('#certFilter').value) || '').trim().toLowerCase();
    const get = SORTS[sort.key] || SORTS.name;
    const rows = recs.filter((r) => !q || r.blob.indexOf(q) !== -1)
      .sort((a, b) => {
        const av = get(a), bv = get(b);
        if (av === bv) return a.name.localeCompare(b.name);
        return (av < bv ? -1 : 1) * sort.dir;
      });
    $('#certHead').innerHTML = headHtml();
    $('#certRows').innerHTML = rows.length
      ? rows.map(rowHtml).join('')
      : `<tr><td colspan="7" class="list-empty">No certificate matches this filter.</td></tr>`;
    $$('#certHead th.sortable').forEach((th) => {
      const apply = () => {
        const key = th.dataset.sort;
        sort = { key, dir: sort.key === key ? -sort.dir : 1 };
        try { localStorage.setItem(CERTS_SORT_KEY, JSON.stringify(sort)); } catch (e) { /* ignore */ }
        render();
      };
      th.addEventListener('click', apply);
      th.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); apply(); } });
    });
    $$('#certRows tr.clickable').forEach((tr) => {
      tr.addEventListener('click', (e) => {
        if (e.target.closest('.ct-clone') || e.target.closest('.ct-renew')) return;
        location.hash = '#/certs/' + encodeURIComponent(tr.dataset.name);
      });
    });
    $$('#certRows .ct-clone').forEach((b) => {
      b.addEventListener('click', (e) => {
        e.stopPropagation();
        const ct = certs.find((x) => x.name === b.dataset.name);
        if (ct) startClone('certs', ct);
      });
    });
    $$('#certRows .ct-renew').forEach((b) => {
      b.addEventListener('click', async (e) => {
        e.stopPropagation();
        b.disabled = true;
        try {
          if (await renewCertificate(b.dataset.name)) await listCerts(c);
        } finally { b.disabled = false; }
      });
    });
    if (isReadOnly()) {
      $$('#certRows .ct-renew').forEach((b) => gateControl(b, false, readOnlyReason()));
    }
  }
  $('#certFilter').addEventListener('input', render);
  render();
}

// ---------- CERTIFICATE EDITOR ----------
// Read-only card describing the material currently installed, not the config
// that asked for it. Omitted entirely when the API reports no state (an older
// build), rather than rendering a wall of dashes.
function certStatusCardHtml(ct) {
  const acme = ct.type === 'acme';
  const days = ct.daysRemaining;
  const sans = arr(ct.sans).length ? arr(ct.sans) : arr(ct.domains);
  const row = (k, v, cls) => `<span class="k">${esc(k)}</span><span class="v ${cls || ''}">${v}</span>`;
  return `<div class="card form-section" id="ct-status">
    <p class="section-label">Status</p>
    <div class="kv">
      ${row('State', certStateChip(ct))}
      ${row('Expiry', `<span class="mono">${esc(ct.notAfter ? fmtTime(ct.notAfter) : '-')}</span>`)}
      ${row('Days remaining', `<span class="mono${days != null && days < 0 ? ' err-text' : ''}">${days != null ? esc(days) : '-'}</span>`)}
      ${row('Issuer', `<span class="mono">${esc(ct.issuer || '-')}</span>`)}
      ${row('SANs', `<span class="mono" style="word-break:break-all">${esc(sans.join(', ') || '-')}</span>`)}
      ${acme ? row('Last renewal attempt', `<span class="mono">${esc(ct.lastAttempt ? fmtTime(ct.lastAttempt) : 'Never')}</span>`) : ''}
      ${acme && ct.lastError ? row('Last error', `<span class="err-text" style="word-break:break-word">${esc(ct.lastError)}</span>`) : ''}
    </div>
  </div>`;
}

async function certEditor(c, name) {
  const isNew = !name;
  const seed = isNew ? takeCloneSeed('certs') : null;
  const [dnsR, certR] = await Promise.all([
    refList('/api/dns-providers', 'DNS providers'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/certificates/' + encodeURIComponent(name)),
  ]);
  const dnsProviders = arr(dnsR);
  const ct = seed ? seed.data : (certR.data || {});
  const acme = ct.acme || {};
  const custom = ct.custom || {};
  const type = ct.type || 'acme';
  // Default a brand-new (non-cloned) cert to dns-01 only when a provider exists
  // to solve it; a clone carries its source object's real challenge/provider.
  const challenge = (isNew && !seed) ? (dnsProviders.length ? 'dns-01' : 'http-01') : certChallenge(ct);

  c.innerHTML = `
    <div class="row-between view-head">
      <div>
        <div class="muted" style="font-size:12px;margin-bottom:3px"><a href="#/certs">Certificates</a> / ${isNew ? 'new' : 'edit'}</div>
        <h2 style="font-family:var(--display)">${esc(isNew ? 'New certificate' : ct.name)}</h2>
        <p>Terminate TLS with an ACME-issued or custom certificate.</p>
      </div>
    </div>

    <div class="form-grid">
      <div class="stack">
      ${(!isNew && ct.state) ? certStatusCardHtml(ct) : ''}
      <div class="card form-section">
        <p class="section-label">Certificate</p>
        <div class="field-group">
          <label>Name</label>
          <input class="field mono" id="ct-name" data-hint="common.name" data-path="name" value="${esc(ct.name || '')}" ${isNew ? '' : 'disabled'} placeholder="${esc(seed ? seed.origName + '-copy' : 'wild')}" />
          <div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div>
        </div>
        <div class="field-group">
          <label>Type</label>
          <select class="field" id="ct-type" data-hint="certificate.type" data-path="type">
            ${enumOptions('certType', ['acme', 'custom'], type)}
          </select>
        </div>
        <div class="field-group">
          <label>Domains</label>
          <div class="chip-input" id="ct-domains" data-hint="certificate.domains" data-path="domains"></div>
          <div class="hint">Press Enter to add each domain.</div>
        </div>
      </div>
      </div>

      <div class="card form-section">
        <div id="acme-fields" style="${type === 'custom' ? 'display:none' : ''}">
          <p class="section-label">ACME</p>
          <div class="field-group"><label>Account email</label><input class="field mono" id="ct-email" data-hint="certificate.acme.email" data-path="acme.email" value="${esc(acme.email || '')}" placeholder="you@example.com" /></div>
          <div class="field-group"><label>Directory URL</label><input class="field mono" id="ct-dir" data-hint="certificate.acme.directoryURL" data-path="acme.directoryURL" value="${esc(acme.directoryURL || '')}" placeholder="https://acme-v02.api.letsencrypt.org/directory" /></div>
          <div class="inline-fields">
            <div class="field-group"><label>Key type</label><input class="field mono" id="ct-keytype" data-hint="certificate.acme.keyType" data-path="acme.keyType" value="${esc(acme.keyType || '')}" placeholder="EC256" /></div>
            <div class="field-group"><label>Challenge</label>
              <select class="field mono" id="ct-challenge" data-hint="certificate.acme.challenge" data-path="acme.challenge">
                <option value="http-01"${challenge === 'http-01' ? ' selected' : ''}>${esc(enumLabel('acmeChallenge', 'http-01'))}</option>
                <option value="dns-01"${challenge === 'dns-01' ? ' selected' : ''}${(dnsProviders.length || challenge === 'dns-01') ? '' : ' disabled'}>${esc(enumLabel('acmeChallenge', 'dns-01'))}${dnsProviders.length ? '' : ' - no provider configured'}</option>
              </select>
              <div class="hint">${dnsProviders.length ? 'http-01 is validated on port 80; dns-01 is the only way to get a wildcard.' : 'dns-01 needs a DNS provider - add one under DNS Providers.'}</div>
            </div>
          </div>
          <div class="field-group" id="ct-dns-group" style="${challenge === 'dns-01' ? '' : 'display:none'}">
            <label>DNS provider</label>
            <select class="field mono" id="ct-dns" data-hint="certificate.acme.dnsProvider" data-path="acme.dnsProvider">
              <option value="">select provider...</option>
              ${dnsProviders.map((p) => `<option value="${esc(p.name)}"${acme.dnsProvider === p.name ? ' selected' : ''}>${esc(p.name)}</option>`).join('')}
            </select>
            ${dnsProviders.length ? '' : '<div class="hint">No DNS providers configured yet. Add one under DNS Providers.</div>'}
          </div>
          <div class="field-group">
            <div class="toggle-line"><div class="tl-text"><div class="nm">External account binding</div><div class="ds">Required by ZeroSSL and Google Public CA</div></div>${switchHtml('ct-eab', !!(acme.eab && acme.eab.kid), 'External account binding', 'certificate.acme.eab')}</div>
          </div>
          <div id="ct-eab-fields" style="${(acme.eab && acme.eab.kid) ? '' : 'display:none'}">
            <div class="field-group"><label>EAB key ID</label><input class="field mono" id="ct-eab-kid" data-hint="certificate.acme.eab.kid" data-path="acme.eab.kid" value="${esc((acme.eab && acme.eab.kid) || '')}" placeholder="kid from the CA" /></div>
            <div class="field-group"><label>EAB HMAC key</label><input class="field mono" id="ct-eab-hmac" data-hint="certificate.acme.eab.hmacKey" data-path="acme.eab.hmacKey" value="${esc((acme.eab && acme.eab.hmacKey) || '')}" placeholder="\${ENV:ACME_EAB_HMAC}" />
              <div class="hint">base64url, as issued by the CA. Use a <span class="mono">\${ENV:...}</span> or <span class="mono">\${FILE:...}</span> placeholder so no secret is committed.</div>
            </div>
          </div>
        </div>
        <div id="custom-fields" style="${type === 'custom' ? '' : 'display:none'}">
          <p class="section-label">Custom certificate</p>
          <div class="hint" style="margin-bottom:8px">Paths are relative to the cert store on the gpm host (<span class="mono">-cert-dir</span>, default <span class="mono">/data/certs</span>) - there is no upload here, the files must already exist there.</div>
          <div class="field-group"><label>Certificate file</label><input class="field mono" id="ct-certfile" data-hint="certificate.custom.certFile" data-path="custom.certFile" value="${esc(custom.certFile || '')}" placeholder="fullchain.pem" /></div>
          <div class="field-group"><label>Key file</label><input class="field mono" id="ct-keyfile" data-hint="certificate.custom.keyFile" data-path="custom.keyFile" value="${esc(custom.keyFile || '')}" placeholder="privkey.pem" /></div>
        </div>
      </div>
    </div>

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        ${(!isNew && type === 'acme') ? `<button class="btn ghost" id="ed-renew" type="button">Renew now</button>` : ''}
        ${isNew ? '' : `<button class="btn ghost" id="ct-clone" type="button">Clone</button>`}
        ${isNew ? '' : `<button class="btn danger" id="ct-delete" type="button">${ICON.trash}Delete</button>`}
        <a class="btn ghost" href="#/certs">Cancel</a>
        <button class="btn primary" id="ct-save" type="button">${isNew ? 'Issue certificate' : 'Save changes'}</button>
      </div>
    </div>`;

  const domainsCtl = makeChipInput($('#ct-domains'), arr(ct.domains), 'add domain...');
  $('#ct-type').addEventListener('change', () => {
    const t = $('#ct-type').value;
    $('#acme-fields').style.display = t === 'acme' ? '' : 'none';
    $('#custom-fields').style.display = t === 'custom' ? '' : 'none';
  });
  // The DNS provider only applies to dns-01; http-01 is solved on port 80.
  $('#ct-challenge').addEventListener('change', () => {
    $('#ct-dns-group').style.display = $('#ct-challenge').value === 'dns-01' ? '' : 'none';
  });
  $('#ct-eab').addEventListener('switchchange', () => {
    $('#ct-eab-fields').style.display = isOn('ct-eab') ? '' : 'none';
  });

  $('#ct-save').addEventListener('click', async () => {
    clearEditorError();
    const nm = isNew ? $('#ct-name').value.trim() : ct.name;
    if (!nm) { toast('Name required', 'Enter a certificate name.', 'err'); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return; }
    const t = $('#ct-type').value;
    const obj = { name: nm, type: t, domains };
    if (t === 'acme') {
      const ch = $('#ct-challenge').value;
      const a = { email: $('#ct-email').value.trim(), challenge: ch };
      const dir = $('#ct-dir').value.trim(); if (dir) a.directoryURL = dir;
      const kt = $('#ct-keytype').value.trim(); if (kt) a.keyType = kt;
      if (ch === 'dns-01') {
        a.dnsProvider = $('#ct-dns').value;
        if (!a.dnsProvider) { toast('DNS provider required', 'Select a DNS provider for dns-01.', 'err'); return; }
      } else if (domains.some((d) => d.startsWith('*.'))) {
        toast('Wildcard needs dns-01', 'A wildcard domain can only be validated over dns-01.', 'err'); return;
      }
      if (isOn('ct-eab')) {
        const kid = $('#ct-eab-kid').value.trim();
        const hmac = $('#ct-eab-hmac').value.trim();
        if (!kid || !hmac) { toast('EAB incomplete', 'Enter both the EAB key ID and HMAC key.', 'err'); return; }
        if (hmac === '***') { toast('Secret masked', 'The EAB HMAC key reads *** - replace it with a real value or a ${ENV:...} placeholder.', 'err'); return; }
        a.eab = { kid, hmacKey: hmac };
      }
      obj.acme = a;
    } else {
      obj.custom = { certFile: $('#ct-certfile').value.trim(), keyFile: $('#ct-keyfile').value.trim() };
    }
    const btn = $('#ct-save'); btn.disabled = true;
    try {
      const r = await api('/api/certificates/' + encodeURIComponent(nm), { method: 'PUT', body: obj });
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      location.hash = '#/certs';
    } catch (e) { showSaveError(e, 'Could not save this certificate'); btn.disabled = false; }
  });

  // Renew now: same shared action as the list's per-row button. The order runs
  // in the background, so this re-reads THIS certificate and repaints its status
  // card rather than leaving the page stale until the next navigation.
  const renewBtn = $('#ed-renew');
  if (renewBtn) renewBtn.addEventListener('click', async () => {
    renewBtn.disabled = true;
    try {
      if (await renewCertificate(ct.name)) {
        const fresh = (await api('/api/certificates/' + encodeURIComponent(ct.name)).catch(() => ({ data: null }))).data;
        const card = $('#ct-status');
        if (fresh && fresh.state && card) card.outerHTML = certStatusCardHtml(fresh);
      }
    } finally { renewBtn.disabled = false; }
  });

  const del = $('#ct-delete');
  if (del) del.addEventListener('click', async () => {
    if (!confirm(`Delete certificate "${ct.name}"?`)) return;
    del.disabled = true;
    try {
      const r = await api('/api/certificates/' + encodeURIComponent(ct.name), { method: 'DELETE' });
      toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'removed', 'ok', { html: true });
      refreshHeadSha(); location.hash = '#/certs';
    } catch (e) { toastErr(e); del.disabled = false; }
  });
  wireCloneButton('certs', ct, 'ct-clone');
}

// ---------- GENERIC SECTIONS (list + JSON-editor form) ----------
const SECTION_META = {
  clientcas: {
    title: 'Client CAs', sub: 'The authority a client certificate is checked against. Add one when a host should ask devices for a certificate at the TLS handshake instead of, or as well as, a sign-in.',
    empty: 'Generate a CA here and gpm can issue the client certificates too; paste an existing certificate if your organisation already runs a CA.',
    singular: 'client CA', addLabel: 'Add client CA',
    summary: (o) => `<span class="k">Revocation</span><span class="v">${o.crlFile ? 'file: ' + esc(o.crlFile) : (o.crlPEM ? 'inline CRL' : 'none')}</span>` +
      ((o.crlFile || o.crlPEM) ? `<span class="k">Policy</span><span class="v">${enumChip('crlPolicy', o.crlPolicy === 'fail-open' ? 'fail-open' : '')}</span>` : ''),
  },
  identity: {
    title: 'Identity Providers', sub: 'Where gpm sends people to prove who they are: an OIDC provider, a trusted forward-auth proxy, or an external auth-request outpost. An auth middleware points at one of these.',
    empty: 'Add the provider first. An auth middleware then references it, and every host that picks up that middleware is protected by it.',
    singular: 'identity provider', addLabel: 'Add identity provider',
    summary: (o) => `<span class="k">Type</span><span class="v">${enumChip('idpType', o.type || '')}</span>` +
      (o.oidc ? `<span class="k">Issuer</span><span class="v">${esc(o.oidc.issuerURL || '')}</span><span class="k">Client ID</span><span class="v">${esc(o.oidc.clientID || '')}</span>` : '') +
      (o.forwardAuth ? `<span class="k">User header</span><span class="v">${esc(o.forwardAuth.userHeader || '')}</span>` : ''),
  },
  access: {
    title: 'Access Lists', sub: 'Who may reach a host, by IP range or country. Attach one to a host or to a single location.',
    empty: 'Start with a default action of deny plus one allow rule for your own network, then attach the list to a host.',
    singular: 'access list', addLabel: 'Add access list',
    summary: (o) => `<span class="k">Rules</span><span class="v">${arr(o.rules).length}</span>` +
      `<span class="k">Sources</span><span class="v">${arr(o.sources).length}</span>` +
      (o.defaultAction ? `<span class="k">Default</span><span class="v">${esc(o.defaultAction)}</span>` : '') +
      // Surfaced on the list so an unmigrated basic-auth block is visible
      // without opening every editor.
      (arr(o.basicAuth).length ? `<span class="k">Legacy</span><span class="v"><span class="chip warn">Deprecated</span> ${arr(o.basicAuth).length} basic-auth user(s)</span>` : ''),
  },
  middleware: {
    title: 'Middleware', sub: 'Reusable steps any host can pick up: authentication, headers, rate limits, guards, path rewrites and deny hooks. Evaluation order is fixed, whatever order a host lists them in: rate limit -> access list -> bouncer -> auth -> guard -> headers -> rewrite -> upstream.',
    empty: 'Most installs start with one auth middleware pointing at an identity provider, then attach it to the hosts that need signing in.',
    singular: 'middleware', addLabel: 'Add middleware',
    summary: (o) => `<span class="k">Type</span><span class="v">${enumChip('middlewareType', o.type || '')}${(o.type === 'auth' && o.auth && o.auth.mode) ? ' - ' + esc(enumLabel('authMode', o.auth.mode).split(' - ')[0]) : ''}</span>` +
      (o.auth && o.auth.identityProvider ? `<span class="k">IdP</span><span class="v">${esc(o.auth.identityProvider)}</span>` : '') +
      (o.auth && o.auth.basic ? `<span class="k">Credentials</span><span class="v">${arr(o.auth.basic.users).length} user(s)</span>` : '') +
      (o.rateLimit ? `<span class="k">Rate</span><span class="v">${o.rateLimit.window ? esc(o.rateLimit.requests) + ' / ' + esc(o.rateLimit.window) : esc(o.rateLimit.requestsPerSecond) + ' r/s'}</span>` : '') +
      (o.rateLimit && o.rateLimit.blockFor ? `<span class="k">Block</span><span class="v">${esc(o.rateLimit.blockFor)}</span>` : '') +
      (o.rewrite && o.rewrite.replacePath && Object.keys(o.rewrite.replacePath).length ? `<span class="k">Rewrite</span><span class="v">${Object.keys(o.rewrite.replacePath).length} path(s)</span>` : '') +
      (o.bouncer ? `<span class="k">Bouncer</span><span class="v">${esc(o.bouncer.provider || 'crowdsec')}${o.bouncer.stream ? ' (stream)' : ''}</span><span class="k">On error</span><span class="v">${esc(o.bouncer.onError || 'fail-open')}</span>` : ''),
  },
  dns: {
    title: 'DNS Providers', sub: 'API credentials for the DNS zones gpm writes into - dns-01 ACME challenges, and published records where DNS sync is on.',
    empty: 'Add the provider that hosts your zone and give it a scoped API token, as a ${ENV:...} placeholder so no secret is committed.',
    singular: 'DNS provider', addLabel: 'Add DNS provider',
    summary: (o) => `<span class="k">Provider</span><span class="v">${esc(o.provider || '')}</span>` +
      (o.config ? `<span class="k">Config keys</span><span class="v">${esc(Object.keys(o.config).join(', '))}</span>` : ''),
  },
  redirects: {
    title: 'Redirects', sub: 'A domain that answers with a redirect instead of proxying anything - retired names, vanity hostnames, apex to www.',
    empty: 'Add the old domain, the target it should send visitors to, and whether the redirect is permanent.',
    singular: 'redirect', addLabel: 'Add redirect',
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>` +
      (o.targetDomain ? `<span class="k">To</span><span class="v">${esc(((o.targetScheme && o.targetScheme !== 'auto') ? o.targetScheme + '://' : '') + o.targetDomain)}</span>` : '') +
      (o.statusCode ? `<span class="k">Code</span><span class="v">${esc(o.statusCode)}</span>` : ''),
  },
  streams: {
    title: 'Streams', sub: 'Raw TCP or UDP forwarding for services that do not speak HTTP, with optional SNI routing and TLS termination.',
    empty: 'Add the port gpm should listen on and the host and port behind it. Leave TLS on none unless you need SNI routing.',
    singular: 'stream', addLabel: 'Add stream',
    summary: (o) => (o.listenPort != null ? `<span class="k">Listen</span><span class="v">:${esc(o.listenPort)}</span>` : '') +
      (o.target && o.target.host ? `<span class="k">Target</span><span class="v">${esc(o.target.host + ':' + (o.target.port != null ? o.target.port : ''))}</span>` : '') +
      (o.protocol ? `<span class="k">Protocol</span><span class="v">${enumChip('streamProtocol', o.protocol)}</span>` : ''),
  },
  parked: {
    title: 'Parked Hosts', sub: 'A domain that answers but serves nothing. Reserve a name you own, or return a clean 404 instead of falling through to another host.',
    empty: 'Add the domains to reserve. They answer 404 by default; change the status code if you want something else.',
    singular: 'parked host', addLabel: 'Add parked host',
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>`,
  },
  upstreams: {
    title: 'Upstream Groups', sub: 'Several backends behind one name, with health checks and a load-distribution policy. A host references the group instead of a single upstream.',
    empty: 'Add two or more backends, then pick how requests are spread across them. A health check keeps a dead one out of the rotation.',
    singular: 'upstream group', addLabel: 'Add upstream group',
    summary: (o) => `<span class="k">Upstreams</span><span class="v">${arr(o.upstreams).map((u) => esc((u.host || '') + ':' + (u.port != null ? u.port : '') + (u.weight ? ' w' + u.weight : ''))).join(' -> ') || '0'}</span>` +
      `<span class="k">Policy</span><span class="v">${enumChip('loadBalance', o.policy || '')}</span>` +
      (o.stickiness && o.stickiness.ttl ? `<span class="k">Sticky</span><span class="v">${esc(o.stickiness.ttl)}</span>` : '') +
      (o.healthCheck && o.healthCheck.path ? `<span class="k">Probe</span><span class="v">GET ${esc(o.healthCheck.path)}</span>` : `<span class="k">Probe</span><span class="v">TCP</span>`),
  },
};

async function genericSection(c, section, sub) {
  if (sub === '_new') return EDITORS[section](c, null);
  if (sub) return EDITORS[section](c, sub);
  return genericList(c, section);
}

// Every value a stored object holds, flattened to one lowercase haystack for
// the list filter. Values only, never keys: matching on keys would make every
// object match "name", "type" and "domains".
function objSearchBlob(o) {
  const out = [];
  (function walk(v) {
    if (v == null) return;
    if (Array.isArray(v)) { v.forEach(walk); return; }
    if (typeof v === 'object') { Object.keys(v).forEach((k) => walk(v[k])); return; }
    out.push(String(v));
  })(o);
  return out.join(' ').toLowerCase();
}

async function genericList(c, section) {
  const meta = SECTION_META[section];
  const plural = PLURAL[section];
  const items = arr((await api('/api/' + plural)).data);
  const head = viewHead(meta.title, meta.sub,
    `<a class="btn primary" href="#/${section}/_new">${ICON.plus}${meta.addLabel}</a>`)
    + aboutPageHtml(PAGE_HINT[section]);
  if (!items.length) {
    c.innerHTML = head + emptyState(`No ${meta.singular}s yet`,
      meta.empty || `Add your first ${meta.singular}.`, meta.addLabel, `#/${section}/_new`);
    return;
  }
  const blobs = {};
  items.forEach((o) => { blobs[o.name] = objSearchBlob(o); });
  const cards = items.map((o) => `
    <div class="card" data-name="${esc(o.name)}" role="button" tabindex="0" style="cursor:pointer">
      <div class="card-head">
        <div><h3>${esc(o.name)}</h3>${o.displayName ? `<div class="faint" style="font-size:11.5px">${esc(o.displayName)}</div>` : ''}</div>
        <div style="display:flex;gap:8px">
          <button class="btn ghost sm gs-clone" data-name="${esc(o.name)}" type="button">Clone</button>
          <button class="btn ghost sm danger gs-del" data-name="${esc(o.name)}" type="button">Delete</button>
        </div>
      </div>
      <div class="kv">${meta.summary(o)}</div>
    </div>`).join('');
  c.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="gsFilter" placeholder="filter ${esc(meta.title.toLowerCase())}..." aria-label="Filter ${esc(meta.title)}" /></div>
    </div>
    <div class="cards" id="gsCards">${cards}</div>
    <div class="list-empty" id="gsNone" hidden>No ${esc(meta.singular)} matches this filter.</div>`;

  // The filter matches the object's stored values, not the summary markup the
  // card happens to render.
  const gsFilter = $('#gsFilter');
  gsFilter.addEventListener('input', () => {
    const q = gsFilter.value.trim().toLowerCase();
    let shown = 0;
    $$('#gsCards .card[data-name]').forEach((el) => {
      const hit = !q || (blobs[el.dataset.name] || '').indexOf(q) !== -1;
      el.style.display = hit ? '' : 'none';
      if (hit) shown++;
    });
    $('#gsNone').hidden = shown > 0;
  });

  $$('.cards .card[data-name]').forEach((el) => {
    const open = () => { location.hash = `#/${section}/` + encodeURIComponent(el.dataset.name); };
    el.addEventListener('click', (e) => { if (!e.target.closest('.gs-del') && !e.target.closest('.gs-clone')) open(); });
    el.addEventListener('keydown', (e) => { if (e.key === 'Enter') open(); });
  });
  $$('.gs-clone').forEach((b) => {
    b.addEventListener('click', (e) => {
      e.stopPropagation();
      const obj = items.find((x) => x.name === b.dataset.name);
      if (obj) startClone(section, obj);
    });
  });
  $$('.gs-del').forEach((b) => {
    b.addEventListener('click', async (e) => {
      e.stopPropagation();
      const nm = b.dataset.name;
      if (!confirm(`Delete ${meta.singular} "${nm}"?`)) return;
      b.disabled = true;
      try {
        const r = await api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'DELETE' });
        toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'removed', 'ok', { html: true });
        refreshHeadSha(); route();
      } catch (err) { toastErr(err); b.disabled = false; }
    });
  });
}

// ---------- editor scaffolding shared by the typed object editors ----------
function editorHead(section, meta, isNew, name) {
  return `<div class="row-between view-head"><div>
    <div class="muted" style="font-size:12px;margin-bottom:3px"><a href="#/${section}">${esc(meta.title)}</a> / ${isNew ? 'new' : 'edit'}</div>
    <h2 style="font-family:var(--display)">${esc(isNew ? 'New ' + meta.singular : name)}</h2>
    <p>${esc(meta.sub)}</p>
  </div></div>`;
}
function saveBar(section, isNew, addLabel) {
  return `<div class="panel save-bar">
    <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
    <div style="display:flex;gap:10px">
      ${isNew ? '' : `<button class="btn ghost" id="ed-clone" type="button">Clone</button>`}
      ${isNew ? '' : `<button class="btn danger" id="ed-delete" type="button">${ICON.trash}Delete</button>`}
      <a class="btn ghost" href="#/${section}">Cancel</a>
      <button class="btn primary" id="ed-save" type="button">${esc(isNew ? addLabel : 'Save changes')}</button>
    </div>
  </div>`;
}
// clonePlaceholder, when set (rendering a clone seed's editor), replaces the
// generic "internal-name" placeholder with "<original>-copy".
//
// foldable collapses the card to its one-line summary on the editors that have
// enough other fields to need the room (redirect, stream, parked). It never
// folds a NEW object's card - the name is required there and immutable
// afterwards, so hiding the only chance to set it would be a trap - and never
// folds one that already holds a display name or a disabled flag.
function nameCard(obj, isNew, clonePlaceholder, foldable) {
  const body = `
    <div class="inline-fields">
      <div class="field-group"><label>Name</label><input class="field mono" id="ed-name" data-hint="common.name" data-path="name" value="${esc(obj.name || '')}" ${isNew ? '' : 'disabled'} placeholder="${esc(clonePlaceholder || 'internal-name')}" /><div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div></div>
      <div class="field-group"><label>Display name</label><input class="field" id="ed-display" data-hint="common.displayName" data-path="displayName" value="${esc(obj.displayName || '')}" placeholder="optional label" /></div>
    </div>
    <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Disabled</div><div class="ds">Exclude from the running proxy</div></div>${switchHtml('ed-disabled', !!obj.disabled, 'Disabled', 'common.disabled')}</div>`;
  if (!foldable) return `<div class="card form-section"><p class="section-label">Identity</p>${body}</div>`;
  const parts = [];
  if (obj.displayName) parts.push(obj.displayName);
  if (obj.disabled) parts.push('disabled');
  const open = isNew || parts.length > 0;
  return foldHtml('ed-id-card', 'Identity', parts.length ? parts.join(' - ') : (obj.name || 'name and status'), open, body);
}
// ---------- automatic certificate selection (L7) ----------
// On an HTTP host (proxy, redirect, parked) gpm picks the certificate from the
// TLS handshake's SNI by matching the served name against every certificate's
// domains - tls.certificateRef is NOT read on those kinds. Offering a picker
// there was a control that did nothing: an operator could select a certificate
// and still be served a different one. So the three HTTP editors show what the
// domains actually resolve to, read-only, and the picker survives only on the
// Stream host editor, where terminate mode does read certificateRef.
//
// Match order mirrors the data plane: an exact domain first, then a wildcard
// covering the immediate parent (*.example.com covers a.example.com and NOT
// b.a.example.com, exactly like a TLS wildcard).
function certForDomain(certs, domain) {
  const d = String(domain || '').trim().toLowerCase();
  if (!d) return null;
  const dot = d.indexOf('.');
  const parent = dot > 0 ? d.slice(dot + 1) : '';
  const list = arr(certs);
  let wildcard = null;
  for (let i = 0; i < list.length; i++) {
    const names = arr(list[i].domains);
    for (let j = 0; j < names.length; j++) {
      const cd = String(names[j] || '').trim().toLowerCase();
      if (!cd) continue;
      if (cd === d) return list[i];
      if (!wildcard && parent && cd.slice(0, 2) === '*.' && cd.slice(2) === parent) wildcard = list[i];
    }
  }
  return wildcard;
}

// Read-only replacement for the certificate picker: one line per domain saying
// which certificate the handshake will present, or that nothing covers it.
function certCoverageHtml(certs, domains) {
  const list = arr(domains).filter(Boolean);
  if (!list.length) return '<div class="hint">Add a domain to see which certificate covers it.</div>';
  return list.map((d) => {
    const ct = certForDomain(certs, d);
    return ct
      ? `<div class="cert-auto"><span class="mono">${esc(d)}</span> Certificate: <b>${esc(ct.name)}</b> (selected automatically by domain)</div>`
      : `<div class="cert-auto warn"><span class="mono">${esc(d)}</span> none covers ${esc(d)}</div>`;
  }).join('');
}

const CERT_AUTO_HINT = 'gpm selects the certificate from the TLS handshake (SNI) by matching the served name against every certificate\'s domains, so there is nothing to pick here. '
  + 'Issue or import a certificate covering the domain under <a href="#/certs">Certificates</a>.';

function tlsCard(tls, certs, domains) {
  tls = tls || {}; const hsts = tls.hsts || {};
  return `<div class="card form-section">
    <p class="section-label">TLS</p>
    <div class="field-group"><label>Certificate</label>
      <div id="ed-cert-auto">${certCoverageHtml(certs, domains)}</div>
      <div class="hint">${CERT_AUTO_HINT}</div>
    </div>
    <div style="margin-top:6px">
      <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS</div></div>${switchHtml('ed-forcessl', !!tls.forceSSL, 'Force SSL', 'proxyHost.tls.forceSSL')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Send <span class="mono">Strict-Transport-Security</span></div></div>${switchHtml('ed-hsts', !!hsts.enabled, 'HSTS', 'proxyHost.tls.hsts.enabled')}</div>
    </div>
    <div id="ed-hsts-fields" style="margin-top:12px;${hsts.enabled ? '' : 'display:none'}">
      <div class="inline-fields"><div class="field-group"><label>Max age (s)</label><input class="field mono" id="ed-hsts-max" data-hint="proxyHost.tls.hsts.maxAge" type="number" value="${esc(hsts.maxAge != null ? hsts.maxAge : HSTS_DEFAULT_MAX_AGE)}" /></div></div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Include subdomains</div></div>${switchHtml('ed-hsts-sub', !!hsts.includeSubdomains, 'Include subdomains', 'proxyHost.tls.hsts.includeSubdomains')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Preload</div><div class="ds">Ask browsers to hard-code this domain as HTTPS-only</div></div>${switchHtml('ed-hsts-preload', !!hsts.preload, 'Preload', 'proxyHost.tls.hsts.preload')}</div>
    </div>
  </div>`;
}
function wireTls() {
  const h = $('#ed-hsts');
  if (h) h.addEventListener('switchchange', () => { $('#ed-hsts-fields').style.display = isOn('ed-hsts') ? '' : 'none'; });
  wireHstsPreload('ed-hsts-preload');
}
// Shared by every editor with an HSTS block. Preload is the one TLS switch that
// outlives the config: submitting to the browser preload list is a months-long
// round trip to undo, and until it clears, a domain that loses HTTPS is
// unreachable rather than merely insecure. So it confirms on the way ON.
function wireHstsPreload(id) {
  const sw = document.getElementById(id);
  if (!sw) return;
  sw.addEventListener('switchchange', async () => {
    if (!isOn(id)) return;
    const ok = await confirmModal({
      title: 'Enable HSTS preload?',
      body: '<p>Preload asks browser vendors to hard-code these domains as HTTPS-only, for every visitor, before the first request.</p>'
        + '<p><b>It is hard to undo.</b> Removal from the preload list is a separate submission and takes months to reach users. Until then, any domain here that stops serving valid HTTPS - including a subdomain, if "include subdomains" is on - is unreachable, not merely insecure.</p>'
        + '<p>Only turn this on once every name is permanently HTTPS.</p>',
      confirmLabel: 'Enable preload',
    });
    if (!ok) sw.setAttribute('aria-checked', 'false');
  });
}
// orig is the tls object as loaded. certificateRef and http2 have no control on
// this form any more (SNI selects the certificate on an HTTP host, and http2 is
// negotiated by ALPN), so both are carried forward verbatim rather than
// rebuilt - a host PUT is a whole-object replacement and would otherwise drop a
// git-authored value on every unrelated save.
function readTls(orig) {
  orig = orig || {};
  const tls = {};
  if (orig.certificateRef) tls.certificateRef = orig.certificateRef;
  if (orig.http2 !== undefined) tls.http2 = orig.http2;
  if (isOn('ed-forcessl')) tls.forceSSL = true;
  if (isOn('ed-hsts')) {
    const hsts = { enabled: true, includeSubdomains: isOn('ed-hsts-sub'), preload: isOn('ed-hsts-preload') };
    const maxAge = hstsMaxAgeFor((orig.hsts || {}).maxAge, $('#ed-hsts-max'));
    if (maxAge != null) hsts.maxAge = maxAge;
    tls.hsts = hsts;
  }
  return Object.keys(tls).length ? tls : null;
}

// Key/value map editor. secret=true masks nothing locally but flags ***-masked
// values so a save cannot silently clobber a real secret.
function makeKVRows(wrap, initial, kPlace, vPlace, secret) {
  function addRow(k, v) {
    const div = document.createElement('div');
    div.className = 'loc-row';
    div.innerHTML = `<input class="field mono kv-k" style="flex:1 1 130px" value="${esc(k == null ? '' : k)}" placeholder="${esc(kPlace || 'key')}" aria-label="key" />
      <input class="field mono kv-v" style="flex:2 1 170px" value="${esc(v == null ? '' : v)}" placeholder="${esc(vPlace || 'value')}" aria-label="value" />
      <button class="icon-btn kv-del" type="button" aria-label="Remove">${ICON.x}</button>`;
    div.querySelector('.kv-del').addEventListener('click', () => div.remove());
    wrap.appendChild(div);
    return div;
  }
  Object.keys(initial || {}).forEach((k) => addRow(k, initial[k]));
  return {
    addRow,
    get() {
      const out = {};
      wrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
        const k = r.querySelector('.kv-k').value.trim();
        if (!k) return;
        out[k] = r.querySelector('.kv-v').value;
      });
      return out;
    },
    masked() {
      let m = false;
      wrap.querySelectorAll(':scope > .loc-row').forEach((r) => { if (secret && r.querySelector('.kv-v').value === '***') m = true; });
      return m;
    },
  };
}

// Security-headers map editor: header name -> value, plus the per-header scope
// selecting which responses it lands on. Shared by the Settings page
// (settings.securityHeaders, the fleet default) and the host editor
// (proxyHost.securityHeaders, which merges over it per key) - the two fields
// have the same shape, so they get the same control.
//
// SERIALIZATION, deliberately not a mirror of what it loaded:
//   - scope "all" is written back as a BARE STRING ("X-Frame-Options": "DENY"),
//     never as {value, scope: "all"}. That is exactly what the Go marshaller
//     emits for an all/empty scope, so a header nobody touched round-trips
//     byte-for-byte and the GitOps YAML diff stays empty. A hand-written
//     {value, scope: "all"} object is therefore NORMALIZED to the bare string on
//     the first save through this editor: the two spellings mean the same thing,
//     the API already renders that header as a bare string, and picking one
//     keeps the diff stable from then on.
//   - a non-default scope is written as {value, scope}.
//   - a value this build does not understand - anything that is neither a string
//     nor a {value, scope} object with a known scope, e.g. a scope added to the
//     API after this UI was built - is NOT edited. Its row is read-only and the
//     loaded value is emitted VERBATIM, so a newer config survives an older UI
//     instead of being flattened into a string or silently dropped.
//   - no rows means get() returns null and the caller omits the field entirely.
//     Both saves are whole-object replacements, so writing {} instead would
//     commit an empty map where the config had none.
const SECURITY_SCOPES = ['all', 'generated-only', 'proxied-only'];
const HEADER_NAME_RE = /^[A-Za-z0-9!#$%&'*+.^_`|~-]+$/;

// stripHeaderListError validates a stripResponseHeaders chip list client-side:
// token syntax and case-insensitive duplicates (the chip input only dedupes
// exact matches). The refused-name policy (hop-by-hop, response-semantic)
// stays server-side only - the API's 400 names the exact header and the reason
// - so the two lists cannot drift.
function stripHeaderListError(names) {
  const seen = {};
  for (const n of names) {
    if (!HEADER_NAME_RE.test(n)) return `"${n}" is not a valid header name.`;
    const lk = n.toLowerCase();
    if (seen[lk]) return `"${n}" is listed more than once (names are case-insensitive).`;
    seen[lk] = true;
  }
  return '';
}

function makeSecurityHeaderRows(wrap, initial) {
  function addRow(name, raw) {
    const isStr = typeof raw === 'string';
    const isObj = !!raw && typeof raw === 'object' && !Array.isArray(raw)
      && typeof raw.value === 'string'
      && (raw.scope == null || raw.scope === '' || SECURITY_SCOPES.indexOf(raw.scope) !== -1);
    const known = raw === undefined || isStr || isObj;
    const value = isStr ? raw : (isObj ? raw.value : '');
    const scope = (isObj && raw.scope) ? raw.scope : 'all';
    const div = document.createElement('div');
    div.className = 'loc-row';
    if (known) {
      div.innerHTML = `<input class="field mono sh-name" data-hint="settings.securityHeaders.name" style="flex:1 1 160px" value="${esc(name || '')}" placeholder="X-Frame-Options" aria-label="Header name" />
        <input class="field mono sh-value" data-hint="settings.securityHeaders.value" style="flex:2 1 190px" value="${esc(value)}" placeholder="DENY" aria-label="Header value" />
        <select class="field mono sh-scope" data-hint="settings.securityHeaders.scope" style="flex:1 1 230px" aria-label="Scope">${enumOptions('headerScope', SECURITY_SCOPES, scope)}</select>
        <button class="icon-btn sh-del" type="button" aria-label="Remove header">${ICON.x}</button>`;
    } else {
      div._raw = raw;
      div.innerHTML = `<input class="field mono sh-name" style="flex:1 1 160px" value="${esc(name || '')}" aria-label="Header name" disabled />
        <input class="field mono" style="flex:2 1 190px" value="${esc(JSON.stringify(raw))}" aria-label="Header value" disabled />
        <span class="hint" style="flex:0 0 150px;margin:0">not editable here - saved as-is</span>
        <button class="icon-btn sh-del" type="button" aria-label="Remove header">${ICON.x}</button>`;
    }
    div.querySelector('.sh-del').addEventListener('click', () => div.remove());
    wrap.appendChild(div);
    return div;
  }
  const init = (initial && typeof initial === 'object') ? initial : {};
  Object.keys(init).forEach((k) => addRow(k, init[k]));
  return {
    addRow,
    get() {
      // Null-prototype: the keys are operator-typed, and a name like __proto__
      // on a plain object would set the prototype instead of a key, silently
      // dropping the header (and, if it were the only one, the whole map).
      // Here it becomes a normal key the server rejects by name.
      const out = Object.create(null);
      wrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
        const name = r.querySelector('.sh-name').value.trim();
        if (!name) return;
        if ('_raw' in r) { out[name] = r._raw; return; }
        const scope = r.querySelector('.sh-scope').value;
        const value = r.querySelector('.sh-value').value;
        out[name] = scope === 'all' ? value : { value: value, scope: scope };
      });
      return Object.keys(out).length ? out : null;
    },
    // Minimal client-side checks - the server validates authoritatively (and its
    // 400 surfaces through toastErr like every other save). These two are worth
    // catching here: a name with a space is an obvious typo the API would only
    // reject after a round trip, and a duplicate name would be silently collapsed
    // by the object build above before the API ever saw it.
    error() {
      const seen = Object.create(null);
      let err = '';
      wrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
        if (err) return;
        const name = r.querySelector('.sh-name').value.trim();
        if (!name) {
          // A nameless row with a value would be silently discarded by get() -
          // surface it instead of losing what the operator typed. (Read-only
          // _raw rows have no .sh-value input and keep their loaded name.)
          const val = r.querySelector('.sh-value');
          if (val && val.value) err = 'a header row has a value but no name: name it or remove the row.';
          return;
        }
        if (!HEADER_NAME_RE.test(name)) {
          err = `"${name}" is not a valid header name: use a token like X-Frame-Options, with no spaces, colons or control characters.`;
          return;
        }
        const lk = name.toLowerCase();
        if (seen[lk]) {
          err = `"${name}" is listed more than once. A header is declared once, at one scope (names are case-insensitive).`;
          return;
        }
        seen[lk] = true;
      });
      return err;
    },
  };
}

// Wire the Save/Delete buttons. `stored` is the object as loaded (or {} for a
// new one) and buildBody(name) returns the object to PUT (common meta is applied
// here) or null to abort after showing its own toast.
//
// stored is NOT optional: this is the shared save for middleware, access lists,
// identity providers, DNS providers, upstream groups, redirects, streams, parked
// hosts and client CAs, and none of those editors renders a labels or tags
// control. A PUT is a whole-object replacement, so without the carry-forward
// every one of those saves silently deleted both keys.
function wireEditor(section, plural, meta, isNew, origName, stored, buildBody) {
  $('#ed-save').addEventListener('click', async () => {
    clearEditorError();
    const nm = isNew ? $('#ed-name').value.trim() : origName;
    if (!nm) { toast('Name required', 'Enter a name.', 'err'); return; }
    const body = buildBody(nm);
    if (!body) return;
    Object.assign(body, metaCarryForward(stored));
    body.name = nm;
    const disp = $('#ed-display'); if (disp && disp.value.trim()) body.displayName = disp.value.trim();
    if (isOn('ed-disabled')) body.disabled = true;
    const btn = $('#ed-save'); btn.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'PUT', body });
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      location.hash = '#/' + section;
    } catch (e) { showSaveError(e, `Could not save this ${meta.singular}`); btn.disabled = false; }
  });
  const del = $('#ed-delete');
  if (del) del.addEventListener('click', async () => {
    if (!confirm(`Delete ${meta.singular} "${origName}"?`)) return;
    del.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(origName), { method: 'DELETE' });
      toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'removed', 'ok', { html: true });
      refreshHeadSha(); clearDirty(); location.hash = '#/' + section;
    } catch (e) { toastErr(e); del.disabled = false; }
  });
}

// ---------- REDIRECT HOST EDITOR ----------
async function redirectEditor(c, name) {
  const meta = SECTION_META.redirects; const isNew = !name;
  const seed = isNew ? takeCloneSeed('redirects') : null;
  const [certsR, objR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/redirect-hosts/' + encodeURIComponent(name)),
  ]);
  const certs = arr(certsR);
  const o = seed ? seed.data : (objR.data || {});
  c.innerHTML = editorHead('redirects', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy', true)}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains" data-hint="redirectHost.domains"></div><div class="hint">Press Enter to add. At least one domain is required.</div></div>
    <div class="card form-section"><p class="section-label">Redirect target</p>
      <div class="inline-fields">
        <div class="field-group"><label>Target scheme</label><select class="field mono" id="ed-tscheme" data-hint="redirectHost.targetScheme">
          ${enumOptions('redirectScheme', ['auto', 'http', 'https'], o.targetScheme || 'auto')}
        </select></div>
        <div class="field-group" style="flex:2"><label>Target domain</label><input class="field mono" id="ed-tdomain" data-hint="redirectHost.targetDomain" value="${esc(o.targetDomain || '')}" placeholder="example.com" /></div>
        <div class="field-group"><label>Status code</label><select class="field mono" id="ed-status" data-hint="redirectHost.statusCode">
          ${enumOptions('redirectStatus', ['301', '302', '307', '308'], String(o.statusCode || 301))}
        </select></div>
      </div>
      <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Preserve path</div><div class="ds">Append the original request path to the target</div></div>${switchHtml('ed-preserve', !!o.preservePath, 'Preserve path', 'redirectHost.preservePath')}</div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, arr(o.domains))}</div></div>` + saveBar('redirects', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...',
    (d) => { $('#ed-cert-auto').innerHTML = certCoverageHtml(certs, d); });
  wireTls();
  wireEditor('redirects', 'redirect-hosts', meta, isNew, name || o.name, o, (nm) => {
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return null; }
    const td = $('#ed-tdomain').value.trim();
    if (!td) { toast('Target required', 'Set the target domain.', 'err'); return null; }
    // absent-stays-absent: both controls always SHOW a value, so sending them
    // unconditionally materialises `targetScheme: auto` / `statusCode: 301` into
    // a file that never carried the key. Send each only when it was already
    // stored, or when the operator picked something other than the default.
    const body = { domains, targetDomain: td };
    const tScheme = $('#ed-tscheme').value;
    if (o.targetScheme || tScheme !== 'auto') body.targetScheme = tScheme;
    const rStatus = parseInt($('#ed-status').value, 10);
    if (!isNaN(rStatus) && (o.statusCode || rStatus !== 301)) body.statusCode = rStatus;
    if (isOn('ed-preserve')) body.preservePath = true;
    const tls = readTls(o.tls); if (tls) body.tls = tls;
    return body;
  });
  wireCloneButton('redirects', o);
}

// ---------- STREAM HOST EDITOR ----------
async function streamEditor(c, name) {
  const meta = SECTION_META.streams; const isNew = !name;
  const seed = isNew ? takeCloneSeed('streams') : null;
  const [certsR, alR, objR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    refList('/api/access-lists', 'access lists'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/stream-hosts/' + encodeURIComponent(name)),
  ]);
  const streamCerts = arr(certsR);
  // A list with basic-auth users cannot be evaluated on a raw stream (there is
  // no request to challenge), so it is offered but disabled rather than
  // accepted-then-rejected by the API.
  const streamAls = arr(alR);
  const o = seed ? seed.data : (objR.data || {});
  const stls = o.tls || {};
  const selAl = arr(o.accessLists);
  // One-line state for the two folded sections (see hostEditor for why).
  const stlsSummary = stls.mode
    ? `${stls.mode}${arr(stls.sniMatch).length ? ', SNI ' + arr(stls.sniMatch).join(', ') : ''}${stls.certificateRef ? ', cert ' + stls.certificateRef : ''}`
    : 'none - the bytes are forwarded blind';
  const streamAlSummary = selAl.length ? selAl.join(', ') : 'none - every client IP is accepted';
  c.innerHTML = editorHead('streams', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy', true)}
    <div class="card form-section"><p class="section-label">Forwarding</p>
      <div class="inline-fields">
        <div class="field-group"><label>Listen port</label><input class="field mono" id="ed-listen" data-hint="streamHost.listenPort" type="number" value="${esc(o.listenPort != null ? o.listenPort : '')}" placeholder="5353" /></div>
        <div class="field-group"><label>Protocol</label><select class="field mono" id="ed-proto" data-hint="streamHost.protocol">
          ${enumOptions('streamProtocol', ['tcp', 'udp', 'both'], o.protocol || 'tcp')}
        </select></div>
      </div>
      <div class="inline-fields" style="margin-top:14px">
        <div class="field-group" style="flex:2"><label>Target host</label><input class="field mono" id="ed-fhost" data-hint="streamHost.target.host" value="${esc((o.target && o.target.host) || '')}" placeholder="10.0.0.5" /></div>
        <div class="field-group"><label>Target port</label><input class="field mono" id="ed-fport" data-hint="streamHost.target.port" type="number" value="${esc(o.target && o.target.port != null ? o.target.port : '')}" placeholder="53" /></div>
      </div>
    </div>
    ${foldHtml('ed-stls-card', 'TLS and SNI', stlsSummary, !!stls.mode, `
      <div class="field-group"><label>Mode</label>
        <select class="field mono" id="ed-tlsmode" data-hint="streamHost.tls.mode">
          ${enumOptions('streamTLSMode', ['', 'passthrough', 'terminate'], stls.mode || '')}
        </select>
        <div class="hint" id="ed-tls-hint">TCP only. Two stream hosts may share a listen port only when every one of them matches on SNI.</div>
      </div>
      <div id="ed-tls-fields" style="display:none">
        <div class="field-group"><label>SNI match</label><div class="chip-input" id="ed-sni" data-hint="streamHost.tls.sniMatch"></div>
          <div class="hint">Server names this host claims, e.g. <span class="mono">db.example.com</span> or <span class="mono">*.example.com</span>. Required when sharing a port; leave empty to take every connection on a port of your own.</div>
        </div>
        <div class="field-group" id="ed-cert-group"><label>Certificate</label>
          <select class="field mono" id="ed-streamcert" data-hint="streamHost.tls.certificateRef">
            <option value="">(select a certificate)</option>
            ${streamCerts.map((ct) => `<option value="${esc(ct.name)}"${stls.certificateRef === ct.name ? ' selected' : ''}>${esc(ct.name)} - ${esc(arr(ct.domains).join(', '))}</option>`).join('')}
          </select>
          <div class="hint">Terminate mode only. Passthrough never decrypts, so it needs no certificate.</div>
        </div>
      </div>
    `)}
    ${foldHtml('ed-stream-al-card', 'Access lists', streamAlSummary, selAl.length > 0, `
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Evaluated on the client IP before any backend is dialled. Only IP rules and geo apply at L4 - a list with basic-auth users cannot issue an HTTP challenge on a raw stream and is unavailable here.</p>
      <div class="check-list" id="ed-stream-al" data-hint="streamHost.accessLists">
        ${streamAls.length ? streamAls.map((a) => {
    const hasBasic = arr(a.basicAuth).length > 0;
    return `<label class="check-item"${hasBasic ? ' style="opacity:.5;cursor:not-allowed" title="This list has basic-auth users, which a raw stream cannot evaluate."' : ''}><input type="checkbox" value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 && !hasBasic ? ' checked' : ''}${hasBasic ? ' disabled' : ''}/>${esc(a.name)}<span class="ci-ty">${hasBasic ? 'basic-auth: n/a at L4' : 'ip / geo'}</span></label>`;
  }).join('') : '<div class="check-empty">No access lists defined yet.</div>'}
      </div>
    `)}
  </div></div>` + saveBar('streams', isNew, meta.addLabel);
  const sniCtl = makeChipInput($('#ed-sni'), arr(stls.sniMatch), 'add server name...');
  // TLS is TCP-only and the certificate applies to terminate alone, so the
  // fields that cannot apply are hidden/disabled rather than accepted and then
  // rejected by the API.
  function refreshStreamTLS() {
    const mode = $('#ed-tlsmode').value;
    const proto = $('#ed-proto').value;
    const tcpOnly = proto === 'tcp';
    $('#ed-tlsmode').disabled = !tcpOnly;
    $('#ed-tls-fields').style.display = mode && tcpOnly ? '' : 'none';
    $('#ed-cert-group').style.display = mode === 'terminate' ? '' : 'none';
    $('#ed-tls-hint').textContent = tcpOnly
      ? 'TCP only. Two stream hosts may share a listen port only when every one of them matches on SNI.'
      : 'TLS/SNI needs protocol tcp: a UDP datagram carries no ClientHello to read.';
  }
  refreshStreamTLS();
  $('#ed-tlsmode').addEventListener('change', refreshStreamTLS);
  $('#ed-proto').addEventListener('change', refreshStreamTLS);
  wireEditor('streams', 'stream-hosts', meta, isNew, name || o.name, o, () => {
    const lp = parseInt($('#ed-listen').value, 10); const fp = parseInt($('#ed-fport').value, 10); const fh = $('#ed-fhost').value.trim();
    if (isNaN(lp)) { toast('Listen port required', 'Enter a listen port.', 'err'); return null; }
    if (!fh || isNaN(fp)) { toast('Target required', 'Set target host and port.', 'err'); return null; }
    const proto = $('#ed-proto').value;
    const body = { listenPort: lp, protocol: proto, target: { host: fh, port: fp } };
    const mode = $('#ed-tlsmode').value;
    if (mode && proto === 'tcp') {
      const tls = { mode };
      const sni = sniCtl.get(); if (sni.length) tls.sniMatch = sni;
      if (mode === 'terminate') {
        const ref = $('#ed-streamcert').value;
        if (!ref) { toast('Certificate required', 'Terminate mode needs a certificate to present.', 'err'); return null; }
        tls.certificateRef = ref;
      }
      body.tls = tls;
    }
    const als = $$('#ed-stream-al input:checked').map((i) => i.value);
    if (als.length) body.accessLists = als;
    return body;
  });
  wireCloneButton('streams', o);
}

// ---------- PARKED HOST EDITOR ----------
async function parkedEditor(c, name) {
  const meta = SECTION_META.parked; const isNew = !name;
  const seed = isNew ? takeCloneSeed('parked') : null;
  const [certsR, objR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/parked-hosts/' + encodeURIComponent(name)),
  ]);
  const certs = arr(certsR);
  const o = seed ? seed.data : (objR.data || {});
  c.innerHTML = editorHead('parked', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy', true)}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains" data-hint="parkedHost.domains"></div><div class="hint">Press Enter to add. At least one domain is required.</div></div>
    <div class="card form-section"><p class="section-label">Response</p>
      <div class="field-group"><label>Status code</label><input class="field mono" id="ed-status" data-hint="parkedHost.statusCode" type="number" value="${esc(o.statusCode != null ? o.statusCode : 404)}" placeholder="404" /><div class="hint">Returned for every request to these domains. Default 404.</div></div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, arr(o.domains))}</div></div>` + saveBar('parked', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...',
    (d) => { $('#ed-cert-auto').innerHTML = certCoverageHtml(certs, d); });
  wireTls();
  wireEditor('parked', 'parked-hosts', meta, isNew, name || o.name, o, () => {
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return null; }
    const body = { domains };
    // absent-stays-absent: the input renders 404 when the object has no status.
    const sc = parseInt($('#ed-status').value, 10);
    if (!isNaN(sc) && (o.statusCode || sc !== 404)) body.statusCode = sc;
    const tls = readTls(o.tls); if (tls) body.tls = tls;
    return body;
  });
  wireCloneButton('parked', o);
}

// DNS-01 providers with a built-in solver (mirrors model.KnownDNSProviders).
// The four token providers authenticate with a single config key, apiToken;
// rfc2136 and acme-dns do not, so `fields` (when present) drives a typed form
// instead of the free-form key/value rows and `required` is per provider.
const DNS_PROVIDERS = [
  { id: 'cloudflare', label: 'Cloudflare', hint: 'config.apiToken: an API token with Zone:DNS:Edit + Zone:Read on the zone.', required: ['apiToken'] },
  { id: 'digitalocean', label: 'DigitalOcean', hint: 'config.apiToken: a personal access token with write scope on domains.', required: ['apiToken'] },
  { id: 'hetzner', label: 'Hetzner DNS', hint: 'config.apiToken: a Hetzner DNS API token (sent as Auth-API-Token).', required: ['apiToken'] },
  { id: 'desec', label: 'deSEC', hint: 'config.apiToken: a deSEC API token (sent as Authorization: Token).', required: ['apiToken'] },
  {
    id: 'rfc2136',
    label: 'RFC 2136 (dynamic update)',
    hint: 'TSIG-signed dynamic DNS UPDATE against any nameserver that accepts one (BIND, Knot, PowerDNS).',
    required: ['server', 'zone', 'tsigKeyName', 'tsigSecret'],
    missing: 'rfc2136 needs server, zone, tsigKeyName and tsigSecret.',
    note: 'Generate the key with <span class="mono">tsig-keygen -a hmac-sha256 gpm-acme</span> on the nameserver and grant it update rights on the zone, then keep the clock in sync - TSIG rejects a signature more than 300 seconds off.',
    fields: [
      { key: 'server', hint: 'dnsProvider.config.server', label: 'Nameserver', placeholder: '192.0.2.53', help: 'Host, <span class="mono">host:port</span>, or <span class="mono">[2001:db8::53]:53</span>. Port defaults to 53.' },
      { key: 'zone', hint: 'dnsProvider.config.zone', label: 'Zone', placeholder: 'example.com', help: 'The zone the UPDATE is addressed to, not the challenge name.' },
      { key: 'tsigKeyName', hint: 'dnsProvider.config.tsigKeyName', label: 'TSIG key name', placeholder: 'gpm-acme', help: 'Exactly the key name configured on the nameserver.' },
      { key: 'tsigSecret', hint: 'dnsProvider.config.tsigSecret', label: 'TSIG secret', placeholder: '${FILE:/run/secrets/tsig_key}', help: 'Base64 secret from <span class="mono">tsig-keygen</span>. Use a <span class="mono">${ENV:...}</span> or <span class="mono">${FILE:...}</span> placeholder so it is not committed.' },
      { key: 'tsigAlgorithm', hint: 'dnsProvider.config.tsigAlgorithm', label: 'TSIG algorithm', select: { group: 'tsigAlgorithm', options: ['hmac-sha1', 'hmac-sha224', 'hmac-sha256', 'hmac-sha384', 'hmac-sha512'], dflt: 'hmac-sha256' }, help: 'Must match the key on the nameserver. <span class="mono">hmac-md5</span> is not offered and is rejected by the API.' },
      { key: 'ttl', hint: 'dnsProvider.config.ttl', label: 'Record TTL (seconds)', placeholder: '60', help: 'TTL of the challenge TXT record, 1 to 86400.' },
      { key: 'transport', hint: 'dnsProvider.config.transport', label: 'Transport', select: { group: 'dnsTransport', options: ['tcp', 'udp'], dflt: 'tcp' }, help: 'UDP is retried over TCP when the reply is truncated.' },
      { key: 'timeout', hint: 'dnsProvider.config.timeout', label: 'Timeout', placeholder: '30s', help: 'Go duration for one exchange, e.g. <span class="mono">30s</span>.' },
    ],
  },
  {
    id: 'acme-dns',
    label: 'acme-dns',
    hint: 'Writes the challenge to a joohoi/acme-dns account; the real zone keeps one permanent CNAME.',
    required: ['baseURL', 'username', 'password', 'subdomain'],
    missing: 'acme-dns needs baseURL, username, password and subdomain.',
    note: 'Add one permanent CNAME per certificate domain in your real zone: <span class="mono">_acme-challenge.example.com. CNAME &lt;subdomain&gt;.acme-dns.example.com.</span> A wildcard validates at the same name, so it needs no second record. gpm warns in the log if the delegation is missing but still attempts issuance.',
    fields: [
      { key: 'baseURL', hint: 'dnsProvider.config.baseURL', label: 'acme-dns base URL', placeholder: 'https://acme-dns.example.com', help: 'API root of your acme-dns server. Must be http or https.' },
      { key: 'username', hint: 'dnsProvider.config.username', label: 'Username', placeholder: 'c0f8ba55-0000-4000-8000-000000000001', help: 'The <span class="mono">username</span> from the acme-dns <span class="mono">/register</span> response.' },
      { key: 'password', hint: 'dnsProvider.config.password', label: 'Password', placeholder: '${FILE:/run/secrets/acme_dns_password}', help: 'The <span class="mono">password</span> from <span class="mono">/register</span>. Use a <span class="mono">${ENV:...}</span> or <span class="mono">${FILE:...}</span> placeholder.' },
      { key: 'subdomain', hint: 'dnsProvider.config.subdomain', label: 'Subdomain', placeholder: 'd420c923-0000-4000-8000-000000000002', help: 'The <span class="mono">subdomain</span> from <span class="mono">/register</span>; this is what the CNAME points at.' },
    ],
  },
];
function dnsProvider(id) { return DNS_PROVIDERS.find((p) => p.id === id) || DNS_PROVIDERS[0]; }

// ---------- DNS PROVIDER EDITOR ----------
async function dnsEditor(c, name) {
  const meta = SECTION_META.dns; const isNew = !name;
  const seed = isNew ? takeCloneSeed('dns') : null;
  const o = seed ? seed.data : (isNew ? { provider: 'cloudflare', config: { apiToken: '${ENV:CF_API_TOKEN}' } } : ((await api('/api/dns-providers/' + encodeURIComponent(name))).data || {}));
  const current = o.provider || 'cloudflare';
  c.innerHTML = editorHead('dns', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    <div class="card form-section"><p class="section-label">Provider</p>
      <div class="field-group"><label>Provider</label>
        <select class="field mono" id="ed-provider" data-hint="dnsProvider.provider" data-path="provider">
          ${DNS_PROVIDERS.map((p) => `<option value="${esc(p.id)}"${current === p.id ? ' selected' : ''}>${esc(p.label)}</option>`).join('')}
        </select>
        <div class="hint" id="ed-provider-hint"></div>
      </div>
    </div>
    <div class="card form-section"><p class="section-label">Credentials</p>
      <div id="ed-typed"></div>
      <div id="ed-kv">
        <div id="ed-config" data-hint="dnsProvider.config"></div>
        <button class="btn ghost sm" id="ed-addcfg" type="button" style="margin-top:6px">${ICON.plus}Add credential</button>
      </div>
      <div class="hint" id="ed-provider-note" style="margin-top:8px"></div>
      <div class="hint" style="margin-top:8px">Use a placeholder like <span class="mono">\${ENV:CF_API_TOKEN}</span> or <span class="mono">\${FILE:/run/secrets/token}</span> so no secret is committed. A masked secret reads <span class="mono">***</span>.</div>
    </div>
  </div></div>` + saveBar('dns', isNew, meta.addLabel);

  const cfgCtl = makeKVRows($('#ed-config'), o.config || {}, 'key (e.g. apiToken)', '${ENV:DNS_API_TOKEN}', true);
  $('#ed-addcfg').addEventListener('click', () => cfgCtl.addRow('', ''));

  // Typed form for the providers whose credentials are not one token. Rendered
  // from the loaded config so a switch away and back does not lose what was
  // typed; the KV rows stay the control for the four token providers.
  let typedValues = Object.assign({}, o.config || {});
  function renderTyped(p) {
    const wrap = $('#ed-typed');
    if (!p.fields) { wrap.innerHTML = ''; return; }
    wrap.innerHTML = p.fields.map((f) => {
      const v = typedValues[f.key] == null ? '' : String(typedValues[f.key]);
      if (f.select) {
        return `<div class="field-group"><label>${esc(f.label)}</label>
          <select class="field mono dnsf" data-key="${esc(f.key)}" data-hint="${esc(f.hint)}">${enumOptions(f.select.group, f.select.options, v || f.select.dflt)}</select>
          <div class="hint">${f.help}</div></div>`;
      }
      return `<div class="field-group"><label>${esc(f.label)}</label>
        <input class="field mono dnsf" data-key="${esc(f.key)}" data-hint="${esc(f.hint)}" value="${esc(v)}" placeholder="${esc(f.placeholder || '')}" />
        <div class="hint">${f.help}</div></div>`;
    }).join('');
    wrap.querySelectorAll('.dnsf').forEach((el) => {
      el.addEventListener('input', () => { typedValues[el.dataset.key] = el.value; });
      el.addEventListener('change', () => { typedValues[el.dataset.key] = el.value; });
    });
  }
  function syncProvider(reseed) {
    const p = dnsProvider($('#ed-provider').value);
    $('#ed-provider-hint').textContent = p.hint;
    $('#ed-provider-note').innerHTML = p.note || '';
    $('#ed-provider-note').hidden = !p.note;
    $('#ed-kv').hidden = !!p.fields;
    // Reseeding on a provider CHANGE stops the operator staring at an apiToken
    // row that no longer applies; the initial render keeps whatever was loaded.
    if (reseed && p.fields) {
      const keep = {};
      p.fields.forEach((f) => { if (typedValues[f.key] != null) keep[f.key] = typedValues[f.key]; });
      typedValues = keep;
    }
    renderTyped(p);
  }
  syncProvider(false);
  $('#ed-provider').addEventListener('change', () => syncProvider(true));

  wireEditor('dns', 'dns-providers', meta, isNew, name || o.name, o, () => {
    const provId = $('#ed-provider').value.trim();
    if (!provId) { toast('Provider required', 'Select a provider.', 'err'); return null; }
    const p = dnsProvider(provId);
    let cfg;
    if (p.fields) {
      cfg = {};
      $$('#ed-typed .dnsf').forEach((el) => {
        const v = el.value.trim();
        // Optional keys are OMITTED rather than sent empty: an empty value is
        // noise in the committed YAML and the API only validates non-empty ones.
        if (v) cfg[el.dataset.key] = v;
      });
      if (Object.values(cfg).some((v) => v === '***')) {
        toast('Secret masked', 'A credential is masked as ***. Replace it with a real value or a ${ENV:...} placeholder before saving.', 'err'); return null;
      }
      if (cfg.ttl != null) {
        const n = Number(cfg.ttl);
        if (!Number.isInteger(n) || n < 1 || n > 86400) { toast('Invalid TTL', 'config.ttl must be a whole number of seconds between 1 and 86400.', 'err'); return null; }
      }
      if (cfg.timeout && !GO_DURATION_RE.test(cfg.timeout)) {
        toast('Invalid timeout', 'config.timeout must be a positive Go duration such as 30s.', 'err'); return null;
      }
      if (cfg.baseURL && !/^https?:\/\//.test(cfg.baseURL)) {
        toast('Invalid base URL', 'config.baseURL must be an http or https URL.', 'err'); return null;
      }
    } else {
      if (cfgCtl.masked()) { toast('Secret masked', 'A credential is masked as ***. Replace it with a real value or a ${ENV:...} placeholder before saving.', 'err'); return null; }
      cfg = cfgCtl.get();
    }
    const missing = arr(p.required).filter((k) => !cfg[k]);
    if (missing.length) {
      if (p.missing) toast('Missing credentials', p.missing, 'err');
      else toast('API token required', 'Every DNS provider needs a config.apiToken credential.', 'err');
      return null;
    }
    const body = { provider: provId };
    if (Object.keys(cfg).length) body.config = cfg;
    return body;
  });
  wireCloneButton('dns', o);
}

// ---------- ACCESS LIST EDITOR ----------
// Basic auth left this tier: it is an auth-middleware mode now, so the editor
// no longer OFFERS basicAuth or satisfyAny (the OR flag that only existed
// because two unrelated kinds of check shared one object). A list that still
// carries them gets a read-only deprecation card and a Migrate action, and its
// stored values round-trip untouched on every unrelated save.
async function accessEditor(c, name) {
  const meta = SECTION_META.access; const isNew = !name;
  const seed = isNew ? takeCloneSeed('access') : null;
  const [objR] = await Promise.all([
    isNew ? Promise.resolve({ data: {} }) : api('/api/access-lists/' + encodeURIComponent(name)),
    loadCapabilities(),
  ]);
  const o = seed ? seed.data : (objR.data || {});
  const geoAvailable = hasCapability('geoip.dbLoaded');
  const geoReason = 'GeoIP database not loaded (set GPM_GEOIP_DB) - geo rules are unavailable.';
  const geo = o.geo || {};
  const listName = name || o.name || '';
  const legacyUsers = arr(o.basicAuth);
  const hasLegacy = legacyUsers.length > 0;
  const legacyCard = hasLegacy ? `<div class="ro-banner warn" id="al-legacy">
    <b>Legacy basic auth.</b> This access list has ${legacyUsers.length} user${legacyUsers.length === 1 ? '' : 's'}
    (<span class="mono">${esc(legacyUsers.map((u) => u.username).join(', '))}</span>). Basic auth in an access list is
    deprecated and will be removed in gpm v2.
    ${o.satisfyAny ? '<br /><b>satisfyAny is set:</b> an allowed IP currently passes without a password.' : ''}
    <div style="margin-top:8px;display:flex;gap:10px;align-items:center">
      <button class="btn sm al-migrate" id="al-migrate" type="button">Migrate</button>
      <a class="hint" href="https://github.com/Rake-Pro/go-proxy-manager/blob/main/docs/how-to/migrate-basic-auth.md" target="_blank" rel="noopener">Learn more</a>
    </div>
  </div>` : '';

  c.innerHTML = editorHead('access', meta, isNew, name) + legacyCard + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    <div class="card form-section"><p class="section-label">Policy</p>
      <div class="field-group"><label>Default action</label><select class="field mono" id="ed-default" data-hint="accessList.defaultAction" data-path="defaultAction">
        ${enumOptions('ruleAction', ['deny', 'allow'], o.defaultAction || 'deny')}
      </select></div>
    </div>
    <div class="card form-section"><p class="section-label">IP rules</p><div id="ed-rules" data-hint="accessList.rules" data-path="rules"></div>
      <button class="btn ghost sm" id="ed-addrule" type="button" style="margin-top:6px">${ICON.plus}Add rule</button>
      <div class="hint" style="margin-top:6px">Evaluated top-down, first match wins. Match a literal CIDR/IP, or a named source declared below. Paths (comma-separated, each must start with "/") narrow a rule to exact request paths; methods (comma-separated, upper-cased) only apply alongside paths and default to GET, HEAD when left blank.</div>
      <div class="hint" style="margin-top:6px">For username/password gating, create an auth middleware in Basic mode under <a href="#/middleware/_new">Middleware</a>.</div>
    </div>
    <div class="card form-section"><p class="section-label">Sources</p><div id="ed-sources" data-hint="accessList.sources" data-path="sources"></div>
      <button class="btn ghost sm" id="ed-addsource" type="button" style="margin-top:6px">${ICON.plus}Add source</button>
      <div class="hint" style="margin-top:6px">Remote IP feeds a rule above can reference by name. URL must be https. Interval blank = 24h (below 1h refused); max entries blank = 10000.</div>
    </div>
    <div class="card form-section" id="ed-src-status-card" style="display:none"><p class="section-label">Source sync status</p>
      <div id="ed-src-status"></div>
      <button class="btn ghost sm" id="ed-src-reconcile" type="button" style="margin-top:6px">Reconcile now</button>
    </div>
    <div class="card form-section" id="ed-geo-card"><p class="section-label">Geo rules</p>
      ${geoAvailable ? '' : `<div class="field-group"><div class="hint warn" id="ed-geo-hint">${esc(geoReason)}</div></div>`}
      <div class="field-group"><label>Country allow</label>
        <div class="chip-input" id="ed-geo-allow" data-hint="accessList.geo.countryAllow"></div>
        <div class="hint">ISO-3166-1 alpha-2 codes (e.g. US). Non-empty allow list takes priority over deny.</div>
      </div>
      <div class="field-group"><label>Country deny</label><div class="chip-input" id="ed-geo-deny" data-hint="accessList.geo.countryDeny"></div></div>
      <div class="field-group"><label>On unknown country</label><select class="field mono" id="ed-geo-unknown" data-hint="accessList.geo.onUnknown">
        ${enumOptions('geoUnknown', ['', 'allow', 'deny'], geo.onUnknown || '')}
      </select><div class="hint">Applied to an IP with no country in the database (private/reserved ranges, DB misses).</div></div>
    </div>
  </div></div>` + saveBar('access', isNew, meta.addLabel);
  const rulesWrap = $('#ed-rules');
  const sourcesWrap = $('#ed-sources');

  // Sources this list declares, read live off the DOM so a rule's source
  // dropdown always reflects rows added/renamed/removed in this same edit.
  function currentSourceNames() {
    const names = [];
    sourcesWrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
      const n = r.querySelector('.src-name').value.trim();
      if (n && names.indexOf(n) === -1) names.push(n);
    });
    return names;
  }
  // Rebuilds one rule row's source <select>, keeping its current selection even
  // if that name is not (or not yet) a declared source - same round-trip
  // guarantee as the middleware editor's RL_WINDOWS/RL_BLOCKS presets, so a
  // rule referencing a source authored in git before this UI existed still
  // shows its real value instead of silently resetting to blank.
  function populateSourceSelect(sel, selected) {
    const cur = selected != null ? selected : sel.value;
    const names = currentSourceNames();
    if (cur && names.indexOf(cur) === -1) names.push(cur);
    sel.innerHTML = `<option value="">select source...</option>` +
      names.map((n) => `<option value="${esc(n)}"${n === cur ? ' selected' : ''}>${esc(n)}</option>`).join('');
  }
  function refreshSourceSelects() {
    rulesWrap.querySelectorAll('.rule-source').forEach((sel) => populateSourceSelect(sel, sel.value));
  }

  function ruleRow(r) {
    r = r || {}; const d = document.createElement('div'); d.className = 'loc-row';
    const isSource = !!r.source;
    d.innerHTML = `<select class="field mono rule-action" data-hint="accessList.rules.action" style="flex:0 0 110px" aria-label="Action">${enumOptions('ruleAction', ['allow', 'deny'], r.action || 'allow')}</select>
      <select class="field mono rule-match" data-hint="accessList.rules.match" style="flex:0 1 170px" aria-label="Match">${enumOptions('ruleMatch', ['cidr', 'source'], isSource ? 'source' : 'cidr')}</select>
      <input class="field mono rule-cidr" data-hint="accessList.rules.cidr" style="flex:1 1 150px;${isSource ? 'display:none' : ''}" value="${esc(r.cidr || '')}" placeholder="10.0.0.0/8" aria-label="CIDR" />
      <select class="field mono rule-source" data-hint="accessList.rules.source" style="flex:1 1 150px;${isSource ? '' : 'display:none'}" aria-label="Source"></select>
      <input class="field mono rule-paths" data-hint="accessList.rules.paths" style="flex:1 1 170px" value="${esc(arr(r.paths).join(', '))}" placeholder="/health, /status (optional)" aria-label="Paths" />
      <input class="field mono rule-methods" data-hint="accessList.rules.methods" style="flex:1 1 130px" value="${esc(arr(r.methods).join(', '))}" placeholder="GET, HEAD (optional)" aria-label="Methods" />
      <button class="icon-btn rule-del" type="button" aria-label="Remove">${ICON.x}</button>`;
    const matchSel = d.querySelector('.rule-match');
    const cidrInput = d.querySelector('.rule-cidr');
    const sourceSel = d.querySelector('.rule-source');
    matchSel.addEventListener('change', () => {
      const src = matchSel.value === 'source';
      cidrInput.style.display = src ? 'none' : '';
      sourceSel.style.display = src ? '' : 'none';
    });
    d.querySelector('.rule-del').addEventListener('click', () => d.remove());
    rulesWrap.appendChild(d);
    populateSourceSelect(sourceSel, r.source || '');
  }
  arr(o.rules).forEach(ruleRow);
  $('#ed-addrule').addEventListener('click', () => ruleRow({ action: 'allow' }));

  function sourceRow(s) {
    s = s || {}; const d = document.createElement('div'); d.className = 'loc-row';
    d.innerHTML = `<input class="field mono src-name" data-hint="accessList.sources.name" style="flex:1 1 130px" value="${esc(s.name || '')}" placeholder="name" aria-label="Source name" />
      <input class="field mono src-url" data-hint="accessList.sources.url" style="flex:2 1 230px" value="${esc(s.url || '')}" placeholder="https://example.com/ips.txt" aria-label="Source URL" />
      <input class="field mono src-interval" data-hint="accessList.sources.interval" style="flex:0 1 90px" value="${esc(s.interval || '')}" placeholder="24h" aria-label="Interval" />
      <input class="field mono src-maxentries" data-hint="accessList.sources.maxEntries" type="number" min="0" style="flex:0 1 110px" value="${s.maxEntries ? esc(s.maxEntries) : ''}" placeholder="10000" aria-label="Max entries" />
      <button class="icon-btn src-del" type="button" aria-label="Remove">${ICON.x}</button>`;
    d.querySelector('.src-del').addEventListener('click', () => { d.remove(); refreshSourceSelects(); });
    d.querySelector('.src-name').addEventListener('input', refreshSourceSelects);
    sourcesWrap.appendChild(d);
  }
  arr(o.sources).forEach(sourceRow);
  $('#ed-addsource').addEventListener('click', () => sourceRow({}));

  // geo rules - country allow/deny + on-unknown, gated on the GeoIP DB being
  // loaded (GET /api/capabilities). Disabled controls stay visible with a
  // tooltip/inline note rather than being hidden; the server still enforces
  // this independently at write time.
  const geoAllowCtl = makeChipInput($('#ed-geo-allow'), arr(geo.countryAllow), 'add country code...');
  const geoDenyCtl = makeChipInput($('#ed-geo-deny'), arr(geo.countryDeny), 'add country code...');
  gateControl($('#ed-geo-allow'), geoAvailable, geoReason);
  gateControl($('#ed-geo-deny'), geoAvailable, geoReason);
  gateControl($('#ed-geo-unknown'), geoAvailable, geoReason);

  // Access-list source sync status + manual reconcile. GET /api/access-list-sources/status
  // reports every list's sources at once, so filter to this one by name. Shown
  // only once there is something to say: this list declares sources, or the
  // (possibly cross-list) status response already names one for it.
  async function renderAccessSourceStatus() {
    const card = $('#ed-src-status-card'); const el = $('#ed-src-status');
    if (!card || !el) return;
    if (!listName) { card.style.display = 'none'; return; }
    try {
      const st = (await api('/api/access-list-sources/status')).data || {};
      const mine = arr(st.sources).filter((s) => s.list === listName);
      if (!arr(o.sources).length && !mine.length) { card.style.display = 'none'; return; }
      card.style.display = '';
      el.innerHTML = mine.length ? `<div class="check-list">${mine.map((s) => `
        <div class="check-item" style="cursor:default"><span class="mono">${esc(s.name)}</span>
        <span class="muted" style="font-size:11px">${s.fetchedAt ? esc(fmtTime(s.fetchedAt)) + ', ' + (s.entryCount || 0) + ' entries' : 'never fetched'}</span>
        ${s.lastError ? `<span class="muted" style="font-size:11px;color:var(--warn)">${esc(s.lastError)}</span>` : ''}</div>`).join('')}</div>`
        : '<p class="hint" style="margin:0">No sources declared yet for this list.</p>';
    } catch (e) {
      card.style.display = arr(o.sources).length ? '' : 'none';
      el.innerHTML = `<p class="hint" style="margin:0">${e && e.status === 501 ? 'Access-list source sync is not wired in this deployment.' : esc('Status unavailable: ' + (e.message || e))}</p>`;
    }
  }
  if (!isNew) renderAccessSourceStatus();
  else $('#ed-src-status-card').style.display = 'none';

  $('#ed-src-reconcile').addEventListener('click', async () => {
    const btn = $('#ed-src-reconcile'); btn.disabled = true;
    try {
      await api('/api/access-list-sources/reconcile', { method: 'POST' });
      toast('Reconciled', 'Access-list sources are back in sync.', 'ok');
    } catch (e) { toastErr(e); }
    await renderAccessSourceStatus();
    btn.disabled = false;
  });

  // Migrate: always two steps. The plan is rendered in a confirmation dialog so
  // the operator reads what will NOT be carried over before one commit is made.
  const migrateBtn = $('#al-migrate');
  if (migrateBtn) migrateBtn.addEventListener('click', () => migrateBasicAuth(listName, migrateBtn));

  wireEditor('access', 'access-lists', meta, isNew, name || o.name, o, () => {
    const body = {};
    // Deprecated and no longer offered, but a list that still carries them is
    // still gated by them: dropping either on an unrelated save would silently
    // remove a password gate. Round-tripped verbatim until Migrate clears them.
    if (legacyUsers.length) body.basicAuth = legacyUsers;
    if (o.satisfyAny) body.satisfyAny = true;
    // absent-stays-absent: the select renders "deny" (the server-side default)
    // for a list that never set the key, so only send it when it was stored or
    // the operator chose the other value.
    const defAction = $('#ed-default').value;
    if (o.defaultAction || defAction !== 'deny') body.defaultAction = defAction;
    const rules = []; let ruleErr = '';
    clearRowErrors(rulesWrap);
    rulesWrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
      if (ruleErr) return;
      const action = r.querySelector('.rule-action').value;
      const isSource = r.querySelector('.rule-match').value === 'source';
      const rule = { action };
      const paths = r.querySelector('.rule-paths').value.split(',').map((p) => p.trim()).filter(Boolean);
      const methodsRaw = r.querySelector('.rule-methods').value.trim();
      // Same rule as the location rows: a row with a path or a method typed
      // into it is half-filled work, not an empty row, so it is reported
      // instead of silently dropped on the way to the API.
      const halfFilled = paths.length > 0 || !!methodsRaw;
      if (isSource) {
        const src = r.querySelector('.rule-source').value.trim();
        if (!src) {
          if (halfFilled) ruleErr = markRowError(r, 'This rule needs a source. Pick one, or remove the row.');
          return;
        }
        rule.source = src;
      } else {
        const cidr = r.querySelector('.rule-cidr').value.trim();
        if (!cidr) {
          if (halfFilled) ruleErr = markRowError(r, 'This rule needs a CIDR or IP address. Fill it in, or remove the row.');
          return;
        }
        rule.cidr = cidr;
      }
      if (paths.length) {
        const bad = paths.find((p) => !p.startsWith('/'));
        if (bad) { ruleErr = markRowError(r, `Rule path "${bad}" must start with "/".`); return; }
        rule.paths = paths;
      }
      const methods = methodsRaw.split(',').map((m) => m.trim().toUpperCase()).filter(Boolean);
      if (methods.length) {
        if (!paths.length) { ruleErr = markRowError(r, 'Methods only apply alongside paths - add a path, or clear the methods.'); return; }
        rule.methods = methods;
      }
      rules.push(rule);
    });
    if (ruleErr) { toast('Rule invalid', ruleErr, 'err'); return null; }
    if (rules.length) body.rules = rules;
    const sources = []; let srcErr = '';
    clearRowErrors(sourcesWrap);
    sourcesWrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
      if (srcErr) return;
      const nm = r.querySelector('.src-name').value.trim();
      const url = r.querySelector('.src-url').value.trim();
      const interval = r.querySelector('.src-interval').value.trim();
      const maxRaw = r.querySelector('.src-maxentries').value.trim();
      if (!nm && !url && !interval && !maxRaw) return; // fully empty row, drop silently
      if (!nm) { srcErr = markRowError(r, 'Every source needs a name.'); return; }
      if (!/^https:\/\//.test(url)) { srcErr = markRowError(r, `Source "${nm}" needs an https:// URL.`); return; }
      const src = { name: nm, url };
      if (interval) src.interval = interval;
      if (maxRaw) {
        const n = parseInt(maxRaw, 10);
        if (!Number.isFinite(n) || n < 0) { srcErr = markRowError(r, `Source "${nm}": max entries must be a non-negative number.`); return; }
        if (n) src.maxEntries = n;
      }
      sources.push(src);
    });
    if (srcErr) { toast('Source invalid', srcErr, 'err'); return null; }
    if (sources.length) body.sources = sources;
    const geoAllow = geoAllowCtl.get(); const geoDeny = geoDenyCtl.get(); const onUnknown = $('#ed-geo-unknown').value;
    const geoBody = {};
    if (geoAllow.length) geoBody.countryAllow = geoAllow;
    if (geoDeny.length) geoBody.countryDeny = geoDeny;
    if (onUnknown) geoBody.onUnknown = onUnknown;
    if (Object.keys(geoBody).length) body.geo = geoBody;
    return body;
  });
  wireCloneButton('access', o);
}

// POST /api/access-lists/{name}/migrate-basic-auth, plan then apply. One list at
// a time is deliberate: each migration has its own allowFrom and its own
// warnings about rules that will NOT be carried over, and both need reading.
async function migrateBasicAuth(listName, btn) {
  btn.disabled = true;
  let plan;
  try {
    plan = (await api('/api/access-lists/' + encodeURIComponent(listName) + '/migrate-basic-auth?plan=1', { method: 'POST' })).data || {};
  } catch (e) {
    migrateError(e);
    btn.disabled = false;
    return;
  }
  const attach = arr(plan.attachTo).map((a) => esc(a.name + (a.path ? ' ' + a.path : '')));
  const body = `<p>Create auth middleware <b>${esc(plan.middleware)}</b> with <b>${arr(plan.users).length}</b> users (<span class="mono">${esc(arr(plan.users).join(', '))}</span>).</p>`
    + (attach.length
      ? `<p>Attach it to:</p><ul>${attach.map((a) => `<li><span class="mono">${a}</span></li>`).join('')}</ul>`
      : '<p>Nothing references this list yet, so nothing is attached.</p>')
    + (arr(plan.allowFrom).length
      ? `<p>Exempt from the password (copied from <span class="mono">satisfyAny</span>): <span class="mono">${esc(arr(plan.allowFrom).join(', '))}</span>.</p>`
      : '<p>No network is exempted - the list required both the IP and the password.</p>')
    + arr(plan.warnings).map((w) => `<p class="warn-text">${esc(w)}</p>`).join('')
    + `<p>Clear <span class="mono">basicAuth</span> and <span class="mono">satisfyAny</span> from <b>${esc(plan.accessList || listName)}</b>.</p>`
    + '<p class="muted">This is one commit and appears once in History.</p>';
  const ok = await confirmModal({ title: 'Migrate basic auth to a middleware?', body, confirmLabel: 'Migrate', danger: false });
  if (!ok) { btn.disabled = false; return; }
  try {
    const r = await api('/api/access-lists/' + encodeURIComponent(listName) + '/migrate-basic-auth', { method: 'POST' });
    const mw = (r.data && r.data.middleware) || plan.middleware;
    toast('Migrated', `Migrated to middleware ${mw}.` + (shortSha(r.commit) ? ' committed ' + shortSha(r.commit) : ''), 'ok');
    refreshHeadSha();
    route();
  } catch (e) {
    migrateError(e);
    btn.disabled = false;
  }
}
function migrateError(e) {
  if (e && e.status === 404) { toast('Not found', 'Access list not found - reload the page.', 'err'); return; }
  toastErr(e);
}

// ---------- IDENTITY PROVIDER EDITOR (polymorphic) ----------
async function idpEditor(c, name) {
  const meta = SECTION_META.identity; const isNew = !name;
  const seed = isNew ? takeCloneSeed('identity') : null;
  const o = seed ? seed.data : (isNew ? { type: 'oidc' } : ((await api('/api/identity-providers/' + encodeURIComponent(name))).data || {}));
  const type = o.type || 'oidc';
  const oidc = o.oidc || {}; const fa = o.forwardAuth || {}; const ar = o.authRequest || {}; const rm = o.roleMapping || {};
  c.innerHTML = editorHead('identity', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    <div class="card form-section"><p class="section-label">Type</p>
      <div class="field-group"><label>Provider type</label><select class="field mono" id="ed-type" data-hint="identityProvider.type">
        ${enumOptions('idpType', ['oidc', 'forward-auth', 'auth-request'], type)}
      </select></div>
    </div>

    <div class="card form-section ed-sub" data-type="oidc" style="${type === 'oidc' ? '' : 'display:none'}"><p class="section-label">OIDC</p>
      <div class="field-group"><label>Issuer URL</label><input class="field mono" id="oidc-issuer" data-hint="identityProvider.oidc.issuerURL" value="${esc(oidc.issuerURL || '')}" placeholder="https://idp.example.com/application/o/app/" /></div>
      <div class="field-group"><label>Client ID</label><input class="field mono" id="oidc-clientid" data-hint="identityProvider.oidc.clientID" value="${esc(oidc.clientID || '')}" placeholder="client-id" /></div>
      <div class="field-group"><label>Client secret</label><input class="field mono" id="oidc-secret" data-hint="identityProvider.oidc.clientSecret" value="${esc(oidc.clientSecret || '')}" placeholder="\${ENV:OIDC_CLIENT_SECRET}" /><div class="hint">Use a <span class="mono">\${ENV:...}</span> placeholder. A masked secret reads <span class="mono">***</span>; replace it to change.</div></div>
      <div class="field-group"><label>Scopes</label><div class="chip-input" id="oidc-scopes" data-hint="identityProvider.oidc.scopes"></div></div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Use PKCE</div><div class="ds">Recommended; public clients can run with no secret</div></div>${switchHtml('oidc-pkce', oidc.usePKCE !== false, 'Use PKCE', 'identityProvider.oidc.usePKCE')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Require verified email</div></div>${switchHtml('oidc-verify', !!oidc.requireVerifiedEmail, 'Require verified email', 'identityProvider.oidc.requireVerifiedEmail')}</div>
    </div>

    <div class="card form-section ed-sub" data-type="forward-auth" style="${type === 'forward-auth' ? '' : 'display:none'}"><p class="section-label">Forward-auth</p>
      <div class="field-group"><label>Trusted proxies (CIDRs)</label><div class="chip-input" id="fa-trusted" data-hint="identityProvider.forwardAuth.trustedProxies"></div><div class="hint">Which peers may assert identity headers such as <span class="mono">Remote-User</span>. Required. This does not affect the client IP - see <a href="#/settings/general">Settings</a> -&gt; Client IP.</div></div>
      <div class="inline-fields">
        <div class="field-group"><label>User header</label><input class="field mono" id="fa-user" data-hint="identityProvider.forwardAuth.userHeader" value="${esc(fa.userHeader || '')}" placeholder="X-authentik-username" /></div>
        <div class="field-group"><label>Email header</label><input class="field mono" id="fa-email" data-hint="identityProvider.forwardAuth.emailHeader" value="${esc(fa.emailHeader || '')}" placeholder="X-authentik-email" /></div>
      </div>
      <div class="inline-fields">
        <div class="field-group"><label>Name header</label><input class="field mono" id="fa-name" data-hint="identityProvider.forwardAuth.nameHeader" value="${esc(fa.nameHeader || '')}" placeholder="X-authentik-name" /></div>
        <div class="field-group"><label>Groups header</label><input class="field mono" id="fa-groups" data-hint="identityProvider.forwardAuth.groupsHeader" value="${esc(fa.groupsHeader || '')}" placeholder="X-authentik-groups" /></div>
      </div>
      <div class="inline-fields">
        <div class="field-group"><label>Groups delimiter</label><input class="field mono" id="fa-delim" data-hint="identityProvider.forwardAuth.groupsDelimiter" value="${esc(fa.groupsDelimiter || '')}" placeholder="," /></div>
        <div class="field-group"><label>AMR header</label><input class="field mono" id="fa-amr" data-hint="identityProvider.forwardAuth.amrHeader" value="${esc(fa.amrHeader || '')}" placeholder="X-authentik-auth-method" /></div>
      </div>
    </div>

    <div class="card form-section ed-sub" data-type="auth-request" style="${type === 'auth-request' ? '' : 'display:none'}"><p class="section-label">Auth-request</p>
      <div class="field-group"><label>Outpost URL</label><input class="field mono" id="ar-outpost" data-hint="identityProvider.authRequest.outpostURL" value="${esc(ar.outpostURL || '')}" placeholder="http://auth-outpost:9000" /></div>
      <div class="inline-fields">
        <div class="field-group"><label>Path prefix</label><input class="field mono" id="ar-prefix" data-hint="identityProvider.authRequest.pathPrefix" value="${esc(ar.pathPrefix || '')}" placeholder="/outpost.goauthentik.io" /></div>
        <div class="field-group"><label>Auth path</label><input class="field mono" id="ar-authpath" data-hint="identityProvider.authRequest.authPath" value="${esc(ar.authPath || '')}" placeholder="/outpost.goauthentik.io/auth/nginx" /></div>
      </div>
      <div class="field-group"><label>Copy headers</label><div class="chip-input" id="ar-copy" data-hint="identityProvider.authRequest.copyHeaders"></div></div>
    </div>
  </div><div class="stack">
    <div class="card form-section"><p class="section-label">Role mapping</p>
      <div class="field-group"><label>Groups claim</label><input class="field mono" id="rm-claim" data-hint="identityProvider.roleMapping.groupsClaim" value="${esc(rm.groupsClaim || '')}" placeholder="groups" /></div>
      <div class="field-group"><label>Admin groups</label><div class="chip-input" id="rm-admin" data-hint="identityProvider.roleMapping.adminGroups"></div></div>
      <div class="field-group"><label>User groups</label><div class="chip-input" id="rm-user" data-hint="identityProvider.roleMapping.userGroups"></div></div>
      <div class="field-group"><label>Default role</label><select class="field mono" id="rm-default" data-hint="identityProvider.roleMapping.defaultRole">
        ${enumOptions('role', ['', 'user', 'admin'], rm.defaultRole || '')}
      </select></div>
    </div>
  </div></div>` + saveBar('identity', isNew, meta.addLabel);
  const scopesCtl = makeChipInput($('#oidc-scopes'), arr(oidc.scopes), 'add scope...');
  const trustedCtl = makeChipInput($('#fa-trusted'), arr(fa.trustedProxies), 'add CIDR...');
  const copyCtl = makeChipInput($('#ar-copy'), arr(ar.copyHeaders), 'add header...');
  const adminCtl = makeChipInput($('#rm-admin'), arr(rm.adminGroups), 'add group...');
  const userCtl = makeChipInput($('#rm-user'), arr(rm.userGroups), 'add group...');
  $('#ed-type').addEventListener('change', () => { const t = $('#ed-type').value; $$('.ed-sub').forEach((el) => { el.style.display = el.dataset.type === t ? '' : 'none'; }); });
  wireEditor('identity', 'identity-providers', meta, isNew, name || o.name, o, () => {
    const t = $('#ed-type').value; const body = { type: t };
    if (t === 'oidc') {
      const issuer = $('#oidc-issuer').value.trim(); const cid = $('#oidc-clientid').value.trim();
      if (!issuer || !cid) { toast('OIDC incomplete', 'Issuer URL and client ID are required.', 'err'); return null; }
      const sec = $('#oidc-secret').value.trim();
      if (sec === '***') { toast('Secret masked', 'The client secret is masked as ***. Replace it with a real value or a ${ENV:...} placeholder.', 'err'); return null; }
      const spec = { issuerURL: issuer, clientID: cid, usePKCE: isOn('oidc-pkce') };
      if (sec) spec.clientSecret = sec;
      const sc = scopesCtl.get(); if (sc.length) spec.scopes = sc;
      if (isOn('oidc-verify')) spec.requireVerifiedEmail = true;
      // trustIdPMFA has no control any more: gpm has no local MFA prompt for it
      // to suppress, so nothing reads it. The stored value is carried forward
      // (an identity-provider PUT is a whole-object replacement) rather than
      // dropped on an unrelated save.
      if (oidc.trustIdPMFA !== undefined) spec.trustIdPMFA = oidc.trustIdPMFA;
      body.oidc = spec;
    } else if (t === 'forward-auth') {
      const tp = trustedCtl.get(); const uh = $('#fa-user').value.trim();
      if (!tp.length) { toast('Trusted proxies required', 'Add at least one trusted proxy CIDR.', 'err'); return null; }
      if (!uh) { toast('User header required', 'Set the user header.', 'err'); return null; }
      const spec = { trustedProxies: tp, userHeader: uh };
      const fields = { emailHeader: 'fa-email', nameHeader: 'fa-name', groupsHeader: 'fa-groups', groupsDelimiter: 'fa-delim', amrHeader: 'fa-amr' };
      Object.keys(fields).forEach((k) => { const v = $('#' + fields[k]).value.trim(); if (v) spec[k] = v; });
      body.forwardAuth = spec;
    } else {
      const out = $('#ar-outpost').value.trim();
      if (!out) { toast('Outpost URL required', 'Set the outpost URL.', 'err'); return null; }
      const spec = { outpostURL: out };
      const pp = $('#ar-prefix').value.trim(); if (pp) spec.pathPrefix = pp;
      const ap = $('#ar-authpath').value.trim(); if (ap) spec.authPath = ap;
      const ch = copyCtl.get(); if (ch.length) spec.copyHeaders = ch;
      body.authRequest = spec;
    }
    const rmBody = {}; const claim = $('#rm-claim').value.trim(); if (claim) rmBody.groupsClaim = claim;
    const ag = adminCtl.get(); if (ag.length) rmBody.adminGroups = ag;
    const ug = userCtl.get(); if (ug.length) rmBody.userGroups = ug;
    const dr = $('#rm-default').value; if (dr) rmBody.defaultRole = dr;
    if (Object.keys(rmBody).length) body.roleMapping = rmBody;
    return body;
  });
  wireCloneButton('identity', o);
}

// ---------- MIDDLEWARE EDITOR (polymorphic) ----------
async function middlewareEditor(c, name) {
  const meta = SECTION_META.middleware; const isNew = !name;
  const seed = isNew ? takeCloneSeed('middleware') : null;
  const [idpR, objR] = await Promise.all([
    refList('/api/identity-providers', 'identity providers'),
    isNew ? Promise.resolve({ data: { type: 'headers' } }) : api('/api/middlewares/' + encodeURIComponent(name)),
  ]);
  const idps = arr(idpR);
  const o = seed ? seed.data : (objR.data || {}); const type = o.type || 'headers';
  const auth = o.auth || {}; const headers = o.headers || {}; const guard = o.guard || {}; const rl = o.rateLimit || {}; const rewrite = o.rewrite || {};
  const bo = o.bouncer || {};
  // Populate from either form: requests+window as-is, or migrate a legacy
  // requestsPerSecond into requests + a 1s window so saving upgrades it.
  const rlRequests = rl.requests != null ? rl.requests : (rl.requestsPerSecond != null ? rl.requestsPerSecond : '');
  const rlWindow = rl.window || '1s';
  const RL_WINDOWS = ['1s', '10s', '30s', '1m', '5m', '15m', '1h'];
  // A hand-authored window outside the presets (e.g. "2m", "90s") must round-trip:
  // without a matching option the browser would silently fall back to 1s on save.
  if (!RL_WINDOWS.includes(rlWindow)) RL_WINDOWS.unshift(rlWindow);
  const rlBlockFor = rl.blockFor || '';
  const RL_BLOCKS = ['', '10s', '30s', '1m', '5m', '15m', '1h'];
  // Same round-trip guarantee as RL_WINDOWS: a hand-authored blockFor (e.g.
  // "2m") must render selected, not silently reset to "none" on save.
  if (!RL_BLOCKS.includes(rlBlockFor)) RL_BLOCKS.splice(1, 0, rlBlockFor);
  c.innerHTML = editorHead('middleware', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    <div class="card form-section"><p class="section-label">Type</p>
      <div class="field-group"><label>Middleware type</label><select class="field mono" id="ed-type" data-hint="middleware.type">
        ${enumOptions('middlewareType', ['auth', 'headers', 'guard', 'rate-limit', 'rewrite', 'bouncer'], type)}
      </select></div>
    </div>

    <div class="card form-section ed-sub" data-type="auth" style="${type === 'auth' ? '' : 'display:none'}"><p class="section-label">Auth</p>
      ${authBlockHtml('mw', auth, idps)}
    </div>

    <div class="card form-section ed-sub" data-type="headers" style="${type === 'headers' ? '' : 'display:none'}"><p class="section-label">Headers</p>
      <div class="field-group"><label>Set request headers</label><div id="hdr-setreq" data-hint="middleware.headers.setRequest"></div><button class="btn ghost sm hdr-add" data-wrap="hdr-setreq" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>
      <div class="field-group"><label>Set response headers</label><div id="hdr-setresp" data-hint="middleware.headers.setResponse"></div><button class="btn ghost sm hdr-add" data-wrap="hdr-setresp" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>
      <div class="field-group"><label>Remove request headers</label><div class="chip-input" id="hdr-rmreq" data-hint="middleware.headers.removeRequest"></div></div>
      <div class="field-group"><label>Remove response headers</label><div class="chip-input" id="hdr-rmresp" data-hint="middleware.headers.removeResponse"></div></div>
    </div>

    <div class="card form-section ed-sub" data-type="guard" style="${type === 'guard' ? '' : 'display:none'}"><p class="section-label">Guard</p>
      <div id="guard-triggers" data-hint="middleware.guard.triggers"></div>
      <button class="btn ghost sm" id="guard-addtrig" type="button" style="margin-top:6px">${ICON.plus}Add trigger</button>
      <div class="inline-fields" style="margin-top:14px">
        <div class="field-group"><label>Deny status</label><input class="field mono" id="guard-deny" data-hint="middleware.guard.denyStatus" type="number" value="${esc(guard.denyStatus != null ? guard.denyStatus : '')}" placeholder="403" /></div>
      </div>
      <div class="field-group"><label>Allow from (CIDRs)</label><div class="chip-input" id="guard-allow" data-hint="middleware.guard.allowFrom"></div></div>
    </div>

    <div class="card form-section ed-sub" data-type="rate-limit" style="${type === 'rate-limit' ? '' : 'display:none'}"><p class="section-label">Rate limit</p>
      <div class="inline-fields">
        <div class="field-group"><label>Requests</label><input class="field mono" id="rl-requests" data-hint="middleware.rateLimit.requests" type="number" step="0.1" value="${esc(rlRequests)}" placeholder="10" /></div>
        <div class="field-group"><label>Per</label><select class="field mono" id="rl-window" data-hint="middleware.rateLimit.window">
          ${RL_WINDOWS.map((w) => `<option value="${esc(w)}"${rlWindow === w ? ' selected' : ''}>${esc(w)}</option>`).join('')}
        </select></div>
        <div class="field-group"><label>Burst</label><input class="field mono" id="rl-burst" data-hint="middleware.rateLimit.burst" type="number" value="${esc(rl.burst != null ? rl.burst : '')}" placeholder="ceil(requests)" /></div>
        <div class="field-group"><label>Block for</label><select class="field mono" id="rl-block" data-hint="middleware.rateLimit.blockFor">
          ${RL_BLOCKS.map((b) => `<option value="${esc(b)}"${rlBlockFor === b ? ' selected' : ''}>${b ? esc(b) : 'none'}</option>`).join('')}
        </select></div>
      </div>
      <div class="field-group"><label>Allow from (CIDRs)</label><div class="chip-input" id="rl-allow" data-hint="middleware.rateLimit.allowFrom"></div></div>
      <div class="hint">Block for: once a client exceeds the limit, further requests from it are rejected for this long, regardless of token refill. Fixed - not extended by repeat requests during the block.</div>
    </div>

    <div class="card form-section ed-sub" data-type="bouncer" style="${type === 'bouncer' ? '' : 'display:none'}"><p class="section-label">Bouncer (deny hook)</p>
      <div class="hint">Asks an operator-run bouncer whether the client IP is banned. gpm ships no rules and no WAF engine - the verdict is entirely the external service's. Runs after the access list (an allow-list still wins) and before auth (a banned IP never reaches the IdP).</div>
      <div class="inline-fields" style="margin-top:12px">
        <div class="field-group"><label>Provider</label><select class="field mono" id="bo-provider" data-hint="middleware.bouncer.provider">
          ${enumOptions('bouncerProvider', ['crowdsec', 'http'], bo.provider || 'crowdsec')}
        </select></div>
        <div class="field-group"><label>On error</label><select class="field mono" id="bo-onerror" data-hint="middleware.bouncer.onError">
          ${enumOptions('bouncerOnError', ['fail-open', 'fail-closed'], bo.onError || 'fail-open')}
        </select></div>
      </div>
      <div class="field-group"><label>URL</label><input class="field mono" id="bo-url" data-hint="middleware.bouncer.url" value="${esc(bo.url || '')}" placeholder="http://crowdsec:8080" /></div>
      <div class="field-group"><label>API key</label><input class="field mono" id="bo-apikey" data-hint="middleware.bouncer.apiKey" value="${esc(bo.apiKey || '')}" placeholder="\${ENV:CROWDSEC_BOUNCER_KEY}" /><div class="hint">Sent as <span class="mono">X-Api-Key</span>. Required for crowdsec - register one with <span class="mono">cscli bouncers add gpm</span>. Use a <span class="mono">\${ENV:...}</span> or <span class="mono">\${FILE:...}</span> placeholder; a masked secret reads <span class="mono">***</span>.</div></div>
      <div class="inline-fields">
        <div class="field-group"><label>Timeout</label><input class="field mono" id="bo-timeout" data-hint="middleware.bouncer.timeout" value="${esc(bo.timeout || '')}" placeholder="2s" /></div>
        <div class="field-group"><label>Cache TTL</label><input class="field mono" id="bo-cachettl" data-hint="middleware.bouncer.cacheTTL" value="${esc(bo.cacheTTL || '')}" placeholder="60s" /></div>
        <div class="field-group"><label>Cache max entries</label><input class="field mono" id="bo-cachemax" data-hint="middleware.bouncer.cacheMaxEntries" type="number" value="${esc(bo.cacheMaxEntries != null ? bo.cacheMaxEntries : '')}" placeholder="10000" /></div>
        <div class="field-group"><label>Deny status</label><input class="field mono" id="bo-denystatus" data-hint="middleware.bouncer.denyStatus" type="number" value="${esc(bo.denyStatus != null ? bo.denyStatus : '')}" placeholder="403" /></div>
      </div>
      <div class="field-group"><label>Deny with</label><select class="field mono" id="bo-denywith" data-hint="middleware.bouncer.denyWith">
        ${enumOptions('bouncerDenyWith', ['error-page', 'plain'], bo.denyWith || 'error-page')}
      </select></div>
      <div class="toggle-line" id="bo-stream-line"><div class="tl-text"><div class="nm">Stream mode (crowdsec only)</div><div class="ds">Pull the whole decision set once, then deltas every cache TTL, so the request path is a local lookup with no per-request LAPI call</div></div>${switchHtml('bo-stream', !!bo.stream, 'Stream mode', 'middleware.bouncer.stream')}</div>
    </div>
    <div class="card form-section ed-sub" data-type="rewrite" style="${type === 'rewrite' ? '' : 'display:none'}"><p class="section-label">Rewrite</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">Rules are tried in order: exact, then prefix (longest first), then regex. The first match wins, and a rewrite is never a redirect - the method, body and query string reach the backend unchanged. Security controls (rate limit, access list, bouncer, auth, guards) always evaluate the ORIGINAL client path.</p>
      <div class="field-group"><label>Exact</label>
        <div class="hint" style="margin:0 0 6px">The request path must equal "From" exactly.</div>
        <div id="rw-replacepath" data-hint="middleware.rewrite.replacePath"></div>
        <button class="btn ghost sm" id="rw-add" type="button" style="margin-top:6px">${ICON.plus}Add exact rule</button>
      </div>
      <div class="field-group" style="margin-top:12px"><label>Prefix</label>
        <div class="hint" style="margin:0 0 6px">Replaces a matched path prefix. Matching stops at a "/" boundary, so <span class="mono">/reports</span> never matches <span class="mono">/reports-evil</span>. The longest matching rule wins.</div>
        <div id="rw-prefix" data-hint="middleware.rewrite.prefixRules"></div>
        <button class="btn ghost sm" id="rw-prefix-add" type="button" style="margin-top:6px">${ICON.plus}Add prefix rule</button>
      </div>
      <div class="field-group" style="margin-top:12px"><label>Regex</label>
        <div class="hint" style="margin:0 0 6px">Matched at the start of the path (anchored with "^" automatically). Use <span class="mono">$1</span> in "To" for a capture group.</div>
        <div id="rw-regex" data-hint="middleware.rewrite.regexRules"></div>
        <button class="btn ghost sm" id="rw-regex-add" type="button" style="margin-top:6px">${ICON.plus}Add regex rule</button>
      </div>
    </div>
  </div></div>` + saveBar('middleware', isNew, meta.addLabel);

  const authCtl = wireAuthBlock('mw', auth, idps);
  const setReqCtl = makeKVRows($('#hdr-setreq'), headers.setRequest || {}, 'Header', 'value', false);
  const setRespCtl = makeKVRows($('#hdr-setresp'), headers.setResponse || {}, 'Header', 'value', false);
  const rmReqCtl = makeChipInput($('#hdr-rmreq'), arr(headers.removeRequest), 'add header...');
  const rmRespCtl = makeChipInput($('#hdr-rmresp'), arr(headers.removeResponse), 'add header...');
  const guardAllowCtl = makeChipInput($('#guard-allow'), arr(guard.allowFrom), 'add CIDR...');
  const rlAllowCtl = makeChipInput($('#rl-allow'), arr(rl.allowFrom), 'add CIDR...');
  const rwCtl = makeKVRows($('#rw-replacepath'), rewrite.replacePath || {}, '/application/o/token', '/application/o/token/', false);
  $$('.hdr-add').forEach((b) => b.addEventListener('click', () => { (b.dataset.wrap === 'hdr-setreq' ? setReqCtl : setRespCtl).addRow('', ''); }));
  $('#rw-add').addEventListener('click', () => rwCtl.addRow('', ''));

  // Prefix and regex rules are ordered arrays of {from,to}, not a map, so they
  // get their own repeater rather than makeKVRows (which is keyed and unordered).
  const REWRITE_MAX_RULES = 32;
  function rewriteRows(wrapId, addId, initial, fromPlace, toPlace, fromLabel) {
    const wrap = $('#' + wrapId);
    function addRow(r) {
      r = r || {};
      const d = document.createElement('div');
      d.className = 'loc-row';
      d.innerHTML = `<input class="field mono rw-from" data-hint="middleware.rewrite.rule.from" style="flex:1 1 170px" value="${esc(r.from || '')}" placeholder="${esc(fromPlace)}" aria-label="${esc(fromLabel)}" />
        <span class="arrow">${ICON.arrow}</span>
        <input class="field mono rw-to" data-hint="middleware.rewrite.rule.to" style="flex:1 1 170px" value="${esc(r.to || '')}" placeholder="${esc(toPlace)}" aria-label="To" />
        <button class="icon-btn rw-del" type="button" aria-label="Remove rule">${ICON.x}</button>`;
      d.querySelector('.rw-del').addEventListener('click', () => { d.remove(); sync(); });
      wrap.appendChild(d);
      sync();
    }
    function rows() { return Array.from(wrap.querySelectorAll(':scope > .loc-row')); }
    function sync() { gateControl($('#' + addId), rows().length < REWRITE_MAX_RULES, `At most ${REWRITE_MAX_RULES} rules.`); }
    arr(initial).forEach(addRow);
    sync();
    $('#' + addId).addEventListener('click', () => addRow({}));
    return { rows, wrap };
  }
  const rwPrefix = rewriteRows('rw-prefix', 'rw-prefix-add', rewrite.prefixRules, '/old-app', '/app', 'From');
  const rwRegex = rewriteRows('rw-regex', 'rw-regex-add', rewrite.regexRules, '/user/([0-9]+)', '/u/$1', 'Pattern');

  // Stream mode is a CrowdSec LAPI protocol feature; grey it out (rather than
  // accept it and let the server reject it) while another provider is selected.
  function syncBouncerProvider() {
    const crowdsec = $('#bo-provider').value === 'crowdsec';
    if (!crowdsec) $('#bo-stream').setAttribute('aria-checked', 'false');
    gateControl($('#bo-stream'), crowdsec, 'Stream mode is only supported by the crowdsec provider.');
  }
  $('#bo-provider').addEventListener('change', syncBouncerProvider);
  syncBouncerProvider();

  const trigWrap = $('#guard-triggers'); const trigCtls = [];
  function trigRow(t) {
    t = t || {}; const d = document.createElement('div'); d.className = 'card form-section'; d.style.marginBottom = '8px';
    d.innerHTML = `<div class="row-between" style="margin-bottom:8px"><span class="ci-ty">trigger</span><button class="icon-btn trig-del" type="button" aria-label="Remove trigger">${ICON.x}</button></div>
      <div class="field-group"><label>Paths</label><div class="chip-input trig-paths" data-hint="middleware.guard.triggers.paths"></div></div>
      <div class="field-group"><label>Methods</label><div class="chip-input trig-methods" data-hint="middleware.guard.triggers.methods"></div></div>
      <div class="field-group"><label>Query equals</label><div class="trig-query" data-hint="middleware.guard.triggers.queryEquals"></div><button class="btn ghost sm trig-addq" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>`;
    trigWrap.appendChild(d);
    const pathsCtl = makeChipInput(d.querySelector('.trig-paths'), arr(t.paths), 'add path...');
    const methodsCtl = makeChipInput(d.querySelector('.trig-methods'), arr(t.methods), 'add method...');
    const queryCtl = makeKVRows(d.querySelector('.trig-query'), t.queryEquals || {}, 'arg', 'value', false);
    d.querySelector('.trig-addq').addEventListener('click', () => queryCtl.addRow('', ''));
    const ctl = { pathsCtl, methodsCtl, queryCtl };
    d.querySelector('.trig-del').addEventListener('click', () => { d.remove(); const i = trigCtls.indexOf(ctl); if (i >= 0) trigCtls.splice(i, 1); });
    trigCtls.push(ctl);
  }
  arr(guard.triggers).forEach(trigRow);
  $('#guard-addtrig').addEventListener('click', () => trigRow({}));

  $('#ed-type').addEventListener('change', () => { const t = $('#ed-type').value; $$('.ed-sub').forEach((el) => { el.style.display = el.dataset.type === t ? '' : 'none'; }); });

  wireEditor('middleware', 'middlewares', meta, isNew, name || o.name, o, () => {
    const t = $('#ed-type').value; const body = { type: t };
    if (t === 'auth') {
      const spec = authCtl.get();
      if (!spec) return null;
      body.auth = spec;
    } else if (t === 'headers') {
      const spec = {};
      const sr = setReqCtl.get(); if (Object.keys(sr).length) spec.setRequest = sr;
      const sp = setRespCtl.get(); if (Object.keys(sp).length) spec.setResponse = sp;
      const rr = rmReqCtl.get(); if (rr.length) spec.removeRequest = rr;
      const rp = rmRespCtl.get(); if (rp.length) spec.removeResponse = rp;
      // Same rule as guard and rewrite: an empty spec is a middleware that does
      // nothing, and committing `headers: {}` looks like a configured object in
      // git while changing no request at all.
      if (!Object.keys(spec).length) { toast('Header rule required', 'Set or remove at least one request or response header.', 'err'); return null; }
      body.headers = spec;
    } else if (t === 'guard') {
      const triggers = [];
      trigCtls.forEach((ctl) => {
        const tr = {}; const p = ctl.pathsCtl.get(); const m = ctl.methodsCtl.get(); const q = ctl.queryCtl.get();
        if (p.length) tr.paths = p;
        if (m.length) tr.methods = m;
        if (Object.keys(q).length) tr.queryEquals = q;
        if (Object.keys(tr).length) triggers.push(tr);
      });
      if (!triggers.length) { toast('Trigger required', 'Add at least one guard trigger.', 'err'); return null; }
      const spec = { triggers };
      const allow = guardAllowCtl.get(); if (allow.length) spec.allowFrom = allow;
      const ds = parseInt($('#guard-deny').value, 10); if (!isNaN(ds)) spec.denyStatus = ds;
      body.guard = spec;
    } else if (t === 'rate-limit') {
      const requests = parseFloat($('#rl-requests').value);
      if (isNaN(requests) || requests <= 0) { toast('Rate required', 'Requests must be > 0.', 'err'); return null; }
      const spec = { requests, window: $('#rl-window').value };
      const burst = parseInt($('#rl-burst').value, 10); if (!isNaN(burst)) spec.burst = burst;
      const allow = rlAllowCtl.get(); if (allow.length) spec.allowFrom = allow;
      const blockFor = $('#rl-block').value; if (blockFor) spec.blockFor = blockFor;
      body.rateLimit = spec;
    } else if (t === 'bouncer') {
      const provider = $('#bo-provider').value;
      const url = $('#bo-url').value.trim();
      if (!url) { toast('URL required', 'Enter the bouncer URL.', 'err'); return null; }
      const apiKey = $('#bo-apikey').value.trim();
      if (apiKey === '***') { toast('Secret masked', 'The API key is masked as ***. Replace it with a real value or a ${ENV:...} placeholder.', 'err'); return null; }
      if (provider === 'crowdsec' && !apiKey) { toast('API key required', 'The crowdsec provider needs an API key (cscli bouncers add gpm).', 'err'); return null; }
      const spec = { provider, url };
      if (apiKey) spec.apiKey = apiKey;
      const timeout = $('#bo-timeout').value.trim(); if (timeout) spec.timeout = timeout;
      const cacheTTL = $('#bo-cachettl').value.trim(); if (cacheTTL) spec.cacheTTL = cacheTTL;
      const cacheMax = parseInt($('#bo-cachemax').value, 10); if (!isNaN(cacheMax)) spec.cacheMaxEntries = cacheMax;
      const onError = $('#bo-onerror').value; if (onError) spec.onError = onError;
      const denyStatus = parseInt($('#bo-denystatus').value, 10); if (!isNaN(denyStatus)) spec.denyStatus = denyStatus;
      const denyWith = $('#bo-denywith').value; if (denyWith) spec.denyWith = denyWith;
      if (provider === 'crowdsec' && isOn('bo-stream')) spec.stream = true;
      body.bouncer = spec;
    } else if (t === 'rewrite') {
      const rp = rwCtl.get();
      const entries = Object.entries(rp);
      for (const [k, v] of entries) {
        if (!k.startsWith('/') || !v.startsWith('/')) { toast('Invalid path', 'Path must be absolute (start with "/").', 'err'); return null; }
        if (k === v) { toast('No-op rewrite', `A path cannot be rewritten to itself ("${k}").`, 'err'); return null; }
      }
      // Per-row validation, so the complaint lands on the row it belongs to
      // rather than in a page-level toast the operator has to map back by hand.
      function readRules(ctl, regex) {
        clearRowErrors(ctl.wrap);
        const out = [];
        let err = '';
        ctl.rows().forEach((r) => {
          if (err) return;
          const from = r.querySelector('.rw-from').value.trim();
          const to = r.querySelector('.rw-to').value.trim();
          if (!from && !to) return;
          if (regex) {
            if (!from) { err = markRowError(r, 'Enter a pattern.'); return; }
            if (from.length > 256) { err = markRowError(r, 'Pattern must be 256 characters or fewer.'); return; }
            try { new RegExp(from); } catch (e) { err = markRowError(r, 'Not a valid regular expression.'); return; }
          } else if (!from || from[0] !== '/') {
            err = markRowError(r, 'From must be an absolute path (start with "/").'); return;
          }
          if (!to || to[0] !== '/') { err = markRowError(r, 'To must be an absolute path (start with "/").'); return; }
          if (!regex && from === to) { err = markRowError(r, 'This rule rewrites a path to itself.'); return; }
          out.push({ from, to });
        });
        return err ? { err } : { out };
      }
      const pre = readRules(rwPrefix, false);
      if (pre.err) { toast('Prefix rule invalid', pre.err, 'err'); return null; }
      const rex = readRules(rwRegex, true);
      if (rex.err) { toast('Regex rule invalid', rex.err, 'err'); return null; }
      if (!entries.length && !pre.out.length && !rex.out.length) {
        toast('Rule required', 'Add at least one rule.', 'err'); return null;
      }
      const spec = {};
      if (entries.length) spec.replacePath = rp;
      if (pre.out.length) spec.prefixRules = pre.out;
      if (rex.out.length) spec.regexRules = rex.out;
      body.rewrite = spec;
    }
    return body;
  });
  wireCloneButton('middleware', o);
}

// section -> editor dispatch for the typed object editors
// ---------- CLIENT CA EDITOR ----------
// The trust anchor for per-host mTLS (tls.clientAuth.caRef). A CA is either
// generated here (self-signed, key stored by gpm, issuance-ready) or brought from
// outside by pasting its certificate. With a signing key present the same object
// also ISSUES client certificates, which is the right-hand column on the edit
// view. Revocation and the signing key are optional, so they collapse to a
// one-line summary; the semantics behind every field live in
// docs/configuration.md rather than in helper paragraphs on this page.

// segHtml renders an either/or picker plus the panels it switches between, so a
// mutually exclusive pair (CRL file vs inline, key file vs inline, generate vs
// paste) is one control instead of two stacked ones the operator has to know
// cannot both be set. opts is [{v, label, panel}].
function segHtml(group, opts, current) {
  const buttons = opts.map((o) =>
    `<button type="button" class="seg-btn${o.v === current ? ' on' : ''}" data-seg="${esc(group)}" data-v="${esc(o.v)}">${esc(o.label)}</button>`).join('');
  const panels = opts.map((o) =>
    `<div data-seg-panel="${esc(group)}" data-v="${esc(o.v)}"${o.v === current ? '' : ' hidden'}>${o.panel}</div>`).join('');
  return `<div class="seg">${buttons}</div>${panels}`;
}
// wireSegs makes every segmented picker on the page switch panels, and calls
// onChange(group, value) so a caller can react to the choice.
function wireSegs(onChange) {
  $$('.seg-btn').forEach((b) => b.addEventListener('click', () => {
    const g = b.dataset.seg;
    $$(`.seg-btn[data-seg="${g}"]`).forEach((x) => x.classList.toggle('on', x === b));
    $$(`[data-seg-panel="${g}"]`).forEach((p) => { p.hidden = p.dataset.v !== b.dataset.v; });
    if (onChange) onChange(g, b.dataset.v);
  }));
}
// resolvePair implements the save rule for one either/or pair: typed is the
// selected side's current value, storedOther is what the OTHER side holds in the
// saved object. It returns [selectedOut, otherOut].
//
// A non-empty selected control wins and clears the other side - that is a
// deliberate switch. An EMPTY selected control preserves the other side
// unchanged, which is what makes toggling the picker to look at the other option
// and saving a byte-for-byte no-op. Clearing the visible field still removes the
// value, because then both sides resolve empty.
function resolvePair(typed, storedOther) {
  return typed ? [typed, ''] : ['', storedOther];
}
// segValue is the currently selected option of a picker.
function segValue(group) {
  const on = $(`.seg-btn[data-seg="${group}"].on`);
  return on ? on.dataset.v : '';
}
// foldHtml is an optional section collapsed to a one-line summary of what it
// holds. It opens automatically when it holds something, so a configured section
// is never hidden from someone scanning the page.
function foldHtml(id, label, summary, open, body) {
  return `<details class="card form-section fold" id="${esc(id)}"${open ? ' open' : ''}>
    <summary><p class="section-label">${esc(label)}</p><span class="fold-sum">${glossaryize(esc(summary))}</span></summary>
    ${body}
  </details>`;
}

async function clientCAEditor(c, name) {
  const meta = SECTION_META.clientcas; const isNew = !name;
  const seed = isNew ? takeCloneSeed('clientcas') : null;
  const [objR, issuedR] = await Promise.all([
    isNew ? Promise.resolve({ data: {} }) : api('/api/client-cas/' + encodeURIComponent(name)),
    isNew ? Promise.resolve({ data: {} }) : api('/api/client-cas/' + encodeURIComponent(name) + '/certificates').catch(() => ({ data: {} })),
  ]);
  const o = seed ? seed.data : (objR.data || {});
  const issued = arr((issuedR.data || {}).certificates);
  // Issuance needs a signing key AND a saved object to POST against, so the card
  // is greyed out (never a button that 422s) until both hold.
  const hasKey = !!(o.caKeyFile || o.caKeyPEM);
  const hasCRL = !!(o.crlFile || o.crlPEM);

  const pastePanel = `<div class="field-group"><label>CA certificate (PEM)</label>
      <textarea class="field mono" id="ed-capem" data-hint="clientCA.caPEM" rows="10" placeholder="-----BEGIN CERTIFICATE-----">${esc(o.caPEM || '')}</textarea>
      <div class="hint">One or more certificates, or a <span class="mono">\${FILE:...}</span> placeholder. Public material only - never a private key.</div>
    </div>`;
  const generatePanel = `<div class="field-group"><label>Common name</label>
      <input class="field mono" id="gen-cn" data-hint="clientCA.generate.commonName" maxlength="64" placeholder="defaults to the CA name" />
    </div>
    <div class="inline-fields">
      <div class="field-group"><label>Validity (days)</label>
        <input class="field mono" id="gen-days" data-hint="clientCA.generate.days" type="number" min="1" max="7300" value="3650" /></div>
      <div class="field-group"><label>Organization</label>
        <input class="field" id="gen-org" data-hint="clientCA.generate.org" maxlength="64" placeholder="optional" /></div>
    </div>
    <div class="field-group"><button class="btn primary" id="gen-btn" type="button">Generate CA</button>
      <div class="hint">Creates a self-signed RSA-4096 CA and stores its key in the certificate store, ready to issue. Saves immediately - no external tooling needed.</div>
    </div>`;

  const anchorCard = `<div class="card form-section"><p class="section-label">Trust anchor</p>
    ${isNew ? segHtml('anchor', [
      { v: 'generate', label: 'Generate new CA', panel: generatePanel },
      { v: 'paste', label: 'Paste existing CA', panel: pastePanel },
      // A clone already HAS a certificate - the seed populated the paste field -
      // so cloning lands on "Paste existing CA". Defaulting to generate would hide
      // the cloned PEM behind the inactive panel and grey out Save, which is the
      // clone flow silently doing nothing.
    ], seed ? 'paste' : 'generate') : pastePanel}
  </div>`;

  const crlSummary = hasCRL
    ? `${o.crlFile || 'inline PEM'}, ${o.crlPolicy || 'fail-closed'}`
    : 'not configured - a revoked certificate passes until it expires';
  const crlCard = foldHtml('crl-card', 'Revocation (CRL)', crlSummary, hasCRL,
    segHtml('crl', [
      { v: 'file', label: 'File', panel: `<div class="field-group"><label>CRL file</label>
          <input class="field mono" id="ed-crlfile" data-hint="clientCA.crlFile" value="${esc(o.crlFile || '')}" placeholder="corp.crl" />
        </div>` },
      { v: 'inline', label: 'Inline PEM', panel: `<div class="field-group"><label>CRL (PEM)</label>
          <textarea class="field mono" id="ed-crlpem" data-hint="clientCA.crlPEM" rows="5" placeholder="-----BEGIN X509 CRL-----">${esc(o.crlPEM || '')}</textarea>
          <div class="hint">For a small list kept in git.</div>
        </div>` },
    ], o.crlPEM ? 'inline' : 'file') +
    `<div class="field-group"><label>When the CRL is unusable</label>
      <select class="field mono" id="ed-crlpolicy" data-hint="clientCA.crlPolicy" data-path="crlPolicy">
        ${enumOptions('crlPolicy', ['', 'fail-open'], o.crlPolicy === 'fail-open' ? 'fail-open' : '')}
      </select>
    </div>`);

  const keySummary = hasKey ? (o.caKeyFile || 'inline key') : 'not configured - issuance disabled';
  const keyCard = foldHtml('cakey-card', 'Signing key', keySummary, hasKey,
    segHtml('cakey', [
      { v: 'file', label: 'File', panel: `<div class="field-group"><label>CA key file</label>
          <input class="field mono" id="ed-cakeyfile" data-hint="clientCA.caKeyFile" value="${esc(o.caKeyFile || '')}" placeholder="client-cas/corp.key" />
        </div>` },
      { v: 'inline', label: 'Inline', panel: `<div class="field-group"><label>CA key</label>
          <input class="field mono" id="ed-cakeypem" data-hint="clientCA.caKeyPEM" value="${esc(o.caKeyPEM || '')}" placeholder="\${FILE:/run/secrets/corp_ca.key}" />
          <div class="hint">Use a <span class="mono">\${FILE:...}</span> placeholder - a literal key is refused at commit.</div>
        </div>` },
    ], o.caKeyPEM ? 'inline' : 'file') +
    `<div class="field-group"><label>Expiry warning (days)</label>
      <input class="field mono" id="ed-warndays" data-hint="clientCA.expiryWarningDays" type="number" min="0" max="3650" value="${esc(o.expiryWarningDays || '')}" placeholder="30" />
    </div>`);

  const configStack = `<div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    ${anchorCard}
    ${crlCard}
    ${keyCard}
  </div>`;
  const actionStack = isNew ? '' : `<div class="stack">
    ${issueCard()}
    ${issuedCertsCard(issued)}
  </div>`;

  c.innerHTML = editorHead('clientcas', meta, isNew, name) + expiryBanner(issued)
    + (isNew ? configStack : `<div class="form-grid">${configStack}${actionStack}</div>`)
    + saveBar('clientcas', isNew, meta.addLabel);

  wireEditor('clientcas', 'client-cas', meta, isNew, name || o.name, o, () => {
    const caPEM = $('#ed-capem').value.trim();
    if (!caPEM) { toast('CA required', 'Paste the CA certificate PEM, or switch to "Generate new CA".', 'err'); return null; }
    // Each either/or pair resolves through resolvePair, so only one side is ever
    // submitted AND merely toggling the picker to look at the other side is a
    // no-op. Reading the selected control alone would silently wipe the stored
    // value on the other side (revocation or issuance quietly switching off).
    let file, inline, keyFile, keyPEM;
    if (segValue('crl') === 'inline') {
      [inline, file] = resolvePair($('#ed-crlpem').value.trim(), o.crlFile || '');
    } else {
      [file, inline] = resolvePair($('#ed-crlfile').value.trim(), o.crlPEM || '');
    }
    if (segValue('cakey') === 'inline') {
      [keyPEM, keyFile] = resolvePair($('#ed-cakeypem').value.trim(), o.caKeyFile || '');
    } else {
      [keyFile, keyPEM] = resolvePair($('#ed-cakeyfile').value.trim(), o.caKeyPEM || '');
    }
    // Checked on the RESOLVED value, so the redaction guard still fires when the
    // masked key is being carried over from the unselected side of the picker.
    if (keyPEM === '***') { toast('Secret masked', 'The inline CA key is masked as ***. Replace it with a ${FILE:...} placeholder or clear it.', 'err'); return null; }
    const body = { caPEM };
    if (file) body.crlFile = file;
    if (inline) body.crlPEM = inline;
    const policy = $('#ed-crlpolicy').value;
    if (policy && (file || inline)) body.crlPolicy = policy;
    if (keyFile) body.caKeyFile = keyFile;
    if (keyPEM) body.caKeyPEM = keyPEM;
    // The API PUT is a whole-object replace, so every field the editor knows
    // about has to be sent back or a UI save silently resets it to the default.
    const warnDays = parseInt($('#ed-warndays').value, 10);
    if (!isNaN(warnDays) && warnDays !== 0) {
      if (warnDays < 0 || warnDays > 3650) { toast('Warning window out of range', 'Expiry warning must be between 0 (default 30) and 3650 days.', 'err'); return null; }
      body.expiryWarningDays = warnDays;
    }
    return body;
  });

  // On the new-CA page the trust-anchor choice decides what the save bar means.
  // With "Generate" selected there is no PEM to save, so #ed-save is greyed out
  // with the reason and "Generate CA" is the action; with "Paste" selected the
  // save bar is live again. Only #ed-save is gated - the Generate button lives
  // inside its own panel, which is hidden whenever it is not the active choice.
  // The same toggle hides the sections generate does not use (below), so no
  // filled-in field is ever silently dropped. A follower has already had every
  // write control gated and that gating is absolute, so this never re-enables
  // anything there.
  const readOnly = hasCapability('ha.readOnly');
  const syncAnchorMode = (v) => {
    // Revocation, the signing key and the expiry window are ordinary config
    // fields that POST /generate does not accept, so they are hidden while
    // generating rather than left on screen to be filled in and discarded. They
    // come back with "Paste existing CA", and are always present when editing.
    const generating = v === 'generate';
    [$('#crl-card'), $('#cakey-card')].forEach((el) => { if (el) el.hidden = generating; });
    if (readOnly) return;
    gateControl($('#ed-save'), !generating, 'Use "Generate CA" to create this CA, or switch to "Paste existing CA".');
  };
  wireSegs((group, v) => { if (group === 'anchor') syncAnchorMode(v); });
  if (isNew) {
    syncAnchorMode(segValue('anchor'));
    wireClientCAGenerate();
  } else {
    wireClientCertIssue(name || o.name, hasKey);
    wireClientCertRenew(name || o.name, hasKey);
  }
  wireCloneButton('clientcas', o);
}

// wireClientCAGenerate wires the "Generate CA" button on the new-CA page. Unlike
// issue and renew this is an ordinary config write, so it reports its commit like
// any save and lands on the created object's edit page - where the issue card is
// now live, because the CA it just made has a signing key.
function wireClientCAGenerate() {
  const btn = $('#gen-btn');
  if (!btn) return;
  btn.addEventListener('click', async () => {
    const nm = $('#ed-name').value.trim();
    if (!nm) { toast('Name required', 'Enter a name for the CA first.', 'err'); return; }
    const days = parseInt($('#gen-days').value, 10);
    if (isNaN(days) || days < 1 || days > 7300) { toast('Validity out of range', 'CA validity must be between 1 and 7300 days.', 'err'); return; }
    const body = { validityDays: days };
    const cn = $('#gen-cn').value.trim(); if (cn) body.commonName = cn;
    const org = $('#gen-org').value.trim(); if (org) body.organization = org;
    btn.disabled = true;
    try {
      const r = await api('/api/client-cas/' + encodeURIComponent(nm) + '/generate', { method: 'POST', body });
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      location.hash = '#/clientcas/' + encodeURIComponent(nm);
    } catch (e) { toastErr(e); btn.disabled = false; }
  });
}

// P12_MIN_PASSWORD mirrors clientcert.MinPasswordLen. The server enforces it; this
// is only so the operator is told before a round trip.
const P12_MIN_PASSWORD = 12;

// ---------- issued client certificates ----------
// gpm remembers what each ClientCA issued (subject, serial, validity - never key
// material) so an operator can see what is about to expire. Nothing renews on its
// own and nothing can: the certificate lives in a keychain on someone's device,
// so every renewal ends in a human importing a new .p12 there. The UI says so
// wherever it offers the action.

// currentIssued drops superseded records: those were already renewed, so warning
// about them again is noise. They stay in the table so the operator remembers old
// copies are still installed somewhere.
function currentIssued(issued) { return issued.filter((r) => !r.supersededBy); }

// expiryBanner renders the pre-expiry warning above the editor. It names each
// certificate and its remaining days, and states the part that is easy to forget:
// there is no client-side renewal, so every device holding the certificate has to
// import the replacement by hand.
function expiryBanner(issued) {
  const bad = currentIssued(issued).filter((r) => r.status === 'expiring' || r.status === 'expired');
  if (!bad.length) return '';
  const anyExpired = bad.some((r) => r.status === 'expired');
  const items = bad.map((r) => {
    const when = r.status === 'expired'
      ? `expired ${Math.abs(r.daysRemaining)} day${Math.abs(r.daysRemaining) === 1 ? '' : 's'} ago`
      : `${r.daysRemaining} day${r.daysRemaining === 1 ? '' : 's'} left`;
    return `<li><b>${esc(r.commonName)}</b> <span class="mono">${esc(r.serial)}</span> - ${esc(when)} (expires ${esc(fmtTime(r.notAfter))})</li>`;
  }).join('');
  return `<div class="ro-banner ${anyExpired ? 'err' : 'warn'}">
    <b>${bad.length} issued client certificate${bad.length === 1 ? '' : 's'} ${anyExpired ? 'expired or expiring' : 'expiring soon'}.</b>
    Renew each one below and re-issue the bundle. <b>There is no client-side renewal:</b> every device
    using the certificate must import the new <span class="mono">.p12</span> by hand, so plan the
    re-import before the date below, not after it.
    <ul>${items}</ul>
  </div>`;
}

// issueCard is the mint-a-certificate form. Every field carries at most one hint
// line; the reasoning behind the password floor and the legacy bundle encoding
// lives in docs/configuration.md.
function issueCard() {
  return `<div class="card form-section" id="issue-card"><p class="section-label">Issue client certificate</p>
    <div class="field-group"><label>Common name</label>
      <input class="field mono" id="iss-cn" data-hint="clientCA.issue.commonName" maxlength="64" placeholder="phone-01" />
      <div class="hint">The certificate subject, and the download filename.</div>
    </div>
    <div class="inline-fields">
      <div class="field-group"><label>Validity (days)</label>
        <input class="field mono" id="iss-days" data-hint="clientCA.issue.days" type="number" min="1" max="3650" value="365" /></div>
      <div class="field-group"><label>Bundle password</label>
        <input class="field mono" id="iss-pw" data-hint="clientCA.issue.password" type="password" minlength="12" placeholder="at least 12 characters" /></div>
    </div>
    <div class="field-group"><label>Subject alternative names</label>
      <input class="field mono" id="iss-sans" data-hint="clientCA.issue.sans" placeholder="phone-01.example.com, ops@example.com, 10.1.2.3" />
      <div class="hint">Optional, comma-separated. An IP becomes an IP SAN, a value with <span class="mono">@</span> an email SAN, anything else DNS.</div>
    </div>
    <div class="field-group"><button class="btn primary" id="iss-btn" type="button">Download .p12</button>
      <div class="hint">The private key exists only in the download - gpm records the subject, serial and validity, never the key.</div>
    </div>
  </div>`;
}

// issuedStatusChip colours the live expiry state. A superseded record is history:
// its certificate is still valid on devices that have not re-imported, but it is
// no longer the one to act on, so it gets a neutral chip and no warning colour.
function issuedStatusChip(r) {
  if (r.supersededBy) return `<span class="chip">${esc(r.status)}</span>`;
  const cls = r.status === 'expired' ? 'err' : (r.status === 'expiring' ? 'warn' : 'ok');
  return `<span class="chip ${cls}"><span class="dot ${cls}"></span>${esc(r.status)}</span>`;
}

// issuedCertsCard lists this CA's issuance records with a per-row Renew action.
function issuedCertsCard(issued) {
  if (!issued.length) {
    return `<div class="card form-section"><p class="section-label">Issued certificates</p>
      <div class="hint">None yet. Certificates issued above appear here with their expiry.</div>
    </div>`;
  }
  const rows = issued.map((r) => `<tr${r.supersededBy ? ' class="superseded"' : ''}>
      <td>${esc(r.commonName)}${r.supersededBy ? ` <span class="chip">superseded</span>` : ''}</td>
      <td class="mono">${esc(r.serial)}</td>
      <td>${esc(fmtTime(r.notAfter))}</td>
      <td>${issuedStatusChip(r)}</td>
      <td style="text-align:right">${r.supersededBy
        ? `<span class="hint">renewed as <span class="mono">${esc(r.supersededBy)}</span></span>`
        : `<button class="btn sm ren-btn" type="button" data-serial="${esc(r.serial)}" data-cn="${esc(r.commonName)}">Renew</button>`}</td>
    </tr>
    ${r.supersededBy ? '' : `<tr class="ren-row" id="ren-${esc(r.serial)}" hidden><td colspan="5">
      <div class="inline-fields">
        <div class="field-group"><label>New bundle password</label>
          <input class="field mono ren-pw" data-hint="clientCA.renew.password" type="password" minlength="12" placeholder="at least 12 characters" /></div>
        <div class="field-group"><label>Validity (days)</label>
          <input class="field mono ren-days" data-hint="clientCA.renew.days" type="number" min="1" max="3650" value="365" /></div>
      </div>
      <div style="display:flex;gap:10px;margin-top:10px">
        <button class="btn primary ren-go" type="button" data-serial="${esc(r.serial)}" data-cn="${esc(r.commonName)}">Confirm renewal</button>
        <button class="btn ghost ren-cancel" type="button" data-serial="${esc(r.serial)}">Cancel</button>
      </div>
      <div class="hint" style="margin-top:8px">New key and serial, same subject. Does not revoke the current certificate - every device must import the new .p12.</div>
    </td></tr>`}`).join('');
  return `<div class="card form-section"><p class="section-label">Issued certificates</p>
    <table class="mini-table">
      <thead><tr><th>Common name</th><th>Serial</th><th>Expires</th><th>Status</th><th></th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
  </div>`;
}

// wireClientCertRenew wires the per-row Renew action. Clicking Renew only reveals
// the form; the request is sent from "Confirm renewal", behind the same native
// confirm() the delete flows use, with the consequences spelled out.
function wireClientCertRenew(name, hasKey) {
  $$('.ren-btn').forEach((b) => {
    // Without a signing key the CA can no longer sign anything, so renewal is as
    // impossible as issuance - grey it out rather than let it 422.
    gateControl(b, hasKey, 'No signing key configured on this client CA - renewal needs the CA key.');
    if (!hasKey) return;
    b.addEventListener('click', () => {
      const row = $('#ren-' + CSS.escape(b.dataset.serial));
      if (row) row.hidden = !row.hidden;
    });
  });
  $$('.ren-cancel').forEach((b) => b.addEventListener('click', () => {
    const row = $('#ren-' + CSS.escape(b.dataset.serial));
    if (row) row.hidden = true;
  }));
  $$('.ren-go').forEach((b) => {
    if (!hasKey) { b.disabled = true; return; }
    b.addEventListener('click', async () => {
      const row = $('#ren-' + CSS.escape(b.dataset.serial));
      const pw = $('.ren-pw', row).value;
      if (!pw) { toast('Password required', 'The renewed PKCS#12 bundle must be password-protected.', 'err'); return; }
      if (pw.length < P12_MIN_PASSWORD) { toast('Password too short', `Use at least ${P12_MIN_PASSWORD} characters: the legacy PKCS#12 encoder barely stretches it, so a short password is cheap to crack offline once the file leaves gpm.`, 'err'); return; }
      const days = parseInt($('.ren-days', row).value, 10);
      if (isNaN(days) || days < 1 || days > 3650) { toast('Validity out of range', 'Validity must be between 1 and 3650 days.', 'err'); return; }
      if (!confirm(`Renew "${b.dataset.cn}"?\n\nA NEW private key and certificate will be generated. `
        + `The download is the only copy - it is never stored and cannot be recovered.\n\n`
        + `The current certificate is NOT revoked and keeps working until it expires, and every device `
        + `using it must import the new .p12 by hand.`)) return;
      b.disabled = true;
      try {
        await downloadP12('/api/client-cas/' + encodeURIComponent(name) + '/certificates/'
          + encodeURIComponent(b.dataset.serial) + '/renew', { password: pw, validityDays: days }, b.dataset.cn);
        toast('Certificate renewed', 'Import the new .p12 on every device using this certificate - the old one is not revoked and still works until it expires.', 'ok');
        route();
      } catch (e) { toastErr(e); b.disabled = false; }
    });
  });
}

// Wires the "Issue client certificate" card. With no signing key on the CA the
// whole card is greyed out with the reason, rather than offering a button whose
// only outcome is a 422 from the API. The password never leaves this function.
function wireClientCertIssue(name, hasKey) {
  const card = $('#issue-card');
  if (!card) return;
  gateControl(card, hasKey, 'No signing key configured on this client CA - set one under "Signing key" and save first.');
  if (!hasKey) return;
  const btn = $('#iss-btn');
  btn.addEventListener('click', async () => {
    const cn = $('#iss-cn').value.trim();
    if (!cn) { toast('Common name required', 'Enter the certificate common name.', 'err'); return; }
    const pw = $('#iss-pw').value;
    if (!pw) { toast('Password required', 'The PKCS#12 bundle must be password-protected.', 'err'); return; }
    if (pw.length < P12_MIN_PASSWORD) { toast('Password too short', `Use at least ${P12_MIN_PASSWORD} characters: the legacy PKCS#12 encoder barely stretches it, so a short password is cheap to crack offline once the file leaves gpm.`, 'err'); return; }
    const days = parseInt($('#iss-days').value, 10);
    if (isNaN(days) || days < 1 || days > 3650) { toast('Validity out of range', 'Validity must be between 1 and 3650 days.', 'err'); return; }
    const sans = $('#iss-sans').value.split(',').map((v) => v.trim()).filter(Boolean);
    const body = { commonName: cn, validityDays: days, password: pw };
    if (sans.length) body.sans = sans;

    btn.disabled = true;
    try {
      const file = await downloadP12('/api/client-cas/' + encodeURIComponent(name) + '/issue', body, cn);
      $('#iss-pw').value = '';
      toast('Certificate issued', `Downloaded ${file}. It is not stored - keep the file and its password.`, 'ok');
      route();
    } catch (e) { toastErr(e); } finally { btn.disabled = false; }
  });
}

// downloadP12 POSTs an issuance/renewal request and saves the PKCS#12 response as
// a file. The bundle is binary, so it cannot go through api() (which parses the
// body as text/JSON); this carries the same CSRF token and credentials the helper
// does. It returns the filename actually saved.
async function downloadP12(path, body, cn) {
  const res = await fetch(path, {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(body),
  });
  redirectOn401(res);
  if (!res.ok) {
    let msg = `Request failed (${res.status})`;
    try { const j = JSON.parse(await res.text()); if (j && j.error) msg = j.error; } catch (e) { /* keep the status message */ }
    throw new Error(msg);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filenameFromDisposition(res.headers.get('Content-Disposition')) || (cn + '.p12');
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  return a.download;
}

// Pulls the filename out of a Content-Disposition header, or returns '' so the
// caller can fall back. The server already sanitizes it; this only unquotes.
function filenameFromDisposition(cd) {
  const m = /filename="([^"]*)"/.exec(cd || '');
  return m ? m[1] : '';
}

// ---------- UPSTREAM GROUP EDITOR ----------
async function upstreamGroupEditor(c, name) {
  const meta = SECTION_META.upstreams; const isNew = !name;
  const seed = isNew ? takeCloneSeed('upstreams') : null;
  const [objR, healthR] = await Promise.all([
    isNew ? Promise.resolve({ data: {} }) : api('/api/upstream-groups/' + encodeURIComponent(name)),
    isNew ? Promise.resolve({ data: {} }) : api('/api/upstream-health').catch(() => ({ data: {} })),
  ]);
  const o = seed ? seed.data : (objR.data || {});
  const hc = o.healthCheck || {};
  const health = {};
  arr((healthR.data || {})[name]).forEach((u) => { health[u.upstream] = u.healthy; });
  const healthChip = (u) => {
    // Match upstreamLabel() server-side: net.JoinHostPort brackets IPv6 hosts.
    const hostPart = (u.host || '').indexOf(':') !== -1 ? `[${u.host}]` : u.host;
    const label = `${u.scheme || 'http'}://${hostPart}:${u.port}`;
    if (!(label in health)) return '';
    // up is the quiet state (dot + text only); down gets the filled/bordered
    // pill, so a bad backend is what catches the eye.
    return health[label]
      ? '<span class="chip flat ok" style="flex:0 0 auto"><span class="dot ok"></span>up</span>'
      : '<span class="chip err" style="flex:0 0 auto"><span class="dot err"></span>down</span>';
  };
  c.innerHTML = editorHead('upstreams', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew, seed && seed.origName + '-copy')}
    <div class="card form-section"><p class="section-label">Upstreams</p>
      <div id="ed-ups" data-hint="upstreamGroup.upstreams"></div>
      <button class="btn ghost sm" id="ed-addup" type="button" style="margin-top:6px">${ICON.plus}Add upstream</button>
      <div class="hint" style="margin-top:6px">Weight (1-256, default 1) sets the relative share for round-robin and least-connections; failover and ip-hash ignore it. Requests are retried on another upstream only when the connection itself fails - never after the request was sent.</div>
    </div>
    <div class="card form-section"><p class="section-label">Policy</p>
      <div class="field-group"><label>Load distribution</label>
        <select class="field mono" id="ed-policy" data-hint="upstreamGroup.policy">
          ${enumOptions('loadBalance', ['', 'round-robin', 'least-connections', 'ip-hash'], o.policy || '')}
        </select>
        <div class="hint">Unhealthy upstreams always drop to the end of the try-order regardless of policy.</div>
      </div>
      <div class="inline-fields" style="margin-top:10px">
        <div class="field-group"><label>Sticky sessions TTL</label>
          <input class="field mono" id="ed-sticky-ttl" data-hint="upstreamGroup.stickiness.ttl" value="${esc((o.stickiness && o.stickiness.ttl) || '')}" placeholder="off (e.g. 30m, 12h, 3d)" />
        </div>
        <div class="field-group"><label>Sticky cookie name</label>
          <input class="field mono" id="ed-sticky-cookie" data-hint="upstreamGroup.stickiness.cookie" value="${esc((o.stickiness && o.stickiness.cookie) || '')}" placeholder="gpm-sticky-<name>" />
        </div>
      </div>
    </div>
  </div><div class="stack">
    <div class="card form-section"><p class="section-label">Health check</p>
      <div class="field-group"><label>HTTP probe path</label>
        <input class="field mono" id="ed-hc-path" data-hint="upstreamGroup.healthCheck.path" value="${esc(hc.path || '')}" placeholder="(blank = TCP connect probe)" />
        <div class="hint">Any response below 500 counts as alive - the probe checks the entry point, not the application behind it. The health-check path is used as written and is <b>not</b> prefixed with the member's base path.</div>
      </div>
      <div class="inline-fields" style="margin-top:8px">
        <div class="field-group"><label>Interval (s)</label><input class="field mono" id="ed-hc-interval" data-hint="upstreamGroup.healthCheck.intervalSeconds" type="number" min="1" max="3600" value="${esc(hc.intervalSeconds || '')}" placeholder="5" /></div>
        <div class="field-group"><label>Timeout (s)</label><input class="field mono" id="ed-hc-timeout" data-hint="upstreamGroup.healthCheck.timeoutSeconds" type="number" min="1" max="60" value="${esc(hc.timeoutSeconds || '')}" placeholder="3" /></div>
      </div>
      <div class="inline-fields" style="margin-top:8px">
        <div class="field-group"><label>Rise</label><input class="field mono" id="ed-hc-rise" data-hint="upstreamGroup.healthCheck.rise" type="number" min="1" max="10" value="${esc(hc.rise || '')}" placeholder="2" /></div>
        <div class="field-group"><label>Fall</label><input class="field mono" id="ed-hc-fall" data-hint="upstreamGroup.healthCheck.fall" type="number" min="1" max="10" value="${esc(hc.fall || '')}" placeholder="2" /></div>
      </div>
    </div>
  </div></div>` + saveBar('upstreams', isNew, meta.addLabel);

  const upsWrap = $('#ed-ups');
  const upCtls = [];
  let upSeq = 0;
  function upRow(u) {
    u = u || {};
    const p = 'ug' + (++upSeq);
    const div = document.createElement('div');
    div.className = 'up-block';
    // The two escape hatches are behind a disclosure so a plain member row stays
    // one line - opened automatically when either already holds a value, so a
    // configured field is never hidden from someone scanning the page.
    const hasExtra = !!(u.path || u.hostHeader);
    div.innerHTML = `
      <div class="loc-row">
        <select class="field mono up-scheme" data-hint="upstreamGroup.upstreams.scheme" style="flex:0 0 90px" aria-label="Upstream scheme"><option value="http"${u.scheme === 'https' ? '' : ' selected'}>http</option><option value="https"${u.scheme === 'https' ? ' selected' : ''}>https</option></select>
        <input class="field mono up-host" data-hint="upstreamGroup.upstreams.host" style="flex:2 1 140px" value="${esc(u.host || '')}" placeholder="10.0.0.5" aria-label="Upstream host" />
        <input class="field mono up-port" data-hint="upstreamGroup.upstreams.port" type="number" style="flex:0 0 90px" value="${esc(u.port != null ? u.port : '')}" placeholder="80" aria-label="Upstream port" />
        <input class="field mono up-weight" data-hint="upstreamGroup.upstreams.weight" type="number" min="1" max="256" style="flex:0 0 80px" value="${esc(u.weight || '')}" placeholder="w:1" aria-label="Weight" />
        ${u.host ? healthChip(u) : ''}
        <button class="icon-btn up-del" type="button" aria-label="Remove upstream">${ICON.x}</button>
      </div>
      <details class="fold sub-fold"${hasExtra ? ' open' : ''}><summary><span class="fold-sum">Advanced${hasExtra ? ' - ' + esc([u.path ? 'base path ' + u.path : '', u.hostHeader ? 'Host ' + u.hostHeader : ''].filter(Boolean).join(', ')) : ''}</span></summary>
        ${upstreamExtraHtml(p, u)}
      </details>`;
    upsWrap.appendChild(div);
    const ctl = { div, _orig: u, extra: wireUpstreamExtra(p) };
    upCtls.push(ctl);
    div.querySelector('.up-del').addEventListener('click', () => {
      div.remove();
      const i = upCtls.indexOf(ctl);
      if (i >= 0) upCtls.splice(i, 1);
    });
  }
  const initial = arr(o.upstreams);
  (initial.length ? initial : [{}]).forEach(upRow);
  $('#ed-addup').addEventListener('click', () => upRow({}));

  wireEditor('upstreams', 'upstream-groups', meta, isNew, name || o.name, o, () => {
    const ups = [];
    let bad = false;
    let aborted = false;
    for (const ctl of upCtls) {
      if (bad || aborted) break;
      const row = ctl.div;
      const host = row.querySelector('.up-host').value.trim();
      const port = parseInt(row.querySelector('.up-port').value, 10);
      if (!host && isNaN(port)) continue; // fully empty row: skip
      if (!host || isNaN(port)) { bad = true; break; }
      // Merged over the stored member for the same reason as the host editor's
      // upstream: path and hostHeader must survive a save that does not touch them.
      const up = Object.assign({}, ctl._orig, { scheme: row.querySelector('.up-scheme').value, host, port });
      const weight = parseInt(row.querySelector('.up-weight').value, 10);
      if (!isNaN(weight) && weight > 0) up.weight = weight; else delete up.weight;
      const extra = ctl.extra.get(host + ':' + port);
      if (!extra) { aborted = true; break; }
      delete up.path; delete up.hostHeader;
      Object.assign(up, extra);
      ups.push(up);
    }
    if (aborted) return null;
    if (bad) { toast('Upstream incomplete', 'Every upstream needs a host and a port.', 'err'); return null; }
    if (!ups.length) { toast('Upstream required', 'Add at least one upstream.', 'err'); return null; }
    const body = { upstreams: ups };
    const policy = $('#ed-policy').value; if (policy) body.policy = policy;
    const stickyTTL = $('#ed-sticky-ttl').value.trim();
    if (stickyTTL) {
      body.stickiness = { ttl: stickyTTL };
      const stickyCookie = $('#ed-sticky-cookie').value.trim();
      if (stickyCookie) body.stickiness.cookie = stickyCookie;
    }
    const hcOut = {};
    const path = $('#ed-hc-path').value.trim(); if (path) hcOut.path = path;
    const iv = parseInt($('#ed-hc-interval').value, 10); if (!isNaN(iv) && iv > 0) hcOut.intervalSeconds = iv;
    const to = parseInt($('#ed-hc-timeout').value, 10); if (!isNaN(to) && to > 0) hcOut.timeoutSeconds = to;
    const rise = parseInt($('#ed-hc-rise').value, 10); if (!isNaN(rise) && rise > 0) hcOut.rise = rise;
    const fall = parseInt($('#ed-hc-fall').value, 10); if (!isNaN(fall) && fall > 0) hcOut.fall = fall;
    if (Object.keys(hcOut).length) body.healthCheck = hcOut;
    return body;
  });
  wireCloneButton('upstreams', o);
}

const EDITORS = {
  redirects: redirectEditor, streams: streamEditor, parked: parkedEditor, dns: dnsEditor,
  clientcas: clientCAEditor,
  identity: idpEditor, access: accessEditor, middleware: middlewareEditor,
  upstreams: upstreamGroupEditor,
};

// ---------- API TOKENS ----------
// Fallback scope subjects for the create form. The live list comes from
// capabilities.scopeSubjects (model.ScopePlurals) - this copy only covers a
// capabilities fetch that failed, and is deliberately not the source of truth:
// it silently went stale when ingress-discovery was added, leaving no way to
// mint a token for it.
const TOKEN_SUBJECTS_FALLBACK = [
  'proxy-hosts', 'redirect-hosts', 'stream-hosts', 'parked-hosts', 'certificates',
  'client-cas', 'dns-providers', 'identity-providers', 'upstream-groups',
  'access-lists', 'middlewares', 'api-tokens', 'settings', 'dns-sync',
  'ingress-discovery',
];

function tokenSubjects() {
  const fromCaps = state.capabilities && state.capabilities.scopeSubjects;
  return Array.isArray(fromCaps) && fromCaps.length ? fromCaps : TOKEN_SUBJECTS_FALLBACK;
}

// Groups subjects into labelled sections for the scope table below. Purely a
// display grouping - tokenSubjects() above stays the source of truth for what
// subjects exist, so a subject this grouping doesn't know about yet (e.g. a
// new one added to model.ScopePlurals before this list catches up) still
// renders, filed under "Other" instead of silently vanishing.
const SCOPE_GROUPS = [
  { label: 'Hosts', subjects: ['proxy-hosts', 'redirect-hosts', 'stream-hosts', 'parked-hosts'] },
  { label: 'Trust & auth', subjects: ['certificates', 'client-cas', 'identity-providers', 'access-lists', 'middlewares'] },
  { label: 'Routing', subjects: ['upstream-groups', 'dns-providers'] },
  { label: 'Operations', subjects: ['settings', 'dns-sync', 'ingress-discovery', 'api-tokens', 'metrics'] },
];

// Subjects with no write action at all, so the scope table renders no write
// checkbox for them. "metrics" is the only one: model.ScopeMetricsRead is the
// sole scope /metrics checks (server.go's GET /metrics handler), there is no
// metrics:write endpoint. Mirrors internal/model/apitoken.go.
const SCOPE_READONLY = new Set(['metrics']);

// Buckets tokenSubjects() into SCOPE_GROUPS order, appending an "Other"
// section for anything not covered above.
function groupedScopeSubjects() {
  const all = tokenSubjects();
  const placed = new Set();
  const groups = SCOPE_GROUPS.map((g) => {
    const subjects = g.subjects.filter((s) => all.includes(s));
    subjects.forEach((s) => placed.add(s));
    return { label: g.label, subjects };
  }).filter((g) => g.subjects.length);
  const rest = all.filter((s) => !placed.has(s));
  if (rest.length) groups.push({ label: 'Other', subjects: rest });
  return groups;
}

// Subjects whose endpoints require Full admin however the per-subject boxes are
// ticked, so the boxes are greyed out instead of granting nothing:
//   api-tokens - a token that could mint tokens could widen itself,
//   settings (write) - a settings write can aim dnsSync/webhooks at an attacker
//     with a ${ENV:...} credential and trigger the delivery that resolves it,
//     and can rewrite adminAuth, so it is admin-equivalent.
const ADMIN_ONLY_SCOPES = {
  'api-tokens': { read: true, write: true, why: 'Token management needs Full admin.' },
  'settings': { read: false, write: true, why: 'Writing settings is admin-equivalent, so it needs Full admin. settings:read still allows GET /api/settings.' },
};

function tokenExpiryLabel(t) {
  if (!t.expiresAt) return 'never';
  const d = new Date(t.expiresAt);
  if (isNaN(d.getTime())) return esc(t.expiresAt);
  return fmtTime(t.expiresAt) + (d.getTime() < Date.now() ? ' (expired)' : '');
}

// One-time reveal. The secret exists only in this response, so it is shown in a
// blocking panel with a copy button and never re-fetched.
function revealToken(secret) {
  const wrap = document.createElement('div');
  wrap.className = 'card form-section';
  wrap.style.cssText = 'border-color:var(--accent);margin-bottom:16px';
  wrap.innerHTML = `
    <p class="section-label">New token - shown once</p>
    <p class="muted" style="font-size:11.5px;margin:0 0 10px">Copy this now. Only its SHA-256 digest is stored, so it cannot be shown again. Lost it? Rotate the token.</p>
    <div class="loc-row">
      <input class="field mono" id="tok-reveal" readonly value="${esc(secret)}" aria-label="New API token" style="flex:3 1 260px" />
      <button class="btn primary sm" id="tok-copy" type="button">Copy</button>
      <button class="btn ghost sm" id="tok-dismiss" type="button">Done</button>
    </div>`;
  const content = $('#content');
  content.insertBefore(wrap, content.firstChild);
  window.scrollTo(0, 0);
  $('#tok-copy').addEventListener('click', async () => {
    const inp = $('#tok-reveal');
    inp.select();
    try { await navigator.clipboard.writeText(secret); toast('Copied', 'Token copied to the clipboard.', 'ok'); }
    catch (e) { toast('Copy manually', 'Select the field and copy it.', 'err'); }
  });
  $('#tok-dismiss').addEventListener('click', () => wrap.remove());
}

async function viewTokens(c) {
  // api-tokens is admin-scope for READS as well as writes, so the "user" role
  // gets a 403 on GET /api/api-tokens. The nav entry is hidden for that role
  // (applyNavGating), but a bookmark or a typed #/tokens still lands here: say
  // what the role cannot do instead of issuing the fetch and rendering its
  // generic "could not load this view" error.
  if (isRoleReadOnly()) {
    c.innerHTML = viewHead('API Tokens',
      'Non-interactive credentials for scripts and CI.') +
      emptyState('Your role cannot manage API tokens',
        'API tokens are readable and writable only with the admin role. Ask an administrator to mint one for you.');
    return;
  }
  // The daemon publishes capabilities.apiTokens.enabled; without a wired token
  // source the whole page cannot work, so say so instead of offering a form
  // whose every save would fail.
  await loadCapabilities();
  if (!hasCapability('apiTokens.enabled')) {
    c.innerHTML = viewHead('API Tokens',
      'Non-interactive credentials for scripts and CI.') +
      emptyState('API tokens are not available',
        'This deployment did not wire an API token source, so tokens cannot be minted or used.');
    return;
  }
  const tokens = arr((await api('/api/api-tokens')).data);
  const scopeChips = (scopes) => {
    const list = arr(scopes);
    if (list.length === 1 && list[0] === 'admin') return '<span class="chip brand">Full admin</span>';
    if (!list.length) return '<span class="faint">none</span>';
    return list.map((s) => `<span class="chip">${esc(s)}</span>`).join(' ');
  };
  const rows = tokens.map((t) => `
    <tr data-name="${esc(t.name)}" data-blob="${esc([t.name, t.displayName, arr(t.scopes).join(' '), t.disabled ? 'disabled' : ''].join(' ').toLowerCase())}">
      <td><span class="mono host">${esc(t.name)}</span>${t.disabled ? ' <span class="chip warn">disabled</span>' : ''}</td>
      <td><div class="chip-row">${scopeChips(t.scopes)}</div></td>
      <td class="mono faint" style="white-space:nowrap">${t.createdAt ? esc(fmtTime(t.createdAt)) : ''}</td>
      <td class="mono" style="white-space:nowrap">${tokenExpiryLabel(t)}</td>
      <td class="mono faint" style="white-space:nowrap">${t.lastUsed ? esc(fmtTime(t.lastUsed)) : 'never'}</td>
      <td>
        <div style="display:flex;gap:8px;justify-content:flex-end">
          <button class="btn ghost sm tok-rotate" data-name="${esc(t.name)}" type="button">Rotate</button>
          <button class="btn ghost sm danger tok-del" data-name="${esc(t.name)}" type="button">Delete</button>
        </div>
      </td>
    </tr>`).join('');

  c.innerHTML = viewHead('API Tokens',
    'Non-interactive credentials for scripts and CI. Send as Authorization: Bearer gpm_...; scopes limit what each token can reach.') +
    aboutPageHtml('page.apiTokens') +
    `<div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Create token</p>
      <div class="inline-fields">
        <div class="field-group"><label>Name</label><input class="field mono" id="tok-name" data-hint="apiToken.name" placeholder="ci-deploy" /></div>
        <div class="field-group field-narrow"><label>Expires</label><input class="field mono" id="tok-expires" data-hint="apiToken.expiresAt" type="date" /></div>
      </div>
      <div class="field-group" style="margin-top:10px">
        <label>Scopes</label>
        <div class="toggle-line" style="padding-top:4px">
          <div class="tl-text"><div class="nm">Full admin</div><div class="ds">Every endpoint, including token management, restore and whole-config revert</div></div>
          ${switchHtml('tok-admin', false, 'Full admin', 'apiToken.scopes.admin')}
        </div>
        <div class="table-wrap scope-table-wrap" id="tok-scopes" data-hint="apiToken.scopes" style="margin-top:8px">
          <table class="scope-table">
            <thead>
              <tr>
                <th>Subject</th>
                <th class="scope-check"><label class="check-item scope-all"><input type="checkbox" id="tok-read-all" aria-label="Toggle all read" />Read</label></th>
                <th class="scope-check"><label class="check-item scope-all"><input type="checkbox" id="tok-write-all" aria-label="Toggle all write" />Write</label></th>
              </tr>
            </thead>
            <tbody>
              ${groupedScopeSubjects().map((g) => `
                <tr class="scope-group"><td colspan="3">${esc(g.label)}</td></tr>
                ${g.subjects.map((p) => `
                  <tr>
                    <td class="mono">${esc(p)}</td>
                    <td class="scope-check"><input type="checkbox" class="tok-read" data-p="${esc(p)}" aria-label="${esc(p)} read" /></td>
                    <td class="scope-check">${SCOPE_READONLY.has(p) ? '<span class="faint" aria-hidden="true">-</span>' : `<input type="checkbox" class="tok-write" data-p="${esc(p)}" aria-label="${esc(p)} write" />`}</td>
                  </tr>`).join('')}`).join('')}
            </tbody>
          </table>
        </div>
        <div class="hint" style="margin-top:8px">Write implies read - checking write ticks and locks read. <span class="mono">api-tokens</span>, <em>writing</em> <span class="mono">settings</span>, restore, whole-config revert and the pprof endpoints need Full admin.</div>
      </div>
      <div class="row-between" style="margin-top:12px">
        <span></span>
        <button class="btn primary" id="tok-create" type="button">${ICON.plus}Create token</button>
      </div>
    </div>` +
    (tokens.length ? `<div class="toolbar">
        <div class="search">${ICON.search}<input class="field mono" id="tokFilter" placeholder="filter: name, scope..." aria-label="Filter tokens" /></div>
      </div>
      <div class="table-wrap"><table>
        <thead><tr><th>Name</th><th>Scopes</th><th>Created</th><th>Expires</th><th>Last used</th><th></th></tr></thead>
        <tbody id="tokRows">${rows}</tbody>
      </table></div>`
      : emptyState('No API tokens yet',
        'Create one above with only the scopes the script needs - write implies read, so a deploy job usually wants proxy-hosts:write and nothing else.'));

  const tokFilter = $('#tokFilter');
  if (tokFilter) {
    tokFilter.addEventListener('input', () => {
      const q = tokFilter.value.trim().toLowerCase();
      $$('#tokRows tr[data-blob]').forEach((tr) => {
        tr.style.display = (!q || tr.dataset.blob.indexOf(q) !== -1) ? '' : 'none';
      });
    });
  }

  const adminSw = $('#tok-admin');
  const scopeGrid = $('#tok-scopes');

  // Write implies read: checking write auto-selects the read box. Read stays
  // an ordinary checkbox (unchecking write leaves it as is).
  scopeGrid.querySelectorAll('.tok-write').forEach((w) => {
    w.addEventListener('change', () => {
      const r = scopeGrid.querySelector(`.tok-read[data-p="${w.dataset.p}"]`);
      if (r && w.checked) r.checked = true;
    });
  });
  // Header "select all" checkboxes flip every enabled box in their column and
  // fire a change event so the write-implies-read wiring above stays in sync.
  const wireScopeAll = (id, cls) => {
    const all = $('#' + id);
    if (!all) return;
    all.addEventListener('change', () => {
      scopeGrid.querySelectorAll('.' + cls + ':not(:disabled)').forEach((box) => {
        box.checked = all.checked;
        box.dispatchEvent(new Event('change', { bubbles: true }));
      });
    });
  };
  wireScopeAll('tok-read-all', 'tok-read');
  wireScopeAll('tok-write-all', 'tok-write');

  const syncAdmin = () => {
    const perSubject = !isOn('tok-admin');
    gateControl(scopeGrid, perSubject, 'Full admin already covers every scope.');
    // Re-disable the boxes that never grant anything, after the grid-wide gate
    // has just re-enabled everything.
    Object.entries(ADMIN_ONLY_SCOPES).forEach(([p, rule]) => {
      ['read', 'write'].forEach((verb) => {
        if (!rule[verb]) return;
        const box = scopeGrid.querySelector(`.tok-${verb}[data-p="${p}"]`);
        if (!box) return;
        box.checked = false;
        box.disabled = true;
        box.title = rule.why;
      });
    });
  };
  adminSw.addEventListener('switchchange', syncAdmin);
  syncAdmin();

  function collectScopes() {
    if (isOn('tok-admin')) return ['admin'];
    const out = [];
    $$('#tok-scopes .tok-write').forEach((i) => { if (i.checked) out.push(i.dataset.p + ':write'); });
    $$('#tok-scopes .tok-read').forEach((i) => {
      // write already implies read; don't send a redundant pair.
      const w = scopeGrid.querySelector(`.tok-write[data-p="${i.dataset.p}"]`);
      if (i.checked && !(w && w.checked)) out.push(i.dataset.p + ':read');
    });
    return out;
  }

  $('#tok-create').addEventListener('click', async () => {
    const nm = $('#tok-name').value.trim();
    if (!nm) { toast('Name required', 'Enter a token name.', 'err'); return; }
    const scopes = collectScopes();
    if (!scopes.length) { toast('Scope required', 'Pick at least one scope, or Full admin.', 'err'); return; }
    const body = { scopes };
    const exp = $('#tok-expires').value;
    if (exp) body.expiresAt = new Date(exp + 'T23:59:59Z').toISOString();
    const btn = $('#tok-create'); btn.disabled = true;
    try {
      const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'PUT', body });
      toastSaved(r.commit); refreshHeadSha();
      const secret = r.data && r.data.token;
      await viewTokens(c);
      if (secret) revealToken(secret);
    } catch (e) { toastErr(e); btn.disabled = false; }
  });

  $$('.tok-rotate').forEach((b) => {
    b.addEventListener('click', async () => {
      const nm = b.dataset.name;
      if (!confirm(`Rotate token "${nm}"? The current secret stops working immediately, and cannot be brought back - reverting an API token from history is refused so a rotation always means revocation.`)) return;
      b.disabled = true;
      const cur = tokens.find((t) => t.name === nm) || {};
      const body = { scopes: arr(cur.scopes) };
      if (cur.expiresAt) body.expiresAt = cur.expiresAt;
      if (cur.disabled) body.disabled = true;
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm) + '?rotate=1', { method: 'PUT', body });
        toastSaved(r.commit); refreshHeadSha();
        const secret = r.data && r.data.token;
        await viewTokens(c);
        if (secret) revealToken(secret);
      } catch (e) { toastErr(e); b.disabled = false; }
    });
  });

  $$('.tok-del').forEach((b) => {
    b.addEventListener('click', async () => {
      const nm = b.dataset.name;
      if (!confirm(`Delete API token "${nm}"? Anything using it stops working immediately.`)) return;
      b.disabled = true;
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'DELETE' });
        toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'token removed', 'ok', { html: true });
        refreshHeadSha(); await viewTokens(c);
      } catch (e) { toastErr(e); b.disabled = false; }
    });
  });
}

// ---------- ACCESS LOGS ----------
async function viewLogs(c) {
  const data = (await api('/api/logs')).data || {};
  const enabled = !!data.enabled;
  const entries = arr(data.entries);
  const statusClass = (s) => (s >= 500 ? 'err' : s >= 400 ? 'warn' : 'ok');
  const rows = entries.map((e) => `
    <tr data-blob="${esc([e.method, e.host, e.path, e.status, e.client].join(' ').toLowerCase())}">
      <td class="mono faint" style="white-space:nowrap">${esc(fmtTime(e.time))}</td>
      <td class="mono">${esc(e.method || '')}</td>
      <td class="mono">${esc(e.host || '')}</td>
      <td class="mono" style="max-width:340px;overflow:hidden;text-overflow:ellipsis">${esc(e.path || '')}</td>
      <td><span class="chip ${statusClass(e.status)}">${esc(e.status)}</span></td>
      <td class="mono">${esc(e.durMs)}ms</td>
      <td class="mono faint">${esc(e.client || '')}</td>
    </tr>`).join('');

  c.innerHTML = `
    <div class="row-between view-head">
      <div><h2>Access Logs</h2><p>Most recent proxied requests, newest first (in-memory buffer).</p>${aboutPageHtml('page.accessLogs')}</div>
      <div style="display:flex;gap:10px">
        <button class="btn" id="logsToggle" type="button">${enabled ? 'Disable capture' : 'Enable capture'}</button>
        <button class="btn" id="logsRefresh" type="button">${ICON.history}Refresh</button>
      </div>
    </div>
    ${entries.length ? `<div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="logFilter" placeholder="filter: host, path, method, status, client..." aria-label="Filter log entries" /></div>
    </div>` : ''}
    ${enabled ? '' : `<div class="card" style="margin-bottom:14px"><div class="hint">Request capture is <b>off</b> - the off path adds zero per-request overhead. "Enable capture" switches it on live, until the next restart; start gpm with <span class="mono">--access-log</span> (or <span class="mono">GPM_ACCESS_LOG=1</span>) to make it the startup default.</div></div>`}
    <div class="table-wrap">
      <table>
        <thead><tr><th>Time</th><th>Method</th><th>Host</th><th>Path</th><th>Status</th><th>Duration</th><th>Client</th></tr></thead>
        <tbody id="logRows">${rows || `<tr><td colspan="7" class="muted" style="font-size:13px;padding:14px">${enabled ? 'No requests captured yet.' : 'Nothing to show while access logging is off.'}</td></tr>`}</tbody>
      </table>
    </div>`;

  const logFilter = $('#logFilter');
  if (logFilter) {
    // Matches the values behind the row (method, host, path, status, client),
    // not the rendered cells - a chip label is not a search index.
    logFilter.addEventListener('input', () => {
      const q = logFilter.value.trim().toLowerCase();
      $$('#logRows tr[data-blob]').forEach((tr) => {
        tr.style.display = (!q || tr.dataset.blob.indexOf(q) !== -1) ? '' : 'none';
      });
    });
  }
  $('#logsRefresh').addEventListener('click', () => viewLogs(c));
  $('#logsToggle').addEventListener('click', async () => {
    const btn = $('#logsToggle');
    btn.disabled = true;
    try {
      await api('/api/logs', { method: 'PUT', body: { enabled: !enabled } });
      toast(enabled ? 'Capture disabled' : 'Capture enabled', 'runtime only - a restart reverts to the --access-log flag', 'ok');
      await viewLogs(c);
    } catch (e) { toastErr(e); btn.disabled = false; }
  });
}

// ---------- HISTORY ----------
async function viewHistory(c) {
  const items = arr((await api('/api/history')).data);
  c.innerHTML = `
    <div class="view-head">
      <h2>History</h2>
      <p>Every change is a git commit. Reviewable, attributable, and reversible.</p>
    </div>
    ${aboutPageHtml('page.history')}
    <div class="card" style="margin-bottom:16px">
      <p class="section-label">Backup &amp; restore</p>
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <button class="btn" id="backupBtn">Download backup</button>
        <button class="btn" id="restoreBtn">Restore from archive...</button>
        <input type="file" id="restoreFile" accept=".gz,.tar.gz,application/gzip" style="display:none" />
        <span class="hint">A portable archive of the whole config. Restore replaces everything as one commit.</span>
      </div>
    </div>
    ${items.length ? `<div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="histFilter" placeholder="filter: message, author, commit..." aria-label="Filter history" /></div>
    </div>` : ''}
    <div class="card">
      ${items.length ? `<div class="timeline">${items.map((h, i) => {
        const obj = parseObjectCommit(h.message);
        // Buttons, not spans: these commit a config change, so they have to be
        // tabbable and operable from the keyboard like every other action here.
        const scoped = obj ? `<button type="button" class="revert" data-revert-obj="${esc(h.hash)}" data-obj-plural="${esc(obj.plural)}" data-obj-name="${esc(obj.name)}" data-obj-kind="${esc(obj.kind)}" title="Revert only ${esc(obj.kind)} &quot;${esc(obj.name)}&quot; to this version; every other object is left untouched">revert this object</button>` : '';
        const whole = i === 0
          ? '<span class="revert disabled" title="Already the current config">current</span>'
          : `<button type="button" class="revert" data-revert="${esc(h.hash)}" title="Revert the ENTIRE config (every object) to this commit">revert entire config</button>`;
        return `<div class="tl-item" data-blob="${esc([h.message, h.author, h.email, h.hash].join(' ').toLowerCase())}">
          <div class="tl-meta">${esc(fmtTime(h.when))} &middot; ${esc(h.author || 'unknown')}${h.email ? ` <span class="muted">&lt;${esc(h.email)}&gt;</span>` : ''}</div>
          <div class="tl-msg">${esc(h.message || '(no message)')}</div>
          <div class="tl-actions"><span class="sha">${esc(shortSha(h.hash))}</span>${scoped}${whole}</div>
        </div>`;
      }).join('')}</div>` : '<div class="muted" style="font-size:13px">No commits yet.</div>'}
    </div>`;

  const histFilter = $('#histFilter');
  if (histFilter) {
    histFilter.addEventListener('input', () => {
      const q = histFilter.value.trim().toLowerCase();
      $$('.timeline .tl-item[data-blob]').forEach((el) => {
        el.style.display = (!q || el.dataset.blob.indexOf(q) !== -1) ? '' : 'none';
      });
    });
  }
  $('#backupBtn').addEventListener('click', () => { window.location.href = '/api/backup'; });
  const fileInput = $('#restoreFile');
  $('#restoreBtn').addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', async () => {
    const f = fileInput.files && fileInput.files[0];
    if (!f) return;
    const ok = await confirmModal({
      title: 'Restore from archive?',
      body: `<p>This <b>replaces the entire current configuration</b> with the contents of <span class="mono">${esc(f.name)}</span> - every proxy host, certificate, access list, middleware and setting.</p>`
        + '<p>Anything created since that archive was taken is removed. It is recorded as one commit, so it can be reverted from this page.</p>',
      confirmLabel: 'Restore config',
      typed: 'RESTORE',
    });
    if (!ok) { fileInput.value = ''; return; }
    try {
      const res = await fetch('/api/restore', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': csrfToken }, body: f });
      redirectOn401(res);
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error((data && data.error) || `Restore failed (${res.status})`);
      toast('Restored', data && data.commit ? `committed <span class="sha">${esc(shortSha(data.commit))}</span>` : 'configuration restored', 'ok', { html: true });
      await viewHistory(c);
    } catch (e) { toastErr(e); } finally { fileInput.value = ''; }
  });
  c.querySelectorAll('[data-revert]').forEach((el) => {
    el.addEventListener('click', async () => {
      const hash = el.getAttribute('data-revert');
      const ok = await confirmModal({
        title: 'Revert the entire config?',
        body: `<p><b>Every object</b> is reset to how it was at <span class="sha mono">${esc(shortSha(hash))}</span>. Anything created after that commit is <b>removed</b>.</p>`
          + '<p>Recorded as a new commit, so this page can undo it.</p>',
        confirmLabel: 'Revert everything',
        typed: 'REVERT',
      });
      if (!ok) return;
      try {
        const r = await api('/api/revert', { method: 'POST', body: { hash } });
        toast('Reverted', r.data && r.data.commit ? `committed <span class="sha">${esc(shortSha(r.data.commit))}</span>` : 'config reverted', 'ok', { html: true });
        await viewHistory(c);
      } catch (e) { toastErr(e); }
    });
  });
  c.querySelectorAll('[data-revert-obj]').forEach((el) => {
    el.addEventListener('click', async () => {
      const hash = el.getAttribute('data-revert-obj');
      const plural = el.getAttribute('data-obj-plural');
      const name = el.getAttribute('data-obj-name');
      const kind = el.getAttribute('data-obj-kind');
      const ok = await confirmModal({
        title: `Revert this ${kind.toLowerCase()}?`,
        body: `<p><b>${esc(kind)} "${esc(name)}"</b> is reset to how it was at <span class="sha mono">${esc(shortSha(hash))}</span>.</p>`
          + '<p>Every other object is left exactly as it is, and this is recorded as a new commit - so this page can undo it.</p>',
        confirmLabel: 'Revert this object',
        danger: false,
      });
      if (!ok) return;
      try {
        const r = await api('/api/' + plural + '/' + encodeURIComponent(name) + '/revert', { method: 'POST', body: { hash } });
        toast('Reverted', r.data && r.data.commit ? `${esc(kind)} "${esc(name)}" committed <span class="sha">${esc(shortSha(r.data.commit))}</span>` : `${esc(kind)} "${esc(name)}" reverted`, 'ok', { html: true });
        await viewHistory(c);
      } catch (e) { toastErr(e); }
    });
  });
}

// ---------- ERROR PAGES ----------
// A top-level section of its own, because error pages are a body of content an
// operator iterates on (write a template, reload, look at it) rather than a
// switch they set once - the Settings page is for the latter. It still edits
// `settings.errorPages`: the config schema is unchanged, so a git-authored
// settings.yaml and this page are the same field. A ProxyHost's own errorPages
// override stays in the host editor, next to the rest of that host's config.
async function viewErrorPages(c) {
  const s = (await api('/api/settings')).data || {};
  const ep = s.errorPages || {};

  c.innerHTML = viewHead('Error Pages',
    'Custom HTML for the errors gpm generates itself - a dead upstream, a denial, a rate limit. Applies to every host unless a host overrides it in its own editor.')
    + aboutPageHtml('page.errorPages')
    + `<div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">What these replace</p>
      <p class="muted" style="font-size:11.5px;margin:0">Errors <b>gpm itself</b> generates: an unreachable or timed-out upstream (502/504), an access-list, guard or bouncer denial, a rate limit (429), an auth-middleware refusal (401/403, and the 502/503 an unavailable auth backend or an uncompilable middleware produces), and a parked host. The upstream's own error response passes through untouched unless you list its status below. Sign-in redirects and pages served by an identity provider are never replaced.</p>
    </div>
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Templates</p>
      <div class="field-group">
        <label>Templates directory</label>
        <input class="field mono" id="errp-dir" data-hint="settings.errorPages.dir" value="${esc(ep.dir || '')}" placeholder="relative to the cert store, e.g. errorpages" />
        <div class="hint">html/template files named "&lt;status&gt;.html" (e.g. 502.html) plus an optional default.html.</div>
      </div>
      <div class="field-group" style="margin-top:10px">
        <label>Inline templates (JSON)</label>
        <textarea class="field mono" id="errp-inline" data-hint="settings.errorPages.inline" rows="6" placeholder='{"502": "&lt;h1&gt;...&lt;/h1&gt;", "default": "&lt;h1&gt;...&lt;/h1&gt;"}'>${esc(ep.inline ? JSON.stringify(ep.inline, null, 2) : '')}</textarea>
        <div class="hint">Status code (or "default") to HTML source. Template vars: {{.Status}} {{.StatusText}} {{.Host}} {{.RequestID}}.</div>
      </div>
      <div class="field-group" style="margin-top:10px">
        <label>Also replace upstream errors for</label>
        <input class="field mono" id="errp-intercept" data-hint="settings.errorPages.interceptUpstream" value="${esc(arr(ep.interceptUpstream).join(', '))}" placeholder="502, 503" />
        <div class="hint">Comma-separated status codes. Normally only gpm's own errors get the custom page; the upstream's own error body passes through untouched.</div>
      </div>
      <div class="hint" style="margin-top:12px">Leave everything blank for gpm's built-in plain-text output. A parse error fails the config reload with a message naming the template. Per-host overrides live in each host's editor, under Error pages.</div>
    </div>
    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        <button class="btn primary" id="errp-save" type="button">Save changes</button>
      </div>
    </div>`;

  $('#errp-save').addEventListener('click', async () => {
    clearEditorError();
    const dir = $('#errp-dir').value.trim();
    const inlineRaw = $('#errp-inline').value.trim();
    const intercept = $('#errp-intercept').value.split(',').map((v) => v.trim()).filter(Boolean)
      .map((v) => parseInt(v, 10)).filter((n) => !isNaN(n));
    const errp = {};
    if (dir) errp.dir = dir;
    if (inlineRaw) {
      try { errp.inline = JSON.parse(inlineRaw); }
      catch (e) { toast('Invalid error pages JSON', 'Inline templates must be valid JSON (status code or "default" -> HTML).', 'err'); return; }
    }
    if (intercept.length) errp.interceptUpstream = intercept;

    // PUT /api/settings is a whole-object replacement, and this page renders one
    // field of it. Merge over the settings as loaded so saving here cannot strip
    // adminAuth, dnsSync, ingressDiscovery or anything else it does not show.
    const body = Object.assign({}, s);
    if (Object.keys(errp).length) body.errorPages = errp; else delete body.errorPages;

    const btn = $('#errp-save'); btn.disabled = true;
    try {
      const r = await api('/api/settings', { method: 'PUT', body });
      // The per-route memo now holds a settings object this save superseded.
      resetRouteMemo();
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      route();
    } catch (e) { showSaveError(e, 'Could not save the error pages'); btn.disabled = false; }
  });
}

// ---------- SETTINGS ----------
// Tabs, not one 4000px scroll. Every tab's controls are rendered into the DOM at
// once and only the active panel is visible, which is what lets ANY tab's Save
// send the whole settings object: PUT /api/settings is a full replacement, and a
// per-tab save that only knew its own tab would wipe the others.
const SETTINGS_TABS = [
  { id: 'general', label: 'General' },
  { id: 'headers', label: 'Response headers' },
  { id: 'advanced', label: 'Advanced' },
  { id: 'operations', label: 'Operations' },
];
function showSettingsTab(id) {
  const known = SETTINGS_TABS.some((t) => t.id === id) ? id : SETTINGS_TABS[0].id;
  $$('#set-tabs .tab-btn').forEach((b) => {
    const on = b.dataset.tab === known;
    b.classList.toggle('on', on);
    b.setAttribute('aria-selected', on ? 'true' : 'false');
  });
  $$('#content .tab-panel').forEach((p) => { p.hidden = p.dataset.tab !== known; });
}

// The paste-ready set from model.RecommendedSecurityHeaders. gpm ships NOTHING
// by default, so this is a one-click starting point rather than a default. It is
// duplicated here because the API does not expose the map; TestRecommendedSecurityHeadersMirrored
// reflects over the Go value and fails if the two drift.
const RECOMMENDED_SECURITY_HEADERS = {
  'X-Content-Type-Options': { value: 'nosniff', scope: 'all' },
  'Referrer-Policy': { value: 'strict-origin-when-cross-origin', scope: 'all' },
  'X-Frame-Options': { value: 'DENY', scope: 'all' },
  'Permissions-Policy': { value: 'geolocation=(), camera=(), microphone=()', scope: 'generated-only' },
  'Content-Security-Policy': { value: "frame-ancestors 'none'", scope: 'generated-only' },
};

// Runtime card. Read-only startup facts; nothing here participates in a save,
// and booleans are words rather than switches because none of them can be
// changed from the panel.
function runtimeCardHtml(rt) {
  if (!rt || rt._error) {
    return `<div class="card form-section"><p class="section-label">Runtime</p>
      <p class="muted" style="font-size:11.5px;margin:0">Runtime details are not available to this token.</p></div>`;
  }
  const listeners = rt.listeners || {};
  const paths = rt.paths || {};
  const roots = arr(rt.secretFileRoots);
  const mono = (v) => `<span class="mono">${esc(v || '-')}</span>`;
  const word = (b, on, off) => `<span class="chip ${b ? 'ok' : ''}">${esc(b ? on : off)}</span>`;
  const row = (k, v, help) => `<span class="k">${esc(k)}</span><span class="v">${v}${help ? `<div class="hint">${help}</div>` : ''}</span>`;
  return `<div class="card form-section" id="set-runtime">
    <p class="section-label">Runtime</p>
    <p class="muted" style="font-size:11.5px;margin:0 0 10px">What this process is. Everything here is set at startup and needs a restart to change.</p>
    <div class="kv">
      ${row('Version', mono(rt.version || 'unknown'))}
      ${row('HA role', rt.haRole === 'follower' ? '<span class="chip warn">follower (read-only)</span>' : '<span class="chip">leader</span>')}
      ${row('HTTP listener', mono(listeners.http))}
      ${row('HTTPS listener', mono(listeners.https))}
      ${row('Admin listener', mono(listeners.admin))}
      ${row('Config directory', mono(paths.configDir), 'The git-backed config repo. This is what to back up.')}
      ${row('Certificate directory', mono(paths.certDir))}
      ${row('Session database', mono(paths.sessionDB))}
      ${row('Secret file roots', roots.length ? roots.map((r) => `<span class="chip mono">${esc(r)}</span>`).join(' ') : mono('none'), 'Allowlisted directories for <span class="mono">${FILE:...}</span> secrets (<span class="mono">GPM_SECRET_FILE_ROOTS</span>).')}
      ${row('Local admin login', rt.localAdminConfigured ? word(true, 'Configured', '') : '<span class="chip warn">Not configured</span>',
    rt.localAdminConfigured ? '' : 'No local password login. Set <span class="mono">GPM_LOCAL_ADMIN_USER</span> and <span class="mono">GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE</span> and restart.')}
      ${row('Local admin TOTP', word(!!rt.localAdminTOTP, 'enabled', 'disabled'))}
      ${row('Prometheus metrics', word(!!rt.metricsEnabled, 'On', 'Off'), rt.metricsEnabled ? '' : 'Start gpm with <span class="mono">GPM_METRICS=1</span> to expose <span class="mono">/metrics</span>.')}
      ${row('Access log', word(!!rt.accessLogEnabled, 'On', 'Off'), 'Live value. The Access Logs page toggles it at runtime; the <span class="mono">-access-log</span> flag decides the state after a restart.')}
      ${row('GeoIP database', word(!!rt.geoipLoaded, 'Loaded', 'Not loaded'), rt.geoipLoaded ? '' : 'Set <span class="mono">GPM_GEOIP_DB</span>. Access lists with geo rules cannot be saved or evaluated until it loads.')}
      ${row('Profiling (pprof)', rt.pprofEnabled ? '<span class="chip warn">On</span>' : '<span class="chip">Off</span>',
    rt.pprofEnabled ? '<span class="mono">/debug/pprof/</span> is exposed on the admin listener (admin role + admin scope).' : '')}
    </div>
  </div>`;
}

function settingsSaveBar(id) {
  return `<div class="panel save-bar">
    <div class="save-note">${ICON.commit}Saving any tab commits the whole settings object as one revision.</div>
    <div style="display:flex;gap:10px">
      <button class="btn primary set-save" id="${esc(id)}" type="button">Save changes</button>
    </div>
  </div>`;
}

async function viewSettings(c, tab) {
  const [setR, idpR, hostsR, , rt] = await Promise.all([
    memoGet('/api/settings'),
    refList('/api/identity-providers', 'identity providers'),
    refList('/api/proxy-hosts', 'proxy hosts'),
    loadCapabilities(),
    loadRuntime(),
  ]);
  const s = setR.data || {};
  const admin = s.adminAuth || {};
  const idps = arr(idpR);
  const proxyHostCount = arr(hostsR).length;
  const oidcIdps = idps.filter((p) => (p.type || 'oidc') === 'oidc');
  const storedProvs = arr(admin.providers);
  // A provider name that is stored but is not an OIDC provider (renamed,
  // deleted, or a forward-auth entry) still gets a checked row: dropping it
  // silently on the next save is how an operator loses their only SSO login.
  const unknownProvs = storedProvs.filter((n) => !oidcIdps.some((p) => p.name === n));
  const pp = s.proxyProtocol || {};
  const maint = s.maintenance || {};
  const idc = s.ingressDiscovery || {};
  const trusted = arr(s.trustedProxies);
  const stls = s.tls || {};

  c.innerHTML = `
    <div class="view-head"><h2>Settings</h2><p>What this instance is, who may administer it, and how it answers. Outbound integrations live under <a href="#/integrations">Integrations</a>.</p>${aboutPageHtml('page.settings')}</div>
    <div class="tabs" id="set-tabs" role="tablist">
      ${SETTINGS_TABS.map((t) => `<button class="tab-btn" type="button" role="tab" data-tab="${esc(t.id)}" aria-selected="false">${esc(t.label)}</button>`).join('')}
    </div>

    <div class="tab-panel" data-tab="general" hidden>
      ${runtimeCardHtml(rt)}
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Identity</p>
        <div class="field-group">
          <label>Instance name</label>
          <input class="field" id="set-appname" data-hint="settings.appName" data-path="appName" value="${esc(s.appName || '')}" placeholder="Go Proxy Manager" />
          <div class="hint">Brand label in the sidebar, the browser title and the login page. Blank falls back to "Go Proxy Manager".</div>
        </div>
        <div class="field-group">
          <label>Public URL of this panel</label>
          <input class="field mono" id="set-url" data-hint="settings.externalBaseURL" data-path="externalBaseURL" value="${esc(s.externalBaseURL || '')}" placeholder="https://gpm.example.com" />
          <div class="hint">The address operators reach this panel at. OIDC builds its <span class="mono">redirect_uri</span> from it, so admin sign-in through an identity provider does not work until it matches what the provider has registered.</div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Admin sign-in</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Local username/password</div><div class="ds" id="set-local-status">Username/password fallback</div></div>${switchHtml('set-local', !!admin.localLoginEnabled, 'Local login', 'settings.adminAuth.localLoginEnabled')}</div>
        <div class="field-group" style="margin-top:12px">
          <label>Single sign-on providers</label>
          <div class="check-list" id="set-providers" data-hint="settings.adminAuth.providers" data-path="adminAuth.providers">
            ${oidcIdps.length || unknownProvs.length ? oidcIdps.map((p) => `
              <label class="check-item"><input type="checkbox" value="${esc(p.name)}"${storedProvs.indexOf(p.name) !== -1 ? ' checked' : ''}/>${esc(p.name)}<span class="ci-ty">oidc</span></label>
            `).join('') + unknownProvs.map((n) => `
              <label class="check-item"><input type="checkbox" value="${esc(n)}" checked/>${esc(n)}<span class="ci-ty" title="Stored in settings but not an OIDC identity provider on this instance">unknown</span></label>
            `).join('') : '<div class="check-empty">No OIDC identity providers defined yet.</div>'}
          </div>
          <div class="hint">Which identity providers may sign in to this admin panel. Only OIDC providers can: forward-auth and auth-request assert identity for proxied hosts, not for the panel. Define them under <a href="#/identity">Identity Providers</a>.</div>
        </div>
        <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Require SSO for admins</div><div class="ds">All admin access goes through SSO</div></div>${switchHtml('set-sso', !!admin.ssoOnly, 'Require SSO for admins', 'settings.adminAuth.ssoOnly')}</div>
        <div class="hint" id="set-sso-hint" style="margin-top:8px">There is no in-band break-glass door: recovering from an identity-provider outage under SSO-only means redeploying with local login re-enabled.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Client IP</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Which proxies in front of gpm may set the client address.</p>
        <div class="field-group">
          <label>Trusted proxies</label>
          <div class="chip-input" id="set-trustedproxies" data-hint="settings.trustedProxies" data-path="trustedProxies"></div>
          <div class="hint">CIDRs or bare IP addresses. When a request arrives from one of these, gpm reads the client address from <span class="mono">X-Forwarded-For</span> instead of the connection. Empty means trust nobody: the connection address is the client, which is correct when gpm faces the internet directly.</div>
          <div class="hint">This one address is what access lists, geo rules, rate limits, <span class="mono">allowFrom</span> exemptions and the access log all compare. <a href="https://github.com/Rake-Pro/go-proxy-manager/blob/main/docs/concepts/request-pipeline.md#client-ip-and-the-three-trust-tiers" target="_blank" rel="noopener">Learn more</a></div>
          <div class="hint warn" id="set-trustedproxies-warn" hidden>${esc(TRUSTED_WILDCARD_WARNING)}</div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">TLS</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">The floor every HTTPS and stream-terminate listener negotiates, for the whole fleet.</p>
        <div class="field-group">
          <label>Minimum TLS version</label>
          <select class="field mono" id="set-mintls" data-hint="settings.tls.minVersion" data-path="tls.minVersion">
            <option value="1.2"${(stls.minVersion || '1.2') === '1.2' ? ' selected' : ''}>1.2 (default, negotiates 1.2/1.3)</option>
            <option value="1.3"${stls.minVersion === '1.3' ? ' selected' : ''}>1.3 only</option>
          </select>
          <div class="hint">Applies to every host that does not pin its own. A proxy host's <span class="mono">Minimum TLS version</span> overrides this either way, so one legacy host can stay on 1.2 under a 1.3 fleet floor.</div>
          <div class="hint">1.3 only refuses the handshake for any client that cannot negotiate it: there is no fallback and no error page. Raise it once every client of every unpinned host supports 1.3.</div>
        </div>
      </div>
      ${settingsSaveBar('set-save')}
    </div>

    <div class="tab-panel" data-tab="headers" hidden>
      <div class="card form-section">
        <p class="section-label">Response security headers</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Fleet-default response headers, applied set-if-absent so an app's own header is never clobbered. Scope selects which responses each one lands on: <span class="mono">all</span>, <span class="mono">generated-only</span> (only responses gpm writes itself - denials, sign-in redirects, error pages) or <span class="mono">proxied-only</span>. Any proxy host can override a header (value and scope) in its own editor. Empty ships nothing.</p>
        <div id="set-secheaders" data-hint="settings.securityHeaders" data-path="securityHeaders"></div>
        <div style="display:flex;gap:8px;margin-top:6px;flex-wrap:wrap">
          <button class="btn ghost sm" id="set-secheaders-add" type="button">${ICON.plus}Add header</button>
          <button class="btn ghost sm" id="set-secheaders-recommend" type="button" title="Add the five headers gpm documents as a safe starting point. Nothing already set is overwritten.">Load recommended</button>
        </div>
        <div class="hint" style="margin-top:6px">Put app-breaking headers - Content-Security-Policy frame-ancestors, Permissions-Policy - at <span class="mono">generated-only</span>: they are safe on gpm's own pages and break a proxied app that ships none of its own. Strict-Transport-Security is refused here; HSTS is a per-host TLS setting.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Strip upstream response headers</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Fleet-default list of header names removed from proxied upstream responses (<span class="mono">Server</span>, <span class="mono">X-Powered-By</span>, ...). Only what the upstream sent is touched - never a header gpm adds. Any proxy host can add its own names in its editor; the lists are unioned.</p>
        <div class="chip-input" id="set-striphdrs" data-hint="settings.stripResponseHeaders" data-path="stripResponseHeaders"></div>
        <div class="hint" style="margin-top:6px">Hop-by-hop and response-semantic names (Content-Type, Content-Length, Content-Encoding, Location, Vary, the WebSocket handshake) are refused on save. Set-Cookie and WWW-Authenticate are allowed but sharp: stripping them breaks the app's own sessions or its basic-auth challenge.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Error pages</p>
        <p class="muted" style="font-size:11.5px;margin:0">Custom HTML for gpm-generated errors has its own section: <a href="#/errorpages">Error pages</a>. It edits the same <span class="mono">settings.errorPages</span> config.</p>
      </div>
      ${settingsSaveBar('set-save-headers')}
    </div>

    <div class="tab-panel" data-tab="advanced" hidden>
      <div class="card form-section">
        <p class="section-label">PROXY protocol (inbound)</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Read the real client address out of the HAProxy PROXY protocol header (v1 and v2) on the <span class="mono">:80</span>/<span class="mono">:443</span> listeners and every TCP stream listener, so gpm behind an L4 load balancer sees the client rather than the balancer. This is the L4 half of the same question the Client IP card answers at L7. The header is an unauthenticated claim, so it is honoured <b>only</b> from the trusted peers below. UDP stream listeners are unaffected.</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Accept PROXY headers</div><div class="ds">From the trusted peers below only</div></div>${switchHtml('set-pp-on', !!pp.enabled, 'Accept PROXY headers', 'settings.proxyProtocol.enabled')}</div>
        <div class="grid-2" style="margin-top:10px">
          <div class="field-group"><label>Trusted peers</label><div class="chip-input" id="set-pp-cidrs" data-hint="settings.proxyProtocol.trustedCIDRs" data-path="proxyProtocol.trustedCIDRs"></div>
            <div class="hint">CIDRs or bare IPs of your load balancers, e.g. <span class="mono">10.0.0.0/8</span>. Required when enabled - there is no "trust everyone" mode.</div>
          </div>
          <div class="field-group"><label>Header timeout</label><input class="field mono" id="set-pp-timeout" data-hint="settings.proxyProtocol.timeout" data-path="proxyProtocol.timeout" value="${esc(pp.timeout || '')}" placeholder="5s" />
            <div class="hint">How long a trusted peer has to deliver a complete header before the connection is closed. Blank = 5s, maximum 1m.</div>
          </div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Annotation and label prefix</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">One prefix per deployment, shared by Kubernetes Ingress discovery and Docker discovery: it names every annotation and label they read (<span class="mono">.../managed</span>, <span class="mono">.../profile</span>) and the <span class="mono">managed-by</span> / <span class="mono">disabled-by</span> labels gpm writes on derived hosts.</p>
        <div class="field-group" style="max-width:320px"><label>Prefix</label>
          <input class="field mono" id="set-id-annprefix" data-hint="settings.ingressDiscovery.annotationPrefix" data-path="ingressDiscovery.annotationPrefix" value="${esc(idc.annotationPrefix || '')}" placeholder="gpm.rake.pro" />
          <div class="hint">Leave blank for the default. Changing it does not relabel existing hosts by itself.</div>
        </div>
        <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Migrate existing hosts to the new prefix</div><div class="ds">Relabels hosts still under the old prefix in the next reconcile, in one commit</div></div>${switchHtml('set-id-annprefix-migrate', !!idc.annotationPrefixMigrate, 'Migrate existing hosts to the new prefix', 'settings.ingressDiscovery.annotationPrefixMigrate')}</div>
      </div>
      <div class="card form-section" id="set-metrics" style="margin-top:16px">
        <p class="section-label">Prometheus metrics</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Opt-in scrape endpoint on this admin listener, enabled with <span class="mono">GPM_METRICS=1</span> (a flag, not a saved setting - it needs a restart). An API token with the <span class="mono">metrics:read</span> scope can scrape it; your admin session can open it directly.</p>
        <a class="btn ghost sm" id="set-metrics-link" href="/metrics" target="_blank" rel="noopener">Open /metrics</a>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">High availability</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">HA is decided at startup, not here. One instance is the leader and owns every write - config commits, ACME orders, DNS and discovery reconciles. A follower serves traffic from the replicated config and refuses config writes with a <span class="mono">503</span>, which is why its write controls are greyed out.</p>
        <div class="kv">
          <span class="k">This instance</span><span class="v">${(rt && rt.haRole === 'follower') ? '<span class="chip warn">follower (read-only)</span>' : '<span class="chip">leader</span>'}</span>
          <span class="k">Config repo</span><span class="v"><span class="mono">${esc((rt && rt.paths && rt.paths.configDir) || '-')}</span><div class="hint">Replicate this directory to the follower; it is the whole state.</div></span>
        </div>
      </div>
      ${settingsSaveBar('set-save-advanced')}
    </div>

    <div class="tab-panel" data-tab="operations" hidden>
      <div class="card form-section danger-zone">
        <p class="section-label">Danger zone</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Two actions that affect every host or every signed-in user at once. Both are deliberate, both are confirmed, and neither has a partial version.</p>
      </div>
      <div class="card form-section danger-zone" style="margin-top:16px">
        <p class="section-label">Fleet-wide maintenance</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">Takes <b>every proxy host</b> out of service for a downtime window: gpm answers each request itself with a <span class="mono">503</span> and a <span class="mono">Retry-After</span> instead of dialling the upstream. It wins over each host's own Maintenance switch, so turning it off returns hosts to whatever they were individually set to. Redirect, parked and stream hosts proxy nothing and keep serving; ACME HTTP-01 is answered before host routing, so certificates still renew. The page served is the <a href="#/errorpages">error page</a> configured for <span class="mono">503</span> (a host's own override first), or gpm's built-in maintenance page when none is.</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Fleet-wide maintenance</div><div class="ds">All proxy hosts serve the maintenance page</div></div>${switchHtml('set-maint-on', !!maint.enabled, 'Fleet-wide maintenance', 'settings.maintenance.enabled')}</div>
        <div class="field-group" style="margin-top:10px;max-width:220px">
          <label>Retry-After (seconds)</label>
          <input class="field mono" id="set-maint-retry" data-hint="settings.maintenance.retryAfterSeconds" data-path="maintenance.retryAfterSeconds" type="number" min="0" max="86400" value="${esc(maint.retryAfterSeconds != null ? String(maint.retryAfterSeconds) : '')}" placeholder="300" />
          <div class="hint">Sent on every maintenance response, per-host ones included. Blank = 300s, maximum 24h.</div>
        </div>
      </div>
      <div class="card form-section danger-zone" style="margin-top:16px">
        <p class="section-label">Per-host SSO sessions</p>
        <p class="muted" style="font-size:11.5px;margin:0 0 10px">SSO cookies on proxied hosts are stateless with a 1-hour cap. Revoking invalidates every outstanding session immediately; users re-authenticate at the identity provider on their next request. This takes effect at once and is not part of a save.</p>
        <button class="btn danger sm" id="set-sso-revoke" type="button">Revoke all SSO sessions</button>
      </div>
      ${settingsSaveBar('set-save-operations')}
    </div>`;

  showSettingsTab(tab || SETTINGS_TABS[0].id);
  $$('#set-tabs .tab-btn').forEach((b) => {
    b.addEventListener('click', () => { location.hash = '#/settings/' + b.dataset.tab; });
  });

  // Metrics endpoint: greyed out (with the reason) unless the daemon reports it
  // mounted, rather than linking at a route that answers 404.
  const metricsAvailable = hasCapability('metrics.enabled');
  // gateControl now removes the href and drops a gated anchor out of the tab
  // order itself, so nothing extra is needed here.
  gateControl($('#set-metrics-link'), metricsAvailable,
    'Metrics are turned off on this instance. Start gpm with GPM_METRICS=1 (or -metrics) and restart.');

  // Local login switch: say whether a credential actually exists behind it, from
  // the same runtime fact the Runtime card reports.
  const localStatus = $('#set-local-status');
  if (localStatus && rt && !rt._error) {
    localStatus.innerHTML = rt.localAdminConfigured
      ? 'Username/password fallback (credential configured)'
      : 'Username/password fallback <span class="warn-text">(no credential configured - this switch grants nothing until <span class="mono">GPM_LOCAL_ADMIN_USER</span> and a bcrypt hash are set)</span>';
  }

  const ppCtl = makeChipInput($('#set-pp-cidrs'), arr(pp.trustedCIDRs), 'add CIDR...');
  const trustedCtl = makeChipInput($('#set-trustedproxies'), trusted, '10.0.0.0/8 or 192.0.2.10', (list) => {
    $('#set-trustedproxies-warn').hidden = !hasWildcardProxy(list);
  });
  $('#set-trustedproxies-warn').hidden = !hasWildcardProxy(trusted);
  function selectedProviders() { return $$('#set-providers input:checked').map((i) => i.value); }

  // SSO-only lockout guard. The API refuses ssoOnly with no providers, but the
  // damage the UI can do is subtler: turning it on with a provider list that
  // cannot actually serve a panel login leaves the redeploy as the only way
  // back. So the switch is unreachable until an OIDC provider exists, and turning
  // it on says what it costs.
  function refreshSSOGate() {
    const usable = oidcIdps.length > 0 || unknownProvs.length > 0;
    gateControl($('#set-sso'), usable || isOn('set-sso'), 'Add an OIDC identity provider first.');
  }
  refreshSSOGate();
  $('#set-sso').addEventListener('switchchange', async () => {
    if (!isOn('set-sso')) return;
    const ok = await confirmModal({
      title: 'Require SSO for admins?',
      body: '<p>Local username/password login is <b>disabled</b> for this admin panel. Every operator signs in through one of the providers above.</p>'
        + '<p>There is deliberately no in-band break-glass door. If the identity provider is unreachable, its configuration drifts, or the provider entry is removed, '
        + '<b>recovery is a redeploy</b> with local login re-enabled - not a login page.</p>',
      confirmLabel: 'Require SSO',
    });
    if (!ok) $('#set-sso').setAttribute('aria-checked', 'false');
  });

  // Fleet-wide maintenance takes every proxy host out of service at once, so it
  // is typed-confirmed on the way ON and says how many hosts that is.
  $('#set-maint-on').addEventListener('switchchange', async () => {
    if (!isOn('set-maint-on')) return;
    const ok = await confirmModal({
      title: 'Turn on fleet-wide maintenance?',
      body: `<p><b>${proxyHostCount} proxy host${proxyHostCount === 1 ? '' : 's'}</b> will stop reaching their upstreams and answer <span class="mono">503</span> with a <span class="mono">Retry-After</span> instead.</p>`
        + '<p>It overrides each host\'s own Maintenance switch. Redirect, parked and stream hosts keep serving, and ACME HTTP-01 is still answered, so certificates continue to renew.</p>'
        + '<p>This takes effect as soon as the save commits.</p>',
      confirmLabel: 'Turn on maintenance',
      typed: 'MAINTENANCE',
    });
    if (!ok) $('#set-maint-on').setAttribute('aria-checked', 'false');
  });

  const secHdrCtl = makeSecurityHeaderRows($('#set-secheaders'), s.securityHeaders);
  $('#set-secheaders-add').addEventListener('click', () => secHdrCtl.addRow('', ''));
  // Additive on purpose: a header the operator already set keeps its value and
  // scope, so this can never silently rewrite a deliberate choice.
  $('#set-secheaders-recommend').addEventListener('click', () => {
    const have = Object.create(null);
    $$('#set-secheaders .sh-name').forEach((i) => { if (i.value.trim()) have[i.value.trim().toLowerCase()] = true; });
    let added = 0;
    Object.keys(RECOMMENDED_SECURITY_HEADERS).forEach((k) => {
      if (have[k.toLowerCase()]) return;
      const v = RECOMMENDED_SECURITY_HEADERS[k];
      secHdrCtl.addRow(k, v.scope === 'all' ? v.value : { value: v.value, scope: v.scope });
      added++;
    });
    toast(added ? 'Recommended headers added' : 'Nothing to add',
      added ? `${added} header${added === 1 ? '' : 's'} added. Review them, then save.` : 'Every recommended header is already listed.', added ? 'ok' : '');
  });
  const stripCtl = makeChipInput($('#set-striphdrs'), arr(s.stripResponseHeaders), 'add header...');

  // The migrate switch only matters once the prefix field actually differs
  // from what is currently saved, so it stays greyed out (and off) otherwise -
  // toggling it on without changing the prefix would be a no-op refusal-bypass
  // for nothing.
  const idPrefixStored = idc.annotationPrefix || '';
  function refreshAnnPrefixMigrateGate() {
    const changed = $('#set-id-annprefix').value.trim() !== idPrefixStored;
    gateControl($('#set-id-annprefix-migrate'), changed, 'Only needed when the prefix above differs from the currently saved one.');
  }
  $('#set-id-annprefix').addEventListener('input', refreshAnnPrefixMigrateGate);
  refreshAnnPrefixMigrateGate();

  $('#set-sso-revoke').addEventListener('click', async () => {
    const ok = await confirmModal({
      title: 'Revoke every SSO session?',
      body: '<p>Every user currently signed in through a proxy host is signed out at once. Their next request goes back to the identity provider to authenticate again.</p>'
        + '<p>There is no partial version of this and no undo: sessions cannot be restored, only re-established by signing in.</p>',
      confirmLabel: 'Revoke all sessions',
      typed: 'REVOKE',
    });
    if (!ok) return;
    const btn = $('#set-sso-revoke'); btn.disabled = true;
    try {
      await api('/api/sso/revoke', { method: 'POST' });
      toast('Sessions revoked', 'All per-host SSO sessions are now invalid.', 'ok');
    } catch (e) { toastErr(e); }
    btn.disabled = false;
  });

  // Every tab's Save runs the SAME handler and sends the SAME whole object.
  $('#set-save').addEventListener('click', () => saveSettings($('#set-save')));
  $$('.set-save').forEach((b) => { if (b.id !== 'set-save') b.addEventListener('click', () => saveSettings(b)); });

  async function saveSettings(btn) {
    clearEditorError();
    // INVARIANT: PUT /api/settings is a whole-object replacement, so EVERY
    // top-level model.Settings field must be either edited on this page or
    // carried forward from `s` below. A field that is neither is silently wiped
    // on every save made here - which is exactly how appName and accessListSync
    // were lost. TestSettingsSaveSendsEveryTopLevelSettingsField reflects over
    // model.Settings and fails when a field appears in neither list.
    const body = {
      schemaVersion: s.schemaVersion,
      appName: $('#set-appname').value.trim(),
      externalBaseURL: $('#set-url').value.trim(),
      adminAuth: {
        localLoginEnabled: isOn('set-local'),
        ssoOnly: isOn('set-sso'),
      },
    };
    const provs = selectedProviders();
    if (provs.length) body.adminAuth.providers = provs;

    // Client IP. Empty means trust nobody, so the key is dropped rather than
    // sent as [] - an untouched settings.yaml round-trips without gaining one.
    const tp = trustedCtl.get();
    if (tp.length) {
      const badTp = firstBadCidr(tp);
      if (badTp) {
        toast('Invalid trusted proxy', `"${badTp}" is not a CIDR or IP address. Use 10.0.0.0/8 or 192.0.2.10.`, 'err');
        return;
      }
      body.trustedProxies = tp;
    }

    // Fleet TLS floor. "1.2" IS the default, so the key is left off the body
    // entirely for it, so an untouched settings.yaml round-trips without gaining a
    // `tls: {}`. A host's own tls.minTLSVersion still overrides whatever lands
    // here (see the proxy host editor).
    const fleetMinTLS = $('#set-mintls').value;
    if (fleetMinTLS === '1.3') body.tls = { minVersion: '1.3' };

    // Maintenance. The key is left off the body entirely when the switch is off
    // and no Retry-After is set, so an untouched settings.yaml round-trips
    // without gaining a `maintenance: {}`.
    const maintRetry = parseInt($('#set-maint-retry').value, 10);
    if (!isNaN(maintRetry) && (maintRetry < 0 || maintRetry > 86400)) {
      toast('Retry-After out of range', 'Maintenance Retry-After must be between 0 and 86400 seconds (24h). Leave it blank for the 300s default.', 'err');
      return;
    }
    const maintenance = {};
    if (isOn('set-maint-on')) maintenance.enabled = true;
    if (!isNaN(maintRetry) && maintRetry > 0) maintenance.retryAfterSeconds = maintRetry;
    if (Object.keys(maintenance).length) body.maintenance = maintenance;

    const ppCidrs = ppCtl.get();
    const ppTimeout = $('#set-pp-timeout').value.trim();
    if (isOn('set-pp-on') && !ppCidrs.length) {
      toast('Trusted peers required', 'A PROXY header from an untrusted peer would let any client spoof its source IP. Add the CIDRs of your load balancers.', 'err');
      return;
    }
    if (isOn('set-pp-on') || ppCidrs.length || ppTimeout) {
      const proxyProtocol = {};
      if (isOn('set-pp-on')) proxyProtocol.enabled = true;
      if (ppCidrs.length) proxyProtocol.trustedCIDRs = ppCidrs;
      if (ppTimeout) proxyProtocol.timeout = ppTimeout;
      body.proxyProtocol = proxyProtocol;
    }

    // securityHeaders (the fleet default) is edited on this page by the row
    // editor above, which replaced the old carry-forward guard. A settings PUT is
    // a whole-object replacement, so the editor's output IS the field: it emits
    // an untouched map byte-equivalently, keeps a shape it does not understand
    // verbatim, and returns null when there are no rows - which leaves the key
    // off the body entirely rather than committing an empty map.
    const secHdrErr = secHdrCtl.error();
    if (secHdrErr) { toast('Invalid security header', secHdrErr, 'err'); return; }
    const secHdrs = secHdrCtl.get();
    if (secHdrs) body.securityHeaders = secHdrs;

    // stripResponseHeaders (the fleet default list of headers removed from
    // proxied upstream responses) is owned by the chip editor above, which
    // replaced the old carry-forward guard. An empty list leaves the key off
    // the body entirely rather than committing an empty array.
    const stripHdrs = stripCtl.get();
    const stripErr = stripHeaderListError(stripHdrs);
    if (stripErr) { toast('Invalid strip header', stripErr, 'err'); return; }
    if (stripHdrs.length) body.stripResponseHeaders = stripHdrs;

    // ingressDiscovery: this page owns only the shared annotation prefix and its
    // migration flag (Advanced tab); the connection, template and profiles live
    // on the Integrations page. MERGED over the loaded block so saving here
    // cannot strip any of it.
    const ingressDiscovery = Object.assign({}, s.ingressDiscovery, {
      annotationPrefix: $('#set-id-annprefix').value.trim(),
    });
    if (isOn('set-id-annprefix-migrate')) ingressDiscovery.annotationPrefixMigrate = true;
    else delete ingressDiscovery.annotationPrefixMigrate;
    body.ingressDiscovery = ingressDiscovery;

    // Carried forward verbatim: each of these has a real editor on ANOTHER page,
    // and a settings PUT is a whole-object replacement, so leaving one out here
    // would wipe it on every save made from this page.
    //   errorPages      -> the Error pages section (#/errorpages)
    //   dnsSync         -> Integrations, DNS sync card
    //   dockerDiscovery -> Integrations, Docker discovery card
    //   accessListSync  -> Integrations, Access-list sync card
    //   webhooks        -> Integrations, Lifecycle webhooks card
    //   notifications   -> Integrations, Notifications card
    if (s.errorPages && Object.keys(s.errorPages).length) body.errorPages = s.errorPages;
    if (s.dnsSync && Object.keys(s.dnsSync).length) body.dnsSync = s.dnsSync;
    if (s.dockerDiscovery && Object.keys(s.dockerDiscovery).length) body.dockerDiscovery = s.dockerDiscovery;
    if (s.accessListSync && Object.keys(s.accessListSync).length) body.accessListSync = s.accessListSync;
    if (arr(s.webhooks).length) body.webhooks = s.webhooks;
    if (s.notifications && Object.keys(s.notifications).length) body.notifications = s.notifications;

    btn.disabled = true;
    try {
      const r = await api('/api/settings', { method: 'PUT', body });
      // The per-route memo now holds a settings object this save superseded.
      resetRouteMemo();
      toastSaved(r.commit); refreshHeadSha(); clearDirty();
      // refresh instance label
      if (body.externalBaseURL) {
        try { state.instance = new URL(body.externalBaseURL).host || body.externalBaseURL; }
        catch (e) { state.instance = body.externalBaseURL; }
        const inst = $('.topbar .instance'); if (inst) inst.textContent = state.instance;
      }
      // The brand label is rendered once, at boot, into the sidebar and the
      // browser title - so an edited appName has to be pushed into both here or
      // the page keeps showing the old name until a reload.
      state.appName = body.appName || 'Go Proxy Manager';
      const nm = document.querySelector('.wordmark .name');
      if (nm) nm.textContent = state.appName;
      document.title = state.appName;
      const pt = $('#pageTitle'); if (pt) pt.textContent = TITLES.settings || state.appName;
      // maintenance.globalEnabled and ingressDiscovery.enabled are capability
      // probe values this save can flip. Drop the cache AND re-read it right
      // away: a null cache reads as "every capability false" everywhere, and
      // the operator who just turned fleet-wide maintenance on is the one
      // person who must not have to reload to see the banner - including on
      // the very view they turned it on from.
      state.capabilities = null;
      await loadCapabilities();
      refreshShellBanners();
    } catch (e) { showSaveError(e, 'Could not save settings'); }
    btn.disabled = false;
  }
}

// keep topbar config sha fresh after writes
async function refreshHeadSha() {
  try {
    const h = (await api('/api/history')).data;
    if (Array.isArray(h) && h.length) {
      state.headSha = h[0].hash;
      const badge = $$('.topbar .badge').find((b) => b.textContent.indexOf('config @') === 0);
      if (badge) badge.textContent = 'config @ ' + shortSha(h[0].hash);
    }
  } catch (e) { /* ignore */ }
}

// Greys out sidebar entries whose backing capability is unavailable. Runs after
// loadTopbar, which is what populates the capability probe (buildShell renders
// the nav before any fetch has happened).
function applyNavGating() {
  const item = document.querySelector('#nav .nav-item[data-view="tokens"]');
  // GET /api/api-tokens answers 403 for the read-only "user" role, so the page
  // would render an error rather than a list: hide the entry outright. Every
  // other page is readable by that role, so nothing else is hidden.
  if (isRoleReadOnly()) { if (item) item.hidden = true; return; }
  gateControl(item, hasCapability('apiTokens.enabled'),
    'API tokens are not wired in this deployment.');
}

// buildShell renders the sidebar before any fetch has happened, so a
// collapsible group with no stored preference is drawn closed and re-evaluated
// here, once loadCounts has said whether the install actually uses it. A stored
// preference is never overridden.
function applyNavGroupDefaults() {
  NAV_GROUPS.filter((g) => g.collapsible).forEach((g) => {
    let stored = null;
    try { stored = localStorage.getItem(NAV_GROUP_KEY + g.label); } catch (e) { /* ignore */ }
    if (stored === 'open' || stored === 'closed') return;
    if (navGroupDefaultOpen(g)) setNavGroupOpen(g.label, true, false);
  });
}

// ---------- boot ----------
async function boot() {
  applyTheme(getTheme());
  await loadHints();
  buildShell();
  await loadTopbar();
  applyNavGating();
  applyNavGroupDefaults();
  window.addEventListener('hashchange', onHashChange);
  await route();
}
boot();
