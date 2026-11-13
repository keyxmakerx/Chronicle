/*
 * notification_bell.js — topbar notification bell.
 *
 * The bell button + unread badge are server-rendered in the app topbar; the
 * badge count is refreshed by an HTMX poll (hx-get /notifications/badge). This
 * widget owns the interactive dropdown: open/close, fetching the list, marking
 * one item read (clicking it, or its row-end check icon), and mark-all. Live
 * "pops" reuse the existing Chronicle.notify toast.
 *
 * Registered via Chronicle.register and auto-mounted by boot.js on
 * [data-widget="notification-bell"].
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

  // A notification type with its own icon; the rest show only the unread dot.
  var TYPE_ICON = { item_given: 'fa-gift' };
  // Types only a DM is sent carry the Director's amber edge.
  var DM_TYPES = { game_night_checkin: true };

  // syncMenuCount copies the unread count into the account menu's
  // Notifications row, which stands in for the bell on a narrow phone.
  function syncMenuCount(el) {
    var badge = $('[data-count]', el);
    var n = badge ? badge.getAttribute('data-count') : '';
    var label = badge ? badge.textContent : '';
    var spots = document.querySelectorAll('[data-notif-menu-count]');
    for (var i = 0; i < spots.length; i++) {
      spots[i].textContent = n ? label : '';
      spots[i].hidden = !n;
    }
  }

  function refreshBadge(el) {
    var badge = $('[data-notif-badge]', el);
    if (!badge) return;
    Chronicle.apiFetch('/notifications/badge')
      .then(function (r) { return r.ok ? r.text() : ''; })
      .then(function (html) { badge.innerHTML = html || ''; syncMenuCount(el); })
      .catch(function () { /* non-fatal */ });
  }

  // markRead marks one notification read and refreshes the badge from the
  // server. keepalive lets the request finish when the click also navigates
  // away; without it the page change can cancel it and the item stays unread.
  function markRead(el, id) {
    return Chronicle.apiFetch('/notifications/' + encodeURIComponent(id) + '/read', { method: 'POST', keepalive: true })
      .then(function (r) {
        if (!r.ok) throw new Error('mark read failed');
        refreshBadge(el);
      });
  }

  // showRead dims a row, clears its unread dot and drops its mark-read icon.
  // It runs before the server answers so the click feels immediate.
  function showRead(row, read) {
    row.classList.toggle('opacity-60', read);
    row.setAttribute('data-notif-read', read ? 'true' : 'false');
    var dot = $('[data-notif-dot]', row);
    if (dot) {
      dot.classList.toggle('bg-accent', !read);
      dot.classList.toggle('bg-transparent', read);
    }
    var btn = $('[data-notif-markone]', row);
    // A class, not the hidden attribute: inline-flex would outrank [hidden].
    if (btn) {
      btn.classList.toggle('hidden', read);
      btn.classList.toggle('inline-flex', !read);
    }
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
      // The row holds the clickable body and, beside it, the mark-read icon;
      // a button cannot sit inside a link, so they are siblings.
      var row = document.createElement('div');
      row.className = 'flex items-start gap-1 pr-2 hover:bg-surface-alt transition-colors border-b border-edge last:border-0';
      row.setAttribute('data-notif-row', n.id || '');
      if (DM_TYPES[n.type]) {
        row.setAttribute('data-dm-only', '');
        row.style.boxShadow = 'inset 3px 0 0 rgb(var(--color-warn-rgb))';
      }

      // A notification with somewhere to go is a link; one without is a
      // button, so clicking it still marks it read.
      var item = document.createElement(n.link ? 'a' : 'button');
      if (n.link) item.href = n.link;
      else item.type = 'button';
      item.className = 'flex-1 min-w-0 flex items-start gap-2 pl-3 py-2 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent';
      var dot = document.createElement('span');
      dot.setAttribute('data-notif-dot', '');
      dot.className = 'mt-1.5 w-1.5 h-1.5 rounded-full shrink-0';
      item.appendChild(dot);
      if (TYPE_ICON[n.type]) {
        var ic = document.createElement('i');
        ic.className = 'fa-solid ' + TYPE_ICON[n.type] + ' mt-1 text-xs shrink-0';
        ic.style.color = '#8b5cf6';
        ic.setAttribute('aria-hidden', 'true');
        item.appendChild(ic);
      }
      var body = document.createElement('div');
      body.className = 'min-w-0';
      var msg = document.createElement('div');
      msg.className = 'text-sm text-fg';
      msg.textContent = n.message || 'Notification';
      var when = document.createElement('div');
      when.className = 'text-xs text-fg-muted mt-0.5';
      // A detail line ("On Bren") goes before the time.
      when.textContent = n.detail ? n.detail + ' · ' + relTime(n.createdAt) : relTime(n.createdAt);
      body.appendChild(msg); body.appendChild(when);
      item.appendChild(body);
      row.appendChild(item);

      // Row-end icon (.ai/ui-standard.md: monochrome, framed only on hover).
      var markOne = document.createElement('button');
      markOne.type = 'button';
      markOne.setAttribute('data-notif-markone', '');
      markOne.title = 'Mark as read';
      markOne.setAttribute('aria-label', 'Mark as read: ' + (n.message || 'notification'));
      markOne.className = 'shrink-0 self-center items-center justify-center w-7 h-7 rounded text-fg-muted border border-transparent hover:text-fg hover:border-edge hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent';
      var check = document.createElement('i');
      check.className = 'fa-solid fa-check text-xs';
      check.setAttribute('aria-hidden', 'true');
      markOne.appendChild(check);
      row.appendChild(markOne);

      showRead(row, !!n.read);

      function read() {
        if (n.read || !n.id) return null;
        n.read = true;
        showRead(row, true);
        return markRead(el, n.id).catch(function () {
          n.read = false;
          showRead(row, false);
          if (Chronicle.notify) Chronicle.notify('Could not mark that notification read.', 'error');
        });
      }
      item.addEventListener('click', function () {
        // A link navigates on its own; the request survives via keepalive.
        read();
      });
      markOne.addEventListener('click', function (e) {
        e.preventDefault(); e.stopPropagation();
        read();
      });
      list.appendChild(row);
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

      // The badge is swapped in by the bell button's own HTMX poll.
      toggle.addEventListener('htmx:afterSwap', function () { syncMenuCount(el); });
      syncMenuCount(el);

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
