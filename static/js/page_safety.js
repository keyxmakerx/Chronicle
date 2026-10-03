/**
 * page_safety.js -- small helpers for undo on world pages.
 *
 * 1. History rows carry <time data-page-time datetime="..."> with a UTC
 *    fallback label; this rewrites them in the reader's own time zone as
 *    "Today 21:14", "Yesterday 18:02" or "28 Sep".
 * 2. After a page is deleted the server redirects to the page list with
 *    ?trashed=<id>; this shows "Moved to Trash" with an Undo button that
 *    restores it, and drops the parameter so a reload doesn't repeat it.
 */
(function () {
  'use strict';

  var MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

  function pad(n) { return n < 10 ? '0' + n : String(n); }

  function sameDay(a, b) {
    return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  }

  function label(d) {
    var now = new Date();
    var yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
    var hm = pad(d.getHours()) + ':' + pad(d.getMinutes());
    if (sameDay(d, now)) return 'Today ' + hm;
    if (sameDay(d, yesterday)) return 'Yesterday ' + hm;
    var s = d.getDate() + ' ' + MONTHS[d.getMonth()];
    return d.getFullYear() === now.getFullYear() ? s : s + ' ' + d.getFullYear();
  }

  function localizeTimes(root) {
    var els = (root || document).querySelectorAll('time[data-page-time]');
    for (var i = 0; i < els.length; i++) {
      var d = new Date(els[i].getAttribute('datetime'));
      if (!isNaN(d.getTime())) {
        els[i].textContent = label(d);
        els[i].title = d.toLocaleString();
      }
    }
  }

  function offerUndo() {
    var params = new URLSearchParams(window.location.search);
    var id = params.get('trashed');
    var m = window.location.pathname.match(/^\/campaigns\/([^/]+)\/entities\/?$/);
    if (!id || !m || !window.Chronicle || !Chronicle.notify) return;

    params.delete('trashed');
    var rest = params.toString();
    history.replaceState(history.state, '', window.location.pathname + (rest ? '?' + rest : ''));

    var btnId = 'undo-trash-' + Date.now();
    Chronicle.notify(
      'Moved to Trash. <button type="button" id="' + btnId + '" class="underline font-medium ml-1">Undo</button>',
      'success',
      { html: true, duration: 10000 }
    );
    var btn = document.getElementById(btnId);
    if (!btn) return;
    btn.addEventListener('click', function () {
      btn.disabled = true;
      Chronicle.apiFetch('/campaigns/' + encodeURIComponent(m[1]) + '/trash/' + encodeURIComponent(id) + '/restore', {
        method: 'POST',
      })
        .then(function (res) {
          if (!res.ok) throw new Error('restore failed: ' + res.status);
          return res.json();
        })
        .then(function (data) {
          window.location.href = data.url;
        })
        .catch(function () {
          btn.disabled = false;
          Chronicle.notify('Could not restore the page. It is still in Manage > Trash.', 'error');
        });
    });
  }

  function init() {
    localizeTimes(document);
    offerUndo();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
  document.addEventListener('htmx:afterSwap', function (e) { localizeTimes(e.target); });
  // hx-boost navigation into the page list arrives without a page load.
  document.addEventListener('htmx:afterSettle', function () { offerUndo(); });
})();
