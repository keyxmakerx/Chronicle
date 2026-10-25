// beside.test.mjs — pins how pages beside work out. Runs the real script in a
// vm sandbox with no document, so only its pure helpers load.
//
// The rules that matter: how many pages fit comes from the width alone;
// opening from a page replaces the panes after it; a full screen folds the
// oldest pane behind Earlier; only this site's campaign pages open beside.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/beside.js'), 'utf8');

function load() {
  const window = {};
  vm.runInNewContext(src, { window, URL });
  return window.Chronicle.Beside;
}

test('pagesFit: pages fit by width, one to four', () => {
  const { pagesFit } = load();
  for (const [w, want] of [[0, 1], [739, 1], [1480, 2], [1700, 2], [2360, 3], [3640, 4], [8000, 4]]) {
    assert.equal(pagesFit(w), want, `width ${w}`);
  }
});

test('plan: opening from the main page starts a new trail', () => {
  const { plan } = load();
  const r = plan(['/a', '/b'], ['/old'], 0, '/c', 3);
  assert.deepEqual([...r.panes], ['/c']);
  assert.deepEqual([...r.earlier], []);
  assert.equal(r.replaced, '/a');
  assert.equal(r.slot, 1);
  assert.equal(r.pages, 2);
});

test('plan: opening from a pane replaces the panes after it', () => {
  const { plan } = load();
  const r = plan(['/a', '/b'], [], 1, '/c', 3);
  assert.deepEqual([...r.panes], ['/a', '/c']);
  assert.equal(r.replaced, '/b');
  assert.equal(r.slot, 2);
  assert.equal(r.folds, null);
});

test('plan: a full screen folds the oldest pane behind Earlier', () => {
  const { plan } = load();
  const r = plan(['/a', '/b'], ['/x'], 2, '/c', 3);
  assert.deepEqual([...r.panes], ['/b', '/c']);
  assert.deepEqual([...r.earlier], ['/x', '/a']);
  assert.equal(r.folds, '/a');
  assert.equal(r.replaced, null);
  assert.equal(r.pages, 3);
});

test('beside: only campaign pages of this site', () => {
  const { beside } = load();
  const o = 'https://chronicle.example';
  const cases = [
    ['/campaigns/abc/entities/1', true],
    ['https://chronicle.example/campaigns/abc/maps', true],
    ['/campaigns/abc/', false],
    ['/campaigns', false],
    ['/login', false],
    ['https://other.example/campaigns/abc/entities/1', false],
    ['javascript:alert(1)', false],
    ['', false],
  ];
  for (const [href, want] of cases) assert.equal(beside(href, o), want, href);
});
