// topbar_tools.test.mjs — pins topbar_tools.js's rules: what stays on the bar
// is remembered as known tools only, at most two; the tab counts only the
// tools it hides; the badges' words read as numbers.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'topbar_tools.js'), 'utf8');

const registered = {};
const sandbox = { Chronicle: { register: (name, impl) => { registered[name] = impl; } } };
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const T = sandbox.Chronicle._topbarTools;

test('the widget registers under the name the templ mounts', () => {
  assert.equal(typeof registered['topbar-tools'].init, 'function');
});

test('readKept keeps known tools on this bar, once each, at most two', () => {
  const all = ['ai', 'table', 'dms', 'pv', 'bell'];
  const cases = [
    ['nothing stored', null, all, []],
    ['not JSON', '{oops', all, []],
    ['not a list', '{"ai":1}', all, []],
    ['two kept', '["dms","pv"]', all, ['dms', 'pv']],
    ['a third is dropped', '["dms","pv","bell"]', all, ['dms', 'pv']],
    ['unknown and repeated names are dropped', '["find","dms","dms","x"]', all, ['dms']],
    ['a tool this viewer lacks is dropped', '["dms","bell"]', ['table', 'bell'], ['bell']],
  ];
  for (const [name, raw, available, want] of cases) assert.deepEqual([...T.readKept(raw, available)], want, name);
});

test('toggleKept keeps, puts back, and refuses a third', () => {
  assert.deepEqual([...T.toggleKept([], 'dms')], ['dms']);
  assert.deepEqual([...T.toggleKept(['dms'], 'pv')], ['dms', 'pv']);
  assert.deepEqual([...T.toggleKept(['dms', 'pv'], 'dms')], ['pv']);
  assert.equal(T.toggleKept(['dms', 'pv'], 'bell'), null);
  assert.equal(T.toggleKept([], 'find'), null, 'Find is always on the bar');
});

test('countOf reads a badge the way it shows', () => {
  const cases = [['3', 3], ['9+', 9], [' 12 ', 12], ['', 0], [null, 0], ['new', 0]];
  for (const [text, n] of cases) assert.equal(T.countOf(text), n, String(text));
  assert.equal(T.label(4), '4');
  assert.equal(T.label(10), '9+');
});

test('the tab counts only what it hides', () => {
  const counts = { ai: 2, table: 1, bell: 4 };
  assert.equal(T.tabCount(counts, []), 7);
  assert.equal(T.tabCount(counts, ['bell']), 3);
  assert.equal(T.tabCount(counts, ['ai', 'table']), 4);
  assert.equal(T.tabCount({}, []), 0);
});

test('the hint says how many more can be kept', () => {
  assert.match(T.hintFor(0), /up to two/);
  assert.match(T.hintFor(1), /one more/);
  assert.match(T.hintFor(2), /put it back/);
});
