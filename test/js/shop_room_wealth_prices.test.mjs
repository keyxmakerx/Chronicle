// shop_room_wealth_prices.test.mjs — a Draw Steel hero pays with Wealth, so
// the shop (static/js/widgets/shop_room.js) shows each listing as the Wealth
// it needs, never as coin the hero does not have.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(dir, 'shop_room_icons.js'), 'utf8'), sandbox);
vm.runInContext(readFileSync(join(dir, 'shop_room.js'), 'utf8'), sandbox);
const SR = sandbox.ShopRoom;

const its = [{ id: 1, n: 'Healing Potion', p: 2, cur: 'sovereign', price: '2 Sovereign', priceUsual: '2 gp' }];

test('only a Wealth buyer sees Wealth levels', () => {
  assert.equal(SR.payingWithWealth({ kind: 'wealth' }), true);
  assert.equal(SR.payingWithWealth({ kind: 'coins' }), false);
  assert.equal(SR.payingWithWealth(null), false);
});

test('Wealth prices read as the level needed and leave the originals alone', () => {
  const out = SR.wealthPrices(its);
  assert.equal(out[0].price, 'Wealth 2');
  assert.equal(out[0].priceUsual, '');
  assert.equal(out[0].p, 2, 'the number the server compares is unchanged');
  assert.equal(its[0].price, '2 Sovereign');
});

test('a Wealth basket summary asks for the dearest level', () => {
  const sm = SR.basketSummary({ 1: 3 }, its, { kind: 'wealth', money: 1 });
  assert.equal(sm.need, 2);
  assert.equal(sm.short, true);
  assert.equal(SR.basketSummary({ 1: 3 }, its, { kind: 'wealth', money: 2 }).short, false);
});
