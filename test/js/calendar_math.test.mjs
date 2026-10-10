// calendar_math.test.mjs — pins that calendar_view.js's CalDate/MoonMath (a
// hand port of internal/plugins/calendar/model.go's day/leap-year/moon-phase
// math, kept so a recurring event's occurrence days, the weekday grid column
// and moon phases can never disagree with the server) actually agree with the
// Go implementation on shared fixture calendars — including across an elapsed
// leap year and on a MonthStartsNewWeek calendar with an intercalary month,
// the two cases dayIndex/weekdayCol previously got wrong (see calendar_view.js's
// CalDate module comment: dayIndex now mirrors Calendar.absDayIndex exactly —
// leap-aware for every year > 0 — and weekdayCol mirrors WeekdayIndex's
// MonthStartsNewWeek branch, including its -1 "outside the week cycle"
// signal for an intercalary month).
//
// Every expected number below is copied verbatim from a throwaway `go test`
// run against internal/plugins/calendar's real Calendar/Event/Moon methods on
// these same fixtures (the Go function is named next to each one); the
// throwaway test file itself was never committed.

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

test('CalDate.weekdayCol matches Calendar.WeekdayIndex(15, 2, 10) == 3, several years after the first leap year', () => {
  const { CalDate } = loadCalendarMath();
  // leapYearsBefore(15) == 3 on this fixture (years 3, 7 and 11 have all
  // elapsed) — this is the case the old constLenDayIndex-based weekdayCol
  // got wrong, since it never accounted for any elapsed leap year at all.
  assert.equal(CalDate.weekdayCol(cal, 15, 2, 10), 3);
});

test("CalDate.occursOn matches Event.OccursOn for a weekly recurrence crossing a leap day", () => {
  const { CalDate } = loadCalendarMath();
  const event = { year: 2, month: 1, day: 1, is_recurring: true, recurrence_type: 'weekly' };
  // Event.OccursOn(cal, event, 3, 3, 2) == true: base (2,1,1) to target
  // (3,3,2) is 150 days apart by the leap-aware AbsoluteDay (year 3 is the
  // fixture's own leap year, so Secondmonth gains a day before Thirdmonth is
  // reached) — an exact multiple of the 5-day week. It is only 149 days
  // apart by the leap-unaware linearDayIndex, which the old
  // constLenDayIndex-based occursOn used and would have missed this
  // occurrence entirely (149 % 5 != 0).
  assert.equal(CalDate.occursOn(cal, event, 3, 3, 2), true);
});

// A second fixture: MonthStartsNewWeek, with an intercalary (festival)
// month — mirrored field-for-field into the same throwaway Go test's second
// Calendar{} literal.
const cal2 = {
  month_starts_new_week: true,
  months: [
    { name: 'Regular', days: 30, is_intercalary: false },
    { name: 'Festival', days: 3, is_intercalary: true }
  ],
  weekdays: [{ name: 'a' }, { name: 'b' }, { name: 'c' }, { name: 'd' }, { name: 'e' }]
};

test('CalDate.weekdayCol matches Calendar.WeekdayIndex on a MonthStartsNewWeek calendar', () => {
  const { CalDate } = loadCalendarMath();
  // WeekdayIndex(1,1,1) == 0: day 1 of a month always restarts the week,
  // regardless of any absolute day count.
  assert.equal(CalDate.weekdayCol(cal2, 1, 1, 1), 0);
  // WeekdayIndex(1,1,7) == 1: (day-1) % weekLen, still independent of the
  // absolute day count.
  assert.equal(CalDate.weekdayCol(cal2, 1, 1, 7), 1);
  // WeekdayIndex(1,2,2) == -1: month 2 (Festival) is intercalary, so it sits
  // outside the weekday cycle entirely — _paintMonth clamps this to 0 (the
  // grid's first column), the same convention view_helpers.go's
  // buildMonthGrid uses for the server's own preview grid.
  assert.equal(CalDate.weekdayCol(cal2, 1, 2, 2), -1);
});

// MoonMath.litPath must draw exactly what the server's MoonLitPath draws
// (moon_silhouette.go): these are its outputs for r=6. Before they matched,
// the calendar page lit the wrong side between first quarter and full, and
// between last quarter and new.
test('MoonMath.litPath matches MoonLitPath at every quarter of the cycle', () => {
  const { MoonMath } = loadCalendarMath();
  const want = {
    0.1: 'M0,-6 A6,6 0 0 1 0,6 A4.85,6 0 0 0 0,-6 Z',
    0.3: 'M0,-6 A6,6 0 0 1 0,6 A1.85,6 0 0 1 0,-6 Z',
    0.5: 'M0,-6 A6,6 0 0 0 0,6 A6,6 0 0 0 0,-6 Z',
    0.6: 'M0,-6 A6,6 0 0 0 0,6 A4.85,6 0 0 0 0,-6 Z',
    0.9: 'M0,-6 A6,6 0 0 0 0,6 A4.85,6 0 0 1 0,-6 Z'
  };
  for (const [phase, path] of Object.entries(want)) assert.equal(MoonMath.litPath(Number(phase), 6), path, `phase ${phase}`);
  assert.equal(MoonMath.litPath(0.002, 6), '', 'no sliver at new moon');
});

// A real-world calendar's Moon is anchored to the Julian Day Number, so its
// phase must come through dayIndex, as the calendar page now reads it.
// Real dates: new moon 2026-09-11, full moon 2026-09-26. tracks_real_time
// must be set: it is what actually selects the JDN branch (see below).
test('a real-world calendar reads its moon through dayIndex and matches the sky', () => {
  const { CalDate, MoonMath } = loadCalendarMath();
  const real = { mode: 'reallife', tracks_real_time: true, months: [], weekdays: [] };
  const theMoon = { cycle_days: 29.530588853, phase_offset: -CalDate.gregorianJDN(2000, 1, 6) };
  const at = (y, m, d) => MoonMath.phase(theMoon, CalDate.dayIndex(real, y, m, d));
  assert.equal(MoonMath.name(at(2026, 9, 26)), 'Full Moon');
  assert.equal(MoonMath.name(at(2026, 9, 11)), 'New Moon');
  assert.equal(MoonMath.name(at(2026, 9, 28)), 'Waning Gibbous');
});

// The real-world Gregorian months/weekdays (reallife.go's gregorianMonths/
// gregorianWeekdays), used below to compare a manual real-world calendar
// (tracks_real_time: false) against a tracked one on the same shape — the
// only thing that must differ is which day-counting branch usesRealTime
// selects.
const realWorldMonths = [
  { name: 'January', days: 31, leap_year_days: 0 },
  { name: 'February', days: 28, leap_year_days: 1 },
  { name: 'March', days: 31, leap_year_days: 0 },
  { name: 'April', days: 30, leap_year_days: 0 },
  { name: 'May', days: 31, leap_year_days: 0 },
  { name: 'June', days: 30, leap_year_days: 0 },
  { name: 'July', days: 31, leap_year_days: 0 },
  { name: 'August', days: 31, leap_year_days: 0 },
  { name: 'September', days: 30, leap_year_days: 0 },
  { name: 'October', days: 31, leap_year_days: 0 },
  { name: 'November', days: 30, leap_year_days: 0 },
  { name: 'December', days: 31, leap_year_days: 0 }
];
const realWorldWeekdays = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((name) => ({ name }));
const manualRealWorld = { mode: 'reallife', tracks_real_time: false, months: realWorldMonths, weekdays: realWorldWeekdays };
const trackedRealWorld = { mode: 'reallife', tracks_real_time: true, months: realWorldMonths, weekdays: realWorldWeekdays };

// CalDate.usesRealTime must mirror Calendar.UsesRealTime exactly: Mode ==
// reallife AND TracksRealTime, not mode alone — mode alone reads true for
// every reallife calendar, tracked or manual.
test('CalDate.usesRealTime requires both reallife mode and tracks_real_time', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.usesRealTime(trackedRealWorld), true);
  assert.equal(CalDate.usesRealTime(manualRealWorld), false);
  assert.equal(CalDate.usesRealTime({ mode: 'fantasy', tracks_real_time: true }), false);
});

// The bug's headline symptom: a manual real-world calendar (owner turned off
// "today follows the real date") must keep its stored, leap-unaware February
// even in a real Gregorian leap year — Calendar.MonthDays never applies the
// true 4/100/400 rule unless UsesRealTime() is true.
test('a manual real-world calendar keeps its stored February length in a real leap year', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.monthDays(manualRealWorld, 1, 2024), 28, 'manual: stored length, no 29th');
  assert.equal(CalDate.monthDays(trackedRealWorld, 1, 2024), 29, 'tracked: true Gregorian rule');
});

// The bug's other symptom: a manual real-world calendar places its weekdays
// by the same leap-aware AbsoluteDay every other calendar uses, not the
// Julian Day Number — so it disagrees with the real weekday. Expected
// indices are copied verbatim from a throwaway `go test` run of
// Calendar.WeekdayIndex against this exact fixture (manual: Monday/index 0;
// tracked: the real Sunday/index 6).
test('a manual real-world calendar places a weekday by AbsoluteDay, not the Julian Day Number', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.weekdayCol(manualRealWorld, 2026, 9, 27), 0, 'manual: AbsoluteDay-based');
  assert.equal(CalDate.weekdayCol(trackedRealWorld, 2026, 9, 27), 6, 'tracked: real Sunday via JDN');
  assert.equal(CalDate.dayIndex(manualRealWorld, 2026, 9, 27), 739760);
  assert.equal(CalDate.dayIndex(trackedRealWorld, 2026, 9, 27), 2461311);
});

// A real-world calendar's grid starts on Sunday (Calendar.GridFirstWeekday),
// while weekdayCol keeps Monday first. 1 October 2026 is a Thursday, so it sits
// after Sun Mon Tue Wed, the same 4 blanks buildMonthGrid gives.
test('a real-world calendar grid starts on Sunday', () => {
  const { CalDate } = loadCalendarMath();
  assert.equal(CalDate.gridFirst(trackedRealWorld), 6);
  assert.equal(CalDate.gridWeekdays(trackedRealWorld)[0].name, 'Sunday');
  assert.equal(CalDate.gridWeekdays(trackedRealWorld)[6].name, 'Saturday');
  assert.equal(CalDate.gridLead(trackedRealWorld, 2026, 10, 1), 4);
  assert.equal(CalDate.weekdayCol(trackedRealWorld, 2026, 10, 1), 3, 'weekday math unchanged');
  const fantasy = { mode: 'fantasy', months: realWorldMonths, weekdays: realWorldWeekdays };
  assert.equal(CalDate.gridFirst(fantasy), 0);
  assert.equal(CalDate.gridWeekdays(fantasy)[0].name, 'Monday');
});
