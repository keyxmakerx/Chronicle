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
    name: 'Untitled encounter', ready: true, heroes: ['h1'], version: 3, stages: [], versions: [],
    creatures: [{ id: 'r1', entityId: 'gob', name: 'Goblin', count: 2, group: 'B', start: 'hidden' }]
  });
});

test('pickers list the categories the server names, and nothing else', () => {
  const { H } = load();
  assert.deepEqual(plain(H.pickTypes([1, 13, 5])), [1, 13, 5]);
  assert.deepEqual(plain(H.pickTypes([1, '2', 0, -3, 1.5, null])), [1]);
  assert.deepEqual(plain(H.pickTypes(null)), []);
  assert.deepEqual(plain(H.pickTypes({ id: 1 })), []);
});

test('creature search waits for the shortest query the server answers', () => {
  const { H } = load();
  assert.equal(H.MIN_SEARCH, 2);
});

// ---- Stages ----

const queen = () => H0().normalise({
  id: 'e1', version: 2,
  creatures: [
    { id: 'r1', entityId: 'queen', name: 'Hollow Queen', count: 1, start: 'seen' },
    { id: 'r2', entityId: '', name: 'Rat swarm', count: 3, start: 'later' }
  ]
});
function H0() { return load().H; }

test('an encounter with no stages reads and saves as none', () => {
  const H = H0();
  const e = H.normalise({ id: 'e1', name: 'Old' });
  assert.deepEqual(plain(e.stages), []);
  assert.deepEqual(plain(H.toSave(e).stages), []);
});

test('normalise keeps a stage within the server\'s ranges', () => {
  const H = H0();
  const e = H.normalise({ stages: [{ id: 's1', when: { kind: 'weekly', percent: 500, round: 1 }, how: 'loud', forms: [{ row: 'r1' }], arrive: ['r2', 7] }] });
  const s = e.stages[0];
  assert.equal(s.when.kind, 'manual');
  assert.equal(s.when.percent, 99);
  assert.equal(s.when.round, 2);
  assert.equal(s.how, 'reveal');
  assert.deepEqual(plain(s.arrive), ['r2']);
  assert.deepEqual(plain(s.forms[0]), { row: 'r1', form: '', formName: '' });
});

test('the Hollow Queen stage builds and saves in the server\'s shape', () => {
  const H = H0();
  let e = H.addStage(queen());
  const id = e.stages[0].id;
  assert.equal(H.stageReady(e, e.stages[0]), false, 'a stage that does nothing is not saved');
  assert.deepEqual(plain(H.toSave(e).stages), []);
  e = H.setWhen(e, id, { kind: 'stamina' });
  assert.equal(e.stages[0].when.row, 'r1', 'Stamina picks the first row');
  e = H.setWhen(e, id, { percent: 50 });
  e = H.addForm(e, id);
  e = H.setForm(e, id, 0, { form: 'unmasked', formName: 'Unmasked' });
  e = H.toggleArrive(e, id, 'r2', true);
  e = H.setStageField(e, id, 'note', '  Dim the lights ');
  assert.deepEqual(plain(H.toSave(e).stages), [{
    id, when: { kind: 'stamina', row: 'r1', percent: 50, round: 0 },
    forms: [{ row: 'r1', form: 'unmasked', formName: 'Unmasked' }], arrive: ['r2'], how: 'reveal', note: 'Dim the lights'
  }]);
});

test('only the trigger\'s own fields are sent', () => {
  const H = H0();
  let e = H.addStage(queen());
  const id = e.stages[0].id;
  e = H.toggleArrive(e, id, 'r2', true);
  e = H.setWhen(e, id, { kind: 'round', round: 3 });
  assert.deepEqual(plain(H.toSave(e).stages[0].when), { kind: 'round', row: '', percent: 0, round: 3 });
  e = H.setWhen(e, id, { kind: 'manual' });
  assert.deepEqual(plain(H.toSave(e).stages[0].when), { kind: 'manual', row: '', percent: 0, round: 0 });
});

test('form changes need a row with a page and follow the chosen option', () => {
  const H = H0();
  let e = H.addStage(queen());
  const id = e.stages[0].id;
  e = H.addForm(e, id);
  assert.deepEqual(plain(e.stages[0].forms[0]), { row: 'r1', form: '', formName: H.FIRST_FORM });
  e = H.setForm(e, id, 0, { form: 'unmasked', formName: 'Unmasked' });
  e = H.setForm(e, id, 0, { form: '' });
  assert.equal(e.stages[0].forms[0].formName, H.FIRST_FORM, 'the first form has its own name');
  e = H.setForm(e, id, 0, { form: 'unmasked', formName: 'Unmasked' });
  e = H.setForm(e, id, 0, { row: 'r1' });
  assert.equal(e.stages[0].forms[0].form, 'unmasked', 'the same creature keeps its form');
  e = H.removeForm(e, id, 0);
  assert.equal(e.stages[0].forms.length, 0);
  const noPage = H.normalise({ creatures: [{ id: 'r1', name: 'Rat' }] });
  const s = H.addStage(noPage);
  assert.equal(H.addForm(s, s.stages[0].id).stages[0].forms.length, 0);
});

test('rows that start later are listed first for arrivals', () => {
  const H = H0();
  assert.deepEqual(plain(H.arriveOrder(queen()).map((c) => c.id)), ['r2', 'r1']);
});

test('stages are capped, and removing a row takes it out of every stage', () => {
  const H = H0();
  let e = queen();
  for (let i = 0; i < H.LIM.stages + 2; i++) e = H.addStage(e);
  assert.equal(e.stages.length, H.LIM.stages);
  const id = e.stages[0].id;
  e = H.setWhen(e, id, { kind: 'stamina', row: 'r1' });
  e = H.addForm(e, id);
  e = H.toggleArrive(e, id, 'r2', true);
  e = H.removeRow(e, 'r1');
  assert.equal(e.stages[0].forms.length, 0);
  assert.equal(e.stages[0].when.row, 'r2', 'the watched row falls to a row that still exists');
  e = H.removeRow(e, 'r2');
  assert.deepEqual(plain(e.stages[0].arrive), []);
  assert.equal(e.stages[0].when.row, '');
  assert.equal(H.stageReady(e, e.stages[0]), false);
});

const base = () => ({
  id: 'e1', name: 'Stalkers', version: 3,
  creatures: [{ id: 'r1', name: 'Stalker', count: 4 }, { id: 'r2', name: 'Hound', count: 2 }]
});

test('normalise reads versions and drops a half pick', () => {
  const { H } = load();
  const e = H.normalise({ ...base(), versions: [
    { id: 'v1', name: 'Won', pick: { test: 't1', result: 'win' }, remove: ['r2', 7], counts: [{ row: 'r1', count: 99 }, {}] },
    { id: 'v2', pick: { test: 't1' } }
  ] });
  assert.equal(e.versions.length, 2);
  assert.deepEqual(plain(e.versions[0].remove), ['r2']);
  assert.deepEqual(plain(e.versions[0].counts), [{ row: 'r1', count: H.LIM.count }]);
  assert.equal(e.versions[1].pick, null);
  assert.deepEqual(plain(e.versions[1].add), []);
});

test('a version keeps, leaves out and recounts base rows', () => {
  const { H } = load();
  let e = H.addVersion(H.normalise(base()));
  const id = e.versions[0].id;
  assert.equal(e.versions[0].name, 'Version 1');
  e = H.setVersionCount(e, id, 'r1', 6);
  assert.equal(H.versionCount(e, e.versions[0], 'r1'), 6);
  e = H.setVersionCount(e, id, 'r1', 4);
  assert.deepEqual(plain(e.versions[0].counts), [], 'the base count is stored as no change');
  e = H.setVersionCount(e, id, 'r2', 3);
  e = H.keepRow(e, id, 'r2', false);
  assert.equal(H.versionCount(e, e.versions[0], 'r2'), 0);
  assert.deepEqual(plain(e.versions[0].counts), [], 'a left-out row loses its count');
  e = H.keepRow(e, id, 'r2', true);
  assert.equal(H.versionCount(e, e.versions[0], 'r2'), 2);
  assert.equal(H.versionTotal(e, e.versions[0]), 6);
});

test('a version adds its own rows and removing it cleans the stages', () => {
  const { H } = load();
  let e = H.addVersion(H.normalise(base()));
  const id = e.versions[0].id;
  e = H.addVersionRow(e, id, { entityId: 'p9', name: 'Matriarch' });
  e = H.addVersionRow(e, id, { entityId: 'p9', name: 'Matriarch' });
  assert.equal(e.versions[0].add.length, 1);
  assert.equal(e.versions[0].add[0].count, 2);
  const row = e.versions[0].add[0].id;
  e = H.setVersionRow(e, id, row, 'count', 99);
  assert.equal(e.versions[0].add[0].count, H.LIM.count);
  e = H.addStage(e);
  e.stages[0].arrive = [row, 'r1'];
  const gone = H.removeVersion(e, id);
  assert.deepEqual(plain(gone.versions), []);
  assert.deepEqual(plain(gone.stages[0].arrive), ['r1']);
  const rowGone = H.removeVersionRow(e, id, row);
  assert.deepEqual(plain(rowGone.stages[0].arrive), ['r1']);
});

test('one result chooses one version, and removing a base row cleans versions', () => {
  const { H } = load();
  let e = H.addVersion(H.addVersion(H.normalise(base())));
  const [a, b] = e.versions.map((v) => v.id);
  e = H.setPick(e, a, 't1', 'win');
  e = H.setPick(e, b, 't1', 'win');
  assert.equal(e.versions[0].pick, null);
  assert.equal(e.versions[1].pick.result, 'win');
  e = H.setPick(e, b, '', '');
  assert.equal(e.versions[1].pick, null);
  e = H.keepRow(H.setVersionCount(e, a, 'r1', 1), b, 'r1', false);
  e = H.removeRow(e, 'r1');
  assert.deepEqual(plain(e.versions[0].counts), []);
  assert.deepEqual(plain(e.versions[1].remove), []);
});

test('toSave sends versions without the read-only names', () => {
  const { H } = load();
  const e = H.normalise({ ...base(), versions: [
    { id: 'v1', name: '  ', pick: { test: 't1', result: 'win', testName: 'Hunt', resultLabel: 'Won' }, add: [{ id: 'a1', name: 'Matriarch' }], counts: [{ row: 'r1', count: 2 }] }
  ] });
  assert.deepEqual(plain(H.toSave(e).versions), [{
    id: 'v1', name: 'Untitled version', pick: { test: 't1', result: 'win' },
    add: [{ id: 'a1', entityId: '', name: 'Matriarch', count: 1, group: 'A', start: 'seen' }],
    remove: [], counts: [{ row: 'r1', count: 2 }]
  }]);
});

test('pickedLine says which version the recorded result chooses', () => {
  const { H } = load();
  const e = (picked) => H.normalise({ ...base(), versions: [{ id: 'v1', name: 'Won' }], picked });
  assert.equal(H.pickedLine(e({ version: 'v1', test: 't1', result: 'win', open: [] })), 'The recorded result chooses Won.');
  assert.match(H.pickedLine(e({ version: '', open: ['t1'] })), /No result recorded yet/);
  assert.match(H.pickedLine(e({ version: '', open: [] })), /base list plays/);
  assert.equal(H.pickedLine(H.normalise(base())), '');
});
