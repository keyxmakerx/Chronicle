/**
 * beside.js -- open a page beside the one you are on.
 *
 * Alt-click (Option-click on a Mac) on a link to a campaign page, Alt+Enter on
 * a focused one, or a beside icon (a[data-beside-open]) opens that page in a
 * pane beside the page it came from. Each pane is the real page in a frame
 * (the server draws it without the sidebar and header: layouts.IsPane), so
 * every widget in it runs exactly as on its own and nothing clashes with the
 * page next to it.
 *
 * How many pages fit comes from the width the page area has, never from a
 * size Chronicle picks: at least MIN_PANE_PX each, at most MAX_PAGES. With no
 * room for a second page, Alt-click opens the link in place, as a plain click
 * does. Opening from a page closes the panes after it; when the screen is full
 * the oldest pane folds behind an "Earlier" link in the next one.
 *
 * Holding Alt a moment shows where the page will open: an outline over that
 * space with its name inside, fainter while the pointer is over it, gone when
 * Alt is let go. A quick Alt-Tab never shows it.
 *
 * The open panes are kept for the tab (sessionStorage), so a page that loads
 * in full keeps them. Inside a pane this script only hands its links and keys
 * to the controller in the page around it.
 */
(function () {
  'use strict';

  var MIN_PANE_PX = 740;
  var MAX_PAGES = 4;
  var HINT_DELAY_MS = 250;
  var STORE_KEY = 'chronicle-beside';

  var C = (window.Chronicle = window.Chronicle || {});
  if (C.Beside) return;

  // ---- pure helpers (test/js/beside.test.mjs) ----

  // pagesFit is how many pages, the main one included, fit side by side.
  function pagesFit(widthPx) {
    return Math.max(1, Math.min(MAX_PAGES, Math.floor(widthPx / MIN_PANE_PX)));
  }

  // plan works out the panes after opening url from page `from` (0 is the
  // main page, n is the nth pane), with `fit` pages on screen. It returns the
  // panes that stay in view, the ones folded behind Earlier, what the new page
  // takes the place of, and its slot among the pages on screen.
  function plan(panes, earlier, from, url, fit) {
    var keep = panes.slice(0, from).concat([url]);
    var replaced = from < panes.length ? panes[from] : null;
    var foldNow = [];
    while (keep.length > fit - 1) foldNow.push(keep.shift());
    // Opening from the main page starts a new trail, so nothing earlier stays.
    var folded = (from === 0 ? [] : earlier).concat(foldNow);
    return { panes: keep, earlier: folded, replaced: replaced, slot: keep.length, pages: keep.length + 1, folds: foldNow.length ? foldNow[foldNow.length - 1] : null };
  }

  // beside reports whether a link is one beside.js may open: a page of this
  // site inside a campaign, not a download, a new tab or a jump on this page.
  function beside(href, origin) {
    if (!href) return false;
    var u;
    try { u = new URL(href, origin); } catch (e) { return false; }
    if (u.origin !== origin) return false;
    return /^\/campaigns\/[^/]+\/./.test(u.pathname);
  }

  C.Beside = { pagesFit: pagesFit, plan: plan, beside: beside };
  if (!window.document || !document.documentElement) return;

  function linkOf(t) {
    var a = t && t.closest ? t.closest('a[href]') : null;
    if (!a || a.hasAttribute('download') || (a.target && a.target !== '_self')) return null;
    if (a.closest('[contenteditable="true"], [data-no-beside]')) return null;
    return beside(a.getAttribute('href'), location.origin) ? a : null;
  }

  function nameOf(a) {
    var n = a.getAttribute('data-beside-name') || a.getAttribute('aria-label') || a.textContent || '';
    return n.replace(/\s+/g, ' ').trim() || 'This page';
  }

  // ---- inside a pane: hand everything to the page around it ----

  var inPane = !!document.querySelector('[data-beside-doc]');
  var host = null;
  if (inPane) {
    try { host = window.parent && window.parent !== window && window.parent.Chronicle && window.parent.Chronicle.Beside; } catch (e) { host = null; }
  }

  function controller() { return inPane ? host : C.Beside; }
  function myIndex() { return inPane && host && host.indexOf ? host.indexOf(window) : 0; }

  // ---- the controller, in the main page ----

  var row = null, state = { panes: [], earlier: [] }, els = [], ghost = null;

  function load() {
    try {
      var s = JSON.parse(sessionStorage.getItem(STORE_KEY) || 'null');
      if (s && Array.isArray(s.panes) && Array.isArray(s.earlier)) state = { panes: s.panes.filter(function (u) { return beside(u, location.origin); }), earlier: s.earlier.filter(function (u) { return beside(u, location.origin); }) };
    } catch (e) { /* no storage: panes last until the page loads in full */ }
  }
  function save() { try { sessionStorage.setItem(STORE_KEY, JSON.stringify(state)); } catch (e) { /* see load */ } }

  function fit() { return row ? pagesFit(row.getBoundingClientRect().width) : 1; }

  function setRoom() {
    var on = fit() >= 2;
    document.documentElement.toggleAttribute('data-beside-room', on);
    els.forEach(function (p) {
      try { p.frame.contentDocument.documentElement.toggleAttribute('data-beside-room', on); } catch (e) { /* still loading */ }
    });
  }

  var CLOSE_ICON = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>';
  var HERE_ICON = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M9.5 3.5h3v3M12.5 3.5L7 9M7 4H4a.5.5 0 0 0-.5.5v7.5a.5.5 0 0 0 .5.5h7.5a.5.5 0 0 0 .5-.5V9"/></svg>';

  function makePane(url) {
    var el = document.createElement('section');
    el.className = 'beside-pane';
    el.innerHTML = '<header class="beside-head"><button type="button" class="beside-earlier" hidden></button>' +
      '<span class="beside-title">Opening…</span>' +
      '<a class="beside-icon" data-beside-here title="Open here, in place of the main page" aria-label="Open here">' + HERE_ICON + '</a>' +
      '<button type="button" class="beside-icon" data-beside-close title="Close" aria-label="Close">' + CLOSE_ICON + '</button></header>' +
      '<iframe class="beside-frame"></iframe>';
    var p = { el: el, url: url, frame: el.querySelector('iframe'), title: el.querySelector('.beside-title') };
    el.querySelector('[data-beside-here]').setAttribute('href', url);
    p.frame.addEventListener('load', function () {
      try {
        var d = p.frame.contentDocument;
        p.url = p.frame.contentWindow.location.pathname + p.frame.contentWindow.location.search;
        var name = (d.title || '').split(' - ')[0] || 'Page';
        p.title.textContent = name;
        el.setAttribute('aria-label', name);
        p.frame.title = name;
        var i = els.indexOf(p);
        if (i >= 0) { state.panes[i] = p.url; save(); }
        p.el.querySelector('[data-beside-here]').setAttribute('href', p.url);
        setRoom();
      } catch (e) { /* a page the frame couldn't show stays as it is */ }
    });
    p.frame.src = url;
    return p;
  }

  // render makes the panes on screen match state, reusing frames that stay
  // so a pane already open never reloads.
  function render(fresh) {
    if (!row) return;
    var keep = [];
    state.panes.forEach(function (url, i) {
      var p = els[i] && els[i].url === url ? els[i] : null;
      if (!p) {
        p = els.filter(function (q) { return q.url === url && keep.indexOf(q) < 0; })[0] || makePane(url);
        if (url === fresh) p.el.classList.add('is-fresh');
      }
      keep.push(p);
    });
    els.forEach(function (p) { if (keep.indexOf(p) < 0) p.el.remove(); });
    keep.forEach(function (p, i) {
      if (row.children[i + 1] !== p.el) row.insertBefore(p.el, row.children[i + 1] || null);
      var e = p.el.querySelector('.beside-earlier');
      var last = i === 0 ? state.earlier[state.earlier.length - 1] : null;
      e.hidden = !last;
      if (last) { e.textContent = '← Earlier'; e.title = 'Back to the page before'; }
    });
    els = keep;
    row.classList.toggle('has-beside', els.length > 0);
    save();
    setRoom();
  }

  function open(url, from) {
    if (fit() < 2) { return false; }
    from = Math.max(0, Math.min(from || 0, state.panes.length));
    var r = plan(state.panes, state.earlier, from, url, fit());
    state = { panes: r.panes, earlier: r.earlier };
    render(url);
    return true;
  }

  function close(i) {
    state.panes.splice(i);
    if (!state.panes.length) state.earlier = [];
    render();
  }

  function back() {
    var url = state.earlier.pop();
    if (!url) return;
    state.panes.unshift(url);
    while (state.panes.length > fit() - 1) state.panes.pop();
    render();
  }

  // A narrower window holds fewer pages: the newest ones go.
  function refit() {
    var max = fit() - 1;
    if (state.panes.length > max) {
      state.panes = state.panes.slice(0, Math.max(0, max));
      if (!state.panes.length) state.earlier = [];
      render();
    }
    else setRoom();
  }

  function indexOf(win) {
    for (var i = 0; i < els.length; i++) if (els[i].frame.contentWindow === win) return i + 1;
    return 0;
  }

  // ---- the hint: where the page will open ----

  function hintFor(url, name, from) {
    var f = fit();
    if (f < 2 || !row) return null;
    var r = plan(state.panes, state.earlier, Math.min(from, state.panes.length), url, f);
    var box = row.getBoundingClientRect(), w = box.width / r.pages;
    var sub = r.replaced ? 'Opens beside, in place of ' + titleOf(r.replaced) : r.folds ? 'Opens beside; ' + titleOf(r.folds) + ' folds behind Earlier' : 'Opens beside';
    return { left: box.left + w * r.slot, top: box.top, width: w, height: box.height, name: name, sub: sub };
  }

  function titleOf(url) {
    for (var i = 0; i < els.length; i++) if (els[i].url === url) return els[i].title.textContent;
    return 'a page';
  }

  function showHint(h, px, py) {
    if (!ghost) {
      ghost = document.createElement('div');
      ghost.className = 'beside-ghost';
      ghost.setAttribute('aria-hidden', 'true');
      ghost.innerHTML = '<span class="beside-ghost-title"></span><span class="beside-ghost-sub"></span>';
      document.body.appendChild(ghost);
    }
    if (!h) { hideHint(); return; }
    ghost.style.left = h.left + 'px'; ghost.style.top = h.top + 'px';
    ghost.style.width = h.width + 'px'; ghost.style.height = h.height + 'px';
    ghost.firstChild.textContent = h.name;
    ghost.lastChild.textContent = h.sub;
    ghost.classList.add('is-on');
    under(px, py);
  }
  function under(px, py) {
    if (!ghost || !ghost.classList.contains('is-on') || px == null) return;
    var r = ghost.getBoundingClientRect();
    ghost.classList.toggle('is-under', px >= r.left && px <= r.right && py >= r.top && py <= r.bottom);
  }
  function hideHint() { if (ghost) ghost.classList.remove('is-on', 'is-under'); }

  // ---- Alt and the pointer, shared by the main page and every pane ----
  // Alt can go down in one document while the pointer is over a link in
  // another, so both live here, in the controller, in page coordinates.

  var altTimer = null, altOn = false, shown = false, hover = null;

  function hintNow() {
    if (!altOn || !hover) { hideHint(); return; }
    var h = hintFor(hover.href, hover.name, hover.from);
    if (h) shown = true;
    showHint(h, hover.x, hover.y);
  }
  function altDown() {
    if (altOn || altTimer) return;
    altTimer = setTimeout(function () { altTimer = null; altOn = true; hintNow(); }, HINT_DELAY_MS);
  }
  function altCancel() { if (altTimer) { clearTimeout(altTimer); altTimer = null; } }
  // altUp reports whether the hint was in use, so letting go of Alt shouldn't
  // also open the browser's menu bar.
  function altUp() {
    var was = shown;
    altCancel(); altOn = false; shown = false; hideHint();
    return was;
  }
  function pointer(link, from, x, y) {
    hover = link ? { href: link.getAttribute('href'), name: nameOf(link), from: from, x: x, y: y } : null;
    if (altOn) hintNow();
  }
  function moved(x, y) {
    if (!hover) return;
    hover.x = x; hover.y = y;
    under(x, y);
  }

  // ---- input, in the main page and in every pane ----

  function offset() {
    if (!inPane) return [0, 0];
    try { var r = window.frameElement.getBoundingClientRect(); return [r.left, r.top]; } catch (e) { return [0, 0]; }
  }

  function hrefOf(a) {
    var u = new URL(a.getAttribute('href'), location.origin);
    return u.pathname + u.search;
  }

  // go opens a beside, or here when there is no room for it.
  function go(a) {
    var ctl = controller();
    if (ctl && ctl.open(hrefOf(a), myIndex())) return;
    location.assign(a.href);
  }

  document.addEventListener('click', function (e) {
    var icon = e.target.closest && e.target.closest('a[data-beside-open]');
    var alt = e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.button === 0;
    var a = icon || (alt ? linkOf(e.target) : null);
    if (!a) return;
    // Alt-click on a link otherwise saves it in some browsers, so it always
    // opens a page: beside when there's room, here when not.
    e.preventDefault();
    e.stopPropagation();
    go(a);
  }, true);

  document.addEventListener('keydown', function (e) {
    var ctl = controller();
    if (!ctl || !ctl.altDown) return;
    if (e.key === 'Alt') { ctl.altDown(); return; }
    ctl.altCancel(); // Alt-Tab and other shortcuts never show the hint
    if (e.key === 'Enter' && e.altKey && !e.ctrlKey && !e.metaKey) {
      var a = linkOf(document.activeElement);
      if (a) { e.preventDefault(); go(a); }
    }
  }, true);
  document.addEventListener('keyup', function (e) {
    var ctl = controller();
    if (e.key === 'Alt' && ctl && ctl.altUp && ctl.altUp()) e.preventDefault();
  }, true);
  window.addEventListener('blur', function () {
    var ctl = controller();
    // Focus moving into a pane is not leaving the page.
    if (ctl && ctl.altUp && !(document.activeElement && document.activeElement.tagName === 'IFRAME')) ctl.altUp();
  });

  document.addEventListener('mouseover', function (e) {
    var ctl = controller();
    if (!ctl || !ctl.pointer) return;
    var a = linkOf(e.target) || (e.target.closest && e.target.closest('a[data-beside-open]'));
    var o = offset();
    ctl.pointer(a, myIndex(), e.clientX + o[0], e.clientY + o[1]);
  });
  document.addEventListener('mousemove', function (e) {
    var ctl = controller();
    if (!ctl || !ctl.moved) return;
    var o = offset();
    ctl.moved(e.clientX + o[0], e.clientY + o[1]);
  }, { passive: true });

  if (inPane) return;

  // ---- wiring the controller to the page ----

  Object.assign(C.Beside, { open: open, close: close, back: back, fit: fit, indexOf: indexOf, altDown: altDown, altCancel: altCancel, altUp: altUp, pointer: pointer, moved: moved });

  function start() {
    row = document.getElementById('beside-row');
    if (!row) return;
    load();
    row.addEventListener('click', function (e) {
      var pane = e.target.closest('.beside-pane');
      if (!pane) return;
      var i = els.map(function (p) { return p.el; }).indexOf(pane);
      if (e.target.closest('[data-beside-close]')) { close(i); return; }
      if (e.target.closest('.beside-earlier')) { back(); return; }
      if (e.target.closest('[data-beside-here]')) { state.panes.splice(i, 1); save(); }
    });
    if (window.ResizeObserver) new ResizeObserver(refit).observe(row);
    else window.addEventListener('resize', refit);
    if (state.panes.length) refit();
    render();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
