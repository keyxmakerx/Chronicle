// craft_scene.test.mjs — the crafting bench's rules: static/js/widgets/craft_scene.js.
//
// Pins what the engine promises whoever writes a recipe, a Director or their
// AI: unknown step kinds and missing ingredients are dropped, counts are
// clamped, colours that are not colours fall back, and every id a skin draws
// is made unique to its scene. Also the Draw Steel verdict: clean work earns
// an edge, botched work a bane only when the Director allows banes. Runs the
// script in a vm with a minimal fake window.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets', 'craft_scene.js'), 'utf8');

function load() {
  const window = { matchMedia: () => ({ matches: false }) };
  const document = { documentElement: { hasAttribute: () => false, getAttribute: () => null } };
  const ctx = vm.createContext({ window, document, matchMedia: window.matchMedia });
  vm.runInContext(src, ctx);
  return window.CraftScene;
}
const C = load(), R = C.rules;
const ITEMS = R.cleanItems({ moss: { name: 'Moss', color: '#123456' }, water: { name: 'Water', color: 'blue' } });

test('buildSteps keeps known kinds with known ingredients and clamps their counts', () => {
  const steps = R.buildSteps({ steps: [
    { t: 'add', item: 'moss', to: 'mortar' },
    { t: 'add', item: 'ghost', to: 'mortar' },
    { t: 'grind', turns: 40 },
    { t: 'dance' },
    { t: 'pour', item: 'water' },
    { t: 'simmer', secs: 0.5 },
    { t: 'strike', n: 'lots' },
    null,
  ] }, ITEMS);
  assert.deepEqual(steps.map((s) => s.t), ['add', 'grind', 'pour', 'simmer', 'strike']);
  assert.equal(steps[1].turns, 8);
  assert.equal(steps[3].secs, 2);
  assert.equal(steps[4].n, 3);
  assert.deepEqual(steps.filter((s) => s.graded).map((s) => s.t), ['pour', 'simmer', 'strike']);
});

test('an add step goes to the mortar unless it says flask', () => {
  const steps = R.buildSteps({ steps: [{ t: 'add', item: 'moss', to: 'cauldron' }, { t: 'add', item: 'moss', to: 'flask' }] }, ITEMS);
  assert.deepEqual(steps.map((s) => s.to), ['mortar', 'flask']);
});

test('the verdict follows Draw Steel: clean work earns an edge, botched work a bane only if allowed', () => {
  const g = (...grades) => grades.map((grade) => ({ grade }));
  assert.equal(R.verdict(g('perfect', 'good'), { bane: true }).k, 'edge');
  assert.equal(R.verdict(g('perfect', 'good'), { bane: true }).v, 2);
  assert.equal(R.verdict(g('good', 'good'), { bane: true }).k, 'plain');
  assert.equal(R.verdict(g('miss', 'perfect'), { bane: true }).why, 'One slip');
  assert.equal(R.verdict(g('miss', 'miss'), { bane: true }).v, -2);
  assert.equal(R.verdict(g('miss', 'miss'), { bane: false }).k, 'plain');
  assert.equal(R.verdict([], { bane: true }).why, 'Steps skipped');
});

test('recipes and items from anyone come out as plain values in range', () => {
  const r = R.cleanRecipes({ x: { name: 7, goal: -4, start: 'a', mod: 99, final: 'red', base: '#ABCDEF', steps: 'no' } }).x;
  assert.equal(r.name, '7');
  assert.equal(r.goal, 1);
  assert.equal(r.start, 0);
  assert.equal(r.mod, 10);
  assert.equal(r.final, '#ff4d6d');
  assert.equal(r.base, '#ABCDEF');
  assert.equal(r.steps.length, 0);
  assert.equal(ITEMS.water.color, '#8a8a8a');
  assert.equal(ITEMS.moss.color, '#123456');
});

test('the scoper makes every id and reference unique to its scene', () => {
  const s = R.scoper('crs1-');
  assert.equal(s('<g id="a"><rect fill="url(#a)"/><use href="#a"/></g>'), '<g id="crs1-a"><rect fill="url(#crs1-a)"/><use href="#crs1-a"/></g>');
  assert.equal(s('<g data-id="a">'), '<g data-id="a">', 'only a real id attribute changes');
});

test('a Director picks actions from fixed lists of effects and sounds', () => {
  assert.deepEqual([...C.effects], ['shake', 'sparkle', 'smoke', 'glow']);
  for (const n of ['purr', 'clink', 'thunder', 'dice']) assert.ok(C.sounds.includes(n), n);
});

test('step names read as instructions, with the ingredient by name', () => {
  const [add, grind] = R.buildSteps({ steps: [{ t: 'add', item: 'moss', to: 'mortar' }, { t: 'grind', turns: 2 }] }, ITEMS);
  assert.equal(C.stepName(add, ITEMS), 'Add the moss');
  assert.equal(C.stepName(grind, ITEMS), 'Grind, 2 turns');
});
