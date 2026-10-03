/**
 * sync_history.js -- the Sync history page (Manage › Sync history).
 *
 * 1. Times arrive in UTC; this rewrites them in the reader's own time zone
 *    and puts a day heading ("Today", "Yesterday", "Thu 1 Oct") above each
 *    day's first row.
 * 2. New rows are fetched every 20 seconds and slid in at the top. Polling
 *    and the "connected" pulse both stop while the reader is away
 *    (MotionRest) and pick up again when they come back.
 */
(function () {
  'use strict';

  var POLL_MS = 20000;
  var DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

  function pad(n, w) { n = String(n); while (n.length < (w || 2)) n = '0' + n; return n; }
  function dayKey(d) { return d.getFullYear() + '-' + d.getMonth() + '-' + d.getDate(); }
  function dayLabel(d) {
    var now = new Date();
    var y = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
    if (dayKey(d) === dayKey(now)) return 'Today';
    if (dayKey(d) === dayKey(y)) return 'Yesterday';
    var s = DAYS[d.getDay()] + ' ' + d.getDate() + ' ' + MONTHS[d.getMonth()];
    return d.getFullYear() === now.getFullYear() ? s : s + ' ' + d.getFullYear();
  }

  function localise(list) {
    list.querySelectorAll('time.sh-t[datetime]').forEach(function (t) {
      if (t.dataset.local) return;
      var d = new Date(t.getAttribute('datetime'));
      if (isNaN(d)) return;
      t.textContent = pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds()) + '.' + pad(d.getMilliseconds(), 3);
      t.dataset.local = '1';
    });
    list.querySelectorAll(':scope > .sh-day').forEach(function (h) { h.remove(); });
    var last = '';
    list.querySelectorAll(':scope > .sh-row').forEach(function (row) {
      var d = new Date(row.dataset.at);
      if (isNaN(d)) return;
      var k = dayKey(d);
      if (k !== last) {
        last = k;
        var h = document.createElement('div');
        h.className = 'sh-day';
        h.textContent = dayLabel(d);
        row.parentNode.insertBefore(h, row);
      }
    });
  }

  function init() {
    var root = document.getElementById('sync-history');
    if (!root || root.dataset.ready) return;
    root.dataset.ready = '1';
    var list = document.getElementById('sh-rows');
    var form = root.querySelector('form');
    var live = root.querySelector('.sh-live');
    var MR = window.MotionRest;
    localise(list);

    list.addEventListener('htmx:afterSettle', function () { localise(list); });

    function topId() {
      var r = list.querySelector(':scope > .sh-row');
      return r ? r.dataset.id : '0';
    }
    var timer = 0, inflight = false;
    function poll() {
      timer = 0;
      if (!document.body.contains(root)) return;
      if (document.hidden || (MR && MR.still())) return;
      if (!inflight) {
        inflight = true;
        var q = new URLSearchParams(new FormData(form));
        q.set('rows', '1');
        q.set('after', topId());
        fetch(location.pathname + '?' + q.toString(), { headers: { 'HX-Request': 'true' }, credentials: 'same-origin' })
          .then(function (r) { return r.ok ? r.text() : ''; })
          .then(function (html) {
            if (!html.trim()) return;
            var tmp = document.createElement('div');
            tmp.innerHTML = html;
            var rows = tmp.querySelectorAll(':scope > .sh-row');
            if (!rows.length) return;
            var empty = list.querySelector(':scope > p');
            if (empty) empty.remove();
            var first = list.firstChild;
            rows.forEach(function (r) { r.classList.add('sh-new'); list.insertBefore(r, first); });
            if (window.htmx) window.htmx.process(list);
            localise(list);
          })
          .catch(function () {})
          .then(function () { inflight = false; });
      }
      timer = setTimeout(poll, POLL_MS);
    }
    timer = setTimeout(poll, POLL_MS);

    // The pulse is CSS; this only pauses it while the reader is away.
    var restCheck = 0;
    function watchRest() {
      restCheck = 0;
      var still = !!(MR && MR.still());
      if (live) live.classList.toggle('is-resting', still);
      if (still) return;
      restCheck = setTimeout(watchRest, 1000);
    }
    if (MR) {
      watchRest();
      MR.onWake(function () {
        if (!restCheck) watchRest();
        if (!timer) poll();
      });
    }
    document.addEventListener('visibilitychange', function () {
      if (!document.hidden && !timer) poll();
    });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
  document.addEventListener('htmx:afterSettle', init);
})();
