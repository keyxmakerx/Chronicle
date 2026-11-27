// dm_layout.test.mjs: the docked screen's layout rules (static/js/widgets/dm_layout.js),
// which the server checks again in internal/plugins/dmscreen/prefs.go.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const L = require('../../static/js/widgets/dm_layout.js');

test('clean keeps one shape: a card in one place, hidden cards nowhere, L left out', () => {
  const cases = [
    { name: 'leaf wins over the column', in: { open: ['world', 'party'], leaves: [['party']] }, open: ['world'], leaves: [['party']] },
    { name: 'hidden is neither open nor on a leaf', in: { open: ['world', 'rules'], leaves: [['notes']], hidden: ['rules', 'notes'] }, open: ['world'], leaves: [] },
    { name: 'bad ids and repeats dropped', in: { open: ['world', 'World', '<b>', 'world', 7] }, open: ['world'], leaves: [] },
    { name: 'three leaves become two', in: { open: [], leaves: [['a'], ['b'], ['c']] }, open: [], leaves: [['a'], ['b']] },
  ];
  for (const c of cases) {
    const got = L.clean(c.in);
    assert.deepEqual(got.open, c.open, c.name);
    assert.deepEqual(got.leaves, c.leaves, c.name);
  }
  assert.deepEqual(L.clean({ sizes: { world: 'L', party: 'S', notes: 'XL' } }).sizes, { party: 'S' });
  assert.equal(L.clean({ cols: 3 }).cols, 1);
  assert.deepEqual(L.clean(null, ['world', 'party']).open, ['world', 'party']);
});

test('equal compares what the screen would draw', () => {
  assert.ok(L.equal({ open: ['a', 'b'], folded: ['b', 'a'] }, { open: ['a', 'b'], folded: ['a', 'b'], sizes: { a: 'L' } }));
  assert.ok(!L.equal({ open: ['a', 'b'] }, { open: ['b', 'a'] }));
  assert.ok(!L.equal({ open: ['a'] }, { open: ['a'], cols: 2 }));
});

test('names: 1 to 40 characters, never Default, never twice', () => {
  const layouts = [{ name: 'Fight night' }];
  const cases = [
    ['', 'Give the layout a name.'],
    ['   ', 'Give the layout a name.'],
    ['x'.repeat(41), 'Keep the name to 40 characters.'],
    ['default', 'Default is always there. Pick another name.'],
    ['FIGHT NIGHT', 'A layout already has that name.'],
    ['Tab\there', 'Use letters, numbers and spaces.'],
    ['x'.repeat(40), null],
    ['Downtime', null],
  ];
  for (const [name, want] of cases) assert.equal(L.nameError(name, layouts), want, JSON.stringify(name));
  assert.equal(L.nameError('fight night', layouts, 'Fight night'), null, 'renaming to itself in another case');
});

test('Default follows the screen; a saved layout holds changes as live', () => {
  const p = L.cleanPrefs({});
  assert.deepEqual(L.shown(p, ['world', 'party']).open, ['world', 'party']);
  assert.deepEqual(L.record(p, { open: ['world'] }), ['home']);
  assert.deepEqual(L.record(p, { open: ['world'] }), [], 'no change, nothing to save');
  assert.equal(L.saveAs(p, 'Fight night', { open: ['fight'], cols: 2 }), null);
  assert.equal(p.current, 'Fight night');
  assert.deepEqual(L.shown(p).open, ['fight']);
  assert.deepEqual(L.record(p, { open: ['fight', 'rules'], cols: 2 }), ['live']);
  assert.ok(L.edited(p));
  assert.deepEqual(L.shown(p).open, ['fight', 'rules']);
  assert.deepEqual(L.record(p, { open: ['fight'], cols: 2 }), ['live'], 'back to as saved clears live');
  assert.equal(p.live, null);
  // Switching back to Default shows the everyday screen as it was left.
  assert.deepEqual(L.pick(p, '', ['world', 'party']).open, ['world']);
  assert.equal(p.current, '');
});

test('save, update, rename and delete', () => {
  const p = L.cleanPrefs({});
  for (let i = 1; i <= 8; i++) assert.equal(L.saveAs(p, 'L' + i, { open: ['a'] }), null);
  assert.equal(L.saveAs(p, 'L9', { open: ['a'] }), 'Eight layouts at most. Delete one to save another.');
  assert.equal(L.rename(p, 'L8', 'Fight night'), null);
  assert.equal(p.current, 'Fight night', 'renaming the current layout keeps it current');
  assert.equal(L.rename(p, 'L1', 'fight night'), 'A layout already has that name.');
  assert.equal(L.update(p, { open: ['b'] }), null);
  assert.deepEqual(L.find(p, 'fight night').open, ['b']);
  assert.equal(L.remove(p, 'Fight night'), null);
  assert.equal(p.current, '', 'deleting the current layout leaves Default current');
  assert.equal(p.layouts.length, 7);
  assert.equal(L.update(p, { open: ['c'] }), 'Pick a saved layout first.');
});

test('cleanPrefs drops what the screen never writes', () => {
  const p = L.cleanPrefs({
    current: 'Gone', live: { open: ['x'] }, owner: 'u2',
    layouts: [{ name: 'Default', open: [] }, { name: 'Fight', open: ['fight'] }, { name: 'fight', open: [] }],
    style: { surface: 'neon', density: 'compact', trayLevel: 'ultra', trayFinish: 'kit', trayShape: 'cup', trayOthers: 'off' }
  });
  assert.deepEqual(p.layouts.map((l) => l.name), ['Fight']);
  assert.equal(p.current, '');
  assert.equal(p.live, null);
  assert.equal(p.owner, undefined);
  // The Dice tray's choices keep an allowed value, and an unknown one is the starting value.
  assert.deepEqual(p.style, { surface: 'solid', density: 'compact', trayLevel: 'detailed', trayFinish: 'kit', trayShape: 'tower', trayOthers: 'off' });
});

test('body carries only the named keys, defaults as empty', () => {
  const p = L.cleanPrefs({ style: { surface: 'solid', density: 'compact', trayLevel: 'flat', trayShape: 'bowl' } });
  assert.deepEqual(L.body(p, ['style']), { style: { surface: '', density: 'compact', trayLevel: 'flat', trayFinish: '', trayShape: 'bowl', trayOthers: '' } });
  assert.deepEqual(L.body(p, ['current', 'live']), { current: '', live: null });
});

test('move: up and down in the shown order, out to a leaf, back in', () => {
  // The column shows the newest first: party, notes, world.
  const s = { open: ['world', 'notes', 'party'], leaves: [] };
  let m = L.move(s, 'party', 'down', 2);
  assert.deepEqual(m.open, ['world', 'party', 'notes']);
  assert.deepEqual(m.where, { box: 'col', pos: 2, of: 3 });
  assert.equal(L.move(s, 'party', 'up', 2), null, 'already first');
  assert.equal(L.move(s, 'world', 'down', 2), null, 'already last');
  m = L.move(s, 'notes', 'out', 2);
  assert.deepEqual(m.leaves, [['notes']]);
  assert.deepEqual(m.open, ['world', 'party']);
  assert.deepEqual(m.where, { box: 0, pos: 1, of: 1 });
  assert.equal(L.move(s, 'notes', 'out', 0), null, 'no room for a leaf');
  assert.equal(L.move({ open: [], leaves: [['a'], ['b']] }, 'b', 'out', 2), null, 'two leaves at most');
  assert.equal(L.move({ open: ['x'], leaves: [['a']] }, 'a', 'out', 1), null, 'alone on the outermost leaf');
  m = L.move({ open: ['world'], leaves: [['notes'], ['rules']] }, 'rules', 'in', 2);
  assert.deepEqual(m.leaves, [['notes', 'rules']]);
  m = L.move({ open: ['world'], leaves: [['notes']] }, 'notes', 'in', 2);
  assert.deepEqual(m.open, ['world', 'notes'], 'into the column, on top');
  assert.deepEqual(m.leaves, []);
  assert.deepEqual(m.where, { box: 'col', pos: 1, of: 2 });
  assert.equal(L.move(s, 'world', 'in', 2), null, 'already in the column');
  assert.equal(L.move(s, 'gone', 'up', 2), null);
});
