// shop_room_counter.test.mjs — the shop room's counter look
// (static/js/widgets/shop_room_counter.js): the goods lay themselves out with
// nothing hidden, every icon a good can carry has a drawing, the lamp's
// falloff is a see-through darkening rather than a blend, a keeper picture is
// only ever a same-site or http(s) address, and the shop room saves the look.

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
for (const f of ['shop_room_icons.js', 'shop_room_counter.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
const C = sandbox.ShopRoomCounter;
const SR = sandbox.ShopRoom;
const ICONS = sandbox.ShopRoomIcons;

const good = (id, ic, p, qty) => ({ id, n: 'Good ' + id, k: 'Goods', ic, c: '#8a1a14', p, price: p + ' gp', qty: qty === undefined ? 3 : qty, out: false });
// Arrays from the widget's realm, as plain arrays here, so deepEqual compares values.
const ids = (a) => Array.from(a, (it) => it.id);
const places = (L) => [].concat(L.shelf, L.case, L.cabinet, L.barrel, L.more);

test('every good is placed exactly once, and what does not fit is counted', () => {
  for (const n of [0, 1, 4, 5, 12, 20, 40]) {
    const its = Array.from({ length: n }, (_, i) => good(i, ['flask', 'sword', 'leaf', 'ring', 'box', 'book'][i % 6], (i % 7) + 1, i % 3 ? 4 : null));
    const L = C.layout(its);
    const ids = places(L).map((it) => it.id).sort((a, b) => a - b);
    assert.deepEqual(ids, its.map((it) => it.id), n + ' goods');
    assert.ok(L.shelf.length <= C.CAP.shelf && L.case.length <= C.CAP.case && L.cabinet.length <= C.CAP.cabinet && L.barrel.length <= C.CAP.barrel);
    if (n <= C.CAP.shelf) assert.equal(L.more.length, 0, n + ' goods all have a place');
  }
});

test('the case holds the dearest goods, and only once a shop has five or more', () => {
  const few = C.layout([good(1, 'flask', 1), good(2, 'ring', 100), good(3, 'flask', 1)]);
  assert.equal(few.case.length, 0);
  const its = [good(1, 'flask', 2), good(2, 'flask', 2), good(3, 'jar', 3), good(4, 'vial', 4), good(5, 'ring', 50), good(6, 'gem', 9)];
  const L = C.layout(its);
  assert.deepEqual(ids(L.case).sort(), [5, 6]);
});

test('barrels take bulk bundles and jars; the cabinet takes the bigger goods', () => {
  // Four goods, so no case: the dear sword goes to the cabinet.
  const L = C.layout([good(1, 'leaf', 1, null), good(2, 'jar', 1, 40), good(3, 'wheat-awn', 1, 12), good(4, 'sword', 5)]);
  assert.deepEqual(ids(L.barrel), [1, 2], 'two barrels, first come');
  assert.deepEqual(ids(L.cabinet), [4]);
  assert.deepEqual(ids(L.shelf), [3], 'a third bulk good waits on the shelf');
  const small = C.layout([good(1, 'leaf', 1, 2), good(2, 'flask', 2)]);
  assert.deepEqual(ids(small.shelf), [1, 2], 'a bundle with little stock stays on the shelf');
});

test('every icon a good can carry has a family with a drawing', () => {
  for (const ic of Object.keys(ICONS)) {
    const f = C.family(ic);
    assert.ok(f, ic);
    const svg = C.ware(good(1, ic, 1), 'R', ICONS);
    assert.ok(svg.length > 80 && !/undefined|NaN/.test(svg), ic + ' draws as ' + f);
  }
  assert.equal(C.family('not-an-icon'), 'crate');
});

test('the falloff darkens by how much a multiply would take away, with no blend', () => {
  const s = C.shade([[0, '#ffffff'], [.5, '#808080'], [1, '#000000']]);
  assert.deepEqual(Array.from(s, (x) => x[2]), [0, +(1 - 384 / 765).toFixed(3), 1]);
  assert.ok(s.every((x) => x[1] === '#0b0e20'));
  const src = readFileSync(join(dir, 'shop_room_counter.js'), 'utf8');
  assert.ok(!/mix-blend-mode:\s*multiply|fePointLight|feSpotLight/.test(src), 'no multiply blend and no point or spot lights');
});

test('the keeper picture is a same-site or http(s) address, never a script or data URL', () => {
  assert.equal(C.safeUrl('/media/a.png'), '/media/a.png');
  assert.equal(C.safeUrl('https://x.test/a.png'), 'https://x.test/a.png');
  for (const bad of ['javascript:alert(1)', 'data:image/svg+xml,<svg/>', '//evil.test/a.png', ' javascript:x', '']) assert.equal(C.safeUrl(bad), '', bad);
});

test('the case monogram takes up to two initials, skipping little words', () => {
  assert.equal(C.initials("Hessa Ord's Workshop"), 'H·O');
  assert.equal(C.initials('The Gilded Anvil'), 'G·A');
  assert.equal(C.initials('Mirela'), 'M');
});

test('the room type picks the shop floor\'s working piece', () => {
  assert.equal(C.floorPiece('apothecary'), 'still');
  assert.equal(C.floorPiece('forge'), 'anvil');
  assert.equal(C.floorPiece('general'), 'crates');
});

test('the shop room saves and loads the counter look', () => {
  const S = { roomType: 'apothecary', pal: 'oak', mood: '', efx: null, setting: 'room', look: 'counter', size: 'm', full: 'normal', deco: 'some', seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [] };
  const L = SR.toLayout(S);
  assert.equal(L.look, 'counter');
  const S2 = Object.assign({}, S, { look: 'lit' });
  SR.fromLayout(S2, L);
  assert.equal(S2.look, 'counter');
  SR.fromLayout(S2, Object.assign({}, L, { look: 'neon' }));
  assert.equal(S2.look, 'lit', 'an unknown look reads as lit');
});
