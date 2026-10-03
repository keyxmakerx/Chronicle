// shop_room.test.mjs — the shop room's drawing engine (static/js/widgets/
// shop_room.js) is a pure function of the room state, so it runs here without a
// DOM. These pin what the generator promises the GM: every shop type and
// setting builds a room with its keeper's counter, nothing overlaps, furniture
// stays inside the room, pinned pieces survive a regenerate, the same seeds
// always give the same room, and user text never reaches the SVG unescaped.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const dir = join(here, '..', '..', 'static', 'js', 'widgets');

function load() {
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(dir, 'shop_room_icons.js'), 'utf8'), sandbox);
  vm.runInContext(readFileSync(join(dir, 'shop_room.js'), 'utf8'), sandbox);
  return sandbox.ShopRoom;
}
const SR = load();

function state(over) {
  return Object.assign({ roomType: 'general', pal: 'oak', setting: 'room', fx: 'full', size: 'm', full: 'normal', deco: 'some', keep: true,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [], key: 42, name: 'Shop', dark: false }, over);
}
const TYPES = SR.ROOM_TYPES.map((t) => t[0]);
const SETTINGS = ['room', 'tower', 'tree', 'burrow', 'cave'];

for (const setting of SETTINGS) {
  test(`${setting}: every shop type gets a counter, no overlaps, all inside the room`, () => {
    for (const roomType of TYPES) {
      for (let seed = 1; seed <= 4; seed++) {
        const S = state({ roomType, setting, seeds: { room: seed, goods: 1, deco: 1 } });
        const room = SR.createRoom(S);
        room.generate();
        assert.ok(S.pieces.some((p) => p.kind === 'counter'), `${roomType}/${seed} has the keeper's counter`);
        for (const p of S.pieces) {
          assert.equal(room.clashes(p, S.pieces, room.size()), false, `${roomType}/${seed}: ${p.kind} is placed legally`);
        }
        const out = room.draw();
        assert.match(out.svg, /^<defs>/);
        assert.ok(out.keeperAt, 'the counter gives the portrait a spot');
      }
    }
  });
}

test('the same seeds always build the same room', () => {
  const a = state({ roomType: 'forge', setting: 'tower' }), b = state({ roomType: 'forge', setting: 'tower' });
  SR.createRoom(a).generate(); SR.createRoom(b).generate();
  assert.deepEqual(JSON.parse(JSON.stringify(a.pieces)), JSON.parse(JSON.stringify(b.pieces)));
});

test('pinned pieces stay where the GM put them through "Generate whole room"', () => {
  const S = state({ roomType: 'library' });
  const room = SR.createRoom(S);
  room.generate();
  const pinned = S.pieces.find((p) => p.kind === 'counter');
  pinned.pinned = true;
  const where = { x: pinned.x, y: pinned.y };
  S.seeds.room = 9;
  room.generate();
  const after = S.pieces.find((p) => p.id === pinned.id);
  assert.ok(after, 'the pinned counter is still there');
  assert.deepEqual({ x: after.x, y: after.y }, where);
  assert.equal(S.pieces.filter((p) => p.kind === 'counter').length, 1, 'a pinned counter counts toward the must-haves');
  S.keep = false; S.seeds.room = 10;
  room.generate();
  assert.ok(!S.pieces.some((p) => p.pinned), 'unticking "keep" lets the generator start over');
});

test('shop and item names are escaped in the drawing', () => {
  const S = state({ name: '<img src=x onerror=alert(1)>' });
  const room = SR.createRoom(S);
  room.generate();
  S.its = SR.shopItems([{ id: 7, relationType: 'sells', targetEntityId: 'e1', targetEntityName: '"><script>x</script>', metadata: { price: 3 } }], {}, room.MAT, room.ICONS);
  const svg = room.draw().svg;
  assert.ok(!svg.includes('<img'), 'shop name is escaped');
  assert.ok(!svg.includes('<script'), 'item name is escaped');
});

test('item look: the GM override, else the item icon, else a box; colours are never raw input', () => {
  const room = SR.createRoom(state());
  const rels = [
    { id: 1, targetEntityId: 'a', targetEntityName: 'Sword', targetEntityIcon: 'fa-sword', targetEntityColor: '#112233', metadata: { price: 15, quantity: 2 } },
    { id: 2, targetEntityId: 'b', targetEntityName: 'Odd', targetEntityIcon: 'fa-not-a-real-icon', targetEntityColor: 'red;stroke:url(x)', metadata: { quantity: 0 } },
    { id: 3, targetEntityId: '', metadata: { custom_name: 'Rope', in_stock: false } },
  ];
  const its = SR.shopItems(rels, { 1: { icon: 'crown', color: 'gold' } }, room.MAT, room.ICONS);
  assert.equal(its[0].ic, 'crown');
  assert.equal(its[0].c, room.MAT.gold);
  assert.equal(its[0].qty, 2);
  assert.equal(its[1].ic, 'box');
  assert.equal(its[1].c, room.MAT.steel, 'a colour that is not #rrggbb falls back');
  assert.equal(its[1].out, true, 'quantity 0 is sold out');
  assert.equal(its[2].n, 'Rope');
  assert.equal(its[2].out, true);
});

test('layout round-trips through the saved form', () => {
  const S = state({ roomType: 'apothecary', setting: 'burrow', lines: ['Hello'] });
  const room = SR.createRoom(S);
  room.generate();
  S.pieces[0].pinned = true; S.portrait = [30, 40]; S.ov = { 5: { icon: 'leaf', color: 'green' } };
  const L = SR.toLayout(S);
  const T = state();
  assert.equal(SR.fromLayout(T, JSON.parse(JSON.stringify(L))), true);
  assert.deepEqual(SR.toLayout(T), L);
});

test('every icon name the server accepts exists in the widget', () => {
  const go = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'armory', 'shop_room_icons.go'), 'utf8');
  const names = [...go.matchAll(/"([a-z-]+)"/g)].map((m) => m[1]);
  const room = SR.createRoom(state());
  assert.ok(names.length >= 50);
  for (const n of names) assert.ok(room.ICONS[n], `icon ${n}`);
  assert.equal(Object.keys(room.ICONS).length, names.length, 'and the widget offers no icon the server would refuse');
});

test('basket: totals, sold-out lines skipped, and why Buy is off', () => {
  const its = [
    { id: 1, p: 10, cur: 'gp', out: false },
    { id: 2, p: 2.5, cur: 'gp', out: false },
    { id: 3, p: 99, cur: 'gp', out: true },
    { id: 4, p: 5, cur: 'sp', out: false },
  ];
  const kaela = { id: 'k', name: 'Kaela', money: 30 };
  let sm = SR.basketSummary({ 1: 2, 2: 2, 3: 1 }, its, kaela);
  assert.equal(sm.count, 4);
  assert.equal(sm.total, 25);
  assert.equal(sm.currency, 'gp');
  assert.equal(sm.short, false);
  assert.equal(sm.mixed, false);
  assert.equal(SR.basketSummary({ 1: 4 }, its, kaela).short, true);
  assert.equal(SR.basketSummary({ 1: 1, 4: 1 }, its, kaela).mixed, true);
  sm = SR.basketSummary({ 1: 1 }, its, { id: 'x', name: 'X', money: null });
  assert.equal(sm.noField, true);
  assert.equal(sm.short, false);
  assert.equal(SR.basketSummary({}, its, kaela).count, 0);
});

test('basket: currency labels match regardless of case and spacing', () => {
  const its = [{ id: 1, p: 1, cur: 'gp', out: false }, { id: 2, p: 1, cur: ' GP', out: false }];
  assert.equal(SR.basketSummary({ 1: 1, 2: 1 }, its, { id: 'k', money: 5 }).mixed, false);
});

test('basket: a purse compares the basket in copper, so silver prices are not read as gold', () => {
  const its = [{ id: 1, p: 5, cur: 'sp', out: false }, { id: 2, p: 10, cur: 'gp', out: false }];
  const purse = { id: 'p', moneyKey: 'gp', kind: 'purse', money: 10.23, moneyCp: 1023, purse: { cp: 3, sp: 2, ep: 0, gp: 10, pp: 0 } };
  let sm = SR.basketSummary({ 1: 1 }, its.slice(0, 1), purse);
  assert.equal(sm.short, false);
  assert.equal(sm.moneyText, '10 gp 2 sp 3 cp');
  // 20 sp = 200 cp fits; 21 gp does not even though 21 > 10.23 gp.
  assert.equal(SR.basketSummary({ 1: 20 }, its.slice(0, 1), purse).short, false);
  assert.equal(SR.basketSummary({ 2: 2 }, its.slice(1), purse).short, true);
  assert.equal(SR.basketSummary({ 2: 1 }, its.slice(1), purse).short, false);
});

test('basket: a gold-only sheet converts a silver price to gold', () => {
  const its = [{ id: 1, p: 5, cur: 'sp', out: false }];
  const gold = { id: 'g', moneyKey: 'gp', kind: 'coins', money: 0.5, moneyCp: 50 };
  assert.equal(SR.basketSummary({ 1: 1 }, its, gold).short, false);
  assert.equal(SR.basketSummary({ 1: 2 }, its, gold).short, true);
  assert.equal(SR.basketSummary({ 1: 1 }, its, gold).moneyText, '0.5 gp');
  // A currency that is not a coin keeps the plain comparison.
  const odd = [{ id: 1, p: 2, cur: 'credits', out: false }];
  assert.equal(SR.basketSummary({ 1: 1 }, odd, gold).short, true);
});

test('basket: half a copper rounds up to a whole copper', () => {
  const its = [{ id: 1, p: 0.5, cur: 'cp', out: false }];
  assert.equal(SR.basketSummary({ 1: 1 }, its, { id: 'g', moneyKey: 'gp', kind: 'coins', money: 0, moneyCp: 0 }).short, true);
  assert.equal(SR.basketSummary({ 1: 1 }, its, { id: 'g', moneyKey: 'gp', kind: 'coins', money: 0.01, moneyCp: 1 }).short, false);
});

test('basket: Wealth needs the dearest unit price, not the total', () => {
  const its = [{ id: 1, p: 2, cur: 'gp', out: false }, { id: 2, p: 4, cur: 'gp', out: false }];
  const hero = { id: 'h', moneyKey: 'wealth', kind: 'wealth', money: 3 };
  let sm = SR.basketSummary({ 1: 5 }, its, hero);
  assert.equal(sm.short, false);
  assert.equal(sm.need, 2);
  assert.equal(sm.moneyText, 'Wealth 3, needs 2');
  sm = SR.basketSummary({ 1: 1, 2: 1 }, its, hero);
  assert.equal(sm.short, true);
  assert.equal(sm.need, 4);
  assert.equal(SR.basketSummary({}, its, hero).moneyText, 'Wealth 3');
  assert.equal(SR.basketSummary({}, its, hero).short, false);
});

test('formatPurse lists coins largest first and skips empty ones', () => {
  assert.equal(SR.formatPurse({ cp: 3, sp: 7, gp: 9 }), '9 gp 7 sp 3 cp');
  assert.equal(SR.formatPurse({ gp: 0 }), '0 gp');
});
