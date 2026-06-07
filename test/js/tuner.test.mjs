// tuner.test.mjs — headless runtime tests for cal-timeline-tuner.js
// (C-TIMELINE-V2-DESIGN-1-TUNER). Exercises the pure helpers: the
// adaptive-tick model, the registry hooks (axis + backdrop), the §J2
// backdrop restraint rules, the swim-lane grouping, and the §J1
// cursor-sync DOM-event protocol (emit / listen / loop-prevention).
// Visual fidelity is the operator's local gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './tuner_harness.mjs';

// Objects/arrays returned from the vm context carry the sandbox realm's
// prototypes, which the strict deep-equal rejects. Clone across the realm
// boundary before structural comparison.
const clone = (x) => JSON.parse(JSON.stringify(x));

test('init runs every block without error', () => {
  const { window: w } = boot();
  const res = w.__tunerResults;
  assert.ok(res, 'init results exposed');
  for (const k of Object.keys(res)) assert.equal(res[k], 'ok', 'block ' + k + ' ok: ' + res[k]);
});

// ── Adaptive ticks (§A) ──────────────────────────────────────────────
test('tickModel adapts granularity across the zoom thresholds', () => {
  const { window: w } = boot();
  const m = (px) => clone(w.__tunerTickModel(px));
  assert.deepEqual(m(0.005), { level: 'max-out', major: 'millennia', minor: 'centuries' });
  assert.deepEqual(m(0.05), { level: 'high-out', major: 'centuries', minor: 'decades' });
  assert.deepEqual(m(0.5), { level: 'medium-out', major: 'decades', minor: 'years' });
  assert.deepEqual(m(2.5), { level: 'default', major: 'years', minor: 'months' });
  assert.deepEqual(m(20), { level: 'medium-in', major: 'months', minor: 'weeks' });
  assert.deepEqual(m(80), { level: 'high-in', major: 'weeks', minor: 'days' });
  assert.deepEqual(m(300), { level: 'max-in', major: 'days', minor: 'hours' });
});

// ── Registry hooks (§A overlays + §J2 backdrop) ──────────────────────
test('registries gained the timelineAxisRender hook; MUST-tier entries implement it', () => {
  const { window: w } = boot();
  const { weather, celestial } = w.__tunerRegistries;
  for (const id of ['rain', 'snow', 'thunderstorm', 'fog', 'cloudy']) {
    assert.ok(weather[id], 'weather has ' + id);
    assert.equal(weather[id].tier, 'must');
    assert.equal(typeof weather[id].timelineAxisRender, 'function', id + ' axis hook');
  }
  for (const id of ['meteor-shower', 'eclipse-solar', 'eclipse-lunar']) {
    assert.ok(celestial[id], 'celestial has ' + id);
    assert.equal(celestial[id].tier, 'must');
    assert.equal(typeof celestial[id].timelineAxisRender, 'function', id + ' axis hook');
  }
});

test('timelineBackdropRender exists on MUST weather + non-routine celestial', () => {
  const { window: w } = boot();
  const { weather, celestial } = w.__tunerRegistries;
  for (const id of ['rain', 'snow', 'thunderstorm', 'fog', 'cloudy']) {
    assert.equal(typeof weather[id].timelineBackdropRender, 'function', id + ' backdrop hook');
  }
  for (const id of ['meteor-shower', 'eclipse-solar', 'eclipse-lunar']) {
    assert.equal(typeof celestial[id].timelineBackdropRender, 'function', id + ' backdrop hook');
  }
  // The hook builds a node into the container without throwing.
  const container = { appendChild(c) { (this.kids = this.kids || []).push(c); return c; } };
  const out = weather.rain.timelineBackdropRender({ container, opacity: 0.2 });
  assert.ok(out, 'rain backdrop returned a node');
  assert.equal(container.kids.length, 1);
});

// ── Backdrop restraint (§J2, binding) ────────────────────────────────
test('backdrop restraint: sun+moons render ONLY on special-moon days', () => {
  const { window: w } = boot();
  const plan = w.__tunerBackdropPlan;
  // Special-moon day → sky-band (sun + moons) renders.
  const special = plan('1492-4-14');
  assert.equal(special.skyBand, true, 'special day is a sky-band day');
  assert.equal(special.sunMoon, true, 'sun + moons render on special day');
  assert.ok(special.active, 'special day backdrop active');
  // Plain weather day → weather renders, but NOT sun/moons.
  const rainy = plan('1492-4-12');
  assert.equal(rainy.weather, 'rain');
  assert.equal(rainy.sunMoon, false, 'no sun/moon backdrop on a regular rainy day');
  assert.equal(rainy.skyBand, false);
  assert.ok(rainy.active);
  // Meteor (non-routine celestial) renders, still no sun/moons.
  const meteor = plan('1489-3-8');
  assert.deepEqual(clone(meteor.celestial), ['meteor-shower']);
  assert.equal(meteor.sunMoon, false, 'no sun/moon backdrop on a meteor-only day');
  // A plain day → nothing active.
  const plain = plan('1492-4-20');
  assert.equal(plain.active, false, 'plain day has no backdrop');
  assert.equal(plain.weather, null);
  assert.equal(plain.sunMoon, false);
});

// ── Swim-lane grouping (§D) ──────────────────────────────────────────
test('groupLanes supports entity / category / tier and drops empty lanes', () => {
  const { window: w } = boot();
  const g = w.__tunerGroupLanes;
  const evs = w.__tunerState ? null : null; // events come from DATA inside the helper
  const byEntity = g(JSON.parse(JSON.stringify([])), 'entity'); // empty → no lanes
  assert.equal(byEntity.length, 0, 'no events → no lanes');

  // Use the live dataset's events via the exposed render path: pass the
  // mock events through directly.
  const mockEvents = [
    { id: 'ev-a', year: 1492, month: 4, day: 14, tier: 'major', category: 'founding', entities: ['ent-aragorn'] },
    { id: 'ev-b', year: 1492, month: 4, day: 12, tier: 'detail', category: 'battle', entities: ['ent-aragorn'] },
    { id: 'ev-c', year: 1492, month: 4, day: 14, tier: 'major', category: 'founding', entities: ['ent-frodo'] },
  ];
  const ent = clone(g(mockEvents, 'entity'));
  assert.ok(ent.length >= 2, 'aragorn + frodo lanes');
  const cat = clone(g(mockEvents, 'category'));
  const catKeys = cat.map((l) => l.key).sort();
  assert.deepEqual(catKeys, ['battle', 'founding']);
  const tier = clone(g(mockEvents, 'tier'));
  const tierKeys = tier.map((l) => l.key).sort();
  assert.deepEqual(tierKeys, ['detail', 'major'], 'empty "standard" lane dropped');
});

// ── Cursor-sync DOM-event protocol (§J1) ─────────────────────────────
test('cursor-sync emits cal:cursor-change when the needle moves', () => {
  const { window: w, bus } = boot();
  const sync = w.__tunerCursorSync;
  assert.ok(sync && sync.selfId, 'cursor-sync exposed with a self id');
  bus.events.length = 0;
  w.__tunerSetCursorDay(w.__tunerDayIndex(1492, 4, 20), { immediate: true });
  const emitted = bus.events.filter((e) => e.type === 'cal:cursor-change');
  assert.equal(emitted.length, 1, 'one cursor-change emitted');
  assert.equal(emitted[0].detail.sourceWidgetId, sync.selfId);
  assert.equal(emitted[0].detail.sourceWidgetType, 'timeline');
  assert.equal(emitted[0].detail.date.day, 20);
});

test('cursor-sync listens to sibling cursor changes and moves the needle', () => {
  const { window: w, bus } = boot();
  const sync = w.__tunerCursorSync;
  const before = w.__tunerState.cursorDay;
  bus.events.length = 0;
  // A sibling (different source) reports a new cursor date.
  w.document.dispatchEvent(new w.CustomEvent('cal:cursor-change', {
    detail: { sourceWidgetId: 'almanac-xyz', sourceWidgetType: 'calendar', date: { year: 1490, month: 1, day: 1 } },
  }));
  assert.equal(sync.lastExternal.sourceWidgetId, 'almanac-xyz', 'external recorded');
  assert.equal(w.__tunerState.cursorDay, w.__tunerDayIndex(1490, 1, 1), 'needle moved to sibling date');
  assert.notEqual(w.__tunerState.cursorDay, before);
});

test('cursor-sync ignores its own events (loop prevention)', () => {
  const { window: w } = boot();
  const sync = w.__tunerCursorSync;
  sync.lastExternal = null;
  const handled = sync._handle({ sourceWidgetId: sync.selfId, date: { year: 1400, month: 1, day: 1 } });
  assert.equal(handled, false, 'self-sourced event ignored');
  assert.equal(sync.lastExternal, null, 'no external state recorded for self event');
});

// ── Connections (§F) ─────────────────────────────────────────────────
test('connectionsFor returns the arcs touching an event (hover-reveal source)', () => {
  const { window: w } = boot();
  const cf = w.__tunerConnectionsFor;
  const aArcs = cf('ev-a');
  assert.equal(aArcs.length, 2, 'ev-a is in two connections');
  const bArcs = cf('ev-b');
  assert.equal(bArcs.length, 1, 'ev-b is in one connection');
  assert.equal(cf('nope').length, 0, 'unknown event has no connections');
});
