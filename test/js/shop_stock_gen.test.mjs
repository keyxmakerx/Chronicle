// shop_stock_gen.test.mjs: the draft behind "Fill the shelves" is a pure
// function of its inputs, keeps what the Director pinned, never invents a
// price, and scales only the everyday list by a day's food.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
const Gen = require(join(here, '..', '..', 'static', 'js', 'widgets', 'shop_stock_gen.js'));
const everyday = JSON.parse(readFileSync(join(here, '..', '..', 'static', 'shop-goods', 'everyday.json'), 'utf8')).goods;

const base = () => ({
  kind: 'general', count: 14, prices: 'fair', foodPrice: 1, currency: 'gp', seed: 'abc',
  sources: { everyday: true }, pool: { everyday },
});
const names = (r) => r.goods.map((g) => g.name);

test('the same seed gives the same draft; another seed differs', () => {
  assert.deepEqual(Gen.run(base()), Gen.run(base()));
  assert.notDeepEqual(names(Gen.run(base())), names(Gen.run({ ...base(), seed: 'xyz' })));
});

test('count is honoured and clamped', () => {
  for (const [count, want] of [[5, 5], [14, 14], [0, 14]]) {
    assert.equal(Gen.run({ ...base(), count }).goods.length, want, 'count ' + count);
  }
  // Asking for more than exists gives all there is, and says it ran short.
  const all = Gen.run({ ...base(), count: 500 });
  const pool = everyday.filter((e) => e.shops.includes('general')).length;
  assert.equal(all.goods.length, pool);
  assert.ok(all.goods.length <= 80 && all.stats.short);
});

test('every shop kind has goods in the everyday list', () => {
  for (const kind of Object.keys(Gen.KIND_WORDS)) {
    const r = Gen.run({ ...base(), kind });
    assert.ok(r.goods.length > 5, kind + ' drafted ' + r.goods.length);
  }
});

test('pinned goods stay, others are replaced, left-out goods never return', () => {
  const first = Gen.run(base());
  const pinned = first.goods.slice(0, 3);
  const dropped = first.goods[3].key;
  const again = Gen.run({ ...base(), seed: 'other', locked: pinned, leftOut: [dropped] });
  pinned.forEach((p) => assert.ok(again.goods.some((g) => g.key === p.key), 'lost pinned ' + p.name));
  assert.ok(!again.goods.some((g) => g.key === dropped));
  assert.equal(again.goods.length, 14);
});

test('goods the shop already sells are skipped, by name or page', () => {
  const first = Gen.run({ ...base(), count: 40 });
  const listed = first.goods.slice(0, 10).map((g) => ({ name: g.name.toUpperCase() }));
  const r = Gen.run({ ...base(), count: 40, pool: { everyday, listed } });
  listed.forEach((l) => assert.ok(!names(r).some((n) => n.toLowerCase() === l.name.toLowerCase())));
  const items = [{ id: 'p1', name: 'Rope', price: 3 }];
  const r2 = Gen.run({ ...base(), sources: { items: true }, pool: { items, listed: [{ itemId: 'p1' }] } });
  assert.equal(r2.goods.length, 0);
});

test('a day of food scales everyday prices; hand-set prices never move', () => {
  const items = [{ id: 'p1', name: 'Heirloom', price: 77, currency: 'sp' }];
  const mk = (foodPrice) => Gen.run({ ...base(), foodPrice, count: 80, sources: { everyday: true, items: true }, pool: { everyday, items } });
  const one = mk(1), five = mk(5);
  const ration = (r) => r.goods.find((g) => g.name === 'Trail rations, one day');
  assert.equal(ration(one).price, 1);
  assert.equal(ration(five).price, 5);
  for (const r of [one, five]) {
    const h = r.goods.find((g) => g.name === 'Heirloom');
    assert.equal(h.price, 77);
    assert.equal(h.currency, 'sp');
    assert.equal(h.itemId, 'p1');
  }
});

test('price level moves the everyday list only', () => {
  const at = (prices) => Gen.run({ ...base(), prices, count: 80 }).goods.find((g) => g.name === 'Trail rations, three days').price;
  assert.ok(at('cheap') < at('fair') && at('fair') < at('dear'));
});

test('a system good with no price is left unpriced, not guessed', () => {
  const system = [
    { slug: 'a', name: 'Axe', hasPrice: false, price: 0, rarity: 1 },
    { slug: 'b', name: 'Bell', hasPrice: true, price: 40, rarity: 1 },
    { slug: 'c', name: 'Crown', hasPrice: true, price: 900, rarity: 3 },
  ];
  const r = Gen.run({ ...base(), sources: { system: true }, rarityCap: 2, pool: { system } });
  const by = Object.fromEntries(r.goods.map((g) => [g.name, g]));
  assert.equal(by.Axe.price, null);
  assert.equal(by.Axe.note, 'Set a price');
  assert.equal(by.Bell.price, 40);
  assert.ok(!by.Crown, 'a rarity above the cap was drafted');
  assert.equal(r.stats.unpriced, 1);
});

test('finds go on today’s shelf and never outnumber what is available', () => {
  const table = { name: 'Odd things', entries: [{ name: 'Glass eye' }, { name: 'Moth jar' }] };
  const r = Gen.run({ ...base(), count: 80, sources: { everyday: true, table: true }, pool: { everyday, table } });
  const finds = r.goods.filter((g) => g.today);
  assert.ok(finds.length >= 1 && finds.length <= 2 + everyday.filter((e) => e.find).length);
  finds.forEach((g) => assert.ok(g.quantity >= 1));
  r.goods.filter((g) => g.source === 'table').forEach((g) => assert.equal(g.price, null));
});

test('with no source on, the draft is empty and says so', () => {
  const r = Gen.run({ ...base(), sources: {} });
  assert.equal(r.goods.length, 0);
  assert.match(r.summary, /Switch on a source/);
});

test('roundPrice keeps small prices exact and rounds large ones', () => {
  assert.equal(Gen.roundPrice(0.74), 0.74);
  assert.ok(Gen.roundPrice(1234.567) >= 1200);
});

test('the shipped everyday list is well formed and priced above nothing', () => {
  assert.ok(everyday.length >= 100);
  const seen = new Set();
  for (const g of everyday) {
    assert.ok(g.name && g.price > 0, g.name);
    assert.ok(g.count[0] >= 1 && g.count[1] >= g.count[0], g.name);
    assert.ok(!seen.has(g.name.toLowerCase()), 'duplicate ' + g.name);
    seen.add(g.name.toLowerCase());
  }
});
