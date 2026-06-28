// go-proxy-manager admin SPA. Dependency-free vanilla ES module.

// ---------- icons ----------
const ICON = {
  arrow: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h16M13 6l7 6-7 6"/></svg>',
  grid: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>',
  globe: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3a9 9 0 100 18 9 9 0 000-18z"/><path d="M3 12h18M12 3c2.5 2.5 2.5 15 0 18M12 3c-2.5 2.5-2.5 15 0 18"/></svg>',
  redirect: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 7h11a4 4 0 014 4v0a4 4 0 01-4 4H4"/><path d="M8 3 4 7l4 4"/></svg>',
  stream: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 8h18M3 16h18M7 4v16M17 4v16"/></svg>',
  skull: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3a8 8 0 00-5 14v3h10v-3a8 8 0 00-5-14z"/><circle cx="9" cy="11" r="1.4"/><circle cx="15" cy="11" r="1.4"/></svg>',
  lock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg>',
  user: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0116 0"/></svg>',
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z"/></svg>',
  shieldCheck: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z"/><path d="M9.5 12l1.8 1.8L15 10"/></svg>',
  list: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 6h16M4 12h10M4 18h7"/></svg>',
  layers: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="5" rx="1.5"/><rect x="3" y="15" width="18" height="5" rx="1.5"/><path d="M8 9v6M16 9v6"/></svg>',
  headers: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 7h16M4 12h16M4 17h10"/></svg>',
  gauge: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 18a8 8 0 1116 0"/><path d="M12 18l4-5"/></svg>',
  history: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M3 12a9 9 0 109-9 9 9 0 00-7 3.3M3 3v4h4"/><path d="M12 7v5l3 2"/></svg>',
  cert: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg>',
  cog: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 00-.1-1.3l2-1.5-2-3.4-2.3 1a7 7 0 00-2.2-1.3L14 2h-4l-.4 2.2a7 7 0 00-2.2 1.3l-2.3-1-2 3.4 2 1.5A7 7 0 005 12a7 7 0 00.1 1.3l-2 1.5 2 3.4 2.3-1a7 7 0 002.2 1.3L10 22h4l.4-2.2a7 7 0 002.2-1.3l2.3 1 2-3.4-2-1.5A7 7 0 0019 12z"/></svg>',
  plus: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14"/></svg>',
  x: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  search: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/></svg>',
  server: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="5" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><path d="M7 8h.01M7 17h.01"/></svg>',
  clientUser: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="8" r="4"/><path d="M5 20a7 7 0 0114 0"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  commit: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="3"/><path d="M12 3v6M12 15v6M3 12h6M15 12h6"/></svg>',
  menu: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 6h16M4 12h16M4 18h16"/></svg>',
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
const state = { me: null, version: null, headSha: null, instance: 'go-proxy-manager', appName: 'Go Proxy Manager' };

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
  location.href = '/auth/login?return=' + encodeURIComponent('/');
}

// ---------- small UI builders ----------
function switchHtml(id, checked, label) {
  return `<button class="switch" type="button" role="switch" id="${id}" aria-checked="${checked ? 'true' : 'false'}"${label ? ` aria-label="${esc(label)}"` : ''}></button>`;
}
function isOn(id) { const el = document.getElementById(id); return el && el.getAttribute('aria-checked') === 'true'; }
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
    return `<tr class="clickable" data-name="${esc(h.name)}">
      <td><span class="host">${esc(primary)}${esc(extra)}</span>${h.displayName ? `<div class="faint" style="font-size:11px">${esc(h.displayName)}</div>` : ''}</td>
      <td class="mono">${esc(upStr)}</td>
      <td>${tls}</td>
      <td>${auth}</td>
      <td>${status}</td>
    </tr>`;
  }).join('');

  c.innerHTML = head + `
    <div class="toolbar">
      <div class="search">${ICON.search}<input class="field mono" id="hostFilter" placeholder="filter: domain, upstream, cert..." aria-label="Filter hosts" /></div>
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
    if (isOn('f-disabled')) obj.disabled = true;
    if (isOn('f-ws')) obj.websocketsUpgrade = true;

    const tlsObj = {};
    const cert = $('#f-cert').value;
    if (cert) tlsObj.certificateRef = cert;
    if (isOn('f-forcessl')) tlsObj.forceSSL = true;
    if (isOn('f-http2')) tlsObj.http2 = true;
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
    template: { name: '', type: 'oidc', oidc: { issuerURL: '', clientID: '', scopes: ['openid', 'profile', 'email'] }, roleMapping: { groupsClaim: 'groups', adminGroups: [], userGroups: [], defaultRole: 'user' } },
    summary: (o) => `<span class="k">Type</span><span class="v">${esc(o.type || '')}</span>` +
      (o.oidc ? `<span class="k">Issuer</span><span class="v">${esc(o.oidc.issuerURL || '')}</span><span class="k">Client ID</span><span class="v">${esc(o.oidc.clientID || '')}</span>` : '') +
      (o.forwardAuth ? `<span class="k">User header</span><span class="v">${esc(o.forwardAuth.userHeader || '')}</span>` : ''),
  },
  access: {
    title: 'Access Lists', sub: 'Reusable IP and basic-auth rules attached to hosts via the chain.',
    singular: 'access list', addLabel: 'Add access list',
    template: { name: '', satisfyAny: false, rules: [{ action: 'allow', cidr: '10.0.0.0/8' }, { action: 'deny', cidr: '0.0.0.0/0' }], defaultAction: 'deny' },
    summary: (o) => `<span class="k">Satisfy</span><span class="v">${o.satisfyAny ? 'any' : 'all'}</span>` +
      `<span class="k">Rules</span><span class="v">${arr(o.rules).length}</span>` +
      `<span class="k">Basic auth</span><span class="v">${arr(o.basicAuth).length} user(s)</span>` +
      (o.defaultAction ? `<span class="k">Default</span><span class="v">${esc(o.defaultAction)}</span>` : ''),
  },
  middleware: {
    title: 'Middleware', sub: 'Reusable, composable objects you drop into any host chain.',
    singular: 'middleware', addLabel: 'Add middleware',
    template: { name: '', type: 'headers', headers: { setResponse: {}, setRequest: {}, removeRequest: [], removeResponse: [] } },
    summary: (o) => `<span class="k">Type</span><span class="v">${esc(o.type || '')}</span>` +
      (o.auth ? `<span class="k">IdP</span><span class="v">${esc(o.auth.identityProvider || '')}</span>` : '') +
      (o.rateLimit ? `<span class="k">Rate</span><span class="v">${esc(o.rateLimit.requestsPerSecond)} r/s</span>` : ''),
  },
  dns: {
    title: 'DNS Providers', sub: 'Credentials used for ACME dns-01 challenges.',
    singular: 'DNS provider', addLabel: 'Add DNS provider',
    template: { name: '', provider: 'cloudflare', config: { apiToken: '${ENV:CF_API_TOKEN}' } },
    summary: (o) => `<span class="k">Provider</span><span class="v">${esc(o.provider || '')}</span>` +
      (o.config ? `<span class="k">Config keys</span><span class="v">${esc(Object.keys(o.config).join(', '))}</span>` : ''),
  },
  redirects: {
    title: 'Redirects', sub: '301/302 redirect rules for legacy or vanity hostnames.',
    singular: 'redirect', addLabel: 'Add redirect',
    template: { name: '', domains: [], forwardScheme: 'https', forwardDomain: '', statusCode: 301 },
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>` +
      (o.forwardDomain ? `<span class="k">To</span><span class="v">${esc((o.forwardScheme ? o.forwardScheme + '://' : '') + o.forwardDomain)}</span>` : '') +
      (o.statusCode ? `<span class="k">Code</span><span class="v">${esc(o.statusCode)}</span>` : ''),
  },
  streams: {
    title: 'Streams', sub: 'Raw TCP/UDP forwarding for non-HTTP services.',
    singular: 'stream', addLabel: 'Add stream',
    template: { name: '', listenPort: 0, forward: { host: '', port: 0 }, protocol: 'tcp' },
    summary: (o) => (o.listenPort != null ? `<span class="k">Listen</span><span class="v">:${esc(o.listenPort)}</span>` : '') +
      (o.forward ? `<span class="k">Forward</span><span class="v">${esc((o.forward.host || '') + ':' + (o.forward.port != null ? o.forward.port : ''))}</span>` : '') +
      (o.protocol ? `<span class="k">Protocol</span><span class="v">${esc(o.protocol)}</span>` : ''),
  },
  dead: {
    title: 'Dead hosts', sub: 'Hosts kept for 404 handling or scheduled decommission.',
    singular: 'dead host', addLabel: 'Add dead host',
    template: { name: '', domains: [] },
    summary: (o) => `<span class="k">Domains</span><span class="v">${esc(arr(o.domains).join(', '))}</span>`,
  },
};

async function genericSection(c, section, sub) {
  if (sub === '_new') return genericEditor(c, section, null);
  if (sub) return genericEditor(c, section, sub);
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

async function genericEditor(c, section, name) {
  const meta = SECTION_META[section];
  const plural = PLURAL[section];
  const isNew = !name;
  let obj;
  if (isNew) obj = JSON.parse(JSON.stringify(meta.template));
  else obj = (await api('/api/' + plural + '/' + encodeURIComponent(name)).then((r) => r.data)) || {};

  c.innerHTML = `
    <div class="row-between view-head">
      <div>
        <div class="muted" style="font-size:12px;margin-bottom:3px"><a href="#/${section}">${esc(meta.title)}</a> / ${isNew ? 'new' : 'edit'}</div>
        <h2 style="font-family:var(--display)">${esc(isNew ? 'New ' + meta.singular : name)}</h2>
        <p>Edit common fields, or the full object as JSON below. JSON is authoritative on save.</p>
      </div>
    </div>

    <div class="card form-section" style="margin-bottom:16px">
      <div class="inline-fields">
        <div class="field-group">
          <label>Name</label>
          <input class="field mono" id="gs-name" value="${esc(obj.name || '')}" ${isNew ? '' : 'disabled'} placeholder="name" />
          <div class="hint">${isNew ? 'Immutable after creation.' : 'Name is immutable.'}</div>
        </div>
      </div>
    </div>

    <div class="card form-section">
      <p class="section-label">Object (JSON)</p>
      <textarea class="field mono" id="gs-json" spellcheck="false" aria-label="Object JSON">${esc(JSON.stringify(obj, null, 2))}</textarea>
      <div class="hint">Full-fidelity editor. Validated with JSON.parse before save. The Name field above overrides the JSON name.</div>
    </div>

    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        ${isNew ? '' : `<button class="btn danger" id="gs-delete" type="button">${ICON.trash}Delete</button>`}
        <a class="btn ghost" href="#/${section}">Cancel</a>
        <button class="btn primary" id="gs-save" type="button">${isNew ? meta.addLabel : 'Save changes'}</button>
      </div>
    </div>`;

  $('#gs-save').addEventListener('click', async () => {
    const nm = isNew ? $('#gs-name').value.trim() : obj.name;
    if (!nm) { toast('Name required', 'Enter a name.', 'err'); return; }
    let body;
    try { body = JSON.parse($('#gs-json').value); }
    catch (e) { toast('Invalid JSON', e.message, 'err'); return; }
    if (!body || typeof body !== 'object' || Array.isArray(body)) { toast('Invalid object', 'JSON must be an object.', 'err'); return; }
    body.name = nm;
    const btn = $('#gs-save'); btn.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(nm), { method: 'PUT', body });
      toastSaved(r.commit); refreshHeadSha();
      location.hash = '#/' + section;
    } catch (e) { toastErr(e); btn.disabled = false; }
  });

  const del = $('#gs-delete');
  if (del) del.addEventListener('click', async () => {
    if (!confirm(`Delete ${meta.singular} "${obj.name}"?`)) return;
    del.disabled = true;
    try {
      const r = await api('/api/' + plural + '/' + encodeURIComponent(obj.name), { method: 'DELETE' });
      toast('Deleted', shortSha(r.commit) ? `committed <span class="sha">${esc(shortSha(r.commit))}</span>` : 'removed', 'ok', { html: true });
      refreshHeadSha(); location.hash = '#/' + section;
    } catch (e) { toastErr(e); del.disabled = false; }
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
    <div class="card">
      ${items.length ? `<div class="timeline">${items.map((h) => `
        <div class="tl-item">
          <div class="tl-meta">${esc(fmtTime(h.when))} · ${esc(h.author || 'unknown')}${h.email ? ` <span class="muted">&lt;${esc(h.email)}&gt;</span>` : ''}</div>
          <div class="tl-msg">${esc(h.message || '(no message)')}</div>
          <div class="tl-actions"><span class="sha">${esc(shortSha(h.hash))}</span><span class="revert disabled" title="Revert is coming soon">revert</span></div>
        </div>`).join('')}</div>` : '<div class="muted" style="font-size:13px">No commits yet.</div>'}
    </div>`;
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
    <div class="panel save-bar">
      <div class="save-note">${ICON.commit}Changes are committed to git as a new revision.</div>
      <div style="display:flex;gap:10px">
        <button class="btn primary" id="set-save" type="button">Save changes</button>
      </div>
    </div>`;

  const provCtl = makeChipInput($('#set-providers'), arr(admin.providers), 'add provider...');

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
