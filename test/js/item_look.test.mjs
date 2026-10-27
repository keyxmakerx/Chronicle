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
