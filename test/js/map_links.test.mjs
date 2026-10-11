// map_links.test.mjs — pins the pure parts of pins that open another map: how
// the trail folds, who the words say can follow a link, the follow address,
// the path to a map in the tree, and what each key does in the tree.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_links.js'), 'utf8');

function load() {
  const saved = { window: globalThis.window, module: globalThis.module };
  delete globalThis.window;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  globalThis.module = saved.module;
  if (saved.window !== undefined) globalThis.window = saved.window;
  return api;
}
const M = load();

const steps = (n) => Array.from({ length: n }, (_, i) => ({ id: 'm' + i, name: 'Map ' + i }));
const ids = (list) => list.map((s) => (s.gap ? '…' : s.id)).join(' ');

test('foldTrail keeps short trails whole and folds the middle past three, past two when narrow', () => {
  const cases = [
    { n: 0, narrow: false, want: '' },
    { n: 1, narrow: false, want: 'm0' },
    { n: 3, narrow: false, want: 'm0 m1 m2' },
    { n: 4, narrow: false, want: 'm0 … m2 m3' },
    { n: 7, narrow: false, want: 'm0 … m5 m6' },
    { n: 2, narrow: true, want: 'm0 m1' },
    { n: 3, narrow: true, want: 'm0 … m2' },
    { n: 6, narrow: true, want: 'm0 … m5' },
  ];
  for (const c of cases) {
    assert.equal(ids(M.foldTrail(steps(c.n), c.narrow)), c.want, `${c.n} steps, narrow=${c.narrow}`);
  }
  assert.deepEqual(M.foldTrail(null, false), []);
});

test('foldTrail does not change the trail it is given', () => {
  const s = steps(5);
  M.foldTrail(s, false);
  assert.equal(s.length, 5);
});

test('audience reads the pin visibility and rules the server filters on', () => {
  const cases = [
    { vis: 'everyone', rules: null, kind: 'everyone' },
    { vis: '', rules: undefined, kind: 'everyone' },
    { vis: 'dm_only', rules: '{"allowed_users":["u1"]}', kind: 'dm' },
    { vis: 'everyone', rules: '{"allowed_users":["u1","u2"]}', kind: 'some', users: ['u1', 'u2'] },
    { vis: 'everyone', rules: { denied_users: ['u3'] }, kind: 'except', users: ['u3'] },
    { vis: 'everyone', rules: '{"allowed_users":[],"denied_users":[]}', kind: 'everyone' },
    { vis: 'everyone', rules: 'not json', kind: 'everyone' },
  ];
  for (const c of cases) {
    const a = M.audience(c.vis, c.rules);
    assert.equal(a.kind, c.kind, JSON.stringify(c));
    if (c.users) assert.deepEqual(a.users, c.users);
  }
});

test('the words name who can follow, and never show a raw user id', () => {
  const members = { u1: 'Mira', u2: 'Kael', u3: 'Tam' };
  const cases = [
    { aud: { kind: 'everyone', users: [] }, want: 'Players can open this' },
    { aud: { kind: 'dm', users: [] }, want: 'Only you and members with DM access' },
    { aud: { kind: 'some', users: ['u1'] }, want: 'Mira can open this' },
    { aud: { kind: 'some', users: ['u1', 'u2'] }, want: 'Mira and Kael can open this' },
    { aud: { kind: 'some', users: ['u1', 'u2', 'u3'] }, want: 'Mira, Kael and Tam can open this' },
    { aud: { kind: 'some', users: ['u1', 'gone'] }, want: 'Mira and 1 other player can open this' },
    { aud: { kind: 'some', users: ['gone', 'gone2'] }, want: '2 players can open this' },
    { aud: { kind: 'except', users: ['u3'] }, want: 'Players except Tam can open this' },
  ];
  for (const c of cases) assert.equal(M.audienceWords(c.aud, members), c.want);
});

test('badgeTip tells players where it goes and DMs who else can follow', () => {
  const members = { u1: 'Mira', u2: 'Kael' };
  const cases = [
    { staff: false, aud: { kind: 'dm', users: [] }, want: 'Opens Port Mirel' },
    { staff: false, aud: { kind: 'some', users: ['u1'] }, want: 'Opens Port Mirel' },
    { staff: true, aud: { kind: 'everyone', users: [] }, want: 'Opens Port Mirel. Players can open it too.' },
    { staff: true, aud: { kind: 'dm', users: [] }, want: 'Opens Port Mirel. Only you and members with DM access can open it.' },
    { staff: true, aud: { kind: 'some', users: ['u1', 'u2'] }, want: 'Opens Port Mirel. Mira and Kael can open it too.' },
  ];
  for (const c of cases) assert.equal(M.badgeTip('Port Mirel', c.aud, members, c.staff), c.want);
});

test('mapURL carries the maps the viewer came through, encoded', () => {
  assert.equal(M.mapURL('c1', 'm2', []), '/campaigns/c1/maps/m2');
  assert.equal(M.mapURL('c1', 'm2', ['m0', 'm1']), '/campaigns/c1/maps/m2?from=m0,m1');
  assert.equal(M.mapURL('c 1', 'm/2', ['a&b', '']), '/campaigns/c%201/maps/m%2F2?from=a%26b');
});

const tree = [
  { id: 'world', name: 'World', children: [
    { id: 'isle', name: 'Isle', children: [{ id: 'port', name: 'Port', children: [{ id: 'cellar', name: 'Cellar' }] }] },
    { id: 'vault', name: 'Vault' },
  ] },
  { id: 'other', name: 'Other' },
];

test('pathTo walks from a root down to the map, or finds nothing', () => {
  const p = (id) => M.pathTo(tree, id).map((n) => n.id).join('>');
  assert.equal(p('cellar'), 'world>isle>port>cellar');
  assert.equal(p('vault'), 'world>vault');
  assert.equal(p('other'), 'other');
  assert.equal(p('missing'), '');
  assert.deepEqual(M.pathTo(null, 'x'), []);
});

test('treeKey moves, opens, folds and closes like a tree', () => {
  // World (open) > Isle (closed), Vault (leaf); Other (leaf).
  const items = [
    { level: 1, expanded: true },
    { level: 2, expanded: false },
    { level: 2, expanded: null },
    { level: 1, expanded: null },
  ];
  const cases = [
    { i: 0, key: 'ArrowDown', want: { focus: 1 } },
    { i: 3, key: 'ArrowDown', want: { focus: 3 } },
    { i: 0, key: 'ArrowUp', want: { focus: 0 } },
    { i: 2, key: 'Home', want: { focus: 0 } },
    { i: 0, key: 'End', want: { focus: 3 } },
    { i: 1, key: 'ArrowRight', want: { expand: 1 } },
    { i: 0, key: 'ArrowRight', want: { focus: 1 } },
    { i: 2, key: 'ArrowRight', want: { none: true } },
    { i: 0, key: 'ArrowLeft', want: { collapse: 0 } },
    { i: 2, key: 'ArrowLeft', want: { focus: 0 } },
    { i: 3, key: 'ArrowLeft', want: { none: true } },
    { i: 2, key: 'Enter', want: { open: 2 } },
    { i: 2, key: ' ', want: { open: 2 } },
    { i: 1, key: 'Escape', want: { close: true, refocus: true } },
    { i: 1, key: 'Tab', want: { close: true, refocus: false } },
    { i: 1, key: 'a', want: null },
    { i: 9, key: 'ArrowDown', want: null },
  ];
  for (const c of cases) assert.deepEqual(M.treeKey(items, c.i, c.key), c.want, `${c.key} on ${c.i}`);
});
