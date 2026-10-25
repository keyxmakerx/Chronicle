/**
 * page_sides.js -- side columns that follow the reader down a page.
 *
 * In a page layout row with two or more columns (show.templ marks them
 * data-page-row / data-page-col), the tallest column is the one being read and
 * scrolls as usual; every shorter column sticks in view beside it. A side
 * column taller than the visible area scrolls with the page until its bottom
 * shows, then holds there, so nothing in it is ever out of reach.
 *
 * Phones stack the columns, so nothing sticks below the md breakpoint.
 */
(function () {
  'use strict';

  // A column this much shorter than the tallest is worth keeping in view.
  var MIN_DIFF_PX = 120;
  // Gap between a stuck column and the top or bottom of the page area.
  var EDGE_PX = 16;

  var wide = window.matchMedia ? window.matchMedia('(min-width: 768px)') : null;
  var watcher = null;
  var queued = false;

  // stickTop is where a column of colH sticks in a view viewH tall: near the
  // top when it fits, or far enough up that its bottom shows. Pure so the rule
  // is testable without a DOM.
  function stickTop(colH, viewH) {
    return Math.min(EDGE_PX, viewH - colH - EDGE_PX);
  }

  function rows() {
    return document.querySelectorAll('[data-page-frame] [data-page-row]');
  }

  function cols(row) {
    var out = [];
    for (var i = 0; i < row.children.length; i++) {
      if (row.children[i].hasAttribute('data-page-col')) out.push(row.children[i]);
    }
    return out;
  }

  // A grid column is stretched to its row's height, so its own height says
  // nothing; what its blocks take up does.
  function contentHeight(col) {
    var top = Infinity, bottom = -Infinity;
    for (var i = 0; i < col.children.length; i++) {
      var r = col.children[i].getBoundingClientRect();
      if (!r.height) continue; // hidden blocks take no room
      if (r.top < top) top = r.top;
      if (r.bottom > bottom) bottom = r.bottom;
    }
    return bottom > top ? bottom - top : 0;
  }

  function unstick(col) {
    col.classList.remove('page-col-stick');
    col.style.removeProperty('--page-stick-top');
  }

  function layout() {
    queued = false;
    var main = document.getElementById('main-content');
    var viewH = main ? main.clientHeight : window.innerHeight;
    var on = !wide || wide.matches;
    rows().forEach(function (row) {
      var list = cols(row);
      if (!on || list.length < 2) { list.forEach(unstick); return; }
      var hs = list.map(contentHeight);
      var maxH = Math.max.apply(null, hs);
      var tallest = list[hs.indexOf(maxH)];
      list.forEach(function (c, i) {
        if (c === tallest || maxH - hs[i] < MIN_DIFF_PX) { unstick(c); return; }
        c.classList.add('page-col-stick');
        c.style.setProperty('--page-stick-top', stickTop(hs[i], viewH) + 'px');
      });
    });
  }

  function queue() {
    if (queued) return;
    queued = true;
    window.requestAnimationFrame(layout);
  }

  // Widgets in a column load their own data and grow afterwards, so every
  // column is watched, not measured once.
  function watch() {
    if (watcher) watcher.disconnect();
    watcher = window.ResizeObserver ? new ResizeObserver(queue) : null;
    var main = document.getElementById('main-content');
    if (watcher && main) watcher.observe(main);
    rows().forEach(function (row) {
      cols(row).forEach(function (c) {
        for (var i = 0; watcher && i < c.children.length; i++) watcher.observe(c.children[i]);
      });
    });
    queue();
  }

  window.addEventListener('resize', queue);
  if (wide && wide.addEventListener) wide.addEventListener('change', queue);
  document.addEventListener('htmx:afterSettle', watch);
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', watch);
  else watch();

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.PageSides = { stickTop: stickTop, refresh: queue };
})();
