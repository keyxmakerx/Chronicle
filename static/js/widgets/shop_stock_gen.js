/**
 * shop_stock_gen.js -- the draft behind a shop's "Fill the shelves".
 *
 * Pure JavaScript with no DOM: the same inputs always give the same draft, so
 * the stock list can show it, and tests can pin it. It follows the contract of
 * the other generators (a seed, kept "locked" goods, a plain-language summary)
 * but stands alone: it needs none of the calendar engine.
 *
 *   ShopStockGen.run({
 *     kind: 'general',            // forge | apothecary | jeweler | market | library | general
 *     count: 14,                  // goods in the whole draft, kept ones included
 *     prices: 'fair',             // cheap | fair | dear, scales the everyday list only
 *     foodPrice: 1,               // what a day's food costs in the main coin
 *     currency: 'gp',
 *     rarityCap: 1,               // system goods up to this rarity (0 = any)
 *     spots: 2,                   // places on today's shelf, for the sentence
 *     sources: { everyday, items, system, table },   // switches
 *     pool: { everyday: [...], items: [...], system: [...], table: {name, entries}, listed: [...] },
 *     seed, locked: [good], leftOut: [key],
 *   }) -> { goods, summary, stats }
 *
 * A good is { key, name, itemId?, slug?, source, kind?, price, currency,
 * quantity, today, note?, edited? }. price is null when no price is known:
 * an unpriced system good or table line is left for the Director, never given
 * a made-up number. Prices already set on a campaign's items and on system
 * goods are used as they are, never scaled; only the everyday list scales.
 */
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module && module.exports) module.exports = api;
  if (root) root.ShopStockGen = api;
})(typeof globalThis !== 'undefined' ? globalThis : (typeof window !== 'undefined' ? window : this), function () {
  'use strict';

  var PRICE_FACTOR = { cheap: 0.75, fair: 1, dear: 1.5 };
  var KIND_WORDS = {
    forge: 'a forge', apothecary: 'an apothecary', jeweler: 'a jeweller',
    market: 'a market', library: 'a library', general: 'a general store'
  };
  var NUMBER_WORDS = ['no', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten',
    'eleven', 'twelve'];

  function hash(str) {
    var h = 1779033703 ^ str.length;
    for (var i = 0; i < str.length; i++) {
      h = Math.imul(h ^ str.charCodeAt(i), 3432918353);
      h = (h << 13) | (h >>> 19);
    }
    return function () {
      h = Math.imul(h ^ (h >>> 16), 2246822507);
      h = Math.imul(h ^ (h >>> 13), 3266489909);
      return (h ^= h >>> 16) >>> 0;
    };
  }

  // A stream named by its purpose, so changing one switch never reshuffles
  // the draws of another source.
  function stream(seed, name) {
    var next = hash(String(seed) + '|' + name);
    var a = next();
    return function () {
      a = (a + 0x6D2B79F5) | 0;
      var t = Math.imul(a ^ (a >>> 15), 1 | a);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  function shuffled(list, rnd) {
    var a = list.slice();
    for (var i = a.length - 1; i > 0; i--) {
      var j = Math.floor(rnd() * (i + 1));
      var t = a[i]; a[i] = a[j]; a[j] = t;
    }
    return a;
  }

  function norm(name) { return String(name || '').trim().toLowerCase(); }
  function words(n) { return n >= 0 && n < NUMBER_WORDS.length ? NUMBER_WORDS[n] : String(n); }
  function cap1(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

  // Friendly figures: small prices keep two places, middling ones one, and
  // big ones round to a whole coin, then to fives.
  function roundPrice(x) {
    if (!(x > 0)) return 0;
    if (x < 1) return Math.max(0.01, Math.round(x * 100) / 100);
    if (x < 20) return Math.round(x * 10) / 10;
    if (x < 200) return Math.round(x);
    return Math.round(x / 5) * 5;
  }

  function between(rnd, lo, hi) { return lo + Math.floor(rnd() * (hi - lo + 1)); }

  function key(source, name) { return source + ':' + norm(name); }

  // How many of the whole draft are finds on today's shelf: about a third,
  // at least one when anything rare is on offer, never more than the pool.
  function findShare(count, findPool) {
    if (findPool <= 0) return 0;
    return Math.min(findPool, Math.max(1, Math.round(count * 0.3)));
  }

  function run(opts) {
    opts = opts || {};
    var kind = KIND_WORDS[opts.kind] ? opts.kind : 'general';
    var count = Math.max(1, Math.min(80, Math.floor(opts.count) || 14));
    var level = PRICE_FACTOR[opts.prices] ? opts.prices : 'fair';
    var food = opts.foodPrice > 0 ? opts.foodPrice : 1;
    var currency = opts.currency || 'gp';
    var cap = Math.max(0, Math.floor(opts.rarityCap) || 0);
    var spots = Math.max(1, Math.floor(opts.spots) || 2);
    var src = opts.sources || {};
    var pool = opts.pool || {};
    var seed = opts.seed == null ? 'shop' : opts.seed;

    var locked = (opts.locked || []).slice(0, count);
    var out = {};
    locked.forEach(function (g) { out[g.key] = true; });
    (opts.leftOut || []).forEach(function (k) { out[k] = true; });
    // Goods the shop already sells are never drafted a second time.
    var already = {};
    (pool.listed || []).forEach(function (l) {
      if (l.itemId) already['id:' + l.itemId] = true;
      if (l.name) already['name:' + norm(l.name)] = true;
    });
    function taken(name, itemId) {
      return already['name:' + norm(name)] || (itemId && already['id:' + itemId]);
    }

    var always = [], finds = [];

    if (src.everyday && pool.everyday) {
      pool.everyday.forEach(function (e) {
        if (!e || !e.name) return;
        if (e.shops && e.shops.indexOf(kind) < 0) return;
        var g = {
          key: key('everyday', e.name), name: e.name, source: 'everyday',
          price: roundPrice(e.price * food * PRICE_FACTOR[level]), currency: currency,
          _count: e.count || [1, 4], today: false
        };
        if (out[g.key] || taken(g.name)) return;
        (e.find ? finds : always).push(g);
      });
    }
    if (src.items && pool.items) {
      pool.items.forEach(function (it) {
        var g = {
          key: key('item', it.name), name: it.name, itemId: it.id, source: 'item',
          price: it.price > 0 ? it.price : null, currency: it.currency || currency,
          _count: it.price > 0 ? [1, 4] : [1, 1], today: false
        };
        if (out[g.key] || taken(g.name, g.itemId)) return;
        // A priced item is stock; one with no price yet is a find the
        // Director will want to look at.
        (g.price ? always : finds).push(g);
      });
    }
    if (src.system && pool.system) {
      pool.system.forEach(function (s) {
        if (cap > 0 && s.rarity > cap) return;
        var g = {
          key: key('system', s.name), name: s.name, slug: s.slug, source: 'system', kind: s.kind || '',
          rarity: s.rarity || 0,
          price: s.hasPrice && s.price > 0 ? s.price : null, currency: currency, _count: [1, 1], today: false
        };
        if (out[g.key] || taken(g.name)) return;
        finds.push(g);
      });
    }
    if (src.table && pool.table && pool.table.entries) {
      pool.table.entries.forEach(function (en) {
        if (!en || !en.name) return;
        var g = {
          key: key('table', en.name), name: en.name, source: 'table', price: null, currency: currency,
          _count: [1, 2], today: false
        };
        if (out[g.key] || taken(g.name)) return;
        finds.push(g);
      });
    }

    var keptAlways = locked.filter(function (g) { return !g.today; }).length;
    var keptFinds = locked.length - keptAlways;
    var room = count - locked.length;
    var wantFinds = Math.max(0, findShare(count, finds.length + keptFinds) - keptFinds);
    wantFinds = Math.min(wantFinds, room, finds.length);
    var wantAlways = Math.min(room - wantFinds, always.length);
    // Leftover room goes to whichever pool still has goods.
    var spare = room - wantFinds - wantAlways;
    if (spare > 0) wantFinds = Math.min(wantFinds + spare, finds.length);

    var rA = stream(seed, 'always'), rF = stream(seed, 'finds'), rQ = stream(seed, 'quantity');
    var drawn = shuffled(always, rA).slice(0, wantAlways).concat(
      shuffled(finds, rF).slice(0, wantFinds).map(function (g) { g.today = true; return g; }));
    drawn.forEach(function (g) {
      g.quantity = between(rQ, g._count[0], g._count[1]);
      delete g._count;
      if (g.price == null) g.note = 'Set a price';
    });

    var all = locked.map(function (g) { return g; }).concat(drawn);
    // Everyday stock first, then today's shelf; names keep a stable order.
    all.sort(function (a, b) {
      if (!!a.today !== !!b.today) return a.today ? 1 : -1;
      var pa = a.source === 'everyday' ? 1 : 0, pb = b.source === 'everyday' ? 1 : 0;
      if (pa !== pb) return pb - pa;
      return a.name < b.name ? -1 : a.name > b.name ? 1 : 0;
    });

    var nAlways = all.filter(function (g) { return !g.today; }).length;
    var nFinds = all.length - nAlways;
    var unpriced = all.filter(function (g) { return g.price == null; }).length;
    var short = all.length < count;
    var parts = [];
    var level2 = level === 'fair' ? 'fair prices' : level === 'cheap' ? 'low prices' : 'high prices';
    parts.push(cap1(words(all.length)) + ' ' + (all.length === 1 ? 'good' : 'goods') + ' for ' + KIND_WORDS[kind] + ', at ' + level2 + '.');
    if (nFinds > 0) {
      parts.push(cap1(words(nAlways)) + (nAlways === 1 ? ' is' : ' are') + ' always on the shelves; ' +
        words(nFinds) + ' rarer ' + (nFinds === 1 ? 'find takes' : 'finds take') + ' turns on today’s shelf, ' +
        words(Math.min(spots, nFinds)) + ' a day.');
    } else if (all.length > 0) {
      parts.push('All of it is always on the shelves.');
    }
    if (short) parts.push('That is all the chosen sources have for this shop.');
    if (all.length === 0) parts = ['Nothing to draft. Switch on a source above.'];

    return {
      goods: all,
      summary: parts.join(' '),
      stats: { count: all.length, always: nAlways, finds: nFinds, unpriced: unpriced, short: short, seed: seed }
    };
  }

  return { run: run, roundPrice: roundPrice, KIND_WORDS: KIND_WORDS, PRICE_FACTOR: PRICE_FACTOR };
});
