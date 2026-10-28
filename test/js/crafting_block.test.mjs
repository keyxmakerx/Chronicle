// crafting_block.test.mjs — the Crafting block and Projects tab's own rules:
// internal/plugins/crafting/static/js/crafting_block.js, crafting_projects.js
// and the bench recipe crafting_lib.js builds for them.
//
// Pins what the block tells a player before the server does: why a recipe
// can't be started (materials, an "any herb" slot after the named materials
// take theirs, a Wealth threshold, coins, no money field), how a held roll
// is said without giving away a secret rule, and how a roll's terms read.
// Runs the scripts in a vm.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const js = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'internal', 'plugins', 'crafting', 'static', 'js');
const window = {};
const ctx = vm.createContext({ window, Chronicle: { register() {} } });
for (const f of ['crafting_lib.js', 'crafting_tray.js', 'crafting_block.js', 'crafting_projects.js']) vm.runInContext(readFileSync(join(js, f), 'utf8'), ctx);
const B = window.CraftingBlock, P = window.CraftingProjects, L = window.CraftingLib;
const plain = (v) => JSON.parse(JSON.stringify(v));

const pack = (items, pay = {}) => ({ items, payKind: '', have: 0, moneyLabel: '', ...pay });
const moss = { itemId: 'moss', name: 'Bloodmoss', qty: 2, tags: ['herb'] };
const leaf = { itemId: 'leaf', name: 'Silverleaf', qty: 1, tags: ['herb'] };
const water = { itemId: 'water', name: 'Spring water', qty: 1, tags: ['liquid'] };
const recipe = (over) => ({ id: 'r', name: 'Salve', goal: 30, makes: 'Salve', needs: [], charge: 0, ...over });

test('a recipe the pack can make has no blockers', () => {
  const r = recipe({ needs: [{ itemId: 'moss', name: 'Bloodmoss', qty: 1 }, { tag: 'herb', name: 'any herb', qty: 1 }] });
  assert.deepEqual(plain(B.blockers(r, pack([moss]), 'Mira')), []);
});

test('blockers name what is missing, first reason first', () => {
  const cases = [
    { name: 'a named item short', r: recipe({ needs: [{ itemId: 'water', name: 'Spring water', qty: 2 }] }), p: pack([water]), want: 'Needs 2 × Spring water. Mira has 1.' },
    { name: 'a named item twice adds up', r: recipe({ needs: [{ itemId: 'moss', name: 'Bloodmoss', qty: 1 }, { itemId: 'moss', name: 'Bloodmoss', qty: 2 }] }), p: pack([moss]), want: 'Needs 3 × Bloodmoss. Mira has 2.' },
    { name: 'an item not carried', r: recipe({ needs: [{ itemId: 'water', name: 'Spring water', qty: 1 }] }), p: pack([]), want: 'Needs 1 × Spring water. Mira has none.' },
    { name: 'a tag slot after the named materials take theirs', r: recipe({ needs: [{ itemId: 'moss', name: 'Bloodmoss', qty: 2 }, { tag: 'herb', name: 'any herb', qty: 1 }] }), p: pack([moss]), want: 'Needs 1 any herb. Mira has none.' },
    { name: 'no goal', r: recipe({ goal: 0 }), p: pack([]), want: 'No goal yet. The Director sets one.' },
    { name: 'makes nothing', r: recipe({ makes: '' }), p: pack([]), want: 'It makes nothing yet. The Director picks the item.' },
    { name: 'Wealth below the threshold', r: recipe({ cost: { amount: 2, currency: '' }, charge: 200 }), p: pack([], { payKind: 'wealth', have: 100 }), want: 'Needs Wealth 2. Mira’s Wealth is 1.' },
    { name: 'too few coins', r: recipe({ cost: { amount: 25, currency: 'gp' }, charge: 2500 }), p: pack([], { payKind: 'coins', have: 1500, moneyLabel: 'Gold' }), want: 'Costs 25 gp. Mira has 15 gp.' },
    { name: 'coins in another unit', r: recipe({ cost: { amount: 25, currency: 'gp' }, charge: 250 }), p: pack([], { payKind: 'coins', have: 100, moneyLabel: 'Platinum' }), want: 'Costs 25 gp. Mira has 1 Platinum.' },
    { name: 'a purse in copper', r: recipe({ cost: { amount: 25, currency: 'gp' }, charge: 2500 }), p: pack([], { payKind: 'purse', have: 1234 }), want: 'Costs 25 gp. Mira has 12.34 gp.' },
    { name: 'no money field', r: recipe({ cost: { amount: 5, currency: 'gp' }, charge: 500 }), p: pack([]), want: 'Costs 5 gp, and Mira’s sheet has no money field.' },
  ];
  for (const c of cases) assert.equal(B.blockers(c.r, c.p, 'Mira')[0], c.want, c.name);
});

test('Wealth at the threshold is enough, and is never shown as spent', () => {
  const r = recipe({ cost: { amount: 2, currency: '' }, charge: 200 });
  assert.deepEqual(plain(B.blockers(r, pack([], { payKind: 'wealth', have: 200 }), 'Mira')), []);
  assert.equal(B.costText(r, pack([], { payKind: 'wealth' })), 'Wealth 2');
  assert.equal(B.costText(recipe({ cost: { amount: 1.5, currency: 'gp' } }), pack([], { payKind: 'coins' })), '1.50 gp');
});

test('an "any herb" slot offers the herbs left after the named materials', () => {
  const r = recipe({ needs: [{ itemId: 'moss', name: 'Bloodmoss', qty: 1 }, { tag: 'herb', name: 'any herb', qty: 1 }] });
  assert.deepEqual(plain(B.tagged(pack([moss, leaf, water]), r, 'herb')), [{ id: 'moss', name: 'Bloodmoss', qty: 1 }, { id: 'leaf', name: 'Silverleaf', qty: 1 }]);
  assert.deepEqual(plain(B.tagged(pack([{ ...moss, qty: 1 }]), r, 'herb')), []);
});

test('a held roll says the rule, or only that a hidden rule holds it', () => {
  assert.equal(B.waitText({ waits: 'Can’t be worked under a full moon.' }, '14 Frostmoot'), 'Can’t be worked under a full moon (the recipe’s rule). Today is 14 Frostmoot, so the roll waits.');
  assert.equal(B.waitText({ waits: 'Only at night.' }, ''), 'Only at night (the recipe’s rule). So the roll waits.');
  assert.equal(B.waitText({ waits: 'A rule the Director keeps hidden.' }, '14 Frostmoot'), 'A rule the Director keeps hidden holds it today, so the roll waits.');
});

test('a roll lists the dice as rolled and every other term signed', () => {
  const rows = B.termRows({ terms: [{ text: '2d10', n: 13 }, { text: 'Reason', n: 2 }, { text: 'A bane: botched work', n: -2 }, { text: 'An edge and a bane cancel', n: 0 }] });
  assert.deepEqual(plain(rows).map((r) => r.n), ['13', '+2', '−2', '0']);
});

test('a known recipe becomes the bench engine’s recipe', () => {
  const r = { name: 'Caustic flask', kind: 'Volatile brew', roll: '2d10 + Reason', statValue: 2, goal: 20, colors: { final: '#a6ff3d' },
    needs: [{ itemId: 'salt-1', name: 'Saltglass', qty: 1, tags: ['magic_item'] }, { tag: 'herb', name: 'any herb', qty: 1 }],
    steps: [{ t: 'add', material: 1, to: 'mortar' }, { t: 'grind', turns: 2 }, { t: 'add', material: 0, to: 'flask' }, { t: 'strike', n: 2 }] };
  const got = plain(L.benchRecipe(r, 'Mira', 9));
  assert.deepEqual(Object.keys(got.items), ['isalt1', 'therb']);
  assert.equal(got.items.therb.name, 'Any herb');
  assert.equal(got.items.therb.color, L.TAG_COLOR.herb);
  assert.equal(got.items.isalt1.color, L.TAG_COLOR.magic_item);
  assert.deepEqual(got.recipe.steps, [{ t: 'add', item: 'therb', to: 'mortar' }, { t: 'grind', turns: 2 }, { t: 'add', item: 'isalt1', to: 'flask' }, { t: 'strike', n: 2 }]);
  assert.equal(got.recipe.stat, 'Reason');
  assert.equal(got.recipe.mod, 2);
  assert.equal(got.recipe.start, 9);
  assert.equal(got.recipe.who, 'Mira');
  assert.equal(got.recipe.final, '#a6ff3d');
  assert.equal(got.recipe.base, L.BENCH_COLORS.base);
});

test('the Projects tab says when the last respite was', () => {
  assert.match(P.respiteLine({ lastRespite: { dayLabel: '12 Frostmoot', at: '2026-01-01T00:00:00Z' } }, 0), /^The last one was on 12 Frostmoot\. /);
  assert.match(P.respiteLine({ lastRespite: null }, 0), /^No respite logged yet\. /);
  assert.equal(P.respiteLine({ today: '14 Frostmoot' }, 2), 'Logged 2 times just now, on 14 Frostmoot. Every project in progress got one roll each time.');
  const now = Date.parse('2026-01-02T00:00:00Z');
  assert.equal(P.ago('2026-01-01T23:59:30Z', now), 'just now');
  assert.equal(P.ago('2026-01-01T23:30:00Z', now), '30 minutes ago');
  assert.equal(P.ago('2026-01-01T21:00:00Z', now), '3 hours ago');
  assert.equal(P.ago('2025-12-31T00:00:00Z', now), '2 days ago');
});
