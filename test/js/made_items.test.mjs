// made_items.test.mjs — the Dark painted made-item pictures:
// static/js/widgets/made_items.js, and how craft_results.js and the shop
// room hand looks to it.
//
// Pins that only an item key and #rrggbb colours reach the markup (a look is
// stored data and the markup is inserted as HTML), that the six drawn shapes
// still find a picture, that the painted set is the default once loaded and
// the owner's lifelike or illustrated choice still wins, and that the
// recolouring keeps a picture's own colours when the recipe gives none.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const widgets = join(root, 'static', 'js', 'widgets');

function load(attr) {
  const window = {};
  const document = { getElementById: () => ({}), documentElement: { getAttribute: (k) => (k === 'data-craft-art' ? attr || null : null) } };
  const ctx = vm.createContext({ window, document });
  for (const f of ['craft_pictures.js', 'made_items.js', 'craft_results.js', 'craft_results_real.js']) vm.runInContext(readFileSync(join(widgets, f), 'utf8'), ctx);
  return window;
}
const W = load(), M = W.MadeItems, R = W.CraftResults;

test('every item key draws as a self-colouring picture', () => {
  assert.equal(M.keys().length, 80);
  for (const k of M.keys()) {
    const h = M.html({ shape: k }, 48);
    assert.match(h, new RegExp('^<img class="mi-pic" data-mi="' + k + '" data-mi-z="s" data-mi-c="#[0-9a-f]{6},#[0-9a-f]{6},#[0-9a-f]{6}" width="48" height="48" alt="" aria-hidden="true" decoding="async">$'));
  }
  // A large picture uses the bigger file.
  assert.match(M.html({ shape: 'sword' }, 180), /data-mi-z="l"/);
});

test('the drawn shapes find a painted item, and back', () => {
  for (const [shape, k] of [['potion', M.has('potion') ? 'potion' : 'bottle'], ['vial', 'ink'], ['blade', 'sword'], ['shield', 'shield'], ['ring', 'ring'], ['bundle', 'pouch']]) assert.equal(M.key(shape), k);
  assert.equal(M.key('castle'), null);
  assert.equal(M.html({ shape: 'castle' }, 40), '');
  assert.equal(M.oldShape('dagger'), 'blade');
  assert.equal(M.oldShape('skull'), 'bundle');
  assert.equal(M.oldShape('potion'), 'potion');
});

test('nothing from a look reaches the markup unchecked', () => {
  const bad = { shape: 'sword', body: 'red" onload="x', trim: '#12345', detail: 'url(javascript:x)' };
  const h = M.html(bad, 40, { label: 'A <sword> "x"', cls: 'crp-look' });
  for (const s of ['onload', 'javascript', '<sword']) assert.ok(!h.includes(s), s);
  // Colours the look did not give properly are the item's own.
  assert.match(h, /data-mi-c="#aab3bd,#b98a3a,#3f2a22"/);
  assert.match(h, /alt="A sword x"/);
  assert.equal(M.html({ shape: 'sword" onload="x' }, 40), '');
  assert.match(M.html({ shape: 'sword', body: '#ABCDEF' }, 40), /data-mi-c="#abcdef,/);
});

test('an item that moves gets its strip only when asked', () => {
  assert.ok(!M.html({ shape: 'candle' }, 180).includes('canvas'));
  assert.match(M.html({ shape: 'candle' }, 180, { move: true }), /<span class="mi-move"><img [^>]+><canvas data-mi-move="candle" data-mi-c="[^"]+" width="256" height="256" aria-hidden="true"><\/canvas><\/span>/);
  assert.ok(!M.html({ shape: 'sword' }, 180, { move: true }).includes('canvas'), 'a sword does not move');
  assert.ok(M.item('candle').moves && !M.item('sword').moves);
});

test('the painted set is the default once loaded; the owner can still pick the others', () => {
  assert.ok(R.isPainted());
  assert.match(R.svg({ shape: 'blade', body: '#336699' }, 48), /^<img class="mi-pic" data-mi="sword"[^>]+data-mi-c="#336699,/);
  for (const attr of ['lifelike', 'illustrated']) {
    const r = load(attr).CraftResults;
    assert.ok(!r.isPainted(), attr);
    // A painted item's look still draws in the older styles, as its nearest shape.
    const svg = r.svg({ shape: 'dagger', body: '#336699' }, 48);
    assert.match(svg, /^<svg class="crx"/, attr);
  }
  assert.ok(load('painted').CraftResults.isPainted());
});

test('shop shelves: an icon with a painted item stands as it', () => {
  assert.equal(M.forIcon('sword'), 'sword');
  assert.equal(M.forIcon('shield-halved'), 'shield');
  assert.equal(M.forIcon('broom'), null);
  assert.equal(M.forIcon('constructor'), null);
  const im = M.svgImage('sword', 10, 20, 30, { body: '#112233' });
  assert.equal(im, '<image data-mi="sword" data-mi-z="shelf" data-mi-c="#112233,#b98a3a,#3f2a22" x="10.0" y="20.0" width="30.0" height="30.0" preserveAspectRatio="xMidYMax meet"/>');
  assert.equal(M.svgImage('castle', 0, 0, 1), '');
  // Every icon it maps names a real item and an icon the room allows.
  const go = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_icons.go'), 'utf8');
  const src = readFileSync(join(widgets, 'made_items.js'), 'utf8');
  const map = src.slice(src.indexOf('var FROM_ICON'), src.indexOf('};', src.indexOf('var FROM_ICON')));
  for (const m of map.matchAll(/'?([a-z-]+)'?: '([a-z]+)'/g)) {
    assert.ok(go.includes('"' + m[1] + '"'), m[1] + ' is a shop room icon');
    assert.ok(M.has(m[2]), m[2]);
  }
});

test('every picture the script asks for is in the repository', () => {
  for (const k of M.keys()) {
    for (const z of ['s', 'l', 'shelf']) assert.ok(readFileSync(join(root, 'static', 'img', 'made', z, k + '.webp')).length > 500, z + '/' + k);
    if (M.item(k).moves) assert.ok(readFileSync(join(root, 'static', 'img', 'made', 'move', k + '.webp')).length > 5000, 'move/' + k);
  }
});

test('a shop room puts painted goods on its shelves with full effects only', () => {
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['shop_room_icons.js', 'made_items.js', 'shop_room.js']) vm.runInContext(readFileSync(join(widgets, f), 'utf8'), sandbox);
  const its = ['sword', 'broom', 'crown'].map((ic, i) => ({ id: i + 1, n: ic, k: 'Goods', ic, c: '#8c96a3', mat: i === 2 ? 'gold' : '', p: 1, cur: 'gp', price: '1 gp', qty: null, out: false }));
  const draw = (fx) => {
    const S = { roomType: 'general', pal: 'oak', setting: 'room', fx, size: 'm', full: 'normal', deco: 'some', keep: true,
      seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], ov: {}, portrait: null, lines: [], mode: 'shop', its, key: 42, name: 'Shop', dark: false };
    const room = sandbox.ShopRoom.createRoom(S); room.generate(); return room.draw().svg;
  };
  const full = draw('full');
  assert.match(full, /<image data-mi="sword" data-mi-z="shelf" data-mi-c="#aab3bd,#b98a3a,#3f2a22"/, 'the sword stands in its own colours');
  assert.match(full, /<image data-mi="crown" data-mi-z="shelf" data-mi-c="#8c96a3,/, 'an owner\'s colour paints the body');
  assert.ok(!full.includes('data-mi="broom"'), 'a good with no painted item keeps its icon');
  assert.ok(!draw('light').includes('<image data-mi'), 'lighter effects keep the flat icons');
});
