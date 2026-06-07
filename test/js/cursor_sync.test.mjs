// cursor_sync.test.mjs — Tuner §J1 cross-widget cursor protocol, the
// Almanac side (touches cal-almanac.js). Verifies the Almanac emits
// cal:cursor-change when its world-state time-of-day moves, listens for a
// sibling's cursor change and mirrors it onto its own sky band, and
// loop-prevents its own events. Visual fidelity is the operator's gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './harness.mjs';

test('Almanac exposes the cursor-sync interface', () => {
  const w = boot({ reduced: true });
  const sync = w.__calCursorSync;
  assert.ok(sync, '__calCursorSync exposed');
  assert.ok(/^almanac-/.test(sync.selfId), 'self id namespaced to almanac');
  assert.equal(sync.type, 'calendar');
  assert.equal(typeof sync.emitEventCreate, 'function');
});

test('Almanac emits cal:cursor-change when its time-of-day changes', () => {
  const w = boot({ reduced: true });
  const bus = w.__bus;
  bus.events.length = 0;
  // A user scrub / type routes through setWorldState; the sync subscriber
  // should emit a cursor-change with the new sky time.
  w.__calSetWorldState({ timeOfDay: 0.321 });
  const emitted = bus.events.filter((e) => e.type === 'cal:cursor-change');
  assert.ok(emitted.length >= 1, 'cursor-change emitted on time change');
  const last = emitted[emitted.length - 1];
  assert.equal(last.detail.sourceWidgetId, w.__calCursorSync.selfId);
  assert.equal(last.detail.sourceWidgetType, 'calendar');
  assert.ok(Math.abs(last.detail.skyTime - 0.321) < 1e-6, 'carries the new sky time');
});

test('Almanac emitEventCreate dispatches cal:event-create', () => {
  const w = boot({ reduced: true });
  const bus = w.__bus;
  bus.events.length = 0;
  w.__calCursorSync.emitEventCreate('ev-new', { year: 1492, month: 4, day: 14 });
  const created = bus.events.filter((e) => e.type === 'cal:event-create');
  assert.equal(created.length, 1);
  assert.equal(created[0].detail.eventId, 'ev-new');
  assert.equal(created[0].detail.sourceWidgetId, w.__calCursorSync.selfId);
});

test('Almanac applies a sibling cursor change to its sky band (and does not loop)', () => {
  const w = boot({ reduced: true });
  const bus = w.__bus;
  // Sibling reports a new cursor with a sky time → Almanac mirrors it.
  w.document.dispatchEvent(new w.CustomEvent('cal:cursor-change', {
    detail: { sourceWidgetId: 'tuner-abc', sourceWidgetType: 'timeline', date: { year: 1492, month: 4, day: 14 }, skyTime: 0.8 },
  }));
  assert.equal(w.__calCursorSync.lastExternal.sourceWidgetId, 'tuner-abc');
  assert.ok(Math.abs(w.__calWorldState.timeOfDay - 0.8) < 1e-6, 'sky band mirrored to sibling time');

  // The external apply must not re-emit a cursor-change back (loop guard).
  bus.events.length = 0;
  w.document.dispatchEvent(new w.CustomEvent('cal:cursor-change', {
    detail: { sourceWidgetId: 'tuner-abc', sourceWidgetType: 'timeline', date: { year: 1492, month: 4, day: 14 }, skyTime: 0.4 },
  }));
  // bus.events also records the incoming event we dispatched; a re-emit
  // would be one sourced from the Almanac itself.
  const echoed = bus.events.filter((e) => e.type === 'cal:cursor-change' &&
    e.detail.sourceWidgetId === w.__calCursorSync.selfId);
  assert.equal(echoed.length, 0, 'no re-emit on external apply (loop prevented)');
});

test('Almanac ignores its own cursor-change events', () => {
  const w = boot({ reduced: true });
  const sync = w.__calCursorSync;
  sync.lastExternal = null;
  w.document.dispatchEvent(new w.CustomEvent('cal:cursor-change', {
    detail: { sourceWidgetId: sync.selfId, date: { year: 1400, month: 1, day: 1 }, skyTime: 0.1 },
  }));
  assert.equal(sync.lastExternal, null, 'self-sourced event ignored');
});
