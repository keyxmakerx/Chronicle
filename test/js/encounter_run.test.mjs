// encounter_run.test.mjs: the pure helpers of the DM Screen's Fight tab:
// turn order, stepping through turns and rounds, and the edits to a line.
// The file is loaded in a bare vm context, the way the server ships it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'encounters', 'static', 'js', 'encounter_run.js'), 'utf8');

function load() {
  const sandbox = { window: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.window.EncounterRunHelpers;
}

const plain = (o) => JSON.parse(JSON.stringify(o));

function fight(H) {
  return H.normaliseRun({
    name: 'Ambush in the nave', round: 1, turn: '',
    lines: [
      { id: 'h-1', kind: 'hero', name: 'Kestrel Vane', hp: [null] },
      { id: 'r1', kind: 'creature', name: 'Drowned Acolyte', hp: [null, null, null, null] },
      { id: 'r2', kind: 'creature', name: 'Storm-eel', hp: [null] },
      { id: 'r3', kind: 'creature', name: 'Reinforcements', hp: [null, null], waiting: true }
    ]
  });
}

test('turn order is initiative high to low, unset last, ties as listed, waiting left out', () => {
  const H = load();
  let r = fight(H);
  r = H.setInit(r, 'r2', 9);
  r = H.setInit(r, 'h-1', 18);
  assert.deepEqual(plain(H.order(r).map((l) => l.id)), ['h-1', 'r2', 'r1']);
  r = H.setInit(r, 'r1', 18);
  assert.deepEqual(plain(H.order(r).map((l) => l.id)), ['h-1', 'r1', 'r2'], 'a tie keeps the listed order');
  assert.deepEqual(plain(H.shown(r).map((l) => l.id)), ['h-1', 'r1', 'r2'], 'before the first turn the list stays as listed');
  r = H.setInit(r, 'r2', 30);
  assert.deepEqual(plain(H.shown(r).map((l) => l.id)), ['h-1', 'r1', 'r2'], 'typing initiative never moves a line before the first turn');
  r = H.nextTurn(r);
  assert.deepEqual(plain(H.shown(r).map((l) => l.id)), ['r2', 'h-1', 'r1'], 'from the first turn on the list is in turn order');
});

test('next turn starts round 1, steps through the order and wraps to a new round', () => {
  const H = load();
  let r = H.setInit(H.setInit(fight(H), 'h-1', 18), 'r1', 14);
  r = H.nextTurn(r);
  assert.equal(r.turn, 'h-1');
  assert.equal(r.round, 1);
  r = H.nextTurn(r);
  assert.equal(r.turn, 'r1');
  r = H.nextTurn(r);
  assert.equal(r.turn, 'r2', 'a line without initiative still gets a turn, last');
  r = H.nextTurn(r);
  assert.equal(r.turn, 'h-1');
  assert.equal(r.round, 2);
  r = H.bringIn(r, 'r3');
  assert.deepEqual(plain(H.order(r).map((l) => l.id)), ['h-1', 'r1', 'r2', 'r3'], 'a line brought in joins the order');
  assert.equal(H.nextTurn(H.normaliseRun({ round: 1, lines: [] })).turn, '', 'an empty fight has no turn');
});

test('hit point boxes show full until touched, and a box at zero is down', () => {
  const H = load();
  let r = H.setMax(fight(H), 'r1', 9);
  let l = r.lines[1];
  assert.equal(H.current(l, 0), 9);
  assert.equal(H.isDown(l, 0), false);
  r = H.setHP(r, 'r1', 2, 0);
  l = r.lines[1];
  assert.equal(H.current(l, 2), 0);
  assert.equal(H.isDown(l, 2), true);
  assert.equal(H.current(l, 3), 9, 'only the touched copy changes');
  assert.equal(H.current(r.lines[0], 0), null, 'no full hit points known shows nothing');
  assert.equal(H.setHP(r, 'r1', 9, 3).lines[1].hp.length, 4, 'a box that is not there is ignored');
});

test('typed numbers clear on empty, ignore junk and stay in range', () => {
  const H = load();
  assert.equal(H.parseNum('', 0, 10), null);
  assert.equal(H.parseNum(' 7 ', 0, 10), 7);
  assert.equal(H.parseNum('-3', -99, 999), -3);
  assert.equal(H.parseNum('12', 0, 10), 10);
  assert.equal(H.parseNum('abc', 0, 10), undefined);
  assert.equal(H.parseNum('1.5', 0, 10), undefined);
});

test('conditions are added once whatever their case, up to the limit, and removed by name', () => {
  const H = load();
  let r = H.addCond(fight(H), 'r2', ' Grappled ');
  r = H.addCond(r, 'r2', 'grappled');
  r = H.addCond(r, 'r2', '');
  assert.deepEqual(plain(r.lines[2].conds), ['Grappled']);
  for (let i = 0; i < 20; i++) r = H.addCond(r, 'r2', 'C' + i);
  assert.equal(r.lines[2].conds.length, H.LIM.conds);
  r = H.removeCond(r, 'r2', 'Grappled');
  assert.equal(r.lines[2].conds.indexOf('Grappled'), -1);
  assert.equal(H.addCond(r, 'r1', 'x'.repeat(60)).lines[1].conds[0].length, H.LIM.cond);
});

test('normaliseRun fills what a short answer lacks', () => {
  const H = load();
  assert.equal(H.normaliseRun(null), null);
  const r = H.normaliseRun({ lines: [{ id: 'a', name: 'A' }] });
  assert.equal(r.round, 1);
  assert.equal(r.turn, '');
  assert.deepEqual(plain(r.lines[0].hp), [null]);
  assert.equal(r.lines[0].kind, 'creature');
  assert.deepEqual(plain(r.lines[0].conds), []);
});

test('a creature’s drop chip says what its page leaves, and nothing when no page has a drop list', () => {
  const H = load();
  const leaves = { troll: 'Has a drop list', rat: 'No drop list' };
  const cases = [
    { name: 'a page with a list', line: { kind: 'creature', entityId: 'troll' }, leaves, want: 'Has a drop list' },
    { name: 'a page without one', line: { kind: 'creature', entityId: 'rat' }, leaves, want: 'No drop list' },
    { name: 'a row with no page', line: { kind: 'creature', entityId: '' }, leaves, want: 'No page, so no drops' },
    { name: 'a page the server said nothing about', line: { kind: 'creature', entityId: 'bat' }, leaves, want: '' },
    { name: 'a hero', line: { kind: 'hero', entityId: '' }, leaves, want: '' },
    { name: 'no drop lists anywhere', line: { kind: 'creature', entityId: '' }, leaves: {}, want: '' },
    { name: 'an older server', line: { kind: 'creature', entityId: 'troll' }, leaves: undefined, want: '' },
  ];
  for (const c of cases) assert.equal(H.dropNote(c.leaves, c.line), c.want, c.name);
});
