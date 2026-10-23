// go-proxy-manager admin SPA. Dependency-free vanilla ES module.

// ---------- icons ----------
const ICON = {
  fanout: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h6M10 12c3.5 0 3.5-5 7-5h2.5M10 12c3.5 0 3.5 5 7 5h2.5M17 4.6l2.4 2.4-2.4 2.4M17 14.6l2.4 2.4-2.4 2.4"/></svg>',
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
  contrast: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" stroke="none"/></svg>',
  // Access Logs and History both used to render ICON.history, which made two
  // unrelated sections read as one. Logs is a request tape (a page of entries);
  // history stays the clock-with-arrow.
  logbook: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M5 3h11l3 3v15H5z"/><path d="M8 9h8M8 13h8M8 17h5"/></svg>',
  // Proxy Hosts keeps ICON.globe; DNS Providers gets its own record-lookup mark
  // so the two are not the same glyph in one sidebar.
  dns: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3v6"/><circle cx="12" cy="11" r="2"/><path d="M12 13v3M6 21v-3a2 2 0 012-2h8a2 2 0 012 2v3"/><circle cx="12" cy="4" r="1.6"/></svg>',
  plug: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M9 3v6M15 3v6"/><path d="M6 9h12v3a6 6 0 01-12 0z"/><path d="M12 18v3"/></svg>',
  chevron: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg>',
  caret: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>',
  sun: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
  moon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.5A8 8 0 019.5 4a8 8 0 1010.5 10.5z"/></svg>',
  check: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5l4.5 4.5L19 7"/></svg>',
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
      { id: 'parked', label: 'Parked Hosts', icon: ICON.parked },
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

// The UI singular for each Go kind, so History and its dialogs speak the
// words the rest of the UI uses ("proxy host", never "ProxyHost").
const KIND_SINGULAR = {
  ProxyHost: 'proxy host', RedirectHost: 'redirect', StreamHost: 'stream',
  ParkedHost: 'parked host', Certificate: 'certificate', ClientCA: 'client CA',
  DNSProvider: 'DNS provider', IdentityProvider: 'identity provider',
  UpstreamGroup: 'upstream group', AccessList: 'access list', Middleware: 'middleware',
  APIToken: 'API token',
};
function kindSingular(kind) { return KIND_SINGULAR[kind] || String(kind || '').toLowerCase(); }
function capFirst(s) { s = String(s || ''); return s.charAt(0).toUpperCase() + s.slice(1); }
// historyText renders a commit message in UI words: `ProxyHost "notes": update`
// reads "Proxy host notes updated". A message it does not recognise is shown
// as written; the raw message always stays in the entry's title.
function historyText(msg) {
  const m = /^(\w+) "(.+)": (update|delete)$/.exec(msg || '');
  if (m && KIND_SINGULAR[m[1]]) return `${capFirst(KIND_SINGULAR[m[1]])} ${m[2]} ${m[3] === 'update' ? 'updated' : 'deleted'}`;
  const r = /^Revert (\w+)\/(.+) to (\w+)$/.exec(msg || '');
  if (r && KIND_SINGULAR[r[1]]) return `${capFirst(KIND_SINGULAR[r[1]])} ${r[2]} reverted to ${shortSha(r[3])}`;
  const c = /^Revert config to (\w+)$/.exec(msg || '');
  if (c) return `Config reverted to ${shortSha(c[1])}`;
  if (msg === 'Settings: update') return 'Settings updated';
  if (msg === 'Restore configuration from archive') return 'Config restored from archive';
  return msg || '(no message)';
}

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
// The main column is the scroll box (app.css), not the document.
function scrollToTop() {
  window.scrollTo(0, 0);
  const m = document.getElementById('main');
  if (m) m.scrollTop = 0;
}
// One helper for every count shown with a noun: plural(1, 'tag', 'tags') is
// "1 tag", plural(2, 'tag', 'tags') is "2 tags".
function plural(n, one, many) { return `${n} ${n === 1 ? one : many}`; }
function fmtTime(s, seconds) {
  const d = new Date(s);
  if (isNaN(d.getTime())) return s || '';
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}${seconds ? ':' + p(d.getSeconds()) : ''}`;
}
// Clock time with seconds, for the access log where a burst of requests falls
// inside one minute; the full timestamp goes in the cell's title.
function fmtClock(s) {
  const d = new Date(s);
  if (isNaN(d.getTime())) return s || '';
  const p = (n) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
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

// Renders up to `max` items (default 2) comma-joined, folding the rest into
// "+N"; the full list is always in the title tooltip. Each domain is its own
// nowrap item (.dl-item), so a list wraps only BETWEEN domains and a name never
// splits mid-label. Shared by any column that can hold a long domain/SAN list.
function domainListHtml(domains, max) {
  domains = arr(domains);
  if (!domains.length) return '';
  const shown = domains.slice(0, max || 2);
  const rest = domains.slice(shown.length);
  const item = (d) => `<span class="dl-item">${esc(d)}</span>`;
  let html = shown.map(item).join(', ');
  if (rest.length) {
    const m = moreToggle(rest, ', ' + rest.map(item).join(', '));
    html += m.list + ' ' + m.btn;
  }
  return `<span class="domain-list" title="${esc(domains.join(', '))}">${html}</span>`;
}

// moreToggle builds the "+N" control for a folded list: a real button, so
// touch and keyboard users can open the rest (a title tooltip alone is
// hover-only), plus the hidden element it reveals. The two are tied by id, not
// by position, so a caller can place the revealed list wherever it fits (inline
// after a domain list, or as its own block under a table cell). The title stays
// for mouse users. One capture-phase listener below does the toggling.
let moreSeq = 0;
function moreToggle(rest, listHtml, block) {
  const id = 'more-' + (++moreSeq);
  const tag = block ? 'div' : 'span';
  return {
    btn: `<button type="button" class="more-btn" aria-expanded="false" aria-controls="${id}" data-label="+${rest.length}" title="${esc(rest.join(', '))}">+${rest.length}</button>`,
    list: `<${tag} class="more-list" id="${id}" hidden>${listHtml}</${tag}>`,
  };
}
// Capture phase, and the event stops here: the button sits inside clickable
// rows and cards, which would otherwise navigate on the same click.
document.addEventListener('click', (e) => {
  const b = e.target && e.target.closest ? e.target.closest('.more-btn') : null;
  if (!b) return;
  e.stopPropagation();
  e.preventDefault();
  // Stopping the click here also keeps it from the document listener that
  // closes an open hint popover, so close it explicitly.
  closeHintPop();
  const open = b.getAttribute('aria-expanded') !== 'true';
  b.setAttribute('aria-expanded', String(open));
  b.textContent = open ? 'less' : b.dataset.label;
  const list = document.getElementById(b.getAttribute('aria-controls'));
  if (list) list.hidden = !open;
}, true);

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

// The UI's word for a middleware type where it differs from the YAML value:
// the rail, the check-lists and the summaries all read from here.
const MW_TYPE_LABEL = { auth: 'sign-in', 'rate-limit': 'rate limit' };

// ---------- list filter ----------
// Every list filter box takes plain words and field-scoped terms. parseFilter
// turns `tag:lan -status:disabled "two words" foo` into terms. A colon only
// scopes a term when its left side is one of the list's fields, so an
// upstream written "group:k8s" is still found by typing it. A leading "-"
// excludes. A field with nothing after the colon (still being typed) matches
// everything. Commas in a scoped term list alternatives (`status:4xx,5xx`
// holds when either does); a quoted value is one value, commas and all.
function parseFilter(q, fields) {
  const terms = [];
  const re = /(-)?(?:([A-Za-z][\w-]*):)?(?:"([^"]*)"|(\S*))/g;
  let m;
  while ((m = re.exec(q || ''))) {
    if (!m[0]) { re.lastIndex++; continue; }
    let field = (m[2] || '').toLowerCase();
    let value = m[3] != null ? m[3] : m[4];
    if (field && !(fields && fields[field])) {
      value = m[2] + ':' + (m[3] != null ? '"' + m[3] + '"' : m[4]);
      field = '';
    }
    value = value.trim().toLowerCase();
    if (!value) continue;
    const values = field && m[3] == null ? value.split(',').map((v) => v.trim()).filter(Boolean) : [value];
    if (!values.length) continue;
    terms.push({ neg: !!m[1], field, value: values[0], values });
  }
  return terms;
}
// A filterable record is {blob, f, m}: blob is every value the row was built
// from (lowercase, never the markup), f maps a field name to its lowercase
// values, and the optional m maps a field name to a matcher (value) => bool
// for a field whose values are not plain text (the access log's client IP).
// Every term must hold; a scoped term holds when any of its alternatives
// matches a substring of any of the field's values (or satisfies the field's
// matcher), a word matches the blob.
function filterHit(rec, terms) {
  for (const t of terms) {
    let hit;
    if (t.field) {
      const match = rec.m && rec.m[t.field];
      const vals = (rec.f && rec.f[t.field]) || [];
      hit = (t.values || [t.value]).some((alt) => (match ? match(alt) : vals.some((v) => v.indexOf(alt) !== -1)));
    } else {
      hit = (rec.blob || '').indexOf(t.value) !== -1;
    }
    if (hit === t.neg) return false;
  }
  return true;
}
// fvals flattens its arguments (strings, numbers, arrays of them) to the
// lowercase value list a record field holds; blanks are dropped.
function fvals() {
  const out = [];
  Array.prototype.forEach.call(arguments, (v) => {
    (Array.isArray(v) ? v : [v]).forEach((x) => {
      if (x == null || x === '' || typeof x === 'boolean') return;
      out.push(String(x).toLowerCase());
    });
  });
  return out;
}
// The suggestion values for a field: every distinct value across the records,
// first spelling wins, sorted.
function fieldValues(recs) {
  const out = {};
  recs.forEach((r) => {
    Object.keys(r.f || {}).forEach((k) => {
      const seen = out[k] || (out[k] = []);
      r.f[k].forEach((v) => { if (seen.indexOf(v) === -1) seen.push(v); });
    });
  });
  Object.keys(out).forEach((k) => out[k].sort());
  return out;
}

// filterBoxHtml renders a list's search control: the committed field terms as
// chips, the input, and the suggestion list under it, plus the Saved menu
// (named filters kept in settings, see wireFilter). fields is
// {name: 'what it matches'}, which the title and the suggestions show.
function filterBoxHtml(id, placeholder, label, fields) {
  const names = Object.keys(fields || {});
  const title = `Words match everything. ${names.map((n) => n + ':').join(' ')} narrow to one field; commas list alternatives, a leading - excludes. Type to see suggestions.`;
  return `<div class="fwrap" id="${id}-fwrap"><div class="search fbox" id="${id}-box">${ICON.search}<div class="chip-input fbox-in" id="${id}-wrap"><input class="mono" id="${id}" placeholder="${esc(placeholder)}" title="${esc(title)}" aria-label="${esc(label)}" autocomplete="off" spellcheck="false" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="${id}-sug" /></div><div class="fsug" id="${id}-sug" role="listbox" hidden></div></div>`
    + `<div class="fsaved"><button class="btn ghost fsaved-btn" id="${id}-saved" type="button" aria-haspopup="menu" aria-expanded="false" aria-controls="${id}-menu" title="Saved filters: apply one, or save the current filter under a name">Saved${ICON.caret}</button><div class="menu fsaved-menu" id="${id}-menu" role="menu" hidden></div></div></div>`;
}
// A dropdown anchored to a button: opened by the button, closed by a click
// outside, Escape, or the next route. onOpen fills the menu's innerHTML (it
// may be async) and the caller handles clicks inside it.
function wireMenu(btn, menu, onOpen) {
  if (!btn || !menu) return { close: () => {} };
  let open = false;
  function close() {
    if (!open) return;
    open = false;
    menu.hidden = true;
    btn.setAttribute('aria-expanded', 'false');
    document.removeEventListener('mousedown', onDoc, true);
    document.removeEventListener('keydown', onKey, true);
  }
  function onDoc(e) { if (!menu.contains(e.target) && e.target !== btn && !btn.contains(e.target)) close(); }
  function onKey(e) { if (e.key === 'Escape') { e.preventDefault(); close(); btn.focus(); } }
  btn.addEventListener('click', async () => {
    if (open) { close(); return; }
    await onOpen();
    open = true;
    menu.hidden = false;
    btn.setAttribute('aria-expanded', 'true');
    document.addEventListener('mousedown', onDoc, true);
    document.addEventListener('keydown', onKey, true);
    const first = menu.querySelector('button:not([disabled])');
    if (first) first.focus();
  });
  window.addEventListener('hashchange', close, { once: true });
  return { close };
}
// The named filters kept in settings (GET/PUT /api/settings/saved-filters).
// Loaded on demand; null when the list could not be read.
async function loadSavedFilters() {
  try { return arr((await api('/api/settings/saved-filters')).data); } catch (e) { return null; }
}
async function putSavedFilters(all, title) {
  const r = await api('/api/settings/saved-filters', { method: 'PUT', body: JSON.stringify(all) });
  toastSaved(r.commit, title);
  return arr(r.data);
}
// wireFilter drives a filterBoxHtml control. fields is {name: hint}; values()
// returns {field: [values]} for the suggestions (read when the list opens, so a
// list that refreshes keeps them current); onChange(terms) runs on every
// change. Typing suggests field names, then the values present in the list; a
// complete field term becomes a chip on space, Enter or when its suggestion
// is taken, and Backspace on an empty input pulls the last chip back to edit
// it. Plain words stay in the input and filter as they are typed.
// list names the list for the filters kept outside the page: the last filter
// typed is remembered per list in this browser (gpm.filter.<list>) and put
// back when the list renders again, and the Saved menu offers the named
// filters stored in settings for the list. Returns {terms, set} for a caller
// that re-renders and needs the current terms, or wants to set the query.
function wireFilter(id, fields, values, onChange, list) {
  const input = $('#' + id), wrap = $('#' + id + '-wrap'), sug = $('#' + id + '-sug');
  if (!input) return { terms: () => [], set: () => {} };
  const chips = [];
  let items = [], active = -1, tokStart = 0;
  const query = () => (chips.join(' ') + ' ' + input.value).trim();
  const terms = () => parseFilter(query(), fields);
  const memKey = list ? 'gpm.filter.' + list : '';
  const remember = () => {
    if (!memKey) return;
    try { const q = query(); if (q) localStorage.setItem(memKey, q); else localStorage.removeItem(memKey); } catch (e) { /* ignore */ }
  };
  const fire = () => { remember(); onChange(terms()); };
  const fieldRe = /^(-?)([A-Za-z][\w-]*):(.*)$/;

  function renderChips() {
    $$('.chip-tok', wrap).forEach((el) => el.remove());
    chips.forEach((tok, i) => {
      const el = document.createElement('span');
      el.className = 'chip-tok';
      el.innerHTML = `<span class="chip-tok-t">${esc(tok)}</span><button type="button" aria-label="Remove ${esc(tok)}" tabindex="-1">${ICON.x}</button>`;
      el.querySelector('button').addEventListener('click', () => { chips.splice(i, 1); renderChips(); fire(); input.focus(); });
      wrap.insertBefore(el, input);
    });
  }
  function close() {
    items = []; active = -1;
    sug.hidden = true; sug.innerHTML = '';
    input.setAttribute('aria-expanded', 'false');
    input.removeAttribute('aria-activedescendant');
  }
  function setActive(i) {
    active = i;
    $$('.fsug-item', sug).forEach((el, j) => el.classList.toggle('on', j === i));
    if (i >= 0) input.setAttribute('aria-activedescendant', `${id}-opt-${i}`);
    else input.removeAttribute('aria-activedescendant');
  }
  // Moves every complete field term in the input to a chip. With `all`
  // false only a term followed by a space goes (the one just finished).
  function commit(all) {
    const v = input.value;
    const re = /(?:^|\s)(-?[A-Za-z][\w-]*:(?:"[^"]*"|\S+))(?=\s|$)/g;
    let out = v, m, moved = false;
    const found = [];
    while ((m = re.exec(v))) found.push({ tok: m[1], end: m.index + m[0].length });
    found.forEach((f) => {
      const fm = fieldRe.exec(f.tok);
      if (!fm || !fields[fm[2].toLowerCase()]) return;
      if (!all && !/\s/.test(v.slice(f.end, f.end + 1))) return;
      chips.push(fm[1] + fm[2].toLowerCase() + ':' + fm[3]);
      out = out.replace(f.tok, '');
      moved = true;
    });
    if (!moved) return false;
    input.value = out.replace(/\s{2,}/g, ' ').replace(/^\s+/, '');
    renderChips();
    return true;
  }
  function suggest() {
    const v = input.value;
    const start = v.search(/\S+$/);
    const tok = start === -1 ? '' : v.slice(start);
    tokStart = start === -1 ? v.length : start;
    const neg = tok.charAt(0) === '-';
    const body = neg ? tok.slice(1) : tok;
    const colon = body.indexOf(':');
    items = [];
    if (colon === -1) {
      const typed = body.toLowerCase();
      Object.keys(fields).forEach((f) => {
        if (!typed || f.indexOf(typed) === 0) items.push({ text: (neg ? '-' : '') + f + ':', hint: fields[f], kind: 'field' });
      });
    } else {
      const f = body.slice(0, colon).toLowerCase();
      if (fields[f]) {
        const typed = body.slice(colon + 1).toLowerCase().replace(/^"|"$/g, '');
        const all = (values && values()[f]) || [];
        all.forEach((s) => {
          if (typed && s.indexOf(typed) === -1) return;
          items.push({ text: (neg ? '-' : '') + f + ':' + (/\s/.test(s) ? '"' + s + '"' : s), hint: '', kind: 'value' });
        });
        items = items.slice(0, 12);
      }
    }
    if (!items.length || document.activeElement !== input) { close(); return; }
    sug.innerHTML = items.map((it, i) => `<div class="fsug-item" role="option" id="${id}-opt-${i}" data-i="${i}" aria-selected="false"><span class="mono">${esc(it.text)}</span>${it.hint ? `<span class="faint">${esc(it.hint)}</span>` : ''}</div>`).join('');
    sug.hidden = false;
    input.setAttribute('aria-expanded', 'true');
    setActive(-1);
  }
  function accept(i) {
    const it = items[i];
    if (!it) return;
    const before = input.value.slice(0, tokStart);
    if (it.kind === 'field') {
      input.value = before + it.text;
      suggest();
    } else {
      chips.push(it.text);
      input.value = before.replace(/\s+$/, '');
      renderChips();
      close();
    }
    fire();
  }

  input.addEventListener('input', () => {
    if (commit(false)) close(); else suggest();
    fire();
  });
  input.addEventListener('focus', suggest);
  input.addEventListener('blur', () => setTimeout(close, 120));
  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); if (sug.hidden) suggest(); if (items.length) setActive((active + 1) % items.length); return; }
    if (e.key === 'ArrowUp' && items.length) { e.preventDefault(); setActive((active - 1 + items.length) % items.length); return; }
    if (e.key === 'Escape' && !sug.hidden) { e.preventDefault(); close(); return; }
    if (e.key === 'Enter') {
      e.preventDefault();
      if (active >= 0) { accept(active); return; }
      if (commit(true)) { close(); fire(); }
      return;
    }
    if (e.key === 'Backspace' && !input.value && chips.length) {
      e.preventDefault();
      input.value = chips.pop();
      renderChips();
      suggest();
      fire();
    }
  });
  // mousedown, so the input keeps focus and blur does not close the list
  // before the click lands.
  sug.addEventListener('mousedown', (e) => {
    const el = e.target.closest('.fsug-item');
    if (!el) return;
    e.preventDefault();
    accept(Number(el.dataset.i));
  });
  wrap.addEventListener('click', (e) => { if (e.target === wrap) input.focus(); });

  // Replaces the whole query: field terms become chips, words stay in the box.
  function setQuery(q) {
    chips.length = 0;
    input.value = String(q || '');
    commit(true);
    renderChips();
    close();
    fire();
  }

  // The Saved menu: the named filters for this list, Save current, Clear.
  const sbtn = $('#' + id + '-saved'), smenu = $('#' + id + '-menu');
  if (sbtn && smenu && list) {
    let mine = [];
    const menu = wireMenu(sbtn, smenu, async () => {
      const all = await loadSavedFilters();
      mine = all === null ? [] : all.filter((f) => f.list === list);
      const q = query(), ro = isReadOnly();
      const rows = mine.map((f, i) => `<div class="fsaved-row"><button type="button" role="menuitem" data-apply="${i}" title="${esc(f.query)}"><b>${esc(f.name)}</b><span class="mono">${esc(f.query)}</span></button><button type="button" class="fsaved-del" data-del="${i}" aria-label="Delete saved filter ${esc(f.name)}"${ro ? ' disabled' : ''}>${ICON.x}</button></div>`).join('');
      smenu.innerHTML = (all === null ? '<div class="m-head"><span>Could not load the saved filters.</span></div>' : '')
        + (rows || (all === null ? '' : '<div class="m-head"><span>No saved filters for this list yet.</span></div>'))
        + '<div class="m-sep"></div>'
        + `<button type="button" role="menuitem" class="fsaved-save" data-save="1"${!q || ro ? ' disabled' : ''}>Save current filter...</button>`
        + `<button type="button" role="menuitem" data-clear="1"${q ? '' : ' disabled'}>Clear filter</button>`;
    });
    smenu.addEventListener('click', async (e) => {
      const b = e.target.closest('button');
      if (!b || b.disabled) return;
      if (b.dataset.apply != null) { menu.close(); setQuery(mine[Number(b.dataset.apply)].query); input.focus(); return; }
      if (b.dataset.clear) { menu.close(); setQuery(''); input.focus(); return; }
      if (b.dataset.del != null) {
        const f = mine[Number(b.dataset.del)];
        menu.close();
        try {
          const all = await loadSavedFilters();
          if (all === null) throw new Error('could not load the saved filters');
          await putSavedFilters(all.filter((x) => !(x.list === f.list && x.name === f.name)), 'Deleted');
        } catch (err) { toastErr(err, `Could not delete "${f.name}"`); }
        return;
      }
      if (b.dataset.save) {
        const q = query();
        menu.close();
        const taken = mine.map((f) => f.name.toLowerCase());
        const name = await confirmModal({
          title: 'Save filter',
          body: `<p>Keeps <span class="mono">${esc(q)}</span> under a name in the Saved menu of this list, on every browser you sign in from. Stored in settings, so it shows in History like any change.</p>`,
          prompt: { label: 'Name', placeholder: 'e.g. outside, errors only', validate: (v) => (v.length > 64 ? 'At most 64 characters.' : (taken.indexOf(v.toLowerCase()) !== -1 ? 'A saved filter with this name exists for this list.' : '')) },
          confirmLabel: 'Save', danger: false,
        });
        if (!name) return;
        try {
          const all = await loadSavedFilters();
          if (all === null) throw new Error('could not load the saved filters');
          await putSavedFilters(all.concat([{ name, list, query: q }]), 'Saved');
        } catch (err) { toastErr(err, `Could not save "${name}"`); }
      }
    });
  }

  // The last filter used on this list comes back with the page.
  if (memKey) {
    let stored = '';
    try { stored = localStorage.getItem(memKey) || ''; } catch (e) { /* ignore */ }
    if (stored) setQuery(stored);
  }
  return { terms, set: setQuery };
}
// highlightTerms wraps every match of the filter's words and values inside
// root's visible rows in <mark>. Previous marks are unwrapped first, so it is
// safe to call on every filter pass. Controls and folded "+N" blocks are
// skipped; a hidden row is not touched.
function highlightTerms(root, terms) {
  if (!root) return;
  $$('mark.hl', root).forEach((m) => { m.replaceWith(document.createTextNode(m.textContent)); });
  root.normalize();
  // A network (10.0.0.0/8) or an @list is matched by address, not by text,
  // so there is nothing to mark for it.
  const needles = [];
  terms.forEach((t) => {
    if (t.neg) return;
    (t.values || [t.value]).forEach((v) => {
      if (!v || v.charAt(0) === '@' || (v.indexOf('/') !== -1 && isCidrOrIP(v))) return;
      needles.push(v.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
    });
  });
  if (!needles.length) return;
  const re = new RegExp(needles.join('|'), 'gi');
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(n) {
      const p = n.parentElement;
      if (!p || !/\S/.test(n.nodeValue)) return NodeFilter.FILTER_REJECT;
      if (p.closest('button, input, select, textarea, mark, .more-list, .sort-btn')) return NodeFilter.FILTER_REJECT;
      const row = p.closest('tr, .tl-item');
      if (row && row.style.display === 'none') return NodeFilter.FILTER_REJECT;
      return NodeFilter.FILTER_ACCEPT;
    },
  });
  const nodes = [];
  let n;
  while ((n = walker.nextNode())) nodes.push(n);
  nodes.forEach((node) => {
    const text = node.nodeValue;
    re.lastIndex = 0;
    let m, last = 0, frag = null;
    while ((m = re.exec(text))) {
      if (!m[0]) { re.lastIndex++; continue; }
      if (!frag) frag = document.createDocumentFragment();
      if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
      const mk = document.createElement('mark');
      mk.className = 'hl';
      mk.textContent = m[0];
      frag.appendChild(mk);
      last = m.index + m[0].length;
    }
    if (!frag) return;
    if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
    node.replaceWith(frag);
  });
}
// A DOM-driven filter pass for lists that render once: rows carry data-i (an
// index into recs) or data-name (a key into recs); a miss hides the row. The
// empty-state element, if given, shows when nothing is left. Returns the
// number of rows shown.
function applyRowFilter(rows, recs, terms, emptyEl) {
  let shown = 0;
  rows.forEach((el) => {
    const rec = el.dataset.i != null ? recs[Number(el.dataset.i)] : recs[el.dataset.name];
    const hit = !rec || filterHit(rec, terms);
    el.style.display = hit ? '' : 'none';
    if (hit) shown++;
  });
  if (emptyEl) emptyEl.hidden = shown > 0;
  return shown;
}

// ---------- table sort ----------
// sortThHtml renders one column header. The th keeps the columnheader role
// (that is what aria-sort is valid on); the control inside it is a real
// button. A column without a key is a plain header.
function sortThHtml(col, sort) {
  const cls = [col.key ? 'sortable' : '', col.cls || ''].filter(Boolean).join(' ');
  const clsAttr = cls ? ` class="${cls}"` : '';
  if (!col.key) return `<th${clsAttr}>${esc(col.label)}</th>`;
  const active = sort.key === col.key;
  const aria = active ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none';
  return `<th${clsAttr} aria-sort="${aria}" data-sort="${esc(col.key)}"${col.type ? ` data-type="${esc(col.type)}"` : ''}><button type="button" class="sort-btn" data-sort="${esc(col.key)}">${esc(col.label)}<span class="sort-mark" aria-hidden="true">${active ? (sort.dir === 1 ? '&#9650;' : '&#9660;') : '&#9650;'}</span></button></th>`;
}
// loadSort reads a remembered column choice; one that no longer names a column
// is ignored. stored says whether anything valid was remembered.
function loadSort(storageKey, def, valid) {
  const sort = { key: def.key, dir: def.dir === -1 ? -1 : 1, stored: false };
  try {
    const raw = localStorage.getItem(storageKey);
    if (raw) {
      const p = JSON.parse(raw);
      if (p && p.key && valid(p.key)) { sort.key = p.key; sort.dir = p.dir === -1 ? -1 : 1; sort.stored = true; }
    }
  } catch (e) { /* ignore */ }
  return sort;
}
function saveSort(storageKey, sort) {
  try { localStorage.setItem(storageKey, JSON.stringify({ key: sort.key, dir: sort.dir })); } catch (e) { /* ignore */ }
}
// wireSortHead wires the sort buttons of a rendered head: a click on a column
// sorts by it, a second click flips the direction, and rerender() repaints.
// Keyboard focus stays on the same column after the repaint. The cell's
// padding sorts on a mouse click too.
function wireSortHead(head, sort, storageKey, rerender) {
  $$('.sort-btn', head).forEach((b) => {
    const th = b.closest('th');
    th.addEventListener('click', (e) => { if (e.target === th) b.click(); });
    b.addEventListener('click', () => {
      const key = b.dataset.sort;
      if (sort.key === key) sort.dir = -sort.dir; else { sort.key = key; sort.dir = 1; }
      saveSort(storageKey, sort);
      rerender();
      const nb = head.querySelector(`.sort-btn[data-sort="${CSS.escape(key)}"]`);
      if (nb) nb.focus();
    });
  });
}
// wireTableSort makes a rendered table sortable by its th[data-sort] headers
// without a re-render: the body rows are reordered in place. A cell's data-v
// (a number, or an ISO timestamp) is its sort value, else its text; a blank
// sorts last whichever way. th[data-type="num"] compares numerically. A
// tr.sub-row travels with the row above it. The choice persists under
// storageKey; only a remembered choice reorders at first paint, the server's
// order stands otherwise. Returns apply(), for a caller that replaces the rows
// and wants the same order back.
function wireTableSort(table, storageKey, def) {
  if (!table || !table.tHead || !table.tBodies.length) return { apply() {} };
  const head = table.tHead;
  const cols = $$('th', head).map((th) => ({
    key: th.dataset.sort || null, label: th.textContent.trim(), cls: th.className.replace(/\bsortable\b/, '').trim(), type: th.dataset.type || '',
  }));
  const valid = (k) => cols.some((c) => c.key === k);
  const sort = loadSort(storageKey, def || { key: '', dir: 1 }, valid);
  function apply() {
    const body = table.tBodies[0];
    const idx = cols.findIndex((c) => c.key === sort.key);
    if (idx === -1) return;
    const num = cols[idx].type === 'num';
    const groups = [];
    Array.from(body.rows).forEach((tr) => {
      if (tr.classList.contains('sub-row') && groups.length) groups[groups.length - 1].rows.push(tr);
      else groups.push({ tr, rows: [tr] });
    });
    groups.forEach((g, i) => {
      const td = g.tr.cells[idx];
      g.i = i;
      g.v = td ? (td.dataset.v != null ? td.dataset.v : td.textContent.trim()) : '';
      g.n = num ? parseFloat(g.v) : NaN;
      g.blank = g.v === '' || (num && isNaN(g.n));
    });
    groups.sort((a, b) => {
      if (a.blank !== b.blank) return a.blank ? 1 : -1;
      const c = num ? a.n - b.n : a.v.localeCompare(b.v, undefined, { numeric: true, sensitivity: 'base' });
      return c === 0 ? a.i - b.i : c * sort.dir;
    });
    groups.forEach((g) => g.rows.forEach((tr) => body.appendChild(tr)));
  }
  function renderHead() {
    head.innerHTML = '<tr>' + cols.map((c) => sortThHtml(c, sort)).join('') + '</tr>';
    wireSortHead(head, sort, storageKey, () => { renderHead(); apply(); });
  }
  renderHead();
  if (sort.stored) apply();
  return { apply };
}
// wireMiniTables wires every table[data-sort-key] under root that has not
// been wired yet; a view that renders a status table calls it after painting.
function wireMiniTables(root) {
  $$('table[data-sort-key]', root || document).forEach((t) => {
    if (t.dataset.sortWired) return;
    t.dataset.sortWired = '1';
    wireTableSort(t, t.dataset.sortKey, { key: '', dir: 1 });
  });
}

// Maps a domain to its registrable zone for the hosts list filter chips: the
// last two labels, so www.notes.example.com and portal.internal.example.com
// both belong to example.com. Where the last two labels are a public second
// level (example.co.uk, example.com.au: a two-letter country code under co,
// com, org and the like) the zone keeps one more label. A leading "*." is
// ignored. A deeper name a host serves at its apex is not a zone of its own:
// notes.example.com is a host under example.com, not a zone beside it.
const PUBLIC_SECOND_LEVEL = new Set(['co', 'com', 'net', 'org', 'gov', 'edu', 'ac', 'or', 'ne', 'go', 'ltd', 'plc', 'sch', 'nom', 'mil']);
function domainZone(d) {
  const parts = String(d || '').toLowerCase().replace(/^\*\./, '').split('.').filter(Boolean);
  const two = parts.slice(-2);
  const publicSecond = parts.length > 2 && two[1].length === 2 && PUBLIC_SECOND_LEVEL.has(two[0]);
  return parts.slice(publicSecond ? -3 : -2).join('.');
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
    'auth': 'Sign-in (authentication)',
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
  if (opts.raw !== undefined && opts.raw !== null) {
    // A non-JSON body (the YAML import route), sent as-is under its own type.
    init.headers['Content-Type'] = opts.contentType || 'text/plain';
    init.body = String(opts.raw);
  } else if (opts.body !== undefined && opts.body !== null) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(opts.body);
  }
  if (init.method !== 'GET' && init.method !== 'HEAD') {
    init.headers['X-CSRF-Token'] = csrfToken;
  }
  if (opts.headers) Object.assign(init.headers, opts.headers);
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
// The one commit toast: "Added" after a create, "Saved" after an edit,
// "Deleted" after a delete, each with the commit. The server sends no commit
// when the write changed nothing, so a save without one says "No changes"
// rather than claiming a commit.
function toastSaved(commit, title) {
  const sha = shortSha(commit);
  if (sha) { toast(title || 'Saved', `committed <span class="sha">${esc(sha)}</span>`, 'ok', { html: true }); return; }
  if (!title || title === 'Saved') { toast('No changes', 'The stored config already matches; nothing was committed.', 'ok'); return; }
  toast(title, '', 'ok');
}
// A failure toast is titled with the action that failed ("Could not delete
// notes"); the server's message is the body.
function toastErr(e, title) {
  toast(title || 'Something went wrong', e && e.message ? e.message : String(e), 'err');
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
// A server complaint names the field it is about as a leading path ("tls.hsts
// .maxAge: ..."), optionally behind the object ("proxy host \"app\": ..."); a
// top-level field is one bare segment ("stripResponseHeaders: ..."). The object
// kind may hold capitals ("client CA \"x\": ...") and the first segment an
// index ("upstreams[0].port: ...").
const ERROR_PATH_RE = /^(?:[a-z][A-Za-z ]* "([^"]*)":\s*)?([A-Za-z][A-Za-z0-9]*(?:\[\d+\])?(?:\.[A-Za-z0-9_[\]-]+)*)\s*:\s*/;

function clearEditorBanner() {
  $$('#content .editor-error').forEach((el) => el.remove());
  $$('#content .field-flagged').forEach((el) => el.classList.remove('field-flagged'));
}
function clearEditorError() {
  clearEditorBanner();
  // A row error left from an earlier attempt must not win the scroll over
  // this attempt's banner.
  clearRowErrors($('#content'));
}
// The typed editors' fields carry their model path in data-path too, so a
// server error that names a field ("listenPort: ...") flags it and offers the
// jump. Their help ids already spell that path after the kind
// ("streamHost.listenPort"), so it is derived from data-hint for every field
// without a data-path of its own (the proxy host and Settings set theirs by hand).
const PATH_FROM_HINT_RE = /^(certificate|redirectHost|streamHost|parkedHost|dnsProvider|accessList|identityProvider|middleware|clientCA|upstreamGroup)\.(.+)$/;
const NOT_A_PATH_RE = /^(generate|issue|renew)\.|\.form$|^rewrite\.rule\./;
function pathsFromHints(root) {
  if (!root) return;
  $$('[data-hint]', root).forEach((el) => {
    if (el.dataset.path) return;
    const m = PATH_FROM_HINT_RE.exec(el.dataset.hint || '');
    if (m && !NOT_A_PATH_RE.test(m[2])) el.dataset.path = m[2];
  });
}
function errorFieldFor(msg) {
  const m = ERROR_PATH_RE.exec(String(msg || ''));
  if (!m) return null;
  // A conflict can be reported against the OTHER object ("proxy host \"b\":
  // domains: ... claimed by \"a\"" while editing "a"); flagging this page's
  // field for it would point at the wrong thing.
  const about = m[1] || '';
  const nameEl = document.querySelector('#content [data-path="name"]');
  const editing = nameEl ? String(nameEl.value !== undefined ? nameEl.value : nameEl.textContent).trim() : '';
  if (about && editing && about !== editing) return null;
  const fieldPath = m[2];
  pathsFromHints($('#content'));
  const parts = fieldPath.split('.');
  for (let n = parts.length; n > 0; n--) {
    const path = parts.slice(0, n).join('.');
    const el = document.querySelector(`#content [data-path="${path}"]`);
    if (el) return { el, path: fieldPath };
    // A repeater's fields carry the path without the index (every member row
    // is "upstreams.port"): "upstreams[1].port" is the second of them.
    const idx = /\[(\d+)\]/.exec(path);
    if (idx) {
      const all = document.querySelectorAll(`#content [data-path="${path.replace(/\[\d+\]/g, '')}"]`);
      const hit = all[Number(idx[1])] || all[0];
      if (hit) return { el: hit, path: fieldPath };
    }
  }
  return null;
}
function focusErrorField(target) {
  if (!target || !target.el) return;
  target.el.classList.add('field-flagged');
  revealTabPanel(target.el);
  try { target.el.scrollIntoView({ block: 'center', behavior: 'smooth' }); } catch (e) { target.el.scrollIntoView(); }
  // A field inside a collapsed <details> is worth nothing until it is open.
  let d = target.el.closest && target.el.closest('details');
  while (d) { d.open = true; d = d.parentElement && d.parentElement.closest('details'); }
  // A wrapper (chip input, row, check-list) is not focusable itself: its
  // first control takes the focus instead.
  const fe = target.el.matches && target.el.matches('input, select, textarea, button, [tabindex]')
    ? target.el : (target.el.querySelector && target.el.querySelector('input:not([type=hidden]):not([disabled]), select:not([disabled]), textarea:not([disabled])'));
  if (fe && fe.focus) { try { fe.focus({ preventScroll: true }); } catch (e) { /* ignore */ } }
}
// A client-side check names its field directly: an element, or the field's
// data-path. The jump button says the path when there is one, else the label.
function invalidTarget(t) {
  let el = t;
  if (typeof t === 'string') el = document.querySelector(`#content [data-path="${t}"]`);
  if (!el || !el.isConnected) return null;
  const fg = el.closest && el.closest('.field-group');
  const lab = fg && fg.querySelector('label');
  return { el, path: (el.dataset && el.dataset.path) || '', label: (lab && lab.textContent.trim()) || el.getAttribute('aria-label') || 'field' };
}
// showSaveError(e, title) renders the banner and returns it. Call clearEditorError()
// at the top of every save handler so a stale failure never outlives its cause.
function showSaveError(e, title, field) {
  const c = $('#content');
  if (!c) { toastErr(e, title); return null; }
  // The banner replaces the previous one; a row this attempt just marked
  // (markRowError) keeps its mark - the save handler cleared stale ones first.
  clearEditorBanner();
  const msg = (e && e.message) ? e.message : String(e);
  const target = (field && invalidTarget(field)) || errorFieldFor(msg);
  const banner = document.createElement('div');
  banner.className = 'editor-error';
  banner.setAttribute('role', 'alert');
  banner.innerHTML = `<div class="ee-body">
      <div class="ee-title">${esc(title || 'Could not save')}</div>
      <div class="ee-msg">${esc(msg)}</div>
      ${target ? `<div class="ee-jump"><button class="btn ghost sm" type="button" id="ee-jump">Go to ${target.path ? `<span class="mono">${esc(target.path)}</span>` : esc(target.label)}</button></div>` : ''}
    </div>
    <button class="ee-close" type="button" aria-label="Dismiss">&times;</button>`;
  c.insertBefore(banner, c.firstChild);
  banner.querySelector('.ee-close').addEventListener('click', () => clearEditorError());
  const jump = banner.querySelector('#ee-jump');
  if (jump) jump.addEventListener('click', () => focusErrorField(target));
  // A row error (markRowError) is already marked on its row: keep that row in
  // view rather than jumping to the banner at the top of a long page.
  const badRow = target ? null : c.querySelector('.row-bad');
  if (target) focusErrorField(target);
  else if (badRow) focusErrorField({ el: badRow });
  else scrollToTop();
  return banner;
}

// A blocking validation message is the same page state as a failed save: the
// operator has to read it, fix a field and try again, which a 7-second toast
// does not leave time for. So it gets the same sticky banner. field is the
// control the message is about (an element or its data-path): it is flagged,
// its fold opened and its tab shown, wherever the Save was pressed from.
function showInvalid(title, msg, field) { showSaveError(new Error(msg), title, field); }

// Create is create-only: every new object's PUT carries If-None-Match: *, so a
// name that is already stored answers 409 instead of being replaced (a PUT is
// otherwise an upsert). showNameTaken turns that 409 into the editor banner,
// with a link to the stored object when it has a page of its own (href), and
// flags the name field. Returns false for any other error.
function showNameTaken(e, singular, name, href, title, nameEl) {
  if (!e || e.status !== 409) return false;
  const article = /^[aeiou]/i.test(singular) ? 'An' : 'A';
  const banner = showSaveError(new Error(`${article} ${singular} named ${name} already exists.`), title);
  const msg = banner && banner.querySelector('.ee-msg');
  if (msg) {
    msg.innerHTML = `${article} ${esc(singular)} named <b class="mono">${esc(name)}</b> already exists. `
      + (href ? `Pick another name, or open <a href="${esc(href)}">${esc(name)}</a> to edit it.` : 'Pick another name.');
  }
  if (nameEl) nameEl.classList.add('field-flagged');
  return true;
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

// Focus goes back to whatever had it before a dialog or wizard opened. When that
// element is gone or hidden by then (a route change removed it, a menu closed),
// the page title takes it, as a nav click does, so the next Tab starts on the
// page rather than at the top of the document.
function restoreFocus(prev) {
  if (prev && prev.isConnected && prev !== document.body && prev.focus && prev.getClientRects().length) {
    try { prev.focus(); } catch (e) { /* ignore */ }
    if (document.activeElement === prev) return;
  }
  const t = $('#pageTitle');
  if (t) { t.tabIndex = -1; try { t.focus(); } catch (e) { /* ignore */ } }
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
//   keepOnNavigate  stays open on a navigation (only the unsaved-changes guard,
//                which is itself the answer to that navigation).
// Resolves false on Cancel, Escape, a scrim click and a navigation (browser
// Back, a link): a dialog never outlives the page it was opened for, and a
// dismissed dialog is never a confirmation. Enter confirms only from the
// dialog's own inputs or its confirm button, so a button placed in the body
// (Copy) answers its own Enter. Footer: the confirm button first, then Cancel.
function confirmModal(opts) {
  opts = opts || {};
  return new Promise((resolve) => {
    const prevFocus = document.activeElement;
    const wrap = document.createElement('div');
    wrap.className = 'modal-scrim';
    wrap.innerHTML = `
      <div class="modal${opts.wide ? ' wide' : ''}" role="dialog" aria-modal="true" aria-labelledby="cm-title">
        <div class="modal-title" id="cm-title">${esc(opts.title || 'Are you sure?')}</div>
        <div class="modal-body">${opts.body || ''}</div>
        ${opts.typed ? `<div class="field-group modal-typed">
          <label for="cm-typed">Type <span class="mono">${esc(opts.typed)}</span> to confirm</label>
          <input class="field mono" id="cm-typed" autocomplete="off" spellcheck="false" aria-label="Type ${esc(opts.typed)} to confirm" />
        </div>` : ''}
        ${opts.prompt ? `<div class="field-group modal-typed">
          <label for="cm-prompt">${esc(opts.prompt.label || 'Value')}</label>
          <input class="field mono" id="cm-prompt" autocomplete="off" spellcheck="false" value="${esc(opts.prompt.value || '')}" placeholder="${esc(opts.prompt.placeholder || '')}" aria-label="${esc(opts.prompt.label || 'Value')}" />
          <div class="hint warn" id="cm-prompt-err" hidden></div>
        </div>` : ''}
        <div class="modal-actions">
          <button class="btn ${opts.danger === false ? 'primary' : 'danger'}" id="cm-ok" type="button">${esc(opts.confirmLabel || 'Confirm')}</button>
          ${opts.closeOnly ? '' : '<button class="btn ghost" id="cm-cancel" type="button">Cancel</button>'}
        </div>
      </div>`;
    document.body.appendChild(wrap);

    const okBtn = wrap.querySelector('#cm-ok');
    const cancelBtn = wrap.querySelector('#cm-cancel');
    const typedInput = wrap.querySelector('#cm-typed');
    const promptInput = wrap.querySelector('#cm-prompt');
    let done = false;
    function finish(v) {
      if (done) return;
      done = true;
      document.removeEventListener('keydown', onKey, true);
      window.removeEventListener('hashchange', onHash);
      wrap.remove();
      restoreFocus(prevFocus);
      resolve(v);
    }
    function onHash() { if (!opts.keepOnNavigate) finish(false); }
    // A prompt's validate(value) answers with the complaint, or '' when the
    // value is acceptable; the complaint shows under the field and holds the
    // confirm button.
    function promptError() {
      if (!opts.prompt || !opts.prompt.validate || !promptInput) return '';
      const v = promptInput.value.trim();
      return v ? (opts.prompt.validate(v) || '') : '';
    }
    function typedOK() {
      if (opts.typed && !(typedInput && typedInput.value.trim() === opts.typed)) return false;
      if (opts.prompt && !(promptInput && promptInput.value.trim())) return false;
      if (promptError()) return false;
      return true;
    }
    // Confirming a prompt dialog answers with the value, not with `true`.
    function answer() { return promptInput ? promptInput.value.trim() : true; }
    function refresh() {
      const errEl = wrap.querySelector('#cm-prompt-err');
      if (errEl) { const err = promptError(); errEl.hidden = !err; errEl.textContent = err; }
      okBtn.disabled = !typedOK();
    }
    function onKey(e) {
      if (e.key === 'Escape') { e.preventDefault(); finish(false); return; }
      if (e.key === 'Enter' && (e.target === okBtn || e.target === typedInput || e.target === promptInput)) {
        e.preventDefault();
        if (typedOK()) finish(answer());
        return;
      }
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
    window.addEventListener('hashchange', onHash);
    wrap.addEventListener('click', (e) => { if (e.target === wrap) finish(false); });
    if (cancelBtn) cancelBtn.addEventListener('click', () => finish(false));
    okBtn.addEventListener('click', () => { if (typedOK()) finish(answer()); });
    if (typedInput) typedInput.addEventListener('input', refresh);
    if (promptInput) promptInput.addEventListener('input', refresh);
    if (typedInput || promptInput) {
      refresh();
      (promptInput || typedInput).focus();
      // A prefilled prompt is a suggestion: typing replaces it.
      if (promptInput && promptInput.value) promptInput.select();
    }
    // A destructive dialog opens on Cancel, so a stray or repeated Enter never
    // confirms it; a non-destructive one opens on its confirm button.
    else (opts.danger === false || !cancelBtn ? okBtn : cancelBtn).focus();
    if (opts.onOpen) opts.onOpen(wrap);
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
// For an action that re-renders the page itself (not through a hash change, so
// onHashChange's guard never runs): ask before unsaved edits are thrown away.
async function confirmDiscardEdits() {
  if (!dirtyFlag) return true;
  return confirmModal({
    title: 'You have unsaved changes',
    body: '<p>This action reloads the page and discards them.</p>',
    confirmLabel: 'Discard and continue',
  });
}
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
// Only the sidebar and the Overview empty-state summary read them, so a failed fetch
// degrades to "everything empty" rather than an error. The response keys are the
// kebab-case API plurals; state.counts keeps its camelCase names because other
// code (navGroupDefaultOpen, the Overview summary) reads them.
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
  // The glyph plus the mode's name in text, so the state is readable without
  // hovering for the tooltip (keyboard and touch users never see one).
  const label = 'Theme: ' + themeLabel(t) + '. Click to change; auto follows your OS.';
  btn.innerHTML = (t === 'light' ? ICON.sun : t === 'dark' ? ICON.moon : ICON.contrast)
    + `<span class="tool-label" aria-hidden="true">${themeLabel(t)}</span>`;
  btn.setAttribute('aria-label', label);
  btn.title = label;
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
// Text only: the PatternFly nav carries no per-item icon, and the group
// headings already give the eye its anchors. The icon stays in NAV_GROUPS for
// anything else that wants a glyph for a section.
function navItemHtml(n) {
  return `<button class="nav-item" data-view="${n.id}">${esc(n.label)}</button>`;
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
  // PatternFly page frame: the masthead spans the full width, the sidebar sits
  // beneath it, the main column opens with a title band the views fill.
  app.innerHTML = `
    <div class="app">
      <a class="skip-link" id="skipLink" href="#content">Skip to content</a>
      <header class="masthead">
        <button class="menu-btn" id="menuBtn" type="button" aria-label="Open navigation" aria-controls="sidebar" aria-expanded="false">${ICON.menu}</button>
        <a class="wordmark" href="#/overview" title="${esc(state.appName)}">
          <span class="logo" aria-hidden="true">${ICON.fanout}</span>
          <span class="name">${esc(state.appName)}</span>
        </a>
        <div class="spacer"></div>
        <div class="tools">
          <button class="tool-btn" id="themeBtn" type="button"></button>
          <button class="user-btn" id="userBtn" type="button" aria-expanded="false" aria-controls="userMenu" aria-label="Account menu"><span class="avatar" id="avatar">?</span>${ICON.caret}</button>
        </div>
        <div class="menu" id="userMenu" hidden></div>
      </header>
      <div class="scrim" id="scrim"></div>
      <div class="page-body">
        <aside class="sidebar" id="sidebar">
          <nav class="nav" id="nav" aria-label="Primary">
            ${NAV_GROUPS.map(navGroupHtml).join('')}
          </nav>
          <div class="side-foot" id="sideFoot"></div>
        </aside>
        <main class="main" id="main">
          <div class="page-head" id="pageHead">
            <div class="page-crumb" id="pageCrumb"></div>
            <div class="head-row"><div><h1 class="page-title" id="pageTitle" tabindex="-1">${esc(state.appName)}</h1><p class="page-sub" id="pageSub"></p><div class="head-labels" id="pageLabels"></div></div><div class="head-actions" id="pageActions"></div></div>
            <div class="tabs page-tabs" id="pageTabs" role="tablist"></div>
          </div>
          <div class="content" id="content"></div>
        </main>
      </div>
    </div>`;

  $$('#nav .nav-item').forEach((b) => {
    b.addEventListener('click', () => {
      location.hash = '#/' + b.dataset.view;
      closeNav();
      // The drawer hides with focus still inside it on phones; park focus on
      // the page title instead.
      const t = $('#pageTitle'); t.tabIndex = -1; t.focus();
    });
  });
  // The collapsible group header is a real <button>, so Enter/Space and the tab
  // order come from the platform rather than from a keydown handler here.
  $$('#nav .nav-group-head').forEach((b) => {
    b.addEventListener('click', () => setNavGroupOpen(b.dataset.group, b.getAttribute('aria-expanded') !== 'true', true));
  });
  // The skip link is a hash link only for the platform's sake: the router owns
  // the hash, so the jump is done here and lands on the page title.
  $('#skipLink').addEventListener('click', (e) => {
    e.preventDefault();
    const t = $('#pageTitle');
    if (t) t.focus();
  });
  // The hamburger is a toggle: a second tap closes the drawer it opened.
  $('#menuBtn').addEventListener('click', () => {
    if (document.body.classList.contains('nav-open')) { closeNav(); return; }
    openNav();
  });
  $('#scrim').addEventListener('click', closeNav);
  document.addEventListener('keydown', (e) => {
    if (!document.body.classList.contains('nav-open')) return;
    // A dialog or wizard above the drawer owns the keys: its own trap must not
    // be undercut by moving focus into the sidebar behind it.
    if (document.querySelector('.modal-scrim')) return;
    if (e.key === 'Escape' && !e.defaultPrevented) {
      closeNav();
      $('#menuBtn').focus();
      return;
    }
    // While the drawer is open the page behind the scrim is inert, and Tab
    // cycles through the hamburger and the drawer only.
    if (e.key !== 'Tab') return;
    const ring = [$('#menuBtn')].concat($$('#sidebar a, #sidebar button').filter((el) => el.offsetParent !== null && !el.disabled));
    const i = ring.indexOf(document.activeElement);
    const n = ring.length;
    e.preventDefault();
    ring[i === -1 ? 0 : (i + (e.shiftKey ? n - 1 : 1)) % n].focus();
  });
  // Widening the window past the drawer breakpoint while the drawer is open
  // would leave the page inert behind a sidebar that is no longer a drawer.
  if (window.matchMedia) {
    const mq = window.matchMedia('(min-width: 801px)');
    const onWide = () => { if (mq.matches) closeNav(); };
    if (mq.addEventListener) mq.addEventListener('change', onWide); else if (mq.addListener) mq.addListener(onWide);
  }
  updateThemeBtn();
  $('#themeBtn').addEventListener('click', () => {
    const order = ['auto', 'light', 'dark'];
    setTheme(order[(order.indexOf(getTheme()) + 1) % order.length]);
  });
  // Account menu: a PatternFly dropdown under the avatar. Click outside or
  // Escape closes it; a chosen entry closes it too (the hash change renders).
  const ub = $('#userBtn'), um = $('#userMenu');
  ub.addEventListener('click', (e) => {
    e.stopPropagation();
    setUserMenu(um.hidden);
    if (!um.hidden) { const f = um.querySelector('a, button'); if (f) f.focus(); }
  });
  document.addEventListener('click', (e) => {
    if (!um.hidden && !um.contains(e.target) && e.target !== ub && !ub.contains(e.target)) setUserMenu(false);
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !um.hidden && !e.defaultPrevented) { setUserMenu(false); ub.focus(); }
  });
  // Tabbing out of the menu closes it, so it never lingers over the page.
  um.addEventListener('focusout', (e) => {
    const to = e.relatedTarget;
    if (to && (um.contains(to) || to === ub)) return;
    setUserMenu(false);
  });
}
function setUserMenu(open) {
  const ub = $('#userBtn'), um = $('#userMenu');
  if (!ub || !um) return;
  um.hidden = !open;
  ub.setAttribute('aria-expanded', open ? 'true' : 'false');
}
function openNav() {
  document.body.classList.add('nav-open');
  const b = $('#menuBtn');
  b.setAttribute('aria-expanded', 'true');
  b.setAttribute('aria-label', 'Close navigation');
  const m = $('#main');
  if (m) m.inert = true;
  // The drawer is visibility:hidden while closed (out of the tab order), so
  // keyboard focus is moved into it on open.
  const first = $('#nav .nav-item.active') || $('#nav .nav-item');
  if (first) first.focus();
}
function closeNav() {
  document.body.classList.remove('nav-open');
  const b = document.getElementById('menuBtn');
  if (b) { b.setAttribute('aria-expanded', 'false'); b.setAttribute('aria-label', 'Open navigation'); }
  const m = document.getElementById('main');
  if (m) m.inert = false;
}

async function loadTopbar() {
  // Best-effort and each independent, so they are issued together instead of
  // one round trip after another: the shell used to wait for six sequential
  // GETs before the first view could render.
  let verStr = '', cfgSha = '';
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
  // A build without a commit reports a placeholder ("none"); only a real hash is
  // shown, and not twice: a dev build is already versioned "dev-<sha>".
  if (v) {
    const sha = /^[0-9a-f]{7,}$/i.test(v.commit || '') ? shortSha(v.commit) : '';
    const ver = v.version || '';
    const dup = sha && ver.toLowerCase().includes(sha.toLowerCase());
    verStr = `${ver ? 'Version ' + ver : ''}${sha && !dup ? ' (' + sha + ')' : ''}`.trim();
  }

  const h = hR && hR.data;
  if (Array.isArray(h) && h.length) { state.headSha = h[0].hash; cfgSha = shortSha(h[0].hash); }

  const me = meR && meR.data;
  state.me = me;
  let name = '', role = '', idp = '';
  if (me) {
    csrfToken = me.csrfToken || '';
    name = me.Name || me.Subject || me.Email || 'user';
    role = me.Role || '';
    idp = me.IdP || '';
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
    // The masthead ellipsizes a long name; the whole one is the link's tooltip.
    const wm = document.querySelector('.wordmark');
    if (wm) wm.title = s.appName;
    document.title = s.appName;
  }

  // Who is signed in lives in the account menu, not on the masthead; the
  // running version and the config commit live at the foot of the sidebar.
  const av = $('#avatar');
  if (av) av.textContent = state.avatarChar || 'g';
  const who = me ? [role ? esc(role) : '', idp ? 'via ' + esc(idp) : ''].filter(Boolean).join(', ') : '';
  const um = $('#userMenu');
  um.innerHTML = `
    <div class="m-head"><b>${esc(name || 'Not signed in')}</b>${who ? `<span>${who}</span>` : ''}${state.instance ? `<span class="mono" title="This instance">${esc(state.instance)}</span>` : ''}</div>
    <a href="#/tokens">API tokens</a>
    <a href="#/history">History</a>
    ${isReadOnly() ? '' : '<button type="button" id="setupBtn">Setup wizard</button>'}
    <div class="m-sep" role="separator"></div>
    <button type="button" id="logoutBtn">Sign out</button>`;
  $('#logoutBtn').addEventListener('click', logout);
  const sb = $('#setupBtn');
  if (sb) sb.addEventListener('click', () => { setUserMenu(false); openSetupWizard(); });
  um.querySelectorAll('a').forEach((a) => a.addEventListener('click', () => setUserMenu(false)));
  $('#sideFoot').innerHTML = `<span title="Build version">${esc(verStr)}</span>`
    + `<a href="#/history" id="sideCommit" title="Current config commit, opens history">${esc(cfgSha)}</a>`;
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
  // A failure that lands after route() has moved on to another render belongs
  // to the abandoned view, not to the list the new view is collecting.
  const owner = state.refListFailed;
  return api(path).then((r) => arr(r.data)).catch(() => {
    if (owner !== state.refListFailed) return null;
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
  // Save only: deleting an object does not depend on a reference list, and
  // greying Delete out under a message about saving would just read as a bug.
  // A view with no Save (a list) gets a banner that does not mention one.
  const saves = container.querySelectorAll('#saveBtn, #ed-save, #ct-save, #set-save, .set-save, .int-save');
  const msg = saves.length
    ? `Could not load ${failed.join(', ')}; save is disabled to avoid stripping references. Reload the page to try again.`
    : `Could not load ${failed.join(', ')}; some details on this page may be missing. Reload the page to try again.`;
  const banner = document.createElement('div');
  banner.className = 'ro-banner warn';
  banner.innerHTML = `<b>Reference list unavailable.</b> ${esc(msg)}`;
  container.prepend(banner);
  saves.forEach((b) => gateControl(b, false, msg));
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
  '.btn.primary', '.btn.danger', '.btn.danger-soft', '[data-revert]', '[data-revert-obj]',
  '.tok-rotate', '.tok-toggle', '#restoreBtn', '#backupBtn', '#set-save', '.set-save', '#errp-save',
  '#set-sso-revoke', '#set-dns-run', '#set-id-run', '#set-dkr-run', '#int-als-run',
  '#ed-src-reconcile', '#logsToggle', '#logsClear', '.wh-test', '.ntf-test', '.ct-renew', '#ed-renew',
  '.al-migrate', '#set-metrics-link',
  // Saving or deleting a named filter writes settings.
  '.fsaved-save', '.fsaved-del',
  // Clone leads to an editor whose Save a read-only caller cannot use, so it is
  // gated like the proxy host band's Clone.
  '#ed-clone', '#ct-clone',
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
    banner.innerHTML = '<b>Read-only access.</b> Your role can view everything and change nothing.';
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
    + '<li>Or add an <span class="mono">oidc</span> identity provider and tick it under <a href="#/settings/general">Admin sign-in &gt; Single sign-on providers</a> <span class="mono muted">(adminAuth.providers)</span>.</li></ul>'
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
    + 'Serve the panel over HTTPS, or set <a href="#/settings/general">Public URL of this panel</a> to an '
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
  return `<div class="inline-error"><b>Could not load this view. Reload the page.</b><br/>${esc(msg)}</div>`;
}
// The title band above the content belongs to the shell; every view fills it
// (title, one summary line, an optional breadcrumb) instead of rendering a
// heading of its own, so a page title is never shown twice. Returns '' so a
// view can drop the call into its template where the old heading stood.
// Arguments are HTML: callers escape their data.
// The band is written only when the view's own content lands: pageHead()
// parks the values, and route()'s content setter applies them together with
// the innerHTML it accepts, or drops them when it refuses a stale render. A
// view that finished loading after the operator moved on can therefore never
// put its title over another page.
let pendingHead = null;
// actionsHtml and tabsHtml are the band's object actions and tab bar; an edit
// view passes them, every other view leaves them empty and the band clears.
function pageHead(titleHtml, subHtml, crumbHtml, titleAttr, labelsHtml, actionsHtml, tabsHtml) {
  pendingHead = { titleHtml, subHtml, crumbHtml, titleAttr, labelsHtml, actionsHtml, tabsHtml };
  return '';
}
function applyHead(h) {
  const t = $('#pageTitle'), sb = $('#pageSub'), c = $('#pageCrumb'), l = $('#pageLabels');
  const a = $('#pageActions'), tb = $('#pageTabs');
  if (t) {
    t.innerHTML = h.titleHtml || esc(state.appName);
    if (h.titleAttr) t.title = h.titleAttr; else t.removeAttribute('title');
  }
  if (sb) sb.innerHTML = h.subHtml || '';
  if (c) c.innerHTML = h.crumbHtml || '';
  if (l) l.innerHTML = h.labelsHtml || '';
  if (a) a.innerHTML = h.actionsHtml || '';
  if (tb) tb.innerHTML = h.tabsHtml || '';
}
function viewHead(title, sub, actionHtml) {
  pageHead(esc(title), glossaryize(esc(sub)));
  return `<div class="view-head">${actionHtml || ''}</div>`;
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
  // Not a button: nothing happens on Enter. It is a focusable term whose
  // definition opens on focus (and hover) and is announced as its description.
  return `<span class="gloss" data-hint="${esc(id)}" tabindex="0" role="term" aria-describedby="hint-pop">${esc(word)}</span>`;
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
  return `<button type="button" class="hint-btn" data-hint-id="${esc(id)}" aria-expanded="false" aria-controls="hint-pop"`
    + ` aria-label="Help for ${esc(label || id)}">?</button>`;
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
      if (el.classList.contains('gloss')) return;
      const target = hintAnchorFor(el);
      if (!target) { if (!el.getAttribute('title')) el.setAttribute('title', entry.text); return; }
      const label = (target.textContent || el.getAttribute('aria-label') || '').trim();
      // A <label> gets the "?" as its next sibling, not as a child: inside it,
      // the button would be the label's labelable descendant, so a click on
      // the label text opened the popover instead of focusing the field.
      // Same for a fold's heading: <summary> allows phrasing content only and
      // must not hold another interactive element, so the "?" goes right after
      // it, at the top of the open section.
      const after = target.tagName === 'LABEL' ? target : target.closest('summary');
      if (after) {
        const next = after.nextElementSibling;
        if (next && next.classList.contains('hint-btn')) return;
        after.insertAdjacentHTML('afterend', hintHtml(id, label));
        return;
      }
      if (target.querySelector('.hint-btn')) return;
      target.insertAdjacentHTML('beforeend', hintHtml(id, label));
    });
    labelFieldGroups(root);
  } finally {
    if (hintObserver) hintObserver.takeRecords();
    hintDecorating = false;
  }
}

// Ties each field-group's <label> to its control with for/id, so clicking the
// label focuses the field and the control gets the label as its accessible
// name. Only a group with exactly one control of its own is tied: a group that
// holds a repeater, a check-list or a matrix has no single control to name, and
// those controls carry their own aria-label or wrapping label.
let labelSeq = 0;
function labelFieldGroups(root) {
  root.querySelectorAll('.field-group > label:not([for])').forEach((lab) => {
    const fg = lab.parentElement;
    const ctls = Array.from(fg.querySelectorAll('input:not([type="hidden"]), select, textarea'))
      .filter((el) => el.closest('.field-group') === fg && !el.closest('label, .check-list, table'));
    if (ctls.length !== 1) return;
    const ctl = ctls[0];
    if (!ctl.id) ctl.id = 'fld-' + (++labelSeq);
    lab.htmlFor = ctl.id;
  });
}

// ---------- help popover ----------
// One popover element for the whole app: opening a second closes the first, and
// Escape or a click anywhere else dismisses it. It is inserted right after its
// owner in the DOM, so Tab from the "?" or the glossary term reaches "Learn
// more" inside it (also inside a wizard, whose focus trap only cycles through
// its own dialog), and it closes once focus leaves the owner and popover pair.
// position:fixed keeps it clear of the overflow and stacking of whatever the
// owner sits in; it follows the owner on scroll.
let hintPopOwner = null;
let hintPopCloseTimer = 0;
function hintPopEl() {
  let el = document.getElementById('hint-pop');
  if (!el) {
    el = document.createElement('div');
    el.id = 'hint-pop';
    el.className = 'hint-pop';
    el.setAttribute('role', 'dialog');
    el.setAttribute('aria-label', 'Help');
    el.hidden = true;
    // A click inside the popover is not a click on the row or card it sits in.
    el.addEventListener('click', (e) => e.stopPropagation());
    el.addEventListener('mouseenter', () => clearTimeout(hintPopCloseTimer));
    el.addEventListener('mouseleave', () => scheduleHintPopClose());
  }
  return el;
}
function closeHintPop() {
  clearTimeout(hintPopCloseTimer);
  const el = document.getElementById('hint-pop');
  if (el) el.hidden = true;
  if (hintPopOwner) {
    if (hintPopOwner.classList.contains('hint-btn')) hintPopOwner.setAttribute('aria-expanded', 'false');
    hintPopOwner = null;
  }
}
// The pointer may cross the gap between a glossary term and its popover: the
// close waits a moment and is dropped while the pointer or the focus is on
// either of them.
function scheduleHintPopClose() {
  clearTimeout(hintPopCloseTimer);
  hintPopCloseTimer = setTimeout(() => {
    const el = document.getElementById('hint-pop');
    const owner = hintPopOwner;
    if (!owner || !el) return;
    const inPair = (n) => n && (owner.contains(n) || el.contains(n));
    if (owner.matches(':hover') || el.matches(':hover') || inPair(document.activeElement)) return;
    closeHintPop();
  }, 300);
}
function placeHintPop() {
  const el = document.getElementById('hint-pop');
  if (!el || el.hidden || !hintPopOwner) return;
  if (!hintPopOwner.isConnected) { closeHintPop(); return; }
  const r = hintPopOwner.getBoundingClientRect();
  const vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight;
  const w = el.offsetWidth, h = el.offsetHeight;
  const left = Math.max(12, Math.min(r.left, vw - w - 12));
  let top = r.bottom + 6;
  if (top + h > vh - 8 && r.top - h - 6 > 8) top = r.top - h - 6;
  el.style.left = left + 'px';
  el.style.top = top + 'px';
}
function openHintPop(owner, id) {
  const e = HINTS[id];
  if (!e) return;
  clearTimeout(hintPopCloseTimer);
  const el = hintPopEl();
  const bullets = Array.isArray(e.bullets) ? e.bullets : [];
  el.innerHTML = `<p>${esc(e.text)}</p>`
    + (bullets.length ? `<ul>${bullets.map((b) => `<li>${esc(b)}</li>`).join('')}</ul>` : '')
    + (e.doc ? `<a href="${esc(docHref(e.doc))}" target="_blank" rel="noopener">Learn more</a>` : '');
  // Right after the owner, or after the element the owner sits in when that
  // element must not hold the popover: a <summary> (a click would toggle the
  // fold), a <label> (it would forward the click), a button or a link.
  const host = (owner.parentElement && owner.parentElement.closest('summary, label, button, a')) || owner;
  if (host.nextElementSibling !== el) host.insertAdjacentElement('afterend', el);
  el.hidden = false;
  hintPopOwner = owner;
  if (owner.classList.contains('hint-btn')) owner.setAttribute('aria-expanded', 'true');
  placeHintPop();
}
window.addEventListener('scroll', placeHintPop, true);
window.addEventListener('resize', placeHintPop);
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
// Escape closes it; focus that was inside the popover goes back to its owner.
function dismissHintPop() {
  const owner = hintPopOwner;
  const el = document.getElementById('hint-pop');
  const inside = el && el.contains(document.activeElement);
  closeHintPop();
  if (inside && owner && owner.isConnected) owner.focus();
}
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && hintPopOwner) dismissHintPop();
});
// Glossary terms are hover/focus, not click: they explain a word in a sentence
// rather than opening a panel about a control.
document.addEventListener('mouseover', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (!g) return;
  if (hintPopOwner === g) { clearTimeout(hintPopCloseTimer); return; }
  closeHintPop(); openHintPop(g, g.getAttribute('data-hint'));
});
document.addEventListener('mouseout', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g && hintPopOwner === g) scheduleHintPopClose();
});
document.addEventListener('focusin', (e) => {
  const g = e.target.closest && e.target.closest('.gloss');
  if (g && hintPopOwner !== g) { closeHintPop(); openHintPop(g, g.getAttribute('data-hint')); }
});
// Focus leaving the owner and popover pair closes it. A focusout with no
// related target (a click on plain text) is left to the click handler.
document.addEventListener('focusout', (e) => {
  const owner = hintPopOwner;
  const el = document.getElementById('hint-pop');
  if (!owner || !el || el.hidden) return;
  const from = e.target, to = e.relatedTarget;
  if (!(owner.contains(from) || el.contains(from)) || !to) return;
  if (owner.contains(to) || el.contains(to)) return;
  closeHintPop();
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
  // chipchange bubbles so a fold summary around the box can follow it: adding
  // or removing a chip fires no input or change event of its own.
  function changed() {
    if (onChange) onChange(tokens.slice());
    container.dispatchEvent(new CustomEvent('chipchange', { bubbles: true }));
  }
  function render() {
    // Keep the input's id across re-renders (the container's own, or the one
    // labelFieldGroups assigned), so a label's for= keeps pointing at it.
    const prev = container.querySelector('input');
    const inputId = (prev && prev.id) || (container.id ? container.id + '-in' : '');
    container.innerHTML = tokens.map((t, i) =>
      `<span class="chip-tok" title="${esc(t)}"><span class="chip-tok-t">${esc(t)}</span><button type="button" aria-label="Remove ${esc(t)}" data-i="${i}">&times;</button></span>`
    ).join('') + `<input class="mono" placeholder="${esc(placeholder || 'add...')}" ${inputId ? `id="${esc(inputId)}" ` : ''}aria-label="${esc(placeholder || 'add')}" />`;
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
    // Text typed but not committed with Enter or "," is still meant: commit it
    // when focus leaves the box (unless it moved to one of this box's own
    // remove buttons), and get() below includes it, so Save never drops it.
    input.addEventListener('blur', (e) => {
      // Moving to this box's own buttons, or straight to Save (which reads
      // the pending text itself), must not redraw the box under the click.
      if (e.relatedTarget && (container.contains(e.relatedTarget) || e.relatedTarget.closest('.save-bar'))) return;
      const v = pending();
      if (v && tokens.indexOf(v) === -1) { tokens.push(v); render(); changed(); }
    });
    container.querySelectorAll('.chip-tok button').forEach((b) => {
      b.addEventListener('click', () => { tokens.splice(parseInt(b.dataset.i, 10), 1); render(); changed(); });
    });
  }
  function pending() {
    const i = container.querySelector('input');
    return i ? i.value.trim().replace(/,$/, '') : '';
  }
  render();
  return { get: () => { const v = pending(); return v && tokens.indexOf(v) === -1 ? tokens.concat(v) : tokens.slice(); } };
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
// ipBytes parses an IPv4 or IPv6 address to its bytes (4 or 16), or null. An
// IPv4-mapped IPv6 address (::ffff:10.0.0.1) comes back as 4 bytes, so a
// client logged either way matches an IPv4 network.
function ipBytes(s) {
  s = String(s == null ? '' : s).trim().toLowerCase();
  if (!s) return null;
  if (s.indexOf(':') === -1) {
    const p = s.split('.');
    if (p.length !== 4 || !p.every((x) => /^\d{1,3}$/.test(x) && parseInt(x, 10) <= 255)) return null;
    return p.map((x) => parseInt(x, 10));
  }
  if (!/^[0-9a-f:.]+$/.test(s)) return null;
  let head = s, tail = '';
  const dbl = s.indexOf('::');
  if (dbl !== -1) {
    if (s.indexOf('::', dbl + 1) !== -1) return null;
    head = s.slice(0, dbl);
    tail = s.slice(dbl + 2);
  }
  let h = head ? head.split(':') : [], t = tail ? tail.split(':') : [];
  // A trailing dotted quad (::ffff:1.2.3.4) is two groups.
  const last = t.length ? t[t.length - 1] : (h.length ? h[h.length - 1] : '');
  if (last.indexOf('.') !== -1) {
    const v4 = ipBytes(last);
    if (!v4) return null;
    const rep = [((v4[0] << 8) | v4[1]).toString(16), ((v4[2] << 8) | v4[3]).toString(16)];
    if (t.length) t = t.slice(0, -1).concat(rep); else h = h.slice(0, -1).concat(rep);
  }
  if (h.concat(t).some((g) => !/^[0-9a-f]{1,4}$/.test(g))) return null;
  const fill = 8 - h.length - t.length;
  if (dbl === -1 ? fill !== 0 : fill < 1) return null;
  const words = h.concat(dbl === -1 ? [] : new Array(fill).fill('0'), t).map((g) => parseInt(g, 16));
  const out = [];
  words.forEach((w) => { out.push(w >> 8, w & 255); });
  if (out.slice(0, 10).every((b) => b === 0) && out[10] === 255 && out[11] === 255) return out.slice(12);
  return out;
}
// cidrHit says whether an address lies in a CIDR, or equals a bare IP. A
// family mismatch (an IPv6 client against an IPv4 network) is no match.
function cidrHit(ip, cidr) {
  cidr = String(cidr == null ? '' : cidr).trim();
  const slash = cidr.indexOf('/');
  const net = ipBytes(slash === -1 ? cidr : cidr.slice(0, slash));
  const addr = ipBytes(ip);
  if (!net || !addr || net.length !== addr.length) return false;
  let bits = net.length * 8;
  if (slash !== -1) {
    const b = cidr.slice(slash + 1);
    if (!/^\d{1,3}$/.test(b)) return false;
    bits = parseInt(b, 10);
    if (bits > net.length * 8) return false;
  }
  for (let i = 0; i < net.length && bits > 0; i++, bits -= 8) {
    const mask = bits >= 8 ? 255 : (256 - (1 << (8 - bits))) & 255;
    if ((net[i] & mask) !== (addr[i] & mask)) return false;
  }
  return true;
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
    return `<span class="chip" title="This target has not fired since gpm started.">${esc(pre)}Never</span>`;
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
      ${existing ? '<span class="chip ok" title="A password is set for this user. Leave the field blank to keep it.">Set</span>' : ''}
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
      <div class="hint">Key: the certificate subject (<span class="mono">CN=ops,O=Corp</span>) or its bare common name. Value: the role.</div>
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
  'auth-request': 'The external outpost decides allow or deny for every request.',
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
    // What the block holds right now, unvalidated: for its fold summary.
    peek() {
      const m = modeSel.value;
      return { mode: m, identityProvider: (m === 'client-cert' || m === 'basic') ? '' : idpSel.value, requiredRoles: rolesCtl.get() };
    },
    // Returns the auth spec, or null after showing the first blocking message.
    get(label) {
      const m = modeSel.value;
      const idp = (m === 'client-cert' || m === 'basic') ? '' : idpSel.value;
      const roles = rolesCtl.get();
      const allow = allowCtl.get();
      const certRoles = m === 'client-cert' ? certRolesCtl.get() : {};
      const where = label ? label + ': ' : '';
      if (m !== 'client-cert' && m !== 'basic' && !idp) {
        showInvalid('Identity provider required', where + 'Select an identity provider.', idpSel); return null;
      }
      if (m === 'auth-request' && roles.length) {
        showInvalid('Roles unsupported', where + 'Required roles are not supported in auth-request mode - the auth server does authorization.', $('#' + p + '-roles')); return null;
      }
      if (m === 'client-cert' && roles.length && !Object.keys(certRoles).length) {
        showInvalid('Mapping required', where + 'Add at least one certificate subject role, or clear the required roles.', $('#' + p + '-certroles')); return null;
      }
      if (allow.length) {
        const bad = firstBadCidr(allow);
        if (bad) { showInvalid('Invalid network', where + `"${bad}" is not a CIDR or IP address.`, $('#' + p + '-allow')); return null; }
        if (m === 'oidc' || m === 'forward-auth') {
          showInvalid('Exemption not applicable', where + 'Exempt networks apply only to auth-request, client-cert and basic modes.', $('#' + p + '-allow')); return null;
        }
        if (!m) {
          const t = (idps.find((x) => x.name === idp) || {}).type || '';
          if (t !== 'auth-request') {
            showInvalid('Set a mode explicitly', where + `With no mode this uses ${idp}'s type (${t || 'unknown'}), where the exemption would be ignored.`, modeSel); return null;
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
        if (uerr) { showInvalid('Credentials incomplete', where + uerr, $('#' + p + '-users .row-bad') || $('#' + p + '-users')); return null; }
        const realm = $('#' + p + '-realm').value.trim();
        if (realm && (realm.length > 128 || /["\\]/.test(realm) || /[^\x20-\x7e]/.test(realm))) {
          showInvalid('Invalid realm', where + 'Realm must be printable ASCII without " or \\ (it is sent verbatim in the WWW-Authenticate header).', $('#' + p + '-realm')); return null;
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
        <button type="button" class="seg-btn${perSecond ? '' : ' on'}" id="${p}-rf-win" aria-pressed="${!perSecond}">Per window</button>
        <button type="button" class="seg-btn${perSecond ? ' on' : ''}" id="${p}-rf-sec" aria-pressed="${!!perSecond}">Per second</button>
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
    winBtn.setAttribute('aria-pressed', String(!perSecond));
    secBtn.setAttribute('aria-pressed', String(perSecond));
    $('#' + p + '-win-fields').hidden = perSecond;
    $('#' + p + '-sec-fields').hidden = !perSecond;
  }
  winBtn.addEventListener('click', () => pick(false));
  secBtn.addEventListener('click', () => pick(true));
  return {
    // What the block holds right now, unvalidated: for its fold summary.
    peek() {
      if (secBtn.classList.contains('on')) return { requestsPerSecond: parseFloat($('#' + p + '-rps').value) || 0 };
      return { requests: parseFloat($('#' + p + '-req').value) || 0, window: $('#' + p + '-win').value.trim() };
    },
    get(label) {
      const where = label ? label + ': ' : '';
      const perSecond = secBtn.classList.contains('on');
      const spec = {};
      if (perSecond) {
        const rps = parseFloat($('#' + p + '-rps').value);
        if (isNaN(rps) || rps <= 0) { showInvalid('Rate required', where + 'Set a rate: requests per second, or requests plus a window.', $('#' + p + '-rps')); return null; }
        spec.requestsPerSecond = rps;
      } else {
        const req = parseFloat($('#' + p + '-req').value);
        const win = $('#' + p + '-win').value.trim();
        if (isNaN(req) || req <= 0) { showInvalid('Rate required', where + 'Set a rate: requests per second, or requests plus a window.', $('#' + p + '-req')); return null; }
        if (!GO_DURATION_RE.test(win)) { showInvalid('Invalid window', where + 'Window must be a duration such as 10s, 1m or 1h.', $('#' + p + '-win')); return null; }
        spec.requests = req;
        spec.window = win;
      }
      const burst = parseInt($('#' + p + '-burst').value, 10);
      if (!isNaN(burst) && burst > 0) spec.burst = burst;
      const allow = allowCtl.get();
      if (allow.length) {
        const bad = firstBadCidr(allow);
        if (bad) { showInvalid('Invalid network', where + `"${bad}" is not a CIDR or IP address.`, $('#' + p + '-allow')); return null; }
        spec.allowFrom = allow;
      }
      const block = $('#' + p + '-block').value.trim();
      if (block) {
        if (!GO_DURATION_RE.test(block)) { showInvalid('Invalid block duration', where + 'Block for must be a duration greater than zero, such as 10m.', $('#' + p + '-block')); return null; }
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
// loc, set for a location's fold, says whether the host itself has a sign-in
// (inline, or an auth middleware): that one still applies on the path.
function authFoldSummary(auth, loc) {
  if (!auth) {
    if (!loc) return 'off - this host is not gated by a sign-in';
    return 'off - no extra sign-in on this path' + (loc.hostSignIn ? ', host sign-in still applies' : '');
  }
  const parts = [];
  if (auth.mode) parts.push(enumLabel('authMode', auth.mode).split(' - ')[0]);
  if (auth.identityProvider) parts.push('via ' + auth.identityProvider);
  if (arr(auth.requiredRoles).length) parts.push(plural(arr(auth.requiredRoles).length, 'required role', 'required roles'));
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
    <div class="inline-fields">
      <div class="field-group"><label>Base path</label>
        <input class="field mono" id="${p}-uppath" data-hint="proxyHost.upstream.path" value="${esc(up.path || '')}" placeholder="/api" />
        <div class="hint">With <span class="mono">/api</span>, a request for <span class="mono">/v1/x</span> arrives as <span class="mono">/api/v1/x</span>.</div>
      </div>
      <div class="field-group"><label>Host header</label>
        <select class="field mono" id="${p}-uphh" data-hint="proxyHost.upstream.hostHeader">${enumOptions('hostHeader', ['', 'upstream', 'custom'], preset)}</select>
        <div class="hint" id="${p}-uphh-hint"></div>
      </div>
      <div class="field-group" id="${p}-uphh-custom-wrap"${preset === 'custom' ? '' : ' hidden'}><label>Custom Host header</label>
        <input class="field mono" id="${p}-uphh-custom" data-hint="proxyHost.upstream.hostHeaderCustom" value="${esc(preset === 'custom' ? hh : '')}" placeholder="app.internal.example.com" />
      </div>
    </div>`;
}
const HOSTHEADER_HINTS = {
  '': 'The upstream sees the hostname the client asked for. This is the default and suits almost every app.',
  'upstream': 'The upstream sees its own address (host:port). Use it for an app that keys its virtual host off its own address.',
  'custom': 'The upstream sees exactly this hostname. Must be a hostname, optionally with a port.',
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
      if (perr) { showInvalid('Invalid base path', where + perr, $('#' + p + '-uppath')); return null; }
      if (path) out.path = path;
      if (sel.value === 'upstream') out.hostHeader = 'upstream';
      else if (sel.value === 'custom') {
        const v = $('#' + p + '-uphh-custom').value.trim();
        if (!v) { showInvalid('Host header required', where + 'Enter a hostname, or choose another option.', $('#' + p + '-uphh-custom')); return null; }
        if (v.length > 253 || !HOSTHEADER_RE.test(v)) { showInvalid('Invalid host header', where + 'Host header must be a hostname, optionally "host:port".', $('#' + p + '-uphh-custom')); return null; }
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
  if (state === 'pending') return 'Pending';
  if (state === 'error') return 'Error';
  if (days == null) return '';
  if (days >= 0) return `In ${plural(days, 'day', 'days')}`;
  const ago = -days;
  return `Expired ${plural(ago, 'day', 'days')} ago`;
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
    titleBits.push(days >= 0 ? `${plural(days, 'day', 'days')} remaining` : `Expired ${plural(-days, 'day', 'days')} ago`);
  }
  if (ct.notAfter) titleBits.push(fmtTime(ct.notAfter));
  const title = titleBits.join('; ');
  const chipCls = flat ? `chip flat${cls ? ' ' + cls : ''}` : `chip ${cls}`;
  return `<span class="${chipCls}" title="${esc(title)}"><span class="dot ${cls}"></span>${esc(label)}</span>`;
}
// The certificate a domain resolves to (certForDomain), as one chip: the hosts
// list's Certificate column and the host editor rail. Covered and valid: a
// green lock and the name. Inside the renewal window (state "expiring"):
// amber, with "expires in N days". Expired or failed: red. Not issued yet:
// amber. Nothing covers the domain: amber "no certificate".
function certCoverHtml(ct, domain) {
  if (!ct) return `<span class="chip warn cert-cover" title="${esc(domain ? 'No certificate covers ' + domain : 'No domain')}">No certificate</span>`;
  const st = ct.state || '';
  const when = ct.notAfter ? ', expires ' + fmtDate(ct.notAfter) : '';
  if (st === 'expiring' || st === 'expired' || st === 'error' || st === 'pending') {
    const cls = (st === 'expiring' || st === 'pending') ? 'warn' : 'err';
    const days = ct.daysRemaining;
    // The label follows the certificate's name inside the chip, so it stays lowercase.
    let label = certExpiryLabel(st, days).toLowerCase();
    if (st === 'expiring') label = days != null && days >= 0 ? 'expires ' + label : 'expiring';
    const title = `Certificate ${ct.name}: ${label}${when}${ct.lastError ? '; ' + ct.lastError : ''}`;
    return `<span class="chip ${cls} cert-cover" title="${esc(title)}"><span class="dot ${cls}"></span><span class="trunc">${esc(ct.name)}</span>${esc(label)}</span>`;
  }
  return `<span class="lock ok cert-cover" title="${esc('Certificate ' + ct.name + when)}">${ICON.lock}<span class="trunc">${esc(ct.name)}</span></span>`;
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
      toastErr(e, `Could not renew ${name}`);
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
// labels are dropped whole: they carry discovery ownership (<prefix>/managed-by),
// and a hand-made copy that kept it would belong to the reconciler, which
// deletes managed objects it has no source for. createdAt/updatedAt belong to
// the source; the store stamps the copy's own on its first save.
function cloneObject(obj, clearDomains) {
  const copy = JSON.parse(JSON.stringify(obj || {}));
  copy.name = '';
  if (clearDomains) copy.domains = [];
  delete copy.labels;
  delete copy.createdAt;
  delete copy.updatedAt;
  stripSecrets(copy);
  return copy;
}
// Stashes a clone seed under the chosen name and navigates to that section's
// "new" editor, which picks it up via takeCloneSeed instead of starting from a
// blank object. Only cloneFlow below reaches this: the list views carry no
// Clone button, so a copy is always made from the object's own page, after
// the naming step.
let cloneSeed = null;
function startClone(section, obj, newName) {
  const data = cloneObject(obj, CLONE_CLEAR_DOMAINS.has(section));
  // Two streams cannot share a listen port (without SNI routing), so the copy
  // starts without one and the dialog says to pick it.
  if (section === 'streams') delete data.listenPort;
  data.name = newName || '';
  cloneSeed = { section, origName: (obj && obj.name) || '', data };
  location.hash = '#/' + section + '/_new';
}
const CLONE_SINGULAR = { hosts: 'proxy host', certs: 'certificate' };
// What the naming dialog says a copy keeps and drops.
function cloneNotesHtml(section, obj, singular) {
  const name = (obj && obj.name) || '';
  const notes = [`<p>Every setting of <b>${esc(name)}</b> is copied into a new ${esc(singular)}. Nothing is saved until you press Save in the editor.</p>`];
  if (CLONE_CLEAR_DOMAINS.has(section)) notes.push('<p>The copy starts with no domains: a domain belongs to one host.</p>');
  if (section === 'streams') notes.push('<p>The copy starts with no listen port: pick a free one in the editor.</p>');
  if (section === 'certs') notes.push(`<p>The copy claims the same domains as <b>${esc(name)}</b>. Only one certificate is presented per name, so change the domains unless the copy is meant to replace it.</p>`);
  if (obj && obj.labels && Object.keys(obj.labels).length) notes.push('<p>Labels are not copied: the copy is managed by hand.</p>');
  if (JSON.stringify(obj || {}).indexOf('"***"') !== -1) notes.push('<p>Secrets are not copied; enter them again in the copy.</p>');
  return notes.join('');
}
// The naming step before a clone. The copy's name is chosen and checked here
// (its shape, and against the names the kind already holds), so the editor
// opens on a copy that is ready to review and save instead of on a blank name
// that fails later.
async function cloneFlow(section, obj) {
  const origName = (obj && obj.name) || '';
  const singular = CLONE_SINGULAR[section] || (SECTION_META[section] && SECTION_META[section].singular) || 'object';
  let names = null;
  const page = location.hash;
  try { names = arr((await api('/api/' + PLURAL[section])).data).map((o) => o.name); } catch (e) { names = null; }
  // The lookup took time; the page it was for may be gone.
  if (location.hash !== page) return;
  const newName = await confirmModal({
    title: `Clone ${origName}`,
    body: cloneNotesHtml(section, obj, singular),
    prompt: {
      label: 'Name of the copy',
      value: origName + '-copy',
      placeholder: origName + '-copy',
      validate: (v) => {
        if (!HOST_NAME_RE.test(v)) return 'Lowercase letters, digits, dots, - or _, starting and ending with a letter or digit.';
        if (v === origName) return 'The copy needs a different name.';
        if (names && names.indexOf(v) !== -1) return `${/^[aeiou]/i.test(singular) ? 'An' : 'A'} ${singular} named "${v}" already exists.`;
        return '';
      },
    },
    confirmLabel: 'Continue to the editor',
    danger: false,
  });
  if (!newName) return;
  startClone(section, obj, newName);
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
// Wires the editor's optional #ed-clone / btnId Clone button (rendered only
// for an existing object) to the naming step for the object loaded in that
// editor.
function wireCloneButton(section, orig, btnId) {
  const btn = $('#' + (btnId || 'ed-clone'));
  if (btn) btn.addEventListener('click', () => cloneFlow(section, orig));
}

// ---------- router ----------
// Every render gets a fresh #content element, and a view that is still
// awaiting a fetch when the operator navigates on is stopped at its next
// render write: that write throws, and route()'s catch drops a stale render
// quietly. Letting it write into the detached element was not enough - views
// wire their controls through document-global $('#id'), so a stale editor of
// the same kind would have wired itself onto the NEW page's controls (its
// data in the new page's fields, its save handler on the new page's Save).
let routeSeq = 0;
const INNER_HTML = Object.getOwnPropertyDescriptor(Element.prototype, 'innerHTML');
async function route() {
  const raw = location.hash.replace(/^#/, '');
  const parts = raw.split('/').filter(Boolean);
  if (!parts.length) { location.hash = '#/overview'; return; }
  const section = parts[0];
  const sub = parts[1] ? decodeURIComponent(parts[1]) : null;

  setActiveNav(section);
  pendingHead = null;
  applyHead({ titleHtml: esc(TITLES[section] || state.appName) });
  const seq = ++routeSeq;
  const prev = $('#content');
  const c = prev.cloneNode(false);
  prev.replaceWith(c);
  Object.defineProperty(c, 'innerHTML', {
    configurable: true,
    get() { return INNER_HTML.get.call(this); },
    set(v) {
      if (seq !== routeSeq) { pendingHead = null; throw new Error('stale render'); }
      INNER_HTML.set.call(this, v);
      if (pendingHead) { applyHead(pendingHead); pendingHead = null; }
    },
  });
  c.innerHTML = loadingHtml();
  scrollToTop();
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
      case 'integrations': await viewIntegrations(c, sub); break;
      case 'tokens': await viewTokens(c); break;
      case 'logs': await viewLogs(c); break;
      case 'history': await viewHistory(c); break;
      case 'settings': await viewSettings(c, sub); break;
      default: location.hash = '#/overview'; return;
    }
    if (seq !== routeSeq) return;
    c.classList.add('view');
    applyRefListGuard(c);
    applyMaintenanceBanner(c);
    applyReadOnlyGating(c);
    applyNoAdminLoginBanner(c);
    applyInsecureCookieBanner(c);
    armDirtyTracking(c);
    decorateHints(c);
    wireMiniTables(c);
    if (hintObserver) { hintObserver.disconnect(); hintObserver.observe(c, { childList: true, subtree: true }); }
    window.dispatchEvent(new CustomEvent('gpm:routed', { detail: { hash: location.hash, ok: true } }));
  } catch (e) {
    if (e && e.message === 'Unauthorized') return;
    if (seq !== routeSeq) return;
    c.innerHTML = inlineError(e && e.message ? e.message : String(e));
    window.dispatchEvent(new CustomEvent('gpm:routed', { detail: { hash: location.hash, ok: false } }));
  }
}
function setActiveNav(section) {
  $$('#nav .nav-item').forEach((n) => {
    const on = n.dataset.view === section;
    n.classList.toggle('active', on);
    if (on) n.setAttribute('aria-current', 'page'); else n.removeAttribute('aria-current');
  });
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
function integrationsTabOf(hash) {
  const m = /^#\/integrations(?:\/([^/]+))?$/.exec(hash || '');
  return m ? (m[1] || INTEGRATIONS_TABS[0].id) : null;
}
let leavePrompt = false;
async function onHashChange() {
  if (revertingHash) { revertingHash = false; currentHash = location.hash; return; }
  const nextTab = settingsTabOf(location.hash);
  if (nextTab && settingsTabOf(currentHash) && document.getElementById('set-panel-general')) {
    currentHash = location.hash;
    showSettingsTab(nextTab);
    return;
  }
  const nextInt = integrationsTabOf(location.hash);
  if (nextInt && integrationsTabOf(currentHash) && document.getElementById('int-panel-dns')) {
    currentHash = location.hash;
    showIntegrationsTab(nextInt);
    return;
  }
  if (dirtyFlag && location.hash !== currentHash) {
    // One prompt at a time: another navigation while it is open waits for its
    // answer (which then leaves for the latest hash, or returns to this page).
    if (leavePrompt) return;
    leavePrompt = true;
    let leave = false;
    try {
      leave = await confirmModal({
        title: 'You have unsaved changes',
        body: '<p>Changes on this page are not committed yet. Leaving discards them.</p>',
        confirmLabel: 'Discard and leave',
        // This prompt is the navigation guard itself, so a navigation must
        // not dismiss it.
        keepOnNavigate: true,
      });
    } finally { leavePrompt = false; }
    if (!leave) { if (location.hash !== currentHash) { revertingHash = true; location.hash = currentHash; } return; }
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
  banner.innerHTML = '<b>Maintenance is on for every proxy host.</b> Each one answers 503 instead of dialling its upstream. '
    + 'Turn off Fleet-wide maintenance under <a href="#/settings/operations">Settings &gt; Operations</a>.';
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

// Every bar saves every card on the page (one settings PUT), so every bar says
// so rather than naming only the card it sits in.
function intSaveBar() {
  return `<div class="panel save-bar int-save-bar">
    <div class="save-note">${ICON.commit}Saves every card on this page as one commit.</div>
    <div style="display:flex;gap:10px"><button class="btn primary int-save" type="button">Save integrations</button></div>
  </div>`;
}

// Shared status renderers. The Ingress and Docker reconcilers report the same
// shape (counters, a hosts table, lastRun/lastSuccess), so they render through
// one pair of helpers rather than two near-identical copies.
const DISCOVERY_ACTION_CLASS = { created: 'ok', updated: 'cyan', unchanged: '', deleted: 'err', skipped: 'warn' };
function discoveryCounters(st) {
  const n = (k) => (st[k] || 0);
  // A zero count stays a plain chip: six coloured pills on every quiet run
  // drew the eye to nothing.
  const c = (k, cls) => `<span class="chip${n(k) ? ' ' + cls : ''}">${n(k)} ${k}</span>`;
  return `<div class="chip-row" style="margin:6px 0">
    <span class="chip">${n('discovered')} discovered</span>
    <span class="chip">${n('managed')} managed</span>
    ${c('created', 'ok')}
    ${c('updated', 'cyan')}
    ${c('deleted', 'err')}
    ${c('skipped', 'warn')}
  </div>`;
}
function discoveryHostsTable(hosts, secondCol, secondKey) {
  if (!arr(hosts).length) return '<div class="hint">No hosts in the last run.</div>';
  return `<div class="table-wrap"><table class="mini-table" data-sort-key="gpm.discovery.sort">
    <thead><tr><th data-sort="host">Host</th><th data-sort="second">${esc(secondCol)}</th><th data-sort="domains">Domains</th><th data-sort="action">Action</th><th data-sort="profile">Profile</th><th data-sort="reason">Reason</th></tr></thead>
    <tbody>${arr(hosts).map((h) => `<tr>
      <td class="col-name"><a href="#/hosts/${encodeURIComponent(h.name || '')}" class="mono trunc" title="${esc(h.name || '')}">${esc(h.name || '')}</a></td>
      <td class="mono faint col-name"><span class="trunc" title="${esc(h[secondKey] || '')}">${esc(h[secondKey] || '')}</span></td>
      <td class="mono faint">${domainListHtml(h.domains)}</td>
      <td class="col-status"><span class="chip ${DISCOVERY_ACTION_CLASS[h.action] || ''}">${esc(h.action || '')}</span></td>
      <td class="mono faint col-name"><span class="trunc" title="${esc(h.profile || '')}">${esc(h.profile || '')}</span></td>
      <td class="faint col-reason">${esc(h.reason || '')}</td>
    </tr>`).join('')}</tbody>
  </table></div>`;
}
function discoveryRunLine(st) {
  const stale = st.lastSuccess && st.lastRun && new Date(st.lastSuccess) < new Date(st.lastRun);
  const tip = 'A failed run freezes the managed hosts; it never deletes them.';
  return `<div class="chip-row"${stale ? ` title="${esc(tip)}"` : ''}>
    <span class="chip${stale ? ' warn' : ''}">Last run: ${esc(st.lastRun ? relTime(st.lastRun) : 'Never')}</span>
    <span class="chip${stale ? ' warn' : ''}">Last success: ${esc(st.lastSuccess ? relTime(st.lastSuccess) : 'Never')}</span>
  </div>`;
}

async function viewIntegrations(c, tab) {
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

  const panel = (id) => `<div class="tab-panel" data-tab="${id}" id="int-panel-${id}" role="tabpanel" aria-labelledby="int-tab-${id}" hidden>`;
  c.innerHTML = `
    ${pageHead('Integrations', 'Systems gpm talks to: DNS zones it writes, hosts it discovers, IP feeds it pulls, endpoints it notifies.', '', '', '', '',
      INTEGRATIONS_TABS.map((t) => `<button class="tab-btn" type="button" role="tab" id="int-tab-${esc(t.id)}" aria-controls="int-panel-${esc(t.id)}" data-tab="${esc(t.id)}" aria-selected="false" tabindex="-1">${esc(t.label)}</button>`).join(''))}
    ${panel('dns')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">DNS sync</p>
      <div class="grid-2">
        <div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Pi-hole (LAN)</div><div class="ds">Local CNAMEs on the LAN resolver</div></div>${switchHtml('set-ph-on', !!ds.pihole.enabled, 'Pi-hole DNS sync', 'settings.dnsSync.pihole.enabled')}</div>
          <div class="field-group" style="margin-top:8px"><label>Pi-hole URL</label><input class="field mono" id="set-ph-url" data-hint="settings.dnsSync.pihole.url" data-path="dnsSync.pihole.url" value="${esc(ds.pihole.url || '')}" placeholder="http://pihole.lan" /></div>
          <div class="field-group"><label>App password</label><input class="field mono" id="set-ph-pw" data-hint="settings.dnsSync.pihole.appPassword" data-path="dnsSync.pihole.appPassword" value="${esc(ds.pihole.appPassword || '')}" placeholder="\${ENV:PIHOLE_APP_PASSWORD}" /></div>
          <div class="field-group"><label>Apex target</label><input class="field mono" id="set-ph-apex" data-hint="settings.dnsSync.pihole.apexTarget" data-path="dnsSync.pihole.apexTarget" value="${esc(ds.pihole.apexTarget || '')}" placeholder="edge.example.com" /><div class="hint">Where managed CNAMEs point.</div></div>
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
        <button class="btn ghost sm" id="set-dns-preview" type="button" title="Read the DNS servers and report what a reconcile would change. Nothing is written.">Preview changes</button>
        <button class="btn ghost sm" id="set-dns-run" type="button">Reconcile now</button>
      </div>
      ${intSaveBar()}
    </div>
    </div>
    ${panel('ingress')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Kubernetes Ingress discovery</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Ingress discovery</div><div class="ds">Reconcile annotated cluster Ingresses into managed proxy hosts</div></div>${switchHtml('set-id-on', !!idc.enabled, 'Ingress discovery', 'settings.ingressDiscovery.enabled')}</div>
      ${foldHtml('id-conn-card', 'Connection', idc.apiURL || 'in-cluster endpoint', !!idc.enabled, `
        <div class="grid-2">
          <div>
            <div class="field-group"><label>API server URL</label><input class="field mono" id="set-id-url" data-hint="settings.ingressDiscovery.apiURL" data-path="ingressDiscovery.apiURL" value="${esc(idc.apiURL || '')}" placeholder="https://k8s.example.lan:6443" /></div>
            <div class="field-group"><label>Token file</label><input class="field mono" id="set-id-token" data-hint="settings.ingressDiscovery.tokenFile" data-path="ingressDiscovery.tokenFile" value="${esc(idc.tokenFile || '')}" placeholder="/run/secrets/gpm-k8s-token" /><div class="hint">Path to a read-only ServiceAccount token.</div></div>
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
        <div class="grid-2">
          <div>
            <div class="field-group">
              <label>Upstream (ingress controller)</label>
              <div class="loc-row bare">
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
            <div class="field-group"><label>Certificate</label><input class="field mono" id="set-id-cert" data-hint="settings.ingressDiscovery.template.tls.certificateRef" data-path="ingressDiscovery.template.tls.certificateRef" value="${esc(idt.tls.certificateRef || '')}" placeholder="wildcard" /><div class="hint">Required: point this at your wildcard certificate.</div></div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS on derived hosts</div></div>${switchHtml('set-id-forcessl', !!idt.tls.forceSSL, 'Force SSL', 'settings.ingressDiscovery.template.tls.forceSSL')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send X-Robots-Tag on derived hosts</div></div>${switchHtml('set-id-robots', !!idt.robotsNoIndex, 'Discourage indexing', 'settings.ingressDiscovery.template.robotsNoIndex')}</div>
          </div>
          <div>
            <div class="field-group"><label>Middlewares</label><div class="chip-input" id="set-id-mw" data-hint="settings.ingressDiscovery.template.middlewares"></div><div class="hint">Applied to every derived host, in order.</div></div>
            <div class="field-group"><label>Access lists</label><div class="chip-input" id="set-id-al" data-hint="settings.ingressDiscovery.template.accessLists"></div></div>
            <div class="field-group">
              <label>Upstream timeouts (seconds)</label>
              <div class="loc-row bare">
                <input class="field mono" id="set-id-to-connect" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(idTo.connectSeconds || '')}" placeholder="connect" aria-label="Connect timeout seconds" />
                <input class="field mono" id="set-id-to-read" data-hint="settings.ingressDiscovery.template.timeouts" style="flex:1 1 90px" value="${esc(idTo.readSeconds || '')}" placeholder="read" aria-label="Read timeout seconds" />
              </div>
              <div class="hint">Leave both blank for the shared pooled transport. Read caps time-to-first-byte only, so streaming upstreams stay safe.</div>
            </div>
            <div class="field-group"><label>Tags</label><div class="chip-input" id="set-id-tags" data-hint="settings.ingressDiscovery.template.tags"></div><div class="hint">Applied to every derived host, for grouping in the host list.</div></div>
            <div class="field-group">
              <label>Allowed domain suffixes override (optional)</label>
              <div class="chip-input" id="set-id-suffixes-override" data-hint="settings.ingressDiscovery.template.allowedDomainSuffixes"></div>
              <div class="hint">Must be a subset of the global list. Empty uses the global list.</div>
            </div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Default LAN direct</div><div class="ds">When the lan-direct annotation is absent</div></div>${switchHtml('set-id-dns-lan', !!idt.defaultDNS.lanDirect, 'Default LAN direct', 'settings.ingressDiscovery.template.defaultDNS.lanDirect')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Default public CNAME</div><div class="ds">When the public-cname annotation is absent</div></div>${switchHtml('set-id-dns-pub', !!idt.defaultDNS.publicCname, 'Default public CNAME', 'settings.ingressDiscovery.template.defaultDNS.publicCname')}</div>
          </div>
        </div>
      `)}
      ${foldHtml('id-profiles-card', 'Named profiles', Object.keys(idProfiles).length ? Object.keys(idProfiles).sort().join(', ') : 'none - every Ingress gets the template', Object.keys(idProfiles).length > 0, `
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">One template only fits a uniform fleet. Define a profile per chain you actually run - a deliberately public one with a rate limit and no access list, an SSO-gated one behind the VPN list - and an Ingress picks one with <span class="mono">${esc(resolvedPrefix)}/profile: "&lt;name&gt;"</span>. <strong>The annotation carries a name and nothing else.</strong> <strong>Every profile is selectable by every annotating Ingress - define only profiles you are willing for any cluster tenant to choose.</strong> An Ingress naming a profile that does not exist is <strong>skipped</strong>, and if it already had a derived host, that host is <strong>disabled</strong>.</p>
        <div id="set-id-profiles" data-hint="settings.ingressDiscovery.profiles"></div>
        <button class="btn ghost sm" id="addIdProfile" type="button" style="margin-top:6px">${ICON.plus}Add profile</button>
      `)}
      ${foldHtml('id-rules-card', 'Profile selection rules', arr(idc.profileRules).length ? plural(arr(idc.profileRules).length, 'rule', 'rules') : 'none - the annotation decides', arr(idc.profileRules).length > 0, `
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
      ${intSaveBar()}
    </div>
    </div>
    ${panel('docker')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Docker discovery</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Docker discovery</div><div class="ds">Turn container discovery on. Everything below is only validated when this is on.</div></div>${switchHtml('set-dkr-on', !!dd.enabled, 'Docker discovery', 'settings.dockerDiscovery.enabled')}</div>
      <div id="dkr-fields">
        ${foldHtml('dkr-conn-card', 'Engine connection', dd.host || dd.socket || '/var/run/docker.sock', !!dd.enabled, `
          <div class="grid-2">
            <div>
              <div class="field-group"><label>Socket</label><input class="field mono" id="set-dkr-socket" data-hint="settings.dockerDiscovery.socket" data-path="dockerDiscovery.socket" value="${esc(dd.socket || '')}" placeholder="/var/run/docker.sock" /><div class="hint">Absolute path. Ignored when a host URL is set.</div></div>
              <div class="field-group"><label>Host URL</label><input class="field mono" id="set-dkr-host" data-hint="settings.dockerDiscovery.host" data-path="dockerDiscovery.host" value="${esc(dd.host || '')}" placeholder="tcp://socket-proxy:2375" /><div class="hint">Used instead of the socket.</div></div>
              <div class="field-group"><label>Poll interval</label><input class="field mono" id="set-dkr-poll" data-hint="settings.dockerDiscovery.pollInterval" data-path="dockerDiscovery.pollInterval" value="${esc(dd.pollInterval || '')}" placeholder="60s" /><div class="hint">Go duration, minimum 15s.</div></div>
            </div>
            <div>
              <div class="field-group"><label>TLS client certificate</label><input class="field mono" id="set-dkr-tlscert" data-hint="settings.dockerDiscovery.tlsCert" data-path="dockerDiscovery.tlsCert" value="${esc(dd.tlsCert || '')}" placeholder="/run/secrets/docker-client.crt" /><div class="hint">Absolute path. Only for an <span class="mono">https://</span> host URL. Set with the key.</div></div>
              <div class="field-group"><label>TLS client key</label><input class="field mono" id="set-dkr-tlskey" data-hint="settings.dockerDiscovery.tlsKey" data-path="dockerDiscovery.tlsKey" value="${esc(dd.tlsKey || '')}" placeholder="/run/secrets/docker-client.key" /></div>
              <div class="field-group"><label>TLS CA bundle</label><input class="field mono" id="set-dkr-tlsca" data-hint="settings.dockerDiscovery.tlsCA" data-path="dockerDiscovery.tlsCA" value="${esc(dd.tlsCA || '')}" placeholder="/run/secrets/docker-ca.crt" /><div class="hint">Absolute path. There is no skip-verify option.</div></div>
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
              <div class="toggle-line"><div class="tl-text"><div class="nm">Use published ports</div><div class="ds">Forward to the host-published port instead of the container IP.</div></div>${switchHtml('set-dkr-pubports', !!dd.usePublishedPorts, 'Use published ports', 'settings.dockerDiscovery.usePublishedPorts')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Include stopped containers</div><div class="ds">List non-running containers too.</div></div>${switchHtml('set-dkr-stopped', !!dd.includeStopped, 'Include stopped containers', 'settings.dockerDiscovery.includeStopped')}</div>
              <div class="field-group" style="margin-top:10px">
                <label>Allowed domain suffixes</label>
                <div class="chip-input" id="set-dkr-suffixes" data-hint="settings.dockerDiscovery.allowedDomainSuffixes" data-path="dockerDiscovery.allowedDomainSuffixes"></div>
                <div class="hint">Required. A derived hostname must equal one of these or end in "." plus one of them.</div>
              </div>
            </div>
          </div>
        `)}
        ${foldHtml('dkr-tmpl-card', 'Default host template', ddt.tls.certificateRef ? 'certificate ' + ddt.tls.certificateRef : 'no certificate set', !!dd.enabled, `
          <div class="grid-2">
            <div>
              <div class="field-group"><label>Certificate</label><input class="field mono" id="set-dkr-cert" data-hint="settings.dockerDiscovery.template.tls.certificateRef" data-path="dockerDiscovery.template.tls.certificateRef" value="${esc(ddt.tls.certificateRef || '')}" placeholder="wildcard" /></div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS on derived hosts</div></div>${switchHtml('set-dkr-forcessl', !!ddt.tls.forceSSL, 'Force SSL', 'settings.dockerDiscovery.template.tls.forceSSL')}</div>
              <div class="toggle-line"><div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send X-Robots-Tag on derived hosts</div></div>${switchHtml('set-dkr-robots', !!ddt.robotsNoIndex, 'Discourage indexing', 'settings.dockerDiscovery.template.robotsNoIndex')}</div>
              <div class="field-group" style="margin-top:10px">
                <label>Upstream timeouts (seconds)</label>
                <div class="loc-row bare">
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
          <div id="set-dkr-profiles" data-hint="settings.dockerDiscovery.profiles"></div>
          <button class="btn ghost sm" id="addDkrProfile" type="button" style="margin-top:6px">${ICON.plus}Add profile</button>
        `)}
      </div>
      <div id="set-dkr-status" style="margin-top:12px"></div>
      <div style="margin-top:6px;display:flex;gap:6px;flex-wrap:wrap">
        <button class="btn ghost sm" id="set-dkr-preview" type="button" title="Read the Engine and report what a reconcile would change. Nothing is written.">Preview changes</button>
        <button class="btn ghost sm" id="set-dkr-run" type="button">Reconcile now</button>
      </div>
      ${intSaveBar()}
    </div>
    </div>
    ${panel('access-sync')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Access-list sync</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Fetch remote sources</div><div class="ds">On by default; turning it off freezes every list at its last fetch</div></div>${switchHtml('set-als-on', alsEnabled, 'Fetch remote access-list sources', 'settings.accessListSync.enabled')}</div>
      <div class="field-group" style="margin-top:10px;max-width:220px">
        <label>Poll interval</label>
        <input class="field mono" id="set-als-poll" data-hint="settings.accessListSync.pollInterval" data-path="accessListSync.pollInterval" value="${esc(alsPoll)}" placeholder="15m" />
        <div class="hint">Go duration, minimum 1m, default 15m.</div>
      </div>
      <div id="int-als-status" style="margin-top:12px"></div>
      <div style="margin-top:6px"><button class="btn ghost sm" id="int-als-run" type="button">Reconcile now</button></div>
      ${intSaveBar()}
    </div>
    </div>
    ${panel('webhooks')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Lifecycle webhooks</p>
      <div id="set-webhooks" data-hint="settings.webhooks" data-path="webhooks"></div>
      <button class="btn ghost sm" id="addWebhook" type="button" style="margin-top:6px">${ICON.plus}Add webhook</button>
      <div class="hint" style="margin-top:8px">Delivery state is kept in memory and resets when gpm restarts. Test sends <span class="mono">{"action":"test","kind":"Webhook","name":"&lt;target&gt;","time":"&lt;RFC3339&gt;"}</span> to the target URL, signed with the same <span class="mono">X-GPM-Webhook-Signature</span> and <span class="mono">X-GPM-Webhook-Timestamp</span> headers a real event carries.</div>
      ${intSaveBar()}
    </div>
    </div>
    ${panel('notifications')}
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Notifications</p>
      <div class="field-group" style="max-width:260px">
        <label>Expiry digest threshold (days)</label>
        <input class="field mono" id="set-ntf-days" data-hint="settings.notifications.expiringThresholdDays" data-path="notifications.expiringThresholdDays" type="number" min="1" max="365" value="${esc((s.notifications && s.notifications.expiringThresholdDays) || '')}" placeholder="14" />
      </div>
      <div id="set-notifications" data-hint="settings.notifications.targets" data-path="notifications.targets" style="margin-top:10px"></div>
      <button class="btn ghost sm" id="addNotification" type="button" style="margin-top:6px">${ICON.plus}Add notification target</button>
      <div class="hint" style="margin-top:8px">Delivery state is kept in memory and resets when gpm restarts. Test sends a synthetic event to the target immediately, using the same payload shape a real alert would (plain text for ntfy, an embed for Discord, a JSON envelope for generic), bypassing this target's Events filter.</div>
      ${intSaveBar()}
    </div>
    </div>`;

  showIntegrationsTab(tab || INTEGRATIONS_TABS[0].id);
  $$('#pageTabs .tab-btn').forEach((b) => {
    b.addEventListener('click', () => { location.hash = '#/integrations/' + b.dataset.tab; });
  });
  wirePageTabKeys();

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
      el.textContent = 'Last run: ' + (st.lastRun ? fmtTime(st.lastRun) : 'Never') + '. ' +
        line('Pi-hole', st.pihole) + ' | ' + line('Cloudflare', st.cloudflare) +
        (st.error ? ' | ' + st.error : '');
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'DNS sync is not available in this build.' : 'Status unavailable: ' + (e.message || e);
    }
  }
  renderDNSStatus();

  // Dry run. gpm only ever deletes records its ownership ledger says it created,
  // so the number that matters before a first reconcile is "left alone" - it
  // should equal everything the operator wrote by hand on that backend.
  async function renderDNSPlan() {
    const el = $('#set-dns-plan');
    if (!el) return;
    el.textContent = 'Reading the DNS servers...';
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
        parts.push(`${plural(b.untouched || 0, 'record', 'records')} gpm does not own, left alone`);
        return `${label}: ` + parts.join('; ');
      };
      el.textContent = 'Dry run, nothing was changed. ' + line('Pi-hole', p.pihole) + ' | ' + line('Cloudflare', p.cloudflare);
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'DNS sync is not available in this build.' : 'Preview unavailable: ' + (e.message || e);
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
    } catch (e) { toastErr(e, 'Could not run DNS sync'); }
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
    div.className = 'sub-block id-profile';
    div.innerHTML = `
      <div class="loc-row" style="margin-bottom:8px">
        <input class="field mono pf-name" data-hint="settings.ingressDiscovery.profiles.name" style="flex:1 1 180px" value="${esc(name || '')}" placeholder="profile name (e.g. sso-internal)" aria-label="Profile name" />
        <button class="icon-btn pf-del" type="button" aria-label="Remove profile">${ICON.x}</button>
      </div>
      <div class="grid-2">
        <div>
          <div class="field-group">
            <label>Upstream (ingress controller)</label>
            <div class="loc-row bare">
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
            <div class="loc-row bare">
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
      parts.push('Last run: ' + (st.lastRun ? fmtTime(st.lastRun) : 'Never'));
      // A failed run freezes the managed hosts, so how stale the good state is
      // matters more than when the last attempt happened.
      if (st.error) parts.push('FAILED - ' + st.error + (st.lastSuccess ? ' (last success ' + fmtTime(st.lastSuccess) + ', managed hosts frozen as they were)' : ' (no successful run yet)'));
      else parts.push(`${st.discovered || 0} annotated Ingresses, ${st.managed || 0} managed hosts (+${st.created || 0} / ~${st.updated || 0} / -${st.deleted || 0}${st.skipped ? ', ' + st.skipped + ' skipped' : ''})`);
      el.textContent = parts.join('. ') + '.';
      if (list) { list.innerHTML = discoveryRunLine(st) + discoveryCounters(st) + discoveryHostsTable(st.hosts, 'Ingress', 'ingress'); wireMiniTables(list); }
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'Ingress discovery is not available in this build.' : 'Status unavailable: ' + (e.message || e);
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
      if (list) { list.innerHTML = discoveryCounters(p) + discoveryHostsTable(p.hosts, 'Ingress', 'ingress'); wireMiniTables(list); }
    } catch (e) {
      el.textContent = e && e.status === 501 ? 'Ingress discovery is not available in this build.' :
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
    } catch (e) { toastErr(e, 'Could not run Ingress discovery'); }
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
    div.className = 'sub-block dkr-profile';
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
            <div class="loc-row bare">
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
    wireMiniTables(el);
  }
  async function renderDockerStatus() {
    const el = $('#set-dkr-status');
    if (!el) return;
    if (!dkrAvailable) { el.innerHTML = '<div class="hint">Docker discovery is turned off.</div>'; return; }
    try {
      renderDockerPayload((await api('/api/docker-discovery/status')).data || {}, false);
    } catch (e) {
      el.innerHTML = `<div class="hint">${e && e.status === 501 ? 'Docker discovery is not available in this build.' : esc('Status unavailable: ' + (e.message || e))}</div>`;
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
    if (st === 501) { toast('Not available', 'Docker discovery is not available in this build.', 'err'); return; }
    if (st === 403) { toast('Scope missing', 'This token lacks the docker-discovery scope.', 'err'); return; }
    if (st === 502) {
      const el = $('#set-dkr-status');
      if (el) el.innerHTML = `<div class="inline-error">${esc((e && e.message) || 'Engine error')}</div>`;
      return;
    }
    toastErr(e, 'Could not run Docker discovery');
  }

  // ----- Access-list sync -----
  async function renderAlsStatus() {
    const el = $('#int-als-status');
    if (!el) return;
    try {
      const st = (await api('/api/access-list-sources/status')).data || {};
      const rows = arr(st.sources);
      el.innerHTML = rows.length ? `<div class="table-wrap"><table class="mini-table" data-sort-key="gpm.als.sort">
        <thead><tr><th data-sort="list">Access list</th><th data-sort="source">Source</th><th class="col-time" data-sort="fetched">Fetched</th><th class="col-num" data-sort="entries" data-type="num">Entries</th><th data-sort="error">Error</th></tr></thead>
        <tbody>${rows.map((r) => `<tr>
          <td class="col-name"><a class="mono trunc" href="#/access/${encodeURIComponent(r.list || '')}" title="${esc(r.list || '')}">${esc(r.list || '')}</a></td>
          <td class="mono col-name"><span class="trunc" title="${esc(r.name || '')}">${esc(r.name || '')}</span></td>
          <td class="faint col-time" data-v="${esc(r.fetchedAt || '')}"${r.fetchedAt ? ` title="${esc(fmtTime(r.fetchedAt))}"` : ''}>${esc(r.fetchedAt ? relTime(r.fetchedAt) : 'Never')}</td>
          <td class="mono col-num">${esc(r.entryCount || 0)}</td>
          <td class="warn-text col-reason">${esc(r.lastError || '')}</td>
        </tr>`).join('')}</tbody></table></div>` : '<div class="hint">No access list declares a remote source yet.</div>';
      wireMiniTables(el);
    } catch (e) {
      el.innerHTML = `<div class="hint">${e && e.status === 501 ? 'Access-list source sync is not available in this build.' : esc('Status unavailable: ' + (e.message || e))}</div>`;
    }
  }
  renderAlsStatus();
  $('#int-als-run').addEventListener('click', async () => {
    const btn = $('#int-als-run'); btn.disabled = true;
    try {
      await api('/api/access-list-sources/reconcile', { method: 'POST' });
      toast('Reconciled', 'Access-list sources are back in sync.', 'ok');
    } catch (e) { toastErr(e, 'Could not refresh the sources'); }
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
      else toastErr(e, `Could not test ${name}`);
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
      // A literal secret is redacted on read; saving it back would commit "***".
      if (secret === '***') { whErr = markRowError(row, 'The secret reads ***. Replace it with a ${ENV:...} or ${FILE:...} placeholder before saving.'); return; }
      if (secret) wh.secret = secret;
      if (row.querySelector('.wh-disabled').checked) wh.disabled = true;
      webhooks.push(wh);
    });
    if (whErr) { showInvalid('Webhook invalid', whErr); return; }
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
      if (secret === '***') { ntfErr = markRowError(row, 'The secret reads ***. Replace it with a ${ENV:...} or ${FILE:...} placeholder before saving.'); return; }
      if (secret) t.secret = secret;
      if (row.querySelector('.ntf-disabled').checked) t.disabled = true;
      // Omitted when the chips are exactly the server-side default, so an
      // untouched row does not gain a redundant explicit list in git.
      const ev = row._events.get();
      if (!sameEventSet(ev, defaultNotifyEvents())) t.events = ev;
      targets.push(t);
    });
    if (ntfErr) { showInvalid('Notification target invalid', ntfErr); return; }
    const ntfDays = parseInt($('#set-ntf-days').value, 10);
    const notifications = {};
    if (targets.length) notifications.targets = targets;
    if (!isNaN(ntfDays) && ntfDays > 0 && ntfDays !== 14) notifications.expiringThresholdDays = ntfDays;
    if (Object.keys(notifications).length) body.notifications = notifications; else delete body.notifications;

    const dnsSync = { pihole: {}, cloudflare: {} };
    const phPw = $('#set-ph-pw').value.trim();
    if (phPw === '***') {
      // A literal secret is redacted on read; saving it back would commit "***".
      showInvalid('Secret masked', 'The Pi-hole app password reads ***. Replace it with a ${ENV:...} or ${FILE:...} placeholder before saving.', $('#set-ph-pw'));
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
        locations: arr(idt.locations).length ? idt.locations : undefined,
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
        locations: arr((row._orig || {}).locations).length ? (row._orig || {}).locations : undefined,
        allowedDomainSuffixes: row._suffixes.get().length ? row._suffixes.get() : undefined,
      };
      if (isOn('pf-dns-lan-' + uid) || isOn('pf-dns-pub-' + uid)) {
        prof.defaultDNS = { lanDirect: isOn('pf-dns-lan-' + uid), publicCname: isOn('pf-dns-pub-' + uid) };
      }
      profiles[pname] = prof;
    });
    if (pfDup) {
      showInvalid('Duplicate profile', `Two discovery profiles are both called "${pfDup}". Profile names must be unique - an Ingress selects one by name.`);
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
    // Docker discovery checks name the fields by their labels on this page.
    if (dkrOn && !dkrSuffixes.length) { showInvalid('Docker discovery', 'Add at least one allowed domain suffix. It is required while Docker discovery is on.', $('#set-dkr-suffixes')); return; }
    if (dkrSocket && dkrSocket[0] !== '/') { showInvalid('Docker discovery', 'Socket must be an absolute path, such as /var/run/docker.sock.', $('#set-dkr-socket')); return; }
    if (dkrHost && !/^(tcp|https):\/\//.test(dkrHost)) { showInvalid('Docker discovery', 'Host URL must start with tcp:// or https://, such as tcp://socket-proxy:2375.', $('#set-dkr-host')); return; }
    if (!!dkrCert !== !!dkrKey) { showInvalid('Docker discovery', 'Set TLS client certificate and TLS client key together, or neither.', dkrCert ? $('#set-dkr-tlskey') : $('#set-dkr-tlscert')); return; }
    if (dkrCert && !/^https:\/\//.test(dkrHost)) { showInvalid('Docker discovery', 'A TLS client certificate needs an https:// Host URL.', $('#set-dkr-host')); return; }
    if ($('#set-dkr-pubhost').value.trim() && !isOn('set-dkr-pubports')) { showInvalid('Docker discovery', 'Published host applies only with Use published ports on. Turn it on, or clear Published host.', $('#set-dkr-pubhost')); return; }
    if ($('#set-dkr-network').value.trim() && isOn('set-dkr-pubports')) { showInvalid('Docker discovery', 'Network and Use published ports cannot both be set. Clear Network, or turn Use published ports off.', $('#set-dkr-network')); return; }
    if (dkrOn && !$('#set-dkr-cert').value.trim()) { showInvalid('Docker discovery', 'Set the template Certificate. It is required while Docker discovery is on.', $('#set-dkr-cert')); return; }
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
        locations: arr(ddt.locations).length ? ddt.locations : undefined,
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
        locations: arr((row._orig || {}).locations).length ? (row._orig || {}).locations : undefined,
        allowedDomainSuffixes: row._suffixes.get().length ? row._suffixes.get() : undefined,
      };
      if (isOn('dpf-dns-lan-' + uid) || isOn('dpf-dns-pub-' + uid)) {
        prof.defaultDNS = { lanDirect: isOn('dpf-dns-lan-' + uid), publicCname: isOn('dpf-dns-pub-' + uid) };
      }
      dkrProfiles[pname] = prof;
    });
    if (dpfDup) {
      showInvalid('Duplicate profile', `Two Docker discovery profiles are both called "${dpfDup}". Profile names must be unique.`);
      return;
    }
    if (Object.keys(dkrProfiles).length) dockerDiscovery.profiles = dkrProfiles;
    body.dockerDiscovery = dockerDiscovery;

    // All bars submit the same body, so all of them are held during the PUT
    // (a second bar must not double-submit), then put back as they were.
    const saveBtns = $$('.int-save');
    const wasDisabled = saveBtns.map((b) => b.disabled);
    saveBtns.forEach((b) => { b.disabled = true; });
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
    saveBtns.forEach((b, i) => { b.disabled = wasDisabled[i]; });
  }
}

// ---------- OVERVIEW ----------
// One row in the "needs attention" list: a severity dot, a title line (may
// carry inline markup, e.g. <b> around a name - callers esc() every dynamic
// piece before it goes in), a mono/muted detail line, and a deep link.
function attnRow(sev, title, detail, href, action) {
  return `<div class="feed-row attn">
      <span class="feed-tick ${sev}"></span>
      <div class="feed-body">
        <div class="feed-msg">${title}</div>
        <div class="feed-meta">${detail}</div>
      </div>
      <a class="btn ghost sm" href="${esc(href)}">${esc(action)}</a>
    </div>`;
}

async function viewOverview(c) {
  // Four independent reads, issued together, each tolerated on its own: a
  // source that fails contributes one warn row ("Could not read ...") instead
  // of failing the whole view. The two discovery status routes 501/404 when
  // the integration is not wired at all, which is not a failure - it just
  // means that source has nothing to report.
  const [healthR, certsR, idR, dkrR, hostsR] = await Promise.all([
    api('/api/health').then((r) => ({ ok: true, data: r.data || {} })).catch((e) => ({ ok: false, err: e })),
    api('/api/certificates').then((r) => ({ ok: true, data: arr(r.data) })).catch((e) => ({ ok: false, err: e })),
    api('/api/ingress-discovery/status').then((r) => ({ ok: true, data: r.data || {} })).catch((e) => ({ ok: false, err: e })),
    api('/api/docker-discovery/status').then((r) => ({ ok: true, data: r.data || {} })).catch((e) => ({ ok: false, err: e })),
    // For the hosts in maintenance: the summary counts only enabled and disabled.
    memoGet('/api/proxy-hosts').then((r) => ({ ok: true, data: arr(r.data) })).catch((e) => ({ ok: false, err: e })),
  ]);
  // Only for the empty-state summary's "hosts live" figure. GET /config/summary
  // is the sidebar's counts-only read and is memoized per route, so this adds
  // no extra request beyond what the shell already made for the nav.
  await loadCounts();

  const health = healthR.ok ? healthR.data : null;
  const certsWithStatus = certsR.ok ? certsR.data : [];

  const errRows = [];
  const warnRows = [];

  // ---- certificates: GET /api/certificates (source order: certs) ----
  if (certsR.ok) {
    certsWithStatus.forEach((ct) => {
      const name = esc(ct.name);
      const domains = domainListHtml(ct.domains);
      const typ = ct.type === 'acme' ? 'ACME' : (ct.type === 'custom' ? 'Custom' : esc(ct.type || 'cert'));
      const href = '#/certs/' + encodeURIComponent(ct.name);
      if (ct.state === 'error') {
        errRows.push(attnRow('err', `Certificate <b>${name}</b> failed to renew`,
          [typ, domains, ct.lastError ? esc(ct.lastError) : ''].filter(Boolean).join(' &middot; '),
          href, 'Open'));
      } else if (ct.state === 'expired') {
        const ago = ct.daysRemaining != null ? -ct.daysRemaining : null;
        errRows.push(attnRow('err', `Certificate <b>${name}</b> has expired`,
          [ago != null ? `expired ${plural(ago, 'day', 'days')} ago` : 'expired', domains].filter(Boolean).join(' &middot; '),
          href, 'Open'));
      } else if (ct.state === 'expiring') {
        const days = ct.daysRemaining;
        warnRows.push(attnRow('warn', `Certificate <b>${name}</b> expires in ${plural(days, 'day', 'days')}`,
          [typ, domains, ct.notAfter ? fmtDate(ct.notAfter) : ''].filter(Boolean).join(' &middot; '),
          href, 'Open'));
      }
    });
  } else {
    warnRows.push(attnRow('warn', 'Could not read certificates',
      esc((certsR.err && certsR.err.message) || 'request failed'), '#/certs', 'Open'));
  }

  // ---- upstream groups: GET /api/health (source order: upstreams) ----
  // UpstreamGroupHealth only carries a healthy/unhealthy count, not member
  // addresses, so the row can only say how many are down, not which - the
  // group editor is where that detail lives.
  if (health) {
    arr(health.upstreamGroups).forEach((g) => {
      if (g.unhealthy > 0) {
        errRows.push(attnRow('err',
          `Upstream group <b>${esc(g.name)}</b>: ${g.unhealthy} of ${plural(g.healthy + g.unhealthy, 'member', 'members')} unhealthy`,
          '', '#/upstreams/' + encodeURIComponent(g.name), 'Open group'));
      }
    });
  } else {
    warnRows.push(attnRow('warn', 'Could not read health status',
      esc((healthR.err && healthR.err.message) || 'request failed'), '#/settings/operations', 'Open'));
  }

  // ---- discovery: GET /api/ingress-discovery/status, /api/docker-discovery/status
  // (source order: discovery, ingress before docker) ----
  function attnDiscovery(res, label) {
    const discoveryTab = label === 'Docker' ? '#/integrations/docker' : '#/integrations/ingress';
    if (!res.ok) {
      const status = res.err && res.err.status;
      if (status === 501 || status === 404) return; // not enabled: no row
      warnRows.push(attnRow('warn', `Could not read ${label.toLowerCase()} discovery status`,
        esc((res.err && res.err.message) || 'request failed'), discoveryTab, 'Open'));
      return;
    }
    const st = res.data;
    if (st.error) {
      errRows.push(attnRow('err', `${label} discovery failed`, esc(st.error), discoveryTab, 'Open discovery'));
    }
    arr(st.hosts).forEach((h) => {
      if (h.action === 'skipped' && h.reason) {
        warnRows.push(attnRow('warn', `${label} discovery skipped <b>${esc(h.name)}</b>`,
          esc(h.reason), discoveryTab, 'Open discovery'));
      }
    });
  }
  attnDiscovery(idR, 'Ingress');
  attnDiscovery(dkrR, 'Docker');

  // ---- config warnings: GET /api/health (source order: config) ----
  if (health) {
    arr(health.configWarnings).forEach((w) => {
      warnRows.push(attnRow('warn', 'Config warning', esc(w), '#/history', 'View history'));
    });
  }

  // ---- hosts in maintenance: GET /api/proxy-hosts ----
  // A host in maintenance answers every request with a 503: not a fault, but
  // the one host state an operator must not forget, so each gets an info row.
  const maintHosts = hostsR.ok ? hostsR.data.filter((h) => !h.disabled && h.maintenance) : [];
  const infoRows = maintHosts.map((h) => attnRow('info',
    `Proxy host <b>${esc(arr(h.domains)[0] || h.name)}</b> is in maintenance`,
    'It answers 503 instead of passing requests to its upstream.',
    '#/hosts/' + encodeURIComponent(h.name), 'Open host'));

  const errCount = errRows.length;
  const warnCount = warnRows.length;

  // The one-line summary: "N hosts live, M in maintenance", certificates,
  // upstream members.
  const disabledHosts = hostsR.ok ? hostsR.data.filter((h) => h.disabled).length
    : ((state.summary && state.summary.disabled && state.summary.disabled['proxy-hosts']) || 0);
  const totalHosts = hostsR.ok ? hostsR.data.length : (state.counts.proxyHosts || 0);
  const liveHosts = Math.max(0, totalHosts - disabledHosts - maintHosts.length);
  const validCerts = certsWithStatus.filter((ct) => ct.state === 'valid');
  const upGroups = arr(health && health.upstreamGroups);
  const upHealthy = upGroups.reduce((n, g) => n + (g.healthy || 0), 0);
  const upTotal = upGroups.reduce((n, g) => n + (g.healthy || 0) + (g.unhealthy || 0), 0);
  const parts = [`${plural(liveHosts, 'host', 'hosts')} live` + (maintHosts.length ? `, ${maintHosts.length} in maintenance` : '')];
  if (validCerts.length) {
    const soonest = validCerts.reduce((min, ct) =>
      (ct.daysRemaining != null && (min == null || ct.daysRemaining < min)) ? ct.daysRemaining : min, null);
    parts.push(`${plural(validCerts.length, 'certificate', 'certificates')} valid`
      + (soonest != null ? `, next expiry in ${plural(soonest, 'day', 'days')}` : ''));
  }
  parts.push(upGroups.length ? `${upHealthy} of ${upTotal} upstream members healthy` : 'no upstream groups');

  let body;
  if (errCount + warnCount + infoRows.length > 0) {
    const headParts = [];
    if (errCount) headParts.push(`<span class="err-text">${errCount}</span> ${errCount === 1 ? 'problem' : 'problems'}`);
    if (warnCount) headParts.push(`<span class="warn-text">${warnCount}</span> ${warnCount === 1 ? 'warning' : 'warnings'}`);
    if (infoRows.length) headParts.push(`${plural(infoRows.length, 'host', 'hosts')} in maintenance`);
    body = `<div class="card">
      <div class="card-head">
        <div>
          <p class="section-label" style="margin:0 0 2px">Needs attention</p>
          <h3 style="margin:0">${headParts.join(' &middot; ')}</h3>
          <p class="muted ov-summary">${parts.join(' &middot; ')}</p>
        </div>
        <button class="btn ghost sm" id="ov-refresh" type="button">Refresh</button>
      </div>
      ${errRows.join('')}${warnRows.join('')}${infoRows.join('')}
    </div>`;
  } else {
    body = `<div class="empty-state">
      <div class="es-title">Nothing needs attention</div>
      <div class="es-sub">${parts.join(' &middot; ')}</div>
      <button class="btn ghost sm" id="ov-refresh" type="button">Refresh</button>
    </div>`;
  }

  c.innerHTML = `
    ${pageHead('Overview', `Status of <span class="mono">${esc(state.instance)}</span>: what needs attention right now.`)}
    ${setupOffered() ? setupCardHtml() : ''}
    ${body}`;
  wireSetupCard(c);

  const refresh = $('#ov-refresh');
  // Through route(), not viewOverview(c) directly: route() resets the request
  // memo (so the host count re-reads) and re-applies the shell banners that a
  // bare re-render would wipe.
  if (refresh) refresh.addEventListener('click', () => route());
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
// The one "disabled" marker: a muted dot plus the word, the quiet style of the
// hosts list's Status column. Every list that shows a disabled object (cards,
// the certificates table, API tokens) uses this, so it means one thing.
const DISABLED_CHIP = '<span class="chip flat muted"><span class="dot muted"></span>Disabled</span>';
const MANAGED_CHIPS = {
  'ingress-discovery': { text: 'k8s-managed', title: 'Derived from an annotated Kubernetes Ingress. Edits other than disable and maintenance are reverted on the next reconcile.' },
  'docker-discovery': { text: 'docker-managed', title: 'Derived from a labelled Docker container. Edits other than disable and maintenance are reverted on the next reconcile.' },
};

async function listHosts(c) {
  const [hostsR, mwR, certsR] = await Promise.all([
    api('/api/proxy-hosts'),
    refList('/api/middlewares', 'middlewares'),
    refList('/api/certificates', 'certificates'),
  ]);
  const hosts = arr(hostsR.data);
  // The Certificate column is what the handshake presents for the primary
  // domain, resolved like the editor's rail; tls.certificateRef is not read on
  // an HTTP host. A list that did not load says so instead of "no certificate".
  const certsOK = certsR !== null;
  const certs = arr(certsR);
  const mwType = {};
  arr(mwR).forEach((m) => { mwType[m.name] = m.type; });

  const addBtn = `<a class="btn primary" href="#/hosts/_new" data-wizard="hosts">${ICON.plus}Add proxy host</a>`;
  const head = viewHead('Proxy Hosts',
    'Public domains routed to an upstream, with the TLS, access and middleware that sit in between.');

  if (!hosts.length) {
    c.innerHTML = head + emptyState('No proxy hosts yet',
      'Add one with the domain your visitors type and the address of the service behind it - gpm picks the certificate by domain, so TLS needs no wiring here.',
      'Add proxy host', '#/hosts/_new');
    const es = c.querySelector('.empty-state a.btn');
    if (es) es.setAttribute('data-wizard', 'hosts');
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
    // A host belongs to the zone of its primary domain, the one the row shows:
    // switching a zone off hides exactly the hosts its chip counts.
    const zone = domains.length ? domainZone(domains[0]) : '';
    if (zone) zoneCounts[zone] = (zoneCounts[zone] || 0) + 1;
    const up = h.upstream || {};
    const upStr = h.upstreamGroupRef ? `group:${h.upstreamGroupRef}` : `${up.host || '?'}:${up.port != null ? up.port : '?'}`;
    const cert = certsOK ? certForDomain(certs, domains[0]) : null;
    const certName = cert ? cert.name : (certsOK ? 'no certificate' : '');
    const authMw = arr(h.middlewares).find((m) => mwType[m] === 'auth') || '';
    // "This host requires sign-in" is ONE fact whichever way it was configured,
    // so an inline auth block reads the same in this column as an attached auth
    // middleware does.
    const authInline = !!h.auth;
    const status = h.disabled ? 'disabled' : (h.maintenance ? 'maintenance' : 'live');
    const tags = arr(h.tags);
    const managed = managedBy(h);
    const mc = MANAGED_CHIPS[managed];
    const sourceTag = mc ? mc.text : '';
    const auth = authMw || (authInline ? 'inline' : 'none');
    return {
      h, domains, zone, upStr, cert, certName, authMw, authInline, managed, status, tags, sourceTag, auth,
      name: h.name || '',
      display: h.displayName || '',
      updated: h.updatedAt || '',
      // The filter haystack: every value the row is BUILT from, never the
      // markup it renders to; f scopes the same values by field.
      blob: [h.name, h.displayName, domains.join(' '), upStr, certName, authMw, authInline ? 'sign-in' : '', managed, sourceTag, status, tags.join(' ')]
        .join(' ').toLowerCase(),
      f: {
        domain: fvals(domains), name: fvals(h.name, h.displayName), upstream: fvals(upStr), cert: fvals(certName),
        auth: fvals(auth), status: fvals(status), tag: fvals(tags, sourceTag), source: fvals(managed || 'manual'),
        zone: fvals(zone), middleware: fvals(h.middlewares), access: fvals(h.accessLists),
      },
    };
  });
  const zoneEntries = Object.keys(zoneCounts)
    .map((z) => [z, zoneCounts[z]])
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const showZones = zoneEntries.length >= 2;
  // A zone that no longer has a host cannot be switched back on from a chip,
  // so a stale entry is dropped instead of silently filtering forever.
  const zonesBefore = zonesOff.size;
  zonesOff.forEach((z) => { if (!zoneCounts[z]) zonesOff.delete(z); });
  if (zonesOff.size !== zonesBefore) saveZonesOff();

  // The default order is the Domain column ascending, marked in its header from
  // the first paint; a stored choice only counts when it names a column.
  const SORTS = {
    domains: (r) => (r.domains[0] || r.name).toLowerCase(),
    upstream: (r) => r.upStr.toLowerCase(),
    cert: (r) => r.certName.toLowerCase(),
    auth: (r) => r.auth.toLowerCase(),
    status: (r) => r.status,
    tags: (r) => [r.sourceTag].concat(r.tags).filter(Boolean).join(' ').toLowerCase(),
    updated: (r) => r.updated || '',
  };
  const sort = loadSort(HOSTS_SORT_KEY, { key: 'domains', dir: 1 }, (k) => !!SORTS[k]);
  const selected = new Set();
  const FILTER_FIELDS = {
    domain: 'a domain', name: 'object or display name', upstream: 'upstream host:port or group', cert: 'covering certificate',
    auth: 'sign-in middleware, inline or none', status: 'live, disabled or maintenance', tag: 'a tag, or k8s-managed / docker-managed',
    source: 'ingress-discovery, docker-discovery or manual', zone: 'domain group', middleware: 'an attached middleware', access: 'an attached access list',
  };
  let terms = [];

  // One line per row, like every other list. The Tags column always shows: a
  // discovered host's source (k8s-managed, docker-managed) is its first tag,
  // and a display or object name that adds something sits beside the domain.
  const COLUMNS = [
    { key: 'domains', label: 'Domain', cls: 'col-domain' },
    { key: 'upstream', label: 'Upstream', cls: 'col-upstream hide-sm hide-md' },
    { key: 'cert', label: 'Certificate', cls: 'col-cert hide-sm' },
    { key: 'auth', label: 'Sign-in', cls: 'col-auth hide-sm' },
    { key: 'status', label: 'Status', cls: 'col-status' },
    { key: 'tags', label: 'Tags', cls: 'col-tags hide-sm' },
    { key: 'updated', label: 'Updated', cls: 'col-updated hide-sm hide-md' },
  ];
  function headHtml() {
    return `<tr><th class="sel-cell"><input type="checkbox" id="hostSelAll" aria-label="Select all listed hosts" /></th>`
      + COLUMNS.map((col) => sortThHtml(col, sort)).join('')
      + `</tr>`;
  }

  function visible() {
    const out = recs.filter((r) => {
      // With no chip row there is no control to undo a zone filter, so it
      // does not apply.
      const zoneOk = !showZones || !r.zone || !zonesOff.has(r.zone);
      return zoneOk && filterHit(r, terms);
    });
    const get = SORTS[sort.key] || SORTS.domains;
    return out.sort((a, b) => {
      const av = get(a), bv = get(b);
      if (av === bv) return a.name.localeCompare(b.name);
      return (av < bv ? -1 : 1) * sort.dir;
    });
  }

  function rowHtml(r) {
    const h = r.h;
    const rest = r.domains.slice(1);
    // The other domains open as their own block under line 2, one per line.
    const more = rest.length ? moreToggle(rest, rest.map((d) => `<span class="trunc" title="${esc(d)}">${esc(d)}</span>`).join(''), true) : null;
    const tls = certsOK
      ? certCoverHtml(r.cert, r.domains[0] || '')
      : '<span class="faint" title="The certificate list did not load.">-</span>';
    const auth = r.authMw
      ? `<span class="chip brand" title="${esc(r.authMw)}">${esc(r.authMw)}</span>`
      : (r.authInline ? `<span class="chip brand" title="Configured inline on this host, under Sign-in.">Sign-in</span>` : `<span class="chip">None</span>`);
    // live/disabled are the quiet states (dot + text, no pill); maintenance is
    // the one that should draw the eye, so it keeps the filled/bordered pill.
    const status = r.status === 'disabled'
      ? DISABLED_CHIP
      : (r.status === 'maintenance'
        ? `<span class="chip warn"><span class="dot warn"></span>Maintenance</span>`
        : `<span class="chip flat ok"><span class="dot ok"></span>Live</span>`);
    // Three tag chips, then "+N" that opens the rest under the cell, as the
    // domains do.
    const tagChip = (t) => `<span class="chip tag" title="${esc(t)}">${esc(t)}</span>`;
    const mc = MANAGED_CHIPS[r.managed];
    // The source is the first tag: where the host came from, for a discovered
    // one (its namespace/name is in the chip's title).
    const managedChip = mc ? `<span class="chip managed" title="${esc(mc.title)}">${esc(mc.text)}</span>` : '';
    const moreTags = r.tags.length > 3 ? moreToggle(r.tags.slice(3), r.tags.slice(3).map(tagChip).join(''), true) : null;
    const tagChips = managedChip + r.tags.slice(0, 3).map(tagChip).join('') + (moreTags ? moreTags.btn : '');
    // A display name or object name equal to the domain says nothing the
    // Domain cell did not, so it is left out.
    const primary = r.domains[0] || h.name;
    const ident = (r.display && r.display !== primary) ? r.display : (h.name !== primary ? h.name : '');
    const identHtml = ident ? `<span class="trunc faint" title="${esc(ident)}">${esc(ident)}</span>` : '';
    return `<tr class="clickable" data-name="${esc(h.name)}">
      <td class="sel-cell"><input type="checkbox" class="host-sel" data-name="${esc(h.name)}" aria-label="Select ${esc(h.name)}"${selected.has(h.name) ? ' checked' : ''} /></td>
      <td class="col-domain">
        <div class="cell-main"><a class="host trunc" href="#/hosts/${encodeURIComponent(h.name)}" title="${esc(r.domains.join(', ') || h.name)}">${esc(primary)}</a>${more ? more.btn : ''}${identHtml}</div>
        ${more ? more.list : ''}
      </td>
      <td class="mono col-upstream hide-sm hide-md"><span class="trunc" title="${esc(r.upStr)}">${esc(r.upStr)}</span></td>
      <td class="col-cert hide-sm">${tls}</td>
      <td class="col-auth hide-sm">${auth}</td>
      <td class="col-status">${status}</td>
      <td class="col-tags hide-sm" title="${esc([r.sourceTag].concat(r.tags).filter(Boolean).join(', '))}">${tagChips || '<span class="faint">-</span>'}${moreTags ? moreTags.list.replace('class="more-list"', 'class="more-list tag-list"') : ''}</td>
      <td class="mono faint col-updated hide-sm hide-md">${esc(r.updated ? fmtTime(r.updated) : '')}</td>
    </tr>`;
  }

  function zoneChipsInner() {
    return '<span class="chip-row-label">Domain</span>'
      + (zonesOff.size ? `<button type="button" class="chip zone all" data-zone-all="1">All</button>` : '')
      + zoneEntries.map(([z, n]) => `<button type="button" class="chip zone${zonesOff.has(z) ? ' off' : ''}" data-zone="${esc(z)}" aria-pressed="${!zonesOff.has(z)}">${esc(z)} (${n})</button>`).join('');
  }
  const zoneChipsHtml = showZones ? `<div class="chip-row" id="zoneChips" role="group" aria-label="Filter by domain">${zoneChipsInner()}</div>` : '';

  c.innerHTML = head + `
    <div class="toolbar">
      ${filterBoxHtml('hostFilter', 'filter hosts...', 'Filter hosts', FILTER_FIELDS)}
      ${zoneChipsHtml}
      <div class="toolbar-actions">${addBtn}</div>
    </div>
    <div id="hostBulk"></div>
    <div class="table-wrap">
      <table class="hosts-table">
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
      <button class="btn ghost sm" data-bulk="maint-on" type="button">Maintenance on</button>
      <button class="btn ghost sm" data-bulk="maint-off" type="button">Maintenance off</button>
      <button class="btn ghost sm" data-bulk="tag-add" type="button">Add tag</button>
      <button class="btn ghost sm" data-bulk="tag-remove" type="button">Remove tag</button>
      <button class="btn ghost sm" data-bulk="clear" type="button">Clear</button>
    </div>`;
    bulkWrap.querySelectorAll('[data-bulk]').forEach((b) => {
      b.addEventListener('click', () => runBulk(b.dataset.bulk));
    });
    // The bar is built on selection, after applyReadOnlyGating has already run
    // over this view, so read-only gating (an HA follower or a read-only role)
    // is re-applied here (the banner is already on the page; only the controls
    // need it). "Clear" only deselects, so it stays live.
    if (isReadOnly()) {
      bulkWrap.querySelectorAll('[data-bulk]:not([data-bulk="clear"])').forEach((el) => {
        gateControl(el, false, readOnlyReason());
      });
    }
  }

  function render() {
    const rows = visible();
    // A bulk action applies to what the operator can see: a name the filter or
    // a zone chip now hides leaves the selection.
    const shown = new Set(rows.map((r) => r.name));
    Array.from(selected).forEach((n) => { if (!shown.has(n)) selected.delete(n); });
    $('#hostHead').innerHTML = headHtml();
    $('#hostRows').innerHTML = rows.length
      ? rows.map(rowHtml).join('')
      : `<tr><td colspan="${COLUMNS.length + 1}" class="list-empty">No host matches this filter.</td></tr>`;
    highlightTerms($('#hostRows'), terms);
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
    wireSortHead($('#hostHead'), sort, HOSTS_SORT_KEY, render);
  }

  function wireRows() {
    $$('#hostRows tr.clickable').forEach((tr) => {
      tr.addEventListener('click', (e) => {
        if (e.target.closest('.sel-cell') || e.target.closest('a')) return;
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
  }

  // Bulk edits are ordinary whole-object PUTs, one per host and in order: the
  // API has no batch endpoint, each write is its own git commit, and a partial
  // run has to leave the hosts it already changed changed. So the progress
  // toast counts them, and a failure stops and says which host it stopped on
  // rather than carrying on and reporting a tidy total.
  async function runBulk(action) {
    if (action === 'clear') { selected.clear(); render(); return; }
    const names = Array.from(selected);
    let targets = names.map((n) => hosts.find((h) => h.name === n)).filter(Boolean);
    if (!targets.length) return;
    // Discovery rewrites a managed host's tags from its template on the next
    // reconcile, so a tag change there would be reverted: those are skipped,
    // and the dialog and the result say so.
    let skipped = [];
    if (action === 'tag-add' || action === 'tag-remove') {
      skipped = targets.filter((h) => managedBy(h));
      targets = targets.filter((h) => !managedBy(h));
      if (!targets.length) {
        toast('No hosts to change', `The selected host${skipped.length === 1 ? ' is' : 's are'} discovery-managed: tags come from the discovery template.`, 'err');
        return;
      }
    }
    const one = targets.length === 1;
    const many = `<b>${targets.length}</b> proxy host${one ? '' : 's'}`;
    // Every confirm names its targets, so a selection is never acted on blind.
    const shownNames = targets.slice(0, 12).map((h) => `<li class="mono">${esc(arr(h.domains)[0] || h.name)}</li>`).join('');
    const listHtml = `<ul class="bulk-targets">${shownNames}${targets.length > 12 ? `<li class="muted">and ${targets.length - 12} more</li>` : ''}</ul>`;
    const skipHtml = skipped.length
      ? `<p class="muted">${plural(skipped.length, 'discovery-managed host', 'discovery-managed hosts')} skipped (${skipped.map((h) => esc(arr(h.domains)[0] || h.name)).join(', ')}): tags come from the discovery template.</p>`
      : '';
    let mutate, title, confirmLabel, body, tag = '';

    if (action === 'enable' || action === 'disable') {
      const off = action === 'disable';
      title = off ? 'Disable the selected hosts?' : 'Enable the selected hosts?';
      confirmLabel = off ? 'Disable hosts' : 'Enable hosts';
      body = off
        ? `<p>${many} ${one ? 'stops' : 'stop'} being served: gpm answers on ${one ? 'its domains' : 'their domains'} as if ${one ? 'it was' : 'they were'} not configured. ${one ? 'Its' : 'Their'} config is kept.</p>${listHtml}<p>Each host is committed to git separately.</p>`
        : `<p>${many} ${one ? 'starts' : 'start'} being served again with the config ${one ? 'it already holds' : 'they already hold'}.</p>${listHtml}<p>Each host is committed to git separately.</p>`;
      mutate = (h) => { const o = Object.assign({}, h); if (off) o.disabled = true; else delete o.disabled; return o; };
    } else if (action === 'maint-on' || action === 'maint-off') {
      const on = action === 'maint-on';
      title = on ? 'Put the selected hosts into maintenance?' : 'Take the selected hosts out of maintenance?';
      confirmLabel = on ? 'Maintenance on' : 'Maintenance off';
      body = on
        ? `<p>${many} ${one ? 'answers' : 'answer'} every request with the maintenance page (503 with Retry-After) instead of proxying, until switched back.</p>${listHtml}<p>Each host is committed to git separately.</p>`
        : `<p>${many} ${one ? 'goes' : 'go'} back to proxying requests.</p>${listHtml}<p>Each host is committed to git separately.</p>`;
      mutate = (h) => { const o = Object.assign({}, h); if (on) o.maintenance = true; else delete o.maintenance; return o; };
    } else if (action === 'tag-add' || action === 'tag-remove') {
      const add = action === 'tag-add';
      const answered = await confirmModal({
        title: add ? 'Add a tag to the selected hosts?' : 'Remove a tag from the selected hosts?',
        body: `<p>Applied to ${many}. Tags are free-form labels used for grouping and filtering; they change nothing about how a host is served.</p>${listHtml}${skipHtml}`,
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

    if (action === 'enable' || action === 'disable' || action === 'maint-on' || action === 'maint-off') {
      const ok = await confirmModal({ title, body, confirmLabel, danger: action === 'disable' || action === 'maint-on' });
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
        // Through route(), not listHosts(c): route() re-applies the shell
        // banners and read-only gating a bare re-render would drop. Skipped
        // when the operator has already moved to another page.
        if (c.isConnected) await route();
        return;
      }
    }
    prog.done('Done', `${plural(done, 'host', 'hosts')} updated and committed.`
      + (skipped.length ? ` ${plural(skipped.length, 'discovery-managed host', 'discovery-managed hosts')} skipped.` : ''), 'ok');
    refreshHeadSha();
    if (c.isConnected) await route();
  }

  wireFilter('hostFilter', FILTER_FIELDS, () => fieldValues(recs), (t) => { terms = t; render(); }, 'hosts');
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
// Tabs in the title band that switch panels in the content area. In-page only:
// a hash change re-renders the editor and would drop unsaved edits. Roving
// tabindex with the arrow keys, Home and End, as the Settings tabs do.
function showPageTab(id) {
  $$('#pageTabs .tab-btn').forEach((b) => {
    const on = b.dataset.tab === id;
    b.classList.toggle('on', on);
    b.setAttribute('aria-selected', on ? 'true' : 'false');
    b.tabIndex = on ? 0 : -1;
  });
  $$('#content .tab-panel[data-tab]').forEach((p) => { p.hidden = p.dataset.tab !== id; });
}
function wirePageTabs(initial) {
  const tabs = $$('#pageTabs .tab-btn');
  if (!tabs.length) return;
  tabs.forEach((b) => b.addEventListener('click', () => showPageTab(b.dataset.tab)));
  wirePageTabKeys();
  showPageTab(initial || tabs[0].dataset.tab);
}
// Arrow, Home and End keys move between the band's tabs and activate the one
// they land on (a click, so a view that routes its tabs through the hash, as
// Settings does, sees it the same way). The bar outlives every page, so its
// key handler is attached once.
function wirePageTabKeys() {
  const bar = $('#pageTabs');
  if (!bar) return;
  if (!bar.dataset.wired) {
    bar.dataset.wired = '1';
    bar.addEventListener('keydown', (e) => {
      const all = $$('#pageTabs .tab-btn');
      const i = all.indexOf(document.activeElement);
      if (i === -1) return;
      let j = -1;
      if (e.key === 'ArrowRight') j = (i + 1) % all.length;
      else if (e.key === 'ArrowLeft') j = (i - 1 + all.length) % all.length;
      else if (e.key === 'Home') j = 0;
      else if (e.key === 'End') j = all.length - 1;
      if (j === -1) return;
      e.preventDefault();
      all[j].focus();
      all[j].click();
    });
  }
}
// A field on a hidden tab panel is worth nothing until its tab is shown.
function revealTabPanel(el) {
  const panel = el && el.closest && el.closest('.tab-panel[data-tab]');
  if (!panel || !panel.hidden) return;
  const tab = $(`#pageTabs .tab-btn[data-tab="${panel.dataset.tab}"]`);
  if (tab) tab.click();
}

// The rail's request chain, in the order the data plane evaluates it: rate
// limit, access lists, bouncer, sign-in, guard, headers, rewrite, then the
// upstream. An inline block sits at its kind's position, just before the
// referenced middlewares of that kind.
const CHAIN_TYPE_ORDER = ['rate-limit', 'bouncer', 'auth', 'guard', 'headers', 'rewrite'];
function renderRailChain(el, ctx) {
  if (!el) return;
  const KIND_LABEL = MW_TYPE_LABEL;
  const byType = (t) => ctx.mwSelected.filter((m) => (ctx.mwType[m] || '') === t).map((m) => ({ name: m, kind: KIND_LABEL[t] || t }));
  const items = [];
  if (ctx.inlineRateLimit) items.push({ name: 'rate limit', kind: 'on this host' });
  items.push(...byType('rate-limit'));
  ctx.alSelected.forEach((a) => items.push({ name: a, kind: 'access list' }));
  items.push(...byType('bouncer'));
  if (ctx.inlineAuth) items.push({ name: ctx.inlineAuth, kind: 'sign-in' });
  items.push(...byType('auth'));
  ['guard', 'headers', 'rewrite'].forEach((t) => items.push(...byType(t)));
  ctx.mwSelected.forEach((m) => {
    const t = ctx.mwType[m] || '';
    if (CHAIN_TYPE_ORDER.indexOf(t) === -1) items.push({ name: m, kind: t || 'middleware' });
  });
  items.push({ name: ctx.upstreamStr, kind: 'upstream', end: true });
  el.innerHTML = items.map((it, i) => `<li${it.end ? ' class="end"' : ''}><span class="n">${i + 1}</span><span class="mono trunc" title="${esc(it.name)}">${esc(it.name)}</span><span class="kind">${esc(it.kind)}</span></li>`).join('');
}

// The YAML the config file holds for this host, from a dry run of the stored
// object: the same serializer and checks as a save, nothing written. The dry
// run stamps updatedAt with the current time, as a save would; every other
// line is the file's.
async function showHostYaml(h) {
  let res;
  try {
    res = await api('/api/proxy-hosts/' + encodeURIComponent(h.name) + '?dryRun=1', { method: 'PUT', body: h });
  } catch (e) { toastErr(e, 'Could not show the YAML'); return; }
  const reply = res.data || {};
  const text = reply.yaml || '';
  await confirmModal({
    title: h.name,
    body: `<div class="yaml-view"><div class="code-head"><span>${esc(reply.path || '')}</span><button class="btn ghost sm" type="button" id="yaml-copy">Copy</button></div><pre class="code">${yamlHtml(text)}</pre></div>`,
    confirmLabel: 'Close',
    danger: false,
    closeOnly: true,
    wide: true,
    onOpen: (wrap) => { const b = wrap.querySelector('#yaml-copy'); if (b) b.addEventListener('click', () => copyText(text, b)); },
  });
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
      ? 'Add one under <a href="#/clientcas">Client CAs</a> first - it is the trust anchor client certificates are verified against.'
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

  // Same markup as the hosts list's Status column: live and disabled are the
  // quiet .flat states, maintenance keeps the pill.
  const statusChip = h.disabled
    ? DISABLED_CHIP
    : ((h.maintenance || globalMaint) && !isNew
      ? `<span class="chip warn"><span class="dot warn"></span>Maintenance</span>`
      : `<span class="chip flat ok"><span class="dot ok"></span>${isNew ? 'New' : 'Live'}</span>`);

  // Progressive disclosure. A proxy host has thirteen sections and a normal one
  // uses four of them, so everything optional collapses to a single line that
  // says what it currently holds ("Locations (2)", "Timeouts (defaults)",
  // "Middleware (sso, rate-limit)"). A section that holds a non-default value
  // opens itself, so nothing configured is ever hidden from someone scanning
  // the page - the fold saves scrolling, it never hides state.
  //
  // The four that stay open are the ones every host needs: domains, the
  // upstream, TLS, and the access/middleware chain.
  const locCount = arr(h.locations).length;
  const locSummary = locCount ? `${plural(locCount, 'path override', 'path overrides')}` : 'none - every path goes to the upstream above';

  const tmo = h.timeouts || {};
  const tmoParts = [];
  if (tmo.connectSeconds) tmoParts.push(`connect ${tmo.connectSeconds}s`);
  if (tmo.readSeconds) tmoParts.push(`read ${tmo.readSeconds}s`);
  if (h.robotsNoIndex) tmoParts.push('indexing discouraged');
  const tmoSummary = tmoParts.length ? tmoParts.join(', ') : 'defaults';

  const compSummary = comp.enabled
    ? ['gzip on', comp.minBytes ? `min ${plural(comp.minBytes, 'byte', 'bytes')}` : '', arr(comp.types).length ? plural(arr(comp.types).length, 'content type', 'content types') : ''].filter(Boolean).join(', ')
    : 'off';

  const epParts = [];
  if (ep.dir) epParts.push(`directory ${ep.dir}`);
  if (ep.inline && Object.keys(ep.inline).length) epParts.push(`${Object.keys(ep.inline).length} inline`);
  if (arr(ep.interceptUpstream).length) epParts.push(`intercepts ${arr(ep.interceptUpstream).join(', ')}`);
  const epSummary = epParts.length ? epParts.join(', ') : 'inherited from Error Pages';

  const secHdrCount = (h.securityHeaders && typeof h.securityHeaders === 'object') ? Object.keys(h.securityHeaders).length : 0;
  const secHdrSummary = secHdrCount ? `${secHdrCount} set` : 'fleet defaults';

  const stripCount = arr(h.stripResponseHeaders).length;
  const stripSummary = stripCount ? `${plural(stripCount, 'header', 'headers')}` : 'none';

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

  // Title-band tabs. Nothing on a tab marks a non-default value: every optional
  // section says in its fold summary what it holds, and the rail shows the
  // certificate and the request chain.
  const HOST_TABS = [['routing', 'Routing'], ['tls', 'TLS'], ['access', 'Access'], ['headers', 'Headers'], ['advanced', 'More']];
  const hostTabsHtml = HOST_TABS.map(([id, label]) => `<button class="tab-btn" type="button" role="tab" id="host-tab-${id}" aria-controls="host-panel-${id}" data-tab="${id}" aria-selected="false" tabindex="-1">${label}</button>`).join('');
  const panel = (id, body) => `<div class="tab-panel" data-tab="${id}" role="tabpanel" id="host-panel-${id}" aria-labelledby="host-tab-${id}" hidden><div class="stack">
${body}      </div></div>`;
  const mc = MANAGED_CHIPS[managedBy(h)];
  const railManaged = mc ? `<span class="muted">${esc(mc.title)}</span>` : '';

  c.innerHTML = `
    ${pageHead(esc(isNew ? 'New proxy host' : (arr(h.domains)[0] || h.name)), isNew ? 'Add a proxy host.' : 'Edit this proxy host.',
      `<a href="#/hosts">Proxy Hosts</a>${ICON.chevron}<span>${isNew ? 'new' : esc(h.name)}</span>`, isNew ? '' : (arr(h.domains).join(', ') || h.name),
      statusChip + (h.updatedAt ? `<span class="head-meta">last change ${esc(fmtTime(h.updatedAt))}</span>` : ''),
      isNew ? '' : `${isRoleReadOnly() ? '' : '<button class="btn ghost sm" id="hostYamlBtn" type="button">YAML</button>'}<button class="btn ghost sm" id="hostCloneBtn" type="button">Clone</button><button class="btn danger-soft sm" id="deleteBtn" type="button">${ICON.trash}Delete</button>`,
      hostTabsHtml)}

    <div class="edit-grid">
      <div class="edit-main">
${panel('routing', `        ${identityCard('f', h, {
    section: 'hosts', isNew, foldable: true, placeholder: seed && seed.origName ? seed.origName + '-copy' : 'internal-name',
    extra: `<div class="toggle-line" style="margin-top:6px">
            <div class="tl-text"><div class="nm">Maintenance</div><div class="ds">${globalMaint
              ? 'Fleet-wide maintenance is on under <a href="#/settings/operations">Settings &gt; Operations</a>: every host serves the maintenance page regardless of this switch.'
              : 'Serve a 503 maintenance page instead of proxying. The host keeps its domains and certificate.'}</div></div>
            ${switchHtml('f-maintenance', !!h.maintenance, 'Maintenance', 'proxyHost.maintenance')}
          </div>`,
  })}

        <div class="card form-section">
          <p class="section-label">Domains</p>
          <div class="chip-input" id="f-domains" data-hint="proxyHost.domains" data-path="domains"></div>
          <div class="hint">Press Enter after each domain.</div>
        </div>

        <div class="card form-section">
          <p class="section-label">Upstream</p>
          ${upstreamGroups.length ? `<div class="field-group" style="margin-bottom:10px">
            <label>Upstream group</label>
            <select class="field mono" id="f-upgroup" data-hint="proxyHost.upstreamGroupRef" data-path="upstreamGroupRef">
              <option value="">(single upstream)</option>
              ${upstreamGroups.map((g) => `<option value="${esc(g.name)}"${h.upstreamGroupRef === g.name ? ' selected' : ''}>${esc(g.name)} (${plural(arr(g.upstreams).length, 'upstream', 'upstreams')})</option>`).join('')}
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
              <input class="field mono" id="f-upport" data-hint="proxyHost.upstream.port" data-path="upstream.port" type="number" value="${esc(up.port != null ? up.port : '')}" placeholder="3000" />
            </div>
          </div>
          ${upstreamExtraHtml('f', up)}
        </div>

        ${foldHtml('f-locs-card', 'Locations', locSummary, locCount > 0, `
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
            <div class="field-group">
              <label>Content types</label>
              <textarea class="field mono" id="f-gzip-types" data-hint="proxyHost.compression.types" data-path="compression.types" rows="2" placeholder="text/html, application/json, ...">${esc(arr(comp.types).join(', '))}</textarea>
              <div class="hint">Comma-separated media types. Blank uses the built-in text/JSON/JS/CSS/SVG/XML list. Never applied to websocket upgrades, streaming, or event-stream responses.</div>
            </div>
          </div>
        `)}

`)}
${panel('tls', `        <div class="card form-section">
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
              <div class="hint">Pair <span class="mono">optional</span> with a client-cert sign-in middleware for LAN-exempt enforcement.</div>
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

`)}
${panel('access', `        <div class="card form-section">
          <p class="section-label">Request chain</p>
          <p class="muted" style="font-size:13.5px;margin:0 0 10px">Attached: <b id="f-mw-attached">${esc(selMw.length ? selMw.join(', ') : 'none')}</b></p>
          <div class="check-list" id="f-mw" data-hint="proxyHost.middlewares" data-path="middlewares">
            ${!mwListOK ? refListUnavailableHtml('the middleware list') : (mwSorted.length ? mwSorted.map((m) => `
              <label class="check-item"><input type="checkbox" value="${esc(m.name)}"${selMw.indexOf(m.name) !== -1 ? ' checked' : ''}/>${esc(m.name)}<span class="ci-ty">${esc(MW_TYPE_LABEL[m.type] || m.type || '')}</span></label>
            `).join('') : '<div class="check-empty">No middleware defined yet.</div>')}
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Access lists</p>
          <p class="muted" style="font-size:13.5px;margin:0 0 10px">Attached: <b id="f-al-attached">${esc(selAl.length ? selAl.join(', ') : 'none')}</b></p>
          <div class="check-list" id="f-al" data-hint="proxyHost.accessLists" data-path="accessLists">
            ${!alListOK ? refListUnavailableHtml('the access list index') : (alSorted.length ? alSorted.map((a) => `
              <label class="check-item"><input type="checkbox" value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 ? ' checked' : ''}/>${esc(a.name)}<span class="ci-ty">access-list</span></label>
            `).join('') : '<div class="check-empty">No access lists defined yet.</div>')}
          </div>
          <div class="hint" id="f-inline-order"${(h.auth || h.rateLimit) ? '' : ' hidden'} style="margin-top:8px">Sign-in and rate limit set below each take the same place in the chain as a middleware of their kind, just before those middlewares. The Request chain in the rail shows the full order.</div>
        </div>

        ${inlineFoldHtml('f-auth', 'Sign-in', 'Gate this host through an identity provider. For a gate shared by several hosts, use a middleware instead.',
    !!h.auth, authFoldSummary(h.auth), authBlockHtml('f-auth', h.auth, idps), 'proxyHost.auth')}

        ${inlineFoldHtml('f-rl', 'Rate limit', 'Throttle this host per client IP. For a limit shared by several hosts, use a middleware instead.',
    !!h.rateLimit, rateFoldSummary(h.rateLimit), rateLimitBlockHtml('f-rl', h.rateLimit), 'proxyHost.rateLimit')}
`)}
${panel('headers', `        ${foldHtml('f-sech-card', 'Response security headers', secHdrSummary, secHdrCount > 0, `
          <div id="f-secheaders" data-hint="proxyHost.securityHeaders" data-path="securityHeaders"></div>
          <button class="btn ghost sm" id="f-secheaders-add" type="button" style="margin-top:6px">${ICON.plus}Add header</button>
          <div class="hint" style="margin-top:6px">Scope: <span class="mono">all</span>, <span class="mono">generated-only</span> (only responses gpm writes itself - denials, sign-in redirects, error pages) or <span class="mono">proxied-only</span>. Strict-Transport-Security is not settable here; HSTS lives in the TLS section.</div>
        `)}

        ${foldHtml('f-strip-card', 'Strip response headers', stripSummary, stripCount > 0, `
          <div class="field-group"><label>Header names</label><div class="chip-input" id="f-striphdrs" data-hint="proxyHost.stripResponseHeaders" data-path="stripResponseHeaders"></div></div>
          <div class="hint">Hop-by-hop and response-semantic names (Content-Type, Content-Length, Content-Encoding, Location, Vary, the WebSocket handshake) are refused on save.</div>
        `)}

`)}
${panel('advanced', `        ${foldHtml('f-errp-card', 'Error pages', epSummary, epParts.length > 0, `
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

        ${foldHtml('f-adv-card', 'Client addresses', tpSummary, tpMode !== 'inherit', `
          <div class="field-group">
            <label>Trusted proxies (override)</label>
            <div class="seg" id="f-tp-seg" data-hint="proxyHost.trustedProxies.mode">
              <button type="button" class="seg-btn${tpMode === 'inherit' ? ' on' : ''}" data-tp="inherit" aria-pressed="${tpMode === 'inherit'}">Inherit fleet setting</button>
              <button type="button" class="seg-btn${tpMode === 'none' ? ' on' : ''}" data-tp="none" aria-pressed="${tpMode === 'none'}">Trust nobody on this host</button>
              <button type="button" class="seg-btn${tpMode === 'custom' ? ' on' : ''}" data-tp="custom" aria-pressed="${tpMode === 'custom'}">Custom list</button>
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
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">LAN direct</div><div class="ds">Local CNAME on the LAN resolver (Pi-hole)</div></div>
            ${switchHtml('f-dns-lan', !!(h.dns && h.dns.lanDirect), 'LAN direct', 'proxyHost.dns.lanDirect')}
          </div>
          <div class="hint" id="f-dns-lan-hint" style="display:none">Pi-hole DNS sync is not configured yet. This takes effect once it is, under <a href="#/integrations">Integrations &gt; DNS sync</a>.</div>
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">Public CNAME</div><div class="ds">Record in the authoritative public zone (Cloudflare)</div></div>
            ${switchHtml('f-dns-public', !!(h.dns && h.dns.publicCname), 'Public CNAME', 'proxyHost.dns.publicCname')}
          </div>
          <div class="hint" id="f-dns-public-hint" style="display:none">Cloudflare DNS sync is not configured yet. This takes effect once it is, under <a href="#/integrations">Integrations &gt; DNS sync</a>.</div>
        `)}
`)}

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Each save is recorded as one git commit.</div>
      <div style="display:flex;gap:10px">
        <button class="btn primary" id="saveBtn" type="button">${isNew ? 'Add proxy host' : 'Save changes'}</button>
        <a class="btn ghost" href="#/hosts">Cancel</a>
      </div>
    </div>
      </div>

      <aside class="rail">
        <div class="card rail-card"><dl class="dl">
          <div><dt>Status</dt><dd>${statusChip}${h.updatedAt ? `<span class="muted">last change ${esc(fmtTime(h.updatedAt))}</span>` : ''}${railManaged}</dd></div>
          <div><dt>Certificate</dt><dd id="railCert"></dd></div>
          <div><dt>Request chain</dt><dd><ol class="chain" id="railChain"></ol></dd></div>
          <div><dt>Config file</dt><dd><span class="mono" id="railPath">${esc('proxy-hosts/' + (h.name || '<name>') + '.yaml')}</span><span class="muted">${isNew ? 'written by the first save' : 'one git commit per save, see <a href="#/history">History</a>'}</span></dd></div>
        </dl></div>
      </aside>
    </div>`;
  wirePageTabs('routing');
  // A new host's config path follows the name being typed (a clone arrives
  // with the name chosen in its dialog already filled in).
  if (isNew) $('#f-name').addEventListener('input', () => { $('#railPath').textContent = 'proxy-hosts/' + ($('#f-name').value.trim() || '<name>') + '.yaml'; });

  // domains chip input
  const domainsCtl = makeChipInput($('#f-domains'), arr(h.domains), 'add domain...', (d) => {
    $('#f-cert-auto').innerHTML = certCoverageHtml(certs, d);
    refreshRailCert(d);
    refreshFlow();
  });
  const tagsCtl = wireIdentityCard('f', h);

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
    $$('#f-tp-seg .seg-btn').forEach((x) => { x.classList.toggle('on', x === b); x.setAttribute('aria-pressed', String(x === b)); });
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
  // The page has been rendered and wired above; if the operator moved on
  // during this await, everything below would look up the NEW page's ids.
  if (!c.isConnected) return;
  const dnsHint = (id, available) => {
    const el = $(id);
    if (el) el.style.display = available ? 'none' : '';
  };
  dnsHint('#f-dns-lan-hint', hasCapability('dnsSync.piholeEnabled'));
  dnsHint('#f-dns-public-hint', hasCapability('dnsSync.cloudflareEnabled'));

  // Fold summaries and the two "Attached:" lines follow the form: each fold's
  // summary is recomputed from its controls on any edit in the editor (the
  // same events that move the rail), so a collapsed fold never says what the
  // host held when the page opened. Location rows add their own entries.
  function hostHasSignIn() {
    return isOn('f-auth-on') || $$('#f-mw input:checked').some((i) => mwType[i.value] === 'auth');
  }
  const foldSums = {
    'f-locs-card': () => (locCtls.length ? `${plural(locCtls.length, 'path override', 'path overrides')}` : 'none - every path goes to the upstream above'),
    'f-crawl-card': () => {
      const parts = [];
      const cs = parseInt($('#f-to-connect').value, 10); if (cs > 0) parts.push(`connect ${cs}s`);
      const rs = parseInt($('#f-to-read').value, 10); if (rs > 0) parts.push(`read ${rs}s`);
      if (isOn('f-robots')) parts.push('indexing discouraged');
      return parts.length ? parts.join(', ') : 'defaults';
    },
    'f-comp-card': () => {
      if (!isOn('f-gzip')) return 'off';
      const mb = parseInt($('#f-gzip-min').value, 10);
      const types = $('#f-gzip-types').value.split(',').map((s) => s.trim()).filter(Boolean).length;
      return ['gzip on', mb > 0 ? `min ${plural(mb, 'byte', 'bytes')}` : '', types ? plural(types, 'content type', 'content types') : ''].filter(Boolean).join(', ');
    },
    'f-errp-card': () => {
      const parts = [];
      const dir = $('#f-errp-dir').value.trim(); if (dir) parts.push(`directory ${dir}`);
      const raw = $('#f-errp-inline').value.trim();
      if (raw) { try { const n = Object.keys(JSON.parse(raw) || {}).length; if (n) parts.push(`${n} inline`); } catch (e) { parts.push('inline (not valid JSON)'); } }
      const ic = $('#f-errp-intercept').value.split(',').map((s) => s.trim()).filter(Boolean);
      if (ic.length) parts.push(`intercepts ${ic.join(', ')}`);
      return parts.length ? parts.join(', ') : 'inherited from Error Pages';
    },
    'f-sech-card': () => { const n = $$('#f-secheaders > .loc-row').length; return n ? `${n} set` : 'fleet defaults'; },
    'f-strip-card': () => { const n = stripCtl.get().length; return n ? `${plural(n, 'header', 'headers')}` : 'none'; },
    'f-adv-card': () => {
      const mode = tpChoice();
      if (mode === 'custom') { const l = trustedCtl.get(); return l.length ? `trusted proxies: ${l.join(', ')}` : 'trusted proxies: custom list (empty)'; }
      return mode === 'none' ? 'trusted proxies: none (connection address only)' : 'trusted proxies inherited from Settings';
    },
    'f-dns-card': () => [isOn('f-dns-lan') ? 'LAN direct' : '', isOn('f-dns-public') ? 'public CNAME' : ''].filter(Boolean).join(', ') || 'off',
    'f-auth-card': () => authFoldSummary(isOn('f-auth-on') ? hostAuthCtl.peek() : null),
    'f-rl-card': () => rateFoldSummary(isOn('f-rl-on') ? hostRlCtl.peek() : null),
  };
  const refreshSums = liveFoldSummaries(c.querySelector('.edit-main'), foldSums);
  function refreshAttached() {
    const mw = $('#f-mw-attached'); if (mw) mw.textContent = mwListOK ? (curMw().join(', ') || 'none') : mw.textContent;
    const al = $('#f-al-attached'); if (al) al.textContent = alListOK ? (curAl().join(', ') || 'none') : al.textContent;
  }

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
    // A path override has its own explicit scheme (the model has no fallback):
    // a row without one starts on the host's current scheme.
    const lScheme = lu.scheme || $('#f-scheme').value || 'http';
    const p = 'loc' + (++locSeq);
    const div = document.createElement('div');
    div.className = 'sub-block loc-block';
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
        <select class="field mono loc-scheme" data-hint="proxyHost.locations.upstream.scheme" style="flex:0 0 90px" aria-label="Upstream scheme"${lu.scheme ? '' : ' data-follow="1"'}><option value="http"${lScheme === 'http' ? ' selected' : ''}>http</option><option value="https"${lScheme === 'https' ? ' selected' : ''}>https</option></select>
        <input class="field mono loc-host" data-hint="proxyHost.locations.upstream.host" style="flex:1 1 110px" value="${esc(lu.host || '')}" placeholder="host (optional)" aria-label="Upstream host" />
        <input class="field mono loc-port" data-hint="proxyHost.locations.upstream.port" type="number" style="flex:0 0 80px" value="${esc(lu.port != null ? lu.port : '')}" placeholder="port" aria-label="Upstream port" />
        <button class="icon-btn loc-del" type="button" aria-label="Remove location">${ICON.x}</button>
      </div>
      <div class="toggle-line" style="margin-top:6px">
        <div class="tl-text"><div class="nm">Strip prefix</div><div class="ds">Removes the location path from the request before it reaches the upstream: <span class="mono">/app/foo</span> arrives as <span class="mono">/foo</span>, and <span class="mono">/app</span> as <span class="mono">/</span>. Use it when the app expects to live at the root.</div></div>
        ${switchHtml(p + '-strip', !!loc.stripPrefix, 'Strip prefix', 'proxyHost.locations.stripPrefix')}
      </div>
      <div class="hint loc-strip-preview" hidden></div>
      ${upstreamExtraHtml(p, lu)}
      <div class="inline-fields">
        <div class="field-group"><label>Middlewares</label>
          <select multiple class="field mono loc-mw" data-hint="proxyHost.locations.middlewares" size="4" aria-label="Location middlewares" title="Middlewares for this path only (blank = host chain applies)">${middlewares.map((m) => `<option value="${esc(m.name)}"${arr(loc.middlewares).indexOf(m.name) !== -1 ? ' selected' : ''}>${esc(m.name)}</option>`).join('')}</select>
        </div>
        <div class="field-group"><label>Access lists</label>
          <select multiple class="field mono loc-al" data-hint="proxyHost.locations.accessLists" size="4" aria-label="Location access lists" title="Access lists for this path only (blank = host lists apply)">${accessLists.map((a) => `<option value="${esc(a.name)}"${arr(loc.accessLists).indexOf(a.name) !== -1 ? ' selected' : ''}>${esc(a.name)}</option>`).join('')}</select>
        </div>
      </div>
      ${inlineFoldHtml(p + '-auth', 'Sign-in', 'Gate this path. The host\'s own sign-in still applies here.',
    !!loc.auth, authFoldSummary(loc.auth, { hostSignIn: hostHasSignIn() }), authBlockHtml(p + '-auth', loc.auth, idps), 'proxyHost.locations.auth')}
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
    foldSums[p + '-auth-card'] = () => authFoldSummary(isOn(p + '-auth-on') ? ctl.auth.peek() : null, { hostSignIn: hostHasSignIn() });
    foldSums[p + '-rl-card'] = () => rateFoldSummary(isOn(p + '-rl-on') ? ctl.rl.peek() : null);

    // An untouched preselected scheme keeps following the host's (below); the
    // operator's own pick stops that.
    const schemeSel = div.querySelector('.loc-scheme');
    schemeSel.addEventListener('change', () => { delete schemeSel.dataset.follow; });
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
  $('#f-scheme').addEventListener('change', () => {
    $$('#f-locs .loc-scheme[data-follow]').forEach((s) => { s.value = $('#f-scheme').value; });
  });

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

  // rail: the request chain follows the form as it is edited
  function curMw() { return $$('#f-mw input:checked').map((i) => i.value); }
  function curAl() { return $$('#f-al input:checked').map((i) => i.value); }
  function refreshFlow() {
    const uphost = $('#f-uphost').value.trim();
    const upport = $('#f-upport').value.trim();
    refreshAttached();
    refreshSums();
    renderRailChain($('#railChain'), {
      upstreamStr: curGroup() ? `group:${curGroup()}` : ((uphost || upport) ? `${uphost || '?'}:${upport || '?'}` : 'upstream not set'),
      mwSelected: curMw(), mwType, alSelected: curAl(),
      inlineAuth: isOn('f-auth-on') ? (($('#f-auth-idp') && $('#f-auth-idp').value) || 'credentials') : '',
      inlineRateLimit: isOn('f-rl-on'),
    });
  }
  // rail: the certificate the handshake will present for the first domain,
  // resolved by SNI matching, not a stored certificateRef the L7 data plane
  // never reads. Same chip as the hosts list's Certificate column, plus how
  // many of the host's domains a certificate covers.
  function refreshRailCert(domains) {
    const el = $('#railCert');
    if (!el) return;
    const list = arr(domains).filter(Boolean);
    const d = list[0];
    if (!d) { el.innerHTML = '<span class="muted">Add a domain to see which certificate covers it.</span>'; return; }
    const ct = certForDomain(certs, d);
    const uncovered = list.filter((x) => !certForDomain(certs, x));
    const parts = [];
    parts.push(ct ? `<a class="rail-cert" href="#/certs/${encodeURIComponent(ct.name)}">${certCoverHtml(ct, d)}</a>` : certCoverHtml(null, d));
    if (ct) {
      const meta = [ct.type === 'acme' ? 'ACME' : (ct.type === 'custom' ? 'Custom' : (ct.type || '')), ct.notAfter ? `expires ${fmtDate(ct.notAfter)}` : ''].filter(Boolean).join(' - ');
      if (meta) parts.push(`<span class="muted">${esc(meta)}</span>`);
    }
    if (list.length > 1) {
      parts.push(`<span class="muted">${list.length - uncovered.length} of ${list.length} domains covered</span>`);
      if (uncovered.length && uncovered.length < list.length) parts.push(`<span class="muted">not covered: <span class="mono">${uncovered.map(esc).join(', ')}</span></span>`);
    }
    if (uncovered.length) parts.push('<span class="muted">Issue or import one under <a href="#/certs">Certificates</a>.</span>');
    el.innerHTML = parts.join('');
  }
  refreshRailCert(arr(h.domains));
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
    if (!nm) { showInvalid('Name required', 'Enter an internal name for this host.', $('#f-name')); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { showInvalid('Domain required', 'Add at least one domain.', $('#f-domains')); return; }
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
      if (!$('#f-uphost').value.trim() || isNaN(portVal)) { showInvalid('Upstream incomplete', 'Set the upstream host and port, or select an upstream group.', $('#f-uphost').value.trim() ? $('#f-upport') : $('#f-uphost')); return; }
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
        catch (e) { showInvalid('Invalid error pages JSON', 'Inline templates must be valid JSON (status code or "default" -> HTML).', $('#f-errp-inline')); return; }
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
    if (secHdrErr) { showInvalid('Invalid security header', secHdrErr, $('#f-secheaders .row-bad') || $('#f-secheaders')); return; }
    const secHdrs = secHdrCtl.get();
    if (secHdrs) obj.securityHeaders = secHdrs;

    // stripResponseHeaders (this host's additions to the fleet strip list) is
    // owned by the chip editor above, which replaced the old carry-forward
    // guard. An empty list leaves the key off the body entirely rather than
    // committing an empty array.
    const stripHdrs = stripCtl.get();
    const stripErr = stripHeaderListError(stripHdrs);
    if (stripErr) { showInvalid('Invalid strip header', stripErr, $('#f-striphdrs')); return; }
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
    // The same rule as the host wizard's Certificate step.
    if (isOn('f-hsts') && !isOn('f-forcessl')) {
      showInvalid('HSTS needs Force SSL', 'HSTS needs Force SSL on, or browsers are told to use HTTPS on a host that still answers plain HTTP.', $('#f-hsts'));
      return;
    }
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
        if (!caRefSel) { showInvalid('Client CA required', 'Select the client CA to verify certificates against, or turn client certificates off.', $('#f-mtls-ca')); return; }
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
      if (!trusted.length) { showInvalid('Trusted proxies empty', 'Add at least one CIDR or IP, or choose "Inherit fleet setting" or "Trust nobody on this host".', $('#f-trustedproxies')); return; }
      const badTp = firstBadCidr(trusted);
      if (badTp) { showInvalid('Invalid trusted proxy', `"${badTp}" is not a CIDR or IP address. Use 10.0.0.0/8 or 192.0.2.10.`, $('#f-trustedproxies')); return; }
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
        const lup = Object.assign({}, origUp, { scheme: ls, host: lh, port: lp });
        const lextra = ctl.up.get('location ' + path);
        if (!lextra) { locAborted = true; break; }
        delete lup.path; delete lup.hostHeader;
        Object.assign(lup, lextra);
        loc.upstream = lup;
      }
      // A picker built from a list that failed to load is empty, not a choice:
      // carry the stored references through, as the host-level save does.
      loc.middlewares = mwListOK ? mwSel : arr((ctl._orig || {}).middlewares);
      loc.accessLists = alListOK ? alSel : arr((ctl._orig || {}).accessLists);
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
    if (locErr) { showInvalid('Location incomplete', locErr, $('#f-locs .row-bad')); return; }
    if (locs.length) obj.locations = locs;

    const btn = $('#saveBtn'); btn.disabled = true;
    try {
      const r = await api('/api/proxy-hosts/' + encodeURIComponent(nm), { method: 'PUT', body: obj, headers: isNew ? { 'If-None-Match': '*' } : {} });
      toastSaved(r.commit, isNew ? 'Added' : 'Saved');
      refreshHeadSha(); clearDirty();
      // One landing rule for every editor: a create opens the new object's own
      // page (as the wizard does), an edit returns to the list.
      if (isNew) location.hash = '#/hosts/' + encodeURIComponent(nm);
      else location.hash = '#/hosts';
    } catch (e) {
      if (!(isNew && showNameTaken(e, 'proxy host', nm, '#/hosts/' + encodeURIComponent(nm), 'Could not add this proxy host', $('#f-name')))) showSaveError(e, 'Could not save this proxy host');
      btn.disabled = false;
    }
  });

  // delete
  const del = $('#deleteBtn');
  if (del) {
    del.addEventListener('click', () => deleteObject('hosts', 'proxy host', h.name, '/api/proxy-hosts/' + encodeURIComponent(h.name), del));
  }
  wireCloneButton('hosts', h, 'hostCloneBtn');
  const yamlBtn = $('#hostYamlBtn');
  if (yamlBtn) yamlBtn.addEventListener('click', () => showHostYaml(h));
  // The band's buttons sit outside #content, so the read-only gate that runs
  // over the page does not reach them.
  if (isReadOnly()) {
    ['#hostCloneBtn', '#deleteBtn'].forEach((sel) => { const b = $(sel); if (b) gateControl(b, false, readOnlyReason()); });
  }
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
  const addBtn = `<a class="btn primary" href="#/certs/_new">${ICON.plus}Add certificate</a>`;
  const head = viewHead('Certificates',
    'The certificates gpm presents at the edge. The handshake picks one by domain; a host never names one.');
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
      blob: [ct.name, ct.displayName, domains.join(' '), type, challenge, provider, detail, ct.issuer, ct.state, ct.disabled ? 'disabled' : ''].join(' ').toLowerCase(),
      f: {
        name: fvals(ct.name, ct.displayName), domain: fvals(domains), type: fvals(type), challenge: fvals(challenge),
        provider: fvals(provider), issuer: fvals(ct.issuer), state: fvals(ct.state), status: fvals(ct.disabled ? 'disabled' : 'live'),
      },
    };
  });

  const SORTS = {
    name: (r) => r.name.toLowerCase(),
    domains: (r) => (r.domains[0] || '').toLowerCase(),
    type: (r) => (r.type + ' ' + r.challenge + ' ' + r.provider).toLowerCase(),
    issuer: (r) => r.issuer.toLowerCase(),
    // Blank expiry sorts last in either direction rather than pretending to be
    // the earliest date, which is what an empty string would do. The '9999'
    // only orders ascending; render() puts blanks last before applying dir.
    expiry: (r) => r.expiry || '9999',
  };
  const sort = loadSort(CERTS_SORT_KEY, { key: 'name', dir: 1 }, (k) => !!SORTS[k]);
  const FILTER_FIELDS = {
    name: 'object or display name', domain: 'a domain or SAN', type: 'acme or custom', challenge: 'dns-01 or http-01',
    provider: 'DNS provider', issuer: 'issuing CA', state: 'certificate state', status: 'live or disabled',
  };
  let terms = [];
  const COLUMNS = [
    { key: 'name', label: 'Name', cls: 'col-name' },
    { key: 'domains', label: 'Domains', cls: 'col-domains' },
    // Type and issuance are one fact ("acme, by dns-01 via cloudflare"), so
    // they share a two-line cell instead of two columns.
    { key: 'type', label: 'Type', cls: 'col-type' },
    { key: 'issuer', label: 'Issuer', cls: 'col-issuer' },
    { key: 'expiry', label: 'Expiry', cls: 'col-time' },
    { key: null, label: '', cls: 'col-actions' },
  ];

  c.innerHTML = head + `
    <div class="toolbar">
      ${filterBoxHtml('certFilter', 'filter certificates...', 'Filter certificates', FILTER_FIELDS)}
      <div class="toolbar-actions">${addBtn}</div>
    </div>
    <div class="table-wrap">
      <table class="certs-table">
        <thead id="certHead"></thead>
        <tbody id="certRows"></tbody>
      </table>
    </div>`;

  function headHtml() {
    return '<tr>' + COLUMNS.map((col) => sortThHtml(col, sort)).join('') + '</tr>';
  }

  function rowHtml(r) {
    const issuance = r.type === 'acme'
      ? `${esc(r.challenge)}${r.provider ? ' via ' + esc(r.provider) : ''}`
      : 'PEM files on the server';
    return `<tr class="clickable" data-name="${esc(r.ct.name)}">
      <td class="col-name">
        <div class="cell-main"><a class="host trunc" href="#/certs/${encodeURIComponent(r.ct.name)}" title="${esc(r.name)}">${esc(r.name)}</a>${(r.ct.displayName && r.ct.displayName !== r.name) ? `<span class="trunc faint" title="${esc(r.ct.displayName)}">${esc(r.ct.displayName)}</span>` : ''}${r.ct.disabled ? DISABLED_CHIP : ''}</div>
      </td>
      <td class="mono col-domains">${domainListHtml(r.domains)}</td>
      <td class="col-type">
        <div class="cell-main"><span class="chip ${r.type === 'acme' ? 'cyan' : ''}">${esc(r.type === 'acme' ? 'ACME' : (r.type === 'custom' ? 'Custom' : r.type))}</span><span class="mono faint">${issuance}</span></div>
      </td>
      <td class="mono faint col-issuer"><span class="trunc" title="${esc(r.issuer || '')}">${esc(r.issuer || '-')}</span></td>
      <td class="col-time">${certExpiryCellHtml(r.ct)}</td>
      <td class="col-actions"><div class="row-actions">
        ${r.type === 'acme' ? `<button class="btn ghost sm ct-renew" data-name="${esc(r.ct.name)}" type="button">Renew now</button>` : ''}
      </div></td>
    </tr>`;
  }

  function render() {
    const get = SORTS[sort.key] || SORTS.name;
    const rows = recs.filter((r) => filterHit(r, terms))
      .sort((a, b) => {
        if (sort.key === 'expiry' && !a.expiry !== !b.expiry) return a.expiry ? -1 : 1;
        const av = get(a), bv = get(b);
        if (av === bv) return a.name.localeCompare(b.name);
        return (av < bv ? -1 : 1) * sort.dir;
      });
    $('#certHead').innerHTML = headHtml();
    $('#certRows').innerHTML = rows.length
      ? rows.map(rowHtml).join('')
      : `<tr><td colspan="6" class="list-empty">No certificate matches this filter.</td></tr>`;
    highlightTerms($('#certRows'), terms);
    wireSortHead($('#certHead'), sort, CERTS_SORT_KEY, render);
    $$('#certRows tr.clickable').forEach((tr) => {
      tr.addEventListener('click', (e) => {
        if (e.target.closest('.ct-renew') || e.target.closest('a')) return;
        location.hash = '#/certs/' + encodeURIComponent(tr.dataset.name);
      });
    });
    $$('#certRows .ct-renew').forEach((b) => {
      b.addEventListener('click', async (e) => {
        e.stopPropagation();
        b.disabled = true;
        try {
          if (await renewCertificate(b.dataset.name) && c.isConnected) await route();
        } finally { b.disabled = false; }
      });
    });
    if (isReadOnly()) {
      $$('#certRows .ct-renew').forEach((b) => gateControl(b, false, readOnlyReason()));
    }
  }
  wireFilter('certFilter', FILTER_FIELDS, () => fieldValues(recs), (t) => { terms = t; render(); }, 'certificates');
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
      ${row('SANs', sans.length ? `<span class="mono">${domainListHtml(sans, sans.length)}</span>` : '<span class="mono">-</span>')}
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
    ${pageHead(esc(isNew ? 'New certificate' : ct.name), isNew ? 'Add a certificate.' : 'Edit this certificate.',
      `<a href="#/certs">Certificates</a>${ICON.chevron}<span>${isNew ? 'new' : esc(ct.name)}</span>`, isNew ? '' : ct.name, '',
      editorActions(isNew, (!isNew && type === 'acme') ? '<button class="btn ghost sm" id="ed-renew" type="button">Renew now</button>' : '', 'ct-clone', 'ct-delete'))}

    <div class="form-grid">
      <div class="stack">
      ${(!isNew && ct.state) ? certStatusCardHtml(ct) : ''}
      ${identityCard('ct', ct, { section: 'certs', isNew, placeholder: seed ? seed.origName + '-copy' : 'wild' })}
      <div class="card form-section">
        <p class="section-label">Certificate</p>
        <div class="field-group">
          <label>Type</label>
          <select class="field" id="ct-type" data-hint="certificate.type" data-path="type">
            ${enumOptions('certType', ['acme', 'custom'], type)}
          </select>
        </div>
        <div class="field-group">
          <label>Domains</label>
          <div class="chip-input" id="ct-domains" data-hint="certificate.domains" data-path="domains"></div>
          <div class="hint">Press Enter after each domain.</div>
        </div>
      </div>
      </div>

      <div class="card form-section">
        <div id="acme-fields" style="${type === 'custom' ? 'display:none' : ''}">
          <p class="section-label">ACME</p>
          <div class="field-group"><label>Account email</label><input class="field mono" id="ct-email" data-hint="certificate.acme.email" data-path="acme.email" value="${esc(acme.email || '')}" placeholder="you@example.com" /></div>
          <div class="field-group"><label>Directory URL</label><input class="field mono" id="ct-dir" data-hint="certificate.acme.directoryURL" data-path="acme.directoryURL" value="${esc(acme.directoryURL || '')}" placeholder="https://acme-v02.api.letsencrypt.org/directory" /></div>
          <div class="inline-fields">
            <div class="field-group"><label>Key type</label><input class="field mono" id="ct-keytype" data-hint="certificate.acme.keyType" data-path="acme.keyType" value="${esc(acme.keyType || '')}" placeholder="ecdsa" /></div>
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
      <div class="save-note">${ICON.commit}Each save is recorded as one git commit.</div>
      <div style="display:flex;gap:10px">
        <button class="btn primary" id="ct-save" type="button">${isNew ? 'Add certificate' : 'Save changes'}</button>
        <a class="btn ghost" href="#/certs">Cancel</a>
      </div>
    </div>`;

  const domainsCtl = makeChipInput($('#ct-domains'), arr(ct.domains), 'add domain...');
  const tagsCtl = wireIdentityCard('ct', ct);
  pathsFromHints($('#content'));
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
    if (!nm) { showInvalid('Name required', 'Enter a certificate name.', $('#ct-name')); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { showInvalid('Domain required', 'Add at least one domain.', $('#ct-domains')); return; }
    const t = $('#ct-type').value;
    // Whole-object PUT: carry the meta this editor renders no control for
    // (labels), or a save deletes it. The name and status card owns the rest.
    const obj = Object.assign({}, metaCarryForward(ct), { name: nm, type: t, domains });
    const display = $('#ct-display').value.trim(); if (display) obj.displayName = display;
    const tags = tagsCtl.get(); if (tags.length) obj.tags = tags; else delete obj.tags;
    if (isOn('ct-disabled')) obj.disabled = true;
    if (t === 'acme') {
      const ch = $('#ct-challenge').value;
      const a = { email: $('#ct-email').value.trim(), challenge: ch };
      const dir = $('#ct-dir').value.trim(); if (dir) a.directoryURL = dir;
      const kt = $('#ct-keytype').value.trim(); if (kt) a.keyType = kt;
      if (ch === 'dns-01') {
        a.dnsProvider = $('#ct-dns').value;
        if (!a.dnsProvider) { showInvalid('DNS provider required', 'Select a DNS provider for dns-01.', $('#ct-dns')); return; }
      } else if (domains.some((d) => d.startsWith('*.'))) {
        showInvalid('Wildcard needs dns-01', 'A wildcard domain can only be validated over dns-01.', $('#ct-challenge')); return;
      }
      if (isOn('ct-eab')) {
        const kid = $('#ct-eab-kid').value.trim();
        const hmac = $('#ct-eab-hmac').value.trim();
        if (!kid || !hmac) { showInvalid('EAB incomplete', 'Enter both the EAB key ID and HMAC key.', kid ? $('#ct-eab-hmac') : $('#ct-eab-kid')); return; }
        if (hmac === '***') { showInvalid('Secret masked', 'The EAB HMAC key reads *** - replace it with a real value or a ${ENV:...} placeholder.', $('#ct-eab-hmac')); return; }
        a.eab = { kid, hmacKey: hmac };
      }
      obj.acme = a;
    } else {
      obj.custom = { certFile: $('#ct-certfile').value.trim(), keyFile: $('#ct-keyfile').value.trim() };
    }
    const btn = $('#ct-save'); btn.disabled = true;
    try {
      const r = await api('/api/certificates/' + encodeURIComponent(nm), { method: 'PUT', body: obj, headers: isNew ? { 'If-None-Match': '*' } : {} });
      toastSaved(r.commit, isNew ? 'Added' : 'Saved'); refreshHeadSha(); clearDirty();
      location.hash = isNew ? '#/certs/' + encodeURIComponent(nm) : '#/certs';
    } catch (e) {
      if (!(isNew && showNameTaken(e, 'certificate', nm, '#/certs/' + encodeURIComponent(nm), 'Could not add this certificate', $('#ct-name')))) showSaveError(e, 'Could not save this certificate');
      btn.disabled = false;
    }
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
  if (del) del.addEventListener('click', () => deleteObject('certs', 'certificate', ct.name, '/api/certificates/' + encodeURIComponent(ct.name), del));
  wireCloneButton('certs', ct, 'ct-clone');
  gateBandActions(['ed-renew', 'ct-clone', 'ct-delete']);
}

// ---------- GENERIC SECTIONS (list + JSON-editor form) ----------
const SECTION_META = {
  clientcas: {
    title: 'Client CAs', sub: 'The authority a client certificate is checked against, for hosts that ask devices for one at the handshake.',
    empty: 'Generate a CA here and gpm can issue the client certificates too; paste an existing certificate if your organisation already runs a CA.',
    singular: 'client CA', addLabel: 'Add client CA',
    summary: (o) => `<span class="k">Revocation</span><span class="v">${o.crlFile ? 'file: ' + esc(o.crlFile) : (o.crlPEM ? 'inline CRL' : 'none')}</span>` +
      ((o.crlFile || o.crlPEM) ? `<span class="k">Policy</span><span class="v">${enumChip('crlPolicy', o.crlPolicy === 'fail-open' ? 'fail-open' : '')}</span>` : ''),
  },
  identity: {
    title: 'Identity Providers', sub: 'Where people prove who they are: an OIDC provider, a forward-auth proxy or an auth-request outpost.',
    empty: 'Add the provider first. A sign-in middleware then references it, and every host that picks up that middleware is protected by it.',
    singular: 'identity provider', addLabel: 'Add identity provider',
    summary: (o) => `<span class="k">Type</span><span class="v">${enumChip('idpType', o.type || '')}</span>` +
      // The client ID stays in the editor: it is a credential half, not a
      // list detail.
      (o.oidc ? `<span class="k">Issuer</span><span class="v">${esc(o.oidc.issuerURL || '')}</span>` : '') +
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
      (arr(o.basicAuth).length ? `<span class="k">Legacy</span><span class="v v-list"><span class="chip warn">Deprecated</span> ${plural(arr(o.basicAuth).length, 'basic-auth user', 'basic-auth users')}</span>` : ''),
  },
  middleware: {
    title: 'Middleware', sub: 'Reusable steps any host can pick up: sign-in, headers, rate limits, guards, path rewrites and deny hooks.',
    empty: 'Most installs start with one sign-in middleware pointing at an identity provider, then attach it to the hosts that need signing in.',
    singular: 'middleware', many: 'middleware', addLabel: 'Add middleware',
    summary: (o) => `<span class="k">Type</span><span class="v">${enumChip('middlewareType', o.type || '')}${(o.type === 'auth' && o.auth && o.auth.mode) ? ' - ' + esc(enumLabel('authMode', o.auth.mode).split(' - ')[0]) : ''}</span>` +
      (o.auth && o.auth.identityProvider ? `<span class="k">Identity provider</span><span class="v">${esc(o.auth.identityProvider)}</span>` : '') +
      (o.auth && o.auth.basic ? `<span class="k">Credentials</span><span class="v">${plural(arr(o.auth.basic.users).length, 'user', 'users')}</span>` : '') +
      (o.rateLimit ? `<span class="k">Rate</span><span class="v">${o.rateLimit.window ? esc(o.rateLimit.requests) + ' / ' + esc(o.rateLimit.window) : esc(o.rateLimit.requestsPerSecond) + ' r/s'}</span>` : '') +
      (o.rateLimit && o.rateLimit.blockFor ? `<span class="k">Block</span><span class="v">${esc(o.rateLimit.blockFor)}</span>` : '') +
      (o.rewrite && o.rewrite.replacePath && Object.keys(o.rewrite.replacePath).length ? `<span class="k">Rewrite</span><span class="v">${plural(Object.keys(o.rewrite.replacePath).length, 'path', 'paths')}</span>` : '') +
      (o.bouncer ? `<span class="k">Bouncer</span><span class="v">${esc(o.bouncer.provider || 'crowdsec')}${o.bouncer.stream ? ' (stream)' : ''}</span><span class="k">On error</span><span class="v">${esc(o.bouncer.onError || 'fail-open')}</span>` : ''),
  },
  dns: {
    title: 'DNS Providers', sub: 'API credentials for the DNS zones gpm writes into: ACME dns-01 challenges and DNS sync records.',
    empty: 'Add the provider that hosts your zone and give it a scoped API token, as a ${ENV:...} placeholder so no secret is committed.',
    singular: 'DNS provider', addLabel: 'Add DNS provider',
    summary: (o) => `<span class="k">Provider</span><span class="v">${esc(o.provider || '')}</span>` +
      (o.config ? `<span class="k">Config keys</span><span class="v v-list">${Object.keys(o.config).map((k) => `<span class="dl-item">${esc(k)}</span>`).join(', ')}</span>` : ''),
  },
  redirects: {
    title: 'Redirects', sub: 'A domain that answers with a redirect: retired names, vanity hostnames, apex to www.',
    empty: 'Add the old domain, the target it should send visitors to, and whether the redirect is permanent.',
    singular: 'redirect', addLabel: 'Add redirect',
    domainFirst: true,
    summary: (o) => alsoDomainsKv(o) +
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
    title: 'Parked Hosts', sub: 'A domain that answers but serves nothing: a reserved name, or a clean 404 instead of another host.',
    empty: 'Add the domains to reserve. They answer 404 by default; change the status code if you want something else.',
    singular: 'parked host', addLabel: 'Add parked host',
    domainFirst: true,
    summary: (o) => alsoDomainsKv(o) +
      `<span class="k">Answers</span><span class="v">${esc(o.statusCode || 404)}</span>`,
  },
  upstreams: {
    title: 'Upstream Groups', sub: 'Several upstreams behind one name, with health checks and a load-distribution policy.',
    empty: 'Add two or more upstreams, then pick how requests are spread across them. A health check keeps a dead one out of the rotation.',
    singular: 'upstream group', addLabel: 'Add upstream group',
    // The members are a pool, not a chain: comma-joined. Health is the live
    // probe state from /api/health, as on the Overview.
    health: true,
    summary: (o, ctx) => `<span class="k">Upstreams</span><span class="v v-list">${arr(o.upstreams).map((u) => `<span class="dl-item">${esc((u.host || '') + ':' + (u.port != null ? u.port : '') + (u.weight ? ' w' + u.weight : ''))}</span>`).join(', ') || '0'}</span>` +
      upstreamHealthKv(o, ctx) +
      `<span class="k">Policy</span><span class="v">${enumChip('loadBalance', o.policy || '')}</span>` +
      (o.stickiness && o.stickiness.ttl ? `<span class="k">Sticky</span><span class="v">${esc(o.stickiness.ttl)}</span>` : '') +
      (o.healthCheck && o.healthCheck.path ? `<span class="k">Probe</span><span class="v">GET ${esc(o.healthCheck.path)}</span>` : `<span class="k">Probe</span><span class="v">TCP</span>`),
  },
};

// A domain-first row shows the first domain as its title; the rest are listed
// under "Also".
function alsoDomainsKv(o) {
  const rest = arr(o.domains).slice(1);
  return rest.length ? `<span class="k">Also</span><span class="v">${domainListHtml(rest, rest.length)}</span>` : '';
}
function upstreamHealthKv(o, ctx) {
  const health = ctx && ctx.health;
  if (!health) return '';
  if (o.disabled) return '<span class="k">Health</span><span class="v faint">Not probed while disabled</span>';
  const g = arr(health.upstreamGroups).find((x) => x.name === o.name);
  if (!g) return '<span class="k">Health</span><span class="v faint">Not probed yet</span>';
  const total = (g.healthy || 0) + (g.unhealthy || 0);
  const cls = !g.unhealthy ? 'ok' : (g.healthy ? 'warn' : 'err');
  return `<span class="k">Health</span><span class="v"><span class="chip ${cls}"><span class="dot ${cls}"></span>${g.healthy || 0} of ${total} healthy</span></span>`;
}

async function genericSection(c, section, sub) {
  if (sub === '_new') return EDITORS[section](c, null);
  if (sub) return EDITORS[section](c, sub);
  return genericList(c, section);
}

// Every value a stored object holds, flattened to one lowercase haystack for
// the list filter. Values only, never keys: matching on keys would make every
// object match "name", "type" and "domains".
// Booleans are not indexed (typing "true" matched every object with any switch
// on); a disabled object is found by the word "disabled" instead.
function objSearchBlob(o) {
  const out = [];
  (function walk(v) {
    if (v == null || typeof v === 'boolean') return;
    if (Array.isArray(v)) { v.forEach(walk); return; }
    if (typeof v === 'object') { Object.keys(v).forEach((k) => walk(v[k])); return; }
    out.push(String(v));
  })(o);
  if (o && o.disabled) out.push('disabled');
  return out.join(' ').toLowerCase();
}
// A section title in running text: words in Title Case drop to lower case,
// acronyms stay ("DNS Providers" -> "DNS providers", "Client CAs" -> "client CAs").
function lowerTitle(t) {
  return String(t).replace(/\b([A-Z])([a-z]+)\b/g, (m, a, b) => a.toLowerCase() + b);
}

async function genericList(c, section) {
  const meta = SECTION_META[section];
  const plural = PLURAL[section];
  const [itemsR, healthR] = await Promise.all([
    api('/api/' + plural),
    meta.health ? api('/api/health').catch(() => null) : Promise.resolve(null),
  ]);
  const items = arr(itemsR.data);
  const ctx = { health: healthR && healthR.data };
  const head = viewHead(meta.title, meta.sub);
  if (!items.length) {
    c.innerHTML = head + emptyState(`No ${meta.many || meta.singular + 's'} yet`,
      meta.empty || `Add your first ${meta.singular}.`, meta.addLabel, `#/${section}/_new`);
    return;
  }
  const recs = {};
  // Redirects and parked hosts read like the Proxy Hosts list: the domain is
  // the title, the object name the subline, then status, tags and last change.
  const LIVE_CHIP = '<span class="chip flat ok"><span class="dot ok"></span>Live</span>';
  // A summary is a run of k/v pairs. The keys, in order of first appearance
  // down the list, are the table's columns, so no section keeps a column list
  // of its own, and a row shows "-" under a key it has nothing for.
  const parsed = items.map((o) => {
    const t = document.createElement('template');
    t.innerHTML = meta.summary(o, ctx);
    const cells = {};
    let k = '';
    Array.from(t.content.children).forEach((el) => {
      if (el.classList.contains('k')) k = el.textContent.trim();
      else if (el.classList.contains('v') && k) { cells[k] = el.outerHTML; k = ''; }
    });
    return cells;
  });
  const keys = [];
  parsed.forEach((cells) => Object.keys(cells).forEach((k) => { if (!keys.includes(k)) keys.push(k); }));
  const anyTags = items.some((o) => arr(o.tags).length);
  const anyUpdated = items.some((o) => o.updatedAt);
  // The filter fields are the columns: name (and domain for a domain-first
  // list), every summary key, tag and status. A summary cell's text is the
  // value the field matches on.
  const fieldKey = (k) => k.toLowerCase().replace(/[^a-z0-9]+/g, '-');
  const cellText = (html) => { const t = document.createElement('template'); t.innerHTML = html || ''; return t.content.textContent.trim(); };
  const FILTER_FIELDS = Object.assign(
    { name: 'object or display name' },
    meta.domainFirst ? { domain: 'a domain' } : {},
    keys.reduce((m, k) => { m[fieldKey(k)] = `the ${k} column`; return m; }, {}),
    { tag: 'a tag', status: 'live or disabled' },
  );
  items.forEach((o, i) => {
    const f = { name: fvals(o.name, o.displayName), tag: fvals(o.tags), status: fvals(o.disabled ? 'disabled' : 'live') };
    if (meta.domainFirst) f.domain = fvals(o.domains);
    keys.forEach((k) => { f[fieldKey(k)] = fvals(cellText(parsed[i][k])); });
    recs[o.name] = { blob: objSearchBlob(o), f };
  });
  const headHtml = '<tr>'
    + `<th class="col-name" data-sort="name">${meta.domainFirst ? 'Domain' : 'Name'}</th>`
    + keys.map((k) => `<th data-sort="${esc(fieldKey(k))}">${esc(k)}</th>`).join('')
    + (anyTags ? '<th class="col-tags" data-sort="tags">Tags</th>' : '')
    + (anyUpdated ? '<th class="col-updated" data-sort="updated">Updated</th>' : '')
    + '</tr>';
  const rowHtml = (o, i) => {
    const href = `#/${section}/${encodeURIComponent(o.name)}`;
    let main, sub;
    // One line per row: the name, then (muted, inline) a display name or
    // object name only when it differs from it, then the status chip. A
    // second line on some rows and not others made the list look uneven.
    if (meta.domainFirst) {
      const primary = arr(o.domains)[0] || o.name;
      const subline = [o.name !== primary ? o.name : '', (o.displayName && o.displayName !== primary && o.displayName !== o.name) ? o.displayName : ''].filter(Boolean).join(' - ');
      main = `<a class="host trunc" href="${href}" title="${esc(arr(o.domains).join(', ') || o.name)}">${esc(primary)}</a>`;
      sub = (subline ? `<span class="trunc faint mono" title="${esc(subline)}">${esc(subline)}</span>` : '') + (o.disabled ? DISABLED_CHIP : LIVE_CHIP);
    } else {
      main = `<a class="host trunc" href="${href}" title="${esc(o.name)}">${esc(o.name)}</a>`;
      sub = ((o.displayName && o.displayName !== o.name) ? `<span class="trunc faint" title="${esc(o.displayName)}">${esc(o.displayName)}</span>` : '')
        + (o.disabled ? DISABLED_CHIP : '');
    }
    return `<tr class="clickable" data-name="${esc(o.name)}">
      <td class="col-name"><div class="cell-main">${main}${sub}</div></td>
      ${keys.map((k) => `<td class="col-v">${parsed[i][k] || '<span class="faint">-</span>'}</td>`).join('')}
      ${anyTags ? `<td class="col-tags" title="${esc(arr(o.tags).join(', '))}">${arr(o.tags).map((t) => `<span class="chip tag">${esc(t)}</span>`).join(' ')}</td>` : ''}
      ${anyUpdated ? `<td class="col-updated mono" data-v="${esc(o.updatedAt || '')}">${o.updatedAt ? esc(fmtTime(o.updatedAt)) : ''}</td>` : ''}
    </tr>`;
  };
  const addBtn = `<a class="btn primary" href="#/${section}/_new">${ICON.plus}${meta.addLabel}</a>`;
  c.innerHTML = head + `
    <div class="toolbar">
      ${filterBoxHtml('gsFilter', `filter ${lowerTitle(meta.title)}...`, `Filter ${meta.title}`, FILTER_FIELDS)}
      <div class="toolbar-actions">${addBtn}</div>
    </div>
    <div class="table-wrap">
      <table class="gs-table">
        <thead>${headHtml}</thead>
        <tbody id="gsRows">${items.map(rowHtml).join('')}</tbody>
      </table>
    </div>
    <div class="list-empty" id="gsNone" hidden>No ${esc(meta.singular)} matches this filter.</div>`;

  wireTableSort($('.gs-table', c), `gpm.${section}.sort`, { key: 'name', dir: 1 });
  // The filter matches the object's stored values, not the summary markup the
  // row happens to render.
  wireFilter('gsFilter', FILTER_FIELDS, () => fieldValues(Object.values(recs)), (terms) => {
    applyRowFilter($$('#gsRows tr[data-name]'), recs, terms, $('#gsNone'));
    highlightTerms($('#gsRows'), terms);
  }, section);

  $$('#gsRows tr[data-name]').forEach((el) => {
    const open = () => { location.hash = `#/${section}/` + encodeURIComponent(el.dataset.name); };
    // The name is the row's link for the keyboard and screen readers; the rest
    // of the row stays clickable for the mouse, except a "+N" toggle. Delete
    // is not offered here: it lives in the object's own save bar, as on proxy
    // hosts and certificates.
    el.addEventListener('click', (e) => { if (!e.target.closest('a') && !e.target.closest('button')) open(); });
  });
}

// ---------- delete ----------
// Delete lives only on an object's own page (the save bar, or the proxy host's
// title band), always behind confirmModal: the title names the object, one line
// says what the delete does, the objects that still reference it are listed
// (the API refuses the delete while any do), and the History line says it can
// be undone.
const DELETE_CONSEQUENCE = {
  hosts: 'Its domains stop being served and are free for another host to claim.',
  certs: 'Its domains are served by another matching certificate, or by none.',
  redirects: 'Its domains stop redirecting and are free for another host to claim.',
  parked: 'Its domains stop answering and are free for another host to claim.',
  streams: 'Its listen port closes and its target is no longer reachable through gpm.',
  access: 'An access list that a host still attaches cannot be deleted: detach it there first.',
  middleware: 'A middleware that a host still attaches cannot be deleted: detach it there first.',
  identity: 'An identity provider that a host or middleware still signs in through cannot be deleted: point those elsewhere first.',
  upstreams: 'An upstream group that a host still routes to cannot be deleted: give those hosts another upstream first.',
  clientcas: 'Client certificates it issued are no longer accepted, and a host that still verifies against it blocks the delete.',
  dns: 'A certificate that still solves dns-01 through it blocks the delete: switch that certificate first.',
};
const DELETE_UNDO = 'Recorded as a commit; History can undo it.';
// The names that reference section/name, for the delete dialog. Proxy hosts
// (and their locations) by middlewares, accessLists, upstreamGroupRef, the
// sign-in's identity provider and the mTLS client CA; a certificate by the
// hosts whose domains it serves. Other kinds are named with their kind. A
// failed fetch lists nothing: the API still refuses a referenced delete.
async function usedBy(section, name) {
  const get = (p) => api(p).then((r) => arr(r.data)).catch(() => []);
  const inScope = (h, test) => [h].concat(arr(h.locations)).some(test);
  const tests = {
    middleware: (s) => arr(s.middlewares).indexOf(name) !== -1,
    access: (s) => arr(s.accessLists).indexOf(name) !== -1,
    upstreams: (s) => s.upstreamGroupRef === name,
    identity: (s) => !!(s.auth && s.auth.identityProvider === name),
    clientcas: (s) => !!(s.tls && s.tls.clientAuth && s.tls.clientAuth.caRef === name),
  };
  if (section === 'dns') {
    return (await get('/api/certificates')).filter((ct) => ct.acme && ct.acme.dnsProvider === name).map((ct) => ct.name + ' (certificate)');
  }
  if (section === 'certs') {
    const [hosts, certs, streams] = await Promise.all([get('/api/proxy-hosts'), get('/api/certificates'), get('/api/stream-hosts')]);
    return hosts.filter((h) => arr(h.domains).some((d) => (certForDomain(certs, d) || {}).name === name)).map((h) => h.name)
      .concat(streams.filter((s) => s.tls && s.tls.certificateRef === name).map((s) => s.name + ' (stream)'));
  }
  const test = tests[section];
  if (!test) return [];
  const [hosts, extra] = await Promise.all([
    get('/api/proxy-hosts'),
    section === 'identity' ? get('/api/middlewares') : (section === 'access' ? get('/api/stream-hosts') : Promise.resolve([])),
  ]);
  const out = hosts.filter((h) => inScope(h, test)).map((h) => h.name);
  if (section === 'identity') extra.filter(test).forEach((m) => out.push(m.name + ' (middleware)'));
  if (section === 'access') extra.filter(test).forEach((s) => out.push(s.name + ' (stream)'));
  return out;
}
// deleteObject runs one Delete button: the dialog, the DELETE, the "Deleted"
// toast and the way back to the list. A refused delete (still referenced, or
// otherwise invalid) is the sticky editor banner, not a toast that is gone
// before it is read.
async function deleteObject(section, kind, name, path, btn) {
  // Not disabled while the references load: a disabled button drops the focus
  // the dialog hands back when it closes.
  if (btn.dataset.busy) return;
  btn.dataset.busy = '1';
  let refs = [];
  const page = location.hash;
  try { refs = await usedBy(section, name); } finally { delete btn.dataset.busy; }
  // The lookup took time; a dialog must not open over a page the user has
  // since left, let alone be confirmed there.
  if (location.hash !== page) return;
  const ok = await confirmModal({
    title: `Delete ${kind} ${name}?`,
    body: `<p>${esc(DELETE_CONSEQUENCE[section] || '')}</p>`
      + (refs.length ? `<p>Used by: <b>${refs.map(esc).join('</b>, <b>')}</b></p>` : '')
      + `<p>${esc(DELETE_UNDO)}</p>`,
    confirmLabel: `Delete ${kind}`,
  });
  if (!ok) return;
  clearEditorError();
  btn.disabled = true;
  try {
    const r = await api(path, { method: 'DELETE' });
    toastSaved(r.commit, 'Deleted');
    refreshHeadSha(); clearDirty();
    location.hash = '#/' + section;
  } catch (e) {
    if (e && (e.status === 409 || e.status === 400)) showSaveError(e, `Could not delete ${name}`);
    else toastErr(e, `Could not delete ${name}`);
    btn.disabled = false;
  }
}

// ---------- editor scaffolding shared by the typed object editors ----------
// Object actions (Clone, Delete, and whatever the editor adds in front of them)
// sit in the title band on every editor, as they do on the proxy host editor;
// the save bar holds only Save and Cancel. On a new object there is nothing to
// act on, so the band stays empty.
function editorActions(isNew, extraHtml, cloneId, deleteId) {
  if (isNew) return '';
  return `${extraHtml || ''}<button class="btn ghost sm" id="${cloneId || 'ed-clone'}" type="button">Clone</button><button class="btn danger-soft sm" id="${deleteId || 'ed-delete'}" type="button">${ICON.trash}Delete</button>`;
}
// The band's buttons sit outside #content, so the read-only gate that runs over
// the page does not reach them; every editor gates its own.
function gateBandActions(ids) {
  if (!isReadOnly()) return;
  ids.forEach((id) => { const b = $('#' + id); if (b) gateControl(b, false, readOnlyReason()); });
}
function editorHead(section, meta, isNew, name) {
  const crumb = `<a href="#/${section}">${esc(meta.title)}</a>${ICON.chevron}<span>${isNew ? 'new' : esc(name)}</span>`;
  const article = /^[aeiou]/i.test(meta.singular) ? 'an' : 'a';
  return pageHead(esc(isNew ? 'New ' + meta.singular : name), esc(isNew ? `Add ${article} ${meta.singular}.` : `Edit this ${meta.singular}.`), crumb, isNew ? '' : name, '', editorActions(isNew));
}
// Save bars put the primary first, as dialogs and the wizard footer do.
function saveBar(section, isNew, addLabel) {
  return `<div class="panel save-bar">
    <div class="save-note">${ICON.commit}Each save is recorded as one git commit.</div>
    <div style="display:flex;gap:10px">
      <button class="btn primary" id="ed-save" type="button">${esc(isNew ? addLabel : 'Save changes')}</button>
      <a class="btn ghost" href="#/${section}">Cancel</a>
    </div>
  </div>`;
}
// ---------- name and status card ----------
// One card on every editor: Name (immutable after creation), Display name,
// Tags and Disabled. A host kind stops serving when disabled; any other kind
// drops out of the running proxy. p prefixes the ids (f on the proxy host, ct
// on the certificate, ed on the rest). opts.placeholder, when set (a clone
// seed's editor), replaces the generic "internal-name" placeholder.
//
// opts.foldable collapses the card to its one-line summary on the editors that
// have enough other fields to need the room (proxy host, redirect, stream,
// parked). It never folds a NEW object's card - the name is required there and
// immutable afterwards, so hiding the only chance to set it would be a trap -
// and never folds one that holds a display name, tags or a status flag.
const HOST_KINDS = new Set(['hosts', 'redirects', 'streams', 'parked']);
function identityCard(p, obj, opts) {
  obj = obj || {};
  opts = opts || {};
  const body = `
    <div class="inline-fields">
      <div class="field-group"><label>Name</label><input class="field mono" id="${p}-name" data-hint="common.name" data-path="name" value="${esc(obj.name || '')}" ${opts.isNew ? '' : 'disabled'} placeholder="${esc(opts.placeholder || 'internal-name')}" /><div class="hint">${opts.isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div></div>
      <div class="field-group"><label>Display name</label><input class="field" id="${p}-display" data-hint="common.displayName" data-path="displayName" value="${esc(obj.displayName || '')}" placeholder="optional label" /></div>
    </div>
    <div class="field-group"><label>Tags</label><div class="chip-input" id="${p}-tags" data-hint="${opts.section === 'hosts' ? 'proxyHost.tags' : 'common.tags'}" data-path="tags"></div><div class="hint">Free-form tags for grouping and filtering. Press Enter to add.</div></div>
    <div class="toggle-line"><div class="tl-text"><div class="nm">Disabled</div><div class="ds">${HOST_KINDS.has(opts.section) ? 'Stop serving this host' : 'Exclude from the running proxy'}</div></div>${switchHtml(p + '-disabled', !!obj.disabled, 'Disabled', 'common.disabled')}</div>
    ${opts.extra || ''}`;
  if (!opts.foldable) return `<div class="card form-section" id="${p}-id-card"><p class="section-label">Name and status</p>${body}</div>`;
  const sum = identitySummary(obj.displayName, arr(obj.tags).length, obj.disabled, obj.maintenance, obj.name);
  return foldHtml(p + '-id-card', 'Name and status', sum.text, opts.isNew || sum.set, body);
}
function identitySummary(display, tagCount, disabled, maintenance, name) {
  const parts = [];
  if (display) parts.push(display);
  if (tagCount) parts.push(`${plural(tagCount, 'tag', 'tags')}`);
  if (disabled) parts.push('disabled');
  if (maintenance) parts.push('maintenance');
  return { set: parts.length > 0, text: parts.length ? parts.join(' - ') : (name || 'name, tags, status') };
}
// Wires the card's tag chips and keeps a folded card's summary following the
// form. Returns the tags control.
function wireIdentityCard(p, obj) {
  const tagsCtl = makeChipInput($('#' + p + '-tags'), arr(obj && obj.tags), 'add tag...');
  liveFoldSummaries($('#' + p + '-id-card'), {
    [p + '-id-card']: () => identitySummary($('#' + p + '-display').value.trim(), tagsCtl.get().length,
      isOn(p + '-disabled'), isOn(p + '-maintenance'), $('#' + p + '-name').value.trim()).text,
  });
  return tagsCtl;
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
      ? `<div class="cert-auto" title="${esc(d)}: certificate ${esc(ct.name)}, selected automatically by domain"><span class="mono">${esc(d)}</span><span>covered by <b>${esc(ct.name)}</b></span></div>`
      : `<div class="cert-auto warn" title="${esc(d)}"><span class="mono">${esc(d)}</span><span>no certificate covers this domain</span></div>`;
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
  const tagsCtl = wireIdentityCard('ed', stored);
  pathsFromHints($('#content'));
  $('#ed-save').addEventListener('click', async () => {
    clearEditorError();
    const nm = isNew ? $('#ed-name').value.trim() : origName;
    if (!nm) { showInvalid('Name required', 'Enter a name.', $('#ed-name')); return; }
    const body = buildBody(nm);
    if (!body) return;
    Object.assign(body, metaCarryForward(stored));
    body.name = nm;
    const disp = $('#ed-display'); if (disp && disp.value.trim()) body.displayName = disp.value.trim();
    // The card owns tags: the stored ones were carried above, the chips replace them.
    const tags = tagsCtl.get(); if (tags.length) body.tags = tags; else delete body.tags;
    if (isOn('ed-disabled')) body.disabled = true;
    const btn = $('#ed-save'); btn.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'PUT', body, headers: isNew ? { 'If-None-Match': '*' } : {} });
      toastSaved(r.commit, isNew ? 'Added' : 'Saved'); refreshHeadSha(); clearDirty();
      location.hash = '#/' + section + (isNew ? '/' + encodeURIComponent(nm) : '');
    } catch (e) {
      if (!(isNew && showNameTaken(e, meta.singular, nm, '#/' + section + '/' + encodeURIComponent(nm), `Could not add this ${meta.singular}`, $('#ed-name')))) showSaveError(e, `Could not save this ${meta.singular}`);
      btn.disabled = false;
    }
  });
  const del = $('#ed-delete');
  if (del) del.addEventListener('click', () => deleteObject(section, meta.singular, origName, '/api/' + plural + '/' + encodeURIComponent(origName), del));
  gateBandActions(['ed-clone', 'ed-delete']);
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
    ${identityCard('ed', o, { section: 'redirects', isNew, placeholder: seed && seed.origName + '-copy', foldable: true })}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains" data-hint="redirectHost.domains"></div><div class="hint">Press Enter after each domain.</div></div>
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
      <div class="toggle-line"><div class="tl-text"><div class="nm">Preserve path</div><div class="ds">Append the original request path to the target</div></div>${switchHtml('ed-preserve', !!o.preservePath, 'Preserve path', 'redirectHost.preservePath')}</div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, arr(o.domains))}</div></div>` + saveBar('redirects', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...',
    (d) => { $('#ed-cert-auto').innerHTML = certCoverageHtml(certs, d); });
  wireTls();
  wireEditor('redirects', 'redirect-hosts', meta, isNew, name || o.name, o, (nm) => {
    const domains = domainsCtl.get();
    if (!domains.length) { showInvalid('Domain required', 'Add at least one domain.', $('#ed-domains')); return null; }
    const td = $('#ed-tdomain').value.trim();
    if (!td) { showInvalid('Target required', 'Set the target domain.', $('#ed-tdomain')); return null; }
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
// Why an access list cannot gate a raw stream, or null when it can. Mirrors the
// API's refusal: basic-auth users need an HTTP challenge, path- or
// method-scoped rules need a request path, and source-backed rules are refused
// on a stream as well.
function streamAlUnavailable(a) {
  if (arr(a.basicAuth).length) return { chip: 'basic-auth: not at L4', why: 'This list has basic-auth users, which a raw stream cannot evaluate.' };
  const rules = arr(a.rules);
  if (rules.some((r) => arr(r.paths).length || arr(r.methods).length)) return { chip: 'path rules: not at L4', why: 'This list has rules scoped to request paths or methods. A raw stream has no request path to match.' };
  if (rules.some((r) => r.source)) return { chip: 'source rules: not at L4', why: 'This list has rules backed by a remote source, which the API refuses on a raw stream.' };
  return null;
}
async function streamEditor(c, name) {
  const meta = SECTION_META.streams; const isNew = !name;
  const seed = isNew ? takeCloneSeed('streams') : null;
  const [certsR, alR, objR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    refList('/api/access-lists', 'access lists'),
    isNew ? Promise.resolve({ data: {} }) : api('/api/stream-hosts/' + encodeURIComponent(name)),
  ]);
  const streamCerts = arr(certsR);
  // A list the API refuses on a raw stream (streamAlUnavailable) is offered but
  // disabled with the reason rather than accepted-then-rejected by the API.
  const streamAls = arr(alR);
  const o = seed ? seed.data : (objR.data || {});
  const stls = o.tls || {};
  const selAl = arr(o.accessLists);
  // One-line state for the two folded sections (see hostEditor for why).
  const stlsSummary = stls.mode
    ? `${stls.mode}${arr(stls.sniMatch).length ? ', SNI ' + arr(stls.sniMatch).join(', ') : ''}${stls.certificateRef ? ', cert ' + stls.certificateRef : ''}`
    : 'none - the bytes are forwarded blind';
  const streamAlSummary = selAl.length ? selAl.join(', ') : 'none - every client IP is accepted';
  c.innerHTML = editorHead('streams', meta, isNew, name) + `<div class="stack">
    ${identityCard('ed', o, { section: 'streams', isNew, placeholder: seed && seed.origName + '-copy', foldable: true })}
    <div class="card form-section"><p class="section-label">Forwarding</p>
      <div class="inline-fields">
        <div class="field-group"><label>Listen port</label><input class="field mono" id="ed-listen" data-hint="streamHost.listenPort" type="number" value="${esc(o.listenPort != null ? o.listenPort : '')}" placeholder="5353" /></div>
        <div class="field-group"><label>Protocol</label><select class="field mono" id="ed-proto" data-hint="streamHost.protocol">
          ${enumOptions('streamProtocol', ['tcp', 'udp', 'both'], o.protocol || 'tcp')}
        </select></div>
      </div>
      <div class="inline-fields">
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
          <div class="hint">Server names this host claims, e.g. <span class="mono">db.example.com</span> or <span class="mono">*.example.com</span>. Required when sharing a port.</div>
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
      <p class="muted" style="font-size:13.5px;margin:0 0 10px">Evaluated on the client IP before any upstream is dialled. Only literal IP rules and geo apply at L4. A list with basic-auth users, rules scoped to paths or methods, or rules backed by a source needs an HTTP request, so it is unavailable here.</p>
      <div class="check-list" id="ed-stream-al" data-hint="streamHost.accessLists">
        ${streamAls.length ? streamAls.map((a) => {
    const off = streamAlUnavailable(a);
    return `<label class="check-item"${off ? ` style="opacity:.5;cursor:not-allowed" title="${esc(off.why)}"` : ''}><input type="checkbox" value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 && !off ? ' checked' : ''}${off ? ' disabled' : ''}/>${esc(a.name)}<span class="ci-ty">${off ? esc(off.chip) : 'ip / geo'}</span></label>`;
  }).join('') : '<div class="check-empty">No access lists defined yet.</div>'}
      </div>
    `)}
  </div>` + saveBar('streams', isNew, meta.addLabel);
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
  liveFoldSummaries($('#content'), {
    'ed-stls-card': () => {
      const mode = $('#ed-tlsmode').disabled ? '' : $('#ed-tlsmode').value;
      if (!mode) return 'none - the bytes are forwarded blind';
      const sni = sniCtl.get();
      const ref = mode === 'terminate' ? $('#ed-streamcert').value : '';
      return `${mode}${sni.length ? ', SNI ' + sni.join(', ') : ''}${ref ? ', cert ' + ref : ''}`;
    },
    'ed-stream-al-card': () => {
      const als = $$('#ed-stream-al input:checked').map((i) => i.value);
      return als.length ? als.join(', ') : 'none - every client IP is accepted';
    },
  });
  wireEditor('streams', 'stream-hosts', meta, isNew, name || o.name, o, () => {
    const lp = parseInt($('#ed-listen').value, 10); const fp = parseInt($('#ed-fport').value, 10); const fh = $('#ed-fhost').value.trim();
    if (isNaN(lp)) { showInvalid('Listen port required', 'Enter a listen port.', $('#ed-listen')); return null; }
    if (!fh || isNaN(fp)) { showInvalid('Target required', 'Set target host and port.', fh ? $('#ed-fport') : $('#ed-fhost')); return null; }
    const proto = $('#ed-proto').value;
    const body = { listenPort: lp, protocol: proto, target: { host: fh, port: fp } };
    const mode = $('#ed-tlsmode').value;
    if (mode && proto === 'tcp') {
      const tls = { mode };
      const sni = sniCtl.get(); if (sni.length) tls.sniMatch = sni;
      if (mode === 'terminate') {
        const ref = $('#ed-streamcert').value;
        if (!ref) { showInvalid('Certificate required', 'Terminate mode needs a certificate to present.', $('#ed-streamcert')); return null; }
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
    ${identityCard('ed', o, { section: 'parked', isNew, placeholder: seed && seed.origName + '-copy', foldable: true })}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains" data-hint="parkedHost.domains"></div><div class="hint">Press Enter after each domain.</div></div>
    <div class="card form-section"><p class="section-label">Response</p>
      <div class="field-group"><label>Status code</label><input class="field mono" id="ed-status" data-hint="parkedHost.statusCode" type="number" value="${esc(o.statusCode != null ? o.statusCode : 404)}" placeholder="404" /><div class="hint">Default 404.</div></div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, arr(o.domains))}</div></div>` + saveBar('parked', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...',
    (d) => { $('#ed-cert-auto').innerHTML = certCoverageHtml(certs, d); });
  wireTls();
  wireEditor('parked', 'parked-hosts', meta, isNew, name || o.name, o, () => {
    const domains = domainsCtl.get();
    if (!domains.length) { showInvalid('Domain required', 'Add at least one domain.', $('#ed-domains')); return null; }
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
  c.innerHTML = editorHead('dns', meta, isNew, name) + `<div class="stack">
    ${identityCard('ed', o, { section: 'dns', isNew, placeholder: seed && seed.origName + '-copy' })}
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
  </div>` + saveBar('dns', isNew, meta.addLabel);

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
    if (!provId) { showInvalid('Provider required', 'Select a provider.', $('#ed-provider')); return null; }
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
        showInvalid('Secret masked', 'A credential is masked as ***. Replace it with a real value or a ${ENV:...} placeholder before saving.', $$('#ed-typed .dnsf').find((el) => el.value.trim() === '***')); return null;
      }
      if (cfg.ttl != null) {
        const n = Number(cfg.ttl);
        if (!Number.isInteger(n) || n < 1 || n > 86400) { showInvalid('Invalid TTL', 'config.ttl must be a whole number of seconds between 1 and 86400.', $('#ed-typed .dnsf[data-key="ttl"]')); return null; }
      }
      if (cfg.timeout && !GO_DURATION_RE.test(cfg.timeout)) {
        showInvalid('Invalid timeout', 'config.timeout must be a positive Go duration such as 30s.', $('#ed-typed .dnsf[data-key="timeout"]')); return null;
      }
      if (cfg.baseURL && !/^https?:\/\//.test(cfg.baseURL)) {
        showInvalid('Invalid base URL', 'config.baseURL must be an http or https URL.', $('#ed-typed .dnsf[data-key="baseURL"]')); return null;
      }
    } else {
      if (cfgCtl.masked()) { showInvalid('Secret masked', 'A credential is masked as ***. Replace it with a real value or a ${ENV:...} placeholder before saving.', $('#ed-config')); return null; }
      cfg = cfgCtl.get();
    }
    const missing = arr(p.required).filter((k) => !cfg[k]);
    if (missing.length) {
      const missingEl = $(`#ed-typed .dnsf[data-key="${missing[0]}"]`) || $('#ed-config');
      if (p.missing) showInvalid('Missing credentials', p.missing, missingEl);
      else showInvalid('API token required', 'Every DNS provider needs a config.apiToken credential.', missingEl);
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
    <b>Legacy basic auth.</b> This access list has ${plural(legacyUsers.length, 'user', 'users')}
    (<span class="mono">${esc(legacyUsers.map((u) => u.username).join(', '))}</span>). Basic auth in an access list is
    deprecated and will be removed in gpm v2.
    ${o.satisfyAny ? '<br /><b>satisfyAny is set:</b> an allowed IP currently passes without a password.' : ''}
    <div style="margin-top:8px;display:flex;gap:10px;align-items:center">
      <button class="btn sm al-migrate" id="al-migrate" type="button">Migrate</button>
      <a class="hint" href="https://github.com/Rake-Pro/go-proxy-manager/blob/main/docs/how-to/migrate-basic-auth.md" target="_blank" rel="noopener">Learn more</a>
    </div>
  </div>` : '';

  c.innerHTML = editorHead('access', meta, isNew, name) + legacyCard + `<div class="stack">
    ${identityCard('ed', o, { section: 'access', isNew, placeholder: seed && seed.origName + '-copy' })}
    <div class="card form-section"><p class="section-label">Policy</p>
      <div class="field-group"><label>Default action</label><select class="field mono" id="ed-default" data-hint="accessList.defaultAction" data-path="defaultAction">
        ${enumOptions('ruleAction', ['deny', 'allow'], o.defaultAction || 'deny')}
      </select></div>
    </div>
    <div class="card form-section"><p class="section-label">IP rules</p><div id="ed-rules" data-hint="accessList.rules" data-path="rules"></div>
      <button class="btn ghost sm" id="ed-addrule" type="button" style="margin-top:6px">${ICON.plus}Add rule</button>
      <div class="hint" style="margin-top:6px">Evaluated top-down, first match wins. Match a literal CIDR/IP, or a named source declared below. Paths (comma-separated, each must start with "/") narrow a rule to exact request paths; methods (comma-separated, upper-cased) only apply alongside paths and default to GET, HEAD when left blank.</div>
      <div class="hint" style="margin-top:6px">For username/password gating, add a sign-in middleware in Basic mode under <a href="#/middleware/_new">Middleware</a>.</div>
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
        <div class="hint">Two-letter codes, e.g. US.</div>
      </div>
      <div class="field-group"><label>Country deny</label><div class="chip-input" id="ed-geo-deny" data-hint="accessList.geo.countryDeny"></div></div>
      <div class="field-group"><label>On unknown country</label><select class="field mono" id="ed-geo-unknown" data-hint="accessList.geo.onUnknown">
        ${enumOptions('geoUnknown', ['', 'allow', 'deny'], geo.onUnknown || '')}
      </select><div class="hint">Applied to an IP with no country in the database (private/reserved ranges, DB misses).</div></div>
    </div>
  </div>` + saveBar('access', isNew, meta.addLabel);
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
  // if that name is not (or not yet) a declared source - the round-trip
  // guarantee any preset <select> needs, so a
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
        <span class="muted" style="font-size:12.5px">${s.fetchedAt ? esc(fmtTime(s.fetchedAt)) + ', ' + plural(s.entryCount || 0, 'entry', 'entries') : 'Never'}</span>
        ${s.lastError ? `<span class="muted" style="font-size:12.5px;color:var(--warn-text)">${esc(s.lastError)}</span>` : ''}</div>`).join('')}</div>`
        : '<p class="hint" style="margin:0">No sources declared yet for this list.</p>';
    } catch (e) {
      card.style.display = arr(o.sources).length ? '' : 'none';
      el.innerHTML = `<p class="hint" style="margin:0">${e && e.status === 501 ? 'Access-list source sync is not available in this build.' : esc('Status unavailable: ' + (e.message || e))}</p>`;
    }
  }
  if (!isNew) renderAccessSourceStatus();
  else $('#ed-src-status-card').style.display = 'none';

  $('#ed-src-reconcile').addEventListener('click', async () => {
    const btn = $('#ed-src-reconcile'); btn.disabled = true;
    try {
      await api('/api/access-list-sources/reconcile', { method: 'POST' });
      toast('Reconciled', 'Access-list sources are back in sync.', 'ok');
    } catch (e) { toastErr(e, 'Could not refresh the sources'); }
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
    if (ruleErr) { showInvalid('Rule invalid', ruleErr, $('#ed-rules .row-bad')); return null; }
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
    if (srcErr) { showInvalid('Source invalid', srcErr, $('#ed-sources .row-bad')); return null; }
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
  if (!(await confirmDiscardEdits())) return;
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
  const body = `<p>Adds the sign-in middleware <b>${esc(plan.middleware)}</b> with <b>${plural(arr(plan.users).length, 'user', 'users')}</b> (<span class="mono">${esc(arr(plan.users).join(', '))}</span>).</p>`
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
    toast('Migrated', `Migrated to middleware ${esc(mw)}.` + (shortSha(r.commit) ? ` committed <span class="sha">${esc(shortSha(r.commit))}</span>` : ''), 'ok', { html: true });
    refreshHeadSha();
    route();
  } catch (e) {
    migrateError(e);
    btn.disabled = false;
  }
}
function migrateError(e) {
  if (e && e.status === 404) { toast('Not found', 'Access list not found - reload the page.', 'err'); return; }
  toastErr(e, 'Could not migrate basic auth');
}

// ---------- IDENTITY PROVIDER EDITOR (polymorphic) ----------
async function idpEditor(c, name) {
  const meta = SECTION_META.identity; const isNew = !name;
  const seed = isNew ? takeCloneSeed('identity') : null;
  const o = seed ? seed.data : (isNew ? { type: 'oidc' } : ((await api('/api/identity-providers/' + encodeURIComponent(name))).data || {}));
  const type = o.type || 'oidc';
  const oidc = o.oidc || {}; const fa = o.forwardAuth || {}; const ar = o.authRequest || {}; const rm = o.roleMapping || {};
  c.innerHTML = editorHead('identity', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${identityCard('ed', o, { section: 'identity', isNew, placeholder: seed && seed.origName + '-copy' })}
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
      <div class="field-group"><label>Proxies that may assert identity (CIDRs)</label><div class="chip-input" id="fa-trusted" data-hint="identityProvider.forwardAuth.trustedProxies"></div><div class="hint">Which peers may send identity headers such as <span class="mono">Remote-User</span>. Required. The client address is set elsewhere, under <a href="#/settings/general">Settings &gt; Client addresses</a>.</div></div>
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
      if (!issuer || !cid) { showInvalid('OIDC incomplete', 'Issuer URL and client ID are required.', issuer ? $('#oidc-clientid') : $('#oidc-issuer')); return null; }
      const sec = $('#oidc-secret').value.trim();
      if (sec === '***') { showInvalid('Secret masked', 'The client secret is masked as ***. Replace it with a real value or a ${ENV:...} placeholder.', $('#oidc-secret')); return null; }
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
      if (!tp.length) { showInvalid('Trusted proxies required', 'Add at least one trusted proxy CIDR.', $('#fa-trusted')); return null; }
      if (!uh) { showInvalid('User header required', 'Set the user header.', $('#fa-user')); return null; }
      const spec = { trustedProxies: tp, userHeader: uh };
      const fields = { emailHeader: 'fa-email', nameHeader: 'fa-name', groupsHeader: 'fa-groups', groupsDelimiter: 'fa-delim', amrHeader: 'fa-amr' };
      Object.keys(fields).forEach((k) => { const v = $('#' + fields[k]).value.trim(); if (v) spec[k] = v; });
      body.forwardAuth = spec;
    } else {
      const out = $('#ar-outpost').value.trim();
      if (!out) { showInvalid('Outpost URL required', 'Set the outpost URL.', $('#ar-outpost')); return null; }
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
  c.innerHTML = editorHead('middleware', meta, isNew, name) + `<div class="stack">
    ${identityCard('ed', o, { section: 'middleware', isNew, placeholder: seed && seed.origName + '-copy' })}
    <div class="card form-section"><p class="section-label">Type</p>
      <div class="field-group"><label>Middleware type</label><select class="field mono" id="ed-type" data-hint="middleware.type">
        ${enumOptions('middlewareType', ['auth', 'headers', 'guard', 'rate-limit', 'rewrite', 'bouncer'], type)}
      </select></div>
    </div>

    <div class="card form-section ed-sub" data-type="auth" style="${type === 'auth' ? '' : 'display:none'}"><p class="section-label">Sign-in</p>
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
      <div class="field-group"><label>Exempt networks</label><div class="chip-input" id="guard-allow" data-hint="middleware.guard.allowFrom"></div></div>
    </div>

    <div class="card form-section ed-sub" data-type="rate-limit" style="${type === 'rate-limit' ? '' : 'display:none'}"><p class="section-label">Rate limit</p>
      ${rateLimitBlockHtml('mwrl', rl)}
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
      <div class="field-group"><label>API key</label><input class="field mono" id="bo-apikey" data-hint="middleware.bouncer.apiKey" value="${esc(bo.apiKey || '')}" placeholder="\${ENV:CROWDSEC_BOUNCER_KEY}" /><div class="hint">Sent as <span class="mono">X-Api-Key</span>. Use a <span class="mono">\${ENV:...}</span> or <span class="mono">\${FILE:...}</span> placeholder; a masked secret reads <span class="mono">***</span>.</div></div>
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
  </div>` + saveBar('middleware', isNew, meta.addLabel);

  const authCtl = wireAuthBlock('mw', auth, idps);
  const setReqCtl = makeKVRows($('#hdr-setreq'), headers.setRequest || {}, 'Header', 'value', false);
  const setRespCtl = makeKVRows($('#hdr-setresp'), headers.setResponse || {}, 'Header', 'value', false);
  const rmReqCtl = makeChipInput($('#hdr-rmreq'), arr(headers.removeRequest), 'add header...');
  const rmRespCtl = makeChipInput($('#hdr-rmresp'), arr(headers.removeResponse), 'add header...');
  const guardAllowCtl = makeChipInput($('#guard-allow'), arr(guard.allowFrom), 'add CIDR...');
  // The same rate-limit block (and the same per-window / per-second forms) the
  // host and location editors use for the same model.
  const rlCtl = wireRateLimitBlock('mwrl', rl);
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
    t = t || {}; const d = document.createElement('div'); d.className = 'sub-block';
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
      if (!Object.keys(spec).length) { showInvalid('Header rule required', 'Set or remove at least one request or response header.', $('#hdr-setreq')); return null; }
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
      if (!triggers.length) { showInvalid('Trigger required', 'Add at least one guard trigger.', $('#guard-triggers')); return null; }
      const spec = { triggers };
      const allow = guardAllowCtl.get(); if (allow.length) spec.allowFrom = allow;
      const ds = parseInt($('#guard-deny').value, 10); if (!isNaN(ds)) spec.denyStatus = ds;
      body.guard = spec;
    } else if (t === 'rate-limit') {
      const spec = rlCtl.get();
      if (!spec) return null;
      body.rateLimit = spec;
    } else if (t === 'bouncer') {
      const provider = $('#bo-provider').value;
      const url = $('#bo-url').value.trim();
      if (!url) { showInvalid('URL required', 'Enter the bouncer URL.', $('#bo-url')); return null; }
      const apiKey = $('#bo-apikey').value.trim();
      if (apiKey === '***') { showInvalid('Secret masked', 'The API key is masked as ***. Replace it with a real value or a ${ENV:...} placeholder.', $('#bo-apikey')); return null; }
      if (provider === 'crowdsec' && !apiKey) { showInvalid('API key required', 'The crowdsec provider needs an API key (cscli bouncers add gpm).', $('#bo-apikey')); return null; }
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
        if (!k.startsWith('/') || !v.startsWith('/')) { showInvalid('Invalid path', 'Path must be absolute (start with "/").', $('#rw-replacepath')); return null; }
        if (k === v) { showInvalid('No-op rewrite', `A path cannot be rewritten to itself ("${k}").`, $('#rw-replacepath')); return null; }
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
      if (pre.err) { showInvalid('Prefix rule invalid', pre.err, $('#rw-prefix .row-bad')); return null; }
      const rex = readRules(rwRegex, true);
      if (rex.err) { showInvalid('Regex rule invalid', rex.err, $('#rw-regex .row-bad')); return null; }
      if (!entries.length && !pre.out.length && !rex.out.length) {
        showInvalid('Rule required', 'Add at least one rule.', $('#rw-replacepath')); return null;
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
    `<button type="button" class="seg-btn${o.v === current ? ' on' : ''}" data-seg="${esc(group)}" data-v="${esc(o.v)}" aria-pressed="${o.v === current}">${esc(o.label)}</button>`).join('');
  const panels = opts.map((o) =>
    `<div data-seg-panel="${esc(group)}" data-v="${esc(o.v)}"${o.v === current ? '' : ' hidden'}>${o.panel}</div>`).join('');
  return `<div class="seg">${buttons}</div>${panels}`;
}
// wireSegs makes every segmented picker on the page switch panels, and calls
// onChange(group, value) so a caller can react to the choice.
function wireSegs(onChange) {
  $$('.seg-btn').forEach((b) => b.addEventListener('click', () => {
    const g = b.dataset.seg;
    $$(`.seg-btn[data-seg="${g}"]`).forEach((x) => { x.classList.toggle('on', x === b); x.setAttribute('aria-pressed', String(x === b)); });
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
//
// A fold's summary is rendered from the stored object; liveFoldSummaries then
// keeps it equal to the form: map is {foldId: () => summary text}, recomputed
// on every input, change, switchchange, chipchange and click inside root, so a
// collapsed fold never shows a stale state. Entries may be added to map later
// (a location row added after the page rendered).
function liveFoldSummaries(root, map) {
  const run = () => Object.keys(map).forEach((id) => {
    const el = document.querySelector(`#${CSS.escape(id)} > summary .fold-sum`);
    if (!el) return;
    const s = map[id]();
    if (s != null) el.innerHTML = glossaryize(esc(s));
  });
  if (root) ['input', 'change', 'switchchange', 'chipchange', 'click'].forEach((ev) => root.addEventListener(ev, run));
  return run;
}
function foldHtml(id, label, summary, open, body) {
  return `<details class="card form-section fold" id="${esc(id)}"${open ? ' open' : ''}>
    <summary><span class="section-label">${esc(label)}</span><span class="fold-sum">${glossaryize(esc(summary))}</span></summary>
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
    ${identityCard('ed', o, { section: 'clientcas', isNew, placeholder: seed && seed.origName + '-copy' })}
    ${anchorCard}
    ${crlCard}
    ${keyCard}
  </div>`;
  const actionStack = isNew ? '' : `<div class="stack">
    ${issueCard()}
    ${issuedCertsCard(issued)}
  </div>`;

  c.innerHTML = editorHead('clientcas', meta, isNew, name) + `<div id="issued-banner">${expiryBanner(issued)}</div>`
    + (isNew ? configStack : `<div class="form-grid">${configStack}${actionStack}</div>`)
    + saveBar('clientcas', isNew, meta.addLabel);

  wireEditor('clientcas', 'client-cas', meta, isNew, name || o.name, o, () => {
    const caPEM = $('#ed-capem').value.trim();
    if (!caPEM) { showInvalid('CA required', 'Paste the CA certificate PEM, or switch to "Generate new CA".', $('#ed-capem')); return null; }
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
    if (keyPEM === '***') { showInvalid('Secret masked', 'The inline CA key is masked as ***. Replace it with a ${FILE:...} placeholder or clear it.', $('#ed-cakeypem')); return null; }
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
      if (warnDays < 0 || warnDays > 3650) { showInvalid('Warning window out of range', 'Expiry warning must be between 0 (default 30) and 3650 days.', $('#ed-warndays')); return null; }
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
  // The two folds' summaries follow the form, on the same rule as their render.
  liveFoldSummaries($('#content'), {
    'crl-card': () => {
      const v = segValue('crl') === 'inline' ? ($('#ed-crlpem').value.trim() ? 'inline PEM' : '') : $('#ed-crlfile').value.trim();
      return v ? `${v}, ${$('#ed-crlpolicy').value || 'fail-closed'}` : 'not configured - a revoked certificate passes until it expires';
    },
    'cakey-card': () => {
      const v = segValue('cakey') === 'inline' ? ($('#ed-cakeypem').value.trim() ? 'inline key' : '') : $('#ed-cakeyfile').value.trim();
      return v || 'not configured - issuance disabled';
    },
  });
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
    clearEditorError();
    const nm = $('#ed-name').value.trim();
    if (!nm) { showInvalid('Name required', 'Enter a name for the CA first.', $('#ed-name')); return; }
    const days = parseInt($('#gen-days').value, 10);
    if (isNaN(days) || days < 1 || days > 7300) { showInvalid('Validity out of range', 'CA validity must be between 1 and 7300 days.', $('#gen-days')); return; }
    const body = { validityDays: days };
    const cn = $('#gen-cn').value.trim(); if (cn) body.commonName = cn;
    const org = $('#gen-org').value.trim(); if (org) body.organization = org;
    // POST /generate takes only the fields above. The rest of this page is
    // hidden while generating, except the identity card: say so before its
    // display name or disabled flag is dropped.
    const disp = $('#ed-display');
    if ((disp && disp.value.trim()) || $$('#ed-tags .chip-tok').length || isOn('ed-disabled')) {
      const go = await confirmModal({
        title: 'Display name, tags and Disabled are not saved',
        body: '<p>Generating creates the CA from its name and the fields in this panel only. Set the display name, tags or the Disabled switch after it is created.</p>',
        confirmLabel: 'Generate anyway',
        danger: false,
      });
      if (!go) return;
    }
    btn.disabled = true;
    try {
      const r = await api('/api/client-cas/' + encodeURIComponent(nm) + '/generate', { method: 'POST', body });
      toastSaved(r.commit, 'Added'); refreshHeadSha(); clearDirty();
      location.hash = '#/clientcas/' + encodeURIComponent(nm);
    } catch (e) { toastErr(e, `Could not generate ${nm}`); btn.disabled = false; }
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
      ? `expired ${plural(Math.abs(r.daysRemaining), 'day', 'days')} ago`
      : `${plural(r.daysRemaining, 'day', 'days')} left`;
    return `<li><b>${esc(r.commonName)}</b> <span class="mono">${esc(r.serial)}</span> - ${esc(when)} (expires ${esc(fmtTime(r.notAfter))})</li>`;
  }).join('');
  return `<div class="ro-banner ${anyExpired ? 'err' : 'warn'}">
    <b>${plural(bad.length, 'issued client certificate', 'issued client certificates')} ${anyExpired ? 'expired or expiring' : 'expiring soon'}.</b>
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
      <div class="hint">Optional, comma-separated.</div>
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
  // One "superseded" chip in the status column; the name cell used to carry a
  // second one beside a status chip that still read "valid".
  if (r.supersededBy) return `<span class="chip" title="${esc(r.status)}">Superseded</span>`;
  const cls = r.status === 'expired' ? 'err' : (r.status === 'expiring' ? 'warn' : 'ok');
  return `<span class="chip ${cls}"><span class="dot ${cls}"></span>${esc(r.status)}</span>`;
}

// issuedCertsCard lists this CA's issuance records with a per-row Renew action.
function issuedCertsCard(issued) {
  if (!issued.length) {
    return `<div class="card form-section" id="issued-card"><p class="section-label">Issued certificates</p>
      <div class="hint">None yet. Certificates issued above appear here with their expiry.</div>
    </div>`;
  }
  const rows = issued.map((r) => `<tr${r.supersededBy ? ' class="superseded"' : ''}>
      <td class="col-name"><span class="trunc" title="${esc(r.commonName)}">${esc(r.commonName)}</span></td>
      <td class="mono col-serial"><span class="trunc" title="${esc(r.serial)}">${esc(r.serial)}</span></td>
      <td class="col-time" data-v="${esc(r.notAfter || '')}">${esc(fmtTime(r.notAfter))}</td>
      <td class="col-status">${issuedStatusChip(r)}</td>
      <td class="col-actions">${r.supersededBy
        ? `<span class="hint">renewed as <span class="mono" title="${esc(r.supersededBy)}">${esc(r.supersededBy)}</span></span>`
        : `<button class="btn sm ren-btn" type="button" data-serial="${esc(r.serial)}" data-cn="${esc(r.commonName)}">Renew</button>`}</td>
    </tr>
    ${r.supersededBy ? '' : `<tr class="ren-row sub-row" id="ren-${esc(r.serial)}" hidden><td colspan="5">
      <div class="inline-fields">
        <div class="field-group"><label>New bundle password</label>
          <input class="field mono ren-pw" data-hint="clientCA.renew.password" type="password" minlength="12" placeholder="at least 12 characters" /></div>
        <div class="field-group"><label>Validity (days)</label>
          <input class="field mono ren-days" data-hint="clientCA.renew.days" type="number" min="1" max="3650" value="365" /></div>
      </div>
      <div style="display:flex;gap:10px">
        <button class="btn primary ren-go" type="button" data-serial="${esc(r.serial)}" data-cn="${esc(r.commonName)}">Confirm renewal</button>
        <button class="btn ghost ren-cancel" type="button" data-serial="${esc(r.serial)}">Cancel</button>
      </div>
      <div class="hint" style="margin-top:8px">New key and serial, same subject. Does not revoke the current certificate - every device must import the new .p12.</div>
    </td></tr>`}`).join('');
  return `<div class="card form-section" id="issued-card"><p class="section-label">Issued certificates</p>
    <div class="table-wrap"><table class="mini-table" data-sort-key="gpm.issued.sort">
      <thead><tr><th data-sort="cn">Common name</th><th data-sort="serial">Serial</th><th class="col-time" data-sort="expires">Expires</th><th data-sort="status">Status</th><th class="col-actions"></th></tr></thead>
      <tbody>${rows}</tbody>
    </table></div>
  </div>`;
}

// Repaints only the issued-certificates card and the expiry banner after an
// issue or a renewal. A full route() would also re-render the config cards on
// the left and throw away any unsaved edit there without asking.
async function refreshIssuedCerts(name, hasKey) {
  const r = await api('/api/client-cas/' + encodeURIComponent(name) + '/certificates').catch(() => ({ data: {} }));
  const issued = arr((r.data || {}).certificates);
  const banner = $('#issued-banner');
  if (banner) banner.innerHTML = expiryBanner(issued);
  const card = $('#issued-card');
  if (card) card.outerHTML = issuedCertsCard(issued);
  wireMiniTables($('#issued-card'));
  wireClientCertRenew(name, hasKey);
}

// wireClientCertRenew wires the per-row Renew action. Clicking Renew only reveals
// the form; the request is sent from "Confirm renewal", behind confirmModal
// with the consequences spelled out.
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
      clearEditorError();
      const row = $('#ren-' + CSS.escape(b.dataset.serial));
      const pw = $('.ren-pw', row).value;
      if (!pw) { showInvalid('Password required', 'The renewed PKCS#12 bundle must be password-protected.', $('.ren-pw', row)); return; }
      if (pw.length < P12_MIN_PASSWORD) { showInvalid('Password too short', `Use at least ${P12_MIN_PASSWORD} characters: the legacy PKCS#12 encoder barely stretches it, so a short password is cheap to crack offline once the file leaves gpm.`, $('.ren-pw', row)); return; }
      const days = parseInt($('.ren-days', row).value, 10);
      if (isNaN(days) || days < 1 || days > 3650) { showInvalid('Validity out of range', 'Validity must be between 1 and 3650 days.', $('.ren-days', row)); return; }
      const go = await confirmModal({
        title: `Renew certificate ${b.dataset.cn}?`,
        body: '<p>A new private key and certificate are generated. The download is the only copy: it is never stored and cannot be recovered.</p>'
          + '<p>The current certificate is <b>not</b> revoked and keeps working until it expires. Every device that uses it must import the new .p12 by hand.</p>',
        confirmLabel: 'Renew certificate',
      });
      if (!go) return;
      b.disabled = true;
      try {
        await downloadP12('/api/client-cas/' + encodeURIComponent(name) + '/certificates/'
          + encodeURIComponent(b.dataset.serial) + '/renew', { password: pw, validityDays: days }, b.dataset.cn);
        toast('Certificate renewed', 'Import the new .p12 on every device using this certificate - the old one is not revoked and still works until it expires.', 'ok');
        await refreshIssuedCerts(name, hasKey);
      } catch (e) { toastErr(e, 'Could not renew the certificate'); b.disabled = false; }
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
    clearEditorError();
    const cn = $('#iss-cn').value.trim();
    if (!cn) { showInvalid('Common name required', 'Enter the certificate common name.', $('#iss-cn')); return; }
    const pw = $('#iss-pw').value;
    if (!pw) { showInvalid('Password required', 'The PKCS#12 bundle must be password-protected.', $('#iss-pw')); return; }
    if (pw.length < P12_MIN_PASSWORD) { showInvalid('Password too short', `Use at least ${P12_MIN_PASSWORD} characters: the legacy PKCS#12 encoder barely stretches it, so a short password is cheap to crack offline once the file leaves gpm.`, $('#iss-pw')); return; }
    const days = parseInt($('#iss-days').value, 10);
    if (isNaN(days) || days < 1 || days > 3650) { showInvalid('Validity out of range', 'Validity must be between 1 and 3650 days.', $('#iss-days')); return; }
    const sans = $('#iss-sans').value.split(',').map((v) => v.trim()).filter(Boolean);
    const body = { commonName: cn, validityDays: days, password: pw };
    if (sans.length) body.sans = sans;

    btn.disabled = true;
    try {
      const file = await downloadP12('/api/client-cas/' + encodeURIComponent(name) + '/issue', body, cn);
      $('#iss-pw').value = '';
      toast('Certificate issued', `Downloaded ${file}. It is not stored - keep the file and its password.`, 'ok');
      await refreshIssuedCerts(name, hasKey);
    } catch (e) { toastErr(e, 'Could not issue the certificate'); } finally { btn.disabled = false; }
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
  // Some browsers start the download after click() returns; revoking at once
  // can cancel it.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
  return a.download;
}

// Pulls the filename out of a Content-Disposition header, or returns '' so the
// caller can fall back. The server already sanitizes it; this only unquotes.
function filenameFromDisposition(cd) {
  const m = /filename="([^"]*)"/.exec(cd || '');
  return m ? m[1] : '';
}

// ---------- UPSTREAM GROUP EDITOR ----------
function upExtraSummary(path, hostHeader) {
  const parts = [path ? 'base path ' + path : '', hostHeader ? 'Host ' + hostHeader : ''].filter(Boolean);
  return 'Advanced' + (parts.length ? ' - ' + parts.join(', ') : '');
}
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
      ? '<span class="chip flat ok" style="flex:0 0 auto"><span class="dot ok"></span>Up</span>'
      : '<span class="chip err" style="flex:0 0 auto"><span class="dot err"></span>Down</span>';
  };
  c.innerHTML = editorHead('upstreams', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${identityCard('ed', o, { section: 'upstreams', isNew, placeholder: seed && seed.origName + '-copy' })}
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
      <div class="inline-fields">
        <div class="field-group"><label>Rise</label><input class="field mono" id="ed-hc-rise" data-hint="upstreamGroup.healthCheck.rise" type="number" min="1" max="10" value="${esc(hc.rise || '')}" placeholder="2" /></div>
        <div class="field-group"><label>Fall</label><input class="field mono" id="ed-hc-fall" data-hint="upstreamGroup.healthCheck.fall" type="number" min="1" max="10" value="${esc(hc.fall || '')}" placeholder="2" /></div>
      </div>
    </div>
  </div></div>` + saveBar('upstreams', isNew, meta.addLabel);

  const upsWrap = $('#ed-ups');
  const advSums = {};
  liveFoldSummaries(upsWrap, advSums);
  const upCtls = [];
  let upSeq = 0;
  function upRow(u) {
    u = u || {};
    const p = 'ug' + (++upSeq);
    const div = document.createElement('div');
    div.className = 'sub-block up-block';
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
      <details class="fold sub-fold" id="${p}-adv"${hasExtra ? ' open' : ''}><summary><span class="fold-sum">${esc(upExtraSummary(u.path, u.hostHeader))}</span></summary>
        ${upstreamExtraHtml(p, u)}
      </details>`;
    upsWrap.appendChild(div);
    const ctl = { div, _orig: u, extra: wireUpstreamExtra(p) };
    upCtls.push(ctl);
    advSums[p + '-adv'] = () => {
      const hh = $('#' + p + '-uphh').value;
      return upExtraSummary($('#' + p + '-uppath').value.trim(), hh === 'custom' ? $('#' + p + '-uphh-custom').value.trim() : hh);
    };
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
    let bad = null;
    let aborted = false;
    clearRowErrors(upsWrap);
    for (const ctl of upCtls) {
      if (bad || aborted) break;
      const row = ctl.div;
      const host = row.querySelector('.up-host').value.trim();
      const port = parseInt(row.querySelector('.up-port').value, 10);
      if (!host && isNaN(port)) continue; // fully empty row: skip
      // A half-filled member is marked on its own row, like a location row.
      if (!host || isNaN(port)) { markRowError(row, host ? 'This upstream needs a port.' : 'This upstream needs a host.'); bad = row.querySelector(host ? '.up-port' : '.up-host'); break; }
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
    if (bad) { showInvalid('Upstream incomplete', 'Every upstream needs a host and a port.', bad); return null; }
    if (!ups.length) { showInvalid('Upstream required', 'Add at least one upstream.', $('#ed-ups .up-host')); return null; }
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
  if (!t.expiresAt) return 'Never';
  const d = new Date(t.expiresAt);
  if (isNaN(d.getTime())) return esc(t.expiresAt);
  return fmtTime(t.expiresAt) + (d.getTime() < Date.now() ? ' (expired)' : '');
}

// One-time reveal. The secret exists only in this response, so it is shown in a
// blocking panel with a copy button and never re-fetched.
function revealToken(secret) {
  const wrap = document.createElement('div');
  wrap.className = 'card form-section';
  wrap.style.cssText = 'border-color:var(--brand);margin-bottom:16px';
  wrap.innerHTML = `
    <p class="section-label">New token - shown once</p>
    <p class="muted" style="font-size:13.5px;margin:0 0 10px">Copy this now. Only its SHA-256 digest is stored, so it cannot be shown again. Lost it? Rotate the token.</p>
    <div class="loc-row">
      <input class="field mono" id="tok-reveal" readonly value="${esc(secret)}" aria-label="New API token" style="flex:3 1 260px" />
      <button class="btn primary sm" id="tok-copy" type="button">Copy</button>
      <button class="btn ghost sm" id="tok-dismiss" type="button">Done</button>
    </div>`;
  const content = $('#content');
  content.insertBefore(wrap, content.firstChild);
  scrollToTop();
  $('#tok-copy').addEventListener('click', async () => {
    const inp = $('#tok-reveal');
    inp.select();
    try { await navigator.clipboard.writeText(secret); toast('Copied', 'Token copied to the clipboard.', 'ok'); }
    catch (e) { toast('Copy manually', 'Select the field and copy it.', 'err'); }
  });
  $('#tok-dismiss').addEventListener('click', () => wrap.remove());
}

// A token's history cannot undo anything: the API refuses to revert an API
// token, because that would bring a revoked secret back to life.
const TOKEN_NO_UNDO = 'Recorded as a commit. History cannot bring a token back: reverting an API token is refused.';
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
    // Three chips then "+N": a broad token used to be a wall of chips that made
    // its row several lines tall. The full list is the cell's title.
    const shown = list.slice(0, 3).map((s) => `<span class="chip">${esc(s)}</span>`).join('');
    if (list.length <= 3) return shown;
    const m = moreToggle(list.slice(3), list.slice(3).map((s) => `<span class="chip">${esc(s)}</span>`).join(''));
    return shown + m.list + m.btn;
  };
  const TOK_FIELDS = { name: 'token name', scope: 'a scope such as proxy-hosts:write', status: 'live, disabled or expired' };
  const tokRecs = {};
  tokens.forEach((t) => {
    const status = t.disabled ? 'disabled' : (t.expiresAt && new Date(t.expiresAt) < new Date() ? 'expired' : 'live');
    tokRecs[t.name] = {
      blob: [t.name, t.displayName, arr(t.scopes).join(' '), status].join(' ').toLowerCase(),
      f: { name: fvals(t.name, t.displayName), scope: fvals(t.scopes), status: fvals(status) },
    };
  });
  const rows = tokens.map((t) => `
    <tr data-name="${esc(t.name)}">
      <td class="col-name">
        <div class="cell-main"><span class="mono host trunc" title="${esc(t.name)}">${esc(t.name)}</span>${t.createdAt ? `<span class="trunc faint mono" title="Created ${esc(fmtTime(t.createdAt))}">created ${esc(fmtDate(t.createdAt))}</span>` : ''}${t.disabled ? DISABLED_CHIP : ''}</div>
      </td>
      <td><div class="chip-row" title="${esc(arr(t.scopes).join(', '))}">${scopeChips(t.scopes)}</div></td>
      <td class="mono col-time" data-v="${esc(t.expiresAt || '')}">${tokenExpiryLabel(t)}</td>
      <td class="mono faint col-time" data-v="${esc(t.lastUsed || '')}">${t.lastUsed ? esc(fmtTime(t.lastUsed)) : 'Never'}</td>
      <td class="col-actions">
        <div class="row-actions">
          <button class="btn ghost sm tok-edit" data-name="${esc(t.name)}" type="button">Edit</button>
          <button class="btn ghost sm tok-toggle" data-name="${esc(t.name)}" type="button">${t.disabled ? 'Enable' : 'Disable'}</button>
          <button class="btn ghost sm tok-rotate" data-name="${esc(t.name)}" type="button">Rotate</button>
          <button class="btn danger-soft sm tok-del" data-name="${esc(t.name)}" type="button">Delete</button>
        </div>
      </td>
    </tr>`).join('');

  // The list comes first; "Add token" opens the create form above it.
  c.innerHTML = viewHead('API Tokens',
    'Credentials for scripts and CI, sent as a Bearer token. Scopes limit what each token can reach.',
    `<button class="btn primary" id="tok-add" type="button" aria-expanded="false" aria-controls="tok-form">${ICON.plus}Add token</button>`) +
    `<div class="card form-section" id="tok-form" style="margin-bottom:16px" hidden>
      <p class="section-label" id="tok-form-title">Add token</p>
      <div class="inline-fields">
        <div class="field-group"><label>Name</label><input class="field mono" id="tok-name" data-hint="apiToken.name" placeholder="ci-deploy" /><div class="hint" id="tok-name-hint" hidden>Immutable after creation.</div></div>
        <div class="field-group field-narrow"><label>Expires</label><input class="field mono" id="tok-expires" data-hint="apiToken.expiresAt" type="date" /></div>
      </div>
      <div class="field-group">
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
      <div class="hint" id="tok-edit-note" hidden>Saving keeps the token's secret: only its scopes and expiry change. Rotate it from the list to mint a new secret.</div>
      <div style="display:flex;gap:10px;margin-top:12px">
        <button class="btn primary" id="tok-create" type="button">${ICON.plus}Add token</button>
        <button class="btn ghost" id="tok-cancel" type="button">Cancel</button>
      </div>
    </div>` +
    (tokens.length ? `<div class="toolbar">
        ${filterBoxHtml('tokFilter', 'filter tokens...', 'Filter tokens', TOK_FIELDS)}
      </div>
      <div class="table-wrap"><table id="tokTable">
        <thead><tr><th class="col-name" data-sort="name">Name</th><th data-sort="scopes">Scopes</th><th class="col-time" data-sort="expires">Expires</th><th class="col-time" data-sort="used">Last used</th><th class="col-actions"></th></tr></thead>
        <tbody id="tokRows">${rows}</tbody>
      </table></div>
      <div class="list-empty" id="tokNone" hidden>No token matches this filter.</div>`
      : emptyState('No API tokens yet',
        'Add one with only the scopes the script needs - write implies read, so a deploy job usually wants proxy-hosts:write and nothing else.'));

  const tokForm = $('#tok-form'), tokAdd = $('#tok-add');
  // One form, two modes. editing holds the stored token while its scopes or
  // expiry are being changed; null means the form adds a new one. editingExtra
  // keeps the scopes the grid has no row for (a wildcard like "*:read") so an
  // edit never silently narrows the token; editingExpiry is the stored expiry
  // as loaded, re-sent verbatim unless the date input was changed.
  let editing = null, editingExtra = [], editingExpiry = '';
  const setMode = (t) => {
    editing = t || null;
    $('#tok-form-title').textContent = editing ? `Edit token ${editing.name}` : 'Add token';
    $('#tok-create').innerHTML = editing ? 'Save changes' : `${ICON.plus}Add token`;
    $('#tok-name').readOnly = !!editing;
    $('#tok-name-hint').hidden = !editing;
    $('#tok-edit-note').hidden = !editing;
  };
  const showForm = (open) => {
    tokForm.hidden = !open;
    tokAdd.setAttribute('aria-expanded', String(open));
    if (open) { (editing ? $('#tok-expires') : $('#tok-name')).focus(); } else { clearEditorError(); tokAdd.focus(); }
  };
  tokAdd.addEventListener('click', () => {
    if (!tokForm.hidden && !editing) { showForm(false); return; }
    fillForm(null);
    showForm(true);
  });
  $('#tok-cancel').addEventListener('click', () => showForm(false));

  wireTableSort($('#tokTable'), 'gpm.tokens.sort', { key: 'name', dir: 1 });
  wireFilter('tokFilter', TOK_FIELDS, () => fieldValues(Object.values(tokRecs)), (terms) => {
    applyRowFilter($$('#tokRows tr[data-name]'), tokRecs, terms, $('#tokNone'));
    highlightTerms($('#tokRows'), terms);
  }, 'tokens');

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

  // fillForm loads a stored token into the form (scopes, expiry) or clears it.
  function fillForm(t) {
    setMode(t);
    $('#tok-name').value = t ? t.name : '';
    editingExpiry = t && t.expiresAt ? String(t.expiresAt) : '';
    $('#tok-expires').value = editingExpiry ? editingExpiry.slice(0, 10) : '';
    const scopes = t ? arr(t.scopes) : [];
    // Any "admin" entry is full admin, whatever else rides beside it.
    $('#tok-admin').setAttribute('aria-checked', scopes.includes('admin') ? 'true' : 'false');
    syncAdmin();
    const known = new Set($$('#tok-scopes .tok-read').map((i) => i.dataset.p));
    editingExtra = scopes.filter((sc) => {
      const m = /^([^:]+):(read|write)$/.exec(sc);
      return sc !== 'admin' && !(m && known.has(m[1]));
    });
    scopeGrid.querySelectorAll('.tok-read, .tok-write').forEach((box) => {
      const verb = box.classList.contains('tok-write') ? 'write' : 'read';
      box.checked = scopes.includes(box.dataset.p + ':' + verb) || (verb === 'read' && scopes.includes(box.dataset.p + ':write'));
    });
  }

  $$('.tok-edit').forEach((b) => {
    b.addEventListener('click', () => {
      const cur = tokens.find((t) => t.name === b.dataset.name);
      if (!cur) return;
      fillForm(cur);
      showForm(true);
      scrollToTop();
    });
  });

  $('#tok-create').addEventListener('click', async () => {
    clearEditorError();
    const nm = $('#tok-name').value.trim();
    if (!nm) { showInvalid('Name required', 'Enter a token name.', $('#tok-name')); return; }
    const scopes = collectScopes();
    if (!scopes.length) { showInvalid('Scope required', 'Pick at least one scope, or Full admin.', $('#tok-scopes')); return; }
    const exp = $('#tok-expires').value;
    const btn = $('#tok-create'); btn.disabled = true;
    if (editing) {
      // Whole-object PUT without ?rotate=1: the server carries the stored
      // digest forward, so the secret in use keeps working. Scopes the grid
      // cannot show ride along unchanged unless Full admin now covers them.
      const merged = scopes.includes('admin') ? scopes : scopes.concat(editingExtra.filter((sc) => !scopes.includes(sc)));
      const body = Object.assign(metaCarryForward(editing), { scopes: merged });
      if (editing.displayName) body.displayName = editing.displayName;
      if (editing.disabled) body.disabled = true;
      if (exp && exp === editingExpiry.slice(0, 10)) body.expiresAt = editingExpiry;
      else if (exp) body.expiresAt = new Date(exp + 'T23:59:59Z').toISOString();
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'PUT', body });
        toastSaved(r.commit, 'Saved'); refreshHeadSha();
        if (c.isConnected) await route();
      } catch (e) { toastErr(e, `Could not save ${nm}`); btn.disabled = false; }
      return;
    }
    const body = { scopes };
    if (exp) body.expiresAt = new Date(exp + 'T23:59:59Z').toISOString();
    try {
      const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'PUT', body, headers: { 'If-None-Match': '*' } });
      toastSaved(r.commit, 'Added'); refreshHeadSha();
      const secret = r.data && r.data.token;
      // route(), not viewTokens(c), so the shell banners and gating survive.
      if (c.isConnected) await route();
      if (secret) revealToken(secret);
    } catch (e) {
      // A token has no page of its own: the stored one is in the list below,
      // where Rotate replaces its secret.
      if (!showNameTaken(e, 'token', nm, '', 'Could not add this token', $('#tok-name'))) toastErr(e, 'Could not add this token');
      btn.disabled = false;
    }
  });

  // Enable and disable are ordinary whole-object PUTs without ?rotate=1, so
  // the stored secret is kept.
  $$('.tok-toggle').forEach((b) => {
    b.addEventListener('click', async () => {
      const nm = b.dataset.name;
      const cur = tokens.find((t) => t.name === nm) || {};
      const body = Object.assign(metaCarryForward(cur), { scopes: arr(cur.scopes) });
      if (cur.displayName) body.displayName = cur.displayName;
      if (cur.expiresAt) body.expiresAt = cur.expiresAt;
      if (!cur.disabled) body.disabled = true;
      clearEditorError();
      b.disabled = true;
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'PUT', body });
        toastSaved(r.commit, 'Saved'); refreshHeadSha();
        if (c.isConnected) await route();
      } catch (e) { toastErr(e, cur.disabled ? `Could not enable ${nm}` : `Could not disable ${nm}`); b.disabled = false; }
    });
  });

  $$('.tok-rotate').forEach((b) => {
    b.addEventListener('click', async () => {
      const nm = b.dataset.name;
      const go = await confirmModal({
        title: `Rotate token ${nm}?`,
        body: '<p>The current secret stops working immediately. The new one is shown once, right after the rotation.</p>'
          + `<p>${esc(TOKEN_NO_UNDO)}</p>`,
        confirmLabel: 'Rotate token',
      });
      if (!go) return;
      clearEditorError();
      b.disabled = true;
      const cur = tokens.find((t) => t.name === nm) || {};
      // Whole-object PUT: carry the meta the rotate action has no control for.
      const body = Object.assign(metaCarryForward(cur), { scopes: arr(cur.scopes) });
      if (cur.displayName) body.displayName = cur.displayName;
      if (cur.expiresAt) body.expiresAt = cur.expiresAt;
      if (cur.disabled) body.disabled = true;
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm) + '?rotate=1', { method: 'PUT', body });
        toastSaved(r.commit, 'Rotated'); refreshHeadSha();
        const secret = r.data && r.data.token;
        if (c.isConnected) await route();
        if (secret) revealToken(secret);
      } catch (e) { toastErr(e, `Could not rotate ${nm}`); b.disabled = false; }
    });
  });

  $$('.tok-del').forEach((b) => {
    b.addEventListener('click', async () => {
      const nm = b.dataset.name;
      const go = await confirmModal({
        title: `Delete token ${nm}?`,
        body: '<p>Anything that uses it stops working immediately.</p>'
          + `<p>${esc(TOKEN_NO_UNDO)}</p>`,
        confirmLabel: 'Delete token',
      });
      if (!go) return;
      clearEditorError();
      b.disabled = true;
      try {
        const r = await api('/api/api-tokens/' + encodeURIComponent(nm), { method: 'DELETE' });
        toastSaved(r.commit, 'Deleted');
        refreshHeadSha(); if (c.isConnected) await route();
      } catch (e) {
        if (e && (e.status === 409 || e.status === 400)) showSaveError(e, `Could not delete ${nm}`);
        else toastErr(e, `Could not delete ${nm}`);
        b.disabled = false;
      }
    });
  });
}

// ---------- ACCESS LOGS ----------
const LOGS_AUTO_KEY = 'gpm.logs.auto';
const LOGS_SORT_KEY = 'gpm.logs.sort';
const LOG_FIELDS = {
  method: 'request method', host: 'the Host header', path: 'request path', status: 'a status code, or a class such as 5xx',
  client: 'an IP, a network such as 10.0.0.0/8, or @list for the networks an access list allows', slow: 'yes: requests slower than 1s',
};
// clientHit matches one client alternative: a CIDR or bare IP by address,
// @name by the literal allow networks of that access list (nets is
// {name: [cidr]}; an unknown name matches nothing), anything else as a
// substring of the logged address, so `client:10.10.2.` still works.
function clientHit(client, alt, nets) {
  if (alt.charAt(0) === '@') {
    const list = (nets || {})[alt.slice(1)];
    return !!list && list.some((n) => cidrHit(client, n));
  }
  if (isCidrOrIP(alt)) return cidrHit(client, alt);
  return String(client || '').toLowerCase().indexOf(alt) !== -1;
}
function logRec(e, nets) {
  const cls = e.status >= 100 ? Math.floor(e.status / 100) + 'xx' : '';
  return {
    blob: [e.method, e.host, e.path, e.status, cls, e.client].join(' ').toLowerCase(),
    f: {
      method: fvals(e.method), host: fvals(e.host), path: fvals(e.path), status: fvals(e.status, cls),
      client: fvals(e.client), slow: fvals(e.durMs >= 1000 ? 'yes' : 'no'),
    },
    m: { client: (alt) => clientHit(e.client, alt, nets) },
  };
}
// The networks each access list allows (its literal cidr rules plus what
// its named sources last fetched), keyed by lower-cased list name, for
// `client:@name` terms and their suggestions. A failed load (or a token
// without access-lists:read) just means no @ terms.
async function accessListNets() {
  let byList;
  try { byList = (await api('/api/access-list-networks')).data || {}; } catch (e) { return {}; }
  const nets = {};
  Object.keys(byList).forEach((name) => {
    const cidrs = arr(byList[name]).map((c) => String(c).trim()).filter(Boolean);
    if (cidrs.length) nets[name.toLowerCase()] = cidrs;
  });
  return nets;
}
function logRowHtml(e, i) {
  const statusClass = e.status >= 500 ? 'err' : (e.status >= 400 ? 'warn' : 'ok');
  return `<tr data-i="${i}">
      <td class="mono faint col-time" data-v="${esc(e.time || '')}" title="${esc(fmtTime(e.time, true))}">${esc(fmtClock(e.time))}</td>
      <td class="mono col-method">${esc(e.method || '')}</td>
      <td class="mono col-host"><span class="trunc" title="${esc(e.host || '')}">${esc(e.host || '')}</span></td>
      <td class="mono col-path"><span class="trunc" title="${esc(e.path || '')}">${esc(e.path || '')}</span></td>
      <td class="col-status" data-v="${esc(e.status)}"><span class="chip ${statusClass}">${esc(e.status)}</span></td>
      <td class="mono col-num" data-v="${esc(e.durMs)}">${esc(e.durMs)}ms</td>
      <td class="mono col-num" data-v="${esc(e.bytes || 0)}">${esc(fmtBytes(e.bytes || 0))}</td>
      <td class="mono faint col-client"><span class="trunc" title="${esc(e.client || '')}">${esc(e.client || '')}</span></td>
    </tr>`;
}
function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
  return (n / (1024 * 1024)).toFixed(1) + ' MB';
}
async function viewLogs(c) {
  const data = (await api('/api/logs')).data || {};
  const enabled = !!data.enabled;
  const nets = await accessListNets();
  let entries = arr(data.entries);
  let recs = entries.map((e) => logRec(e, nets));
  const capacity = data.capacity || 0;
  let auto = 0;
  try { auto = Number(localStorage.getItem(LOGS_AUTO_KEY)) || 0; } catch (e) { /* ignore */ }
  const AUTO = [[0, 'Off'], [2, 'Every 2s'], [5, 'Every 5s'], [15, 'Every 15s'], [60, 'Every minute']];

  c.innerHTML = `
    ${pageHead('Access Logs', 'Most recent proxied requests, newest first (in-memory buffer).')}
    <div class="view-head">
      <label class="auto-refresh" for="logsAuto"><span>Auto-refresh</span><select class="field" id="logsAuto" aria-label="Auto-refresh interval">${AUTO.map(([v, l]) => `<option value="${v}"${v === auto ? ' selected' : ''}>${l}</option>`).join('')}</select></label>
      <button class="btn" id="logsRefresh" type="button">${ICON.history}Refresh</button>
      <button class="btn" id="logsClear" type="button"${entries.length ? '' : ' disabled'}>Clear</button>
      <button class="btn" id="logsToggle" type="button">${enabled ? 'Disable capture' : 'Enable capture'}</button>
      <div class="fsaved"><button class="btn" id="logsExport" type="button" aria-haspopup="menu" aria-expanded="false" aria-controls="logsExportMenu"${entries.length ? '' : ' disabled'} title="Download the rows the filter shows, in the table's order">Export${ICON.caret}</button><div class="menu fsaved-menu" id="logsExportMenu" role="menu" hidden>
        <button type="button" role="menuitem" data-fmt="csv">CSV, the rows shown</button>
        <button type="button" role="menuitem" data-fmt="json">JSON, the rows shown</button>
      </div></div>
    </div>
    <div class="toolbar" id="logsToolbar"${entries.length ? '' : ' hidden'}>
      ${filterBoxHtml('logFilter', 'filter requests...', 'Filter log entries', LOG_FIELDS)}
      <div class="toolbar-actions"><span class="muted" id="logsCount"></span></div>
    </div>
    <div class="table-wrap" id="logsWrap"${entries.length ? '' : ' hidden'}>
      <table id="logsTable">
        <thead><tr><th class="col-time" data-sort="time">Time</th><th data-sort="method">Method</th><th data-sort="host">Host</th><th data-sort="path">Path</th><th data-sort="status" data-type="num">Status</th><th class="col-num" data-sort="dur" data-type="num">Duration</th><th class="col-num" data-sort="bytes" data-type="num">Bytes</th><th data-sort="client">Client</th></tr></thead>
        <tbody id="logRows"></tbody>
      </table>
    </div>
    <div class="list-empty" id="logsNone" hidden>No request matches this filter.</div>
    <div id="logsEmpty"></div>`;

  // Capture off: say so where the rows would be; the button above turns it
  // on live (the -access-log flag / GPM_ACCESS_LOG decide the startup state).
  const emptyHtml = () => (enabled
    ? emptyState('No requests captured yet', 'Requests that reach a proxy host appear here as they arrive.')
    : emptyState('Capture is off', 'Enable capture to record requests from now on; it stays on until the next restart.'));
  let terms = [];
  const sorter = wireTableSort($('#logsTable'), LOGS_SORT_KEY, { key: 'time', dir: -1 });
  function paint() {
    $('#logRows').innerHTML = entries.map(logRowHtml).join('');
    sorter.apply();
    const shown = applyRowFilter($$('#logRows tr[data-i]'), recs, terms);
    $('#logsNone').hidden = !entries.length || shown > 0;
    highlightTerms($('#logRows'), terms);
    $('#logsCount').textContent = (shown === entries.length ? `${entries.length}` : `${shown} of ${entries.length}`)
      + ` ${entries.length === 1 ? 'request' : 'requests'}` + (capacity ? ` (buffer holds ${capacity})` : '');
    $('#logsToolbar').hidden = !entries.length;
    $('#logsWrap').hidden = !entries.length;
    $('#logsClear').disabled = !entries.length || $('#logsClear').classList.contains('cap-disabled');
    $('#logsExport').disabled = !entries.length;
    $('#logsEmpty').innerHTML = entries.length ? '' : emptyHtml();
  }
  paint();
  // Export: the rows the filter shows, in the order the table sorts them.
  // A cell that would read as a spreadsheet formula is prefixed with a quote
  // so a crafted request path cannot run in whatever opens the CSV.
  const shownEntries = () => $$('#logRows tr[data-i]').filter((r) => r.style.display !== 'none').map((r) => entries[Number(r.dataset.i)]).filter(Boolean);
  const csvCell = (v) => {
    let t = v == null ? '' : String(v);
    if (/^[=+\-@\t\r]/.test(t)) t = "'" + t;
    return '"' + t.replace(/"/g, '""') + '"';
  };
  function exportLogs(fmt) {
    const rows = shownEntries();
    const stamp = new Date().toISOString().replace(/[:.]/g, '-');
    let text, type;
    if (fmt === 'csv') {
      const cols = ['time', 'method', 'host', 'path', 'status', 'durMs', 'bytes', 'client'];
      text = [cols.join(',')].concat(rows.map((e) => cols.map((k) => csvCell(e[k])).join(','))).join('\r\n') + '\r\n';
      type = 'text/csv';
    } else {
      text = JSON.stringify(rows, null, 2) + '\n';
      type = 'application/json';
    }
    const a = document.createElement('a');
    const url = URL.createObjectURL(new Blob([text], { type }));
    a.href = url;
    a.download = `gpm-access-log-${stamp}.${fmt}`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    toast('Exported', `${rows.length} ${rows.length === 1 ? 'request' : 'requests'} as ${fmt.toUpperCase()}`, 'ok');
  }
  const exportMenu = wireMenu($('#logsExport'), $('#logsExportMenu'), () => {});
  $('#logsExportMenu').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-fmt]');
    if (!b) return;
    exportMenu.close();
    exportLogs(b.dataset.fmt);
  });
  // Suggestions for client: the @lists first, then the addresses present.
  const suggestions = () => {
    const v = fieldValues(recs);
    v.client = Object.keys(nets).sort().map((n) => '@' + n).concat(v.client || []);
    return v;
  };
  wireFilter('logFilter', LOG_FIELDS, suggestions, (t) => { terms = t; paint(); }, 'logs');

  // A refresh replaces the rows in place and keeps the filter, the sort and
  // the scroll position; the capture state and the shell banners are only
  // re-read through route().
  async function refresh() {
    let d;
    try { d = (await api('/api/logs')).data || {}; } catch (e) { toastErr(e, 'Could not refresh the access log'); return; }
    if (!!d.enabled !== enabled) { if (c.isConnected) await route(); return; }
    entries = arr(d.entries);
    recs = entries.map((e) => logRec(e, nets));
    paint();
  }
  // The timer belongs to this render: it stops when the route moves on
  // (routeSeq changes) or the tab is hidden, and never stacks.
  const mySeq = routeSeq;
  let timer = null;
  function schedule() {
    if (timer) { clearInterval(timer); timer = null; }
    if (!auto) return;
    timer = setInterval(() => {
      if (mySeq !== routeSeq || !c.isConnected) { clearInterval(timer); timer = null; return; }
      if (document.hidden) return;
      refresh();
    }, auto * 1000);
  }
  schedule();
  $('#logsAuto').addEventListener('change', () => {
    auto = Number($('#logsAuto').value) || 0;
    try { localStorage.setItem(LOGS_AUTO_KEY, String(auto)); } catch (e) { /* ignore */ }
    schedule();
  });
  $('#logsRefresh').addEventListener('click', refresh);
  $('#logsClear').addEventListener('click', async () => {
    const ok = await confirmModal({
      title: 'Clear the access log?',
      body: `<p>Drops the ${plural(entries.length, 'captured request', 'captured requests')} held in memory. Capture ${enabled ? 'stays on, so new requests keep arriving' : 'stays off'}. Nothing on disk changes.</p>`,
      confirmLabel: 'Clear log', danger: true,
    });
    if (!ok) return;
    try {
      await api('/api/logs', { method: 'DELETE' });
      toast('Access log cleared', '', 'ok');
      await refresh();
    } catch (e) { toastErr(e, 'Could not clear the access log'); }
  });
  // route(), not viewLogs(c): a bare re-render drops the shell banners and the
  // read-only gating of "Disable capture".
  $('#logsToggle').addEventListener('click', async () => {
    const btn = $('#logsToggle');
    btn.disabled = true;
    try {
      await api('/api/logs', { method: 'PUT', body: { enabled: !enabled } });
      toast(enabled ? 'Capture disabled' : 'Capture enabled', 'Runtime only: a restart goes back to the -access-log flag.', 'ok');
      if (c.isConnected) await route();
    } catch (e) { toastErr(e, enabled ? 'Could not disable capture' : 'Could not enable capture'); btn.disabled = false; }
  });
}

// ---------- HISTORY ----------
// showDiff fetches what one commit changed ({to}) or what changed between two
// ({from, to}) and renders it in the wide modal: a file summary with +/-
// counts, then the unified diff with added, removed and hunk lines coloured.
async function showDiff(q) {
  let d;
  try {
    const qs = new URLSearchParams();
    qs.set('to', q.to);
    if (q.from) qs.set('from', q.from);
    d = (await api('/api/history/diff?' + qs.toString())).data || {};
  } catch (e) { toastErr(e, 'Could not load the changes'); return; }
  const files = arr(d.files);
  const title = q.from ? `Changes from ${shortSha(q.from)} to ${shortSha(q.to)}` : `Changes in ${shortSha(q.to)}`;
  const summary = files.length
    ? `<ul class="diff-files">${files.map((f) => `<li><span class="chip ${f.status === 'added' ? 'ok' : f.status === 'deleted' ? 'err' : ''}">${esc(f.status)}</span><span class="mono trunc" title="${esc(f.path)}">${esc(f.oldPath ? f.oldPath + ' -> ' + f.path : f.path)}</span><span class="mono diff-count"><span class="diff-add">+${f.additions}</span> <span class="diff-del">-${f.deletions}</span></span></li>`).join('')}</ul>`
    : '<p class="muted">No file changed between these two points.</p>';
  const lines = String(d.patch || '').split('\n').map((l) => {
    let cls = '';
    if (l.startsWith('diff --git ')) cls = 'diff-file';
    else if (l.startsWith('@@')) cls = 'diff-hunk';
    else if (l.startsWith('+++') || l.startsWith('---') || l.startsWith('index ') || l.startsWith('new file') || l.startsWith('deleted file') || l.startsWith('rename ') || l.startsWith('similarity ')) cls = 'diff-meta';
    else if (l.startsWith('+')) cls = 'diff-add';
    else if (l.startsWith('-')) cls = 'diff-del';
    return `<span class="${cls}">${esc(l)}</span>`;
  }).join('\n');
  const body = summary
    + (files.length ? `<pre class="diff">${lines}</pre>` : '')
    + (d.truncated ? '<p class="hint warn">The patch was cut at 2 MiB; the file list above is complete.</p>' : '');
  await confirmModal({ title, body, wide: true, closeOnly: true, confirmLabel: 'Close', danger: false, keepOnNavigate: false });
}

async function viewHistory(c) {
  const items = arr((await api('/api/history')).data);
  // GET /api/history/diff is admin scope (the patch carries whole object
  // files, api-token digests included), so the read-only role gets no diff
  // controls rather than buttons that 403.
  const canDiff = !isRoleReadOnly();
  const HIST_FIELDS = { author: 'commit author', kind: 'object kind of a single-object change', name: 'object name', action: 'update, delete, import, restore, revert or settings', hash: 'commit hash' };
  const histRecs = items.map((h) => {
    const obj = parseObjectCommit(h.message);
    const action = obj ? 'update' : ((/^(delete|import|restore|revert|settings)/i.exec(historyText(h.message) || '') || [''])[0].toLowerCase());
    return {
      blob: [h.message, historyText(h.message), h.author, h.email, h.hash].join(' ').toLowerCase(),
      f: { author: fvals(h.author, h.email), kind: fvals(obj && kindSingular(obj.kind)), name: fvals(obj && obj.name), action: fvals(action), hash: fvals(h.hash) },
    };
  });
  c.innerHTML = `
    ${pageHead('History', 'Every change is a git commit. Reviewable, attributable, and reversible.')}
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
      ${filterBoxHtml('histFilter', 'filter commits...', 'Filter history', HIST_FIELDS)}
    </div>` : ''}
    <div class="card">
      ${items.length ? `<div class="timeline">${items.map((h, i) => {
        const obj = parseObjectCommit(h.message);
        // Buttons, not spans: these commit a config change, so they have to be
        // tabbable and operable from the keyboard like every other action here.
        const scoped = obj ? `<button type="button" class="revert" data-revert-obj="${esc(h.hash)}" data-obj-plural="${esc(obj.plural)}" data-obj-name="${esc(obj.name)}" data-obj-kind="${esc(obj.kind)}" title="Revert only ${esc(kindSingular(obj.kind))} &quot;${esc(obj.name)}&quot; to this version; every other object is left untouched">Revert this object</button>` : '';
        const whole = i === 0
          ? '<span class="revert disabled" title="Already the current config">Current</span>'
          : `<button type="button" class="revert" data-revert="${esc(h.hash)}" title="Revert the entire config (every object) to this commit">Revert entire config</button>`;
        // Changes opens what this one commit did; the compare box picks two
        // commits for a diff between them. Both read the repo, so the "user"
        // role (no admin scope) does not get them.
        const changes = canDiff ? `<button type="button" class="revert" data-changes="${esc(h.hash)}" title="Every file this commit changed, as a diff">Changes</button>` : '';
        const pick = canDiff ? `<label class="check-item tl-pick" title="Pick two commits to compare"><input type="checkbox" class="tl-cmp" data-hash="${esc(h.hash)}" data-idx="${i}" aria-label="Compare ${esc(shortSha(h.hash))}" />Compare</label>` : '';
        return `<div class="tl-item" data-i="${i}">
          <div class="tl-meta">${esc(fmtTime(h.when))} &middot; <span${h.email ? ` title="${esc(h.email)}"` : ''}>${esc(h.author || 'unknown')}</span></div>
          <div class="tl-msg" title="${esc(h.message || '')}">${esc(historyText(h.message))}</div>
          <div class="tl-actions"><span class="sha">${esc(shortSha(h.hash))}</span>${changes}${scoped}${whole}${pick}</div>
        </div>`;
      }).join('')}<div class="list-empty" id="histNone" hidden>No commit matches this filter.</div></div>` : '<div class="muted" style="font-size:13.5px">No commits yet.</div>'}
    </div>`;

  // Compare bar: appears once two commits are ticked. The diff always runs
  // from the older commit to the newer one, whatever order they were ticked.
  const cmpBar = document.createElement('div');
  cmpBar.className = 'panel save-bar cmp-bar';
  cmpBar.hidden = true;
  cmpBar.innerHTML = `<div class="save-note" id="cmpNote"></div><div style="display:flex;gap:10px"><button class="btn primary" id="cmpGo" type="button">Compare</button><button class="btn ghost" id="cmpClear" type="button">Clear</button></div>`;
  c.appendChild(cmpBar);
  const picked = () => $$('.tl-cmp:checked').map((b) => ({ hash: b.dataset.hash, idx: Number(b.dataset.idx) }));
  const syncCmp = () => {
    const p = picked();
    // A third tick is refused rather than silently dropping one.
    $$('.tl-cmp').forEach((b) => { b.disabled = p.length >= 2 && !b.checked; });
    cmpBar.hidden = p.length === 0;
    $('#cmpNote').textContent = p.length === 2
      ? `Compare ${shortSha(p.find((x) => x.idx === Math.max(p[0].idx, p[1].idx)).hash)} to ${shortSha(p.find((x) => x.idx === Math.min(p[0].idx, p[1].idx)).hash)}`
      : 'Pick one more commit to compare.';
    $('#cmpGo').disabled = p.length !== 2;
  };
  $$('.tl-cmp').forEach((b) => b.addEventListener('change', syncCmp));
  $('#cmpClear').addEventListener('click', () => { $$('.tl-cmp').forEach((b) => { b.checked = false; }); syncCmp(); });
  $('#cmpGo').addEventListener('click', () => {
    const p = picked();
    if (p.length !== 2) return;
    const older = p[0].idx > p[1].idx ? p[0] : p[1];
    const newer = older === p[0] ? p[1] : p[0];
    showDiff({ from: older.hash, to: newer.hash });
  });
  c.querySelectorAll('[data-changes]').forEach((el) => {
    el.addEventListener('click', () => showDiff({ to: el.getAttribute('data-changes') }));
  });

  wireFilter('histFilter', HIST_FIELDS, () => fieldValues(histRecs), (terms) => {
    applyRowFilter($$('.timeline .tl-item[data-i]'), histRecs, terms, $('#histNone'));
    highlightTerms($('.timeline', c), terms);
  }, 'history');
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
      refreshHeadSha();
      if (c.isConnected) await route();
    } catch (e) { toastErr(e, 'Could not restore the archive'); } finally { fileInput.value = ''; }
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
        refreshHeadSha();
        if (c.isConnected) await route();
      } catch (e) { toastErr(e, 'Could not revert the config'); }
    });
  });
  c.querySelectorAll('[data-revert-obj]').forEach((el) => {
    el.addEventListener('click', async () => {
      const hash = el.getAttribute('data-revert-obj');
      const plural = el.getAttribute('data-obj-plural');
      const name = el.getAttribute('data-obj-name');
      const kind = kindSingular(el.getAttribute('data-obj-kind'));
      const ok = await confirmModal({
        title: `Revert this ${kind}?`,
        body: `<p>The ${esc(kind)} <b>${esc(name)}</b> is reset to how it was at <span class="sha mono">${esc(shortSha(hash))}</span>.</p>`
          + '<p>Every other object is left exactly as it is, and this is recorded as a new commit - so this page can undo it.</p>',
        confirmLabel: 'Revert this object',
        danger: false,
      });
      if (!ok) return;
      try {
        const r = await api('/api/' + plural + '/' + encodeURIComponent(name) + '/revert', { method: 'POST', body: { hash } });
        toast('Reverted', r.data && r.data.commit ? `${esc(capFirst(kind))} ${esc(name)}, committed <span class="sha">${esc(shortSha(r.data.commit))}</span>` : `${esc(capFirst(kind))} ${esc(name)} reverted`, 'ok', { html: true });
        refreshHeadSha();
        if (c.isConnected) await route();
      } catch (e) { toastErr(e, `Could not revert ${name}`); }
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
    'Custom HTML for the errors gpm itself generates. Applies to every host unless a host overrides it.')
    + `<div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">What these replace</p>
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
      <div class="save-note">${ICON.commit}Each save is recorded as one git commit.</div>
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
      catch (e) { showInvalid('Invalid error pages JSON', 'Inline templates must be valid JSON (status code or "default" -> HTML).', $('#errp-inline')); return; }
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
// Integrations is tabbed the same way: one rendered view, every tab's controls
// in the DOM at once, so any tab's Save sends the whole settings object.
const INTEGRATIONS_TABS = [
  { id: 'dns', label: 'DNS sync' },
  { id: 'ingress', label: 'Ingress discovery' },
  { id: 'docker', label: 'Docker discovery' },
  { id: 'access-sync', label: 'Access-list sync' },
  { id: 'webhooks', label: 'Webhooks' },
  { id: 'notifications', label: 'Notifications' },
];
function showIntegrationsTab(id) { showTabIn(INTEGRATIONS_TABS, id); }
function showSettingsTab(id) { showTabIn(SETTINGS_TABS, id); }
// showTabIn selects tab id among tabs: the band's buttons and the content's
// panels (both carry data-tab) flip together.
function showTabIn(tabs, id) {
  const known = tabs.some((t) => t.id === id) ? id : tabs[0].id;
  $$('#pageTabs .tab-btn').forEach((b) => {
    const on = b.dataset.tab === known;
    b.classList.toggle('on', on);
    b.setAttribute('aria-selected', on ? 'true' : 'false');
    // Roving tabindex: only the selected tab is a tab stop; the arrow keys
    // move between tabs.
    b.tabIndex = on ? 0 : -1;
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
      <p class="muted" style="font-size:13.5px;margin:0">Runtime details are not available to this token.</p></div>`;
  }
  const listeners = rt.listeners || {};
  const paths = rt.paths || {};
  const roots = arr(rt.secretFileRoots);
  const mono = (v) => `<span class="mono">${esc(v || '-')}</span>`;
  const word = (b, on, off) => `<span class="chip ${b ? 'ok' : ''}">${esc(b ? on : off)}</span>`;
  const row = (k, v, help) => `<span class="k">${esc(k)}</span><span class="v">${v}${help ? `<div class="hint">${help}</div>` : ''}</span>`;
  return `<div class="card form-section" id="set-runtime">
    <p class="section-label">Runtime</p>
    <div class="kv">
      ${row('Version', mono(rt.version || 'unknown'))}
      ${row('HA role', rt.haRole === 'follower' ? '<span class="chip warn">Follower (read-only)</span>' : '<span class="chip">Leader</span>')}
      ${row('HTTP listener', mono(listeners.http))}
      ${row('HTTPS listener', mono(listeners.https))}
      ${row('Admin listener', mono(listeners.admin))}
      ${row('Config directory', mono(paths.configDir), 'The git-backed config repo. This is what to back up.')}
      ${row('Certificate directory', mono(paths.certDir))}
      ${row('Session database', mono(paths.sessionDB))}
      ${row('Secret file roots', roots.length ? roots.map((r) => `<span class="chip mono">${esc(r)}</span>`).join(' ') : mono('none'), 'Allowlisted directories for <span class="mono">${FILE:...}</span> secrets (<span class="mono">GPM_SECRET_FILE_ROOTS</span>).')}
      ${row('Local admin sign-in', rt.localAdminConfigured ? word(true, 'Configured', '') : '<span class="chip warn">Not configured</span>',
    rt.localAdminConfigured ? '' : 'No local password login. Set <span class="mono">GPM_LOCAL_ADMIN_USER</span> and <span class="mono">GPM_LOCAL_ADMIN_PASSWORD_HASH_FILE</span> and restart.')}
      ${row('Local admin TOTP', word(!!rt.localAdminTOTP, 'On', 'Off'))}
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
    <div class="save-note">${ICON.commit}Saving any tab writes the whole settings object as one commit.</div>
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
  const ae = s.adminEdge || {};
  const aeTLS = ae.tls || {};
  const aeHost = (() => { try { return new URL(s.externalBaseURL || '').hostname; } catch (e) { return ''; } })();
  // What the fleet-wide switch leaves up, from the capability probe: the
  // admin edge and the hosts the admin sign-in providers live behind.
  const maintExempt = arr(state.capabilities && state.capabilities.maintenance && state.capabilities.maintenance.exempt);
  const idc = s.ingressDiscovery || {};
  const trusted = arr(s.trustedProxies);
  const stls = s.tls || {};

  c.innerHTML = `
    ${pageHead('Settings', 'What this instance is, who may administer it, and how it answers. Outbound systems are under <a href="#/integrations">Integrations</a>.', '', '', '', '',
      SETTINGS_TABS.map((t) => `<button class="tab-btn" type="button" role="tab" id="set-tab-${esc(t.id)}" aria-controls="set-panel-${esc(t.id)}" data-tab="${esc(t.id)}" aria-selected="false" tabindex="-1">${esc(t.label)}</button>`).join(''))}

    <div class="tab-panel" data-tab="general" id="set-panel-general" role="tabpanel" aria-labelledby="set-tab-general" hidden>
      ${runtimeCardHtml(rt)}
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Instance</p>
        <div class="field-group">
          <label>Instance name</label>
          <input class="field" id="set-appname" data-hint="settings.appName" data-path="appName" value="${esc(s.appName || '')}" placeholder="Go Proxy Manager" />
          <div class="hint">Shown in the masthead, the browser title and the login page.</div>
        </div>
        <div class="field-group">
          <label>Public URL of this panel</label>
          <input class="field mono" id="set-url" data-hint="settings.externalBaseURL" data-path="externalBaseURL" value="${esc(s.externalBaseURL || '')}" placeholder="https://gpm.example.com" />
          <div class="hint">The address operators reach this panel at. OIDC builds its <span class="mono">redirect_uri</span> from it, so admin sign-in through an identity provider does not work until it matches what the provider has registered.</div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Admin sign-in</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Local sign-in</div><div class="ds" id="set-local-status">Username/password fallback</div></div>${switchHtml('set-local', !!admin.localLoginEnabled, 'Local sign-in', 'settings.adminAuth.localLoginEnabled')}</div>
        <div class="field-group" style="margin-top:12px">
          <label>Single sign-on providers</label>
          <div class="check-list" id="set-providers" data-hint="settings.adminAuth.providers" data-path="adminAuth.providers">
            ${oidcIdps.length || unknownProvs.length ? oidcIdps.map((p) => `
              <label class="check-item"><input type="checkbox" value="${esc(p.name)}"${storedProvs.indexOf(p.name) !== -1 ? ' checked' : ''}/>${esc(p.name)}<span class="ci-ty">oidc</span></label>
            `).join('') + unknownProvs.map((n) => `
              <label class="check-item"><input type="checkbox" value="${esc(n)}" checked/>${esc(n)}<span class="ci-ty" title="Stored in settings but not an OIDC identity provider on this instance">unknown</span></label>
            `).join('') : '<div class="check-empty">No OIDC identity providers defined yet.</div>'}
          </div>
          <div class="hint">Only OIDC providers can sign in to this panel. Define them under <a href="#/identity">Identity Providers</a>.</div>
        </div>
        <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Require SSO for admins</div><div class="ds">All admin access goes through SSO</div></div>${switchHtml('set-sso', !!admin.ssoOnly, 'Require SSO for admins', 'settings.adminAuth.ssoOnly')}</div>
        <div class="hint" id="set-sso-hint" style="margin-top:8px">There is no in-band break-glass door: recovering from an identity-provider outage under SSO-only means redeploying with local sign-in re-enabled.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Admin front door</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Serve the panel at the edge</div><div class="ds">${aeHost ? `on <span class="mono">${esc(aeHost)}</span>` : 'set the public URL above first'}</div></div>${switchHtml('set-ae-on', !!ae.enabled, 'Serve the panel at the edge', 'settings.adminEdge.enabled')}</div>
        <div id="set-ae-body" style="margin-top:12px">
          <div class="field-group"><label>Access lists</label><div class="chip-input" id="set-ae-al" data-hint="settings.adminEdge.accessLists" data-path="adminEdge.accessLists"></div><div class="hint">Who may reach the panel at all. A LAN/VPN list is the usual choice; the panel's own sign-in sits behind it.</div></div>
          <div class="field-group"><label>Middlewares</label><div class="chip-input" id="set-ae-mw" data-hint="settings.adminEdge.middlewares" data-path="adminEdge.middlewares"></div><div class="hint">Optional, in order. An mTLS or rate-limit middleware is sensible; a sign-in middleware gates the panel a second time.</div></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect plain HTTP to HTTPS</div></div>${switchHtml('set-ae-forcessl', !!aeTLS.forceSSL, 'Force SSL', 'settings.adminEdge.tls.forceSSL')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Strict-Transport-Security, one year, subdomains included</div></div>${switchHtml('set-ae-hsts', !!(aeTLS.hsts && aeTLS.hsts.enabled), 'HSTS', 'settings.adminEdge.tls.hsts.enabled')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Hide from search engines</div><div class="ds">X-Robots-Tag: noindex, nofollow</div></div>${switchHtml('set-ae-robots', !!ae.robotsNoIndex, 'Hide from search engines', 'settings.adminEdge.robotsNoIndex')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">LAN record</div><div class="ds">DNS sync publishes a local record pointing straight at gpm</div></div>${switchHtml('set-ae-dns-lan', !!(ae.dns && ae.dns.lanDirect), 'LAN record', 'settings.adminEdge.dns.lanDirect')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Public CNAME</div><div class="ds">DNS sync publishes the public record</div></div>${switchHtml('set-ae-dns-pub', !!(ae.dns && ae.dns.publicCname), 'Public CNAME', 'settings.adminEdge.dns.publicCname')}</div>
          <div class="hint" style="margin-top:8px">Replaces a hand-written proxy host for this hostname: while this is on, no host may claim it (the save is refused), so delete that host first or right after. Other TLS settings (minimum version, client certificates) are kept as stored and editable in <span class="mono">settings.yaml</span>.</div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Client addresses</p>
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

    <div class="tab-panel" data-tab="headers" id="set-panel-headers" role="tabpanel" aria-labelledby="set-tab-headers" hidden>
      <div class="card form-section">
        <p class="section-label">Response security headers</p>
        <div id="set-secheaders" data-hint="settings.securityHeaders" data-path="securityHeaders"></div>
        <div style="display:flex;gap:8px;margin-top:6px;flex-wrap:wrap">
          <button class="btn ghost sm" id="set-secheaders-add" type="button">${ICON.plus}Add header</button>
          <button class="btn ghost sm" id="set-secheaders-recommend" type="button" title="Add the five headers gpm documents as a safe starting point. Nothing already set is overwritten.">Load recommended</button>
        </div>
        <div class="hint" style="margin-top:6px">Put app-breaking headers - Content-Security-Policy frame-ancestors, Permissions-Policy - at <span class="mono">generated-only</span>: they are safe on gpm's own pages and break a proxied app that ships none of its own. Strict-Transport-Security is refused here; HSTS is a per-host TLS setting.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Strip upstream response headers</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">Fleet-default list of header names removed from proxied upstream responses (<span class="mono">Server</span>, <span class="mono">X-Powered-By</span>, ...). Only what the upstream sent is touched - never a header gpm adds. Any proxy host can add its own names in its editor; the lists are unioned.</p>
        <div class="chip-input" id="set-striphdrs" data-hint="settings.stripResponseHeaders" data-path="stripResponseHeaders"></div>
        <div class="hint" style="margin-top:6px">Hop-by-hop and response-semantic names (Content-Type, Content-Length, Content-Encoding, Location, Vary, the WebSocket handshake) are refused on save. Set-Cookie and WWW-Authenticate are allowed but sharp: stripping them breaks the app's own sessions or its basic-auth challenge.</div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Error pages</p>
        <p class="muted" style="font-size:13.5px;margin:0">Custom HTML for gpm-generated errors has its own section: <a href="#/errorpages">Error pages</a>. It edits the same <span class="mono">settings.errorPages</span> config.</p>
      </div>
      ${settingsSaveBar('set-save-headers')}
    </div>

    <div class="tab-panel" data-tab="advanced" id="set-panel-advanced" role="tabpanel" aria-labelledby="set-tab-advanced" hidden>
      <div class="card form-section">
        <p class="section-label">PROXY protocol (inbound)</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">Read the real client address out of the HAProxy PROXY protocol header (v1 and v2) on the <span class="mono">:80</span>/<span class="mono">:443</span> listeners and every TCP stream listener, so gpm behind an L4 load balancer sees the client rather than the balancer. This is the L4 half of the same question the Client addresses card answers at L7. The header is an unauthenticated claim, so it is honoured <b>only</b> from the trusted peers below. UDP stream listeners are unaffected.</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Accept PROXY headers</div><div class="ds">From the trusted peers below only</div></div>${switchHtml('set-pp-on', !!pp.enabled, 'Accept PROXY headers', 'settings.proxyProtocol.enabled')}</div>
        <div class="grid-2" style="margin-top:10px">
          <div class="field-group"><label>Trusted peers</label><div class="chip-input" id="set-pp-cidrs" data-hint="settings.proxyProtocol.trustedCIDRs" data-path="proxyProtocol.trustedCIDRs"></div>
            <div class="hint">For example <span class="mono">10.0.0.0/8</span>. Required when enabled.</div>
          </div>
          <div class="field-group"><label>Header timeout</label><input class="field mono" id="set-pp-timeout" data-hint="settings.proxyProtocol.timeout" data-path="proxyProtocol.timeout" value="${esc(pp.timeout || '')}" placeholder="5s" />
            <div class="hint">Blank = 5s, maximum 1m.</div>
          </div>
        </div>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">Annotation and label prefix</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">One prefix per deployment, shared by Kubernetes Ingress discovery and Docker discovery: it names every annotation and label they read (<span class="mono">.../managed</span>, <span class="mono">.../profile</span>) and the <span class="mono">managed-by</span> / <span class="mono">disabled-by</span> labels gpm writes on derived hosts.</p>
        <div class="field-group" style="max-width:320px"><label>Prefix</label>
          <input class="field mono" id="set-id-annprefix" data-hint="settings.ingressDiscovery.annotationPrefix" data-path="ingressDiscovery.annotationPrefix" value="${esc(idc.annotationPrefix || '')}" placeholder="gpm.rake.pro" />
          <div class="hint">Leave blank for the default. Changing it does not relabel existing hosts by itself.</div>
        </div>
        <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Migrate existing hosts to the new prefix</div><div class="ds">Relabels hosts still under the old prefix in the next reconcile, in one commit</div></div>${switchHtml('set-id-annprefix-migrate', !!idc.annotationPrefixMigrate, 'Migrate existing hosts to the new prefix', 'settings.ingressDiscovery.annotationPrefixMigrate')}</div>
      </div>
      <div class="card form-section" id="set-metrics" style="margin-top:16px">
        <p class="section-label">Prometheus metrics</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">Opt-in scrape endpoint on this admin listener, enabled with <span class="mono">GPM_METRICS=1</span> (a flag, not a saved setting - it needs a restart). An API token with the <span class="mono">metrics:read</span> scope can scrape it; your admin session can open it directly.</p>
        <a class="btn ghost sm" id="set-metrics-link" href="/metrics" target="_blank" rel="noopener">Open /metrics</a>
      </div>
      <div class="card form-section" style="margin-top:16px">
        <p class="section-label">High availability</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">HA is decided at startup, not here. One instance is the leader and owns every write - config commits, ACME orders, DNS and discovery reconciles. A follower serves traffic from the replicated config and refuses config writes with a <span class="mono">503</span>, which is why its write controls are greyed out.</p>
        <div class="kv">
          <span class="k">This instance</span><span class="v">${(rt && rt.haRole === 'follower') ? '<span class="chip warn">Follower (read-only)</span>' : '<span class="chip">Leader</span>'}</span>
          <span class="k">Config repo</span><span class="v"><span class="mono">${esc((rt && rt.paths && rt.paths.configDir) || '-')}</span><div class="hint">Replicate this directory to the follower; it is the whole state.</div></span>
        </div>
      </div>
      ${settingsSaveBar('set-save-advanced')}
    </div>

    <div class="tab-panel" data-tab="operations" id="set-panel-operations" role="tabpanel" aria-labelledby="set-tab-operations" hidden>
      <div class="card form-section danger-zone">
        <p class="section-label">Danger zone</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">Two actions that affect every host or every signed-in user at once. Both are deliberate, both are confirmed, and neither has a partial version.</p>
      </div>
      <div class="card form-section danger-zone" style="margin-top:16px">
        <p class="section-label">Fleet-wide maintenance</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">Takes <b>every proxy host</b> out of service for a downtime window: gpm answers each request itself with a <span class="mono">503</span> and a <span class="mono">Retry-After</span> instead of dialling the upstream. It wins over each host's own Maintenance switch, so turning it off returns hosts to whatever they were individually set to. Redirect, parked and stream hosts proxy nothing and keep serving; ACME HTTP-01 is answered before host routing, so certificates still renew. The page served is the <a href="#/errorpages">error page</a> configured for <span class="mono">503</span> (a host's own override first), or gpm's built-in maintenance page when none is.</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Fleet-wide maintenance</div><div class="ds">${maintExempt.length ? `Every proxy host except ${maintExempt.map((d) => `<span class="mono">${esc(d)}</span>`).join(', ')} (this panel and its sign-in) serves the maintenance page` : 'All proxy hosts serve the maintenance page'}</div></div>${switchHtml('set-maint-on', !!maint.enabled, 'Fleet-wide maintenance', 'settings.maintenance.enabled')}</div>
        <div class="field-group" style="margin-top:10px;max-width:220px">
          <label>Retry-After (seconds)</label>
          <input class="field mono" id="set-maint-retry" data-hint="settings.maintenance.retryAfterSeconds" data-path="maintenance.retryAfterSeconds" type="number" min="0" max="86400" value="${esc(maint.retryAfterSeconds != null ? String(maint.retryAfterSeconds) : '')}" placeholder="300" />
          <div class="hint">Blank = 300s, maximum 24h.</div>
        </div>
      </div>
      <div class="card form-section danger-zone" style="margin-top:16px">
        <p class="section-label">Per-host SSO sessions</p>
        <p class="muted" style="font-size:13.5px;margin:0 0 10px">SSO cookies on proxied hosts are stateless with a 1-hour cap. Revoking invalidates every outstanding session immediately; users re-authenticate at the identity provider on their next request. This takes effect at once and is not part of a save.</p>
        <button class="btn danger-soft sm" id="set-sso-revoke" type="button">Revoke all SSO sessions</button>
      </div>
      ${settingsSaveBar('set-save-operations')}
    </div>`;

  showSettingsTab(tab || SETTINGS_TABS[0].id);
  // Each tab is a route (#/settings/<tab>), so a click sets the hash and
  // onHashChange shows the panel; the band's shared key handler does the rest.
  $$('#pageTabs .tab-btn').forEach((b) => {
    b.addEventListener('click', () => { location.hash = '#/settings/' + b.dataset.tab; });
  });
  wirePageTabKeys();

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

  const aeAlCtl = makeChipInput($('#set-ae-al'), arr(ae.accessLists), 'add access list...');
  const aeMwCtl = makeChipInput($('#set-ae-mw'), arr(ae.middlewares), 'add middleware...');
  // The edge needs a hostname to serve on; without a public URL the switch is
  // greyed with the reason, and the chain fields follow the switch.
  const refreshAdminEdge = () => {
    gateControl($('#set-ae-on'), !!aeHost || isOn('set-ae-on'), 'Set the public URL of this panel first.');
    gateControl($('#set-ae-body'), isOn('set-ae-on'), 'Turn the front door on to edit its chain.');
  };
  refreshAdminEdge();
  // A block that was never configured gets the sensible defaults the moment
  // the door is first switched on; a stored false (an omitted key) is kept.
  let aeDefaulted = !!s.adminEdge;
  $('#set-ae-on').addEventListener('switchchange', () => {
    if (isOn('set-ae-on') && !aeDefaulted) {
      aeDefaulted = true;
      ['set-ae-forcessl', 'set-ae-robots'].forEach((id) => $('#' + id).setAttribute('aria-checked', 'true'));
    }
    refreshAdminEdge();
  });
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
      body: '<p>Local sign-in is <b>disabled</b> for this admin panel. Every operator signs in through one of the providers above.</p>'
        + '<p>There is deliberately no in-band break-glass door. If the identity provider is unreachable, its configuration drifts, or the provider entry is removed, '
        + '<b>recovery is a redeploy</b> with local sign-in re-enabled - not a sign-in page.</p>',
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
      body: `<p><b>${plural(proxyHostCount, 'proxy host', 'proxy hosts')}</b> will stop reaching their upstreams and answer <span class="mono">503</span> with a <span class="mono">Retry-After</span> instead.</p>`
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
      added ? `${plural(added, 'header', 'headers')} added. Review them, then save.` : 'Every recommended header is already listed.', added ? 'ok' : '');
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
    } catch (e) { toastErr(e, 'Could not revoke sessions'); }
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
        showInvalid('Invalid trusted proxy', `"${badTp}" is not a CIDR or IP address. Use 10.0.0.0/8 or 192.0.2.10.`, $('#set-trustedproxies'));
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
      showInvalid('Retry-After out of range', 'Maintenance Retry-After must be between 0 and 86400 seconds (24h). Leave it blank for the 300s default.', $('#set-maint-retry'));
      return;
    }
    const maintenance = {};
    if (isOn('set-maint-on')) maintenance.enabled = true;
    if (!isNaN(maintRetry) && maintRetry > 0) maintenance.retryAfterSeconds = maintRetry;
    if (Object.keys(maintenance).length) body.maintenance = maintenance;

    // Admin edge (adminEdge). Rebuilt from the controls, with the TLS block
    // merged over the stored one so a minTLSVersion or clientAuth written in
    // settings.yaml survives a save here. Off with nothing else set leaves the
    // key off the body entirely.
    const adminEdge = {};
    if (isOn('set-ae-on')) adminEdge.enabled = true;
    const aeAls = aeAlCtl.get(), aeMws = aeMwCtl.get();
    if (aeAls.length) adminEdge.accessLists = aeAls;
    if (aeMws.length) adminEdge.middlewares = aeMws;
    const aeTls = Object.assign({}, ae.tls, { forceSSL: isOn('set-ae-forcessl') });
    if (isOn('set-ae-hsts')) aeTls.hsts = Object.assign({ maxAge: 31536000, includeSubdomains: true }, ae.tls && ae.tls.hsts, { enabled: true });
    else delete aeTls.hsts;
    if (!aeTls.forceSSL) delete aeTls.forceSSL;
    if (Object.keys(aeTls).length) adminEdge.tls = aeTls;
    if (isOn('set-ae-robots')) adminEdge.robotsNoIndex = true;
    if (isOn('set-ae-dns-lan') || isOn('set-ae-dns-pub')) adminEdge.dns = { lanDirect: isOn('set-ae-dns-lan'), publicCname: isOn('set-ae-dns-pub') };
    if (Object.keys(adminEdge).length) body.adminEdge = adminEdge;

    const ppCidrs = ppCtl.get();
    const ppTimeout = $('#set-pp-timeout').value.trim();
    if (isOn('set-pp-on') && !ppCidrs.length) {
      showInvalid('Trusted peers required', 'A PROXY header from an untrusted peer would let any client spoof its source IP. Add the CIDRs of your load balancers.', $('#set-pp-cidrs'));
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
    if (secHdrErr) { showInvalid('Invalid security header', secHdrErr, $('#set-secheaders .row-bad') || $('#set-secheaders')); return; }
    const secHdrs = secHdrCtl.get();
    if (secHdrs) body.securityHeaders = secHdrs;

    // stripResponseHeaders (the fleet default list of headers removed from
    // proxied upstream responses) is owned by the chip editor above, which
    // replaced the old carry-forward guard. An empty list leaves the key off
    // the body entirely rather than committing an empty array.
    const stripHdrs = stripCtl.get();
    const stripErr = stripHeaderListError(stripHdrs);
    if (stripErr) { showInvalid('Invalid strip header', stripErr, $('#set-striphdrs')); return; }
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
    // Saved list filters are edited from each list's Saved menu; carried forward.
    if (arr(s.savedFilters).length) body.savedFilters = s.savedFilters;

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
        const inst = $('#userMenu .m-head .mono'); if (inst) inst.textContent = state.instance;
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
    } catch (e) {
      // This page carries webhooks and DNS sync forward untouched and has no
      // field for their secrets, so say where the refused literal is fixed.
      const lit = e && e.message && e.message.indexOf('refusing to commit literal secret') !== -1;
      showSaveError(lit ? new Error(e.message + '. Webhook, notification and DNS sync secrets are edited on the Integrations page (or in git), not here.') : e, 'Could not save settings');
    }
    btn.disabled = false;
  }
}

// keep topbar config sha fresh after writes
async function refreshHeadSha() {
  try {
    const h = (await api('/api/history')).data;
    if (Array.isArray(h) && h.length) {
      state.headSha = h[0].hash;
      const sc = $('#sideCommit');
      if (sc) sc.textContent = shortSha(h[0].hash);
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
    'API tokens are not available in this build.');
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


// ---------- wizard ----------
// PatternFly-style creation wizard: a dark title band, a numbered step list on
// the left where a finished step summarizes what was chosen and jumps back,
// one concern per step, Next / Back / Cancel in the footer. Every step reads
// and writes one draft object; the primitive owns the frame, the focus trap,
// the discard guard and the error line. Pure DOM, the same patterns as
// confirmModal(). Resolves with onFinish's result, or null when dismissed.
//
//   opts.steps[i] = { id, title, render(pane, draft) -> ctl | Promise<ctl>,
//                     validate(draft) -> '' | message, summary(draft) -> html }
//   ctl.read(draft) copies the pane's controls into the draft.
//   opts.onFinish(draft) performs the write; a thrown error shows in the footer.
//   opts.importYAML(text) -> { draft, invalid, note, full } fills the steps
//   from pasted YAML (see openHostWizard for the shape).
//   opts.altLink = { label, href, carry(draft)? } is the footer's way out to
//   the full editor; carry, when set, receives the committed draft.
function wizard(opts) {
  return new Promise((resolve) => {
    const steps = opts.steps;
    let draft = opts.draft;
    let cur = 0;
    let ctl = null;
    let doneUpTo = -1;
    let dirty = false;
    let closed = false;
    let prompting = false;
    // The page the wizard opened over: a declined navigation prompt puts the
    // hash back here, and the hashchange that puts it back is swallowed.
    const openHash = location.hash;
    let restoringHash = false;
    const prevFocus = document.activeElement;
    const wrap = document.createElement('div');
    wrap.className = 'modal-scrim wz-scrim';
    wrap.innerHTML = `
      <div class="modal wizard" role="dialog" aria-modal="true" aria-labelledby="wz-title">
        <div class="wz-head"><div><h2 id="wz-title">${esc(opts.title)}</h2>${opts.desc ? `<p class="desc">${esc(opts.desc)}</p>` : ''}</div>
          <button class="tool-btn" id="wz-close" type="button" aria-label="Close">${ICON.x}</button></div>
        <div class="wz-body"><nav class="wz-nav" aria-label="Steps"><ol id="wz-steps"></ol></nav><div class="wz-main" id="wz-main"></div></div>
        <div class="wz-foot">
          <button class="btn primary" id="wz-next" type="button">Next</button>
          <button class="btn ghost" id="wz-back" type="button">Back</button>
          <button class="btn ghost" id="wz-cancel" type="button">Cancel</button>
          <span class="spacer"></span>
          ${opts.importYAML ? '<a href="#" id="wz-import">Import from YAML</a>' : ''}
          ${opts.altLink ? `<a href="${esc(opts.altLink.href)}" id="wz-alt">${esc(opts.altLink.label)}</a>` : ''}
          <div class="wz-err" id="wz-err" role="alert"></div>
        </div>
      </div>`;
    document.body.appendChild(wrap);
    const main = wrap.querySelector('#wz-main');
    const errEl = wrap.querySelector('#wz-err');
    const nextBtn = wrap.querySelector('#wz-next');
    const backBtn = wrap.querySelector('#wz-back');
    // Typing clears a stale validation message; it comes back on the next Next.
    const mark = () => { dirty = true; setErr(''); };
    wrap.addEventListener('input', mark);
    wrap.addEventListener('change', mark);
    wrap.addEventListener('switchchange', mark);
    function setErr(msg) { errEl.textContent = msg || ''; }
    function renderNav() {
      wrap.querySelector('#wz-steps').innerHTML = steps.map((st, i) => {
        const done = i <= doneUpTo && i !== cur;
        const cls = i === cur ? 'cur' : (done ? 'done' : '');
        const sum = done && st.summary ? st.summary(draft) : '';
        const tag = done ? 'button' : 'div';
        return `<li><${tag} class="wz-step ${cls}"${done ? ` type="button" data-step="${i}" title="Back to ${esc(st.title)}"` : ''}${i === cur ? ' aria-current="step"' : ''}>`
          + `<span class="n" aria-hidden="true">${done ? ICON.check : String(i + 1)}</span><span>${esc(st.title)}</span>${sum ? `<span class="sum">${sum}</span>` : ''}</${tag}></li>`;
      }).join('');
      wrap.querySelectorAll('.wz-step[data-step]').forEach((b) => b.addEventListener('click', () => go(parseInt(b.dataset.step, 10))));
    }
    async function show(i) {
      cur = i;
      setErr('');
      renderNav();
      main.innerHTML = '';
      ctl = null;
      nextBtn.hidden = false;
      backBtn.hidden = i === 0;
      nextBtn.textContent = i === steps.length - 1 ? (opts.finishLabel || 'Add') : 'Next';
      const r = steps[i].render(main, draft);
      const got = (r && typeof r.then === 'function') ? await r : r;
      if (closed || cur !== i) return;
      ctl = got || null;
      decorateHints(main);
      const f = main.querySelector('input:not([type=hidden]):not([disabled]), select, textarea');
      if (f) f.focus(); else nextBtn.focus();
    }
    function commit() { if (ctl && ctl.read) ctl.read(draft); }
    // Moving forward validates the step; moving back keeps its edits as they are.
    function go(i) {
      commit();
      if (i < cur) { show(i); return; }
      const err = steps[cur].validate ? steps[cur].validate(draft) : '';
      if (err) { setErr(err); return; }
      if (cur > doneUpTo) doneUpTo = cur;
      show(i);
    }
    // Every step is validated again before the write, whatever route led
    // here (an import lands on Review without walking the steps); the first
    // failing step is shown with its message.
    async function finish() {
      commit();
      for (let i = 0; i < steps.length; i++) {
        const err = steps[i].validate ? steps[i].validate(draft) : '';
        if (err) { if (i !== cur) await show(i); setErr(err); return; }
      }
      nextBtn.disabled = true;
      setErr('');
      try {
        const out = await opts.onFinish(draft);
        close(out === undefined ? true : out);
      } catch (e) {
        setErr(e && e.message ? e.message : String(e));
        nextBtn.disabled = false;
      }
    }
    // navigating: the page underneath changed, so the prompt speaks of leaving
    // and a decline returns to the page the wizard opened on.
    async function cancel(navigating) {
      if (dirty) {
        prompting = true;
        let leave = false;
        try {
          leave = await confirmModal(navigating === true
            ? { title: `Discard this ${opts.noun || 'draft'}?`, body: '<p>Nothing has been created yet. Leaving this page throws the steps away.</p>', confirmLabel: 'Discard and leave', keepOnNavigate: true }
            : { title: `Discard this ${opts.noun || 'draft'}?`, body: '<p>Nothing has been created yet. Closing throws the steps away.</p>', confirmLabel: 'Discard' });
        } finally { prompting = false; }
        if (!leave) {
          // Declined: the wizard stays over the page it opened on, whichever
          // navigation happened while the prompt was up.
          if (location.hash !== openHash) { restoringHash = true; location.hash = openHash; }
          return;
        }
      }
      close(null);
    }
    function close(v) {
      if (closed) return;
      closed = true;
      document.removeEventListener('keydown', onKey, true);
      window.removeEventListener('hashchange', onHash);
      wrap.remove();
      restoreFocus(prevFocus);
      resolve(v);
    }
    // The page underneath navigated (browser Back, a link in a step): the
    // wizard does not belong over another page. Clean drafts just close;
    // a dirty one asks first and stays if the operator declines.
    function onHash() {
      if (closed) return;
      if (restoringHash) { if (location.hash === openHash) restoringHash = false; return; }
      if (!dirty) { close(null); return; }
      if (prompting) return; // the open prompt answers this navigation too
      cancel(true);
    }
    function onKey(e) {
      if (prompting) return; // the discard prompt on top owns the keys
      if (e.key === 'Escape') {
        e.preventDefault();
        // A help popover open inside the wizard closes first, on its own.
        const hp = document.getElementById('hint-pop');
        if (hp && !hp.hidden) { e.stopPropagation(); dismissHintPop(); return; }
        cancel();
        return;
      }
      if (e.key === 'Tab') {
        const f = Array.from(wrap.querySelectorAll('a[href], button:not([disabled]):not([hidden]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex="0"]'))
          .filter((el) => el.offsetParent !== null);
        if (!f.length) return;
        const first = f[0], last = f[f.length - 1];
        if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      }
    }
    // The import pane replaces the current step until the YAML is used or the
    // operator goes back; the step list stays as it was.
    function showImport() {
      commit();
      setErr('');
      ctl = null;
      nextBtn.hidden = true;
      backBtn.hidden = true;
      main.innerHTML = `<h3>Import from YAML</h3>
        <p class="lead">Paste one ${esc(opts.noun || 'object')} document. The steps fill in from it.</p>
        <div class="field-group"><label for="wz-yaml">YAML</label><textarea class="field mono" id="wz-yaml" rows="14" spellcheck="false" placeholder="${esc(opts.importPlaceholder || '')}"></textarea></div>
        <div style="display:flex;gap:8px;margin-top:12px;flex-wrap:wrap"><button class="btn primary" id="wz-parse" type="button">Use this YAML</button><button class="btn link" id="wz-import-back" type="button">Back to the steps</button></div>
        <div id="wz-import-note"></div>`;
      const ta = main.querySelector('#wz-yaml');
      ta.focus();
      main.querySelector('#wz-import-back').addEventListener('click', () => show(cur));
      main.querySelector('#wz-parse').addEventListener('click', async () => {
        const btn = main.querySelector('#wz-parse');
        btn.disabled = true;
        setErr('');
        let r;
        try { r = await opts.importYAML(ta.value); }
        catch (e) { setErr(e && e.message ? e.message : String(e)); btn.disabled = false; return; }
        if (closed) return;
        draft = r.draft;
        dirty = true;
        // Walk the imported draft through every step's own checks: the first
        // step that objects is shown with its message, otherwise Review.
        const landing = () => {
          for (let i = 0; i < steps.length - 1; i++) {
            const err = steps[i].validate ? steps[i].validate(draft) : '';
            if (err) { doneUpTo = i - 1; return show(i).then(() => setErr(err)); }
          }
          doneUpTo = steps.length - 2;
          return show(steps.length - 1);
        };
        if (r.note) {
          const note = main.querySelector('#wz-import-note');
          note.innerHTML = `<div class="ro-banner warn" style="margin:16px 0 0"><b>Partly covered.</b> ${esc(r.note)}</div>
            <div style="display:flex;gap:8px;flex-wrap:wrap"><button class="btn primary" id="wz-import-go" type="button">Continue in the wizard</button>${r.full ? '<button class="btn ghost" id="wz-import-full" type="button">Open the full editor with everything</button>' : ''}</div>`;
          note.querySelector('#wz-import-go').addEventListener('click', landing);
          const full = note.querySelector('#wz-import-full');
          if (full) full.addEventListener('click', () => { close(null); r.full(); });
          btn.disabled = false;
          return;
        }
        await landing();
      });
    }
    document.addEventListener('keydown', onKey, true);
    window.addEventListener('hashchange', onHash);
    wrap.querySelector('#wz-close').addEventListener('click', () => cancel());
    wrap.querySelector('#wz-cancel').addEventListener('click', () => cancel());
    backBtn.addEventListener('click', () => go(cur - 1));
    nextBtn.addEventListener('click', () => { if (cur === steps.length - 1) finish(); else go(cur + 1); });
    const imp = wrap.querySelector('#wz-import');
    if (imp) imp.addEventListener('click', (e) => { e.preventDefault(); showImport(); });
    // The alternative link hands the draft over when the caller can take it
    // (altLink.carry): the current step is committed first and nothing is
    // lost, so there is no discard prompt. Without carry it is a plain exit.
    const alt = wrap.querySelector('#wz-alt');
    if (alt) alt.addEventListener('click', (e) => {
      if (!opts.altLink.carry) { close(null); return; }
      e.preventDefault();
      commit();
      close(null);
      opts.altLink.carry(draft);
    });
    show(0);
  });
}

// Light YAML colouring for the Review step: keys only, on escaped text.
function yamlHtml(text) {
  return esc(text).split('\n').map((line) => line.replace(/^(\s*(?:- )?)([A-Za-z0-9_.-]+)(:)(?=\s|$)/, '$1<span class="k">$2</span>$3')).join('\n');
}
function copyText(text, btn) {
  const done = () => { const t = btn.textContent; btn.textContent = 'Copied'; setTimeout(() => { btn.textContent = t; }, 1200); };
  if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).then(done, () => {});
}

// ---------- new proxy host wizard ----------
// Four steps: Service, Certificate, Access, Review. The Review step asks the
// server for a dry run (PUT ?dryRun=1), so what it prints is the exact YAML a
// Create commits, validated by the same code. Everything the wizard does not
// ask about keeps its default and is reachable in the editor afterwards; the
// full editor stays one click away for the cases the steps do not cover.
const HOST_NAME_RE = /^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$/;
function hostNameFrom(domain) {
  // "*.example.com" names the host after the zone, not after the star.
  const d = String(domain || '').trim().toLowerCase().replace(/^\*\./, '');
  const label = d.split('.')[0].replace(/[^a-z0-9-]/g, '');
  return label.replace(/^-+|-+$/g, '');
}
function hostWizardObject(d) {
  const o = { name: d.name, domains: d.domains.slice(), upstream: { scheme: d.scheme, host: d.host, port: parseInt(d.port, 10) } };
  if (d.path) o.upstream.path = d.path;
  const tls = {};
  if (d.forceSSL) tls.forceSSL = true;
  if (d.hsts) tls.hsts = { enabled: true };
  if (Object.keys(tls).length) o.tls = tls;
  if (d.idp) o.auth = { identityProvider: d.idp };
  if (d.accessLists.length) o.accessLists = d.accessLists.slice();
  if (d.middlewares.length) o.middlewares = d.middlewares.slice();
  return o;
}
let hostWizardBusy = false;
async function openHostWizard() {
  if (hostWizardBusy) return null; // a double click must not stack two wizards
  hostWizardBusy = true;
  try { return await runHostWizard(); } finally { hostWizardBusy = false; }
}
async function runHostWizard() {
  const [certsR, idpsR, alsR, mwsR, hostsR] = await Promise.all([
    refList('/api/certificates', 'certificates'),
    refList('/api/identity-providers', 'identity providers'),
    refList('/api/access-lists', 'access lists'),
    refList('/api/middlewares', 'middlewares'),
    refList('/api/proxy-hosts', 'proxy hosts'),
  ]);
  const certs = arr(certsR), idps = arr(idpsR), als = arr(alsR), mws = arr(mwsR);
  const listsDown = [certsR, idpsR, alsR, mwsR, hostsR].some((x) => x === null);
  // PUT is an upsert, so a name that already exists would silently replace
  // that host. The wizard only ever creates; the editor is where edits go.
  const existing = new Set(arr(hostsR).map((h) => h.name));
  // forceSSLTouched: Force SSL follows the domains' coverage (on only when
  // every domain is covered) until the operator flips the switch.
  // nameTouched: the name follows the first domain until the name field
  // itself is typed in (a Back and re-render does not count as typing).
  const draft = { name: '', nameTouched: false, domains: [], scheme: 'http', host: '', port: '', path: '', forceSSL: null, forceSSLTouched: false, hsts: false, idp: '', accessLists: [], middlewares: [] };
  const allCovered = (d) => d.domains.length > 0 && d.domains.every((x) => certForDomain(certs, x));
  // One coverage phrase for the step list and Review: which certificate covers
  // the host, or how many of its domains are covered when only some are.
  const coverText = (d) => {
    const cov = d.domains.map((x) => certForDomain(certs, x));
    const n = cov.filter(Boolean).length;
    if (!n) return 'no certificate yet';
    if (n < d.domains.length) return `${n} of ${d.domains.length} domains covered`;
    const names = Array.from(new Set(cov.map((ct) => ct.name)));
    return `covered by <span class="mono">${names.map(esc).join(', ')}</span>`;
  };
  let reviewErr = '';
  let reviewSeq = 0;

  const steps = [
    {
      id: 'service', title: 'Service',
      render(pane, d) {
        pane.innerHTML = `<h3>Service</h3><p class="lead">The public name and the upstream that answers it.</p>
          <div class="field-group"><label>Domains</label><div class="chip-input" id="wz-domains" data-hint="proxyHost.domains"></div>
            <div class="hint">Press Enter after each domain. The first one names the host.</div></div>
          <div class="field-group"><label>Name</label><input class="field mono" id="wz-name" value="${esc(d.name)}" placeholder="app" autocomplete="off" spellcheck="false" data-auto="${d.nameTouched ? '0' : '1'}" data-hint="common.name" />
            <div class="hint">Internal name and config file name, fixed after creation. Taken from the first domain; lowercase letters, digits, dots, - and _.</div></div>
          <div class="inline-fields">
            <div class="field-group" style="flex:0 0 130px"><label>Scheme</label><select class="field mono" id="wz-scheme" data-hint="proxyHost.upstream.scheme"><option value="http"${d.scheme !== 'https' ? ' selected' : ''}>http</option><option value="https"${d.scheme === 'https' ? ' selected' : ''}>https</option></select></div>
            <div class="field-group"><label>Host</label><input class="field mono" id="wz-host" value="${esc(d.host)}" placeholder="10.0.0.5" autocomplete="off" spellcheck="false" data-hint="proxyHost.upstream.host" /></div>
            <div class="field-group" style="flex:0 0 120px"><label>Port</label><input class="field mono" id="wz-port" value="${esc(d.port)}" placeholder="3000" inputmode="numeric" autocomplete="off" data-hint="proxyHost.upstream.port" /></div>
          </div>
          <div class="hint">http is right for most apps on your LAN; gpm terminates TLS in front of it.</div>
          ${foldHtml('wz-more-routing', 'More routing', d.path ? 'base path ' + d.path : 'base path none, host header kept, timeouts default', !!d.path,
            `<div class="field-group"><label>Base path</label><input class="field mono" id="wz-path" value="${esc(d.path)}" placeholder="/" autocomplete="off" spellcheck="false" data-hint="proxyHost.upstream.path" />
              <div class="hint">Prefixed to every request sent to the upstream. Empty forwards the path unchanged. Host header, timeouts and path overrides are in the editor after creation.</div></div>`)}`;
        const nameEl = pane.querySelector('#wz-name');
        nameEl.addEventListener('input', () => { nameEl.dataset.auto = '0'; d.nameTouched = true; });
        const chips = makeChipInput(pane.querySelector('#wz-domains'), d.domains, 'add domain...', (list) => {
          if (nameEl.dataset.auto === '1') nameEl.value = hostNameFrom(list[0]);
        });
        return {
          read(dd) {
            dd.domains = chips.get().map((x) => String(x).trim().toLowerCase()).filter(Boolean);
            // A changed domain list re-derives the Force SSL default, even
            // when the Certificate step is not shown again.
            if (!dd.forceSSLTouched) dd.forceSSL = allCovered(dd);
            dd.name = nameEl.value.trim();
            dd.nameTouched = nameEl.dataset.auto === '0';
            dd.scheme = pane.querySelector('#wz-scheme').value;
            dd.host = pane.querySelector('#wz-host').value.trim();
            dd.port = pane.querySelector('#wz-port').value.trim();
            dd.path = pane.querySelector('#wz-path').value.trim();
          },
        };
      },
      validate(d) {
        if (!d.domains.length) return 'Add at least one domain.';
        const bad = d.domains.find((x) => !/^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/.test(x));
        if (bad) {
          return /[^\x00-\x7f]/.test(bad)
            ? `"${bad}" has non-ASCII characters; enter its punycode (xn--) form.`
            : `"${bad}" is not a valid domain name.`;
        }
        if (!d.name) return 'Give the host a name.';
        if (d.name.length > 253) return 'The name is too long (253 characters at most).';
        if (!HOST_NAME_RE.test(d.name)) return 'The name must be lowercase letters, digits, dots, - or _, and start and end with a letter or digit.';
        if (existing.has(d.name)) return `A proxy host named "${d.name}" already exists. Pick another name, or edit that host from the list.`;
        if (!d.host) return 'Enter the upstream host or IP address.';
        const port = parseInt(d.port, 10);
        if (isNaN(port) || port < 1 || port > 65535 || String(port) !== d.port) return 'Enter a port between 1 and 65535.';
        if (d.path && d.path[0] !== '/') return 'A base path must start with "/".';
        return '';
      },
      summary(d) {
        const more = d.domains.length > 1 ? ` +${d.domains.length - 1}` : '';
        return `${esc(d.domains[0])}${more} to <span class="mono">${esc(d.scheme)}://${esc(d.host)}:${esc(d.port)}${esc(d.path)}</span>`;
      },
    },
    {
      id: 'cert', title: 'Certificate',
      render(pane, d) {
        const covered = d.domains.map((x) => certForDomain(certs, x));
        const nCovered = covered.filter(Boolean).length;
        const every = allCovered(d);
        if (!d.forceSSLTouched) d.forceSSL = every;
        const sslWhy = every
          ? (d.domains.length > 1 ? 'On because a certificate covers every domain.' : 'On because a certificate covers the domain.')
          : (nCovered
            ? `Off because only ${nCovered} of ${d.domains.length} domains are covered, so visitors to the others would be sent to a handshake that fails.`
            : 'Off until a certificate covers the domain, so visitors are not sent to a handshake that fails.');
        pane.innerHTML = `<h3>Certificate</h3><p class="lead">How ${esc(d.domains[0])} is served over HTTPS.</p>
          <div class="field-group"><label>Coverage</label>${certCoverageHtml(certs, d.domains)}
            <div class="hint">${CERT_AUTO_HINT}</div></div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Send http:// visitors to https://. ${sslWhy}</div></div>${switchHtml('wz-forcessl', !!d.forceSSL, 'Force SSL', 'proxyHost.tls.forceSSL')}</div>
          <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Tell browsers to use HTTPS only. Turn it on once HTTPS works for good; it is hard to undo for visitors.</div></div>${switchHtml('wz-hsts', !!d.hsts, 'HSTS', 'proxyHost.tls.hsts.enabled')}</div>
          <p class="hint" style="margin-top:12px">Minimum TLS version and client certificates (mTLS) are in the host editor after creation.</p>`;
        const sslSw = pane.querySelector('#wz-forcessl');
        if (sslSw) sslSw.addEventListener('switchchange', () => { d.forceSSLTouched = true; });
        return { read(dd) { dd.forceSSL = isOn('wz-forcessl'); dd.hsts = isOn('wz-hsts'); } };
      },
      validate(d) {
        if (d.hsts && !d.forceSSL) return 'HSTS needs Force SSL on, or browsers are told to use HTTPS on a host that still answers plain HTTP.';
        return '';
      },
      summary(d) {
        return `${coverText(d)}, Force SSL ${d.forceSSL ? 'on' : 'off'}${d.hsts ? ', HSTS on' : ''}`;
      },
    },
    {
      id: 'access', title: 'Access',
      render(pane, d) {
        const alHtml = als.length
          ? `<div class="check-list" id="wz-als" data-hint="proxyHost.accessLists">${als.map((a) => `<label class="check-item"><input type="checkbox" value="${esc(a.name)}"${d.accessLists.indexOf(a.name) !== -1 ? ' checked' : ''}/>${esc(a.name)}${a.displayName ? `<span class="ci-ty">${esc(a.displayName)}</span>` : ''}</label>`).join('')}</div>`
          : `<div class="hint">No access lists yet. Add one under <a href="#/access">Access Lists</a> to allow or deny by network or country.</div>`;
        const mwHtml = mws.length
          ? `<div class="check-list" id="wz-mws">${mws.map((m) => `<label class="check-item"><input type="checkbox" value="${esc(m.name)}"${d.middlewares.indexOf(m.name) !== -1 ? ' checked' : ''}/>${esc(m.name)}<span class="ci-ty">${esc(MW_TYPE_LABEL[m.type] || m.type || '')}</span></label>`).join('')}</div>`
          : `<div class="hint">No middleware yet. Rate limits, headers, rewrites and reusable sign-in gates live under <a href="#/middleware">Middleware</a>.</div>`;
        pane.innerHTML = `<h3>Access</h3><p class="lead">Who gets through, and how they prove it.</p>
          <div class="field-group"><label>Sign-in</label><select class="field" id="wz-idp" data-hint="proxyHost.auth"><option value="">None, the app has its own login</option>${idps.map((i) => `<option value="${esc(i.name)}"${d.idp === i.name ? ' selected' : ''}>${esc(i.name)}${i.type ? ' (' + esc(i.type) + ')' : ''}</option>`).join('')}</select>
            <div class="hint">${idps.length ? 'Visitors sign in at the identity provider before gpm forwards the request. Required roles, the mode and exempt networks are in the editor.' : 'No identity providers yet. Add one under <a href="#/identity">Identity Providers</a> to gate hosts behind a sign-in.'}</div></div>
          <div class="field-group"><label>Access lists</label>${alHtml}${als.length ? '<div class="hint">Evaluated before sign-in. A request denied here never reaches the identity provider.</div>' : ''}</div>
          ${foldHtml('wz-mw', 'Middleware', d.middlewares.length ? d.middlewares.join(', ') : 'none', !!d.middlewares.length, `<div class="field-group" data-hint="proxyHost.middlewares"><label>Attach</label>${mwHtml}</div>`)}`;
        return {
          read(dd) {
            dd.idp = pane.querySelector('#wz-idp').value;
            dd.accessLists = Array.from(pane.querySelectorAll('#wz-als input:checked')).map((i) => i.value);
            dd.middlewares = Array.from(pane.querySelectorAll('#wz-mws input:checked')).map((i) => i.value);
          },
        };
      },
      validate() { return ''; },
      summary(d) {
        const parts = [d.idp ? `sign-in ${esc(d.idp)}` : 'no sign-in'];
        if (d.accessLists.length) parts.push(`access lists ${d.accessLists.map(esc).join(', ')}`);
        if (d.middlewares.length) parts.push(`${d.middlewares.length} middleware`);
        return parts.join('; ');
      },
    },
    {
      id: 'review', title: 'Review',
      async render(pane, d) {
        reviewErr = '';
        const tok = ++reviewSeq; // a reply from an earlier render is dropped
        pane.innerHTML = `<h3>Review</h3><p class="lead">What gets committed to git when you add the host.</p>
          <dl class="dl-h" style="margin-bottom:20px">
            <dt>Domains</dt><dd class="mono">${d.domains.map(esc).join(', ')}</dd>
            <dt>Name</dt><dd class="mono">${esc(d.name)}</dd>
            <dt>Upstream</dt><dd class="mono">${esc(d.scheme)}://${esc(d.host)}:${esc(d.port)}${esc(d.path)}</dd>
            <dt>Certificate</dt><dd>${coverText(d)}, Force SSL ${d.forceSSL ? 'on' : 'off'}${d.hsts ? ', HSTS on' : ''}</dd>
            <dt>Sign-in</dt><dd>${d.idp ? esc(d.idp) : 'none'}</dd>
            <dt>Access lists</dt><dd>${d.accessLists.length ? `<span class="mono">${d.accessLists.map(esc).join(', ')}</span>` : 'none'}</dd>
            <dt>Middleware</dt><dd>${d.middlewares.length ? `<span class="mono">${d.middlewares.map(esc).join(', ')}</span>` : 'none'}</dd>
            <dt>Everything else</dt><dd>defaults</dd>
          </dl>
          <div class="loading" id="wz-dry"><span class="spinner"></span>Checking with the server...</div>`;
        try {
          const r = await api('/api/proxy-hosts/' + encodeURIComponent(d.name) + '?dryRun=1', { method: 'PUT', body: hostWizardObject(d) });
          if (tok !== reviewSeq || !pane.isConnected) return null;
          const y = r.data || {};
          const slot = pane.querySelector('#wz-dry');
          if (!slot) return null;
          if (y.exists) {
            // Checked on the server, not only against the list loaded when
            // the wizard opened: Create must never replace a stored host.
            reviewErr = `A proxy host named "${d.name}" already exists. Pick another name on the Service step, or edit that host from the list.`;
            slot.outerHTML = `<div class="inline-error"><b>This name is taken.</b><br/>${esc(reviewErr)}</div>`;
            return null;
          }
          // The dry run stamps its own createdAt/updatedAt, which Add restamps;
          // they are left out so every line shown is the line committed.
          const yaml = String(y.yaml || '').split('\n').filter((l) => !/^(createdAt|updatedAt):/.test(l)).join('\n');
          slot.outerHTML = `<div class="code-head"><span>${esc(y.path || '')}</span><button class="btn plain sm" type="button" id="wz-copy">Copy</button></div><pre class="code">${yamlHtml(yaml)}</pre>
            <p class="hint">createdAt and updatedAt are stamped when you add the host.</p>`;
          pane.querySelector('#wz-copy').addEventListener('click', (e) => copyText(yaml, e.currentTarget));
        } catch (e) {
          if (tok !== reviewSeq || !pane.isConnected) return null;
          reviewErr = (e && e.message) || String(e);
          const slot = pane.querySelector('#wz-dry');
          if (slot) slot.outerHTML = `<div class="inline-error"><b>The server rejected this host.</b><br/>${esc(reviewErr)}</div>`;
        }
        return null;
      },
      validate() { return reviewErr ? 'Fix the rejected step first: ' + reviewErr : ''; },
    },
  ];

  const r = await wizard({
    title: 'New proxy host',
    desc: 'Publish an upstream on a public name.',
    noun: 'host',
    steps, draft,
    finishLabel: 'Add proxy host',
    // The full editor opens on what the steps hold so far, through the same
    // clone-seed hand-over as "Open the full editor with everything".
    altLink: {
      label: 'Open the full editor instead', href: '#/hosts/_new',
      carry: (d) => {
        const o = hostWizardObject(d);
        if (!o.upstream.host) delete o.upstream.host;
        if (isNaN(o.upstream.port)) delete o.upstream.port;
        cloneSeed = { section: 'hosts', origName: '', data: o };
        location.hash = '#/hosts/_new';
      },
    },
    importPlaceholder: 'name: app\ndomains: [app.example.com]\nupstream: { scheme: http, host: 10.0.0.5, port: 3000 }',
    importYAML: async (text) => {
      if (!String(text).trim()) throw new Error('Paste a YAML document first.');
      const res = await api('/api/proxy-hosts/_parse', { method: 'POST', raw: text, contentType: 'application/yaml' });
      const data = res.data || {};
      const o = data.object || {};
      const up = o.upstream || {}, tls = o.tls || {}, auth = o.auth || {};
      const nd = {
        name: data.name || o.name || '', nameTouched: !!(data.name || o.name), domains: arr(o.domains), scheme: up.scheme || 'http', host: up.host || '',
        port: up.port != null ? String(up.port) : '', path: up.path || '', forceSSL: !!tls.forceSSL, forceSSLTouched: true,
        hsts: !!(tls.hsts && tls.hsts.enabled), idp: auth.identityProvider || '',
        accessLists: arr(o.accessLists), middlewares: arr(o.middlewares),
      };
      // Everything the steps do not cover is listed, so an import never
      // silently drops a field; the full editor keeps all of them.
      const empty = (v) => v == null || v === false || v === '' || v === 0
        || (Array.isArray(v) ? !v.length : (typeof v === 'object' && Object.keys(v).every((k) => empty(v[k]))));
      const skip = ['name', 'domains', 'upstream', 'tls', 'auth', 'accessLists', 'middlewares', 'createdAt', 'updatedAt', 'websocketsUpgrade', 'labels'];
      const extra = Object.keys(o).filter((k) => skip.indexOf(k) === -1 && !empty(o[k]));
      Object.keys(up).forEach((k) => { if (['scheme', 'host', 'port', 'path'].indexOf(k) === -1 && !empty(up[k])) extra.push('upstream.' + k); });
      Object.keys(tls).forEach((k) => { if (['forceSSL', 'hsts'].indexOf(k) === -1 && !empty(tls[k])) extra.push('tls.' + k); });
      if (tls.hsts) Object.keys(tls.hsts).forEach((k) => { if (k !== 'enabled' && !empty(tls.hsts[k])) extra.push('tls.hsts.' + k); });
      Object.keys(auth).forEach((k) => { if (k !== 'identityProvider' && !empty(auth[k])) extra.push('auth.' + k); });
      const note = extra.length ? `The steps cover the basics; this YAML also sets ${extra.join(', ')}, which the wizard would leave out.` : '';
      return {
        draft: nd,
        invalid: data.valid === false ? (data.error || 'The server could not validate this object.') : '',
        note,
        // The editor opens on the pasted object minus what a hand-made host
        // must not carry: ownership labels (discovery would delete the host as
        // its own on the next pass) and the pasted timestamps.
        full: () => {
          const seed = Object.assign({}, o);
          delete seed.labels; delete seed.createdAt; delete seed.updatedAt;
          cloneSeed = { section: 'hosts', origName: '', data: seed };
          location.hash = '#/hosts/_new';
        },
      };
    },
    onFinish: async (d) => {
      // Create-only: the server answers 409 for a stored name instead of
      // replacing it, whatever this wizard believed when it opened.
      let res;
      try {
        res = await api('/api/proxy-hosts/' + encodeURIComponent(d.name), { method: 'PUT', body: hostWizardObject(d), headers: { 'If-None-Match': '*' } });
      } catch (e) {
        if (e && e.status === 409) throw new Error(`A proxy host named "${d.name}" was created in the meantime. Pick another name on the Service step.`);
        throw e;
      }
      toastSaved(res.commit, 'Added');
      refreshHeadSha();
      location.hash = '#/hosts/' + encodeURIComponent(d.name);
      return true;
    },
  });
  if (r === null && listsDown) toast('Some lists did not load', 'Certificates, identity providers, access lists or middleware could not be read; reload and try again.', 'err');
  return r;
}
// Any "Add proxy host" control (the list toolbar, the empty state) opens the
// wizard; its href stays the full editor for middle-click and no-script cases.
document.addEventListener('click', (e) => {
  const b = e.target.closest && e.target.closest('[data-wizard="hosts"]');
  if (!b || e.defaultPrevented || e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return;
  e.preventDefault();
  openHostWizard();
});


// ---------- first-time setup wizard ----------
// Offered on the Overview while the instance is fresh (no proxy host, no
// certificate) and reachable from the account menu afterwards. Three short
// steps on the same primitive as the host wizard: what this instance is
// called and where it is reached, which proxies in front of it to believe for
// client addresses, then a review. It writes settings only (the full object as
// loaded, with these fields overlaid: PUT /api/settings replaces the whole
// thing), and hands over to the proxy host wizard when it finishes.
const SETUP_DONE_KEY = 'gpm.setup.done';
function setupDismissed() {
  try { return localStorage.getItem(SETUP_DONE_KEY) === '1'; } catch (e) { return false; }
}
function markSetupDone() {
  try { localStorage.setItem(SETUP_DONE_KEY, '1'); } catch (e) { /* ignore */ }
}
function setupOffered() {
  if (isRoleReadOnly() || hasCapability('ha.readOnly') || setupDismissed()) return false;
  const c = state.counts || {};
  return !(c.proxyHosts || 0) && !(c.certificates || 0);
}
let setupWizardBusy = false;
async function openSetupWizard() {
  if (setupWizardBusy) return null;
  setupWizardBusy = true;
  try {
    // The setup can end on the Proxy Hosts page, so unsaved edits on the page
    // underneath are settled first, not by a prompt hidden under the wizard.
    if (dirtyFlag && !(await confirmModal({
      title: 'You have unsaved changes',
      body: '<p>Changes on this page are not committed yet. The setup can end on the Proxy Hosts page, which discards them.</p>',
      confirmLabel: 'Discard and continue',
    }))) return null;
    return await runSetupWizard();
  } finally { setupWizardBusy = false; }
}
// Resolves after the next route render: true when it rendered hash.
function nextRouteRendered(hash) {
  return new Promise((resolve) => {
    const on = (e) => { window.removeEventListener('gpm:routed', on); resolve(!!(e.detail && e.detail.ok && e.detail.hash === hash)); };
    window.addEventListener('gpm:routed', on);
  });
}
async function runSetupWizard() {
  const [setR, rtR] = await Promise.all([
    api('/api/settings').then((r) => r.data || {}).catch(() => null),
    api('/api/runtime').then((r) => r.data || {}).catch(() => ({})),
  ]);
  if (!setR) { toast('Could not load settings', 'Reload the page and try again.', 'err'); return null; }
  const rt = rtR || {};
  const ls = rt.listeners || {};
  const draft = {
    appName: setR.appName || '',
    externalBaseURL: setR.externalBaseURL || '',
    trustedProxies: arr(setR.trustedProxies),
  };
  // The hand-off to the host wizard is for a first service only: an instance
  // that already has a proxy host stays on the page it is on.
  const hostCount = async () => {
    try { return arr((await api('/api/proxy-hosts')).data).length; } catch (e) { return -1; }
  };
  const firstHost = (await hostCount()) === 0;
  const steps = [
    {
      id: 'instance', title: 'Instance',
      render(pane, d) {
        pane.innerHTML = `<h3>Instance</h3><p class="lead">A few facts about this instance, and what to call it.</p>
          <dl class="dl-h" style="margin-bottom:20px">
            <dt>Proxy listeners</dt><dd class="mono">${esc([ls.https, ls.http].filter(Boolean).join('  ') || 'not reported')}</dd>
            <dt>Admin panel</dt><dd class="mono">${esc(ls.admin || 'not reported')}</dd>
            ${rt.paths && rt.paths.configDir ? `<dt>Config repository</dt><dd class="mono">${esc(rt.paths.configDir)}</dd>` : ''}
            <dt>Version</dt><dd class="mono">${esc(rt.version || (state.version && state.version.version) || '')}</dd>
          </dl>
          <div class="field-group"><label>Instance name</label><input class="field" id="su-name" value="${esc(d.appName)}" placeholder="Go Proxy Manager" autocomplete="off" data-hint="settings.appName" />
            <div class="hint">Shown in the masthead, the browser title and the login page.</div></div>
          <div class="field-group"><label>Public URL of this panel</label><input class="field mono" id="su-url" value="${esc(d.externalBaseURL)}" placeholder="https://gpm.example.com" autocomplete="off" spellcheck="false" data-hint="settings.externalBaseURL" />
            <div class="hint">The address operators reach this panel at. Single sign-on builds its redirect from it, and an https:// URL marks the session cookie Secure. Leave it empty while you only use an IP address.</div></div>`;
        return { read(dd) { dd.appName = pane.querySelector('#su-name').value.trim(); dd.externalBaseURL = pane.querySelector('#su-url').value.trim(); } };
      },
      validate(d) {
        if (d.externalBaseURL) {
          let u = null;
          try { u = new URL(d.externalBaseURL); } catch (e) { u = null; }
          if (!u || (u.protocol !== 'http:' && u.protocol !== 'https:') || !u.host) return 'The public URL must be an absolute http:// or https:// address, or empty.';
        }
        return '';
      },
      summary(d) { return `${esc(d.appName || 'Go Proxy Manager')}${d.externalBaseURL ? `, <span class="mono">${esc(d.externalBaseURL)}</span>` : ''}`; },
    },
    {
      id: 'network', title: 'Client addresses',
      render(pane, d) {
        pane.innerHTML = `<h3>Client addresses</h3><p class="lead">Which proxies in front of gpm may say who the client is.</p>
          <div class="field-group"><label>Trusted proxies</label><div class="chip-input" id="su-tp" data-hint="settings.trustedProxies"></div>
            <div class="hint">CIDRs or addresses of load balancers or tunnels that sit in front of gpm. A request from one of them has its client address read from <span class="mono">X-Forwarded-For</span>; from anyone else the connection address is the client. Leave it empty when gpm faces the internet directly, which is the usual homelab setup.</div></div>
          <div class="hint warn" id="su-tp-warn"${hasWildcardProxy(d.trustedProxies) ? '' : ' hidden'}>${esc(TRUSTED_WILDCARD_WARNING)}</div>
          <p class="hint">Access lists, geo rules, rate limits and the access log all judge that one address, so a wrong entry here lets a visitor choose their own.</p>`;
        const warn = pane.querySelector('#su-tp-warn');
        const chips = makeChipInput(pane.querySelector('#su-tp'), d.trustedProxies, 'add CIDR or address...', (list) => { warn.hidden = !hasWildcardProxy(list); });
        return { read(dd) { dd.trustedProxies = chips.get().map((x) => String(x).trim()).filter(Boolean); } };
      },
      validate(d) {
        const bad = firstBadCidr(d.trustedProxies);
        if (bad) return `"${bad}" is not a CIDR or IP address. Use 10.0.0.0/8 or 192.0.2.10.`;
        // A wildcard is warned about, not refused, exactly as Settings does.
        return '';
      },
      summary(d) { return d.trustedProxies.length ? `trusts ${d.trustedProxies.map(esc).join(', ')}` : 'gpm faces the internet directly'; },
    },
    {
      id: 'review', title: 'Review',
      render(pane, d) {
        pane.innerHTML = `<h3>Review</h3><p class="lead">Saved to settings as one git commit.</p>
          <dl class="dl-h" style="margin-bottom:20px">
            <dt>Instance name</dt><dd>${esc(d.appName || 'Go Proxy Manager')}</dd>
            <dt>Public URL</dt><dd class="mono">${esc(d.externalBaseURL || 'none yet')}</dd>
            <dt>Trusted proxies</dt><dd>${d.trustedProxies.length ? `<span class="mono">${d.trustedProxies.map(esc).join(', ')}</span>` : 'none, the connection address is the client'}</dd>
          </dl>
          ${hasWildcardProxy(d.trustedProxies) ? `<div class="hint warn" style="margin-bottom:12px">${esc(TRUSTED_WILDCARD_WARNING)}</div>` : ''}
          ${firstHost
            ? '<p>Next: the Proxy Hosts page opens with the host wizard, to publish your first service. Certificates are issued under Certificates (Let\'s Encrypt over HTTP-01 or DNS-01), and an identity provider for sign-in goes under Identity Providers; both can wait until the host works.</p>'
            : '<p>This instance already has proxy hosts, so the setup ends here. The same fields are under Settings &gt; General.</p>'}`;
        return null;
      },
      validate() { return ''; },
    },
  ];
  const r = await wizard({
    title: 'Set up this instance',
    desc: 'Three steps; everything here can be changed later under Settings.',
    noun: 'setup',
    steps, draft,
    finishLabel: 'Finish setup',
    onFinish: async (d) => {
      const body = Object.assign({}, setR);
      if (d.appName) body.appName = d.appName; else delete body.appName;
      body.externalBaseURL = d.externalBaseURL;
      if (d.trustedProxies.length) body.trustedProxies = d.trustedProxies; else delete body.trustedProxies;
      const res = await api('/api/settings', { method: 'PUT', body });
      resetRouteMemo();
      toastSaved(res.commit); // "No changes" when the settings already held these values
      refreshHeadSha();
      markSetupDone();
      state.appName = d.appName || 'Go Proxy Manager';
      const nm = document.querySelector('.wordmark .name');
      if (nm) nm.textContent = state.appName;
      document.title = state.appName;
      if (d.externalBaseURL) {
        try { state.instance = new URL(d.externalBaseURL).host || d.externalBaseURL; } catch (e) { state.instance = d.externalBaseURL; }
        const inst = $('#userMenu .m-head .mono'); if (inst) inst.textContent = state.instance;
      }
      return true;
    },
  });
  // Hand over to the host wizard only on an instance with no proxy host yet,
  // and only once the Proxy Hosts page has rendered (the hash change would
  // otherwise close a wizard opened before it lands).
  if (r && firstHost && (await hostCount()) === 0) {
    if (location.hash === '#/hosts') { openHostWizard(); return r; }
    clearDirty(); // the operator agreed to discard before the setup opened
    const rendered = nextRouteRendered('#/hosts');
    location.hash = '#/hosts';
    if (await rendered) openHostWizard();
  }
  return r;
}
// The Overview card for a fresh instance.
function setupCardHtml() {
  return `<div class="card" id="setup-card" style="margin-bottom:16px">
    <h3>Welcome to ${esc(state.appName)}</h3>
    <p class="muted" style="margin:4px 0 14px">Nothing is published yet. The setup takes three short steps and ends in the host wizard; everything it asks is editable later under Settings.</p>
    <div style="display:flex;gap:8px;flex-wrap:wrap"><button class="btn primary" id="setup-start" type="button">Set up this instance</button><button class="btn link" id="setup-skip" type="button">Skip for now</button></div>
  </div>`;
}
function wireSetupCard(c) {
  const start = c.querySelector('#setup-start');
  if (start) start.addEventListener('click', openSetupWizard);
  const skip = c.querySelector('#setup-skip');
  if (skip) skip.addEventListener('click', () => { markSetupDone(); const card = c.querySelector('#setup-card'); if (card) card.remove(); });
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
