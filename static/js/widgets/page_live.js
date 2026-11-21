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
 */
(function () {
  'use strict';

  // True when a refresh would not take work away from the reader.
  function isQuiet(s) {
    return !(s.dirty || s.editing || s.fieldFocused || s.dialogOpen);
  }

  var api = { isQuiet: isQuiet };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  if (typeof document === 'undefined') return;

  var DEBOUNCE = 1200;  // a quick burst of saves is one refresh
  var RETRY = 4000;

  function main() { return document.getElementById('main-content'); }

  // Closed dialogs stay in the page marked hidden, so only a shown one counts.
  function openDialog() {
    var all = document.querySelectorAll('dialog[open], [role="dialog"][aria-modal="true"]');
    for (var i = 0; i < all.length; i++) {
      if (all[i].getClientRects().length > 0) return true;
    }
    return false;
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
        // The swap replaces this widget, which ends this connection and
        // starts a new one on the fresh markup.
        window.htmx.ajax('GET', url, {
          target: m,
          select: '#main-content > *',
          swap: 'innerHTML',
          headers: { 'HX-Boosted': 'true' }
        }).then(function () {
          var m2 = main();
          if (m2 && location.pathname + location.search === url) m2.scrollTop = top;
        }, function () { /* the next change tries again */ }).then(function () { busy = false; });
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
