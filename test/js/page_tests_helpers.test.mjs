// page_tests_helpers.test.mjs: the pure helpers of the Tests block. The
// widget file is loaded in a bare vm context, the way the server ships it,
// and its exported helpers are driven directly; the DOM is not needed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'pagetests', 'static', 'js', 'page_tests.js'), 'utf8');

function load() {
  const sandbox = { window: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.window.PageTestsHelpers;
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('normalise clamps totals and keeps only the known triggers', () => {
  const H = load();
  const t = H.normalise({ id: 't1', name: 'Hunt', needed: 99, limit: -2, results: [{ id: 'a', label: 'Won', on: 'successes' }, { id: 'b', label: 'Lost', on: 'whenever' }] });
  assert.equal(t.needed, H.LIM.total);
  assert.equal(t.limit, 0);
  assert.equal(t.results[1].on, '');
  assert.equal(t.recorded, null);
  assert.equal(H.normalise(null).name, '');
});

test('a player\'s answer carries only the name and the recorded result', () => {
  const H = load();
  const t = H.normalise({ id: 't1', name: 'Hunt', shown: true, recorded: { result: 'a', label: 'Won', dateLabel: '3 Frost' } });
  assert.deepEqual(plain(t.recorded), { result: 'a', label: 'Won', dateLabel: '3 Frost' });
  assert.deepEqual(plain(t.results), []);
});

test('starter uses the system\'s setup, dropping triggers on a missing total', () => {
  const H = load();
  const s = H.starter({ kind: 'Montage', needed: 5, limit: 0, results: [
    { label: 'Total success', on: 'successes' }, { label: 'Partial', on: 'successes' }, { label: 'Total failure', on: 'failures' }
  ] });
  assert.equal(s.kind, 'Montage');
  assert.deepEqual(plain(s.results.map((r) => r.on)), ['successes', '', '']);
  const plainStart = H.starter(null);
  assert.deepEqual(plain(plainStart.results.map((r) => r.label)), ['Success', 'Partial success', 'Failure']);
  assert.equal(plainStart.shown, false);
});

test('totals says what ends a test', () => {
  const H = load();
  assert.equal(H.totals({ needed: 5, limit: 3 }), '5 successes before 3 failures');
  assert.equal(H.totals({ needed: 1, limit: 0 }), '1 success to finish');
  assert.equal(H.totals({ needed: 0, limit: 1 }), 'Ends at 1 failure');
  assert.equal(H.totals({ needed: 0, limit: 0 }), '');
});

test('setOn gives a total to one result only', () => {
  const H = load();
  let t = H.normalise({ needed: 3, results: [{ id: 'a', label: 'A', on: 'successes' }, { id: 'b', label: 'B' }] });
  t = H.setOn(t, 'b', 'successes');
  assert.deepEqual(plain(t.results.map((r) => r.on)), ['', 'successes']);
  assert.equal(H.offerNote({ offer: 'b' }, { id: 'b', on: 'failures' }), 'The failures are full. Press to record it.');
  assert.equal(H.offerNote({ offer: '' }, { id: 'b', on: 'failures' }), '');
});

test('toSave sends the setup only and clears a trigger on a total the test lacks', () => {
  const H = load();
  const t = H.normalise({ id: 't1', name: ' ', kind: ' Montage ', needed: 0, limit: 2, successes: 4, version: 7,
    results: [{ id: 'a', label: 'Won', on: 'successes' }, { id: 'b', label: ' Lost ', on: 'failures' }, { id: 'c', label: '  ' }] });
  assert.deepEqual(plain(H.toSave(t)), {
    name: 'Untitled test', kind: 'Montage', shown: false, needed: 0, limit: 2,
    results: [{ id: 'a', label: 'Won', on: '' }, { id: 'b', label: 'Lost', on: 'failures' }], version: 7
  });
});
