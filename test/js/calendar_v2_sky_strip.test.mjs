// calendar_v2_sky_strip.test.mjs — C-CAL-SKY-STRIP. Pins computeSyncChipState,
// the pure function behind the Calendaria sync chip: in-sync / stale / drift /
// never-synced. Runs the real calendar_v2_shell.js source through a minimal
// vm sandbox (the window.__calMoonSim / window.__calSkyFxMeta reuse-seam
// convention from skybox_widget.test.mjs) rather than re-implementing the
// logic in the test, so a change to the production function is what the test
// actually exercises.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const shellSrc = readFileSync(join(root, 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_v2_shell.js'), 'utf8');

function boot() {
  const noopDoc = {
    readyState: 'complete',
    querySelector: () => null,
    querySelectorAll: () => [],
    contains: () => true,
    addEventListener: () => {},
  };
  const sandbox = {
    console,
    document: noopDoc,
    window: null,
    matchMedia: () => ({ matches: false }),
    setTimeout, clearTimeout, setInterval, clearInterval,
    Date,
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(shellSrc, sandbox, { filename: 'calendar_v2_shell.js' });
  return sandbox;
}

const { computeSyncChipState } = boot().__calSkyStripSync;

test('never connected (neverSeen=true) is always never_synced', () => {
  const s = computeSyncChipState(true, null, Date.now(), 900000, '', '');
  assert.equal(s.status, 'never_synced');
  assert.equal(s.agoMs, null);
});

test('null lastSeen with neverSeen=false still reads never_synced (defensive)', () => {
  const s = computeSyncChipState(false, null, Date.now(), 900000, '', '');
  assert.equal(s.status, 'never_synced');
});

test('recently seen, within the freshness window, reads in_sync', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 2 * 60 * 1000; // 2 minutes ago
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '', '');
  assert.equal(s.status, 'in_sync');
  assert.equal(s.agoMs, 2 * 60 * 1000);
});

test('seen long ago, beyond the freshness window, reads stale', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 45 * 60 * 1000; // 45 minutes ago
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '', '');
  assert.equal(s.status, 'stale');
});

test('exactly at the freshness boundary reads in_sync (not >, not stale)', () => {
  const now = 1_000_000_000;
  const staleAfter = 15 * 60 * 1000;
  const lastSeen = now - staleAfter; // exactly the boundary
  const s = computeSyncChipState(false, lastSeen, now, staleAfter, '', '');
  assert.equal(s.status, 'in_sync');
});

test('a last-confirmed Foundry date that differs from Chronicle current reads drift', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '12 Jul 2026', '14 Jul 2026');
  assert.equal(s.status, 'drift');
  assert.equal(s.date, '12 Jul 2026');
});

test('matching confirmed dates do not trigger drift, even though both are known', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '14 Jul 2026', '14 Jul 2026');
  assert.equal(s.status, 'in_sync');
});

test('negative clock skew (lastSeen "after" now) clamps agoMs to 0, never negative', () => {
  const now = 1_000_000_000;
  const lastSeen = now + 5000; // clock skew: presence ping timestamped slightly ahead
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '', '');
  assert.equal(s.agoMs, 0);
  assert.equal(s.status, 'in_sync');
});
