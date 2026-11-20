/**
 * entry_told.js -- the small "Changed recently" mark on a hero's entry names.
 *
 * A sheet links each entry it names (ancestry, culture, career, kit) by kind
 * and name: a[data-entry-peek] whose address is
 * /campaigns/:id/systems/:mod/named/:kind?name=... (peek_address.js). When the
 * players were told about a change to the entry that name leads to, a dot in
 * the accent with the words "Changed recently" for screen readers sits right
 * after the link, and the link is described by those words, so focus on it
 * reads both. The mark is never inside the link: a sheet reads and animates
 * the name's own text. Which names are marked comes from one request per new set of
 * names, GET .../systems/:mod/told?n=<kind>:<name>, answered for the reader,
 * so a sheet that renders again asks nothing new. Any game system's sheet
 * that emits the same link gets the mark.
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.entryTold) return;

  var NAMED = /^\/campaigns\/([A-Za-z0-9_-]+)\/systems\/([A-Za-z0-9_-]+)\/named\/([A-Za-z0-9_-]+)\/peek\?name=([^&]+)/;
  var MAX_ASKED = 20;
  var known = {};   // "<cid>/<mod>|<kind>:<name>" -> true (told), false, or 'asking'
  var timer = 0;

  // ref reads one link's campaign, system, kind and name from its own peek
  // address, so only an address peek_address.js accepts is ever asked about.
  function ref(a) {
    var addr = C.peekAddress ? C.peekAddress(a.getAttribute('data-entry-peek') || '') : '';
    var m = NAMED.exec(addr);
    if (!m) return null;
    var name;
    try { name = decodeURIComponent(m[4]); } catch (err) { return null; }
    var group = m[1] + '/' + m[2];
    var n = m[3] + ':' + name;
    return { group: group, cid: m[1], mod: m[2], n: n, key: group + '|' + n };
  }

  var uid = 0;

  // mark puts one mark right after a told link; a mark already there, kept
  // when a sheet re-renders only the link, is reused, never doubled.
  function mark(a) {
    var next = a.nextElementSibling;
    if (next && next.classList.contains('entry-told-mark')) return;
    var box = document.createElement('span');
    box.className = 'entry-told-mark';
    var dot = document.createElement('span');
    dot.className = 'entry-told';
    dot.setAttribute('aria-hidden', 'true');
    var words = document.createElement('span');
    words.className = 'entry-told-sr';
    words.id = 'entry-told-' + (++uid);
    words.textContent = 'Changed recently';
    box.appendChild(dot);
    box.appendChild(words);
    a.parentNode.insertBefore(box, a.nextSibling);
    a.setAttribute('aria-describedby', words.id);
  }

  // unmarkStale removes a mark whose link went or no longer leads to a told
  // entry, as when a sheet swaps the link for another name's.
  function unmarkStale() {
    var marks = document.querySelectorAll('.entry-told-mark');
    for (var i = 0; i < marks.length; i++) {
      var a = marks[i].previousElementSibling;
      var r = a && a.matches('a[data-entry-peek]') ? ref(a) : null;
      if (r && known[r.key] === true) continue;
      var words = marks[i].querySelector('.entry-told-sr');
      if (a && words && a.getAttribute('aria-describedby') === words.id) a.removeAttribute('aria-describedby');
      marks[i].parentNode.removeChild(marks[i]);
    }
  }

  function decorate() {
    unmarkStale();
    var links = document.querySelectorAll('a[data-entry-peek]');
    var ask = {};
    for (var i = 0; i < links.length; i++) {
      var r = ref(links[i]);
      if (!r) continue;
      var k = known[r.key];
      if (k === true) mark(links[i]);
      else if (k === undefined) {
        var g = ask[r.group] || (ask[r.group] = { cid: r.cid, mod: r.mod, ns: [] });
        if (g.ns.indexOf(r.n) < 0 && g.ns.length < MAX_ASKED) g.ns.push(r.n);
      }
    }
    Object.keys(ask).forEach(function (group) { send(group, ask[group]); });
  }

  function send(group, g) {
    g.ns.forEach(function (n) { known[group + '|' + n] = 'asking'; });
    var url = '/campaigns/' + g.cid + '/systems/' + g.mod + '/told?' +
      g.ns.map(function (n) { return 'n=' + encodeURIComponent(n); }).join('&');
    var get = C.apiFetch ? C.apiFetch(url) : fetch(url, { credentials: 'same-origin' });
    get.then(function (res) { return res.ok ? res.json() : { told: [] }; })
      .then(function (body) {
        var told = (body && Array.isArray(body.told)) ? body.told : [];
        g.ns.forEach(function (n) { known[group + '|' + n] = told.indexOf(n) >= 0; });
        decorate();
      }, function () {
        // A failed question leaves the names unmarked; it is not asked again
        // on every render.
        g.ns.forEach(function (n) { known[group + '|' + n] = false; });
      });
  }

  function soon() {
    clearTimeout(timer);
    timer = setTimeout(decorate, 60);
  }

  function start() {
    decorate();
    if (window.MutationObserver) {
      new MutationObserver(function (records) {
        for (var i = 0; i < records.length; i++) {
          if (records[i].addedNodes && records[i].addedNodes.length) { soon(); return; }
        }
      }).observe(document.body, { childList: true, subtree: true });
    }
  }

  C.entryTold = { refresh: decorate };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
