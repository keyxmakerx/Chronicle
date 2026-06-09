// event_drag.test.mjs — C-CAL-INTERACTIONS. The drag-to-create day-range logic
// (single vs multi-day), and that the drag layer re-inits on boosted nav + the
// reschedule/resize write paths are present (QA2-style seam coverage; the
// pointer/HTML5-DnD DOM flow itself is the operator visual gate).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js');
const src = readFileSync(jsPath, 'utf8');

function boot() {
  const docEvents = {};
  const sandbox = {
    console,
    document: {
      readyState: 'complete',
      querySelector: () => null,        // no root → init() bails; dayRange + htmx wiring still run
      querySelectorAll: () => [],
      addEventListener: (t, fn) => { (docEvents[t] = docEvents[t] || []).push(fn); },
      removeEventListener() {},
    },
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'event_grid.js' });
  return { docEvents, dayRange: sandbox.window.__calDayRange };
}

test('drag-create: a single cell is a single-day add (no range)', () => {
  const { dayRange } = boot();
  const r = dayRange(5, 5);
  assert.equal(r.startDay, 5);
  assert.equal(r.endDay, 5);
  assert.equal(r.multi, false);
});

test('drag-create: dragging across cells yields an ordered multi-day range', () => {
  const { dayRange } = boot();
  const r = dayRange(5, 8);
  assert.equal(r.startDay, 5);
  assert.equal(r.endDay, 8);
  assert.equal(r.multi, true);
});

test('drag-create: a reversed drag (end before start) normalizes order', () => {
  const { dayRange } = boot();
  const r = dayRange(12, 9);
  assert.equal(r.startDay, 9);
  assert.equal(r.endDay, 12);
  assert.equal(r.multi, true);
});

test('the drag layer re-inits on boosted nav (htmx:afterSettle + htmx:load)', () => {
  const { docEvents } = boot();
  assert.ok(docEvents['htmx:afterSettle'] && docEvents['htmx:afterSettle'].length, 'htmx:afterSettle not wired');
  assert.ok(docEvents['htmx:load'] && docEvents['htmx:load'].length, 'htmx:load not wired');
});

test('the seams are present: per-root guard, wired-once drag-create, reschedule + resize writes', () => {
  // Source-level (QA2 pattern): the DOM pointer / HTML5-DnD flow is the visual
  // gate, but the wiring must not silently regress.
  assert.match(src, /__eventGridInited/, 'per-root re-init guard missing');
  assert.match(src, /_dayDragWired/, 'drag-create must be wired ONCE (no listener leak)');
  // Drag-to-reschedule: a drop PUTs the moved event.
  assert.match(src, /drop['"]?\s*,/, 'cell drop handler missing');
  assert.match(src, /method:\s*['"]PUT['"]/, 'reschedule/resize PUT missing');
  // Drag-to-resize on the multi-day ribbon edge.
  assert.match(src, /data-ribbon-resize/, 'ribbon resize handle wiring missing');
});
