// Applies a saved explicit theme choice before first paint, so a light/dark
// pick never flashes the other palette. "auto" (or unset) leaves no data-theme
// attribute and app.css's prefers-color-scheme media queries decide instead.
// Kept in sync with the gpm.theme key app.js reads/writes.
//
// A SEPARATE FILE, not an inline <script>: the admin listener sends
// Content-Security-Policy "script-src 'self'", which blocks inline execution -
// so as an inline block this never ran and the flash guard it exists for did
// nothing. Loaded synchronously (no defer, no module) in <head> before the
// stylesheet, which is what makes it beat first paint.
(function () {
  try {
    var t = localStorage.getItem('gpm.theme');
    if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  } catch (e) { /* ignore */ }
})();
