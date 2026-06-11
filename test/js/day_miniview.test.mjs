// day_miniview.test.mjs — the day MINI-VIEW wiring contracts (cordinator#33
// item 4). Source-level seams on calendar_v2_shell.js + event_grid.js:
//   1. A date-cell click opens the day mini-view (not the create drawer); the
//      "+N more" overflow opens the same card. Opening is a same-trigger press
//      (pointerup), so a cross-cell drag-create still wins.
//   2. It REUSES the existing worldstate peek (GET /calendar/world-state) and
//      routes an event row back through the existing event-card click path.
//   3. "Add event" (Scribe) opens the prefilled create drawer via the exposed
//      window.calV2OpenCreateDrawer hook; the single-cell drag-create no longer
//      opens the drawer directly (the mini-view is the first tier).
//   4. Leak discipline (E4/E5 class): every closer binds to the page root (or
//      the popover node), never document; per-node wiring guard.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const base = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js');
const shell = readFileSync(join(base, 'calendar_v2_shell.js'), 'utf8');
const grid = readFileSync(join(base, 'event_grid.js'), 'utf8');

// Isolate the wireDayPopover function body for leak/scope assertions.
function dayPopoverBlock() {
  const start = shell.indexOf('function wireDayPopover');
  assert.ok(start >= 0, 'wireDayPopover present');
  // up to the next top-level function in the IIFE
  const after = shell.indexOf('\n    if (document.readyState', start);
  return shell.slice(start, after > start ? after : undefined);
}

test('a date-cell press opens the mini-view (same-trigger, not a drag)', () => {
  const block = dayPopoverBlock();
  // Opens on a date cell (empty space, not a chip / button / link).
  assert.match(block, /\[data-day-cell\]/, 'must target date cells');
  assert.match(block, /\[data-event-card\], a, button/, 'must exclude chips/buttons/links from the cell trigger');
  // Same-trigger press: down and up on the same element (so a cross-cell drag
  // releases on a different trigger and is skipped — drag-create still wins).
  assert.match(block, /addEventListener\('pointerup'/, 'opens on pointerup');
  assert.match(block, /up\.key !== down\.key/, 'must require down+up on the SAME trigger (drag suppression)');
  assert.match(block, /openPopover\(/, 'a qualifying press opens the popover');
});

test('the "+N more" overflow opens the same day card', () => {
  assert.match(dayPopoverBlock(), /\[data-cell-overflow-toggle\]/, 'overflow remains an open trigger');
});

test('reuses the existing worldstate peek + the existing event-card click path', () => {
  const block = dayPopoverBlock();
  assert.match(block, /renderWorldStatePeek\(day\)/, 'opening reuses renderWorldStatePeek');
  assert.match(block, /\/calendar\/world-state\?year=/, 'peek reuses the existing #401 GET seed');
  assert.match(block, /card\.click\(\)/, 'an event row routes through the existing event-card click path');
});

test('"Add event" (Scribe) opens the prefilled create drawer via the exposed hook', () => {
  const block = dayPopoverBlock();
  assert.match(block, /data-day-popover-add/, 'wires the Scribe Add-event button');
  assert.match(block, /window\.calV2OpenCreateDrawer\(\{/, 'Add event opens the prefilled create drawer');
  // event_grid exposes the create-drawer hook used above.
  assert.match(grid, /window\.calV2OpenCreateDrawer = function/, 'event_grid must expose calV2OpenCreateDrawer');
});

test('the single-cell drag-create no longer opens the drawer directly', () => {
  // The multi-cell drag still opens the drawer (drag-create wins)...
  assert.match(grid, /if \(r\.multi\) \{[\s\S]*?_openDrawer\(\{/, 'multi-cell drag still opens the create drawer');
  // ...but the single-cell branch that used to open the drawer is gone — the
  // mini-view is the first tier now.
  assert.doesNotMatch(grid, /_openDrawer\(\{ year: s\.year, month: s\.month, day: r\.startDay \}\)/,
    'single-cell press must NOT open the drawer directly anymore');
});

test('closers are root/popover-bound (no document listener leak) + per-node guard', () => {
  const block = dayPopoverBlock();
  assert.match(block, /root\.addEventListener\('pointerdown'/, 'press handling binds to root');
  assert.match(block, /root\.addEventListener\('click'/, 'outside-click closer binds to root');
  assert.doesNotMatch(block, /document\.addEventListener/, 'wireDayPopover must not add document listeners');
  assert.match(block, /dataset\.dvWired/, 'element-scoped wiring is guarded per node');
});
