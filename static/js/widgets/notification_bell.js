/*
 * notification_bell.js — topbar notification bell (C-SCHED-P2).
 *
 * The bell button + unread badge are server-rendered in the app topbar; the
 * badge count is refreshed by an HTMX poll (hx-get /notifications/badge). This
 * widget owns the interactive dropdown: open/close, fetching the list, marking
 * items read, and mark-all. Live "pops" reuse the existing Chronicle.notify
 * toast. Scheduler-scoped in this slice — proposals are the only source.
 *
 * ES5 style to match the rest of static/js. Registered via Chronicle.register
 * and auto-mounted by boot.js on [data-widget="notification-bell"].
 */
(function () {
  'use strict';

  function $(sel, root) { return (root || document).querySelector(sel); }

  // relTime renders a short "3m ago" / "2h ago" / "Jul 4" label.
  function relTime(iso) {
    var then = new Date(iso).getTime();
    if (isNaN(then)) return '';
    var s = Math.max(0, Math.floor((Date.now() - then) / 1000));
    if (s < 60) return 'just now';
    var m = Math.floor(s / 60); if (m < 60) return m + 'm ago';
    var h = Math.floor(m / 60); if (h < 24) return h + 'h ago';
    var d = Math.floor(h / 24); if (d < 7) return d + 'd ago';
    return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function refreshBadge(el) {
    var badge = $('[data-notif-badge]', el);
    if (!badge) return;
    Chronicle.apiFetch('/notifications/badge')
      .then(function (r) { return r.ok ? r.text() : ''; })
      .then(function (html) { badge.innerHTML = html || ''; })
      .catch(function () { /* non-fatal */ });
  }

  function renderList(el, items) {
    var list = $('[data-notif-list]', el);
    if (!list) return;
    list.innerHTML = '';
    if (!items || !items.length) {
      var empty = document.createElement('div');
      empty.className = 'px-3 py-6 text-center text-sm text-fg-muted';
      empty.textContent = 'No notifications yet.';
      list.appendChild(empty);
      return;
    }
    items.forEach(function (n) {
      var item = document.createElement('a');
      item.href = n.link || '#';
      item.className = 'flex items-start gap-2 px-3 py-2 hover:bg-surface-alt transition-colors border-b border-edge last:border-0' + (n.read ? ' opacity-60' : '');
      var dot = document.createElement('span');
      dot.className = 'mt-1.5 w-1.5 h-1.5 rounded-full shrink-0 ' + (n.read ? 'bg-transparent' : 'bg-accent');
      item.appendChild(dot);
      var body = document.createElement('div');
      body.className = 'min-w-0';
      var msg = document.createElement('div');
      msg.className = 'text-sm text-fg';
      msg.textContent = n.message || 'Notification';
      var when = document.createElement('div');
      when.className = 'text-xs text-fg-muted mt-0.5';
      when.textContent = relTime(n.createdAt);
      body.appendChild(msg); body.appendChild(when);
      item.appendChild(body);
      item.addEventListener('click', function () {
        if (!n.read && n.id) {
          Chronicle.apiFetch('/notifications/' + encodeURIComponent(n.id) + '/read', { method: 'POST' })
            .then(function () { refreshBadge(el); }).catch(function () {});
        }
        // Let the anchor navigate normally.
      });
      list.appendChild(item);
    });
  }

  function loadList(el) {
    var list = $('[data-notif-list]', el);
    if (list) list.innerHTML = '<div class="px-3 py-6 text-center text-sm text-fg-muted">Loading…</div>';
    Chronicle.apiFetch('/notifications')
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (items) { renderList(el, items); })
      .catch(function () { renderList(el, []); });
  }

  Chronicle.register('notification-bell', {
    init: function (el) {
      var toggle = $('[data-notif-toggle]', el);
      var panel = $('[data-notif-panel]', el);
      var markAll = $('[data-notif-markall]', el);
      if (!toggle || !panel) return;

      function open() { panel.hidden = false; loadList(el); }
      function close() { panel.hidden = true; }

      toggle.addEventListener('click', function (e) {
        e.stopPropagation();
        if (panel.hidden) open(); else close();
      });
      document.addEventListener('click', function (e) {
        if (!panel.hidden && !el.contains(e.target)) close();
      });
      if (markAll) {
        markAll.addEventListener('click', function (e) {
          e.preventDefault(); e.stopPropagation();
          Chronicle.apiFetch('/notifications/read-all', { method: 'POST' })
            .then(function () { refreshBadge(el); loadList(el); })
            .catch(function () {});
        });
      }
      el._notifClose = close;
    },
    destroy: function (el) {
      if (el && el._notifClose) el._notifClose();
    }
  });
})();
