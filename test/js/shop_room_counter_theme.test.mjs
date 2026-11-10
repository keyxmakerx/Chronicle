// shop_room_counter_theme.test.mjs — the counter look's theme
// (static/js/widgets/shop_room_counter.js and shop_room.js): odd values fall
// back, each shop type starts from its own starter, the theme rides in the
// saved layout, and the widget's options are the ones the server accepts.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const dir = join(root, 'static', 'js', 'widgets');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
for (const f of ['shop_room_icons.js', 'shop_room_counter.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
const C = sandbox.ShopRoomCounter;
const SR = sandbox.ShopRoom;
const plain = (o) => JSON.parse(JSON.stringify(o));

const goValidate = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_validate.go'), 'utf8');
const goSet = (name) => {
  const m = goValidate.match(new RegExp(name + '\\s*=\\s*setOf\\(([^)]*)\\)'));
  assert.ok(m, name);
  return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]).sort();
};

function state(over) {
  return Object.assign({ roomType: 'forge', pal: 'oak', setting: 'room', look: 'counter', size: 'm', full: 'normal', deco: 'some', mood: '', efx: null,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], counter: null }, over);
}

test('odd or missing theme values fall back to the plain counter', () => {
  const plainTheme = plain(C.normalise(null));
  assert.equal(plainTheme.name, 'Plain counter');
  for (const bad of [undefined, null, 'smithy', 7, [], {}]) assert.deepEqual(plain(C.normalise(bad)), plainTheme, String(bad));
  const t = plain(C.normalise({ name: 'x'.repeat(200), skeleton: { shape: 'hexagon', display: 'domes' }, theme: { wall: 'marble', wood: 'red', cloth: '#ABCDEF', warmth: 9, light: 5 }, props: ['anvil', 'anvil', 'dragon', 7, 'herbs'] }));
  assert.equal(t.name.length, 60);
  assert.equal(t.skeleton.shape, 'square', 'unknown shape falls back');
  assert.equal(t.skeleton.display, 'domes', 'a known value is kept');
  assert.equal(t.theme.wall, 'plaster');
  assert.equal(t.theme.wood, '#6b4527', 'a bad colour falls back');
  assert.equal(t.theme.cloth, '#abcdef', 'colours are lower-cased');
  assert.equal(t.theme.warmth, 0.7, 'out-of-range warmth falls back');
  assert.equal(t.theme.light, 'lamp');
  assert.deepEqual(plain(t.props), ['anvil', 'herbs'], 'props are known, once each');
});

test('each shop type starts from its own starter, anything else from the plain counter', () => {
  assert.deepEqual(plain(C.starters.map((s) => s.name)), ['Plain counter', 'The Smithy', 'The Apothecary', 'The Arcane Shop', 'The Night Market']);
  const by = { forge: 'The Smithy', apothecary: 'The Apothecary', library: 'The Arcane Shop', market: 'The Night Market', general: 'Plain counter', jeweler: 'Plain counter', constructor: 'Plain counter', '': 'Plain counter' };
  for (const k of Object.keys(by)) assert.equal(C.starterFor(k).name, by[k], k);
  // Every starter is already a valid, complete theme.
  for (const s of C.starters) assert.deepEqual(plain(C.normalise(s)), plain(s), s.name);
});

test('the theme round trips through the saved layout, and no theme is not saved', () => {
  const theme = C.normalise(C.starters[3]);
  const L = SR.toLayout(state({ counter: theme }));
  assert.deepEqual(plain(L.counter), plain(theme));
  const S2 = state({ look: 'lit' });
  SR.fromLayout(S2, plain(L));
  assert.deepEqual(plain(S2.counter), plain(theme));
  assert.equal('counter' in SR.toLayout(state()), false, 'a shop with no theme saves none');
  SR.fromLayout(S2, plain(SR.toLayout(state())));
  assert.equal(S2.counter, null);
  for (const odd of ['plain', 5, true]) { SR.fromLayout(S2, Object.assign(plain(L), { counter: odd })); assert.equal(S2.counter, null, String(odd)); }
  SR.fromLayout(S2, Object.assign(plain(L), { counter: { skeleton: { shape: 'hexagon' }, props: ['dragon'] } }));
  assert.equal(S2.counter.skeleton.shape, 'square', 'an odd saved theme is normalised on the way in');
  assert.deepEqual(plain(SR.toLayout(S2).counter), plain(S2.counter), 'and on the way out');
});

test('the widget offers the options the server accepts', () => {
  const O = C.options;
  assert.deepEqual([...O.skeleton.shape].sort(), goSet('shopRoomCounterShapes'));
  assert.deepEqual([...O.skeleton.opening].sort(), goSet('shopRoomCounterOpenings'));
  assert.deepEqual([...O.skeleton.display].sort(), goSet('shopRoomCounterDisplays'));
  assert.deepEqual([...O.skeleton.counter].sort(), goSet('shopRoomCounterCounters'));
  assert.deepEqual([...O.theme.wall].sort(), goSet('shopRoomCounterWalls'));
  assert.deepEqual([...O.theme.light].sort(), goSet('shopRoomCounterLights'));
  assert.deepEqual([...O.theme.outside].sort(), goSet('shopRoomCounterOutsides'));
  assert.deepEqual([...O.props].sort(), goSet('shopRoomCounterProps'));
  for (const p of O.props) assert.ok(O.slots[O.propAt[p].at], p + ' has a slot to be placed in');
});
