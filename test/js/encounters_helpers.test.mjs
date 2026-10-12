// encounters_helpers.test.mjs: the pure helpers of the Encounters block and
// its builder. The widget file is loaded in a bare vm context with a stubbed
// Chronicle, the way the server ships it, and its exported helpers are driven
// directly; the DOM is not needed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'encounters', 'static', 'js', 'encounters.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.EncountersHelpers, registered };
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('the widget registers once under its mount name', () => {
  const { registered } = load();
  assert.deepEqual(Object.keys(registered), ['encounters']);
  assert.equal(typeof registered.encounters.init, 'function');
  assert.equal(typeof registered.encounters.destroy, 'function');
});

test('normalise fills gaps and keeps start states to the three the server knows', () => {
  const { H } = load();
  const e = H.normalise({ id: 'e1', creatures: [{ id: 'r1', name: 'Rat', count: 99, group: 'Z', start: 'revealed' }, { id: 'r2', name: 'Bat', start: 'hidden' }] });
  assert.equal(e.name, '');
  assert.deepEqual(plain(e.heroes), []);
  assert.deepEqual(plain(e.creatures[0]), { id: 'r1', entityId: '', name: 'Rat', count: H.LIM.count, group: 'A', start: 'seen' });
  assert.equal(e.creatures[1].start, 'hidden');
  assert.equal(e.creatures[1].count, 1);
});

test('adding a creature already on the list adds one more, not a row', () => {
  const { H } = load();
  let e = H.normalise({ id: 'e1' });
  e = H.addCreature(e, { entityId: 'gob', name: 'Goblin' });
  e = H.addCreature(e, { entityId: 'gob', name: 'Goblin' });
  e = H.addCreature(e, { entityId: '', name: 'Skeleton' });
  e = H.addCreature(e, { entityId: '', name: 'skeleton' });
  assert.equal(e.creatures.length, 2);
  assert.equal(e.creatures[0].count, 2);
  assert.equal(e.creatures[1].count, 2);
  assert.equal(e.creatures[1].start, 'seen', 'a new row starts seen');
});

test('a new row joins the last row\'s initiative group', () => {
  const { H } = load();
  let e = H.addCreature(H.normalise({}), { entityId: 'a', name: 'A' });
  e = H.setRow(e, e.creatures[0].id, 'group', 'C');
  e = H.addCreature(e, { entityId: 'b', name: 'B' });
  assert.equal(e.creatures[1].group, 'C');
});

test('counts stay between one and the limit, and edits never touch the original', () => {
  const { H } = load();
  const e = H.addCreature(H.normalise({}), { entityId: 'a', name: 'A' });
  const id = e.creatures[0].id;
  assert.equal(H.setCount(e, id, 0).creatures[0].count, 1);
  assert.equal(H.setCount(e, id, 1000).creatures[0].count, H.LIM.count);
  assert.equal(H.setCount(e, id, 5).creatures[0].count, 5);
  assert.equal(e.creatures[0].count, 1);
  assert.equal(H.removeRow(e, id).creatures.length, 0);
  assert.equal(e.creatures.length, 1);
});

test('heroes toggle on and off, up to the limit', () => {
  const { H } = load();
  let e = H.normalise({});
  e = H.toggleHero(e, { id: 'h1', name: 'Ash' }, true);
  e = H.toggleHero(e, { id: 'h1', name: 'Ash' }, true);
  assert.equal(e.heroes.length, 1);
  e = H.toggleHero(e, { id: 'h1', name: 'Ash' }, false);
  assert.equal(e.heroes.length, 0);
  for (let i = 0; i < H.LIM.heroes + 3; i++) e = H.toggleHero(e, { id: 'h' + i, name: 'H' }, true);
  assert.equal(e.heroes.length, H.LIM.heroes);
});

test('the card line says how the fight opens', () => {
  const { H } = load();
  assert.equal(H.summary(H.normalise({})), 'No creatures yet');
  const e = H.normalise({ creatures: [
    { id: 'a', name: 'Chief', count: 1, start: 'hidden' },
    { id: 'b', name: 'Soldier', count: 2, start: 'seen' },
    { id: 'c', name: 'Archer', count: 4, start: 'later' }
  ] });
  assert.equal(H.summary(e), '7 creatures, 1 hidden, 4 arriving later');
  assert.deepEqual(plain(H.countByStart(e)), { seen: 2, hidden: 1, later: 4 });
});

test('the save body is the whole encounter in the server\'s shape', () => {
  const { H } = load();
  const e = H.normalise({ id: 'e1', name: '  ', version: 3, ready: true, heroes: [{ id: 'h1', name: 'Ash' }],
    creatures: [{ id: 'r1', entityId: 'gob', name: 'Goblin', count: 2, group: 'B', start: 'hidden' }] });
  assert.deepEqual(plain(H.toSave(e)), {
    name: 'Untitled encounter', ready: true, heroes: ['h1'], version: 3,
    creatures: [{ id: 'r1', entityId: 'gob', name: 'Goblin', count: 2, group: 'B', start: 'hidden' }]
  });
});

test('pickers take heroes from character categories and creatures from creature ones', () => {
  const { H } = load();
  const types = [{ id: 1, preset_category: 'character' }, { id: 2, preset_category: 'creature' }, { id: 3 }, { id: 4, preset_category: 'creature' }];
  assert.deepEqual(plain(H.pickTypes(types, 'creature')), [2, 4]);
  assert.deepEqual(plain(H.pickTypes(types, 'character')), [1]);
  assert.deepEqual(plain(H.pickTypes(null, 'creature')), []);
});
