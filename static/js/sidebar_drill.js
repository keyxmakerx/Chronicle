/**
 * sidebar_drill.js -- the category drill panel (nav.templ's .nav-drill)
 *
 * Clicking a category row slides a panel over the categories area only
 * (transform on .nav-drill; .nav-top above is never touched). The panel's
 * own content comes from GET /campaigns/:id/sidebar/drill/:slug
 * (SidebarDrillPanel), fetched with htmx.ajax so its hx-get search box and
 * lazy-loaded tree keep working exactly as the fragment declares them.
 *
 * A full page load on a category's own page (or a sub-category's) already
 * has the panel open and filled — NavActiveDrillSlug, rendered server-side
 * — so this only has to handle opening, switching straight from one
 * category to another, and closing. Closing (or never opening) puts the
 * panel out of the tab order and hidden from assistive tech via `inert`,
 * the same way the phone drawer already does.
 */
(function () {
  'use strict';

  var lastOpener = null;

  function panel() { return document.getElementById('sidebar-drill'); }

  function cssEscape(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, '\\$&');
  }

  function isOpen(p) { return !!p && p.classList.contains('is-open'); }

  /** The category button a freshly-opened-by-the-server panel belongs to:
   *  whichever row (a category itself, or one sitting under a current
   *  sub-category) the server already marked current — so Back, on a page
   *  never clicked into from this session, still returns focus somewhere
   *  sensible. */
  function initialOpener() {
    var direct = document.querySelector('#sidebar-nav-list [data-drill-open][aria-current="page"]');
    if (direct) return direct;
    var curSub = document.querySelector('#sidebar-nav-list .nav-row.nav-sub[aria-current="page"]');
    var rw = curSub && curSub.closest('.nav-rw');
    return (rw && rw.querySelector('[data-drill-open]')) || null;
  }

  /** The category button a sidebar row key belongs to: itself, when the row
   *  IS a category button, else its parent category's button when it is one
   *  of that category's sub-category rows. */
  function openerForKey(key) {
    var row = key && document.querySelector('#sidebar-nav-list [data-nav-key="' + cssEscape(key) + '"]');
    if (!row) return null;
    if (row.hasAttribute('data-drill-open')) return row;
    var rw = row.closest('.nav-rw');
    return rw ? rw.querySelector('[data-drill-open]') : null;
  }

  function setInert(p, on) {
    if (!p) return;
    if (on) p.setAttribute('inert', ''); else p.removeAttribute('inert');
  }

  /** Opens the panel to url, or — already open — just swaps its content, so
   *  clicking a second category never plays a close-then-open. */
  function openDrill(url, opener) {
    var p = panel();
    if (!p || !url) return;
    lastOpener = opener || lastOpener;
    setInert(p, false);
    p.classList.add('is-open');
    if (window.htmx && htmx.ajax) {
      htmx.ajax('GET', url, { target: '#sidebar-cat-content', swap: 'innerHTML' });
    }
  }

  function closeDrill() {
    var p = panel();
    if (!isOpen(p)) return;
    p.classList.remove('is-open');
    setInert(p, true);
    if (lastOpener && lastOpener.isConnected) lastOpener.focus({ preventScroll: true });
    lastOpener = null;
  }

  // --- Wiring ----------------------------------------------------------------

  document.addEventListener('click', function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    var opener = t.closest('[data-drill-open]');
    if (opener) {
      e.preventDefault();
      var url = opener.getAttribute('data-drill-open');
      if (url) openDrill(url, opener);
      return;
    }
    if (t.closest('[data-drill-back]')) {
      e.preventDefault();
      closeDrill();
    }
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var p = panel();
    if (isOpen(p) && p.contains(document.activeElement)) closeDrill();
  });

  // Focus lands on Back once the fetched panel has actually settled in.
  document.addEventListener('htmx:afterSwap', function (e) {
    var target = e.detail && e.detail.target;
    if (!target || target.id !== 'sidebar-cat-content') return;
    var p = panel();
    if (!isOpen(p)) return;
    var back = target.querySelector('[data-drill-back]');
    if (back) back.focus({ preventScroll: true });
  });

  // Boosted navigation never re-renders the sidebar (only #main-content), so
  // an open panel stays as it was through in-category browsing; leaving the
  // category entirely (Dashboard, a different category's own page, Manage…)
  // closes it rather than leaving it open over whatever is now current.
  document.addEventListener('htmx:afterSettle', function (e) {
    var target = e.detail && e.detail.target;
    if (!target || target.id !== 'main-content') return;
    var p = panel();
    if (!isOpen(p)) return;
    var marker = document.querySelector('#main-content [data-nav-current]');
    var key = marker ? marker.getAttribute('data-nav-current') || '' : '';
    var rowOpener = openerForKey(key);
    if (!rowOpener || rowOpener !== lastOpener) closeDrill();
  });

  function init() {
    var p = panel();
    if (!p) return;
    if (isOpen(p)) lastOpener = initialOpener();
    else setInert(p, true);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();

  // Back and Forward restore a whole new sidebar; its own inert state is
  // whatever the restored HTML says, so just re-read it.
  document.addEventListener('htmx:historyRestore', init);
})();
