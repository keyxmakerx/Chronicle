// calendar_math.test.mjs — pins that calendar_view.js's CalDate/MoonMath (a
// hand port of internal/plugins/calendar/model.go's day/leap-year/moon-phase
// math, kept so a recurring event's occurrence days, the weekday grid column
// and moon phases can never disagree with the server) actually agree with the
// Go implementation on a shared fixture calendar.
//
// Every expected number below is copied verbatim from a throwaway `go test`
// run against internal/plugins/calendar's real Calendar/Moon methods on this
// same fixture (the Go function is named next to each one); the throwaway
// test file itself was never committed.
//
// The weekday-index fixture date (year 2) is deliberately BEFORE the
// fixture's first leap year (year 3: leap_year_every=4, leap_year_offset=3):
// Go's WeekdayIndex/OccursOn route every year>0 through the leap-aware
// AbsoluteDay (model.go's absDayIndex, reconciled so recurrence, weekday
// placement and moon phase can't drift apart there), but this file's
// CalDate.weekdayCol still goes through the leap-UNAWARE constLenDayIndex —
// see the "Two independent day counters" comment atop calendar_view.js's
// CalDate module. The two happen to agree before any leap year has elapsed,
// which is what this test exercises; past that point they are a known,
// separate divergence this test makes no claim about.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

// loadCalendarMath mirrors appearance_save_sequence.test.mjs's approach: the
// widget's browser IIFE only needs a no-op Chronicle.register stub (nothing
// here calls init(), so no DOM is required) plus the module.exports guard
// the source itself checks for.
function loadCalendarMath() {
  const sandbox = { module: { exports: {} }, Chronicle: { register: function () {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const CalDate = sandbox.module.exports.CalDate;
  const MoonMath = sandbox.module.exports.MoonMath;
  assert.equal(typeof CalDate, 'object', 'calendar_view.js must export CalDate for this test');
  assert.equal(typeof MoonMath, 'object', 'calendar_view.js must export MoonMath for this test');
  return { CalDate, MoonMath };
}

// The fixture calendar, mirrored field-for-field into the throwaway Go
// test's Calendar{} literal that produced every expected value below.
const cal = {
  leap_year_every: 4,
  leap_year_offset: 3,
  months: [
    { name: 'Firstmonth', days: 30, leap_year_days: 0 },
    { name: 'Secondmonth', days: 28, leap_year_days: 1 },
    { name: 'Thirdmonth', days: 32, leap_year_days: 0 }
  ],
  weekdays: [{ name: 'a' }, { name: 'b' }, { name: 'c' }, { name: 'd' }, { name: 'e' }]
};
const moon = { cycle_days: 29.5, phase_offset: 0 };

test('CalDate.weekdayCol matches Calendar.WeekdayIndex(2, 3, 5) == 3', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.weekdayCol(cal, 2, 3, 5), 3);
});

test('CalDate.monthDays matches Calendar.MonthDays across a leap year', () => {
  const { CalDate } = loadCalendarMath();
  // MonthDays(1, 3) == 29: year 3 is the fixture's leap year, so Secondmonth
  // (28 base + 1 leap_year_days) gains its extra day.
  assert.equal(CalDate.monthDays(cal, 1, 3), 29);
  // MonthDays(1, 2) == 28: year 2 is not a leap year.
  assert.equal(CalDate.monthDays(cal, 1, 2), 28);
});

test('CalDate.absoluteDay matches Calendar.AbsoluteDay(5, 3, 10) == 519, past the leap year', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.absoluteDay(cal, 5, 3, 10), 519);
});

test('MoonMath.phase matches Moon.MoonPhase(519)', () => {
  const { CalDate, MoonMath } = loadCalendarMath();
  const absDay = CalDate.absoluteDay(cal, 5, 3, 10);
  const phase = MoonMath.phase(moon, absDay);
  // Moon.MoonPhase(519) == 0.5932203389830519 (float64); a small epsilon
  // covers any float rounding difference between Go's float64 and JS's
  // double (the same IEEE 754 binary64 type, but not guaranteed
  // bit-identical across independent implementations of the same formula).
  assert.ok(Math.abs(phase - 0.5932203389830519) < 1e-9, `MoonPhase(519) = ${phase}`);
});
