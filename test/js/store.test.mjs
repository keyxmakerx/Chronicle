// store.test.mjs — the pure parts of static/js/widgets/store.js: the gallery
// query each view sends, paging, the notes on a thing and the empty-state
// sentences. The server decides what is visible; these only shape and word it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(root, 'static/js/widgets/store.js'), 'utf8'), sandbox);
const U = sandbox.Chronicle.Store.util;
const plain = (o) => JSON.parse(JSON.stringify(o));

test('the query carries only the filters that are set, and only known kinds', () => {
  const cases = [
    [{ page: 1 }, '?page=1'],
    [{ page: 0 }, '?page=1'],
    [{ page: 3, all: true }, '?page=3&system=all'],
    [{ page: 1, kind: 'rooms' }, '?page=1&kind=rooms'],
    [{ page: 1, kind: 'crafting', q: 'moon & star' }, '?page=1&kind=crafting&q=moon%20%26%20star'],
    [{ page: 1, kind: 'home' }, '?page=1'],
    [{ page: 1, kind: 'rooms"><x' }, '?page=1'],
  ];
  for (const [f, want] of cases) assert.equal(U.query(f), want, JSON.stringify(f));
});

test('paging clamps to the pages there are', () => {
  assert.deepEqual(plain(U.pages(0, 1, 12)), { pages: 1, page: 1, from: 0, to: 0, prev: false, next: false });
  assert.deepEqual(plain(U.pages(30, 2, 12)), { pages: 3, page: 2, from: 13, to: 24, prev: true, next: true });
  assert.deepEqual(plain(U.pages(30, 9, 12)), { pages: 3, page: 3, from: 25, to: 30, prev: true, next: false });
});

test('notes name another game system first, then what the campaign lacks', () => {
  assert.deepEqual(plain(U.notes({ sameSystem: true, lacks: [] }, 'drawsteel')), []);
  const n = U.notes({ sameSystem: false, systemId: 'dnd5e', lacks: ['This campaign has no oak.', ''] }, 'drawsteel');
  assert.equal(n.length, 2);
  assert.match(n[0], /^Made for D&D 5e\. This campaign uses Draw Steel\./);
  assert.equal(n[1], 'This campaign has no oak.');
});

test('every gallery kind belongs to a Store kind with a word for it', () => {
  for (const k of ['room-theme', 'crafting-circle', 'crafting-item-look', 'crafting-bench']) {
    assert.ok(U.KINDS.some((s) => s.id === U.kindOf(k)), k);
    assert.notEqual(U.kindWord(k), 'Design', k);
  }
  assert.equal(U.kindOf('something-new'), '');
});

test('a report reason read back from the page is one of three', () => {
  assert.equal(U.reason('spam'), 'spam');
  assert.equal(U.reason('adult'), 'adult');
  assert.equal(U.reason('<script>'), 'other');
});

test('empty sentences say who fills the Store and never offer what is not there', () => {
  assert.match(U.emptyLine('home', false, 'Draw Steel', ''), /^Nothing has been shared for Draw Steel yet\. When owners on this server share/);
  assert.match(U.emptyLine('rooms', true, 'Draw Steel', ''), /^No shop rooms have been shared on this server yet\./);
  assert.equal(U.emptyLine('rooms', false, 'Draw Steel', 'kettle'), 'Nothing matches “kettle”.');
});
