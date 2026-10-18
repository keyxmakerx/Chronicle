/**
 * dice.js — Chronicle.dice, thrown 3D dice for any roll on the site.
 *
 *   Chronicle.dice.roll(groups, opts) -> Promise<[{ label, values, crits }]>
 *
 * groups: [{ label, dice: [die] }]. A die is { sides } with sides 4, 6, 8, 10, 12 or 20, or
 *   { sides: 10, tens: true } for the percentile tens die (00, 10 … 90). It may also carry
 *   value (the number it shows, decided by the caller, as opts.results does), set and colour (its
 *   roller's look), succ and fail (its roller's crit effects), crit ('succ' | 'fail' | 'none', the
 *   game system's word on this die) and counts (true on the dice that count, like advantage's kept
 *   die; when none is marked, every die counts).
 * values: dN gives 1..N; the tens die gives 0, 10 … 90 (00 is 0).
 * crits: per die 'succ', 'fail' or null.
 *
 * opts:
 *   from              an element or client { x, y } the throw starts at
 *   describe(group, values)   the words read out to screen readers
 *   results           values decided by the caller, one array per group
 *   crits(group, values, index)   the game system's crit rule: 'succ', 'fail' or null per die, or
 *                     false for none. Without one, a natural 20 on a d20 that counts is a success
 *                     and a natural 1 a failure.
 *   crit              'succ' | 'fail': show that crit on the first d20 that counts (a preview)
 *   set, colour, succ, fail   the table's look and crits, for dice that don't name their own
 *   quality           { mode: 'auto' | 'basic' | 'hd', capBasic } (capBasic: the table holds
 *                     everyone to Basic)
 *   still             set the dice down instead of throwing them
 *   skippable         false to keep a press from skipping the animations
 *   onSettle(points)  called once, when the dice land (points: where, in client pixels) or at once
 *                     when nothing is drawn: the moment to show the number
 *   onEnd()           called once the dice have gone
 *
 * The promise resolves with onSettle, except that still and Calm rolls resolve at once. It rejects
 * only when the roll is refused (an unknown die, an impossible value) or this script cannot load;
 * a throw whose files are late or fail goes on without its dice.
 *
 *   Chronicle.dice.catalogue() -> Promise<{ sets, crits, defaults }>   every look, for pickers
 *   Chronicle.dice.defaults    { set, colour, succ, fail, quality }
 *   Chronicle.dice.stop()
 *
 * The values and crits are decided here before anything is drawn, so Calm, Off and a browser
 * without WebGL resolve with the same numbers a throw would show. dice/engine.js draws the throw;
 * it, three.js and cannon-es are fetched on the first throw.
 */
(function () {
  'use strict';

  var W = typeof window !== 'undefined' ? window : globalThis;
  // The layout lists the hashed library URLs beside dice.js's own.
  var SELF = typeof document !== 'undefined' ? document.getElementById('chronicle-dice-src') || document.currentScript : null;
  var THREE_SRC = (SELF && SELF.getAttribute('data-three-src')) || '/static/vendor/three-dice.min.js';
  var CANNON_SRC = (SELF && SELF.getAttribute('data-cannon-src')) || '/static/vendor/cannon-es.js';
  // Bare, with no version query: the set and crit files import the engine by this same URL
  // (see the note at the top of engine.js). Unversioned static files are always revalidated.
  var ENGINE_SRC = '/static/js/dice/engine.js';

  var SIDES = [4, 6, 8, 10, 12, 20];
  // What a campaign starts with until its owner or a player picks otherwise.
  var DEFAULTS = { set: 'jewelled', colour: 'onyx', succ: 'phoenix', fail: 'grasp', quality: 'auto' };
  // How long a throw waits for its files before the roll goes on without the dice, in ms.
  var LOAD_WAIT = 4000;

  // ---------- Values: decided first, from the browser's crypto source ----------

  function rand(n) {
    var c = W.crypto;
    if (c && c.getRandomValues) {
      // Rejection sampling keeps every face equally likely.
      var buf = new Uint32Array(1), lim = Math.floor(4294967296 / n) * n;
      do { c.getRandomValues(buf); } while (buf[0] >= lim);
      return buf[0] % n + 1;
    }
    return Math.floor(Math.random() * n) + 1;
  }
  function randomValue(die) { return die.tens ? (rand(10) - 1) * 10 : rand(die.sides); }
  function validValue(die, v) {
    if (typeof v !== 'number' || v !== Math.floor(v)) return false;
    return die.tens ? v >= 0 && v <= 90 && v % 10 === 0 : v >= 1 && v <= die.sides;
  }

  // Checks the groups and settles every value, throwing on anything the
  // dice could never show.
  function decide(groups, results) {
    if (!Array.isArray(groups)) throw new TypeError('Chronicle.dice.roll: groups must be an array');
    return groups.map(function (g, gi) {
      if (!g || !Array.isArray(g.dice)) throw new TypeError('Chronicle.dice.roll: group ' + gi + ' needs a dice array');
      var given = results && results[gi];
      return {
        label: g.label == null ? '' : String(g.label),
        values: g.dice.map(function (d, di) {
          if (!d || SIDES.indexOf(d.sides) < 0) throw new TypeError('Chronicle.dice.roll: unsupported die d' + (d && d.sides));
          if (d.tens && d.sides !== 10) throw new TypeError('Chronicle.dice.roll: only a d10 can be a tens die');
          var v = given && given[di] !== undefined ? given[di] : d.value;
          if (v !== undefined && v !== null) {
            if (!validValue(d, v)) throw new RangeError('Chronicle.dice.roll: ' + v + ' is not a face of that die');
            return v;
          }
          return randomValue(d);
        })
      };
    });
  }

  function isD20(d) { return d.sides === 20 && !d.tens; }
  function anyCounts(groups) { return groups.some(function (g) { return g.dice.some(function (d) { return d && d.counts; }); }); }

  // A crit preview shows on the first d20 that counts, or the first d20.
  function forceCrit(groups, out, kind) {
    if (kind !== 'succ' && kind !== 'fail') return;
    var counts = anyCounts(groups), at = null;
    groups.forEach(function (g, gi) { g.dice.forEach(function (d, di) { if (!at && isD20(d) && (!counts || d.counts)) at = [gi, di]; }); });
    if (at) out[at[0]].values[at[1]] = kind === 'succ' ? 20 : 1;
  }

  // Whether each die crits. A die's own crit field wins, then the caller's rule, then the d20 one.
  function critsOf(groups, out, opts) {
    var counts = anyCounts(groups);
    return groups.map(function (g, gi) {
      var values = out[gi].values, rule = typeof opts.crits === 'function' ? opts.crits(g, values.slice(), gi) || [] : null;
      return g.dice.map(function (d, di) {
        if (d.crit === 'succ' || d.crit === 'fail') return d.crit;
        if (d.crit === 'none' || opts.crits === false) return null;
        if (rule) return rule[di] === 'succ' || rule[di] === 'fail' ? rule[di] : null;
        if (!isD20(d) || (counts && !d.counts)) return null;
        return values[di] === 20 ? 'succ' : values[di] === 1 ? 'fail' : null;
      });
    });
  }

  // ---------- Where and how to draw ----------

  var GL = null;
  function hasGL() {
    if (GL === null) {
      try { var c = document.createElement('canvas'); GL = !!(c.getContext('webgl2') || c.getContext('webgl')); } catch (e) { GL = false; }
    }
    return GL;
  }

  // Matches MotionLevel: html[data-motion] is "calm" or "off" (theme.js sets it).
  function motion() {
    var m = document.documentElement.getAttribute('data-motion');
    if (m === 'off' || m === 'calm') return m;
    if (W.matchMedia && W.matchMedia('(prefers-reduced-motion: reduce)').matches) return 'off';
    return 'full';
  }

  function fromPoint(from) {
    if (!from) return null;
    if (typeof from.getBoundingClientRect === 'function') {
      var r = from.getBoundingClientRect();
      return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
    }
    return typeof from.x === 'number' && typeof from.y === 'number' ? { x: from.x, y: from.y } : null;
  }

  // Dice land below the top bar, never under it.
  function topClear() {
    var bar = document.getElementById('app-topbar');
    return bar ? Math.max(0, bar.getBoundingClientRect().bottom) : 0;
  }

  var ENGINE = null;
  function engine() {
    if (!ENGINE) {
      ENGINE = import(ENGINE_SRC).then(function (E) { E.configure({ three: THREE_SRC, cannon: CANNON_SRC }); return E; });
      // A failed load must not stick: the next roll tries again.
      ENGINE.catch(function () { ENGINE = null; });
    }
    return ENGINE;
  }

  // The engine's dice for the groups, each in its own look or the table's. Names the engine doesn't
  // know fall back to the defaults.
  function engineDice(E, groups, out, opts) {
    var table = E.hasSet(opts.set) ? opts.set : DEFAULTS.set, list = [];
    groups.forEach(function (g, gi) {
      g.dice.forEach(function (d, di) {
        var own = E.hasSet(d.set);
        list.push({
          sides: d.tens ? 100 : d.sides, value: out[gi].values[di], crit: out[gi].crits[di], counts: !!d.counts,
          kind: own ? d.set : table, colour: own ? d.colour : (d.colour || opts.colour),
          succFx: E.hasCrit(d.succ, 'succ') ? d.succ : null, failFx: E.hasCrit(d.fail, 'fail') ? d.fail : null
        });
      });
    });
    return list;
  }

  // ---------- Announcing the result ----------

  function announce(text) {
    if (!text || typeof document === 'undefined') return;
    var live = document.getElementById('chronicle-dice-live');
    if (!live) {
      live = document.createElement('div');
      live.id = 'chronicle-dice-live'; live.className = 'sr-only';
      live.setAttribute('aria-live', 'polite');
      document.body.appendChild(live);
    }
    live.textContent = '';
    // A fresh node each time, so the same total twice is still read out.
    setTimeout(function () { live.textContent = text; }, 30);
  }
  function describeAll(groups, out, describe) {
    return out.map(function (r, i) {
      if (describe) return describe(groups[i], r.values);
      return (r.label ? r.label + ': ' : '') + r.values.join(', ');
    }).filter(Boolean).join('. ');
  }

  function call(fn, arg) {
    if (typeof fn !== 'function') return;
    try { fn(arg); } catch (e) { setTimeout(function () { throw e; }, 0); }
  }

  // ---------- One roll ----------

  // Each roll takes a number; a roll still loading when a newer one starts is never drawn.
  var seq = 0;
  function stop() {
    seq++;
    if (ENGINE) ENGINE.then(function (E) { E.stop(document.body); }, function () {});
  }

  // Auto quality measures the device once, when the page is idle after a throw, so nobody waits for
  // a test before their first roll.
  var measured = false;
  function measureLater(E, quality) {
    if (measured || (quality && quality.mode && quality.mode !== 'auto') || E.cachedQuality()) return;
    measured = true;
    var go = function () { E.detectQuality().catch(function () {}); };
    if (W.requestIdleCallback) W.requestIdleCallback(go, { timeout: 5000 }); else setTimeout(go, 1500);
  }

  function roll(groups, opts) {
    opts = opts || {};
    var out, told;
    try {
      out = decide(groups, opts.results);
      forceCrit(groups, out, opts.crit);
      critsOf(groups, out, opts).forEach(function (c, i) { out[i].crits = c; });
      told = describeAll(groups, out, opts.describe);
    } catch (e) { return Promise.reject(e); }
    var count = out.reduce(function (n, r) { return n + r.values.length; }, 0);
    var m = typeof document === 'undefined' ? 'off' : motion();
    if (!count || m === 'off' || !hasGL()) {
      announce(told); call(opts.onSettle, []); call(opts.onEnd);
      return Promise.resolve(out);
    }
    var still = !!opts.still || m === 'calm', me = ++seq, from = fromPoint(opts.from);
    return new Promise(function (resolve) {
      var resolved = false, settled = false, ended = false, late = false;
      function resolveNow() { if (!resolved) { resolved = true; announce(told); resolve(out); } }
      function settle(points) { resolveNow(); if (!settled) { settled = true; call(opts.onSettle, points || []); } }
      function end() { settle([]); if (!ended) { ended = true; call(opts.onEnd); } }
      // Calm and still rolls do not wait for the drawing.
      if (still) resolveNow();
      // Past the wait, the roll is over without its dice, even if the files arrive later.
      var timer = setTimeout(function () { late = true; end(); }, LOAD_WAIT);
      engine().then(function (E) {
        var dice = engineDice(E, groups, out, opts);
        var succ = E.hasCrit(opts.succ, 'succ') ? opts.succ : DEFAULTS.succ, fail = E.hasCrit(opts.fail, 'fail') ? opts.fail : DEFAULTS.fail;
        var crits = [];
        if (dice.some(function (d) { return d.crit; })) {
          crits = [succ, fail];
          dice.forEach(function (d) { if (d.crit === 'succ' && d.succFx) crits.push(d.succFx); if (d.crit === 'fail' && d.failFx) crits.push(d.failFx); });
        }
        return Promise.all([E.ready(), E.loadSets(dice.map(function (d) { return d.kind; })), E.loadCrits(crits)]).then(function () {
          clearTimeout(timer);
          // Too late, or a newer roll took over: this one goes on without its dice.
          if (late || me !== seq) { end(); return; }
          return E.roll({
            dice: dice, succFx: succ, failFx: fail, quality: opts.quality || { mode: DEFAULTS.quality },
            from: from, padTop: topClear(), still: still, skippable: opts.skippable !== false,
            onSettle: settle
          }).then(function () { end(); measureLater(E, opts.quality); });
        });
      }).catch(function () { clearTimeout(timer); end(); });
    });
  }

  function catalogue() {
    return engine().then(function (E) { return E.catalogue(); }).then(function (c) {
      c.defaults = Object.assign({}, DEFAULTS);
      return c;
    });
  }

  W.Chronicle = W.Chronicle || {};
  W.Chronicle.dice = {
    roll: roll,
    stop: stop,
    catalogue: catalogue,
    defaults: Object.assign({}, DEFAULTS),
    // Exposed for test/js/dice.test.mjs.
    _internal: { decide: decide, critsOf: critsOf, forceCrit: forceCrit, engineDice: engineDice }
  };
})();
