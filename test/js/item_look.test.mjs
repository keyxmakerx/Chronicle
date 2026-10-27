// item_look.test.mjs — the item look gallery (static/js/widgets/item_look_fx.js)
// must name exactly the looks, colours and sounds the server accepts
// (internal/plugins/armory/item_look_model.go), or a saved look would draw
// nothing or a pick would fail to save. resolve() and kindFor() are pure, so
// their promises are pinned here without a DOM.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');

function load() {
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(join(root, 'static', 'js', 'widgets', 'item_look_fx.js'), 'utf8'), sandbox);
  return sandbox.ItemLookFX;
}
const FX = load();

function goSet(name) {
  const src = readFileSync(join(root, 'internal', 'plugins', 'armory', 'item_look_model.go'), 'utf8');
  const m = src.match(new RegExp(name + '\\s*=\\s*setOf\\(([^)]*)\\)'));
  assert.ok(m, `no ${name} set in item_look_model.go`);
  return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]).sort();
}

test('the gallery matches the looks the server accepts', () => {
  assert.deepEqual(Object.keys(FX.LOOKS).sort(), goSet('itemLooks'));
});
test('the colours match the server palette', () => {
  assert.deepEqual([...FX.TINTS].sort(), goSet('itemTints'));
});
test('the sounds match the server list, and each has a name', () => {
  assert.deepEqual([...FX.SOUNDS].sort(), goSet('itemSounds'));
  for (const s of FX.SOUNDS) assert.ok(FX.SOUND_NAMES[s], `sound ${s} has no name`);
});
test('every look uses a motif and sound the engine can draw and play', () => {
  for (const [k, L] of Object.entries(FX.LOOKS)) {
    assert.ok(L.motif === 'none' || FX.MOTIF_NAMES[L.motif], `${k}: motif ${L.motif}`);
    assert.ok(FX.SOUNDS.includes(L.sound), `${k}: sound ${L.sound}`);
    assert.match(L.color, /^#[0-9a-f]{6}$/);
  }
});

test('resolve applies the colour, sound and words, and leaves the gallery untouched', () => {
  const L = FX.resolve({ look: 'ember', tint: '#8fd8ff', sound: 'bell', words: '6 fire' });
  assert.equal(L.color, '#8fd8ff');
  assert.equal(L.lightColor, '#8fd8ff');
  assert.equal(L.sound, 'bell');
  assert.equal(L.words, '6 fire');
  assert.equal(FX.LOOKS.ember.color, '#ff7a2f');
  assert.equal(FX.LOOKS.ember.sound, 'crackle');
});
test("resolve keeps the look's own colour and sound when none is chosen", () => {
  const L = FX.resolve({ look: 'frost' });
  assert.equal(L.color, FX.LOOKS.frost.color);
  assert.equal(L.sound, 'ping');
  assert.equal(L.words, '');
});
test('resolve ignores values off the lists and names no look for junk', () => {
  const L = FX.resolve({ look: 'ember', tint: 'url(x)', sound: 'scream' });
  assert.equal(L.color, FX.LOOKS.ember.color);
  assert.equal(L.sound, 'crackle');
  assert.equal(FX.resolve({ look: 'fireball' }), null);
  assert.equal(FX.resolve(null), null);
  assert.equal(FX.resolve({ look: '__proto__' }), null);
});
test('kindFor reads the item type', () => {
  const cases = [['Dagger', false, 'blade'], ['Weapon (martial)', false, 'blade'], ['Healing potion', false, 'flask'], ['Elixir', true, 'flask'],
    ['Ring', false, 'ring'], ['amulet', false, 'ring'], ['Reagent', true, 'picture'], ['Reagent', false, 'sigil'], ['', false, 'sigil'], [undefined, true, 'picture']];
  for (const [type, img, want] of cases) assert.equal(FX.kindFor(type, img), want, `${type} → ${want}`);
});

test('spotLayers keeps only known spots and gallery looks', () => {
  const S = FX.spotLayers({ impact: { look: 'ember' }, trail: { look: 'frost', tint: '#a99bff' }, hand: { look: 'ember' }, ground: { look: 'fireball' } });
  assert.deepEqual(Object.keys(S).sort(), ['impact', 'trail']);
  assert.equal(S.impact.motif, 'embers');
  assert.equal(S.impact.sound, 'crackle');
  assert.equal(S.trail.c, '#a99bff');
  assert.equal(FX.spotLayers({ __proto__: { item: { look: 'ember' } } }), null);
  assert.equal(FX.spotLayers({ hand: { look: 'ember' } }), null);
  assert.equal(FX.spotLayers(null), null);
  assert.equal(FX.spotLayers('ember'), null);
});
test('every station shares one bench spot list; moments and motions use parts of it', () => {
  assert.deepEqual([...FX.BENCH_SPOTS], ['window', 'heat', 'work', 'tool', 'trail', 'contact', 'bench', 'result']);
  assert.deepEqual([...FX.MOMENT_SPOTS], ['window', 'heat', 'work', 'bench', 'result']);
  assert.deepEqual([...FX.MOTION_SPOTS], ['tool', 'trail', 'contact', 'work', 'bench']);
  for (const k of [...FX.MOMENT_SPOTS, ...FX.MOTION_SPOTS]) assert.ok(FX.BENCH_SPOTS.includes(k), k);
  const S = FX.spotLayers({ heat: { look: 'ember', move: 'shake' }, impact: { look: 'ember' } }, FX.MOMENT_SPOTS);
  assert.deepEqual(Object.keys(S), ['heat']);
  const M = FX.spotLayers({ contact: { look: 'storm' }, result: { look: 'venom' } }, FX.MOTION_SPOTS);
  assert.deepEqual(Object.keys(M), ['contact']);
});
test('a look with no particles shows its glints on a spot, with its sound', () => {
  const S = FX.spotLayers({ item: { look: 'quiet', sound: 'bell' }, impact: { look: 'ember' } });
  assert.equal(S.item.motif, 'glints');
  assert.equal(S.item.sound, 'bell');
  assert.equal(S.impact.amount, 1);
  assert.equal(FX.spotLayers({ item: { look: 'ember' } }).item.amount, 0.8, 'the item itself plays softer');
});
test('the spots match the ones the server accepts', () => {
  assert.deepEqual([...FX.SPOTS].sort(), goSet('itemSpots'));
});
test('cast shadows fall away from the light, longer when it is brighter', () => {
  const right = FX.castShadow(500, 60, { x: 300, k: 1 });
  const left = FX.castShadow(500, 60, { x: 700, k: 1 });
  assert.ok(right.cx > 500, 'light on the left throws the shadow right');
  assert.ok(left.cx < 500, 'light on the right throws the shadow left');
  const dim = FX.castShadow(500, 60, { x: 300, k: 0.3 });
  assert.ok(dim.len < right.len);
  const far = FX.castShadow(500, 60, { x: -1500, k: 1 });
  assert.ok(far.alpha < right.alpha, 'a farther light casts a fainter shadow');
  assert.equal(FX.castShadow(500, 60, { x: 300, k: 0 }), null);
});

test('an item with no look of its own carries its source\'s, keeping its own words', () => {
  const L = FX.carried({ look: '', words: '6 fire', madeFrom: 'x' }, { look: 'glass', words: 'not these' });
  assert.equal(L.motif, 'shards');
  assert.equal(L.words, '6 fire');
});
test('an item with its own look shows its source on top', () => {
  const L = FX.carried({ look: 'ember' }, { look: 'rot' });
  assert.equal(L.motif, 'embers');
  assert.equal(L.extra.length, 1);
  assert.equal(L.extra[0].motif, 'smoke');
  assert.equal(L.extra[0].sound, 'thrum');
  assert.notEqual(L.lightColor, FX.LOOKS.ember.color, 'some of the source\'s light shows');
  assert.equal(FX.LOOKS.ember.extra, undefined, 'the gallery is untouched');
  assert.equal(FX.carried({ look: 'ember' }, { look: 'quiet' }).extra[0].motif, 'glints');
});
test('carried is null with neither look, and ignores junk', () => {
  assert.equal(FX.carried(null, null), null);
  assert.equal(FX.carried({ look: '', madeFrom: 'x' }, null), null);
  assert.equal(FX.carried({ look: '' }, { look: 'fireball' }), null);
  assert.equal(FX.carried({ look: 'frost' }, { look: 'fireball' }).extra, undefined);
});
