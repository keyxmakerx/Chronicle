// calendar_event_roll_unfit.test.mjs — the Roll section shows one dot per
// night in the on-the-night window: filled when rolled, hollow when no table
// line fit. A hollow dot, pressed, reveals that night's reason in one line.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_event_roll.js'), 'utf8');
const sandbox = { Chronicle: { escapeHtml: String }, document: {} };
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(src, sandbox);
const { nightDots, unfitReason } = sandbox.Chronicle.calendarEventRoll;

const label = (d) => 'Tue ' + d.day;
const night = (day, why) => ({ year: 5, month: 2, day, why });

test('one dot per night, in date order, rolled and unfit together', () => {
  const dots = Array.from(nightDots([night(2), night(6)], [night(4, 'The lines wait on winter (it is Two).')], label));
  assert.deepEqual(dots.map((d) => [d.key, d.unfit]), [['5_2_2', false], ['5_2_4', true], ['5_2_6', false]]);
});

test('a hollow dot is named "no line fits" and carries the reason line', () => {
  const [d] = Array.from(nightDots([], [night(4, 'The lines wait on winter (it is Two).')], label));
  assert.equal(d.label, 'Tue 4: no line fits');
  assert.equal(d.reason, 'Tue 4: the lines wait on winter (it is Two).');
});

test('a filled dot is named rolled and has no reason', () => {
  const [d] = Array.from(nightDots([night(2)], [], label));
  assert.equal(d.label, 'Tue 2: rolled');
  assert.equal(d.reason, '');
});

test('a night with no reason still says no line fits', () => {
  assert.equal(unfitReason('Tue 3'), 'Tue 3: no line of the table fits.');
});

test('no nights gives no dots', () => {
  assert.deepEqual(Array.from(nightDots(undefined, undefined, label)), []);
});
