// go-proxy-manager admin SPA. Dependency-free vanilla ES module.

// ---------- icons ----------
const ICON = {
  arrow: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h16M13 6l7 6-7 6"/></svg>',
  grid: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>',
  globe: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3a9 9 0 100 18 9 9 0 000-18z"/><path d="M3 12h18M12 3c2.5 2.5 2.5 15 0 18M12 3c-2.5 2.5-2.5 15 0 18"/></svg>',
  redirect: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 7h11a4 4 0 014 4v0a4 4 0 01-4 4H4"/><path d="M8 3 4 7l4 4"/></svg>',
  stream: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 8h18M3 16h18M7 4v16M17 4v16"/></svg>',
  skull: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3a8 8 0 00-5 14v3h10v-3a8 8 0 00-5-14z"/><circle cx="9" cy="11" r="1.4"/><circle cx="15" cy="11" r="1.4"/></svg>',
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
};

// ---------- nav ----------
const NAV = [
  { id: 'overview', label: 'Overview', icon: ICON.grid },
  { id: 'hosts', label: 'Proxy Hosts', icon: ICON.globe },
  { id: 'redirects', label: 'Redirects', icon: ICON.redirect },
  { id: 'streams', label: 'Streams', icon: ICON.stream },
  { id: 'dead', label: 'Dead hosts', icon: ICON.skull },
  { id: 'certs', label: 'Certificates', icon: ICON.cert },
  { id: 'identity', label: 'Identity', icon: ICON.user },
  { id: 'access', label: 'Access Lists', icon: ICON.shield },
  { id: 'middleware', label: 'Middleware', icon: ICON.layers },
  { id: 'dns', label: 'DNS Providers', icon: ICON.globe },
  { id: 'logs', label: 'Access Logs', icon: ICON.history },
  { id: 'history', label: 'History', icon: ICON.history },
  { id: 'settings', label: 'Settings', icon: ICON.cog },
];

const TITLES = {
  overview: 'Overview', hosts: 'Proxy Hosts', redirects: 'Redirects', streams: 'Streams',
  dead: 'Dead hosts', certs: 'Certificates', identity: 'Identity', access: 'Access Lists',
  middleware: 'Middleware', dns: 'DNS Providers', history: 'History', settings: 'Settings',
};

// plural API paths per section
const PLURAL = {
  hosts: 'proxy-hosts', redirects: 'redirect-hosts', streams: 'stream-hosts',
  dead: 'dead-hosts', certs: 'certificates', identity: 'identity-providers',
  access: 'access-lists', middleware: 'middlewares', dns: 'dns-providers',
};

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
function arr(v) { return Array.isArray(v) ? v : []; }
function $(sel, root) { return (root || document).querySelector(sel); }
function $$(sel, root) { return Array.from((root || document).querySelectorAll(sel)); }

// ---------- api ----------
let csrfToken = '';

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
  if (res.status === 401) {
    location.href = '/auth/login?return=' + encodeURIComponent(location.pathname + location.hash);
    throw new Error('Unauthorized');
  }
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

// ---------- shell ----------
const state = { me: null, version: null, headSha: null, instance: 'go-proxy-manager', appName: 'Go Proxy Manager', capabilities: null };

// GET /api/capabilities probe (e.g. { geoip: { dbLoaded } }), cached on state the
// same way loadTopbar caches /api/me and /api/settings. Call from anywhere that
// needs a capability check; the network fetch only happens once.
async function loadCapabilities() {
  if (state.capabilities) return state.capabilities;
  try { state.capabilities = (await api('/api/capabilities')).data || {}; }
  catch (e) { state.capabilities = {}; }
  return state.capabilities;
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
          ${NAV.map((n) => `<button class="nav-item" data-view="${n.id}">${n.icon}${esc(n.label)}</button>`).join('')}
        </nav>
        <div class="sidebar-footer">
          <span class="dot live" aria-hidden="true"></span>
          data plane: live
        </div>
      </aside>
      <div class="main">
        <header class="topbar">
          <button class="menu-btn" id="menuBtn" aria-label="Open navigation">${ICON.menu}</button>
          <h1 class="page-title" id="pageTitle">${esc(state.appName)}</h1>
          <div class="spacer"></div>
          <div class="ident" id="ident"></div>
        </header>
        <main class="content" id="content"></main>
      </div>
    </div>`;

  $$('#nav .nav-item').forEach((b) => {
    b.addEventListener('click', () => { location.hash = '#/' + b.dataset.view; closeNav(); });
  });
  $('#menuBtn').addEventListener('click', () => document.body.classList.add('nav-open'));
  $('#scrim').addEventListener('click', closeNav);
}
function closeNav() { document.body.classList.remove('nav-open'); }

async function loadTopbar() {
  // best-effort, each independent
  const ident = $('#ident');
  let verStr = '', cfgSha = '', principal = '';
  try {
    const v = (await api('/version')).data;
    state.version = v;
    if (v) verStr = `${v.version || ''}${v.commit ? ' · ' + shortSha(v.commit) : ''}`;
  } catch (e) { /* ignore */ }
  try {
    const h = (await api('/api/history')).data;
    if (Array.isArray(h) && h.length) { state.headSha = h[0].hash; cfgSha = shortSha(h[0].hash); }
  } catch (e) { /* ignore */ }
  try {
    const me = (await api('/api/me')).data;
    state.me = me;
    if (me) {
      csrfToken = me.csrfToken || '';
      const name = me.Name || me.Subject || me.Email || 'user';
      const role = me.Role || '';
      const idp = me.IdP || '';
      principal = `<b>${esc(name)}</b>${role ? ' · ' + esc(role) : ''}${idp ? ' · via ' + esc(idp) : ''}`;
      state.avatarChar = (name[0] || '?').toLowerCase();
    }
  } catch (e) { /* ignore */ }
  try {
    await loadCapabilities();
  } catch (e) { /* ignore */ }
  try {
    const s = (await api('/api/settings')).data;
    if (s && s.externalBaseURL) {
      try { state.instance = new URL(s.externalBaseURL).host || s.externalBaseURL; }
      catch (e) { state.instance = s.externalBaseURL; }
    }
    if (s && s.appName) {
      state.appName = s.appName;
      const nm = document.querySelector('.wordmark .name');
      if (nm) nm.textContent = s.appName;
      document.title = s.appName + ' admin';
    }
  } catch (e) { /* ignore */ }

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
function switchHtml(id, checked, label) {
  return `<button class="switch" type="button" role="switch" id="${id}" aria-checked="${checked ? 'true' : 'false'}"${label ? ` aria-label="${esc(label)}"` : ''}></button>`;
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
  if (!available) el.title = reason; else el.removeAttribute('title');
  el.querySelectorAll('input, select, button').forEach((child) => { child.disabled = !available; });
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
    <div><h2>${esc(title)}</h2><p>${esc(sub)}</p></div>
    ${actionHtml || ''}
  </div>`;
}

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
function makeChipInput(container, initial, placeholder) {
  const tokens = (initial || []).slice();
  function render() {
    container.innerHTML = tokens.map((t, i) =>
      `<span class="chip-tok">${esc(t)} <button type="button" aria-label="Remove ${esc(t)}" data-i="${i}">${esc('×')}</button></span>`
    ).join('') + `<input class="mono" placeholder="${esc(placeholder || 'add...')}" aria-label="${esc(placeholder || 'add')}" />`;
    const input = container.querySelector('input');
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ',') {
        e.preventDefault();
        const v = input.value.trim().replace(/,$/, '');
        if (v && tokens.indexOf(v) === -1) { tokens.push(v); render(); container.querySelector('input').focus(); }
      } else if (e.key === 'Backspace' && !input.value && tokens.length) {
        tokens.pop(); render(); container.querySelector('input').focus();
      }
    });
    container.querySelectorAll('.chip-tok button').forEach((b) => {
      b.addEventListener('click', () => { tokens.splice(parseInt(b.dataset.i, 10), 1); render(); });
    });
  }
  render();
  return { get: () => tokens.slice() };
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
      case 'identity': await genericSection(c, 'identity', sub); break;
      case 'access': await genericSection(c, 'access', sub); break;
      case 'middleware': await genericSection(c, 'middleware', sub); break;
      case 'dns': await genericSection(c, 'dns', sub); break;
      case 'redirects': await genericSection(c, 'redirects', sub); break;
      case 'streams': await genericSection(c, 'streams', sub); break;
      case 'dead': await genericSection(c, 'dead', sub); break;
      case 'logs': await viewLogs(c); break;
      case 'history': await viewHistory(c); break;
      case 'settings': await viewSettings(c); break;
      default: location.hash = '#/overview'; return;
    }
    c.classList.add('view');
  } catch (e) {
    if (e && e.message === 'Unauthorized') return;
    c.innerHTML = inlineError(e && e.message ? e.message : String(e));
  }
}
function setActiveNav(section) {
  $$('#nav .nav-item').forEach((n) => n.classList.toggle('active', n.dataset.view === section));
}

// ---------- OVERVIEW ----------
async function viewOverview(c) {
  const cfg = (await api('/api/config')).data || {};
  let history = [];
  try { history = (await api('/api/history')).data || []; } catch (e) { /* shown as empty feed */ }

  const hosts = arr(cfg.proxyHosts);
  const liveHosts = hosts.filter((h) => !h.disabled).length;
  const certs = arr(cfg.certificates);
  const idps = arr(cfg.identityProviders);
  const idpSub = idps.length ? idps.map((p) => p.name).join(', ') : 'none configured';

  const feed = arr(history).slice(0, 6).map((h) => `
    <div class="feed-row">
      <span class="feed-tick"></span>
      <div class="feed-body">
        <div class="feed-meta">${esc(fmtTime(h.when))} · ${esc(h.author || 'unknown')}</div>
        <div class="feed-msg">${esc(h.message || '(no message)')}</div>
        <div class="feed-actions"><span class="sha">${esc(shortSha(h.hash))}</span></div>
      </div>
    </div>`).join('') || `<div class="muted" style="font-size:13px">No commits yet.</div>`;

  const certRows = certs.length ? certs.map((ct) => {
    const domains = arr(ct.domains).join(', ');
    const typ = ct.type === 'acme' ? 'ACME' : (ct.type === 'custom' ? 'Custom' : (ct.type || 'cert'));
    const detail = ct.type === 'acme'
      ? (ct.acme && ct.acme.dnsProvider ? `DNS-01 via ${ct.acme.dnsProvider}` : 'ACME')
      : 'Custom certificate';
    return `<div class="cert-row">
      <span class="cert-ico">${ICON.cert.replace('stroke="currentColor"', 'stroke="var(--ok)"')}</span>
      <div style="flex:1;min-width:0">
        <div class="host" style="font-size:14px">${esc(ct.name)} <span class="mono muted" style="font-weight:400;font-size:12px">${esc(domains)}</span></div>
        <div class="muted" style="font-size:11.5px">${esc(typ)} · ${esc(detail)}</div>
      </div>
    </div>`;
  }).join('') : `<div class="muted" style="font-size:13px">No certificates yet.</div>`;

  c.innerHTML = `
    <div class="view-head">
      <h2>Overview</h2>
      <p>Edge and gateway control plane status for <span class="mono">${esc(state.instance)}</span>.</p>
    </div>
    <div class="stat-grid">
      <div class="stat s-ok">
        <div class="k">Proxy hosts</div>
        <div class="v">${hosts.length}</div>
        <div class="sub"><b>${liveHosts}</b> live · ${hosts.length - liveHosts} disabled</div>
      </div>
      <div class="stat s-warn">
        <div class="k">Certificates</div>
        <div class="v">${certs.length}</div>
        <div class="sub"><b>${certs.filter((x) => x.type === 'acme').length}</b> ACME · ${certs.filter((x) => x.type === 'custom').length} custom</div>
      </div>
      <div class="stat s-cyan">
        <div class="k">Identity providers</div>
        <div class="v">${idps.length}</div>
        <div class="sub">${esc(idpSub)}</div>
      </div>
      <div class="stat">
        <div class="k">Data plane</div>
        <div class="v" style="color:var(--ok)">live</div>
        <div class="sub"><b>${arr(cfg.accessLists).length}</b> access lists · ${arr(cfg.middlewares).length} middleware</div>
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
        <p class="section-label">Certificates</p>
        ${certRows}
        <a class="btn ghost sm" href="#/certs" style="margin-top:14px;width:100%;justify-content:center">Manage certificates</a>
      </div>
    </div>`;
}

// ---------- PROXY HOSTS LIST ----------
async function listHosts(c) {
  const [hostsR, mwR] = await Promise.all([
    api('/api/proxy-hosts'),
    api('/api/middlewares').catch(() => ({ data: [] })),
  ]);
  const hosts = arr(hostsR.data);
  const mwType = {};
  arr(mwR.data).forEach((m) => { mwType[m.name] = m.type; });

  const head = viewHead('Proxy Hosts', 'Reverse-proxy entries routing public domains to internal upstreams.',
    `<a class="btn primary" href="#/hosts/_new">${ICON.plus}Add proxy host</a>`);

  if (!hosts.length) {
    c.innerHTML = head + emptyState('No proxy hosts yet', 'Add your first one to route a domain to an upstream.', 'Add proxy host', '#/hosts/_new');
    return;
  }

  const rows = hosts.map((h) => {
    const domains = arr(h.domains);
    const primary = domains[0] || h.name;
    const extra = domains.length > 1 ? ` +${domains.length - 1}` : '';
    const up = h.upstream || {};
    const upStr = `${up.host || '?'}:${up.port != null ? up.port : '?'}`;
    const cert = h.tls && h.tls.certificateRef;
    const tls = cert
      ? `<span class="lock ok">${ICON.lock}${esc(cert)}</span>`
      : `<span class="chip">none</span>`;
    const authMw = arr(h.middlewares).find((m) => mwType[m] === 'auth');
    const auth = authMw ? `<span class="chip brand">${esc(authMw)}</span>` : `<span class="chip">none</span>`;
    const status = h.disabled
      ? `<span class="chip"><span class="dot" style="background:var(--faint)"></span>disabled</span>`
      : `<span class="chip ok"><span class="dot ok"></span>live</span>`;
    const tagChips = arr(h.tags).map((t) => `<span class="chip" style="font-size:10px;padding:1px 6px">${esc(t)}</span>`).join(' ');
    return `<tr class="clickable" data-name="${esc(h.name)}">
      <td><span class="host">${esc(primary)}${esc(extra)}</span>${h.displayName ? `<div class="faint" style="font-size:11px">${esc(h.displayName)}</div>` : ''}${tagChips ? `<div style="margin-top:3px;display:flex;gap:4px;flex-wrap:wrap">${tagChips}</div>` : ''}</td>
      <td class="mono">${esc(upStr)}</td>
      <td>${tls}</td>
      <td>${auth}</td>
      <td>${status}</td>
    </tr>`;
  }).join('');

  c.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="hostFilter" placeholder="filter: domain, upstream, cert, tag..." aria-label="Filter hosts" /></div>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Domain</th><th>Upstream</th><th>TLS</th><th>Auth</th><th>Status</th></tr></thead>
        <tbody id="hostRows">${rows}</tbody>
      </table>
    </div>`;

  $$('#hostRows tr').forEach((tr) => {
    tr.addEventListener('click', () => { location.hash = '#/hosts/' + encodeURIComponent(tr.dataset.name); });
  });
  const filter = $('#hostFilter');
  filter.addEventListener('input', () => {
    const q = filter.value.toLowerCase();
    $$('#hostRows tr').forEach((tr) => {
      tr.style.display = tr.textContent.toLowerCase().indexOf(q) !== -1 ? '' : 'none';
    });
  });
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
  return ICON.layers;
}

function renderHostFlow(rootEl, ctx) {
  // ctx: { certRef, certDomains, upstreamStr, mwSelected:[name], mwType:{}, alSelected:[name] }
  const nodes = [];
  nodes.push(flowNode('', 'CLIENT', ':443', ICON.clientUser, true));
  if (ctx.certRef) {
    nodes.push(flowNode('TLS · termination', 'TLS termination', ctx.certDomains || ctx.certRef, ICON.lock, false));
  }
  ctx.mwSelected.forEach((m) => {
    const ty = ctx.mwType[m] || 'middleware';
    nodes.push(flowNode(`${ty}`, m, ty, mwIcon(ty), false));
  });
  ctx.alSelected.forEach((a) => {
    nodes.push(flowNode('access-list · ip', a, 'rules', ICON.list, false));
  });
  nodes.push(flowNode('', 'UPSTREAM', ctx.upstreamStr, ICON.server, true));
  rootEl.innerHTML = nodes.join(connector);
}

async function hostEditor(c, name) {
  const isNew = !name;
  const [certsR, mwR, alR, hostR] = await Promise.all([
    api('/api/certificates').catch(() => ({ data: [] })),
    api('/api/middlewares').catch(() => ({ data: [] })),
    api('/api/access-lists').catch(() => ({ data: [] })),
    isNew ? Promise.resolve({ data: {} }) : api('/api/proxy-hosts/' + encodeURIComponent(name)),
  ]);
  const certs = arr(certsR.data);
  const middlewares = arr(mwR.data);
  const accessLists = arr(alR.data);
  const mwType = {}; middlewares.forEach((m) => { mwType[m.name] = m.type; });
  const certDomains = {}; certs.forEach((ct) => { certDomains[ct.name] = arr(ct.domains).join(', '); });
  const h = hostR.data || {};
  const up = h.upstream || {};
  const tls = h.tls || {};
  const hsts = tls.hsts || {};

  const selMw = arr(h.middlewares);
  const selAl = arr(h.accessLists);

  const statusChip = h.disabled
    ? `<span class="chip"><span class="dot" style="background:var(--faint)"></span>disabled</span>`
    : `<span class="chip ok"><span class="dot ok"></span>${isNew ? 'new' : 'live'}</span>`;

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
      <div class="flow-head"><h3>Request flow</h3><span class="chip cyan">left to right · pipeline order</span></div>
      <div class="flow" id="flow"></div>
    </div>

    <div class="form-grid">
      <div class="stack">
        <div class="card form-section">
          <p class="section-label">Identity</p>
          <div class="inline-fields">
            <div class="field-group">
              <label>Name</label>
              <input class="field mono" id="f-name" value="${esc(h.name || '')}" ${isNew ? '' : 'disabled'} placeholder="internal-name" />
              <div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div>
            </div>
            <div class="field-group">
              <label>Display name</label>
              <input class="field" id="f-display" value="${esc(h.displayName || '')}" placeholder="optional label" />
            </div>
          </div>
          <div class="field-group" style="margin-top:10px">
            <label>Tags</label>
            <div class="chip-input" id="f-tags"></div>
            <div class="hint">Free-form labels for grouping and filtering. Press Enter to add.</div>
          </div>
          <div class="toggle-line" style="margin-top:6px">
            <div class="tl-text"><div class="nm">Disabled</div><div class="ds">Stop serving this host</div></div>
            ${switchHtml('f-disabled', !!h.disabled, 'Disabled')}
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Domains</p>
          <div class="chip-input" id="f-domains"></div>
          <div class="hint">Press Enter to add. At least one domain is required.</div>
        </div>

        <div class="card form-section">
          <p class="section-label">Upstream</p>
          <div class="inline-fields">
            <div class="field-group">
              <label>Scheme</label>
              <select class="field mono" id="f-scheme">
                <option value="http"${up.scheme === 'https' ? '' : ' selected'}>http</option>
                <option value="https"${up.scheme === 'https' ? ' selected' : ''}>https</option>
              </select>
            </div>
            <div class="field-group" style="flex:2">
              <label>Host</label>
              <input class="field mono" id="f-uphost" value="${esc(up.host || '')}" placeholder="10.0.0.5" />
            </div>
            <div class="field-group">
              <label>Port</label>
              <input class="field mono" id="f-upport" type="number" value="${esc(up.port != null ? up.port : '')}" placeholder="8080" />
            </div>
          </div>
          <div class="field-group" style="margin-top:14px">
            <div class="toggle-line">
              <div class="tl-text"><div class="nm">Websocket support</div><div class="ds">Pass through <span class="mono">Upgrade</span> / <span class="mono">Connection</span> headers</div></div>
              ${switchHtml('f-ws', !!h.websocketsUpgrade, 'Websocket support')}
            </div>
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Locations</p>
          <div id="f-locs"></div>
          <button class="btn ghost sm" id="addLoc" type="button" style="margin-top:6px">${ICON.plus}Add path override</button>
        </div>

        <div class="card form-section">
          <p class="section-label">Crawling &amp; timeouts</p>
          <div class="toggle-line">
            <div class="tl-text"><div class="nm">Discourage indexing</div><div class="ds">Send <span class="mono">X-Robots-Tag: noindex, nofollow</span></div></div>
            ${switchHtml('f-robots', !!h.robotsNoIndex, 'Discourage indexing')}
          </div>
          <div class="inline-fields" style="margin-top:10px">
            <div class="field-group">
              <label>Connect timeout (s)</label>
              <input class="field mono" id="f-to-connect" type="number" min="0" max="3600" value="${esc(h.timeouts && h.timeouts.connectSeconds ? h.timeouts.connectSeconds : '')}" placeholder="default" />
            </div>
            <div class="field-group">
              <label>Read timeout (s)</label>
              <input class="field mono" id="f-to-read" type="number" min="0" max="3600" value="${esc(h.timeouts && h.timeouts.readSeconds ? h.timeouts.readSeconds : '')}" placeholder="default" />
            </div>
          </div>
          <div class="hint">Blank keeps the shared pooled transport. Read timeout caps time-to-first-byte; it does not cut off slow streaming/websocket bodies.</div>
        </div>
      </div>

      <div class="stack">
        <div class="card form-section">
          <p class="section-label">TLS</p>
          <div class="field-group">
            <label>Certificate</label>
            <select class="field mono" id="f-cert">
              <option value="">none (no TLS)</option>
              ${certs.map((ct) => `<option value="${esc(ct.name)}"${tls.certificateRef === ct.name ? ' selected' : ''}>${esc(ct.name)}${certDomains[ct.name] ? ' (' + esc(certDomains[ct.name]) + ')' : ''}</option>`).join('')}
            </select>
          </div>
          <div class="field-group">
            <label>Minimum TLS version</label>
            <select class="field mono" id="f-mintls">
              <option value="1.2"${(tls.minTLSVersion || '1.2') === '1.2' ? ' selected' : ''}>1.2 (default — negotiates 1.2/1.3)</option>
              <option value="1.3"${tls.minTLSVersion === '1.3' ? ' selected' : ''}>1.3 only</option>
            </select>
            <div class="hint">1.3-only drops clients that can't do TLS 1.3 (old smart TVs / embedded). Keep 1.2 for public hosts.</div>
          </div>
          <div style="margin-top:6px">
            <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS</div></div>${switchHtml('f-forcessl', !!tls.forceSSL, 'Force SSL')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">HTTP/2</div><div class="ds">Enable HTTP/2 for this host</div></div>${switchHtml('f-http2', !!tls.http2, 'HTTP/2')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Send <span class="mono">Strict-Transport-Security</span></div></div>${switchHtml('f-hsts', !!hsts.enabled, 'HSTS')}</div>
          </div>
          <div id="hsts-fields" style="margin-top:12px;${hsts.enabled ? '' : 'display:none'}">
            <div class="inline-fields">
              <div class="field-group"><label>Max age (s)</label><input class="field mono" id="f-hsts-max" type="number" value="${esc(hsts.maxAge != null ? hsts.maxAge : 31536000)}" /></div>
            </div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Include subdomains</div></div>${switchHtml('f-hsts-sub', !!hsts.includeSubdomains, 'Include subdomains')}</div>
            <div class="toggle-line"><div class="tl-text"><div class="nm">Preload</div></div>${switchHtml('f-hsts-preload', !!hsts.preload, 'Preload')}</div>
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Middleware chain</p>
          <p class="muted" style="font-size:11.5px;margin:0 0 10px">Applied in order. Reflected in the request flow above.</p>
          <div class="check-list" id="f-mw">
            ${middlewares.length ? middlewares.map((m) => `
              <label class="check-item"><input type="checkbox" value="${esc(m.name)}"${selMw.indexOf(m.name) !== -1 ? ' checked' : ''}/>${esc(m.name)}<span class="ci-ty">${esc(m.type || '')}</span></label>
            `).join('') : '<div class="check-empty">No middleware defined yet.</div>'}
          </div>
        </div>

        <div class="card form-section">
          <p class="section-label">Access lists</p>
          <div class="check-list" id="f-al">
            ${accessLists.length ? accessLists.map((a) => `
              <label class="check-item"><input type="checkbox" value="${esc(a.name)}"${selAl.indexOf(a.name) !== -1 ? ' checked' : ''}/>${esc(a.name)}<span class="ci-ty">access-list</span></label>
            `).join('') : '<div class="check-empty">No access lists defined yet.</div>'}
          </div>
        </div>
      </div>
    </div>

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        ${isNew ? '' : `<button class="btn danger" id="deleteBtn" type="button">${ICON.trash}Delete</button>`}
        <a class="btn ghost" href="#/hosts">Cancel</a>
        <button class="btn primary" id="saveBtn" type="button">Save changes</button>
      </div>
    </div>`;

  // domains chip input
  const domainsCtl = makeChipInput($('#f-domains'), arr(h.domains), 'add domain...');
  const tagsCtl = makeChipInput($('#f-tags'), arr(h.tags), 'add tag...');

  // locations
  const locsWrap = $('#f-locs');
  function locRow(loc) {
    loc = loc || {};
    const lu = loc.upstream || {};
    const div = document.createElement('div');
    div.className = 'loc-row';
    div.innerHTML = `
      <input class="field mono loc-path" style="flex:1 1 120px" value="${esc(loc.path || '')}" placeholder="/api" aria-label="Path" />
      <span class="arrow">${ICON.arrow}</span>
      <select class="field mono loc-scheme" style="flex:0 0 90px"><option value="">(host default)</option><option value="http"${lu.scheme === 'http' ? ' selected' : ''}>http</option><option value="https"${lu.scheme === 'https' ? ' selected' : ''}>https</option></select>
      <input class="field mono loc-host" style="flex:1 1 110px" value="${esc(lu.host || '')}" placeholder="host (optional)" aria-label="Upstream host" />
      <input class="field mono loc-port" type="number" style="flex:0 0 80px" value="${esc(lu.port != null ? lu.port : '')}" placeholder="port" aria-label="Upstream port" />
      <button class="icon-btn loc-del" type="button" aria-label="Remove location">${ICON.x}</button>`;
    div.querySelector('.loc-del').addEventListener('click', () => div.remove());
    locsWrap.appendChild(div);
  }
  arr(h.locations).forEach(locRow);
  $('#addLoc').addEventListener('click', () => locRow({}));

  // HSTS show/hide
  $('#f-hsts').addEventListener('switchchange', () => {
    $('#hsts-fields').style.display = isOn('f-hsts') ? '' : 'none';
  });

  // flow
  const flowEl = $('#flow');
  function curMw() { return $$('#f-mw input:checked').map((i) => i.value); }
  function curAl() { return $$('#f-al input:checked').map((i) => i.value); }
  function refreshFlow() {
    const cert = $('#f-cert').value;
    const uphost = $('#f-uphost').value || '?';
    const upport = $('#f-upport').value || '?';
    renderHostFlow(flowEl, {
      certRef: cert, certDomains: certDomains[cert] || cert,
      upstreamStr: `${uphost}:${upport}`,
      mwSelected: curMw(), mwType, alSelected: curAl(),
    });
  }
  refreshFlow();
  $('#f-cert').addEventListener('change', refreshFlow);
  $('#f-uphost').addEventListener('input', refreshFlow);
  $('#f-upport').addEventListener('input', refreshFlow);
  $$('#f-mw input, #f-al input').forEach((i) => i.addEventListener('change', refreshFlow));

  // save
  $('#saveBtn').addEventListener('click', async () => {
    const nm = isNew ? $('#f-name').value.trim() : h.name;
    if (!nm) { toast('Name required', 'Enter an internal name for this host.', 'err'); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return; }
    const portVal = parseInt($('#f-upport').value, 10);
    if (!$('#f-uphost').value.trim() || isNaN(portVal)) { toast('Upstream incomplete', 'Set the upstream host and port.', 'err'); return; }

    const obj = { name: nm, domains: domains, upstream: { scheme: $('#f-scheme').value, host: $('#f-uphost').value.trim(), port: portVal } };
    const display = $('#f-display').value.trim();
    if (display) obj.displayName = display;
    const tags = tagsCtl.get(); if (tags.length) obj.tags = tags;
    if (isOn('f-disabled')) obj.disabled = true;
    if (isOn('f-ws')) obj.websocketsUpgrade = true;
    if (isOn('f-robots')) obj.robotsNoIndex = true;
    const toConnect = parseInt($('#f-to-connect').value, 10);
    const toRead = parseInt($('#f-to-read').value, 10);
    const timeouts = {};
    if (!isNaN(toConnect) && toConnect > 0) timeouts.connectSeconds = toConnect;
    if (!isNaN(toRead) && toRead > 0) timeouts.readSeconds = toRead;
    if (Object.keys(timeouts).length) obj.timeouts = timeouts;

    const tlsObj = {};
    const cert = $('#f-cert').value;
    if (cert) tlsObj.certificateRef = cert;
    if (isOn('f-forcessl')) tlsObj.forceSSL = true;
    if (isOn('f-http2')) tlsObj.http2 = true;
    const minTLS = $('#f-mintls') && $('#f-mintls').value;
    if (minTLS && minTLS !== '1.2') tlsObj.minTLSVersion = minTLS;
    if (isOn('f-hsts')) {
      tlsObj.hsts = {
        enabled: true,
        maxAge: parseInt($('#f-hsts-max').value, 10) || 0,
        includeSubdomains: isOn('f-hsts-sub'),
        preload: isOn('f-hsts-preload'),
      };
    }
    if (Object.keys(tlsObj).length) obj.tls = tlsObj;

    const mws = curMw(); if (mws.length) obj.middlewares = mws;
    const als = curAl(); if (als.length) obj.accessLists = als;

    const locs = [];
    $$('#f-locs .loc-row').forEach((row) => {
      const path = row.querySelector('.loc-path').value.trim();
      if (!path) return;
      const loc = { path };
      const lh = row.querySelector('.loc-host').value.trim();
      const lp = parseInt(row.querySelector('.loc-port').value, 10);
      const ls = row.querySelector('.loc-scheme').value;
      if (lh && !isNaN(lp)) loc.upstream = { scheme: ls || 'http', host: lh, port: lp };
      locs.push(loc);
    });
    if (locs.length) obj.locations = locs;

    const btn = $('#saveBtn'); btn.disabled = true;
    try {
      const r = await api('/api/proxy-hosts/' + encodeURIComponent(nm), { method: 'PUT', body: obj });
      toastSaved(r.commit);
      refreshHeadSha();
      if (isNew) location.hash = '#/hosts/' + encodeURIComponent(nm);
      else location.hash = '#/hosts';
    } catch (e) { toastErr(e); btn.disabled = false; }
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
        refreshHeadSha();
        location.hash = '#/hosts';
      } catch (e) { toastErr(e); del.disabled = false; }
    });
  }
}

// ---------- CERTIFICATES LIST ----------
async function listCerts(c) {
  const certs = arr((await api('/api/certificates')).data);
  const head = viewHead('Certificates', 'ACME and custom certificates terminating TLS at the edge.',
    `<a class="btn primary" href="#/certs/_new">${ICON.plus}Add certificate</a>`);
  if (!certs.length) {
    c.innerHTML = head + emptyState('No certificates yet', 'Request an ACME certificate or upload a custom one.', 'Add certificate', '#/certs/_new');
    return;
  }
  const cards = certs.map((ct) => {
    const domains = arr(ct.domains).join(', ');
    const kv = ct.type === 'acme'
      ? `<span class="k">Type</span><span class="v">ACME</span>
         <span class="k">Account</span><span class="v">${esc((ct.acme && ct.acme.email) || '')}</span>
         <span class="k">Challenge</span><span class="v">${esc((ct.acme && ct.acme.challenge) || 'dns-01')} via ${esc((ct.acme && ct.acme.dnsProvider) || '?')}</span>`
      : `<span class="k">Type</span><span class="v">Custom</span>
         <span class="k">Cert file</span><span class="v">${esc((ct.custom && ct.custom.certFile) || '')}</span>
         <span class="k">Key file</span><span class="v">${esc((ct.custom && ct.custom.keyFile) || '')}</span>`;
    return `<div class="card" data-name="${esc(ct.name)}" role="button" tabindex="0" style="cursor:pointer">
      <div class="card-head">
        <div><h3>${esc(ct.name)}</h3><div class="mono muted" style="font-size:12px">${esc(domains)}</div></div>
        <span class="chip ${ct.type === 'acme' ? 'cyan' : ''}">${esc(ct.type || 'cert')}</span>
      </div>
      <div class="kv">${kv}</div>
    </div>`;
  }).join('');
  c.innerHTML = head + `<div class="cards">${cards}
    <div class="card dim-card" role="button" tabindex="0" id="newCert">
      <span class="plus">${ICON.plus}</span>
      <div style="font-weight:600;color:var(--text)">Issue certificate</div>
      <div style="font-size:12px">Request a new ACME cert or upload a custom one.</div>
    </div></div>`;
  $$('.cards .card[data-name]').forEach((el) => {
    const open = () => { location.hash = '#/certs/' + encodeURIComponent(el.dataset.name); };
    el.addEventListener('click', open);
    el.addEventListener('keydown', (e) => { if (e.key === 'Enter') open(); });
  });
  const nc = $('#newCert');
  nc.addEventListener('click', () => { location.hash = '#/certs/_new'; });
  nc.addEventListener('keydown', (e) => { if (e.key === 'Enter') location.hash = '#/certs/_new'; });
}

// ---------- CERTIFICATE EDITOR ----------
async function certEditor(c, name) {
  const isNew = !name;
  const [dnsR, certR] = await Promise.all([
    api('/api/dns-providers').catch(() => ({ data: [] })),
    isNew ? Promise.resolve({ data: {} }) : api('/api/certificates/' + encodeURIComponent(name)),
  ]);
  const dnsProviders = arr(dnsR.data);
  const ct = certR.data || {};
  const acme = ct.acme || {};
  const custom = ct.custom || {};
  const type = ct.type || 'acme';

  c.innerHTML = `
    <div class="row-between view-head">
      <div>
        <div class="muted" style="font-size:12px;margin-bottom:3px"><a href="#/certs">Certificates</a> / ${isNew ? 'new' : 'edit'}</div>
        <h2 style="font-family:var(--display)">${esc(isNew ? 'New certificate' : ct.name)}</h2>
        <p>Terminate TLS with an ACME-issued or custom certificate.</p>
      </div>
    </div>

    <div class="form-grid">
      <div class="card form-section">
        <p class="section-label">Certificate</p>
        <div class="field-group">
          <label>Name</label>
          <input class="field mono" id="ct-name" value="${esc(ct.name || '')}" ${isNew ? '' : 'disabled'} placeholder="wild" />
          <div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div>
        </div>
        <div class="field-group">
          <label>Type</label>
          <select class="field" id="ct-type">
            <option value="acme"${type === 'acme' ? ' selected' : ''}>ACME (Let's Encrypt)</option>
            <option value="custom"${type === 'custom' ? ' selected' : ''}>Custom (upload)</option>
          </select>
        </div>
        <div class="field-group">
          <label>Domains</label>
          <div class="chip-input" id="ct-domains"></div>
          <div class="hint">Press Enter to add each domain.</div>
        </div>
      </div>

      <div class="card form-section">
        <div id="acme-fields" style="${type === 'custom' ? 'display:none' : ''}">
          <p class="section-label">ACME</p>
          <div class="field-group"><label>Account email</label><input class="field mono" id="ct-email" value="${esc(acme.email || '')}" placeholder="you@example.com" /></div>
          <div class="field-group"><label>Directory URL</label><input class="field mono" id="ct-dir" value="${esc(acme.directoryURL || '')}" placeholder="https://acme-v02.api.letsencrypt.org/directory" /></div>
          <div class="inline-fields">
            <div class="field-group"><label>Key type</label><input class="field mono" id="ct-keytype" value="${esc(acme.keyType || '')}" placeholder="EC256" /></div>
            <div class="field-group"><label>Challenge</label><input class="field mono" id="ct-challenge" value="dns-01" disabled /></div>
          </div>
          <div class="field-group">
            <label>DNS provider</label>
            <select class="field mono" id="ct-dns">
              <option value="">select provider...</option>
              ${dnsProviders.map((p) => `<option value="${esc(p.name)}"${acme.dnsProvider === p.name ? ' selected' : ''}>${esc(p.name)}</option>`).join('')}
            </select>
            ${dnsProviders.length ? '' : '<div class="hint">No DNS providers configured yet. Add one under DNS Providers.</div>'}
          </div>
        </div>
        <div id="custom-fields" style="${type === 'custom' ? '' : 'display:none'}">
          <p class="section-label">Custom certificate</p>
          <div class="field-group"><label>Certificate file</label><input class="field mono" id="ct-certfile" value="${esc(custom.certFile || '')}" placeholder="/path/fullchain.pem" /></div>
          <div class="field-group"><label>Key file</label><input class="field mono" id="ct-keyfile" value="${esc(custom.keyFile || '')}" placeholder="/path/privkey.pem" /></div>
        </div>
      </div>
    </div>

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
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

  $('#ct-save').addEventListener('click', async () => {
    const nm = isNew ? $('#ct-name').value.trim() : ct.name;
    if (!nm) { toast('Name required', 'Enter a certificate name.', 'err'); return; }
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return; }
    const t = $('#ct-type').value;
    const obj = { name: nm, type: t, domains };
    if (t === 'acme') {
      const a = { email: $('#ct-email').value.trim(), challenge: 'dns-01', dnsProvider: $('#ct-dns').value };
      const dir = $('#ct-dir').value.trim(); if (dir) a.directoryURL = dir;
      const kt = $('#ct-keytype').value.trim(); if (kt) a.keyType = kt;
      if (!a.dnsProvider) { toast('DNS provider required', 'Select a DNS provider for dns-01.', 'err'); return; }
      obj.acme = a;
    } else {
      obj.custom = { certFile: $('#ct-certfile').value.trim(), keyFile: $('#ct-keyfile').value.trim() };
    }
    const btn = $('#ct-save'); btn.disabled = true;
    try {
      const r = await api('/api/certificates/' + encodeURIComponent(nm), { method: 'PUT', body: obj });
      toastSaved(r.commit); refreshHeadSha();
      location.hash = '#/certs';
    } catch (e) { toastErr(e); btn.disabled = false; }
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
}

// ---------- GENERIC SECTIONS (list + JSON-editor form) ----------
const SECTION_META = {
  identity: {
    title: 'Identity', sub: 'Single sign-on providers and admin authentication.',
    singular: 'identity provider', addLabel: 'Add identity provider',
    summary: (o) => `<span class="k">Type</span><span class="v">${esc(o.type || '')}</span>` +
      (o.oidc ? `<span class="k">Issuer</span><span class="v">${esc(o.oidc.issuerURL || '')}</span><span class="k">Client ID</span><span class="v">${esc(o.oidc.clientID || '')}</span>` : '') +
      (o.forwardAuth ? `<span class="k">User header</span><span class="v">${esc(o.forwardAuth.userHeader || '')}</span>` : ''),
  },
  access: {
    title: 'Access Lists', sub: 'Reusable IP and basic-auth rules attached to hosts via the chain.',
    singular: 'access list', addLabel: 'Add access list',
    summary: (o) => `<span class="k">Satisfy</span><span class="v">${o.satisfyAny ? 'any' : 'all'}</span>` +
      `<span class="k">Rules</span><span class="v">${arr(o.rules).length}</span>` +
      `<span class="k">Basic auth</span><span class="v">${arr(o.basicAuth).length} user(s)</span>` +
      (o.defaultAction ? `<span class="k">Default</span><span class="v">${esc(o.defaultAction)}</span>` : ''),
  },
  middleware: {
    title: 'Middleware', sub: 'Reusable, composable objects you drop into any host chain.',
    singular: 'middleware', addLabel: 'Add middleware',
    summary: (o) => `<span class="k">Type</span><span class="v">${esc(o.type || '')}</span>` +
      (o.auth ? `<span class="k">IdP</span><span class="v">${esc(o.auth.identityProvider || '')}</span>` : '') +
      (o.rateLimit ? `<span class="k">Rate</span><span class="v">${esc(o.rateLimit.requestsPerSecond)} r/s</span>` : ''),
  },
  dns: {
    title: 'DNS Providers', sub: 'Credentials used for ACME dns-01 challenges.',
    singular: 'DNS provider', addLabel: 'Add DNS provider',
    summary: (o) => `<span class="k">Provider</span><span class="v">${esc(o.provider || '')}</span>` +
      (o.config ? `<span class="k">Config keys</span><span class="v">${esc(Object.keys(o.config).join(', '))}</span>` : ''),
  },
  redirects: {
    title: 'Redirects', sub: '301/302 redirect rules for legacy or vanity hostnames.',
    singular: 'redirect', addLabel: 'Add redirect',
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>` +
      (o.targetDomain ? `<span class="k">To</span><span class="v">${esc(((o.targetScheme && o.targetScheme !== 'auto') ? o.targetScheme + '://' : '') + o.targetDomain)}</span>` : '') +
      (o.statusCode ? `<span class="k">Code</span><span class="v">${esc(o.statusCode)}</span>` : ''),
  },
  streams: {
    title: 'Streams', sub: 'Raw TCP/UDP forwarding for non-HTTP services.',
    singular: 'stream', addLabel: 'Add stream',
    summary: (o) => (o.listenPort != null ? `<span class="k">Listen</span><span class="v">:${esc(o.listenPort)}</span>` : '') +
      (o.forwardHost ? `<span class="k">Forward</span><span class="v">${esc((o.forwardHost || '') + ':' + (o.forwardPort != null ? o.forwardPort : ''))}</span>` : '') +
      (o.protocol ? `<span class="k">Protocol</span><span class="v">${esc(o.protocol)}</span>` : ''),
  },
  dead: {
    title: 'Dead hosts', sub: 'Hosts kept for 404 handling or scheduled decommission.',
    singular: 'dead host', addLabel: 'Add dead host',
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>`,
  },
};

async function genericSection(c, section, sub) {
  if (sub === '_new') return EDITORS[section](c, null);
  if (sub) return EDITORS[section](c, sub);
  return genericList(c, section);
}

async function genericList(c, section) {
  const meta = SECTION_META[section];
  const plural = PLURAL[section];
  const items = arr((await api('/api/' + plural)).data);
  const head = viewHead(meta.title, meta.sub,
    `<a class="btn primary" href="#/${section}/_new">${ICON.plus}${meta.addLabel}</a>`);
  if (!items.length) {
    c.innerHTML = head + emptyState(`No ${meta.singular}s yet`, `Add your first ${meta.singular}.`, meta.addLabel, `#/${section}/_new`);
    return;
  }
  const cards = items.map((o) => `
    <div class="card" data-name="${esc(o.name)}" role="button" tabindex="0" style="cursor:pointer">
      <div class="card-head">
        <div><h3>${esc(o.name)}</h3></div>
        <button class="btn ghost sm danger gs-del" data-name="${esc(o.name)}" type="button">Delete</button>
      </div>
      <div class="kv">${meta.summary(o)}</div>
    </div>`).join('');
  c.innerHTML = head + `<div class="cards">${cards}</div>`;
  $$('.cards .card[data-name]').forEach((el) => {
    const open = () => { location.hash = `#/${section}/` + encodeURIComponent(el.dataset.name); };
    el.addEventListener('click', (e) => { if (!e.target.closest('.gs-del')) open(); });
    el.addEventListener('keydown', (e) => { if (e.key === 'Enter') open(); });
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
      ${isNew ? '' : `<button class="btn danger" id="ed-delete" type="button">${ICON.trash}Delete</button>`}
      <a class="btn ghost" href="#/${section}">Cancel</a>
      <button class="btn primary" id="ed-save" type="button">${esc(isNew ? addLabel : 'Save changes')}</button>
    </div>
  </div>`;
}
function nameCard(obj, isNew) {
  return `<div class="card form-section">
    <p class="section-label">Identity</p>
    <div class="inline-fields">
      <div class="field-group"><label>Name</label><input class="field mono" id="ed-name" value="${esc(obj.name || '')}" ${isNew ? '' : 'disabled'} placeholder="internal-name" /><div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div></div>
      <div class="field-group"><label>Display name</label><input class="field" id="ed-display" value="${esc(obj.displayName || '')}" placeholder="optional label" /></div>
    </div>
    <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Disabled</div><div class="ds">Exclude from the compiled data plane</div></div>${switchHtml('ed-disabled', !!obj.disabled, 'Disabled')}</div>
  </div>`;
}
function tlsCard(tls, certs, certDomains) {
  tls = tls || {}; const hsts = tls.hsts || {};
  return `<div class="card form-section">
    <p class="section-label">TLS</p>
    <div class="field-group"><label>Certificate</label>
      <select class="field mono" id="ed-cert"><option value="">none (no TLS)</option>
        ${certs.map((ct) => `<option value="${esc(ct.name)}"${tls.certificateRef === ct.name ? ' selected' : ''}>${esc(ct.name)}${certDomains[ct.name] ? ' (' + esc(certDomains[ct.name]) + ')' : ''}</option>`).join('')}
      </select></div>
    <div style="margin-top:6px">
      <div class="toggle-line"><div class="tl-text"><div class="nm">Force SSL</div><div class="ds">Redirect HTTP to HTTPS</div></div>${switchHtml('ed-forcessl', !!tls.forceSSL, 'Force SSL')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">HTTP/2</div></div>${switchHtml('ed-http2', !!tls.http2, 'HTTP/2')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">HSTS</div><div class="ds">Send <span class="mono">Strict-Transport-Security</span></div></div>${switchHtml('ed-hsts', !!hsts.enabled, 'HSTS')}</div>
    </div>
    <div id="ed-hsts-fields" style="margin-top:12px;${hsts.enabled ? '' : 'display:none'}">
      <div class="inline-fields"><div class="field-group"><label>Max age (s)</label><input class="field mono" id="ed-hsts-max" type="number" value="${esc(hsts.maxAge != null ? hsts.maxAge : 31536000)}" /></div></div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Include subdomains</div></div>${switchHtml('ed-hsts-sub', !!hsts.includeSubdomains, 'Include subdomains')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Preload</div></div>${switchHtml('ed-hsts-preload', !!hsts.preload, 'Preload')}</div>
    </div>
  </div>`;
}
function wireTls() {
  const h = $('#ed-hsts');
  if (h) h.addEventListener('switchchange', () => { $('#ed-hsts-fields').style.display = isOn('ed-hsts') ? '' : 'none'; });
}
function readTls() {
  const tls = {};
  const cert = $('#ed-cert').value; if (cert) tls.certificateRef = cert;
  if (isOn('ed-forcessl')) tls.forceSSL = true;
  if (isOn('ed-http2')) tls.http2 = true;
  if (isOn('ed-hsts')) tls.hsts = { enabled: true, maxAge: parseInt($('#ed-hsts-max').value, 10) || 0, includeSubdomains: isOn('ed-hsts-sub'), preload: isOn('ed-hsts-preload') };
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

// Wire the Save/Delete buttons. buildBody(name) returns the object to PUT (common
// meta is applied here) or null to abort after showing its own toast.
function wireEditor(section, plural, meta, isNew, origName, buildBody) {
  $('#ed-save').addEventListener('click', async () => {
    const nm = isNew ? $('#ed-name').value.trim() : origName;
    if (!nm) { toast('Name required', 'Enter a name.', 'err'); return; }
    const body = buildBody(nm);
    if (!body) return;
    body.name = nm;
    const disp = $('#ed-display'); if (disp && disp.value.trim()) body.displayName = disp.value.trim();
    if (isOn('ed-disabled')) body.disabled = true;
    const btn = $('#ed-save'); btn.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'PUT', body });
      toastSaved(r.commit); refreshHeadSha();
      location.hash = '#/' + section;
    } catch (e) { toastErr(e); btn.disabled = false; }
  });
  const del = $('#ed-delete');
  if (del) del.addEventListener('click', async () => {
    if (!confirm(`Delete ${meta.singular} "${origName}"?`)) return;
    del.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(origName), { method: 'DELETE' });
      toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'removed', 'ok', { html: true });
      refreshHeadSha(); location.hash = '#/' + section;
    } catch (e) { toastErr(e); del.disabled = false; }
  });
}

// ---------- REDIRECT HOST EDITOR ----------
async function redirectEditor(c, name) {
  const meta = SECTION_META.redirects; const isNew = !name;
  const [certsR, objR] = await Promise.all([
    api('/api/certificates').catch(() => ({ data: [] })),
    isNew ? Promise.resolve({ data: {} }) : api('/api/redirect-hosts/' + encodeURIComponent(name)),
  ]);
  const certs = arr(certsR.data); const certDomains = {}; certs.forEach((ct) => { certDomains[ct.name] = arr(ct.domains).join(', '); });
  const o = objR.data || {};
  c.innerHTML = editorHead('redirects', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains"></div><div class="hint">Press Enter to add. At least one domain is required.</div></div>
    <div class="card form-section"><p class="section-label">Redirect target</p>
      <div class="inline-fields">
        <div class="field-group"><label>Target scheme</label><select class="field mono" id="ed-tscheme">
          ${['auto', 'http', 'https'].map((s) => `<option value="${s}"${(o.targetScheme || 'auto') === s ? ' selected' : ''}>${s}</option>`).join('')}
        </select></div>
        <div class="field-group" style="flex:2"><label>Target domain</label><input class="field mono" id="ed-tdomain" value="${esc(o.targetDomain || '')}" placeholder="example.com" /></div>
        <div class="field-group"><label>Status code</label><select class="field mono" id="ed-status">
          ${[301, 302, 307, 308].map((s) => `<option value="${s}"${(o.statusCode || 301) === s ? ' selected' : ''}>${s}</option>`).join('')}
        </select></div>
      </div>
      <div class="toggle-line" style="margin-top:6px"><div class="tl-text"><div class="nm">Preserve path</div><div class="ds">Append the original request path to the target</div></div>${switchHtml('ed-preserve', !!o.preservePath, 'Preserve path')}</div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, certDomains)}</div></div>` + saveBar('redirects', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...');
  wireTls();
  wireEditor('redirects', 'redirect-hosts', meta, isNew, name || o.name, (nm) => {
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return null; }
    const td = $('#ed-tdomain').value.trim();
    if (!td) { toast('Target required', 'Set the target domain.', 'err'); return null; }
    const body = { domains, targetScheme: $('#ed-tscheme').value, targetDomain: td, statusCode: parseInt($('#ed-status').value, 10) };
    if (isOn('ed-preserve')) body.preservePath = true;
    const tls = readTls(); if (tls) body.tls = tls;
    return body;
  });
}

// ---------- STREAM HOST EDITOR ----------
async function streamEditor(c, name) {
  const meta = SECTION_META.streams; const isNew = !name;
  const o = isNew ? {} : ((await api('/api/stream-hosts/' + encodeURIComponent(name))).data || {});
  c.innerHTML = editorHead('streams', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Forwarding</p>
      <div class="inline-fields">
        <div class="field-group"><label>Listen port</label><input class="field mono" id="ed-listen" type="number" value="${esc(o.listenPort != null ? o.listenPort : '')}" placeholder="5353" /></div>
        <div class="field-group"><label>Protocol</label><select class="field mono" id="ed-proto">
          ${['tcp', 'udp', 'both'].map((p) => `<option value="${p}"${(o.protocol || 'tcp') === p ? ' selected' : ''}>${p}</option>`).join('')}
        </select></div>
      </div>
      <div class="inline-fields" style="margin-top:14px">
        <div class="field-group" style="flex:2"><label>Forward host</label><input class="field mono" id="ed-fhost" value="${esc(o.forwardHost || '')}" placeholder="10.0.0.5" /></div>
        <div class="field-group"><label>Forward port</label><input class="field mono" id="ed-fport" type="number" value="${esc(o.forwardPort != null ? o.forwardPort : '')}" placeholder="53" /></div>
      </div>
    </div>
  </div></div>` + saveBar('streams', isNew, meta.addLabel);
  wireEditor('streams', 'stream-hosts', meta, isNew, name || o.name, () => {
    const lp = parseInt($('#ed-listen').value, 10); const fp = parseInt($('#ed-fport').value, 10); const fh = $('#ed-fhost').value.trim();
    if (isNaN(lp)) { toast('Listen port required', 'Enter a listen port.', 'err'); return null; }
    if (!fh || isNaN(fp)) { toast('Forward target required', 'Set forward host and port.', 'err'); return null; }
    return { listenPort: lp, protocol: $('#ed-proto').value, forwardHost: fh, forwardPort: fp };
  });
}

// ---------- DEAD HOST EDITOR ----------
async function deadEditor(c, name) {
  const meta = SECTION_META.dead; const isNew = !name;
  const [certsR, objR] = await Promise.all([
    api('/api/certificates').catch(() => ({ data: [] })),
    isNew ? Promise.resolve({ data: {} }) : api('/api/dead-hosts/' + encodeURIComponent(name)),
  ]);
  const certs = arr(certsR.data); const certDomains = {}; certs.forEach((ct) => { certDomains[ct.name] = arr(ct.domains).join(', '); });
  const o = objR.data || {};
  c.innerHTML = editorHead('dead', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Domains</p><div class="chip-input" id="ed-domains"></div><div class="hint">Press Enter to add. At least one domain is required.</div></div>
    <div class="card form-section"><p class="section-label">Response</p>
      <div class="field-group"><label>Status code</label><input class="field mono" id="ed-status" type="number" value="${esc(o.statusCode != null ? o.statusCode : 404)}" placeholder="404" /><div class="hint">Returned for every request to these domains. Default 404.</div></div>
    </div>
  </div><div class="stack">${tlsCard(o.tls, certs, certDomains)}</div></div>` + saveBar('dead', isNew, meta.addLabel);
  const domainsCtl = makeChipInput($('#ed-domains'), arr(o.domains), 'add domain...');
  wireTls();
  wireEditor('dead', 'dead-hosts', meta, isNew, name || o.name, () => {
    const domains = domainsCtl.get();
    if (!domains.length) { toast('Domain required', 'Add at least one domain.', 'err'); return null; }
    const body = { domains };
    const sc = parseInt($('#ed-status').value, 10); if (!isNaN(sc)) body.statusCode = sc;
    const tls = readTls(); if (tls) body.tls = tls;
    return body;
  });
}

// ---------- DNS PROVIDER EDITOR ----------
async function dnsEditor(c, name) {
  const meta = SECTION_META.dns; const isNew = !name;
  const o = isNew ? { provider: 'cloudflare', config: { apiToken: '${ENV:CF_API_TOKEN}' } } : ((await api('/api/dns-providers/' + encodeURIComponent(name))).data || {});
  c.innerHTML = editorHead('dns', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Provider</p>
      <div class="field-group"><label>Provider</label>
        <input class="field mono" id="ed-provider" list="dns-provider-list" value="${esc(o.provider || 'cloudflare')}" placeholder="cloudflare" />
        <datalist id="dns-provider-list"><option value="cloudflare"></option></datalist>
        <div class="hint">P0 ships cloudflare.</div>
      </div>
    </div>
    <div class="card form-section"><p class="section-label">Credentials</p>
      <div id="ed-config"></div>
      <button class="btn ghost sm" id="ed-addcfg" type="button" style="margin-top:6px">${ICON.plus}Add credential</button>
      <div class="hint" style="margin-top:8px">Use a placeholder like <span class="mono">\${ENV:CF_API_TOKEN}</span> or <span class="mono">\${FILE:/run/secrets/token}</span> so no secret is committed. A masked secret reads <span class="mono">***</span>.</div>
    </div>
  </div></div>` + saveBar('dns', isNew, meta.addLabel);
  const cfgCtl = makeKVRows($('#ed-config'), o.config || {}, 'key (e.g. apiToken)', '${ENV:CF_API_TOKEN}', true);
  $('#ed-addcfg').addEventListener('click', () => cfgCtl.addRow('', ''));
  wireEditor('dns', 'dns-providers', meta, isNew, name || o.name, () => {
    const prov = $('#ed-provider').value.trim();
    if (!prov) { toast('Provider required', 'Enter a provider.', 'err'); return null; }
    if (cfgCtl.masked()) { toast('Secret masked', 'A credential is masked as ***. Replace it with a real value or a ${ENV:...} placeholder before saving.', 'err'); return null; }
    const body = { provider: prov };
    const cfg = cfgCtl.get(); if (Object.keys(cfg).length) body.config = cfg;
    return body;
  });
}

// ---------- ACCESS LIST EDITOR ----------
async function accessEditor(c, name) {
  const meta = SECTION_META.access; const isNew = !name;
  const o = isNew ? {} : ((await api('/api/access-lists/' + encodeURIComponent(name))).data || {});
  await loadCapabilities();
  const geoAvailable = hasCapability('geoip.dbLoaded');
  const geoReason = 'GeoIP database not loaded (set GPM_GEOIP_DB) - geo rules are unavailable.';
  const geo = o.geo || {};
  c.innerHTML = editorHead('access', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Policy</p>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Satisfy any</div><div class="ds">Pass if EITHER basic-auth or IP matches (off = require both)</div></div>${switchHtml('ed-satisfy', !!o.satisfyAny, 'Satisfy any')}</div>
      <div class="field-group" style="margin-top:12px"><label>Default action</label><select class="field mono" id="ed-default">
        ${['deny', 'allow'].map((a) => `<option value="${a}"${(o.defaultAction || 'deny') === a ? ' selected' : ''}>${a}</option>`).join('')}
      </select><div class="hint">Applied when no IP rule matches.</div></div>
    </div>
    <div class="card form-section"><p class="section-label">IP rules</p><div id="ed-rules"></div>
      <button class="btn ghost sm" id="ed-addrule" type="button" style="margin-top:6px">${ICON.plus}Add rule</button>
      <div class="hint" style="margin-top:6px">Evaluated top-down. CIDR or bare IP.</div>
    </div>
    <div class="card form-section" id="ed-geo-card"><p class="section-label">Geo rules</p>
      ${geoAvailable ? '' : `<div class="field-group"><div class="hint warn" id="ed-geo-hint">${esc(geoReason)}</div></div>`}
      <div class="field-group"><label>Country allow</label>
        <div class="chip-input" id="ed-geo-allow"></div>
        <div class="hint">ISO-3166-1 alpha-2 codes (e.g. US). Non-empty allow list takes priority over deny.</div>
      </div>
      <div class="field-group"><label>Country deny</label><div class="chip-input" id="ed-geo-deny"></div></div>
      <div class="field-group"><label>On unknown country</label><select class="field mono" id="ed-geo-unknown">
        <option value="">(default: allow)</option>
        ${['allow', 'deny'].map((a) => `<option value="${a}"${geo.onUnknown === a ? ' selected' : ''}>${a}</option>`).join('')}
      </select><div class="hint">Applied to an IP with no country in the database (private/reserved ranges, DB misses).</div></div>
    </div>
  </div><div class="stack">
    <div class="card form-section"><p class="section-label">Basic auth users</p><div id="ed-basic"></div>
      <button class="btn ghost sm" id="ed-addbasic" type="button" style="margin-top:6px">${ICON.plus}Add user</button>
      <div class="hint" style="margin-top:6px">Password must be a bcrypt hash.</div>
    </div>
  </div></div>` + saveBar('access', isNew, meta.addLabel);
  const rulesWrap = $('#ed-rules');
  function ruleRow(r) {
    r = r || {}; const d = document.createElement('div'); d.className = 'loc-row';
    d.innerHTML = `<select class="field mono rule-action" style="flex:0 0 100px">${['allow', 'deny'].map((a) => `<option value="${a}"${r.action === a ? ' selected' : ''}>${a}</option>`).join('')}</select>
      <input class="field mono rule-cidr" style="flex:1 1 160px" value="${esc(r.cidr || '')}" placeholder="10.0.0.0/8" aria-label="CIDR" />
      <button class="icon-btn rule-del" type="button" aria-label="Remove">${ICON.x}</button>`;
    d.querySelector('.rule-del').addEventListener('click', () => d.remove());
    rulesWrap.appendChild(d);
  }
  arr(o.rules).forEach(ruleRow);
  $('#ed-addrule').addEventListener('click', () => ruleRow({ action: 'allow' }));
  const basicWrap = $('#ed-basic');
  function basicRow(u) {
    u = u || {}; const d = document.createElement('div'); d.className = 'loc-row';
    d.innerHTML = `<input class="field mono basic-user" style="flex:1 1 110px" value="${esc(u.username || '')}" placeholder="username" aria-label="Username" />
      <input class="field mono basic-hash" style="flex:2 1 180px" value="${esc(u.passwordHash || '')}" placeholder="bcrypt hash" aria-label="Password hash" />
      <button class="icon-btn basic-del" type="button" aria-label="Remove">${ICON.x}</button>`;
    d.querySelector('.basic-del').addEventListener('click', () => d.remove());
    basicWrap.appendChild(d);
  }
  arr(o.basicAuth).forEach(basicRow);
  $('#ed-addbasic').addEventListener('click', () => basicRow({}));

  // geo rules - country allow/deny + on-unknown, gated on the GeoIP DB being
  // loaded (GET /api/capabilities). Disabled controls stay visible with a
  // tooltip/inline note rather than being hidden; the server still enforces
  // this independently at write time.
  const geoAllowCtl = makeChipInput($('#ed-geo-allow'), arr(geo.countryAllow), 'add country code...');
  const geoDenyCtl = makeChipInput($('#ed-geo-deny'), arr(geo.countryDeny), 'add country code...');
  gateControl($('#ed-geo-allow'), geoAvailable, geoReason);
  gateControl($('#ed-geo-deny'), geoAvailable, geoReason);
  gateControl($('#ed-geo-unknown'), geoAvailable, geoReason);

  wireEditor('access', 'access-lists', meta, isNew, name || o.name, () => {
    const body = {};
    if (isOn('ed-satisfy')) body.satisfyAny = true;
    body.defaultAction = $('#ed-default').value;
    const rules = [];
    rulesWrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
      const cidr = r.querySelector('.rule-cidr').value.trim();
      if (!cidr) return;
      rules.push({ action: r.querySelector('.rule-action').value, cidr });
    });
    if (rules.length) body.rules = rules;
    const basic = []; let bad = false;
    basicWrap.querySelectorAll(':scope > .loc-row').forEach((r) => {
      const u = r.querySelector('.basic-user').value.trim(); const h = r.querySelector('.basic-hash').value.trim();
      if (!u && !h) return;
      if (!u || !h) bad = true;
      basic.push({ username: u, passwordHash: h });
    });
    if (bad) { toast('Basic auth incomplete', 'Each user needs a username and a password hash.', 'err'); return null; }
    if (basic.length) body.basicAuth = basic;
    const geoAllow = geoAllowCtl.get(); const geoDeny = geoDenyCtl.get(); const onUnknown = $('#ed-geo-unknown').value;
    const geoBody = {};
    if (geoAllow.length) geoBody.countryAllow = geoAllow;
    if (geoDeny.length) geoBody.countryDeny = geoDeny;
    if (onUnknown) geoBody.onUnknown = onUnknown;
    if (Object.keys(geoBody).length) body.geo = geoBody;
    return body;
  });
}

// ---------- IDENTITY PROVIDER EDITOR (polymorphic) ----------
async function idpEditor(c, name) {
  const meta = SECTION_META.identity; const isNew = !name;
  const o = isNew ? { type: 'oidc' } : ((await api('/api/identity-providers/' + encodeURIComponent(name))).data || {});
  const type = o.type || 'oidc';
  const oidc = o.oidc || {}; const fa = o.forwardAuth || {}; const ar = o.authRequest || {}; const rm = o.roleMapping || {};
  c.innerHTML = editorHead('identity', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Type</p>
      <div class="field-group"><label>Provider type</label><select class="field mono" id="ed-type">
        ${[['oidc', 'OIDC'], ['forward-auth', 'Forward-auth (trusted headers)'], ['auth-request', 'Auth-request (external)']].map(([v, l]) => `<option value="${v}"${type === v ? ' selected' : ''}>${esc(l)}</option>`).join('')}
      </select></div>
    </div>

    <div class="card form-section ed-sub" data-type="oidc" style="${type === 'oidc' ? '' : 'display:none'}"><p class="section-label">OIDC</p>
      <div class="field-group"><label>Issuer URL</label><input class="field mono" id="oidc-issuer" value="${esc(oidc.issuerURL || '')}" placeholder="https://idp.example.com/application/o/app/" /></div>
      <div class="field-group"><label>Client ID</label><input class="field mono" id="oidc-clientid" value="${esc(oidc.clientID || '')}" placeholder="client-id" /></div>
      <div class="field-group"><label>Client secret</label><input class="field mono" id="oidc-secret" value="${esc(oidc.clientSecret || '')}" placeholder="\${ENV:OIDC_CLIENT_SECRET}" /><div class="hint">Use a <span class="mono">\${ENV:...}</span> placeholder. A masked secret reads <span class="mono">***</span>; replace it to change.</div></div>
      <div class="field-group"><label>Scopes</label><div class="chip-input" id="oidc-scopes"></div><div class="hint">Default openid, profile, email, groups.</div></div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Use PKCE</div><div class="ds">Recommended; public clients can run with no secret</div></div>${switchHtml('oidc-pkce', oidc.usePKCE !== false, 'Use PKCE')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Require verified email</div></div>${switchHtml('oidc-verify', !!oidc.requireVerifiedEmail, 'Require verified email')}</div>
      <div class="toggle-line"><div class="tl-text"><div class="nm">Trust IdP MFA</div><div class="ds">Trust acr/amr instead of a second local prompt</div></div>${switchHtml('oidc-mfa', !!oidc.trustIdPMFA, 'Trust IdP MFA')}</div>
    </div>

    <div class="card form-section ed-sub" data-type="forward-auth" style="${type === 'forward-auth' ? '' : 'display:none'}"><p class="section-label">Forward-auth</p>
      <div class="field-group"><label>Trusted proxies (CIDRs)</label><div class="chip-input" id="fa-trusted"></div><div class="hint">Identity headers are honoured only from these. Required.</div></div>
      <div class="inline-fields">
        <div class="field-group"><label>User header</label><input class="field mono" id="fa-user" value="${esc(fa.userHeader || '')}" placeholder="X-authentik-username" /></div>
        <div class="field-group"><label>Email header</label><input class="field mono" id="fa-email" value="${esc(fa.emailHeader || '')}" placeholder="X-authentik-email" /></div>
      </div>
      <div class="inline-fields">
        <div class="field-group"><label>Name header</label><input class="field mono" id="fa-name" value="${esc(fa.nameHeader || '')}" placeholder="X-authentik-name" /></div>
        <div class="field-group"><label>Groups header</label><input class="field mono" id="fa-groups" value="${esc(fa.groupsHeader || '')}" placeholder="X-authentik-groups" /></div>
      </div>
      <div class="inline-fields">
        <div class="field-group"><label>Groups delimiter</label><input class="field mono" id="fa-delim" value="${esc(fa.groupsDelimiter || '')}" placeholder="," /></div>
        <div class="field-group"><label>AMR header</label><input class="field mono" id="fa-amr" value="${esc(fa.amrHeader || '')}" placeholder="X-authentik-auth-method" /></div>
      </div>
    </div>

    <div class="card form-section ed-sub" data-type="auth-request" style="${type === 'auth-request' ? '' : 'display:none'}"><p class="section-label">Auth-request</p>
      <div class="field-group"><label>Outpost URL</label><input class="field mono" id="ar-outpost" value="${esc(ar.outpostURL || '')}" placeholder="http://auth-outpost:9000" /></div>
      <div class="inline-fields">
        <div class="field-group"><label>Path prefix</label><input class="field mono" id="ar-prefix" value="${esc(ar.pathPrefix || '')}" placeholder="/outpost.goauthentik.io" /></div>
        <div class="field-group"><label>Auth path</label><input class="field mono" id="ar-authpath" value="${esc(ar.authPath || '')}" placeholder="/outpost.goauthentik.io/auth/nginx" /></div>
      </div>
      <div class="field-group"><label>Copy headers</label><div class="chip-input" id="ar-copy"></div><div class="hint">Response headers copied to the upstream on success.</div></div>
    </div>
  </div><div class="stack">
    <div class="card form-section"><p class="section-label">Role mapping</p>
      <div class="field-group"><label>Groups claim</label><input class="field mono" id="rm-claim" value="${esc(rm.groupsClaim || '')}" placeholder="groups" /></div>
      <div class="field-group"><label>Admin groups</label><div class="chip-input" id="rm-admin"></div></div>
      <div class="field-group"><label>User groups</label><div class="chip-input" id="rm-user"></div></div>
      <div class="field-group"><label>Default role</label><select class="field mono" id="rm-default">
        ${[['', 'deny (no match)'], ['user', 'user'], ['admin', 'admin']].map(([v, l]) => `<option value="${v}"${(rm.defaultRole || '') === v ? ' selected' : ''}>${esc(l)}</option>`).join('')}
      </select></div>
    </div>
  </div></div>` + saveBar('identity', isNew, meta.addLabel);
  const scopesCtl = makeChipInput($('#oidc-scopes'), arr(oidc.scopes), 'add scope...');
  const trustedCtl = makeChipInput($('#fa-trusted'), arr(fa.trustedProxies), 'add CIDR...');
  const copyCtl = makeChipInput($('#ar-copy'), arr(ar.copyHeaders), 'add header...');
  const adminCtl = makeChipInput($('#rm-admin'), arr(rm.adminGroups), 'add group...');
  const userCtl = makeChipInput($('#rm-user'), arr(rm.userGroups), 'add group...');
  $('#ed-type').addEventListener('change', () => { const t = $('#ed-type').value; $$('.ed-sub').forEach((el) => { el.style.display = el.dataset.type === t ? '' : 'none'; }); });
  wireEditor('identity', 'identity-providers', meta, isNew, name || o.name, () => {
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
      if (isOn('oidc-mfa')) spec.trustIdPMFA = true;
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
}

// ---------- MIDDLEWARE EDITOR (polymorphic) ----------
async function middlewareEditor(c, name) {
  const meta = SECTION_META.middleware; const isNew = !name;
  const [idpR, objR] = await Promise.all([
    api('/api/identity-providers').catch(() => ({ data: [] })),
    isNew ? Promise.resolve({ data: { type: 'headers' } }) : api('/api/middlewares/' + encodeURIComponent(name)),
  ]);
  const idps = arr(idpR.data);
  const o = objR.data || {}; const type = o.type || 'headers';
  const auth = o.auth || {}; const headers = o.headers || {}; const guard = o.guard || {}; const rl = o.rateLimit || {};
  c.innerHTML = editorHead('middleware', meta, isNew, name) + `<div class="form-grid"><div class="stack">
    ${nameCard(o, isNew)}
    <div class="card form-section"><p class="section-label">Type</p>
      <div class="field-group"><label>Middleware type</label><select class="field mono" id="ed-type">
        ${[['auth', 'Auth'], ['headers', 'Headers'], ['guard', 'Guard'], ['rate-limit', 'Rate limit']].map(([v, l]) => `<option value="${v}"${type === v ? ' selected' : ''}>${esc(l)}</option>`).join('')}
      </select></div>
    </div>

    <div class="card form-section ed-sub" data-type="auth" style="${type === 'auth' ? '' : 'display:none'}"><p class="section-label">Auth</p>
      <div class="field-group"><label>Identity provider</label><select class="field mono" id="mw-idp">
        <option value="">select provider...</option>
        ${idps.map((p) => `<option value="${esc(p.name)}"${auth.identityProvider === p.name ? ' selected' : ''}>${esc(p.name)}${p.type ? ' (' + esc(p.type) + ')' : ''}</option>`).join('')}
      </select>${idps.length ? '' : '<div class="hint">No identity providers yet. Add one under Identity.</div>'}</div>
      <div class="field-group"><label>Mode</label><select class="field mono" id="mw-mode">
        ${[['', '(from IdP type)'], ['oidc', 'oidc'], ['forward-auth', 'forward-auth'], ['auth-request', 'auth-request']].map(([v, l]) => `<option value="${v}"${(auth.mode || '') === v ? ' selected' : ''}>${esc(l)}</option>`).join('')}
      </select></div>
      <div class="field-group"><label>Required roles</label><div class="chip-input" id="mw-roles"></div><div class="hint">Not supported in auth-request mode.</div></div>
      <div class="field-group"><label>Allow from (CIDRs)</label><div class="chip-input" id="mw-allow"></div><div class="hint">Client CIDRs that bypass auth entirely.</div></div>
    </div>

    <div class="card form-section ed-sub" data-type="headers" style="${type === 'headers' ? '' : 'display:none'}"><p class="section-label">Headers</p>
      <div class="field-group"><label>Set request headers</label><div id="hdr-setreq"></div><button class="btn ghost sm hdr-add" data-wrap="hdr-setreq" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>
      <div class="field-group"><label>Set response headers</label><div id="hdr-setresp"></div><button class="btn ghost sm hdr-add" data-wrap="hdr-setresp" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>
      <div class="field-group"><label>Remove request headers</label><div class="chip-input" id="hdr-rmreq"></div></div>
      <div class="field-group"><label>Remove response headers</label><div class="chip-input" id="hdr-rmresp"></div></div>
    </div>

    <div class="card form-section ed-sub" data-type="guard" style="${type === 'guard' ? '' : 'display:none'}"><p class="section-label">Guard</p>
      <div id="guard-triggers"></div>
      <button class="btn ghost sm" id="guard-addtrig" type="button" style="margin-top:6px">${ICON.plus}Add trigger</button>
      <div class="inline-fields" style="margin-top:14px">
        <div class="field-group"><label>Deny status</label><input class="field mono" id="guard-deny" type="number" value="${esc(guard.denyStatus != null ? guard.denyStatus : '')}" placeholder="403" /></div>
      </div>
      <div class="field-group"><label>Allow from (CIDRs)</label><div class="chip-input" id="guard-allow"></div><div class="hint">Client CIDRs exempt from the deny.</div></div>
    </div>

    <div class="card form-section ed-sub" data-type="rate-limit" style="${type === 'rate-limit' ? '' : 'display:none'}"><p class="section-label">Rate limit</p>
      <div class="inline-fields">
        <div class="field-group"><label>Requests / second</label><input class="field mono" id="rl-rps" type="number" step="0.1" value="${esc(rl.requestsPerSecond != null ? rl.requestsPerSecond : '')}" placeholder="10" /></div>
        <div class="field-group"><label>Burst</label><input class="field mono" id="rl-burst" type="number" value="${esc(rl.burst != null ? rl.burst : '')}" placeholder="ceil(rps)" /></div>
      </div>
    </div>
  </div></div>` + saveBar('middleware', isNew, meta.addLabel);

  const rolesCtl = makeChipInput($('#mw-roles'), arr(auth.requiredRoles), 'add role...');
  const mwAllowCtl = makeChipInput($('#mw-allow'), arr(auth.allowFrom), 'add CIDR...');
  const setReqCtl = makeKVRows($('#hdr-setreq'), headers.setRequest || {}, 'Header', 'value', false);
  const setRespCtl = makeKVRows($('#hdr-setresp'), headers.setResponse || {}, 'Header', 'value', false);
  const rmReqCtl = makeChipInput($('#hdr-rmreq'), arr(headers.removeRequest), 'add header...');
  const rmRespCtl = makeChipInput($('#hdr-rmresp'), arr(headers.removeResponse), 'add header...');
  const guardAllowCtl = makeChipInput($('#guard-allow'), arr(guard.allowFrom), 'add CIDR...');
  $$('.hdr-add').forEach((b) => b.addEventListener('click', () => { (b.dataset.wrap === 'hdr-setreq' ? setReqCtl : setRespCtl).addRow('', ''); }));

  const trigWrap = $('#guard-triggers'); const trigCtls = [];
  function trigRow(t) {
    t = t || {}; const d = document.createElement('div'); d.className = 'card form-section'; d.style.marginBottom = '8px';
    d.innerHTML = `<div class="row-between" style="margin-bottom:8px"><span class="ci-ty">trigger</span><button class="icon-btn trig-del" type="button" aria-label="Remove trigger">${ICON.x}</button></div>
      <div class="field-group"><label>Paths</label><div class="chip-input trig-paths"></div></div>
      <div class="field-group"><label>Methods</label><div class="chip-input trig-methods"></div></div>
      <div class="field-group"><label>Query equals</label><div class="trig-query"></div><button class="btn ghost sm trig-addq" type="button" style="margin-top:6px">${ICON.plus}Add</button></div>`;
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

  wireEditor('middleware', 'middlewares', meta, isNew, name || o.name, () => {
    const t = $('#ed-type').value; const body = { type: t };
    if (t === 'auth') {
      const idp = $('#mw-idp').value;
      if (!idp) { toast('Identity provider required', 'Select an identity provider.', 'err'); return null; }
      const mode = $('#mw-mode').value; const roles = rolesCtl.get(); const allow = mwAllowCtl.get();
      if (mode === 'auth-request' && roles.length) { toast('Roles unsupported', 'Required roles are not supported in auth-request mode.', 'err'); return null; }
      const spec = { identityProvider: idp };
      if (mode) spec.mode = mode;
      if (roles.length) spec.requiredRoles = roles;
      if (allow.length) spec.allowFrom = allow;
      body.auth = spec;
    } else if (t === 'headers') {
      const spec = {};
      const sr = setReqCtl.get(); if (Object.keys(sr).length) spec.setRequest = sr;
      const sp = setRespCtl.get(); if (Object.keys(sp).length) spec.setResponse = sp;
      const rr = rmReqCtl.get(); if (rr.length) spec.removeRequest = rr;
      const rp = rmRespCtl.get(); if (rp.length) spec.removeResponse = rp;
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
    } else {
      const rps = parseFloat($('#rl-rps').value);
      if (isNaN(rps) || rps <= 0) { toast('Rate required', 'Requests per second must be > 0.', 'err'); return null; }
      const spec = { requestsPerSecond: rps };
      const burst = parseInt($('#rl-burst').value, 10); if (!isNaN(burst)) spec.burst = burst;
      body.rateLimit = spec;
    }
    return body;
  });
}

// section -> editor dispatch for the typed object editors
const EDITORS = {
  redirects: redirectEditor, streams: streamEditor, dead: deadEditor, dns: dnsEditor,
  identity: idpEditor, access: accessEditor, middleware: middlewareEditor,
};

// ---------- ACCESS LOGS ----------
async function viewLogs(c) {
  const data = (await api('/api/logs')).data || {};
  const enabled = !!data.enabled;
  const entries = arr(data.entries);
  const statusClass = (s) => (s >= 500 ? 'err' : s >= 400 ? 'warn' : 'ok');
  const rows = entries.map((e) => `
    <tr>
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
      <div><h2>Access Logs</h2><p>Most recent data-plane requests, newest first (in-memory buffer).</p></div>
      <button class="btn" id="logsRefresh" type="button">${ICON.history}Refresh</button>
    </div>
    ${enabled ? '' : `<div class="card" style="margin-bottom:14px"><div class="hint">Request capture is <b>off</b>. Start gpm with <span class="mono">--access-log</span> (or <span class="mono">GPM_ACCESS_LOG=1</span>) to populate this view. The default off-path adds zero per-request overhead.</div></div>`}
    <div class="table-wrap">
      <table>
        <thead><tr><th>Time</th><th>Method</th><th>Host</th><th>Path</th><th>Status</th><th>Duration</th><th>Client</th></tr></thead>
        <tbody>${rows || `<tr><td colspan="7" class="muted" style="font-size:13px;padding:14px">${enabled ? 'No requests captured yet.' : 'Nothing to show while access logging is off.'}</td></tr>`}</tbody>
      </table>
    </div>`;

  $('#logsRefresh').addEventListener('click', () => viewLogs(c));
}

// ---------- HISTORY ----------
async function viewHistory(c) {
  const items = arr((await api('/api/history')).data);
  c.innerHTML = `
    <div class="view-head">
      <h2>History</h2>
      <p>Every change is a git commit. Reviewable, attributable, and reversible.</p>
    </div>
    <div class="card" style="margin-bottom:16px">
      <p class="section-label">Backup &amp; restore</p>
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <button class="btn" id="backupBtn">Download backup</button>
        <button class="btn" id="restoreBtn">Restore from archive…</button>
        <input type="file" id="restoreFile" accept=".gz,.tar.gz,application/gzip" style="display:none" />
        <span class="hint">A portable archive of the whole config. Restore replaces everything as one commit.</span>
      </div>
    </div>
    <div class="card">
      ${items.length ? `<div class="timeline">${items.map((h, i) => `
        <div class="tl-item">
          <div class="tl-meta">${esc(fmtTime(h.when))} · ${esc(h.author || 'unknown')}${h.email ? ` <span class="muted">&lt;${esc(h.email)}&gt;</span>` : ''}</div>
          <div class="tl-msg">${esc(h.message || '(no message)')}</div>
          <div class="tl-actions"><span class="sha">${esc(shortSha(h.hash))}</span>${i === 0 ? '<span class="revert disabled" title="Already the current config">current</span>' : `<span class="revert" data-revert="${esc(h.hash)}" title="Revert the whole config to this commit">revert</span>`}</div>
        </div>`).join('')}</div>` : '<div class="muted" style="font-size:13px">No commits yet.</div>'}
    </div>`;

  $('#backupBtn').addEventListener('click', () => { window.location.href = '/api/backup'; });
  const fileInput = $('#restoreFile');
  $('#restoreBtn').addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', async () => {
    const f = fileInput.files && fileInput.files[0];
    if (!f) return;
    if (!confirm(`Restore config from "${f.name}"? This REPLACES the entire current configuration (recorded as one commit).`)) { fileInput.value = ''; return; }
    try {
      const res = await fetch('/api/restore', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': csrfToken }, body: f });
      const data = await res.json().catch(() => null);
      if (!res.ok) throw new Error((data && data.error) || `Restore failed (${res.status})`);
      toast('Restored', data && data.commit ? `committed <span class="sha">${esc(shortSha(data.commit))}</span>` : 'configuration restored', 'ok', { html: true });
      await viewHistory(c);
    } catch (e) { toastErr(e); } finally { fileInput.value = ''; }
  });
  c.querySelectorAll('[data-revert]').forEach((el) => {
    el.addEventListener('click', async () => {
      const hash = el.getAttribute('data-revert');
      if (!confirm(`Revert the entire config to ${shortSha(hash)}? This is recorded as a new commit, so it can be undone.`)) return;
      try {
        const r = await api('/api/revert', { method: 'POST', body: { hash } });
        toast('Reverted', r.data && r.data.commit ? `committed <span class="sha">${esc(shortSha(r.data.commit))}</span>` : 'config reverted', 'ok', { html: true });
        await viewHistory(c);
      } catch (e) { toastErr(e); }
    });
  });
}

// ---------- SETTINGS ----------
async function viewSettings(c) {
  const s = (await api('/api/settings')).data || {};
  const admin = s.adminAuth || {};
  const bg = admin.breakGlass || {};
  c.innerHTML = `
    <div class="view-head"><h2>Settings</h2><p>Instance configuration and admin authentication.</p></div>
    <div class="grid-2" style="margin-bottom:16px">
      <div class="card form-section">
        <p class="section-label">Instance</p>
        <div class="field-group">
          <label>External base URL</label>
          <input class="field mono" id="set-url" value="${esc(s.externalBaseURL || '')}" placeholder="https://gpm.example.com" />
        </div>
        <div class="field-group">
          <label>Admin providers</label>
          <div class="chip-input" id="set-providers"></div>
          <div class="hint">Identity provider names allowed for admin sign-in. Enter to add.</div>
        </div>
      </div>
      <div class="card form-section">
        <p class="section-label">Admin authentication</p>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Local login</div><div class="ds">Username/password fallback</div></div>${switchHtml('set-local', !!admin.localLoginEnabled, 'Local login')}</div>
        <div class="toggle-line"><div class="tl-text"><div class="nm">SSO-only mode</div><div class="ds">All admin access goes through SSO</div></div>${switchHtml('set-sso', !!admin.ssoOnly, 'SSO-only mode')}</div>
        <div class="toggle-line"><div class="tl-text"><div class="nm">Break-glass localhost only</div><div class="ds">Emergency local access from loopback only</div></div>${switchHtml('set-bg', !!bg.localhostOnly, 'Break-glass localhost only')}</div>
      </div>
    </div>
    <div class="card form-section" style="margin-bottom:16px">
      <p class="section-label">Lifecycle webhooks</p>
      <p class="muted" style="font-size:11.5px;margin:0 0 10px">POST a JSON event to each URL after every config change (create/update/delete, restore, revert, settings). Delivery is async and best-effort, so a slow endpoint never blocks a save.</p>
      <div id="set-webhooks"></div>
      <button class="btn ghost sm" id="addWebhook" type="button" style="margin-top:6px">${ICON.plus}Add webhook</button>
    </div>
    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        <button class="btn primary" id="set-save" type="button">Save changes</button>
      </div>
    </div>`;

  const provCtl = makeChipInput($('#set-providers'), arr(admin.providers), 'add provider...');

  // webhook rows
  const whWrap = $('#set-webhooks');
  function webhookRow(w) {
    w = w || {};
    const div = document.createElement('div');
    div.className = 'loc-row';
    div.style.marginBottom = '8px';
    div.innerHTML = `
      <input class="field mono wh-name" style="flex:0 0 140px" value="${esc(w.name || '')}" placeholder="name" aria-label="Webhook name" />
      <input class="field mono wh-url" style="flex:2 1 220px" value="${esc(w.url || '')}" placeholder="https://hooks.example.com/x" aria-label="Webhook URL" />
      <input class="field mono wh-secret" style="flex:1 1 140px" value="${esc(w.secret || '')}" placeholder="secret (\${ENV:...} optional)" aria-label="Webhook secret" />
      <label class="check-item" style="flex:0 0 auto" title="Keep configured but do not fire"><input type="checkbox" class="wh-disabled"${w.disabled ? ' checked' : ''}/>off</label>
      <button class="icon-btn wh-del" type="button" aria-label="Remove webhook">${ICON.x}</button>`;
    div.querySelector('.wh-del').addEventListener('click', () => div.remove());
    whWrap.appendChild(div);
  }
  arr(s.webhooks).forEach(webhookRow);
  $('#addWebhook').addEventListener('click', () => webhookRow({}));

  $('#set-save').addEventListener('click', async () => {
    const body = {
      schemaVersion: s.schemaVersion,
      externalBaseURL: $('#set-url').value.trim(),
      adminAuth: {
        localLoginEnabled: isOn('set-local'),
        ssoOnly: isOn('set-sso'),
      },
    };
    const provs = provCtl.get();
    if (provs.length) body.adminAuth.providers = provs;
    if (isOn('set-bg') || (bg && Object.keys(bg).length)) {
      body.adminAuth.breakGlass = Object.assign({}, bg, { localhostOnly: isOn('set-bg') });
    }
    const webhooks = [];
    $$('#set-webhooks .loc-row').forEach((row) => {
      const name = row.querySelector('.wh-name').value.trim();
      const url = row.querySelector('.wh-url').value.trim();
      if (!name && !url) return;
      const wh = { name, url };
      const secret = row.querySelector('.wh-secret').value.trim();
      if (secret) wh.secret = secret;
      if (row.querySelector('.wh-disabled').checked) wh.disabled = true;
      webhooks.push(wh);
    });
    if (webhooks.length) body.webhooks = webhooks;
    const btn = $('#set-save'); btn.disabled = true;
    try {
      const r = await api('/api/settings', { method: 'PUT', body });
      toastSaved(r.commit); refreshHeadSha();
      // refresh instance label
      if (body.externalBaseURL) {
        try { state.instance = new URL(body.externalBaseURL).host || body.externalBaseURL; }
        catch (e) { state.instance = body.externalBaseURL; }
        const inst = $('.topbar .instance'); if (inst) inst.textContent = state.instance;
      }
    } catch (e) { toastErr(e); }
    btn.disabled = false;
  });
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

// ---------- boot ----------
async function boot() {
  buildShell();
  await loadTopbar();
  window.addEventListener('hashchange', route);
  await route();
}
boot();
