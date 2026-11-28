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
for (const f of ['shop_room_icons.js', 'shop_room_counter.js', 'shop_room_parts_strange.js', 'shop_room_parts_trades.js', 'shop_room_parts_places.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
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
  const t = plain(C.normalise({ name: 'x'.repeat(200), skeleton: { shape: 'hexagon', display: 'domes' }, theme: { wall: 'wallpaper', wood: 'red', cloth: '#ABCDEF', warmth: 9, light: 5 }, props: ['anvil', 'anvil', 'dragon', 7, 'herbs'] }));
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
  assert.deepEqual(plain(C.starters.slice(0, 5).map((s) => s.name)), ['Plain counter', 'The Smithy', 'The Apothecary', 'The Arcane Shop', 'The Night Market'], 'the starters a shop type begins from keep their places');
  assert.equal(C.starters.length, 32);
  assert.equal(new Set(C.starters.map((s) => s.name)).size, 32, 'every starter has its own name');
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

test('the view outside is kept when known, and the town is saved as none', () => {
  const base = plain(C.starters[1]);
  const withScene = (scene) => plain(C.normalise(Object.assign({}, base, { theme: Object.assign({}, base.theme, { scene }) }))).theme;
  assert.equal(withScene('castle').scene, 'castle');
  for (const none of ['town', 'atlantis', 7, null, undefined]) assert.equal('scene' in withScene(none), false, String(none));
  assert.ok(C.options.theme.scene.length >= 18, 'the parts packs add their views');
});

test('every view draws cleanly under every sky and at any width', () => {
  const K = C.kit;
  for (const s of C.options.theme.scene) {
    const f = K.CM_SCENES[s];
    assert.equal(typeof f, 'function', s);
    for (const tm of ['', 'dusk', 'night', 'rain']) for (const [w, h] of [[212, 92], [1800, 140]]) {
      const svg = f(10, 200, w, h, '#090d1a', 9, tm);
      assert.ok(svg.length > 50, s + ' draws something');
      assert.doesNotMatch(svg, /NaN|undefined|Infinity/, s + ' ' + tm + ' ' + w);
      assert.equal(f(10, 200, w, h, '#090d1a', 9, tm), svg, s + ' draws the same each time');
    }
  }
});

test("the owner's picture is kept only as a media id, with known placements", () => {
  const id = '0b9c2f4e-1111-4222-8333-444455556666';
  const base = plain(C.starters[1]);
  const withPic = (picture) => plain(C.normalise(Object.assign({}, base, { theme: Object.assign({}, base.theme, { picture }) })));
  assert.deepEqual(withPic({ id: id.toUpperCase(), on: 'outside', fit: 'repeat' }).theme.picture, { id, on: 'outside', fit: 'repeat' }, 'kept, id lower-cased');
  assert.deepEqual(withPic({ id, on: 'ceiling', fit: 'stretch' }).theme.picture, { id, on: 'wall', fit: 'fill' }, 'odd placements fall back');
  for (const bad of [null, 'x', { id: '/media/' + id }, { id: 'https://evil.example/a.png' }, { id: id + '/../x' }, { on: 'wall' }]) {
    assert.equal('picture' in withPic(bad).theme, false, 'no picture from ' + JSON.stringify(bad));
  }
  assert.equal('picture' in plain(C.normalise(base)).theme, false, 'a theme without one gains none');
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
  assert.deepEqual([...O.theme.scene].sort(), goSet('shopRoomCounterScenes'));
  assert.deepEqual([...O.props].sort(), goSet('shopRoomCounterProps'));
  assert.deepEqual([...O.picture.on].sort(), goSet('shopRoomPictureOn'));
  assert.deepEqual([...O.picture.fit].sort(), goSet('shopRoomPictureFit'));
  for (const p of O.props) assert.ok(O.slots[O.propAt[p].at], p + ' has a slot to be placed in');
});

test('the server starters in Go are the widget starters', () => {
  const src = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_counter_starters.go'), 'utf8');
  const lit = src.match(/shopRoomCounterStartersJSON = `([^`]*)`/);
  assert.ok(lit, 'starters literal');
  assert.deepEqual(JSON.parse(lit[1]), plain(C.starters));
  const map = src.match(/shopRoomCounterStarterFor = map\[string\]int\{([^}]*)\}/);
  assert.ok(map, 'type mapping');
  const goFor = Object.fromEntries([...map[1].matchAll(/"([^"]*)":\s*(\d+)/g)].map((x) => [x[1], +x[2]]));
  for (const type of ['forge', 'apothecary', 'jeweler', 'library', 'market', 'general', '']) {
    assert.equal(C.starters[goFor[type] || 0].name, C.starterFor(type).name, type);
  }
});
