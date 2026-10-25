/**
 * wide_tip.js -- one tip on a very wide screen: the browser's zoom makes
 * Chronicle larger.
 *
 * Chronicle never enlarges itself: pop-ups, hover cards, menus and drags place
 * themselves by screen position, and a page that scales itself puts them in the
 * wrong place. The browser's own zoom scales everything evenly and remembers it
 * for the site, so on a screen this wide at normal zoom the page says so once.
 * Zooming in narrows the page, which is also what hides the tip.
 *
 * Either answer hides it for good (localStorage). It never shows inside a page
 * opened beside another.
 */
(function () {
  'use strict';

  // Wider than this many CSS pixels is a 4K-class screen at normal zoom.
  var WIDE_PX = 3000;
  var SEEN_KEY = 'chronicle-wide-tip-seen';

  // wideTip reports whether the tip belongs on a page this wide. Pure so the
  // rule is testable without a DOM.
  function wideTip(widthPx, seen, inPane) {
    return !seen && !inPane && widthPx >= WIDE_PX;
  }

  var C = (window.Chronicle = window.Chronicle || {});
  C.WideTip = { wideTip: wideTip };
  if (!window.document || !document.documentElement) return;

  var tip = null;

  function seen() {
    try { return localStorage.getItem(SEEN_KEY) === '1'; } catch (e) { return true; } // no storage: never nag on every page
  }

  function dismiss() {
    try { localStorage.setItem(SEEN_KEY, '1'); } catch (e) { /* see seen */ }
    if (tip) { tip.remove(); tip = null; }
    window.removeEventListener('resize', check);
  }

  function show() {
    var mac = /Mac|iPhone|iPad/.test(navigator.platform || '');
    tip = document.createElement('div');
    tip.className = 'wide-tip';
    tip.setAttribute('role', 'note');
    tip.innerHTML = '<b class="wide-tip__title">This screen is very wide</b>' +
      '<span>Chronicle looks small on it. Press <kbd>' + (mac ? 'Cmd' : 'Ctrl') + '</kbd> and <kbd>+</kbd> a few times to make everything larger. Your browser remembers it for Chronicle.</span>' +
      '<span class="wide-tip__acts"><button type="button" class="btn-primary btn-sm" data-wide-tip-ok>Got it</button>' +
      '<button type="button" class="wide-tip__quiet" data-wide-tip-no>Not now</button></span>';
    tip.addEventListener('click', function (e) {
      if (e.target.closest('[data-wide-tip-ok], [data-wide-tip-no]')) dismiss();
    });
    document.body.appendChild(tip);
  }

  function check() {
    var on = wideTip(window.innerWidth, seen(), !!document.querySelector('[data-beside-doc]'));
    if (on && !tip) show();
    else if (!on && tip) { tip.remove(); tip = null; }
  }

  function start() {
    if (seen()) return;
    check();
    window.addEventListener('resize', check);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
