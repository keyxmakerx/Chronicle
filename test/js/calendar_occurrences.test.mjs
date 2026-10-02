// calendar_occurrences.test.mjs — pins that calendar_view.js reads a
// repeating event's dates from the month read's `occurrences` when they are
// there (rule events, skipped and moved dates), and falls back to its own
// recurrence arithmetic only for an event that arrived without them.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load() {
  const sandbox = { module: { exports: {} }, Chronicle: { register() {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.module.exports.CalDate;
}
const CalDate = load();

const cal = {
  months: [{ name: 'One', days: 10 }, { name: 'Two', days: 10 }],
  weekdays: [{ name: 'A' }, { name: 'B' }, { name: 'C' }, { name: 'D' }, { name: 'E' }],
  leap_year_every: 0
};
const at = (e, y, m, d) => CalDate.occursOn(cal, e, y, m, d);
const days = (e, m) => Array.from({ length: 10 }, (_, i) => i + 1).filter((d) => at(e, 1, m, d));

const rule = { id: 'r', year: 1, month: 1, day: 2, is_recurring: true, recurrence_type: 'rule' };

test('a rule event shows on exactly the dates the server listed', () => {
  const e = { ...rule, occurrences: [{ year: 1, month: 1, day: 4 }, { year: 1, month: 1, day: 9 }] };
  assert.deepEqual(days(e, 1), [4, 9]);
  assert.deepEqual(days(e, 2), []);
});

test('the start date is not shown when the rule does not land on it', () => {
  const e = { ...rule, occurrences: [{ year: 1, month: 1, day: 6 }] };
  assert.equal(at(e, 1, 1, 2), false);
});

test('a skipped date is returned, flagged, so editors can strike it through', () => {
  const e = { ...rule, occurrences: [{ year: 1, month: 1, day: 4, skipped: true }, { year: 1, month: 1, day: 9 }] };
  assert.equal(at(e, 1, 1, 4), true);
  assert.equal(CalDate.occurrenceOn(e, 1, 1, 4).skipped, true);
  assert.equal(CalDate.occurrenceOn(e, 1, 1, 9).skipped, undefined);
});

test('a player is not sent a skipped date, so it is not there', () => {
  const e = { ...rule, occurrences: [{ year: 1, month: 1, day: 9 }] };
  assert.equal(at(e, 1, 1, 4), false);
});

test('a moved date shows on the new date only, remembering where it came from', () => {
  const e = { ...rule, occurrences: [{ year: 1, month: 1, day: 7, moved_from: { year: 1, month: 1, day: 4 } }] };
  assert.equal(at(e, 1, 1, 4), false);
  assert.equal(at(e, 1, 1, 7), true);
  assert.deepEqual(CalDate.occurrenceOn(e, 1, 1, 7).moved_from, { year: 1, month: 1, day: 4 });
});

test('a plain weekly event with occurrences also follows them', () => {
  const e = { id: 'w', year: 1, month: 1, day: 1, is_recurring: true, recurrence_type: 'weekly', occurrences: [{ year: 1, month: 1, day: 6 }] };
  assert.deepEqual(days(e, 1), [6]);
});

test('an empty occurrences list means no dates this month, not the start date', () => {
  const e = { ...rule, year: 1, month: 1, day: 2, occurrences: [] };
  assert.equal(CalDate.hasExpansion(e), true);
  assert.deepEqual(days(e, 1), []);
});

test('truncated: what was found, plus the start date, and no invented dates', () => {
  const none = { ...rule, occurrences: [], occurrences_truncated: true };
  assert.deepEqual(days(none, 1), [2]);
  assert.deepEqual(days(none, 2), []);
  const some = { ...rule, occurrences: [{ year: 1, month: 1, day: 5 }], occurrences_truncated: true };
  assert.deepEqual(days(some, 1), [2, 5]);
  // A truncated event whose start date was skipped does not come back on it.
  const skipped = { ...rule, occurrences: [{ year: 1, month: 1, day: 2, skipped: true }], occurrences_truncated: true };
  assert.equal(CalDate.occurrenceOn(skipped, 1, 1, 2).skipped, true);
  // omitted occurrences with the flag set (older server shapes) still keep the event.
  assert.equal(CalDate.hasExpansion({ occurrences_truncated: true }), true);
});

test('an event without occurrences keeps the old arithmetic', () => {
  const weekly = { id: 'w', year: 1, month: 1, day: 1, is_recurring: true, recurrence_type: 'weekly' };
  assert.equal(CalDate.hasExpansion(weekly), false);
  assert.deepEqual(days(weekly, 1), [1, 6]);
  const once = { id: 'o', year: 1, month: 1, day: 3 };
  assert.deepEqual(days(once, 1), [3]);
  // A rule event that arrived bare is its start date only, as before.
  assert.deepEqual(days({ ...rule }, 1), [2]);
});
