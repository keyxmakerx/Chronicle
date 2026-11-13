// calendar_grid_week.test.mjs — CalDate.occursOn on a calendar where each
// month starts a new week must agree with Event.OccursOn (model.go). The table
// is the same one as TestGridWeekRecurrenceExamples in
// internal/plugins/calendar/recurrence_grid_week_test.go; change both together.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');
const sandbox = { module: { exports: {} }, Chronicle: { register() {} }, console };
vm.createContext(sandbox);
vm.runInContext(src, sandbox);
const { CalDate } = sandbox.module.exports;

function harptos(on) {
  const months = [];
  for (let i = 0; i < 12; i++) {
    months.push({ days: 30 });
    months.push({ days: 1, is_intercalary: true, leap_year_days: i === 11 ? 1 : 0 });
  }
  return { months, weekdays: Array.from({ length: 10 }, () => ({})), leap_year_every: 4, leap_year_offset: 0, month_starts_new_week: on };
}
function sevenDay(on) {
  return {
    months: [{ days: 31 }, { days: 28, leap_year_days: 1 }, { days: 31 }, { days: 30 }],
    weekdays: Array.from({ length: 7 }, () => ({})), leap_year_every: 4, leap_year_offset: 0, month_starts_new_week: on
  };
}
const ev = (type, y, m, d, extra) => Object.assign({ year: y, month: m, day: d, is_recurring: true, recurrence_type: type }, extra);

const H = harptos(true), S = sevenDay(true);
const cases = [
  ['weekly day 1', H, ev('weekly', 1, 1, 1), 1, 1, 1, true],
  ['weekly day 11', H, ev('weekly', 1, 1, 1), 1, 1, 11, true],
  ['weekly day 21', H, ev('weekly', 1, 1, 1), 1, 1, 21, true],
  ['weekly next month day 1', H, ev('weekly', 1, 1, 1), 1, 3, 1, true],
  ['weekly next month day 11', H, ev('weekly', 1, 1, 1), 1, 3, 11, true],
  ['weekly next month day 10 (no)', H, ev('weekly', 1, 1, 1), 1, 3, 10, false],
  ['weekly next month day 30 (no)', H, ev('weekly', 1, 1, 1), 1, 3, 30, false],
  ['weekly never on a festival', H, ev('weekly', 1, 1, 1), 1, 2, 1, false],
  ['biweekly day 11 (no)', H, ev('biweekly', 1, 1, 1), 1, 1, 11, false],
  ['biweekly day 21', H, ev('biweekly', 1, 1, 1), 1, 1, 21, true],
  ['biweekly next month day 11', H, ev('biweekly', 1, 1, 1), 1, 3, 11, true],
  ['biweekly next month day 1 (no)', H, ev('biweekly', 1, 1, 1), 1, 3, 1, false],
  ['custom 3 next month day 1', H, ev('custom', 1, 1, 1, { recurrence_interval: 3 }), 1, 3, 1, true],
  ['custom 3 same month day 11 (no)', H, ev('custom', 1, 1, 1, { recurrence_interval: 3 }), 1, 1, 11, false],
  ['custom 3 across year', H, ev('custom', 1, 1, 1, { recurrence_interval: 3 }), 2, 1, 1, true],
  ['custom 5 across year (no)', H, ev('custom', 1, 1, 1, { recurrence_interval: 5 }), 2, 1, 1, false],
  ['leap year festival day 2 is no weekday', H, ev('weekly', 1, 1, 1), 4, 24, 2, false],
  ['before base', H, ev('weekly', 1, 3, 1), 1, 1, 1, false],
  ['base on festival, own date', H, ev('weekly', 1, 2, 1), 1, 2, 1, true],
  ['base on festival, next month day 1 (no)', H, ev('weekly', 1, 2, 1), 1, 3, 1, false],
  ['base on festival, next year (no)', H, ev('weekly', 1, 2, 1), 2, 2, 1, false],
  ['max occurrences kept', H, ev('weekly', 1, 1, 1, { recurrence_max_occurrences: 3 }), 1, 1, 21, true],
  ['max occurrences exceeded', H, ev('weekly', 1, 1, 1, { recurrence_max_occurrences: 3 }), 1, 3, 1, false],
  ['end date inclusive', H, ev('weekly', 1, 1, 1, { recurrence_end_year: 1, recurrence_end_month: 1, recurrence_end_day: 11 }), 1, 1, 11, true],
  ['after end date', H, ev('weekly', 1, 1, 1, { recurrence_end_year: 1, recurrence_end_month: 1, recurrence_end_day: 11 }), 1, 1, 21, false],
  ['switch off keeps day stride', harptos(false), ev('weekly', 1, 1, 1), 1, 3, 10, true],
  ['switch off, no stride hit', harptos(false), ev('weekly', 1, 1, 1), 1, 3, 1, false],
  ['short row: 31 lands in col 2', S, ev('weekly', 1, 1, 3), 1, 1, 31, true],
  ['short row: col 3 has no day 32', S, ev('weekly', 1, 1, 4), 1, 1, 31, false],
  ['short row: next month col 3 first', S, ev('weekly', 1, 1, 4), 1, 2, 4, true],
  ['short row: next month day 1 for col 0', S, ev('weekly', 1, 1, 1), 1, 2, 1, true],
  ['short row: biweekly skips month boundary rows', S, ev('biweekly', 1, 1, 1), 1, 2, 1, false],
  ['short row: biweekly next month row 1', S, ev('biweekly', 1, 1, 1), 1, 2, 8, true],
  ['switch off seven-day', sevenDay(false), ev('weekly', 1, 1, 1), 1, 2, 5, true]
];

for (const [name, cal, e, y, m, d, want] of cases) {
  test('grid-week occursOn: ' + name, () => {
    assert.equal(CalDate.occursOn(cal, e, y, m, d), want);
  });
}

test('grid-week occursOn: weekly from year -1 stays in column 0 across year 0', () => {
  const e = ev('weekly', -1, 1, 1);
  assert.equal(CalDate.occursOn(H, e, 0, 1, 1), true);
  assert.equal(CalDate.occursOn(H, e, 1, 1, 11), true);
  assert.equal(CalDate.occursOn(H, e, 0, 1, 2), false);
});
