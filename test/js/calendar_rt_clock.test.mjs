// calendar_rt_clock.test.mjs — the real-time live-clock wiring contracts
// (C-REAL-CALENDAR-P3). Source-level seams on calendar_v2_shell.js's
// wireRealTimeClocks (the same static-analysis convention as the other shell
// tests — the tick itself is manual-verified on a live client):
//   1. Renders the live time in the calendar's anchor zone via Intl at MINUTE
//      granularity (no per-second work), keyed off [data-cal-rt-clock] +
//      data-rt-zone.
//   2. Draws the "Your time" dual line ONLY when the viewer's browser zone
//      differs from the anchor zone, computed client-side from the same instant.
//   3. prefers-reduced-motion → paint once, no live interval (static).
//   4. Runs document-wide (header + dashboard) outside the [data-cal-v2-root]
//      shell guard, with a per-node guard + a reaper so htmx swaps don't leak
//      minute intervals on detached nodes.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const base = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js');
const shell = readFileSync(join(base, 'calendar_v2_shell.js'), 'utf8');

// Isolate the wireRealTimeClocks function body.
function clockBlock() {
  const start = shell.indexOf('function wireRealTimeClocks');
  assert.ok(start >= 0, 'wireRealTimeClocks present');
  const after = shell.indexOf('\n    // boot wires', start);
  return shell.slice(start, after > start ? after : undefined);
}

test('renders the live time in the anchor zone at minute granularity', () => {
  const block = clockBlock();
  assert.match(block, /\[data-cal-rt-clock\]/, 'scans clock nodes');
  assert.match(block, /getAttribute\('data-rt-zone'\)/, 'reads the anchor zone from data-rt-zone');
  assert.match(block, /Intl\.DateTimeFormat/, 'formats via Intl (zone-aware)');
  assert.match(block, /hour:\s*'2-digit'[\s\S]*minute:\s*'2-digit'/, 'HH:MM fields');
  assert.match(block, /hour12:\s*false/, '24-hour clock');
  assert.match(block, /\[data-rt-anchor\]/, 'writes the anchor line');
  // Minute tick, no per-second work.
  assert.match(block, /setInterval\([^,]+,\s*60000\)/, 'ticks once per minute (60000ms)');
  assert.doesNotMatch(block, /,\s*1000\)/, 'no per-second interval');
});

test('draws the viewer-local dual line only when zones differ', () => {
  const block = clockBlock();
  assert.match(block, /resolvedOptions\(\)\.timeZone/, 'reads the browser zone');
  assert.match(block, /\[data-rt-local\]/, 'targets the dual-line node');
  assert.match(block, /browserZone\s*&&\s*browserZone\s*!==\s*zone/, 'gates the dual line on a zone mismatch');
  assert.match(block, /Your time:/, 'labels the viewer-local line');
  assert.match(block, /classList\.remove\('hidden'\)[\s\S]*classList\.add\('hidden'\)/, 'shows on mismatch, hides otherwise');
});

test('honors prefers-reduced-motion (static, no live tick)', () => {
  const block = clockBlock();
  assert.match(block, /prefers-reduced-motion/, 'queries reduced-motion');
  // Paint once regardless, then bail before scheduling the tick under reduce.
  assert.match(block, /paint\(clock\);[\s\S]*if\s*\(reduce\)\s*continue/, 'paints once then skips the tick when reduced');
});

test('runs document-wide with per-node guard + leak reaper', () => {
  const block = clockBlock();
  assert.match(block, /document\.querySelectorAll\('\[data-cal-rt-clock\]'\)/, 'document-wide scan (works on the dashboard, no shell root)');
  assert.match(block, /__rtClockInited/, 'per-node init guard (no double-wiring on htmx settle)');
  assert.match(block, /document\.contains\(/, 'reaper checks node attachment');
  assert.match(block, /clearInterval\(/, 'reaps stale minute intervals on detached nodes');
});

test('the clock wiring runs outside the shell root guard (boot, not init)', () => {
  // init() early-returns without [data-cal-v2-root]; the clocks must still run on
  // the dashboard, so boot() calls wireRealTimeClocks() alongside init().
  assert.match(shell, /function boot\(\)\s*\{[\s\S]*init\(\);[\s\S]*wireRealTimeClocks\(\);[\s\S]*\}/, 'boot() wires clocks outside init()');
  assert.match(shell, /addEventListener\('htmx:afterSettle',\s*boot\)/, 're-wires on htmx settle');
});
