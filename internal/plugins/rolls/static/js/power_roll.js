/**
 * power_roll.js — Chronicle.powerRoll, the arithmetic of a power roll.
 *
 *   plan(rules, name, value, net)  what to ask the server for: { formula, label }
 *   read(roll, rules)              how a roll in the log reads: { tier, crit, total, what } or null
 *   net(edges, banes)              edges and banes cancel one for one
 *
 * The server rolls the dice and keeps the formula and label; the tier is worked
 * out here from those, so every viewer of the log reads the same one. One net
 * edge or bane is the system's +2 / -2 in the formula. Two or more move the
 * tier a step instead and add nothing. A natural 19 or 20 on the two dice is
 * always the top tier. A critical comes from the server (dice[i].crit); it is
 * never worked out here.
 */
(function () {
  'use strict';

  var W = window;
  W.Chronicle = W.Chronicle || {};

  var PLAIN = 'Power roll';
  var WORDS = { '-2': 'double bane', '-1': 'bane', '1': 'edge', '2': 'double edge' };

  // Net edges: each edge cancels one bane.
  function net(edges, banes) { return (Number(edges) || 0) - (Number(banes) || 0); }

  function clampNet(n) { n = Math.trunc(Number(n) || 0); return n > 2 ? 2 : n < -2 ? -2 : n; }

  // The tier a total falls in, from the system's two breaks (11 and 16).
  function baseTier(total, tiers) {
    return total <= tiers[0] ? 1 : total <= tiers[1] ? 2 : 3;
  }

  // tierOf is the tier of dice that fell as `dice`, for a total and a net edge.
  function tierOf(dice, total, n, tiers) {
    var sum = 0;
    for (var i = 0; i < dice.length; i++) sum += Number(dice[i].value) || 0;
    if (dice.length === 2 && sum >= 19) return 3;
    var t = baseTier(total, tiers);
    if (n >= 2) t += 1;
    else if (n <= -2) t -= 1;
    return t > 3 ? 3 : t < 1 ? 1 : t;
  }

  // plan: the formula the server rolls and the label it keeps. A single edge or
  // bane is added into the bonus; a double one is only named in the label.
  function plan(rules, name, value, n) {
    n = clampNet(n);
    var mod = Math.trunc(Number(value) || 0);
    if (n === 1) mod += rules.edge;
    else if (n === -1) mod += rules.bane;
    var formula = rules.dice + (mod > 0 ? '+' + mod : mod < 0 ? '-' + Math.abs(mod) : '');
    return { formula: formula, label: (name || PLAIN) + (n ? ', ' + WORDS[n] : '') };
  }

  // read: null when the roll is not one of this system's power rolls (the
  // label names a characteristic or "Power roll", on two d10s) or is hidden.
  function read(r, rules) {
    if (!r || r.hidden || !rules || !r.dice || r.dice.length !== 2 || r.total == null) return null;
    if (r.dice[0].sides !== 10 || r.dice[1].sides !== 10) return null;
    var m = /^(.+?)(?:, (double )?(edge|bane))?$/.exec(String(r.label || ''));
    if (!m) return null;
    var known = m[1] === PLAIN;
    for (var i = 0; !known && i < (rules.characteristics || []).length; i++) known = rules.characteristics[i].label === m[1];
    if (!known) return null;
    var n = m[3] ? (m[2] ? 2 : 1) * (m[3] === 'bane' ? -1 : 1) : 0;
    var crit = r.dice.some(function (d) { return d.crit === 'succ'; });
    return { tier: tierOf(r.dice, Number(r.total), n, rules.tiers), crit: crit, total: Number(r.total), what: String(r.label) };
  }

  // headline: "Tier 2 · 14", or "Critical · Tier 3 · 20".
  function headline(p) { return (p.crit ? 'Critical · ' : '') + 'Tier ' + p.tier + ' · ' + p.total; }

  W.Chronicle.powerRoll = { net: net, plan: plan, read: read, tierOf: tierOf, headline: headline, WORDS: WORDS, PLAIN: PLAIN };
})();
