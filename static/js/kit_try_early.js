/**
 * kit_try_early.js — an open "Try a style kit here" is put back on before
 * the first paint of a page loaded in full, so the saved kit never shows
 * first (kit_try.js runs the strip; this only puts the kit on).
 *
 * Loaded in <head>, before the first paint. It acts only where the server
 * says the owner's brush is on this page (html[data-kit-try-ok], drawn when
 * the brush is and pages wear the Material look), only in the top window (a
 * framed page is driven by its parent, kit_try.js), and only for the
 * campaign the try-out was started in. What the server drew is kept on
 * data-kit-saved and data-kit-saved-entrance, for "Put it back".
 */
(function () {
  'use strict';
  try {
    var h = document.documentElement;
    if (window.top !== window || !h.hasAttribute('data-kit-try-ok')) return;
    var s = JSON.parse(sessionStorage.getItem('chronicle-kit-try') || 'null');
    if (!s || typeof s.css !== 'string' || typeof s.url !== 'string' || !/^[a-z]+$/.test(s.kit || '')) return;
    var m = /^\/campaigns\/[^/?#]+/.exec(s.url);
    var p = location.pathname;
    if (!m || (p !== m[0] && p.indexOf(m[0] + '/') !== 0)) return;
    h.setAttribute('data-kit-saved', h.getAttribute('data-cz-kit') || 'material');
    var e = h.getAttribute('data-cz-entrance');
    if (e) h.setAttribute('data-kit-saved-entrance', e);
    var st = document.createElement('style');
    st.setAttribute('data-kit-try-css', '');
    st.textContent = s.css;
    document.head.appendChild(st);
    h.setAttribute('data-kit-try', s.kit);
    if (s.kit === 'material') {
      h.removeAttribute('data-cz-kit');
      if (s.entrance && /^[a-z]+$/.test(s.entrance)) h.setAttribute('data-cz-entrance', s.entrance);
    } else {
      h.setAttribute('data-cz-kit', s.kit);
      h.removeAttribute('data-cz-entrance');
    }
  } catch (x) { /* storage refused: the page shows as saved */ }
})();
