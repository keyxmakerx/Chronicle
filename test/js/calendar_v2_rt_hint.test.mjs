// calendar_v2_rt_hint.test.mjs — C-CAL-UX-PAIR §Fix 2. On a real-time
// (UsesRealTime) calendar, event times render in the calendar's anchor zone
// for every viewer; this pins the pure math behind the client-side
// "your time HH:MM" hint calendar_v2_shell.js paints next to those times —
// the same reuse-seam convention as calendar_permissions.test.mjs (load the
// file in a `window`-only vm sandbox so the DOM driver at the bottom is
// skipped and only window.__calRtHint's pure functions run).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_v2_shell.js');

// FakeIntl delegates every call to the real Intl.DateTimeFormat EXCEPT the
// exact no-argument form (`new Intl.DateTimeFormat()`) — rtBrowserZone's
// probe for "what zone is this browser in" — which real Node has no way to
// fake (it always reports the host's actual zone), so that one form returns
// a stub reporting `browserTimeZone` instead. Every other call (the ones
// zonedWallTimeToUTC/formatInZone make with explicit `timeZone` options)
// goes through untouched, so those still exercise real ICU zone math.
function fakeIntl(browserTimeZone) {
  function DateTimeFormat(locale, opts) {
    if (arguments.length === 0) {
      return { resolvedOptions: () => ({ timeZone: browserTimeZone }) };
    }
    return new Intl.DateTimeFormat(locale, opts);
  }
  return { DateTimeFormat: DateTimeFormat };
}

function load(browserTimeZone) {
  const sandbox = { Intl: browserTimeZone ? fakeIntl(browserTimeZone) : Intl };
  sandbox.window = sandbox; // window defined, document undefined → DOM driver skipped
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'calendar_v2_shell.js' });
  return sandbox.window.__calRtHint;
}

test('window.__calRtHint is exposed with the expected pure-function surface', () => {
  const H = load();
  assert.equal(typeof H.anchorZone, 'function');
  assert.equal(typeof H.browserZone, 'function');
  assert.equal(typeof H.zonedWallTimeToUTC, 'function');
  assert.equal(typeof H.formatInZone, 'function');
  assert.equal(typeof H.eventHint, 'function');
});

test('anchorZone() is empty without a document (non-RT / no live clock)', () => {
  const H = load();
  assert.equal(H.anchorZone(), '');
});

test('zonedWallTimeToUTC + formatInZone round-trip a plain non-DST offset (New York, winter, EST -05:00)', () => {
  const H = load();
  // 2026-01-15 19:00 America/New_York (EST, UTC-5) is 2026-01-16 00:00 UTC.
  const utc = H.zonedWallTimeToUTC(2026, 1, 15, 19, 0, 'America/New_York');
  assert.equal(utc.toISOString(), '2026-01-16T00:00:00.000Z');
  assert.equal(H.formatInZone(utc, 'America/New_York'), '19:00');
  assert.equal(H.formatInZone(utc, 'UTC'), '00:00');
});

test('zonedWallTimeToUTC handles the summer DST offset too (New York, EDT -04:00)', () => {
  const H = load();
  // 2026-07-15 19:00 America/New_York (EDT, UTC-4) is 2026-07-15 23:00 UTC.
  const utc = H.zonedWallTimeToUTC(2026, 7, 15, 19, 0, 'America/New_York');
  assert.equal(utc.toISOString(), '2026-07-15T23:00:00.000Z');
});

test('zonedWallTimeToUTC is correct on both sides of a DST transition boundary', () => {
  const H = load();
  // US DST began 2026-03-08 02:00 -> 03:00 local (spring forward). The day
  // before (still EST, -05:00) and the day after (already EDT, -04:00) must
  // resolve to different UTC offsets for the "same" 19:00 local wall time.
  const before = H.zonedWallTimeToUTC(2026, 3, 7, 19, 0, 'America/New_York');
  const after = H.zonedWallTimeToUTC(2026, 3, 9, 19, 0, 'America/New_York');
  assert.equal(before.toISOString(), '2026-03-08T00:00:00.000Z'); // UTC-5
  assert.equal(after.toISOString(), '2026-03-09T23:00:00.000Z');  // UTC-4
});

test('eventHint: suppressed when the calendar is not real-time (no anchor zone)', () => {
  const H = load();
  const ev = { year: 2026, month: 7, day: 15, start_hour: 19, start_minute: 0, all_day: false };
  assert.equal(H.eventHint(ev, ''), '');
});

test('eventHint: suppressed for all-day events', () => {
  const H = load();
  const ev = { year: 2026, month: 7, day: 15, start_hour: 19, start_minute: 0, all_day: true };
  assert.equal(H.eventHint(ev, 'America/New_York'), '');
});

test('eventHint: suppressed when the event has no start time', () => {
  const H = load();
  const ev = { year: 2026, month: 7, day: 15, start_hour: null, start_minute: null, all_day: false };
  assert.equal(H.eventHint(ev, 'America/New_York'), '');
});

test('eventHint: suppressed when the viewer zone equals the anchor zone (nothing to add)', () => {
  const H = load('America/New_York');
  const ev = { year: 2026, month: 7, day: 15, start_hour: 19, start_minute: 0, all_day: false };
  assert.equal(H.eventHint(ev, 'America/New_York'), '');
});

test('eventHint: renders "HH:MM" in the viewer zone when zones differ (winter, no DST either side)', () => {
  const H = load('Europe/Berlin'); // UTC+1 in January
  const ev = { year: 2026, month: 1, day: 15, start_hour: 19, start_minute: 0, all_day: false };
  // 19:00 America/New_York (EST -05:00) = 00:00 UTC = 01:00 Europe/Berlin (CET +01:00), next day.
  assert.equal(H.eventHint(ev, 'America/New_York'), '01:00');
});

test('eventHint: renders correctly straddling a DST boundary on the viewer side', () => {
  const H = load('Europe/Berlin'); // CET/CEST — DST starts 2026-03-29
  const evBefore = { year: 2026, month: 3, day: 20, start_hour: 12, start_minute: 0, all_day: false };
  const evAfter = { year: 2026, month: 4, day: 5, start_hour: 12, start_minute: 0, all_day: false };
  // 12:00 EST (-05:00, before US DST starts 3/8... both dates here are after
  // US DST start, so New York is EDT -04:00 for both) vs Berlin CET/CEST:
  // before EU DST (3/20): Berlin is CET +01:00 -> 12:00 EDT = 16:00 UTC = 17:00 CET.
  assert.equal(H.eventHint(evBefore, 'America/New_York'), '17:00');
  // after EU DST (4/5): Berlin is CEST +02:00 -> 12:00 EDT = 16:00 UTC = 18:00 CEST.
  assert.equal(H.eventHint(evAfter, 'America/New_York'), '18:00');
});

test('eventHint: defaults start_minute to 0 when null/undefined', () => {
  const H = load('UTC');
  const ev = { year: 2026, month: 6, day: 1, start_hour: 9, start_minute: null, all_day: false };
  assert.equal(H.eventHint(ev, 'America/New_York'), '13:00'); // EDT -04:00 in June
});
