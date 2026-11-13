// shop_rolls.test.mjs — the pure parts of rolls at a shop's counter: the roll sheet's words
// (static/js/widgets/shop_rolls.js) and the shop room's roll settings and haggled prices
// (static/js/widgets/shop_room.js), run here without a page.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets');
function load(files) {
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of files) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
  return sandbox;
}
const U = load(['shop_rolls.js']).ShopRolls.util;
const SR = load(['shop_room_icons.js', 'shop_room.js']).ShopRoom;

test('the roll button names the test and its dice', () => {
  assert.equal(U.rollLabel({ label: 'Presence', dice: '2d10', mod: 2 }), 'Presence test (2d10 + 2)');
  assert.equal(U.rollLabel({ label: 'Charisma', dice: '1d20', mod: -1 }), 'Charisma test (1d20 − 1)');
  assert.equal(U.rollLabel({ label: '', dice: '1d20', mod: 0 }), 'Roll (1d20)', 'a system that adds nothing rolls the bare dice');
  assert.equal(U.rollLabel(null), 'Roll');
});

test('the sum line adds the dice and the modifier', () => {
  assert.equal(U.sumLine([{ value: 9 }, { value: 8 }], 2, 19), '9 + 8 + 2 = 19');
  assert.equal(U.sumLine([{ value: 12 }], -1, 11), '12 + (-1) = 11');
  assert.equal(U.sumLine([{ value: 4 }, { value: 5 }], 0, 9), '4 + 5 = 9');
});

test('each haggle and look outcome says what changed', () => {
  assert.equal(U.outcome('haggle', { tier: 3, haggle: { pct: -20, price: 40, currency: 'gp' } }).head, 'A great deal');
  assert.match(U.outcome('haggle', { tier: 2, haggle: { pct: -10, price: 45, currency: 'gp' } }).line, /now 45 gp for you today/);
  assert.match(U.outcome('haggle', { tier: 1, haggle: { pct: 10, price: 55, currency: 'gp' } }).line, /went up to 55 gp.*last word/);
  assert.equal(U.outcome('haggle', { tier: 2, haggle: { pct: 0, price: 50 } }).head, 'No change');
  assert.equal(U.outcome('look', { tier: 3 }).head, 'You notice everything');
  assert.equal(U.outcome('look', { tier: 2 }).head, 'You notice something');
  assert.equal(U.outcome('look', { tier: 1 }).head, 'Nothing more to see');
});

test('the dice look is the one picked at the Table, stepping down to what this page can draw', () => {
  assert.equal(U.lookKey('c1', 'u1'), 'chronicle.table.look.c1.u1');
  assert.equal(U.pickLook('rune', true, true), 'rune');
  assert.equal(U.pickLook('rune', false, true), 'quick');
  assert.equal(U.pickLook('rune', false, false), 'none');
  assert.equal(U.pickLook('', true, true), 'quick', 'the Table starts on Quick 2D');
  assert.equal(U.pickLook('none', true, true), 'none');
  assert.deepEqual(Array.from(U.LOOKS, (l) => l[0]), ['rune', 'quick', 'none']);
});

test('how hard the keeper is reads as a sentence', () => {
  assert.equal(U.againstLine('Hard'), ' The keeper is hard to move today.');
  assert.equal(U.againstLine('DC 15'), ' You need DC 15.');
  assert.equal(U.againstLine('<b>x</b>'), '', 'anything else says nothing');
});

test('roll settings fall back to the defaults for anything odd', () => {
  const d = SR.normRolls(null);
  assert.deepEqual(JSON.parse(JSON.stringify(d)), { haggle: false, look: false, stubborn: 'usual', haggleStat: '', lookStat: '', great: 20, middling: 10, poor: 10 });
  const n = SR.normRolls({ haggle: true, look: 'yes', stubborn: 'mean', haggleStat: 'Presence!', lookStat: 'reason', great: 95, middling: '15', poor: 150 });
  assert.equal(n.haggle, true);
  assert.equal(n.look, false, 'only a real true switches a roll on');
  assert.equal(n.stubborn, 'usual');
  assert.equal(n.haggleStat, '', "a stat key is a plain slug, like the game system's");
  assert.equal(n.lookStat, 'reason');
  assert.equal(n.great, 20, 'more than 90% off is not allowed');
  assert.equal(n.middling, 15);
  assert.equal(n.poor, 150);
});

test('a haggled price replaces the listed one for display and keeps what it was', () => {
  const its = [{ id: 11, n: 'Healing draught', p: 50, price: '50 gold', priceUsual: '50 gp' }, { id: 12, n: 'Rope', p: 1, price: '1 gold', priceUsual: '' }];
  const out = SR.haggled(its, { 11: { haggle: { tier: 3, pct: -20, price: 40, was: 50 } }, 12: { look: { tier: 2, note: 'x', extra: '' } } });
  assert.equal(out[0].p, 40);
  assert.equal(out[0].price, '40 gold');
  assert.equal(out[0].priceUsual, '40 gp');
  assert.equal(out[0].was, '50 gold');
  assert.equal(its[0].p, 50, 'the listing itself is untouched');
  assert.equal(out[1], its[1], 'a good with no haggle is passed through');
  assert.equal(SR.haggled(its, null), its);
});
