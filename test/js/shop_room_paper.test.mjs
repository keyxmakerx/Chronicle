// shop_room_paper.test.mjs — the shop room's paper look
// (static/js/widgets/shop_room_paper.js): every shop type and room shape
// stands up as cut card from the same room the lit look draws, price tags
// lean back to face the viewer, pointers map back onto the floor, and moving
// one of the room's own decorations never moves the goods.

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
for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
const SR = sandbox.ShopRoom;
const Paper = sandbox.ShopRoomPaper;

function state(over) {
  return Object.assign({ roomType: 'forge', pal: 'oak', setting: 'room', look: 'paper', fx: 'full', size: 'm', full: 'normal', deco: 'some', keep: true,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [], key: 3, name: 'Shop', dark: false }, over);
}
const goods = (n) => Array.from({ length: n }, (_, i) => ({ id: 'r' + i, n: 'Item ' + i, ic: 'gem', c: '#60a5fa', price: i + ' gp', out: i === 2 }));
const count = (s, re) => (s.match(re) || []).length;

test('every furniture kind has a paper drawing', () => {
  const room = SR.createRoom(state());
  for (const k of room.KINDS) assert.equal(typeof Paper.KIND[k], 'function', k);
});

test('every shop type and room shape stands up as paper, with its goods and tags', () => {
  for (const [type] of SR.ROOM_TYPES) {
    for (const setting of ['room', 'tower', 'tree', 'burrow', 'cave']) {
      const S = state({ roomType: type, setting, its: goods(6) });
      const room = SR.createRoom(S);
      room.generate();
      const view = room.draw();
      const html = Paper.html({ view, its: S.its, name: 'The Gilded Anvil', icons: room.ICONS, fx: 'full' }).html;
      const where = `${type} in a ${setting}`;
      assert.equal(count(html, / data-p="/g), S.pieces.length, `${where}: one card per piece`);
      const shown = view.placed.filter((q) => q.good !== undefined).length;
      assert.equal(count(html, / data-i="/g), shown, `${where}: one card per good on show`);
      assert.equal(count(html, /class="srp-tag"[^>]*rotateX\(-56deg\)/g), shown, `${where}: every tag leans back to face the viewer`);
      assert.equal(count(html, /class="srp-at srp-wall"/g), view.room.back.length, `${where}: one card per back wall`);
      assert.ok(!/mix-blend-mode/.test(html), `${where}: no blending inside the 3D book`);
      assert.match(html, /class="srp-at srp-keeper"/, `${where}: a keeper`);
    }
  }
});

test('warm lights warm the cards near them, and the light setting draws them flat', () => {
  const S = state({ roomType: 'forge' });
  const room = SR.createRoom(S);
  room.generate();
  const view = room.draw();
  const full = Paper.html({ view, its: [], icons: room.ICONS, fx: 'full' }).html;
  assert.match(full, /class="srp-glow warm"/);
  assert.match(full, /sepia\(0\.[1-9]/, 'something near the forge is warmed');
  const light = Paper.html({ view, its: [], icons: room.ICONS, fx: 'light' }).html;
  assert.ok(!/--lf:/.test(light));
});

test('a pointer maps back onto the floor exactly', () => {
  // A made-up perspective view of a floor 8 tiles across.
  const H = [1.2, -0.4, 300, 0.5, 0.7, 120, 0.0004, 0.0009];
  const floor = [[0, 0], [8, 0], [8, 8], [0, 8]];
  const screen = floor.map(([x, y]) => Paper.apply(H, x, y));
  const back = Paper.solve(screen, floor);
  for (const [x, y] of [[1, 1], [4, 6.5], [7.25, 0.5]]) {
    const [sx, sy] = Paper.apply(H, x, y);
    const [fx, fy] = Paper.apply(back, sx, sy);
    assert.ok(Math.abs(fx - x) < 1e-6 && Math.abs(fy - y) < 1e-6, `${x},${y} came back as ${fx},${fy}`);
  }
});

test('picking up one of the room\'s own decorations keeps every decoration and every good where it was', () => {
  const S = state({ roomType: 'general', deco: 'lots', its: goods(5) });
  const room = SR.createRoom(S);
  room.generate();
  const before = room.draw().placed;
  sandbox.ShopRoomDecor.freeze(S, before);
  assert.equal(S.deco, 'none');
  const after = room.draw().placed;
  const key = (q) => `${q.piece}:${q.spot}:${q.good ?? q.icon}`;
  assert.deepEqual(after.map(key).sort(), before.map(key).sort());
  assert.ok(after.filter((q) => q.icon).every((q) => q.mine), 'every decoration is now hand-placed');
});

test('the look round-trips, and a room saved before paper reads as the lit room', () => {
  const S = state({ look: 'paper' });
  SR.createRoom(S).generate();
  const L = SR.toLayout(S);
  assert.equal(L.look, 'paper');
  const S2 = state({ look: 'lit' });
  SR.fromLayout(S2, JSON.parse(JSON.stringify(L)));
  assert.equal(S2.look, 'paper');
  delete L.look;
  SR.fromLayout(S2, L);
  assert.equal(S2.look, 'lit');
});
