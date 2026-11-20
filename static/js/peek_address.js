/**
 * peek_address.js -- the one check for a page peek address.
 *
 * Chronicle.peekAddress(value) takes a page link, a preview address or a peek
 * address and returns the page's peek address, or '' when the value is not
 * exactly one of those three shapes for a page in a campaign. A rules entry's
 * address (/campaigns/:id/systems/:mod/:cat/:item, or its /peek) is the fourth
 * shape; it may carry only ?hero=<id> and ?printed=1, rebuilt in one order.
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
  // The system's own static segments are never an entry's category.
  var NOT_CATEGORY = { book: 1, data: 1, widgets: 1 };
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

  C.peekAddress = function (value) {
    var v = typeof value === 'string' ? value : '';
    var m = SHAPE.exec(v);
    if (m) return '/campaigns/' + m[1] + '/entities/' + m[2] + '/peek';
    var e = ENTRY.exec(v);
    if (!e || NOT_CATEGORY[e[3]]) return '';
    var q = entryQuery(e[5]);
    if (q === null) return '';
    return '/campaigns/' + e[1] + '/systems/' + e[2] + '/' + e[3] + '/' + e[4] + '/peek' + q;
  };
})();
