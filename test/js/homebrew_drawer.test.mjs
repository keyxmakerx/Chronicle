// homebrew_drawer.test.mjs: the pure helpers of the Homebrew drawer, loaded
// in a bare vm context with a stubbed Chronicle the way the server ships it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'homebrew', 'static', 'js', 'homebrew_drawer.js'), 'utf8');

function load() {
  const registered = {};
  const sandbox = { window: {}, Chronicle: { register: (n, i) => { registered[n] = i; } }, document: {}, console, Date };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { H: sandbox.window.HomebrewDrawer, registered };
}

test('registers the drawer widget', () => {
  const { registered } = load();
  assert.equal(typeof registered['homebrew-drawer'].init, 'function');
});

test('ago says when a run happened', () => {
  const { H } = load();
  const now = Date.parse('2026-10-19T12:00:00Z');
  assert.equal(H.ago('2026-10-19T11:59:40Z', now), 'just now');
  assert.equal(H.ago('2026-10-19T11:45:00Z', now), '15 min ago');
  assert.equal(H.ago('2026-10-19T11:00:00Z', now), '1 hour ago');
  assert.equal(H.ago('2026-10-19T07:00:00Z', now), '5 hours ago');
  assert.equal(H.ago('not a date', now), '');
});

test('stepNote says what Run does with each step', () => {
  const { H } = load();
  assert.equal(H.stepNote({ byHand: true }, false), 'For you to do');
  assert.equal(H.stepNote({ browser: true }, true), 'Runs here');
  assert.match(H.stepNote({ browser: true }, false), /^For you to do/);
  assert.equal(H.stepNote({}, false), 'Runs on the server');
});

test('statusText and pressLabel read plainly', () => {
  const { H } = load();
  assert.equal(H.statusText('dismissed'), 'Set aside');
  assert.equal(H.statusText('failed'), 'Stopped');
  assert.equal(H.pressLabel({ label: 'Log a respite', rules: 1 }), 'Log a respite (1 rule)');
  assert.equal(H.pressLabel({ label: 'Log a respite', rules: 3 }), 'Log a respite (3 rules)');
});
