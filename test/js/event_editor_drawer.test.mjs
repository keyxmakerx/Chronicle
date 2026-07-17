// event_editor_drawer.test.mjs — the redesigned full event editor DRAWER
// (C-CAL-LARGE-EDITOR, design slice 5 of 6). Source-level seams on
// event_grid.js, matching the harness style used across this suite (the drawer
// logic is a browser IIFE; we pin the load-bearing wiring by asserting the
// source carries it rather than driving a headless DOM):
//   1. Modal a11y: focus-trap on Tab, Esc + scrim close, focus restore.
//   2. All-day ⇒ nil clock time; times parsed from the HH:MM inputs; integer
//      coercion so <select> month/day/year reach Go as ints.
//   3. Inline 422 rendering (data-drawer-error) — no toast-only validation.
//   4. Tier segment + type chips ↔ hidden fields.
//   5. Entry-point contract preserved: every consumer still funnels through
//      openDrawer / _openDrawer / calV2OpenDrawerByID / calV2OpenCreateDrawer.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(
  join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js'),
  'utf8',
);

test('focus trap: Tab/Shift+Tab cycle inside the open drawer, focus restored on close', () => {
  assert.match(src, /function onTrapKeydown/, 'a Tab-key trap handler must exist');
  assert.match(src, /drawer\.addEventListener\('keydown', onTrapKeydown\)/, 'trap must attach on open');
  assert.match(src, /drawer\.removeEventListener\('keydown', onTrapKeydown\)/, 'trap must detach on close');
  assert.match(src, /_lastFocus/, 'the opener element must be remembered');
  assert.match(src, /_lastFocus\.focus\(\)/, 'focus must be restored to the opener on close');
  // Only VISIBLE focusables are trapped (hidden time inputs / collapsed panels
  // must not become dead tab stops).
  assert.match(src, /offsetParent !== null/, 'trap must filter to visible focusables');
});

test('Esc and scrim both close the drawer', () => {
  assert.match(src, /e\.key === 'Escape'[\s\S]*?closeDrawer\(false\)/, 'Esc must close the drawer');
  assert.match(src, /data-drawer-backdrop[\s\S]*?closeDrawer\(false\)/, 'scrim/backdrop click must close');
});

test('all-day drops the clock time and sends all_day; otherwise times parse from HH:MM', () => {
  // readDrawer branch.
  assert.match(src, /if \(isAllday\(\)\) \{[\s\S]*?body\.all_day = true;[\s\S]*?delete body\.start_hour/,
    'all-day must set all_day:true and drop the clock fields');
  assert.match(src, /body\.all_day = false;[\s\S]*?parseTimeInput\(drawer\.querySelector\('\[data-time-start\]'\)\)/,
    'timed save must parse the start-time input');
  assert.match(src, /function parseTimeInput/, 'HH:MM parser must exist');
  // Integer coercion so month/day/year (a <select> is a string value) reach Go
  // as real ints.
  assert.match(src, /\['year', 'month', 'day'[\s\S]*?parseInt\(body\[k\], 10\)/,
    'integer fields must be coerced before send');
});

test('all-day toggle shows/hides the time inputs', () => {
  assert.match(src, /function syncAllday/, 'syncAllday must exist');
  assert.match(src, /data-time-fields[\s\S]*?classList\.toggle\('hidden'/, 'time inputs hide when all-day');
});

test('inline 422 rendering — validation errors surface next to the form, not a toast', () => {
  assert.match(src, /function showDrawerError/, 'inline error renderer must exist');
  assert.match(src, /\[data-drawer-error\]/, 'the inline error region selector must be used');
  // saveDrawer surfaces the endpoint {error,message} inline on failure.
  const save = src.slice(src.indexOf('function saveDrawer'), src.indexOf('function commitDelete'));
  assert.match(save, /showDrawerError\(/, 'saveDrawer must render failures inline');
  assert.match(save, /b\.message/, 'saveDrawer must read the endpoint message body');
});

test('type chips + tier segment drive their hidden fields', () => {
  assert.match(src, /function syncTypeChips[\s\S]*?data-type-chip/, 'type chips must sync a hidden category');
  assert.match(src, /function syncTierSeg[\s\S]*?data-tier-chip/, 'tier segment must sync a hidden tier');
  assert.match(src, /\[data-tier-chip\][\s\S]*?syncTierSeg/, 'tier chips must be click-wired');
  assert.match(src, /\[data-type-chip\][\s\S]*?syncTypeChips/, 'type chips must be click-wired');
});

test('fantasy-equivalence hint recomputes from the active-calendar data on date change', () => {
  assert.match(src, /function updateFantasyHint/, 'fantasy hint updater must exist');
  assert.match(src, /drawer\.dataset\.calEras/, 'hint must read the eras carried on the drawer');
  assert.match(src, /drawer\.dataset\.calEpoch/, 'hint must read the epoch carried on the drawer');
});

test('entry-point contract preserved — every consumer still funnels through openDrawer', () => {
  // The redesign rebuilds the SHARED drawer; it must NOT change the open API the
  // quick-edit expand, day-popover +Event, scribe cell-add, drag-create, +N-more
  // fallback, and permalink focus all depend on.
  assert.match(src, /_openDrawer = openDrawer/, 'the drag-create live hook must stay wired');
  assert.match(src, /window\.calV2OpenDrawerByID = openDrawer/, 'the +N-more / popover-row hook must stay exposed');
  assert.match(src, /window\.calV2OpenCreateDrawer = function/, 'the day-popover +Event create hook must stay exposed');
  assert.match(src, /data-qe-expand[\s\S]*?openDrawer\(id\)/, 'quick-edit "Full editor" must open the drawer');
  assert.match(src, /data-cell-add-event[\s\S]*?openDrawer\(\{/, 'scribe cell-add must open the create drawer');
  assert.match(src, /focus[\s\S]*?openDrawer\(focusID\)/, 'permalink focus must reopen the drawer');
});
