/**
 * site_admin.js — the way into and back out of Site admin.
 *
 * Site admin has its own menu, shown in place of the everyday one on admin
 * pages. A click on the footer's Site admin button remembers the page the
 * admin came from, so "Back to Chronicle" returns there. That click and the
 * click on "Back to Chronicle" each mark the way in or out for the next page
 * (site_admin_reveal.js plays it) and, where the browser has cross-document
 * view transitions, opt this page in so the next one can move over it. Only
 * those two clicks play anything; moving between admin pages does not.
 *
 * The remembered page lives in sessionStorage, which any script on the site
 * could write, so it is checked before use: a same-site path only, never one
 * that a browser would read as another site ("//host", "/\host"), and never
 * an admin page. Anything else falls back to the campaigns list.
 */
(function () {
  'use strict';
  window.Chronicle = window.Chronicle || {};
  if (window.Chronicle.siteAdminArrive) return;

  var RETURN_KEY = 'chronicle-admin-return';
  var VT_KEY = 'chronicle-admin-vt';
  var FALLBACK = '/campaigns';

  function store() {
    try { return window.sessionStorage; } catch (e) { return null; }
  }

  function underAdmin(path) {
    var p = path.split(/[?#]/)[0];
    return p === '/admin' || p.indexOf('/admin/') === 0;
  }

  // safeReturn returns path when it is a same-site page outside Site admin,
  // otherwise the fallback.
  function safeReturn(path) {
    if (typeof path !== 'string' || path.length < 1 || path.length > 2048) return FALLBACK;
    if (path.charAt(0) !== '/') return FALLBACK;
    var second = path.charAt(1);
    if (second === '/' || second === '\\') return FALLBACK;
    // Control characters and backslashes can be read as other URLs by some browsers.
    if (/[\u0000-\u001f\u007f\\]/.test(path)) return FALLBACK;
    if (underAdmin(path)) return FALLBACK;
    return path;
  }

  // optIn asks for a view transition on the navigation this click starts.
  // Motion off and browsers without cross-document transitions skip it.
  function optIn() {
    if (!window.CSSViewTransitionRule) return;
    if (document.documentElement.getAttribute('data-motion') === 'off') return;
    var st = document.createElement('style');
    st.textContent = '@view-transition{navigation:auto}';
    document.head.appendChild(st);
    // A navigation that never happens (a "leave this page?" prompt answered
    // no) must not leave every later one transitioning.
    setTimeout(function () { if (st.parentNode) st.parentNode.removeChild(st); }, 8000);
  }

  document.addEventListener('click', function (e) {
    var t = e.target && e.target.closest ? e.target : null;
    var entry = t && t.closest('[data-site-admin-entry]');
    var back = !entry && t && t.closest('[data-admin-back]');
    if (!entry && !back) return;
    // A new tab keeps this page as it is, so nothing to remember or play.
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var s = store();
    if (!s) return;
    var here = location.pathname + location.search;
    try {
      if (entry && !underAdmin(here)) {
        s.setItem(RETURN_KEY, safeReturn(here));
        s.setItem(VT_KEY, 'in');
        optIn();
      } else if (back && underAdmin(here)) {
        s.setItem(VT_KEY, 'out');
        optIn();
      }
    } catch (err) { /* storage full or blocked: the menu still works */ }
  });

  // siteAdminArrive points every way out ("Back to Chronicle" in the menu,
  // "Leave Site admin" in the top bar) at the remembered page. Called by the
  // admin menu's init, which also runs when a pin swaps the menu in again.
  window.Chronicle.siteAdminArrive = function (menu) {
    if (!menu) return;
    var s = store(), path = FALLBACK;
    if (s) {
      try { path = safeReturn(s.getItem(RETURN_KEY)); } catch (err) { /* fall back */ }
    }
    var backs = [menu.querySelector('[data-admin-back]')];
    if (document.querySelectorAll) backs = backs.concat([].slice.call(document.querySelectorAll('[data-admin-back]')));
    backs.forEach(function (b) { if (b) b.setAttribute('href', path); });
  };

  // Exposed for the unit test only.
  window.Chronicle._siteAdminSafeReturn = safeReturn;
})();
