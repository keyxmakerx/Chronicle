/**
 * site_admin_reveal.js — the way into and out of Site admin, as the new page
 * is shown.
 *
 * Loaded in <head>, before the first paint, because a page must ask for a
 * view transition before the browser shows it. site_admin.js marks the
 * footer click ("in") or "Back to Chronicle" ("out") in sessionStorage. Here
 * the new page names the direction on html[data-admin-vt] and, where the
 * browser has cross-document view transitions, opts in for this one
 * navigation (.admin-vt); input.css moves the page. The opt-in comes off
 * again once the transition ends, so other navigation is untouched.
 *
 * Without cross-document view transitions this page plays the move itself
 * (input.css): "in" is the admin page rising; "out" is a sheet in the control
 * room's colour that covers this page at first paint and falls away, so what
 * is uncovered is the page itself, never a blank colour. Motion off plays
 * nothing.
 */
(function () {
  'use strict';
  var KEY = 'chronicle-admin-vt';

  // plan decides what this page plays: "" (nothing), "vt" (a view transition)
  // or "self" (the page animates itself). A mark that doesn't match the page
  // (an "in" outside Site admin, an "out" inside it) is stale and ignored.
  function plan(dir, path, motion, hasVT) {
    if (dir !== 'in' && dir !== 'out') return '';
    var p = String(path || '').split(/[?#]/)[0];
    var admin = p === '/admin' || p.indexOf('/admin/') === 0;
    if ((dir === 'in') !== admin || motion === 'off') return '';
    return hasVT ? 'vt' : 'self';
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle._siteAdminRevealPlan = plan;

  var s, dir;
  try { s = window.sessionStorage; dir = s.getItem(KEY); } catch (e) { return; }
  if (!dir) return;
  var h = document.documentElement;
  var what = plan(dir, location.pathname, h.getAttribute('data-motion'), !!window.CSSViewTransitionRule);
  function clear() { try { s.removeItem(KEY); } catch (e) { /* nothing to clear */ } }
  if (!what) { clear(); return; }

  h.setAttribute('data-admin-vt', dir);
  var optIn = null;
  function done() {
    h.removeAttribute('data-admin-vt');
    h.classList.remove('admin-vt');
    if (optIn && optIn.parentNode) optIn.parentNode.removeChild(optIn);
    clear();
  }

  if (what === 'self') {
    // The rise plays on body; the fall on html::after, whose events reach html.
    var name = dir === 'in' ? 'admin-rise' : 'admin-fall';
    var end = function (e) {
      if (e && (e.target !== (dir === 'in' ? document.body : h) || e.animationName !== name)) return;
      document.removeEventListener('animationend', end);
      done();
    };
    document.addEventListener('animationend', end);
    // A move that never ends (a hidden tab) must not leave the page covered.
    setTimeout(end, 3000);
    return;
  }

  h.classList.add('admin-vt');
  optIn = document.createElement('style');
  optIn.textContent = '@view-transition{navigation:auto}';
  document.head.appendChild(optIn);
  window.addEventListener('pagereveal', function (e) {
    if (!e.viewTransition) { done(); return; }
    e.viewTransition.finished.then(done, done);
  }, { once: true });
})();
