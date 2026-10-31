/**
 * table_dice.js — Chronicle.tableDice, the quick 2D dice of the Table tab.
 *
 *   Chronicle.tableDice.svg(sides, value)          one flat die as SVG markup
 *   Chronicle.tableDice.throwInto(tray, dice, opts) -> Promise, resolves when they land
 *
 * dice: [{ sides, value }], the values already decided by the server. The dice
 * come in from one side, skid, turn over a few times and settle on their
 * value; nothing bounces back up. opts.fromLeft picks the side (the roller's
 * own rolls come from the left). html[data-motion] decides how much moves:
 * calm sets them down with a fade, off (or reduced motion) draws nothing.
 */
(function () {
  'use strict';

  var W = window;
  W.Chronicle = W.Chronicle || {};

  // Each shape is an outline (o), a lit front face (f), facet lines from
  // face to outline points (k), shaded lower facets (sh) and where the
  // number sits (t), on a 56 by 56 grid.
  var GEO = {
    4: { o: [[28, 4], [53, 49], [3, 49]], f: [[28, 17], [43, 44], [13, 44]], k: [[0, 0], [1, 1], [2, 2]], sh: [[[3, 49], [53, 49], [43, 44], [13, 44]]], t: [28, 35] },
    6: { o: [[8, 5], [48, 5], [52, 9], [52, 47], [48, 51], [8, 51], [4, 47], [4, 9]], f: [[10, 10], [46, 10], [46, 44], [10, 44]], k: [[0, 0], [1, 1], [2, 4], [3, 5]], sh: [[[4, 47], [8, 51], [48, 51], [52, 47], [46, 44], [10, 44]]], t: [28, 27] },
    8: { o: [[28, 2], [54, 28], [28, 54], [2, 28]], f: [[28, 9], [47, 39], [9, 39]], k: [[0, 0], [1, 1], [2, 3], [1, 2], [2, 2]], sh: [[[9, 39], [47, 39], [28, 54]]], t: [28, 29] },
    10: { o: [[28, 2], [53, 23], [28, 54], [3, 23]], f: [[28, 11], [42, 26], [28, 43], [14, 26]], k: [[0, 0], [1, 1], [2, 2], [3, 3]], sh: [[[14, 26], [28, 43], [28, 54], [3, 23]], [[42, 26], [53, 23], [28, 54], [28, 43]]], t: [28, 27] },
    12: { o: [[28, 2], [53, 20], [44, 52], [12, 52], [3, 20]], f: [[28, 13], [42, 24], [37, 41], [19, 41], [14, 24]], k: [[0, 0], [1, 1], [2, 2], [3, 3], [4, 4]], sh: [[[19, 41], [37, 41], [44, 52], [12, 52]]], t: [28, 29] },
    20: { o: [[28, 2], [51, 15], [51, 41], [28, 54], [5, 41], [5, 15]], f: [[28, 12], [45, 41], [11, 41]], k: [[0, 0], [0, 1], [0, 5], [1, 1], [1, 2], [1, 3], [2, 3], [2, 4], [2, 5]], sh: [[[11, 41], [45, 41], [28, 54]], [[45, 41], [51, 41], [51, 15]]], t: [28, 31] }
  };

  // A die with no shape of its own borrows the nearest: a d100 and a d3 are
  // drawn as a d10 and a d6, anything bigger as a d20.
  function shapeOf(sides) {
    if (GEO[sides]) return GEO[sides];
    if (sides === 100) return GEO[10];
    if (sides <= 3) return GEO[6];
    return GEO[20];
  }

  function pts(a) { return a.map(function (p) { return p.join(','); }).join(' '); }

  // Only whole numbers reach the markup, whatever the payload held.
  function svg(sides, value) {
    var g = shapeOf(Number(sides) || 0);
    value = value == null || value === '' ? '' : (isFinite(Number(value)) ? Math.trunc(Number(value)) : '');
    var lines = g.k.map(function (k) {
      var a = g.f[k[0]], b = g.o[k[1]];
      return '<line x1="' + a[0] + '" y1="' + a[1] + '" x2="' + b[0] + '" y2="' + b[1] + '" class="tdl"/>';
    }).join('');
    return '<svg viewBox="0 0 56 56" aria-hidden="true" focusable="false"><polygon class="tdb" points="' + pts(g.o) + '"/>' +
      g.sh.map(function (s) { return '<polygon class="tds" points="' + pts(s) + '"/>'; }).join('') +
      '<polygon class="tdf" points="' + pts(g.f) + '"/>' + lines + '<polygon class="tdr" points="' + pts(g.o) + '"/>' +
      '<text x="' + g.t[0] + '" y="' + g.t[1] + '">' + value + '</text></svg>';
  }

  function motion() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    try { if (W.matchMedia && W.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off'; } catch (e) { /* no media queries */ }
    return 'full';
  }

  function randomFace(sides) { return 1 + Math.floor(Math.random() * sides); }

  // How long the dice stay after landing, in ms, before they fade.
  var STAY = 1800;

  function throwInto(tray, dice, opts) {
    opts = opts || {};
    return new Promise(function (resolve) {
      var m = motion();
      if (!tray || m === 'off' || !dice || !dice.length || typeof Element.prototype.animate !== 'function') { resolve(); return; }
      var w = tray.clientWidth, h = tray.clientHeight;
      var shown = dice.slice(0, 12);
      var gap = Math.min(64, (w - 40) / Math.max(1, shown.length));
      var cx = w * 0.5, cy = h * (opts.y || 0.42);
      var side = opts.fromLeft ? -1 : 1;
      var longest = 0;
      shown.forEach(function (d, i) {
        var el = document.createElement('div');
        el.className = 'td-die' + (d.crit === 'succ' ? ' td-hi' : d.crit === 'fail' ? ' td-lo' : '');
        var ex = cx + (i - (shown.length - 1) / 2) * gap, ey = cy + (i % 2 ? 12 : -8);
        el.style.left = (ex - 28) + 'px';
        el.style.top = (ey - 28) + 'px';
        el.innerHTML = svg(d.sides, m === 'calm' ? d.value : '');
        tray.appendChild(el);
        var text = el.querySelector('text');
        var dur = 820 + i * 50;
        longest = Math.max(longest, m === 'calm' ? 200 : dur);
        if (m === 'calm') {
          el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200, fill: 'forwards' });
        } else {
          var sx = side * w * 0.48, sy = h * 0.3, spin = side * (600 + i * 70);
          el.animate([
            { transform: 'translate(' + sx + 'px,' + sy + 'px) rotate(' + spin + 'deg) scale(0.82)', opacity: 0 },
            { transform: 'translate(' + sx * 0.55 + 'px,' + sy * 0.2 + 'px) rotate(' + spin * 0.55 + 'deg) scale(1.04)', opacity: 1, offset: 0.3 },
            { transform: 'translate(' + sx * 0.12 + 'px,-6px) rotate(' + spin * 0.12 + 'deg)', offset: 0.7 },
            { transform: 'translate(' + sx * 0.02 + 'px,0) rotate(' + spin * 0.01 + 'deg)', offset: 0.9 },
            { transform: 'none' }
          ], { duration: dur, easing: 'cubic-bezier(0.22,0.8,0.24,1)', fill: 'forwards' });
          // The face changes as the die turns over, then holds the real one.
          [0.18, 0.4, 0.62, 0.8].forEach(function (at, k, all) {
            setTimeout(function () { text.textContent = k === all.length - 1 ? d.value : randomFace(d.sides); }, dur * at);
          });
        }
        setTimeout(function () {
          var a = el.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 240, fill: 'forwards' });
          a.onfinish = function () { el.remove(); };
        }, longest + STAY);
      });
      setTimeout(resolve, longest + 40);
    });
  }

  W.Chronicle.tableDice = { svg: svg, throwInto: throwInto, motion: motion };
})();
