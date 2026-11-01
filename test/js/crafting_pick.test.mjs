// crafting_pick.test.mjs — typing a recipe outside the Crafting app:
// internal/plugins/crafting/static/js/crafting_pick.js.
//
// Pins the words "How it's made" and the Workshop's Crafting recipes part
// read: "2 moo" is two of whatever "moo" finds, a few letters find an item
// by its start before its middle and a kind as "Any …", and a step is a
// short line that names its material, which joins the recipe when it isn't
// listed yet. Runs the scripts in a vm, on the shared library fixture.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const js = (f) => readFileSync(join(root, 'internal', 'plugins', 'crafting', 'static', 'js', f), 'utf8');
const window = {};
const ctx = vm.createContext({ window });
vm.runInContext(js('crafting_lib.js'), ctx);
vm.runInContext(js('crafting_pick.js'), ctx);
const P = window.CraftingPick;
const plain = (v) => JSON.parse(JSON.stringify(v));

const lib = { ...JSON.parse(readFileSync(join(root, 'test', 'js', 'fixtures', 'craft_book_pages.json'), 'utf8')).library, pictures: [] };
const items = [
  { id: 'moon', name: 'Moonwater', tags: ['liquid'] },
  { id: 'silver', name: 'Silverleaf', tags: ['herb'] },
  { id: 'honey', name: 'Wild honeymoon', tags: [] },
  { id: 'fang', name: 'Ash wolf fang', tags: ['bone'] },
];
const byId = Object.fromEntries(items.map((i) => [i.id, i]));

test('splitQty reads a count before the name', () => {
  const cases = [['2 moo', 2, 'moo'], ['moo', 1, 'moo'], ['3x silver', 3, 'silver'], ['99 moo', 99, 'moo'], ['0 moo', 1, 'moo'], ['  ', 1, '']];
  for (const [text, qty, q] of cases) assert.deepEqual(plain(P.splitQty(text)), { qty, q }, text);
});

test('search finds a start first, then a middle, then a kind', () => {
  const hits = P.search(lib, items, 'moo');
  assert.deepEqual(plain(hits.map((h) => h.name)), ['Moonwater', 'Wild honeymoon']);
  assert.equal(hits[0].itemId, 'moon');
  const kind = P.search(lib, items, 'her');
  assert.ok(kind.some((h) => h.tag === 'herb' && /^Any herb/i.test(h.name)), 'a kind is offered as Any herb');
  assert.equal(P.search(lib, items, '').length, 0);
  assert.ok(P.search(lib, items, 'any her').some((h) => h.tag === 'herb'), '"any" in front still finds the kind');
});

test('a step is a short line', () => {
  const r = { materials: [{ itemId: 'silver', qty: 1 }], steps: [] };
  assert.equal(P.step('grind 3', lib, r, items).step.turns, 3);
  assert.equal(P.step('strike 9', lib, r, items).step.n, 6, 'strikes are capped');
  assert.equal(P.step('simmer long', lib, r, items).step.secs, 10);
  assert.equal(P.step('Tip.', lib, r, items).step.t, 'tip');
  assert.match(P.step('dance', lib, r, items).error, /Start with add/);
});

test('a step naming a listed material points at it', () => {
  const r = { materials: [{ itemId: 'silver', qty: 1 }, { itemId: 'moon', qty: 1 }], steps: [] };
  const got = P.step('pour moonwater', lib, r, items);
  assert.equal(got.step.t, 'pour');
  assert.equal(got.step.material, 1);
  assert.equal(got.addMaterial, undefined);
});

test('a step naming an unlisted material offers it to the recipe', () => {
  const r = { materials: [], steps: [] };
  const got = P.step('add silverleaf to the mortar', lib, r, items);
  assert.equal(got.step.to, 'mortar');
  assert.deepEqual(plain(got.addMaterial), { itemId: 'silver', qty: 1 });
});

test('a step refuses a material its tool does not take', () => {
  const r = { materials: [], steps: [] };
  const got = P.step('pour silverleaf', lib, r, items);
  assert.match(got.error, /isn’t one/);
  assert.match(P.step('pour nothing-like-it', lib, r, items).error, /Nothing called/);
});
