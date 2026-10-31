/**
 * mini_bar_chart.js — a small bar chart drawn with plain elements, for the
 * sync API dashboard's 24-hour request and security-event series.
 *
 * Mounts via data-widget="mini-bar-chart" (admin_dashboard.templ in the
 * syncapi plugin), with the series as JSON in data-points: [{x, y}, ...],
 * x a label and y a count. Loads on sight, so it draws after a boosted swap
 * too, where a page script would have been stripped.
 */
(function () {
  'use strict';

  function el(tag, cls) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    return n;
  }

  Chronicle.register('mini-bar-chart', {
    init: function (root) {
      var points = [];
      try { points = JSON.parse(root.dataset.points || '[]') || []; } catch (e) { points = []; }
      root.textContent = '';
      if (!points.length) {
        var none = el('p', 'text-sm text-fg-muted italic text-center pt-16');
        none.textContent = 'No data available';
        root.appendChild(none);
        return;
      }
      var max = Math.max.apply(null, points.map(function (p) { return Number(p.y) || 0; }).concat([1]));
      var row = el('div', 'flex items-end h-full gap-px');
      points.forEach(function (p) {
        var y = Number(p.y) || 0;
        var col = el('div', 'flex-1 flex flex-col items-center justify-end h-full');
        var bar = el('div', 'w-full bg-accent/70 rounded-t');
        bar.style.height = Math.max((y / max) * 100, 2) + '%';
        bar.title = String(p.x) + ': ' + y;
        var label = el('span', 'text-[9px] text-fg-muted mt-1 truncate w-full text-center');
        label.textContent = String(p.x);
        col.appendChild(bar);
        col.appendChild(label);
        row.appendChild(col);
      });
      root.appendChild(row);
    },
  });
})();
