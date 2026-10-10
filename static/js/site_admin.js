/**
 * site_admin.js — the way into and back out of Site admin.
 *
 * Site admin has its own menu, shown in place of the everyday one on admin
 * pages. A click on the footer's Site admin button remembers the page the
 * admin came from, so "Back to Chronicle" returns there, and asks the admin
 * menu to arrive with its weighted entrance. Only that click plays the
 * entrance; moving between admin pages does not.
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
  var ARRIVE_KEY = 'chronicle-admin-arrive';
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

  document.addEventListener('click', function (e) {
    var a = e.target && e.target.closest && e.target.closest('[data-site-admin-entry]');
    if (!a) return;
    // A new tab keeps this page as it is, so nothing to remember or play.
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var s = store();
    if (!s) return;
    var here = location.pathname + location.search;
    try {
      if (!underAdmin(here)) {
        s.setItem(RETURN_KEY, safeReturn(here));
        s.setItem(ARRIVE_KEY, '1');
      }
    } catch (err) { /* storage full or blocked: the menu still works */ }
  });

  // siteAdminArrive points "Back to Chronicle" at the remembered page and, after
  // the footer click, plays the entrance once. Called by the admin menu's init.
  window.Chronicle.siteAdminArrive = function (menu) {
    if (!menu) return;
    var s = store(), back = menu.querySelector('[data-admin-back]');
    var path = FALLBACK, arrive = false;
    if (s) {
      try {
        path = safeReturn(s.getItem(RETURN_KEY));
        arrive = s.getItem(ARRIVE_KEY) === '1';
        s.removeItem(ARRIVE_KEY);
      } catch (err) { /* fall back */ }
    }
    if (back) back.setAttribute('href', path);
    if (arrive) {
      var sb = menu.closest('.nav-sb') || menu;
      sb.classList.add('admin-arrive');
      sb.addEventListener('animationend', function done(ev) {
        if (ev.target !== menu) return;
        sb.classList.remove('admin-arrive');
        sb.removeEventListener('animationend', done);
      });
    }
  };

  // Exposed for the unit test only.
  window.Chronicle._siteAdminSafeReturn = safeReturn;
})();
