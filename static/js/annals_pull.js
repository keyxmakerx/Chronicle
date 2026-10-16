/**
 * annals_pull.js — following a connection under the Annals page look.
 *
 * A click on a link in the connections block opens the linked page without
 * leaving the one being read: the old page stays where it is and dims, the
 * new page pulls out from under the connections block, and the block itself
 * stays, swapping its links in place. Links both pages share move to their
 * new place, links only the old page had fold away, and the new page's own
 * links fold in with a glow, the page you came from among them. Following
 * the link back to the page you came from plays it in reverse: the page
 * slides back under the block. Calm fades instead; Off and a device that
 * reduces motion leave the link as an ordinary page load.
 *
 * It is a same-document view transition around a boosted swap of
 * #main-content, the same swap the sidebar uses, so the page's widgets,
 * history entry, title and sidebar marker all come from the usual htmx path.
 * The scroll resets inside the transition, after the old page is captured:
 * resetting it first would show the old page jump before the move starts.
 * page_look_annals.css draws the move from html[data-annals-mode],
 * [data-annals-pull] and [data-annals-side].
 */
(function () {
  'use strict';

  // What a click plays: "" (an ordinary page load), "slide" or "fade".
  function modeOf(o) {
    if (!o.annals || !o.hasVT || o.reducedMotion || o.motion === 'off') return '';
    if (o.motion === 'calm' || o.czReduce || o.viewMotion === 'calm') return 'fade';
    return 'slide';
  }

  // Which side of the page the connections block sits on, so the new page
  // comes out from under it. A block across the middle counts as right.
  function sideOf(rail, main) {
    var mid = main.left + main.width / 2;
    return rail.left + rail.width / 2 < mid - main.width / 6 ? 'left' : 'right';
  }

  // Following a link back to the page the last pull came from reverses it.
  // trail holds one {from, to} per pull, newest last.
  function isBack(trail, here, to) {
    var top = trail.length ? trail[trail.length - 1] : null;
    return !!top && top.to === here && top.from === to;
  }

  // The transition name a row takes, from the id of the page it links to.
  function rowName(id) {
    var s = String(id || '').replace(/[^A-Za-z0-9_-]/g, '');
    return s ? 'annals-rel-' + s : '';
  }

  function lastSegment(path) {
    var p = String(path || '').split(/[?#]/)[0].replace(/\/+$/, '');
    return p.slice(p.lastIndexOf('/') + 1);
  }

  var api = { modeOf: modeOf, sideOf: sideOf, isBack: isBack, rowName: rowName, lastSegment: lastSegment };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  var H = document.documentElement;
  var RAIL = '[data-widget="relations"]';
  var LINK = RAIL + ' .rel-item .rel-name a[href]';
  // Enough rows to cover any real page; past it rows travel with the block.
  var MAX_ROWS = 80;
  // How long the new page waits for its connections to arrive before it
  // moves anyway, so a slow fetch never holds the click up.
  var RAIL_WAIT = 700;
  var trail = [];
  var busy = false;

  function currentMode() {
    var reduce = false;
    try { reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) { /* treated as no */ }
    return modeOf({
      annals: H.getAttribute('data-cz-page') === 'annals',
      hasVT: typeof document.startViewTransition === 'function',
      reducedMotion: reduce,
      motion: H.getAttribute('data-motion'),
      czReduce: H.hasAttribute('data-cz-reduce'),
      viewMotion: H.getAttribute('data-view-motion')
    });
  }

  // Name each row once by the page it links to; a page linked twice keeps
  // its second row inside the block. Returns the ids it named.
  function nameRows(rail) {
    var seen = {}, n = 0;
    Array.prototype.forEach.call(rail.querySelectorAll('.rel-item'), function (row) {
      var a = row.querySelector('.rel-name a[href]');
      var id = a ? lastSegment(a.getAttribute('href')) : '';
      var name = rowName(id);
      if (!name || seen[id] || n >= MAX_ROWS) return;
      seen[id] = true; n++;
      row.style.viewTransitionName = name;
      row.style.viewTransitionClass = 'annals-rel';
      row.setAttribute('data-annals-id', id);
    });
    return seen;
  }

  function waitForRail(main) {
    return new Promise(function (resolve) {
      var t = setTimeout(done, RAIL_WAIT);
      function on(ev) { if (main.contains(ev.target)) done(); }
      function done() {
        clearTimeout(t);
        document.removeEventListener('chronicle:relations-rendered', on);
        resolve();
      }
      document.addEventListener('chronicle:relations-rendered', on);
    });
  }

  function swap(link, url, main, mode, oldIds, fromId) {
    var railReady = waitForRail(main);
    link.setAttribute('hx-push-url', 'true');
    return window.htmx.ajax('GET', url.pathname + url.search, {
      source: link,
      target: main,
      select: '#main-content > *',
      swap: 'innerHTML',
      headers: { 'HX-Boosted': 'true' }
    }).then(function () {
      link.removeAttribute('hx-push-url');
      if (location.pathname !== url.pathname) throw new Error('page not swapped');
      main.scrollTop = 0;
      if (!main.querySelector(RAIL)) return;
      return railReady.then(function () {
        var rail = main.querySelector(RAIL);
        if (!rail || mode !== 'slide') return;
        rail.style.viewTransitionName = 'annals-rail';
        var ids = nameRows(rail);
        Object.keys(ids).forEach(function (id) {
          var row = rail.querySelector('.rel-item[data-annals-id="' + id + '"]');
          if (id === fromId) row.setAttribute('data-annals-from', '');
          else if (!oldIds[id]) row.setAttribute('data-annals-in', '');
        });
      });
    });
  }

  function pull(link, url, mode, main) {
    busy = true;
    var rail = link.closest(RAIL);
    var here = location.pathname;
    var back = isBack(trail, here, url.pathname);
    var oldIds = {};
    if (window.Chronicle && Chronicle.hovercard && Chronicle.hovercard.close) Chronicle.hovercard.close();

    H.setAttribute('data-annals-mode', mode);
    H.setAttribute('data-annals-pull', back ? 'back' : 'out');
    H.setAttribute('data-annals-side', sideOf(rail.getBoundingClientRect(), main.getBoundingClientRect()));
    main.style.viewTransitionName = 'annals-page';
    if (mode === 'slide') {
      rail.style.viewTransitionName = 'annals-rail';
      oldIds = nameRows(rail);
    }

    var failed = false;
    var vt = document.startViewTransition(function () {
      return swap(link, url, main, mode, oldIds, lastSegment(here)).catch(function (err) {
        failed = true;
        throw err;
      });
    });
    vt.finished.catch(function () { /* the swap's own failure is handled below */ }).then(function () {
      main.style.viewTransitionName = '';
      H.removeAttribute('data-annals-mode');
      H.removeAttribute('data-annals-pull');
      H.removeAttribute('data-annals-side');
      Array.prototype.forEach.call(main.querySelectorAll('[style*="view-transition"]'), function (el) {
        el.style.viewTransitionName = '';
        el.style.viewTransitionClass = '';
      });
      busy = false;
      if (failed) { location.href = url.href; return; }
      if (back) trail.pop(); else trail.push({ from: here, to: url.pathname });
    });
  }

  document.addEventListener('click', function (e) {
    if (busy || e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var link = e.target.closest && e.target.closest(LINK);
    var main = document.getElementById('main-content');
    if (!link || !main || !main.contains(link) || link.target) return;
    var url = new URL(link.href, location.href);
    if (url.origin !== location.origin || url.pathname === location.pathname) return;
    // Unsaved changes keep the ordinary load, so the browser still asks.
    if (window.Chronicle && Chronicle.isDirty && Chronicle.isDirty()) return;
    var mode = currentMode();
    if (!mode || !window.htmx || typeof window.htmx.ajax !== 'function') return;
    e.preventDefault();
    pull(link, url, mode, main);
  });

  // Any other navigation ends the trail, so a later link back is a new pull.
  document.addEventListener('htmx:historyRestore', function () { trail = []; });
  document.addEventListener('htmx:pushedIntoHistory', function () {
    var top = trail[trail.length - 1];
    if (!busy && top && top.to !== location.pathname) trail = [];
  });
})();
