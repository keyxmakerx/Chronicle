// shop_room_mood.test.mjs — shop room moods (static/js/widgets/shop_room.js
// and shop_room_paper.js): every mood the widget offers is one the server
// accepts, brings furniture both looks can draw and decorations it has icons
// for, survives the saved layout with its effects, and swapping moods keeps
// what the owner placed by hand.

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
for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox);
const SR = sandbox.ShopRoom;
const Paper = sandbox.ShopRoomPaper;

// The server's allowlists, read from the Go source so the two cannot drift.
const goIcons = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_icons.go'), 'utf8');
const goValidate = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_room_validate.go'), 'utf8');
const goSet = (src, name) => {
  const m = src.match(new RegExp(name + '\\s*=\\s*(?:setOf\\(|\\[\\]string\\{)([\\s\\S]*?)[)}]\\n'));
  assert.ok(m, name);
  return new Set([...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]));
};

function state(over) {
  return Object.assign({ roomType: 'forge', pal: 'oak', setting: 'room', look: 'lit', fx: 'full', size: 'm', full: 'normal', deco: 'some', keep: true, mood: '', efx: null,
    seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], decor: [], ov: {}, portrait: null, lines: [], mode: 'shop', its: [], key: 3, name: 'Shop', dark: false }, over);
}

test('the widget and the server list the same icons, moods and furniture', () => {
  assert.deepEqual([...goSet(goIcons, 'shopRoomIcons')].sort(), Object.keys(sandbox.ShopRoomIcons).sort());
  assert.deepEqual([...goSet(goValidate, 'shopRoomMoods')].filter(Boolean).sort(), [...SR.MOOD_ORDER].sort());
  const kinds = goSet(goValidate, 'shopRoomKinds');
  for (const k of SR.createRoom(state()).KINDS) assert.ok(kinds.has(k), k);
});

test('every mood brings furniture both looks can draw and decorations with icons', () => {
  const room = SR.createRoom(state());
  for (const m of SR.MOOD_ORDER) {
    const M = SR.MOODS[m];
    for (const k of M.need) {
      assert.ok(room.KINDS.includes(k), `${m}: ${k} is a furniture kind`);
      assert.equal(typeof Paper.KIND[k], 'function', `${m}: ${k} has paper art`);
    }
    for (const d of M.deco || []) assert.ok(sandbox.ShopRoomIcons[d], `${m}: icon ${d}`);
  }
  for (const k of Object.keys(SR.MOOD_KIND)) assert.ok(SR.MOODS[SR.MOOD_KIND[k]], k);
});

test('each mood draws in both looks without broken numbers', () => {
  for (const m of SR.MOOD_ORDER) {
    const S = state({ mood: m });
    const room = SR.createRoom(S);
    room.generate();
    const svg = room.draw().svg;
    assert.doesNotMatch(svg, /NaN|undefined/, `${m}: lit`);
    for (const k of SR.MOODS[m].need) assert.ok(S.pieces.some((p) => p.kind === k), `${m}: brings ${k}`);
    for (const p of S.pieces) {
      const art = Paper.KIND[p.kind];
      if (art) assert.doesNotMatch(art(60, 80, { wood: '#a07a55', acc: '#c00', wall: '#eee' }), /NaN|undefined/, `${m}: paper ${p.kind}`);
    }
  }
});

test('mood and effects survive the saved layout; bad values fall back', () => {
  const S = state({ mood: 'haunted' });
  S.efx = SR.moodEfx('haunted');
  S.efx.haze = 40;
  S.efx.dust = false;
  SR.createRoom(S).generate();
  const L = JSON.parse(JSON.stringify(SR.toLayout(S)));
  assert.equal(L.mood, 'haunted');
  assert.equal(L.effects.haze, 40);
  assert.equal(L.effects.weather, 'rain');
  const T = state();
  SR.fromLayout(T, L);
  assert.equal(T.mood, 'haunted');
  assert.equal(T.efx.haze, 40);
  assert.equal(T.efx.dust, false);

  const U = state();
  SR.fromLayout(U, Object.assign({}, L, { mood: 'volcano', effects: { haze: 'lots', time: 'night' } }));
  assert.equal(U.mood, '', 'unknown mood is dropped');
  assert.equal(U.efx.haze, 100, 'wrongly typed effect keeps the default');
  assert.equal(U.efx.time, 'night');

  const V = state();
  SR.fromLayout(V, Object.assign({}, L, { effects: null }));
  assert.equal(V.efx, null, 'no effects means the mood defaults');
});

test('a mood\'s defaults start from the plain look', () => {
  const plain = SR.moodEfx('cozy');
  assert.equal(plain.haze, 100);
  assert.equal(plain.weather, '');
  assert.equal(SR.moodEfx('dwarven').smoke, true);
  assert.equal(SR.moodEfx('nope').haze, 100);
});

test('switching mood swaps its furniture but keeps what was moved by hand', () => {
  const S = state({ mood: 'eldritch' });
  const room = SR.createRoom(S);
  room.generate();
  const tent = S.pieces.find((p) => p.kind === 'tentacle');
  tent.pinned = true;
  const kept = S.pieces.filter((p) => !SR.MOOD_KIND[p.kind]).length;
  S.mood = 'candy';
  room.applyMood();
  assert.ok(S.pieces.includes(tent), 'hand-placed mood piece stays');
  assert.equal(S.pieces.filter((p) => p.kind === 'monolith').length, 0, 'old mood furniture goes');
  assert.ok(S.pieces.some((p) => p.kind === 'lolly'), 'new mood furniture arrives');
  assert.equal(S.pieces.filter((p) => !SR.MOOD_KIND[p.kind]).length, kept, 'ordinary furniture untouched');
  assert.equal(new Set(S.pieces.map((p) => p.id)).size, S.pieces.length, 'ids stay unique');
});
