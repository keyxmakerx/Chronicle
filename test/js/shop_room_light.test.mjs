// shop_room_light.test.mjs — the lit room (static/js/widgets/shop_room.js):
// shadows fall away from the nearest warm light, the room's living details
// (flames, embers, dust, sparks) are drawn only with full effects, and the
// same room draws the same details every time.

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
const castOffset = sandbox.ShopRoomLight.castOffset;

function state(over) {
  return Object.assign({ roomType: 'forge', pal: 'oak', setting: 'room', fx: 'full', size: 'm', full: 'normal', deco: 'none', keep: true,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [], key: 3, name: 'Shop', dark: false }, over);
}

test('a shadow points away from the nearest warm light', () => {
  const cases = [
    { warm: [[0, 0]], at: [3, 0], want: [1, 0] },
    { warm: [[0, 0]], at: [0, 3], want: [0, 1] },
    { warm: [[5, 5], [0, 0]], at: [1, 1], want: [1, 1] },
    { warm: [[5, 5], [0, 0]], at: [4, 4], want: [-1, -1] },
  ];
  for (const c of cases) {
    const [dx, dy] = castOffset(c.warm, c.at[0], c.at[1], 1);
    assert.ok(Math.sign(Math.round(dx * 1000)) === c.want[0] && Math.sign(Math.round(dy * 1000)) === c.want[1], `${JSON.stringify(c)} gave ${dx},${dy}`);
  }
});

test('taller pieces cast longer shadows, and a room with no warm light keeps the old direction', () => {
  const short = castOffset([[0, 0]], 3, 0, .7), tall = castOffset([[0, 0]], 3, 0, 2.6);
  assert.ok(tall[0] > short[0]);
  const none = castOffset([], 3, 3, 2);
  assert.deepEqual([...none], [1, .32]);
});

test('a forge room draws flames and embers with full effects, and none with light effects', () => {
  const S = state();
  const room = SR.createRoom(S);
  room.generate();
  assert.ok(S.pieces.some((p) => p.kind === 'forge'), 'forge recipe places a forge');
  const full = room.draw().svg;
  assert.match(full, /class="shr-life"/);
  assert.match(full, /class="shr-flame"/);
  assert.match(full, /class="shr-ember"/);
  assert.match(full, /class="shr-spark"/, 'the anvil throws sparks');
  S.fx = 'light';
  const light = room.draw().svg;
  assert.doesNotMatch(light, /shr-life|shr-ember/);
});

test('the same room draws the same living details every time', () => {
  const S = state();
  const room = SR.createRoom(S);
  room.generate();
  assert.equal(room.draw().svg, room.draw().svg);
});
