// crafting_lib.test.mjs — what a recipe means to the Crafting app:
// internal/plugins/crafting/static/js/crafting_lib.js.
//
// Pins that the Director's "What players see" preview reads exactly as the
// player's book the server builds (one shared fixture, written by
// internal/plugins/crafting/book_test.go), that the rules editor's cleaner
// keeps exactly what the server's does (the other shared fixture), and that
// the editor's checks, templates and saves say and send what they should.
// Runs the scripts in a vm.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const fixtures = join(root, 'test', 'js', 'fixtures');
const plain = (v) => JSON.parse(JSON.stringify(v));
// The server leaves empty strings and nulls out of its JSON; both mean none.
function dropEmpty(v) {
  if (Array.isArray(v)) return v.map(dropEmpty);
  if (v && typeof v === 'object') {
    const out = {};
    for (const [k, e] of Object.entries(v)) if (e !== '' && e !== null && e !== undefined) out[k] = dropEmpty(e);
    return out;
  }
  return v;
}

function loadLib() {
  const window = {};
  vm.runInContext(readFileSync(join(root, 'internal', 'plugins', 'crafting', 'static', 'js', 'crafting_lib.js'), 'utf8'), vm.createContext({ window }));
  return window.CraftingLib;
}
const L = loadLib();
const BOOK = JSON.parse(readFileSync(join(fixtures, 'craft_book_pages.json'), 'utf8'));
const lib = BOOK.library;

test('the preview reads as the player\'s book the server builds', () => {
  assert.ok(BOOK.cases.length >= 3);
  for (const c of BOOK.cases) {
    const items = {};
    for (const [id, name] of Object.entries(c.items)) items[id] = { id, name, tags: [] };
    const got = dropEmpty(plain(L.book(lib, c.recipe, items)));
    assert.deepEqual(got, dropEmpty(c.want), c.name);
  }
});

test('the rules editor keeps what the server keeps', () => {
  const window = {};
  const ctx = vm.createContext({ window, document: { documentElement: { getAttribute: () => null }, createElement: () => ({ getContext: () => null, style: {} }) },
    navigator: {}, requestAnimationFrame: () => 0, cancelAnimationFrame: () => {} });
  for (const f of ['item_look_fx.js', 'craft_rules_logic.js']) vm.runInContext(readFileSync(join(root, 'static', 'js', 'widgets', f), 'utf8'), ctx);
  const R = window.CraftRules, FX = window.ItemLookFX, fx = { spots: FX.MOMENT_SPOTS, looks: FX.LOOKS };
  const { cases } = JSON.parse(readFileSync(join(fixtures, 'craft_rules_clean.json'), 'utf8'));
  assert.ok(cases.length >= 20);
  for (const c of cases) assert.deepEqual(plain(R.cleanRules(c.input, fx).map(R.ruleData)), c.want, c.name);
});

const ITEMS = {
  moss: { id: 'moss', name: 'Bloodmoss', tags: ['herb'] },
  water: { id: 'water', name: 'Spring water', tags: ['liquid'] },
  ingot: { id: 'ingot', name: 'Iron ingot', tags: ['metal'] },
  plain: { id: 'plain', name: 'Plain stone', tags: [] },
};
const recipe = (over) => Object.assign({ id: 'r', name: 'Test', template: '', bench: 'apothecary', materials: [], steps: [], result: { qty: 1 }, goal: 30, stat: '',
  prerequisite: '', source: '', cost: { amount: 0, currency: '' }, who: 'all', known: [], colors: {}, rules: [], version: 3 }, over);

test('each step says what it needs, and names the fix', () => {
  const r = recipe({
    materials: [{ itemId: 'ingot', qty: 1 }, { tag: 'herb', qty: 1 }, { itemId: 'plain', qty: 1 }, { itemId: 'gone', qty: 1 }, { itemId: 'water', qty: 1 }],
    steps: [{ t: 'grind', turns: 3 }, { t: 'add', material: 0, to: 'mortar' }, { t: 'grind', turns: 3 }, { t: 'add', material: 1, to: 'mortar' }, { t: 'grind', turns: 2 },
      { t: 'pour', material: 2 }, { t: 'pour', material: 3 }, { t: 'pour' }, { t: 'pour', material: 4 }, { t: 'hammer' }],
  });
  const say = (i) => plain(L.stepCheck(lib, r, i, ITEMS));
  assert.deepEqual(say(0), { warn: true, msg: 'Nothing is in the mortar yet. Add something to grind first.' });
  assert.equal(say(2).msg, 'Grind takes herbs, bones, body parts or remnant parts. Iron ingot is a metal.');
  assert.equal(say(4).msg, 'Grinds the herb.');
  assert.deepEqual(say(5), { warn: true, msg: 'Plain stone has no crafting tag yet. Give it one under Materials.' });
  assert.deepEqual(say(6), { warn: true, msg: 'That item was deleted. Pick another.' });
  assert.deepEqual(say(7), { warn: true, msg: 'Pick which material this step uses.' });
  assert.equal(say(8).warn, undefined);
  assert.deepEqual(say(9), { warn: true, msg: 'This step isn’t one the bench knows.' });
  const all = plain(L.checks(lib, r, ITEMS)).map((c) => c.msg);
  assert.ok(all.includes('Pick the Armory item it makes.'), all.join(' | '));
});

test('a template takes a fitting material, or adds an "any" slot for one', () => {
  const r = L.applyTemplate(lib, recipe({ materials: [{ itemId: 'moss', qty: 1 }] }), 'steady', ITEMS);
  assert.equal(r.template, 'steady');
  assert.equal(r.bench, 'apothecary');
  assert.equal(r.steps[0].t, 'add');
  assert.equal(r.steps[0].material, 0, 'the herb step takes the bloodmoss');
  const pour = r.steps.find((s) => s.t === 'pour');
  assert.deepEqual(plain(r.materials[pour.material]), { tag: 'liquid', qty: 1 }, 'no liquid yet, so an "any liquid" slot');
  assert.ok(r.steps.every((s) => L.stepCheck(lib, r, r.steps.indexOf(s), ITEMS).warn !== true));
});

test('removing a material unhooks its steps and shifts the rest', () => {
  const r = recipe({ materials: [{ itemId: 'moss', qty: 1 }, { itemId: 'water', qty: 1 }, { tag: 'herb', qty: 1 }],
    steps: [{ t: 'add', material: 0, to: 'mortar' }, { t: 'pour', material: 1 }, { t: 'add', material: 2, to: 'flask' }] });
  L.removeMaterial(r, 1);
  assert.deepEqual(r.steps.map((s) => s.material), [0, undefined, 1]);
});

test('a new step picks a material it accepts and its default setting', () => {
  const r = recipe({ materials: [{ itemId: 'ingot', qty: 1 }, { itemId: 'water', qty: 1 }] });
  assert.deepEqual(plain(L.newStep(lib, r, 'pour', ITEMS)), { t: 'pour', material: 1 });
  assert.deepEqual(plain(L.newStep(lib, r, 'grind', ITEMS)), { t: 'grind', turns: 3 });
  assert.deepEqual(plain(L.newStep(lib, r, 'add', ITEMS)), { t: 'add', material: 0, to: 'mortar' });
});

test('a save sends the fields the server reads, and never the rules', () => {
  const sent = plain(L.input(recipe({ rules: [{ when: [], then: {} }], draft: true })));
  assert.deepEqual(Object.keys(sent).sort(), ['bench', 'colors', 'cost', 'goal', 'known', 'materials', 'name', 'prerequisite', 'result', 'source', 'stat', 'steps', 'template', 'version', 'who']);
});

test('the page escapes every word', () => {
  const p = L.book(lib, recipe({ name: '<img src=x onerror=alert(1)>', materials: [{ itemId: 'evil', qty: 1 }], result: { itemId: 'evil', qty: 1 },
    source: '"><script>', rules: [{ when: [{ kind: 'always' }], then: { kind: 'say', text: '<b>loud</b>' }, players: 'book' }] }),
  { evil: { id: 'evil', name: '<svg onload=alert(1)>', tags: [] } });
  const html = L.bookHTML(lib, p);
  assert.ok(!/<img|<svg|<script|<b>/.test(html), html);
  assert.match(html, /&lt;img src=x/);
});

test('money reads as written', () => {
  assert.equal(L.money({ amount: 25, currency: 'gp' }), '25 gp');
  assert.equal(L.money({ amount: 2.5, currency: '' }), '2.50');
});
