// shop_room_keeper.test.mjs — the keeper's manner (static/js/widgets/
// shop_room_keeper.js): the manners and moments the widget offers are the ones
// the server accepts, every manner has lines for every moment, the GM's own
// lines win over the manner's, and the saved layout keeps manner and moments.

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
for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_keeper.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
const KP = sandbox.ShopRoomKeeper;
const SR = sandbox.ShopRoom;

const goValidate = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_validate.go'), 'utf8');
const goSet = (name) => {
  const m = goValidate.match(new RegExp(name + '\\s*=\\s*setOf\\(([^)]*)\\)'));
  assert.ok(m, name);
  return new Set([...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]));
};
const goConst = (name) => +goValidate.match(new RegExp(name + '\\s*=\\s*(\\d+)'))[1];

test('the widget and the server list the same manners and moments', () => {
  assert.deepEqual([...goSet('shopRoomManners')].filter(Boolean).sort(), [...KP.ORDER].sort());
  assert.deepEqual([...Object.keys(KP.MANNERS)].sort(), [...KP.ORDER].sort());
  assert.deepEqual([...goSet('shopRoomMoments')].sort(), [...KP.MOMENTS.map((m) => m[0])].sort());
});

test('every manner has starting lines for every moment, within the server limits', () => {
  const most = goConst('maxShopRoomMomentLines'), long = goConst('maxShopRoomLine');
  for (const k of KP.ORDER) {
    for (const [moment] of KP.MOMENTS) {
      const ls = KP.MANNERS[k].lines[moment];
      assert.ok(Array.isArray(ls) && ls.length > 0 && ls.length <= most, k + ' ' + moment);
      for (const l of ls) assert.ok(l.length > 0 && [...l].length <= long, k + ' ' + moment + ': ' + l);
    }
  }
});

test("the GM's lines win; an empty moment falls back to the manner; no manner says nothing", () => {
  const cases = [
    ['gruff', { hello: ['Mine'] }, 'hello', ['Mine']],
    ['gruff', { hello: [] }, 'hello', KP.MANNERS.gruff.lines.hello],
    ['gruff', null, 'bye', KP.MANNERS.gruff.lines.bye],
    ['', { look: ['Only mine'] }, 'look', ['Only mine']],
    ['', {}, 'buy', []],
    ['sly', {}, 'buy', []]
  ];
  for (const [manner, moments, moment, want] of cases) assert.deepEqual([...KP.linesFor(manner, moments, moment)], [...want], manner + ' ' + moment);
  assert.equal(KP.pick('', {}, 'buy'), '');
});

test('a pick never repeats the last line when there is a choice', () => {
  const moments = { look: ['a', 'b'] };
  const zero = () => 0;
  const first = KP.pick('', moments, 'look', zero);
  const second = KP.pick('', moments, 'look', zero);
  assert.notEqual(first, second);
  assert.equal(KP.pick('', { bye: ['only'] }, 'bye', zero), 'only');
  assert.equal(KP.pick('', { bye: ['only'] }, 'bye', zero), 'only');
});

test('the saved layout keeps the manner and the moments that have lines', () => {
  const S = { roomType: 'forge', pal: 'oak', setting: 'room', look: 'lit', size: 'm', full: 'normal', deco: 'some', mood: '', efx: null,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], manner: 'grand', moments: { hello: ['Enter!'], bye: [] } };
  const L = SR.toLayout(S);
  assert.equal(L.manner, 'grand');
  assert.deepEqual(JSON.parse(JSON.stringify(L.moments)), { hello: ['Enter!'] });
  const T = { lines: [], moments: {} };
  SR.fromLayout(T, L);
  assert.equal(T.manner, 'grand');
  assert.deepEqual(JSON.parse(JSON.stringify(T.moments)), { hello: ['Enter!'] });
  // An older layout, or a manner the widget doesn't know, reads as none.
  const U = { lines: [] };
  SR.fromLayout(U, { version: 1, manner: 'sly' });
  assert.equal(U.manner, '');
  assert.deepEqual(JSON.parse(JSON.stringify(U.moments)), {});
});
