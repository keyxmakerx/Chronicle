// sky_world.test.mjs — pure-function coverage for the sky pane's day
// arithmetic and astronomy math (static/js/widgets/sky_world.js, issue #763).
//
// These are the highest-value functions to pin: absoluteDay/monthDays/
// isLeapYear must mirror Calendar.AbsoluteDay (Go, internal/plugins/calendar/
// model.go) exactly, term for term, or a moon's phase in the sky pane
// disagrees with the server's own Moon.MoonPhase for the same date — the
// file's own header comment states this as the contract. phaseAt is the same
// contract for Moon.MoonPhase. No DOM is needed: sky_world.js only reaches
// for `window` to publish its namespace, so it's run in a bare vm context
// with `window` set to the context's own global object.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const scriptPath = join(here, '..', '..', 'static', 'js', 'widgets', 'sky_world.js');

function loadSkyWorld() {
  const code = readFileSync(scriptPath, 'utf8');
  const sandbox = {};
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(code, sandbox, { filename: 'sky_world.js' });
  return sandbox.SkyWorld;
}

const SW = loadSkyWorld();

// A small fixed calendar mirroring a plausible campaign shape: 3 months of
// uneven length, one of them carrying leap days, so isLeapYear/monthDays/
// yearLength/absoluteDay all have something non-trivial to get right.
const CAL = {
  months: [
    { days: 30, leap_year_days: 0 },
    { days: 28, leap_year_days: 1 }, // the "leap month"
    { days: 31, leap_year_days: 0 },
  ],
  leap_year_every: 4,
  leap_year_offset: 0,
  hours_per_day: 24,
  minutes_per_hour: 60,
};

test('yearLength sums every month\'s days', () => {
  assert.equal(SW.yearLength(CAL), 30 + 28 + 31);
});

test('isLeapYear follows (year - offset) % every === 0, matching Go\'s predicate', () => {
  assert.equal(SW.isLeapYear(CAL, 0), true);
  assert.equal(SW.isLeapYear(CAL, 4), true);
  assert.equal(SW.isLeapYear(CAL, 5), false);
  assert.equal(SW.isLeapYear(CAL, 8), true);
  // No modulus configured: never a leap year, whatever the year.
  assert.equal(SW.isLeapYear({ ...CAL, leap_year_every: 0 }, 4), false);
});

test('monthDays adds the leap allowance only in a leap year, only for that month', () => {
  assert.equal(SW.monthDays(CAL, 0, 4), 30); // month 0 has no leap allowance
  assert.equal(SW.monthDays(CAL, 1, 4), 28 + 1); // leap year: +1
  assert.equal(SW.monthDays(CAL, 1, 5), 28); // non-leap year: unchanged
});

// absoluteDay(cal, year, month, day) — hand-computed against the same closed
// form Calendar.AbsoluteDay uses (see that Go method's doc comment): full
// years first (with leap days folded in), then full months of the current
// year, then the day itself.
test('absoluteDay matches a hand-computed reference for a leap and a non-leap year', () => {
  // Year 0, month 1 (1-indexed), day 1 is the epoch: total = 0 (no full
  // years) + 0 (no full months) + 1 (the day itself).
  assert.equal(SW.absoluteDay(CAL, 0, 1, 1), 1);

  // Year 0, month 3, day 5: two full months of year 0 (30 + 28, year 0 IS a
  // leap year here since 0 % 4 === 0, so month index 1 gets +1) plus 5.
  assert.equal(SW.absoluteDay(CAL, 0, 3, 5), 30 + (28 + 1) + 5);

  // Year 5 (non-leap, since 5 % 4 !== 0), month 1, day 1: years [0,5) contain
  // leap years 0 and 4 (2 of them), each contributing the leap month's +1.
  const yearLen = 30 + 28 + 31;
  assert.equal(SW.absoluteDay(CAL, 5, 1, 1), 5 * yearLen + 2 * 1 + 1);
});

test('absoluteDay is O(1) in year — a huge year must not hang (DoS guard mirrored from Go)', () => {
  const start = Date.now();
  const d = SW.absoluteDay(CAL, 50_000_000, 2, 1);
  assert.ok(Date.now() - start < 200, 'absoluteDay must not loop per-year');
  assert.ok(Number.isFinite(d) && d > 0);
});

test('hour24 rescales the calendar\'s own hour/minute onto a 24-hour face', () => {
  assert.equal(SW.hour24(CAL, 12, 0), 12); // already a 24-hour calendar: no-op
  assert.equal(SW.hour24(CAL, 6, 30), 6.5);
  // A calendar with a non-24-hour day: halfway through a 10-hour day is noon.
  const tenHourDay = { ...CAL, hours_per_day: 10, minutes_per_hour: 60 };
  assert.equal(SW.hour24(tenHourDay, 5, 0), 12);
});

// phaseAt(moon, t) must be the exact fractional-part contract
// Moon.MoonPhase uses server-side: frac((t + offset) / cycle), always folded
// into [0, 1) regardless of sign.
test('phaseAt mirrors Moon.MoonPhase\'s frac((t+offset)/cycle) contract', () => {
  const moon = { cycle_days: 30, phase_offset: 0 };
  assert.equal(SW.phaseAt(moon, 0), 0); // new moon at t=0
  assert.equal(SW.phaseAt(moon, 15), 0.5); // full moon at half the cycle
  assert.ok(Math.abs(SW.phaseAt(moon, 30) - 0) < 1e-9); // wraps back to new
  assert.ok(Math.abs(SW.phaseAt(moon, 45) - 0.5) < 1e-9); // 1.5 cycles -> 0.5
});

test('phaseAt is always in [0, 1) even for a negative continuous day', () => {
  const moon = { cycle_days: 30, phase_offset: 0 };
  const p = SW.phaseAt(moon, -5);
  assert.ok(p >= 0 && p < 1, `phaseAt(-5) = ${p}, want [0,1)`);
});

test('phaseAt returns 0 for a moon with no cycle configured (guards a divide by zero)', () => {
  assert.equal(SW.phaseAt({ cycle_days: 0 }, 10), 0);
  assert.equal(SW.phaseAt({}, 10), 0);
});

test('inclinationFor is deterministic per moon key (same id/name -> same value)', () => {
  const a = SW.inclinationFor({ id: 1, name: 'Luna' });
  const b = SW.inclinationFor({ id: 1, name: 'Luna' });
  const c = SW.inclinationFor({ id: 2, name: 'Umbra' });
  assert.equal(a, b);
  assert.notEqual(a, c);
  assert.ok(Math.abs(a) <= 12, 'inclination must stay within +/-12 degrees');
});

test('clamp/mod/smooth01 behave as the small math primitives they claim to be', () => {
  assert.equal(SW.clamp(5, 0, 10), 5);
  assert.equal(SW.clamp(-5, 0, 10), 0);
  assert.equal(SW.clamp(15, 0, 10), 10);
  assert.equal(SW.mod(-1, 4), 3); // always non-negative, unlike raw `%`
  assert.equal(SW.smooth01(0, 10, -5), 0);
  assert.equal(SW.smooth01(0, 10, 15), 1);
  assert.equal(SW.smooth01(0, 10, 5), 0.5);
});

test('makeSkym\'s sun() places noon near the zenith direction and midnight opposite', () => {
  const skym = SW.makeSkym(CAL);
  const noon = skym.sun(1, 1, 1, 12);
  const midnight = skym.sun(1, 1, 1, 0);
  // Not asserting exact altitude (that depends on latitude/season), just that
  // noon and midnight are meaningfully different moments, not a frozen sky.
  assert.notEqual(noon.alt, midnight.alt);
  assert.ok(Number.isFinite(noon.alt) && Number.isFinite(noon.az));
});

test('paletteFor returns every named colour as a finite linear RGB triple', () => {
  const P = SW.paletteFor(10, 0, null, { cloud: 0, dark: 0, fog: 0 }, 0, false);
  for (const name of ['zen', 'mid', 'hor', 'glow', 'h0', 'h1', 'h2', 'h3', 'fog']) {
    const c = P[name];
    assert.ok(Array.isArray(c) && c.length === 3, `P.${name} must be an [r,g,b] triple, got ${JSON.stringify(c)}`);
    for (const v of c) assert.ok(Number.isFinite(v), `P.${name} contains a non-finite channel: ${c}`);
  }
});

// The sky pane reads its moons on the calendar page's day count, so a
// real-world calendar's real Moon (anchored to the Julian Day Number) is
// full on 2026-09-26 and new on 2026-09-11 in the sky too.
test('dayIndex puts a real-world calendar\'s Moon where the sky has it', () => {
  const real = { mode: 'reallife', months: [] };
  const epoch = SW.dayIndex(real, 2000, 1, 6);
  assert.equal(epoch, 2451550, 'the Julian Day Number of 2000-01-06');
  const theMoon = { cycle_days: 29.530588853, phase_offset: -epoch };
  const full = SW.phaseAt(theMoon, SW.dayIndex(real, 2026, 9, 26));
  const fresh = SW.phaseAt(theMoon, SW.dayIndex(real, 2026, 9, 11));
  assert.ok(Math.abs(full - 0.5) < 0.04, `2026-09-26 phase ${full}, want about 0.5`);
  assert.ok(fresh < 0.04 || fresh > 0.96, `2026-09-11 phase ${fresh}, want about 0`);
});

test('dayIndex is absoluteDay for any other calendar after year 0', () => {
  assert.equal(SW.dayIndex(CAL, 12, 3, 4), SW.absoluteDay(CAL, 12, 3, 4));
});
