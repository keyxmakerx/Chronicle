/**
 * craft_moments.js -- short scenes the bench plays outside a recipe's steps
 * (window.CraftMoments)
 *
 * An experiment, breaking an item down and a hand on a party member's
 * project each play at the bench as the signed clips show them: what goes in
 * drops into the flask or the mortar, then the bench's own particles and
 * sounds end the moment. The host has already asked the server what came of
 * it, so a moment only shows a known result; it never decides one.
 *
 * Usage, from a play-mode bench mounted with opts.moment:
 *   CraftMoments.play(scene.cue, {kind: 'close', colors: ['#4f7d3a'], first: '…', last: '…', buttons: [...], done: fn});
 * kind is 'found', 'close' or 'nothing' (an experiment), 'breakdown' or
 * 'help'. done runs when the moment ends. Under Calm, Off or reduced motion
 * the host shows its panel instead and never calls play; if it does, the
 * last words show at once.
 */
window.CraftMoments = (function () {
  'use strict';
  var FALLBACK = ['#7bbf5a', '#9aa3c7'];

  function rnd(a, b) { return a + Math.random() * (b - a); }

  /* Each step is [wait before the next, what to do]. */
  function steps(c, o) {
    var F = c.L.flask, M = c.L.mortar, fx = c.fx, P = c.P, cols = (o.colors && o.colors.length ? o.colors : FALLBACK);
    function pour(col) { for (var i = 0; i < 14; i++) fx.drop(P, F.x + rnd(-6, 6), F.neck, col); c.sfx('drop'); }
    function steam(n) { for (var i = 0; i < n; i++) c.later(function () { fx.steam(P, F.x, F.neck, 1.2); }, i * 90); }
    var brew = [[300, function () { c.say(o.first || ''); }], [900, function () { pour(cols[0]); }],
      [700, function () { fx.fizz(P, F.x, F.y, cols[0]); c.sfx('fizz'); }]];
    if (cols[1]) brew.push([700, function () { pour(cols[1]); fx.fizz(P, F.x, F.y, cols[1]); c.sfx('fizz'); }]);
    switch (o.kind) {
      case 'found':
        return brew.concat([[900, function () { steam(8); }],
          [1100, function () { fx.success(P, F.x, F.y - 30, cols[cols.length - 1]); c.sfx('done'); }]]);
      case 'close':
        return brew.concat([[1200, function () { steam(10); }], [1400, function () { fx.smoke(P, F.x, F.neck - 20); }]]);
      case 'nothing':
        return brew.concat([[1200, function () { steam(4); }], [900, function () { fx.smoke(P, F.x, F.neck - 20); }]]);
      case 'breakdown':
        return [[300, function () { c.say(o.first || ''); }],
          [900, function () { for (var i = 0; i < 10; i++) fx.drop(P, M.x + rnd(-10, 10), M.y - 40, cols[0]); c.sfx('drop'); }],
          [600, function () {
            for (var i = 0; i < 6; i++) c.later(function () { for (var j = 0; j < 6; j++) fx.dust(P, M.x + rnd(-20, 20), M.y - 10, cols[0]); c.sfx('grind'); }, i * 180);
          }],
          [1300, function () { fx.spark(P, M.x, M.y - 20, 'perfect'); fx.smoke(P, M.x, M.y - 20); }],
          [900, function () { for (var i = 0; i < 8; i++) fx.drop(P, M.x + rnd(-10, 10), M.y - 30, cols[1] || cols[0], true); }]];
      case 'help':
        return [[300, function () { c.say(o.first || ''); }],
          [900, function () { fx.fizz(P, F.x, F.y, cols[0]); c.sfx('fizz'); }], [1000, function () { steam(8); }],
          [1100, function () { fx.spark(P, F.x, F.neck, 'perfect'); c.sfx('perfect'); }],
          [800, function () { if (o.finished) { fx.success(P, F.x, F.y - 30, cols[0]); c.sfx('done'); } }]];
    }
    return [];
  }

  function play(c, o) {
    o = o || {};
    function end() { c.say(o.last || '', o.buttons || []); if (o.done) o.done(); }
    if (!c || c.calm) { if (c) end(); else if (o.done) o.done(); return; }
    var list = steps(c, o), at = 0;
    list.forEach(function (s) { c.later(s[1], at); at += s[0]; });
    c.later(end, at);
  }

  return {play: play};
})();
