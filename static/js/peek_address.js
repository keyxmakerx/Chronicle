/**
 * peek_address.js -- the one check for a page peek address.
 *
 * Chronicle.peekAddress(value) takes a page link, a preview address or a peek
 * address and returns the page's peek address, or '' when the value is not
 * exactly one of those three shapes for a page in a campaign. The pattern is
 * anchored at both ends and its two ids may hold only letters, digits,
 * underscore and hyphen, so nothing else (another route with a lookalike tail,
 * a query or fragment, a backslash, a protocol-relative or javascript: value)
 * can ever be fetched into the peek panel. Every caller that builds or
 * accepts a peek address goes through this.
 */
(function () {
  'use strict';

  var C = window.Chronicle = window.Chronicle || {};
  if (C.peekAddress) return;

  var SHAPE = /^\/campaigns\/([A-Za-z0-9_-]+)\/entities\/([A-Za-z0-9_-]+)(?:\/(?:preview|peek))?$/;

  C.peekAddress = function (value) {
    var m = SHAPE.exec(typeof value === 'string' ? value : '');
    return m ? '/campaigns/' + m[1] + '/entities/' + m[2] + '/peek' : '';
  };
})();
