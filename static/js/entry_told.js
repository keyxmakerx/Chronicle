/**
 * entry_told.js -- the small "Changed recently" mark on a hero's entry names.
 *
 * A sheet links each entry it names (ancestry, culture, career, kit) by kind
 * and name: a[data-entry-peek] whose address is
 * /campaigns/:id/systems/:mod/named/:kind?name=... (peek_address.js). When the
 * players were told about a change to the entry that name leads to, the link
 * carries a dot in the accent with the words "Changed recently" for screen
 * readers. Which names are marked comes from one request per new set of
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

  function mark(a) {
    if (a.querySelector('.entry-told')) return;
    var dot = document.createElement('span');
    dot.className = 'entry-told';
    dot.setAttribute('aria-hidden', 'true');
    var words = document.createElement('span');
    words.className = 'entry-told-sr';
    words.textContent = ' Changed recently';
    a.appendChild(dot);
    a.appendChild(words);
  }

  function decorate() {
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
