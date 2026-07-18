// calendar_v2_sky_strip.test.mjs — C-CAL-SKY-STRIP + C-SYNC-DATE-BEACON.
// Pins computeSyncChipState, the pure function behind the Calendaria sync
// chip: in-sync / stale / drift / never-synced. Runs the real
// calendar_v2_shell.js source through a minimal vm sandbox (the
// window.__calMoonSim / window.__calSkyFxMeta reuse-seam convention from
// skybox_widget.test.mjs) rather than re-implementing the logic in the
// test, so a change to the production function is what the test actually
// exercises.
//
// The bottom section (bootWithDom + "wireSkyStrip integration" tests) flips
// C-CAL-SKY-STRIP (#545)'s dormant drift coverage to a WIRED pin: #545 could
// only unit-test computeSyncChipState's drift branch by calling it directly
// with hand-picked args, because no caller ever populated
// fmConfirmedDate/chronicleCurrentDate with anything but ''
// (C-SYNC-DATE-BEACON's Step-0 finding). Now that wireSkyStrip fetches the
// served-date beacon and threads it through, these tests drive the actual
// DOM-wiring function end-to-end — fetch mocks in, painted chip out — so a
// regression in the wiring itself (not just the pure function) fails here.

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

// --- fmAppliedDate preference (C-SYNC-APPLIED-BEACON) ---

test('omitting fmAppliedDate entirely is byte-identical to pre-C-SYNC-APPLIED-BEACON behavior (drift from served alone)', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  // No 7th arg at all — every pre-existing caller/test shape.
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '12 Jul 2026', '14 Jul 2026');
  assert.equal(s.status, 'drift');
  assert.equal(s.date, '12 Jul 2026');
});

test('an empty-string fmAppliedDate (no confirm ever landed) falls back to the served date, same as omitting it', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '12 Jul 2026', '14 Jul 2026', '');
  assert.equal(s.status, 'drift');
  assert.equal(s.date, '12 Jul 2026');
});

test('a fresh applied date that matches Chronicle current overrides a differing served date — resolves to in_sync, not drift', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  // served says 12 Jul (stale relative to what actually happened), but the
  // module has since confirmed it applied 14 Jul — applied wins.
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '12 Jul 2026', '14 Jul 2026', '14 Jul 2026');
  assert.equal(s.status, 'in_sync');
});

test('a fresh applied date that DIFFERS from Chronicle current still reads drift, using the applied date not the served one', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '14 Jul 2026', '16 Jul 2026', '12 Jul 2026');
  assert.equal(s.status, 'drift');
  assert.equal(s.date, '12 Jul 2026');
});

test('applied date present but served date empty still drives drift (confirm-before-any-GET case)', () => {
  const now = 1_000_000_000;
  const lastSeen = now - 60 * 1000;
  const s = computeSyncChipState(false, lastSeen, now, 15 * 60 * 1000, '', '14 Jul 2026', '12 Jul 2026');
  assert.equal(s.status, 'drift');
  assert.equal(s.date, '12 Jul 2026');
});

// --- wireSkyStrip integration (C-SYNC-DATE-BEACON: drift wired, not dormant) ---

// makeEl builds a minimal DOM-element stub. `children` maps a querySelector
// selector string to a child stub, so nested lookups (chip -> dot/word/detail)
// resolve without a real DOM.
function makeEl(children) {
  const kids = children || {};
  return {
    dataset: {},
    className: '',
    title: '',
    textContent: '',
    style: {},
    classList: { add() {}, remove() {}, toggle() {} },
    setAttribute() {},
    getAttribute() { return null; },
    addEventListener() {},
    querySelector(sel) { return kids[sel] || null; },
  };
}

// bootWithDom runs the real calendar_v2_shell.js source with just enough DOM
// to reach wireSkyStrip's fetch-and-paint chain: a [data-cal-v2-root], a
// [data-cal-sky-strip] carrying campaignId/currentDate, and the chip's child
// nodes. `responses` supplies the two fetch bodies (presence, beacon) or an
// Error to simulate a failed fetch; apiCalls records every URL requested so
// tests can assert the beacon endpoint was actually hit.
async function bootWithDom(responses, currentDate) {
  const dot = makeEl();
  const word = makeEl();
  const detail = makeEl();
  const chip = makeEl({
    '[data-cal-sky-sync-dot]': dot,
    '[data-cal-sky-sync-word]': word,
    '[data-cal-sky-sync-detail]': detail,
  });
  const toggle = makeEl();
  const pane = makeEl();
  const chevron = makeEl();
  const syncNowBtn = makeEl();
  const syncHelp = makeEl();
  const strip = makeEl({
    '[data-cal-sky-strip-toggle]': toggle,
    '[data-cal-sky-strip-pane]': pane,
    '[data-cal-sky-chevron]': chevron,
    '[data-cal-sky-sync-now]': syncNowBtn,
    '[data-cal-sky-sync-help]': syncHelp,
    '[data-cal-sky-sync-chip]': chip,
  });
  strip.dataset.campaignId = 'camp-1';
  strip.dataset.calCurrentDate = currentDate;

  const rootEl = makeEl();
  rootEl.dataset.calV2CampaignId = 'camp-1';

  const apiCalls = [];
  const apiFetch = (url) => {
    apiCalls.push(url);
    const isBeacon = url.indexOf('calendar-sync-beacon') !== -1;
    const resp = isBeacon ? responses.beacon : responses.presence;
    if (resp instanceof Error) return Promise.reject(resp);
    return Promise.resolve({ ok: true, json: () => Promise.resolve(resp) });
  };

  const store = {};
  const localStorage = {
    getItem: (k) => (Object.prototype.hasOwnProperty.call(store, k) ? store[k] : null),
    setItem: (k, v) => { store[k] = v; },
  };

  const doc = {
    readyState: 'complete',
    querySelector(sel) {
      if (sel === '[data-cal-v2-root]') return rootEl;
      if (sel === '[data-cal-sky-strip]') return strip;
      return null;
    },
    querySelectorAll: () => [],
    getElementById: () => null,
    contains: () => true,
    addEventListener: () => {},
  };
  const sandbox = {
    console,
    document: doc,
    window: null,
    Chronicle: { apiFetch },
    localStorage,
    matchMedia: () => ({ matches: false }),
    setTimeout, clearTimeout, setInterval, clearInterval,
    Date,
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(shellSrc, sandbox, { filename: 'calendar_v2_shell.js' });

  // boot() runs synchronously (readyState 'complete'), but the fetch chain
  // inside wireSkyStrip resolves over microtasks/a macrotask tick — flush both.
  await new Promise((r) => setTimeout(r, 0));
  await new Promise((r) => setTimeout(r, 0));

  return { chip, word, detail, dot, apiCalls };
}

const FRESH_PRESENCE = { never_seen: false, last_seen: new Date(Date.now() - 60 * 1000).toISOString() };

test('wireSkyStrip: a fresh beacon with a differing date paints the chip drift', async () => {
  const { word, apiCalls } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: { last_served_date: '2026-07-12', last_served_at: new Date(Date.now() - 60 * 1000).toISOString() },
  }, '2026-07-14');

  assert.ok(apiCalls.some((u) => u.indexOf('foundry-presence') !== -1), 'presence endpoint was fetched');
  assert.ok(apiCalls.some((u) => u.indexOf('calendar-sync-beacon') !== -1), 'beacon endpoint was fetched');
  assert.equal(word.textContent, 'Drift');
});

test('wireSkyStrip: a fresh beacon matching the current date paints in_sync, not drift', async () => {
  const { word } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: { last_served_date: '2026-07-14', last_served_at: new Date(Date.now() - 60 * 1000).toISOString() },
  }, '2026-07-14');

  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: a STALE beacon (older than the freshness window) is ignored even though the date differs', async () => {
  const staleAt = new Date(Date.now() - 20 * 60 * 1000).toISOString(); // 20 min > 15 min freshness window
  const { word } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: { last_served_date: '2026-07-01', last_served_at: staleAt },
  }, '2026-07-14');

  // A stale beacon can't be trusted, so it must NOT drive drift — falls
  // back to presence-only in_sync (the module IS connected and recent).
  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: no beacon ever recorded (empty response) does not crash and reads in_sync', async () => {
  const { word } = await bootWithDom({ presence: FRESH_PRESENCE, beacon: {} }, '2026-07-14');
  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: a failed beacon fetch degrades independently — presence still paints the chip', async () => {
  const { word } = await bootWithDom({ presence: FRESH_PRESENCE, beacon: new Error('network error') }, '2026-07-14');
  // The beacon fetch's own .catch(() => ({})) swallows the failure before
  // Promise.all, so presence alone still determines the state.
  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: a failed presence fetch falls back to never_synced (whole-chip failure, unchanged from #545)', async () => {
  const { word } = await bootWithDom({ presence: new Error('network error'), beacon: {} }, '2026-07-14');
  assert.equal(word.textContent, 'No module');
});

// --- applied-date beacon (C-SYNC-APPLIED-BEACON) ---

test('wireSkyStrip: a fresh applied date overrides a differing served date — reads in_sync when applied matches current', async () => {
  const { word } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: {
      last_served_date: '2026-07-12', last_served_at: new Date(Date.now() - 60 * 1000).toISOString(),
      last_applied_date: '2026-07-14', last_applied_at: new Date(Date.now() - 60 * 1000).toISOString(),
    },
  }, '2026-07-14');

  // Served alone would read drift (12th vs 14th); a fresh confirmed applied
  // date of the 14th proves the module actually caught up — in_sync wins.
  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: a fresh applied date drives drift using its own date, not the served date', async () => {
  const { word, detail } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: {
      last_served_date: '2026-07-14', last_served_at: new Date(Date.now() - 60 * 1000).toISOString(),
      last_applied_date: '2026-07-10', last_applied_at: new Date(Date.now() - 60 * 1000).toISOString(),
    },
  }, '2026-07-14');

  assert.equal(word.textContent, 'Drift');
  assert.ok(detail.textContent.indexOf('2026-07-10') !== -1, 'detail shows the APPLIED date, not the served date');
});

test('wireSkyStrip: a STALE applied date (older than the freshness window) is ignored, falling back to the served date', async () => {
  const staleAt = new Date(Date.now() - 20 * 60 * 1000).toISOString(); // 20 min > 15 min freshness window
  const { word } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: {
      last_served_date: '2026-07-14', last_served_at: new Date(Date.now() - 60 * 1000).toISOString(),
      last_applied_date: '2026-07-01', last_applied_at: staleAt,
    },
  }, '2026-07-14');

  // The stale applied date can't be trusted, so served (fresh, matching) wins.
  assert.equal(word.textContent, 'In sync');
});

test('wireSkyStrip: graceful fallback — a beacon response with no applied fields at all (older module, never confirms) behaves byte-identical to pre-C-SYNC-APPLIED-BEACON', async () => {
  const { word } = await bootWithDom({
    presence: FRESH_PRESENCE,
    beacon: { last_served_date: '2026-07-12', last_served_at: new Date(Date.now() - 60 * 1000).toISOString() },
  }, '2026-07-14');

  assert.equal(word.textContent, 'Drift');
});
