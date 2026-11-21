// roll_tables_page.test.mjs — pins the Rolling tables page's document logic
// and the "keep them with the table" answer: what the builder saves (day
// conditions, who sees a day's roll) is what a calendar day's Roll one obeys.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const require = createRequire(import.meta.url);
const G = require('../../static/js/widgets/chronicle_gen.js');

function load() {
  const window = { Chronicle: {} };
  const document = { currentScript: { getAttribute: () => '/x/chronicle_gen.js' }, querySelector: () => null };
  const ctx = { window, document, matchMedia: () => ({ matches: true }), Chronicle: window.Chronicle };
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/roll_tables.js'), 'utf8'), ctx);
  vm.runInNewContext(readFileSync(join(root, 'static/js/widgets/roll_tables_page.js'), 'utf8'), ctx);
  return { RT: window.Chronicle.RollTables, P: window.Chronicle.RollTablesPage };
}

const draft = (over) => Object.assign({
  name: 'Dockside omens',
  lines: [
    { name: 'Gulls circle the {place} wreck', brief: 'Old sailors wait.', weight: 2 },
    { name: 'A wharf eel in the nets', brief: '', weight: 1 },
    { name: 'Salt crusts the shrine', brief: 'Overnight.', weight: 3 },
    { name: '   ', brief: 'ignored', weight: 2 },
  ],
  when: 'any', see: 'everyone', credit: null,
}, over);

test('examples save the words, odds, conditions and who sees a day\'s roll', () => {
  const { P } = load();
  const t = P.tableFromDraft(draft({ when: 'full', see: 'dm_only' }), 'dockside-omens', 'examples');
  assert.equal(t.visibility, 'dm_only');
  assert.deepEqual([...t.entries.map((e) => e.weight)], [2, 1, 3]);
  for (const e of t.entries) assert.deepEqual(JSON.parse(JSON.stringify(e.when)), { moon: { name: 'any', phase: 'full', within: 1 } });
  // Each line gets its own copy, so editing one never changes the others.
  assert.notEqual(t.entries[0].when, t.entries[1].when);
});

test('blank saves plain lines and nothing about days; any time saves no condition', () => {
  const { P } = load();
  const blank = P.tableFromDraft(draft({ when: 'winter', see: 'dm_only' }), 'x', 'blank');
  assert.equal(blank.visibility, undefined);
  assert.ok(blank.entries.every((e) => e.when === undefined));
  const any = P.tableFromDraft(draft(), 'x', 'examples');
  assert.ok(any.entries.every((e) => e.when === undefined));
  assert.equal(any.visibility, 'everyone');
});

test('a draft says in plain words what is missing', () => {
  const { P } = load();
  const lines = (n) => Array.from({ length: n }, (_, i) => ({ name: 'l' + i, brief: '', weight: 2 }));
  const cases = [
    [draft({ name: ' ' }), 'examples', ['Give the table a name.']],
    [draft({ lines: lines(2) }), 'examples', ['Write three or more examples.']],
    [draft({ lines: lines(3) }), 'examples', []],
    [draft({ lines: lines(1) }), 'blank', []],
    [draft({ lines: [] }), 'blank', ['Write at least one line.']],
    [draft({ name: '', lines: [] }), 'blank', ['Give the table a name.', 'Write at least one line.']],
  ];
  for (const [d, tab, want] of cases) assert.deepEqual([...P.problems(d, tab)], want);
});

test('whenOf reads a saved table back to the choice it came from', () => {
  const { P } = load();
  for (const w of P.WHEN) {
    const t = P.tableFromDraft(draft({ when: w.id }), 'x', 'examples');
    assert.equal(P.whenOf(t.entries), w.id);
  }
  assert.equal(P.whenOf([{ when: { season: ['winter'] } }, { when: { season: ['summer'] } }]), '');
  assert.equal(P.whenOf([{ when: { weekday: ['Moonday'] } }]), '');
});

test('rename, remove and reorder change one table and leave the rest, fields and all', () => {
  const { P } = load();
  const list = ['a', 'b', 'c'].map((id) => ({ id, name: id.toUpperCase(), visibility: 'everyone', entries: [{ name: id, when: { rest: true } }] }));
  const ids = (l) => [...l.map((t) => t.id)];
  assert.deepEqual(ids(P.move(list, 'b', -1)), ['b', 'a', 'c']);
  assert.deepEqual(ids(P.move(list, 'b', 1)), ['a', 'c', 'b']);
  assert.deepEqual(ids(P.move(list, 'a', -1)), ['a', 'b', 'c']);
  assert.deepEqual(ids(P.move(list, 'c', 1)), ['a', 'b', 'c']);
  assert.deepEqual(ids(P.move(list, 'zzz', 1)), ['a', 'b', 'c']);
  const renamed = P.rename(list, 'b', 'Bee');
  assert.equal(renamed[1].name, 'Bee');
  assert.equal(renamed[1].visibility, 'everyone');
  assert.deepEqual(renamed[1].entries, list[1].entries);
  assert.equal(list[1].name, 'B', 'the original list is not changed');
  assert.deepEqual(ids(P.remove(list, 'b')), ['a', 'c']);
});

test('a saved table\'s conditions decide which calendar days Roll one can use', () => {
  const { RT, P } = load();
  const cal = G.calendar.fromSky({ suns: 1, moons: 1, planets: 0, seed: 'x' }).preset.calendar;
  const full = P.tableFromDraft(draft({ when: 'full' }), 'full-omens', 'examples');
  const nw = P.tableFromDraft(draft({ when: 'new' }), 'new-omens', 'examples');
  const list = [full, nw];
  let fullDays = 0, newDays = 0;
  for (let d = 1; d <= cal.months[0].days; d++) {
    const date = { year: cal.current_year, month: 1, day: d };
    const f = RT.roll(G, list, 'full-omens', { calendar: cal, date });
    const n = RT.roll(G, list, 'new-omens', { calendar: cal, date });
    if (!f.problem) fullDays++;
    if (!n.problem) newDays++;
    assert.ok(f.problem || n.problem, 'a day cannot be both near a full and a new moon');
  }
  assert.ok(fullDays > 0 && fullDays < cal.months[0].days, 'full-moon lines come up on some days only: ' + fullDays);
  assert.ok(newDays > 0 && newDays < cal.months[0].days, 'new-moon lines come up on some days only: ' + newDays);
  // A page has no moon, so a moon-only table has nothing to say there.
  assert.equal(RT.roll(G, list, 'full-omens').problem, true);
});

test('who sees a day\'s roll follows the table, and only the campaign\'s own tables decide it', () => {
  const { RT, P } = load();
  const everyone = P.tableFromDraft(draft(), 'seen', 'examples');
  const hidden = P.tableFromDraft(draft({ see: 'dm_only' }), 'hidden', 'examples');
  const plain = { id: 'plain', name: 'Plain', entries: [{ name: 'a', brief: 'b', weight: 1 }] };
  const list = [everyone, hidden, plain];
  assert.equal(RT.roll(G, list, 'seen').visibility, 'everyone');
  assert.equal(RT.roll(G, list, 'hidden').visibility, 'dm_only');
  assert.equal(RT.roll(G, list, 'plain').visibility, undefined);
  // A starter table says "everyone" itself, but a roll on it stays the DM's to show.
  assert.equal(RT.roll(G, list, 'omens').visibility, undefined);
});

test('the engine accepts what the page saves: braces on another table, and the day\'s own words', () => {
  const { RT, P } = load();
  const t = P.tableFromDraft(draft({ lines: [
    { name: 'Gulls circle the {place} wreck', brief: 'A {trade} cart waits.', weight: 2 },
    { name: 'A {person} watches', brief: '', weight: 1 },
    { name: 'Salt on the shrine', brief: '', weight: 3 },
  ] }), 'dockside-omens', 'examples');
  const v = G.tables.validate(RT._internal.setWith(G, [t]));
  assert.ok(v.ok, JSON.stringify(v.errors));
  for (let i = 0; i < 20; i++) assert.doesNotMatch(RT.roll(G, [t], 'dockside-omens').name, /[{}]/);
});

test('editing an own table in the page roller keeps its conditions and who sees it', () => {
  const { RT } = load();
  const own = [{ id: 't', name: 'T', visibility: 'dm_only', entries: [{ name: 'a', brief: '', weight: 1, when: { season: ['winter'] } }] }];
  assert.deepEqual(JSON.parse(JSON.stringify(RT._internal.entriesFor(G, own, 't')[0].when)), { season: ['winter'] });
});
