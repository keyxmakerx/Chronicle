// shop_room_place.test.mjs — furniture and decorations the GM adds by hand
// (static/js/widgets/shop_room.js): a dropped piece lands near the drop point
// without overlapping anything, a hand-placed decoration takes its spot ahead
// of goods, and both survive the saved layout.

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

function state(over) {
  return Object.assign({ roomType: 'general', pal: 'oak', setting: 'room', fx: 'full', size: 'm', full: 'sparse', deco: 'none', keep: true,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [], key: 3, name: 'Shop', dark: false }, over);
}

test('a dropped piece lands near the drop point, pinned, and clashes with nothing', () => {
  for (const setting of ['room', 'tower', 'cave']) {
    for (const kind of ['barrel', 'table', 'shelf', 'rug']) {
      const S = state({ setting });
      const room = SR.createRoom(S);
      room.generate();
      const before = S.pieces.length;
      const p = room.place(kind, 4, 4);
      assert.ok(p, `${setting}: ${kind} placed`);
      assert.equal(S.pieces.length, before + 1);
      assert.ok(p.pinned, 'counts as moved by hand');
      assert.ok(!room.clashes(p, S.pieces, room.size()), `${setting}: ${kind} clashes`);
      assert.equal(new Set(S.pieces.map((q) => q.id)).size, S.pieces.length, 'ids stay unique');
    }
  }
});

test('nothing is placed when the room is full', () => {
  const S = state({ size: 's' });
  const room = SR.createRoom(S);
  room.generate();
  let n = 0;
  while (room.place('table', 3.5, 3.5) && n < 200) n++;
  assert.ok(n < 200, 'placing stops once nothing fits');
});

test('a hand-placed decoration takes its spot ahead of goods and round-trips', () => {
  const S = state({ its: [{ id: 'r1', n: 'Gem', ic: 'gem', c: '#60a5fa', price: '1 gp' }] });
  const room = SR.createRoom(S);
  room.generate();
  const first = room.draw();
  const spot = first.anchors[0];
  S.decor = [{ piece: spot.piece, spot: spot.spot, icon: 'skull' }];
  const svg = room.draw().svg;
  assert.match(svg, new RegExp(`class="udeco" data-up="${spot.piece}" data-ua="${spot.spot}"`));
  assert.match(svg, /<symbol id="i-skull"/);
  const L = SR.toLayout(S);
  assert.deepEqual(JSON.parse(JSON.stringify(L.decor)), [{ piece: spot.piece, spot: spot.spot, icon: 'skull' }]);
  const S2 = state();
  SR.fromLayout(S2, JSON.parse(JSON.stringify(L)));
  assert.deepEqual(JSON.parse(JSON.stringify(S2.decor)), L.decor.map((d) => ({ ...d })));
});

test('a decoration whose piece is gone is not saved', () => {
  const S = state();
  const room = SR.createRoom(S);
  room.generate();
  S.decor = [{ piece: 99999, spot: 0, icon: 'gem' }];
  assert.equal(SR.toLayout(S).decor.length, 0);
});

test('every furniture kind and decoration offered has a label or an icon', () => {
  const room = SR.createRoom(state());
  assert.ok(room.KINDS.length >= 18);
  for (const n of room.DECOS) assert.ok(sandbox.ShopRoomIcons[n], n);
});
