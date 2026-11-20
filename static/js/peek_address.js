/**
 * peek_address.js -- the one check for a page peek address.
 *
 * Chronicle.peekAddress(value) takes a page link, a preview address or a peek
 * address and returns the page's peek address, or '' when the value is not
 * exactly one of those three shapes for a page in a campaign. A rules entry's
 * address (/campaigns/:id/systems/:mod/:cat/:item, or its /peek) is the fourth
 * shape; it may carry only ?hero=<id> and ?printed=1, rebuilt in one order.
 * An entry by kind and name (/campaigns/:id/systems/:mod/named/:kind?name=,
 * for sheets that store names) is the fifth; it carries name=<encoded name>
 * once and may carry hero=<id>, and the name is decoded and encoded again.
 * The patterns are anchored at both ends and every id may hold only letters,
 * digits, underscore and hyphen, so nothing else (another route with a lookalike tail,
 * a query or fragment, a backslash, a protocol-relative or javascript: value)
 * can ever be fetched into the peek panel. Every caller that builds or
 * accepts a peek address goes through this.
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.peekAddress) return;

  var SHAPE = /^\/campaigns\/([A-Za-z0-9_-]+)\/entities\/([A-Za-z0-9_-]+)(?:\/(?:preview|peek))?$/;
  var ENTRY = /^\/campaigns\/([A-Za-z0-9_-]+)\/systems\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)(?:\/peek)?(?:\?([A-Za-z0-9_=&-]*))?$/;
  var NAMED = /^\/campaigns\/([A-Za-z0-9_-]+)\/systems\/([A-Za-z0-9_-]+)\/named\/([A-Za-z0-9_-]+)(?:\/peek)?\?([A-Za-z0-9_.!~*'()%=&-]+)$/;
  var ENCODED = /^(?:[A-Za-z0-9_.!~*'()-]|%[0-9A-Fa-f]{2})+$/;
  // The system's own static segments are never an entry's category.
  var NOT_CATEGORY = { book: 1, data: 1, widgets: 1, named: 1, told: 1 };
  var ID = /^[A-Za-z0-9_-]+$/;

  // entryQuery keeps hero=<id> and printed=1, each at most once; anything
  // else makes the whole address unusable.
  function entryQuery(q) {
    if (!q) return '';
    var hero = '', printed = false;
    var parts = q.split('&');
    for (var i = 0; i < parts.length; i++) {
      var kv = parts[i].split('=');
      if (kv.length !== 2) return null;
      if (kv[0] === 'hero' && !hero && ID.test(kv[1])) hero = kv[1];
      else if (kv[0] === 'printed' && !printed && kv[1] === '1') printed = true;
      else return null;
    }
    var out = [];
    if (hero) out.push('hero=' + hero);
    if (printed) out.push('printed=1');
    return out.length ? '?' + out.join('&') : '';
  }

  // namedQuery keeps name (required, once) and hero (at most once), in that
  // order; anything else makes the whole address unusable.
  function namedQuery(q) {
    var name = null, hero = '';
    var parts = q.split('&');
    for (var i = 0; i < parts.length; i++) {
      var kv = parts[i].split('=');
      if (kv.length !== 2) return null;
      if (kv[0] === 'name' && name === null && ENCODED.test(kv[1])) {
        try { name = decodeURIComponent(kv[1]); } catch (err) { return null; }
        if (!name || name.length > 100) return null;
      } else if (kv[0] === 'hero' && !hero && ID.test(kv[1])) hero = kv[1];
      else return null;
    }
    if (name === null) return null;
    // The marks encodeURIComponent leaves alone are escaped too, so the
    // address never holds a quote.
    var enc = encodeURIComponent(name).replace(/[!'()*]/g, function (ch) {
      return '%' + ch.charCodeAt(0).toString(16).toUpperCase();
    });
    return '?name=' + enc + (hero ? '&hero=' + hero : '');
  }

  C.peekAddress = function (value) {
    var v = typeof value === 'string' ? value : '';
    var m = SHAPE.exec(v);
    if (m) return '/campaigns/' + m[1] + '/entities/' + m[2] + '/peek';
    var n = NAMED.exec(v);
    if (n) {
      var nq = namedQuery(n[4]);
      return nq === null ? '' : '/campaigns/' + n[1] + '/systems/' + n[2] + '/named/' + n[3] + '/peek' + nq;
    }
    var e = ENTRY.exec(v);
    if (!e || NOT_CATEGORY[e[3]]) return '';
    var q = entryQuery(e[5]);
    if (q === null) return '';
    return '/campaigns/' + e[1] + '/systems/' + e[2] + '/' + e[3] + '/' + e[4] + '/peek' + q;
  };
})();
