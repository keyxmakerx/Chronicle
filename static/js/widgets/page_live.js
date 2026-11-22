/**
 * page_live.js — an open page fetches itself again when someone else changes it.
 * Mounted by an empty `page-live` widget on the page (data-campaign-id,
 * data-entity-id), so it loads only where it is used and ends with the page.
 *
 * The server sends a content-free `page.changed` message for a page everyone
 * may read. This reloads the page's own markup through the same boosted swap
 * the sidebar uses, so what a reader sees is still filtered per reader on the
 * server. A refresh never replaces something a person is working on: it waits
 * while a page editor is open, a form has unsaved changes, focus is in a field
 * or a dialog is open, and tries again until the moment is quiet.
 *
 * Widgets on the fresh markup often draw themselves only after their own
 * data arrives, so for a moment the page is shorter than it was. The old
 * height is held by an empty block under the content while they draw, and the
 * reader's scroll position is put back each frame until they scroll
 * themselves, the page has grown back, or HOLD_MS has passed.
 */
(function () {
  'use strict';

  // True when a refresh would not take work away from the reader.
  function isQuiet(s) {
    return !(s.dirty || s.editing || s.fieldFocused || s.dialogOpen);
  }

  // The held height that keeps the page as tall as it was: what the content
  // now lacks of the old height, never negative.
  function holdFor(oldHeight, natural) {
    return Math.max(0, Math.round(oldHeight - natural));
  }

  var api = { isQuiet: isQuiet, holdFor: holdFor };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  var DEBOUNCE = 1200;  // a quick burst of saves is one refresh
  var RETRY = 4000;
  var HOLD_MS = 4000;
  var SETTLED_FRAMES = 20;

  function main() { return document.getElementById('main-content'); }

  // Closed dialogs stay in the page marked hidden, so only a shown one counts.
  function openDialog() {
    var all = document.querySelectorAll('dialog[open], [role="dialog"][aria-modal="true"]');
    for (var i = 0; i < all.length; i++) {
      if (all[i].getClientRects().length > 0) return true;
    }
    return false;
  }

  // Where the content of m ends, measured at `mark`, an empty block after it,
  // or at a scratch one: scrollHeight can't say, since it never reads less
  // than the scroller's own height and it counts any hold in place.
  function contentEnd(m, mark) {
    var scratch = !mark;
    if (scratch) {
      mark = document.createElement('div');
      mark.style.cssText = 'display:block;height:0;margin:0;padding:0;border:0';
      m.appendChild(mark);
    }
    var y = mark.getBoundingClientRect().top - m.getBoundingClientRect().top - m.clientTop +
      m.scrollTop + (parseFloat(getComputedStyle(m).paddingBottom) || 0);
    if (scratch) m.removeChild(mark);
    return y;
  }

  // The hold of the refresh in progress, so a refresh that comes while the
  // last one still holds the page takes it over rather than measuring it.
  var active = null;

  // Keeps the content of m as tall as it was and its scroll at `top` while
  // the swapped-in widgets draw: follow() starts after the swap, end() lets
  // go. The height is held by an empty block after the new content.
  function holdPlace(m, top) {
    var prev = active && active.m === m ? active : null;
    var height = prev ? Math.max(prev.height, contentEnd(m, prev.spacer())) : contentEnd(m, null);
    if (active) active.end();
    var spacer = null, moved = false, done = false, still = 0, landed = false;
    var start = 0;
    // The browser's own scroll anchoring would move the page when a widget
    // grows above the reader; the position is kept here instead.
    var anchor = m.style.overflowAnchor;
    m.style.overflowAnchor = 'none';
    // A reader within a screen of the bottom also gets that much room under
    // the held height, so a widget folding away between two frames cannot
    // pull them up; the room goes with the hold. A reader at the top cannot
    // be pulled up, so a page that fits its window never grows a scrollbar.
    var room = top > 0 ? Math.max(0, top + 2 * m.clientHeight - height) : 0;
    // Returns what the content still lacks of the old height.
    function size() {
      if (!spacer) return 0;
      if (spacer.parentNode !== m) m.appendChild(spacer);
      var natural = contentEnd(m, spacer);
      spacer.style.height = holdFor(height + room, natural) + 'px';
      return holdFor(height, natural);
    }
    function touched() { moved = true; }
    var evs = ['wheel', 'touchstart', 'keydown', 'pointerdown'];
    evs.forEach(function (t) { m.addEventListener(t, touched, { passive: true }); });
    // In before the new markup is painted, so the reader never sees the page
    // shrink under them.
    function swapped(e) {
      if (done || !e.detail || e.detail.target !== m) return;
      spacer = document.createElement('div');
      spacer.setAttribute('aria-hidden', 'true');
      spacer.setAttribute('data-page-live-hold', '');
      spacer.style.cssText = 'display:block;height:0;margin:0;padding:0;border:0;pointer-events:none';
      m.appendChild(spacer);
      // Until the swap settles the new markup can still shrink by any amount
      // (htmx puts its settled attributes back), so a reader who could be
      // pulled up gets the whole old height then.
      if (top > 0) spacer.style.height = (height + room) + 'px';
      else size();
      if (!moved) m.scrollTop = top;
    }
    // htmx scrolls the target to the top when the swap settles; put it back
    // before anything is painted.
    function settled(e) {
      if (done || !e.detail || e.detail.target !== m) return;
      landed = true;
      size();
      if (!moved) m.scrollTop = top;
    }
    document.addEventListener('htmx:afterSwap', swapped);
    document.addEventListener('htmx:afterSettle', settled);
    function end() {
      if (done) return;
      done = true;
      if (active === self) active = null;
      document.removeEventListener('htmx:afterSwap', swapped);
      document.removeEventListener('htmx:afterSettle', settled);
      evs.forEach(function (t) { m.removeEventListener(t, touched); });
      if (spacer && spacer.parentNode) spacer.parentNode.removeChild(spacer);
      spacer = null;
      m.style.overflowAnchor = anchor;
    }
    function frame() {
      if (done) return;
      var h = landed ? size() : height;
      var off = m.scrollTop !== top;
      if (!moved && off) m.scrollTop = top;
      // Watched for a moment after the page has grown back too, since the
      // settle comes a frame or two after the swap.
      still = landed && !h && !off ? still + 1 : 0;
      if (moved || still >= SETTLED_FRAMES || Date.now() - start > HOLD_MS) { end(); return; }
      requestAnimationFrame(frame);
    }
    var self = {
      m: m,
      height: height,
      spacer: function () { return spacer; },
      follow: function () { start = Date.now(); requestAnimationFrame(frame); },
      end: end
    };
    active = self;
    return self;
  }

  function state() {
    var m = main();
    var a = document.activeElement;
    var field = !!(a && m && m.contains(a) &&
      (/^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName) || a.isContentEditable));
    return {
      dirty: !!(window.Chronicle && Chronicle.isDirty && Chronicle.isDirty()),
      editing: !!document.querySelector('.chronicle-editor--editing'),
      fieldFocused: field,
      dialogOpen: openDialog()
    };
  }

  Chronicle.register('page-live', {
    init: function (el, config) {
      var campaignId = config.campaignId || '';
      var entityId = config.entityId || '';
      var url = location.pathname + location.search;
      var ws = null, wsTimer = null, wsDelay = 2000, pending = null;
      var destroyed = false, busy = false;
      if (!campaignId || !entityId || typeof window.WebSocket !== 'function') return;

      function schedule(delay) {
        if (pending) clearTimeout(pending);
        pending = setTimeout(attempt, delay);
      }

      function attempt() {
        pending = null;
        if (destroyed || busy || location.pathname + location.search !== url) return;
        if (document.visibilityState === 'hidden' || !isQuiet(state())) { schedule(RETRY); return; }
        var m = main();
        if (!m || !window.htmx || typeof window.htmx.ajax !== 'function') return;
        busy = true;
        var top = m.scrollTop;
        var place = holdPlace(m, top);
        // The swap replaces this widget, which ends this connection and
        // starts a new one on the fresh markup.
        window.htmx.ajax('GET', url, {
          target: m,
          select: '#main-content > *',
          swap: 'innerHTML',
          headers: { 'HX-Boosted': 'true' }
        }).then(function () {
          if (main() === m && location.pathname + location.search === url) place.follow();
          else place.end();
        }, function () { place.end(); }).then(function () { busy = false; });
      }

      function connect() {
        if (destroyed) return;
        try {
          var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
          var sock = new WebSocket(proto + '//' + location.host + '/ws?campaign=' + encodeURIComponent(campaignId));
          var opened = false;
          ws = sock;
          sock.addEventListener('open', function () { opened = true; wsDelay = 2000; });
          sock.addEventListener('message', function (ev) {
            var msg;
            try { msg = JSON.parse(ev.data); } catch (e) { return; }
            if (msg && msg.type === 'page.changed' && msg.campaignId === campaignId && msg.resourceId === entityId) schedule(DEBOUNCE);
          });
          sock.addEventListener('error', function (e) { if (e.preventDefault) e.preventDefault(); });
          sock.addEventListener('close', function () {
            if (ws !== sock) return;
            ws = null;
            // Never opened: the reader is not signed in, nothing to follow.
            if (destroyed || !opened) return;
            wsTimer = setTimeout(connect, wsDelay);
            wsDelay = Math.min(wsDelay * 2, 60000);
          });
        } catch (e) { ws = null; }
      }

      connect();

      return {
        destroy: function () {
          destroyed = true;
          if (pending) clearTimeout(pending);
          if (wsTimer) clearTimeout(wsTimer);
          if (ws) { var s = ws; ws = null; try { s.close(); } catch (e) { /* already closed */ } }
        }
      };
    }
  });
})();
