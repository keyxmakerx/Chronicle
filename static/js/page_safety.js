/**
 * page_safety.js -- small helpers for undo on world pages.
 *
 * 1. History rows carry <time data-page-time datetime="..."> with a UTC
 *    fallback label; this rewrites them in the reader's own time zone as
 *    "Today 21:14", "Yesterday 18:02" or "28 Sep".
 * 2. After a page is deleted the server redirects to the page list with
 *    ?trashed=<id>; this shows "Moved to Trash" with an Undo button that
 *    restores it, and drops the parameter so a reload doesn't repeat it.
 *    ?notes=<n> (set when the page had Player Notes) adds "with its n notes".
 *    Another kind of thing (a game night) adds &kind=<kind>, on whichever
 *    campaign page its delete lands, and restores through the Trash's
 *    route for that kind.
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
    var kind = params.get('kind') || '';
    var m = kind
      ? window.location.pathname.match(/^\/campaigns\/([^/]+)\//)
      : window.location.pathname.match(/^\/campaigns\/([^/]+)\/entities\/?$/);
    if (!id || !m || !window.Chronicle || !Chronicle.notify) return;
    if (kind && !/^[a-z-]+$/.test(kind)) return;

    var notes = parseInt(params.get('notes'), 10);
    var withNotes = notes > 0 ? ' with its ' + notes + (notes === 1 ? ' note' : ' notes') : '';
    params.delete('trashed');
    params.delete('notes');
    params.delete('kind');
    var rest = params.toString();
    history.replaceState(history.state, '', window.location.pathname + (rest ? '?' + rest : ''));

    Chronicle.notify('Moved to Trash' + withNotes + '.', 'success', { duration: 10000, action: { label: 'Undo', onClick: function (btn) {
      btn.disabled = true;
      var path = '/campaigns/' + encodeURIComponent(m[1]) + '/trash/' + (kind ? kind + '/' : '') + encodeURIComponent(id) + '/restore';
      Chronicle.apiFetch(path, {
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
          Chronicle.notify('Could not put it back. It is still in Manage > Trash.', 'error');
        });
    } } });
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
