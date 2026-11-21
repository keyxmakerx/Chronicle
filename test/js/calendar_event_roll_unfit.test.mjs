// calendar_event_roll_unfit.test.mjs — the Roll section tells the Director
// which nights the roll on the night left unrolled because no table line
// fit, in one quiet sentence each, and counts the rest past a few.

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
const { unfitNotes } = sandbox.Chronicle.calendarEventRoll;

const text = (u) => u.day + ' Frostmoon';
const night = (day, why) => ({ year: 5, month: 2, day, why });

test('each unfit night gets one sentence with the server\'s reason', () => {
  const notes = Array.from(unfitNotes([night(3, 'The lines wait on the moon.'), night(9, 'The lines wait on the season (it is autumn).')], text, 5));
  assert.deepEqual(notes, [
    '3 Frostmoon was left unrolled. The lines wait on the moon.',
    '9 Frostmoon was left unrolled. The lines wait on the season (it is autumn).',
  ]);
});

test('a night with no reason still says no line fits', () => {
  assert.deepEqual(Array.from(unfitNotes([night(3)], text, 5)), ['3 Frostmoon was left unrolled. No line of the table fits.']);
});

test('nights past the limit are counted, not listed', () => {
  const many = Array.from({ length: 8 }, (_, i) => night(i + 1, 'Why.'));
  const notes = Array.from(unfitNotes(many, text, 5));
  assert.equal(notes.length, 6);
  assert.equal(notes[5], '3 more nights were left unrolled the same way.');
  assert.equal(Array.from(unfitNotes(many.slice(0, 6), text, 5))[5], '1 more night was left unrolled the same way.');
});

test('no unfit nights says nothing', () => {
  assert.deepEqual(Array.from(unfitNotes(undefined, text, 5)), []);
  assert.deepEqual(Array.from(unfitNotes([], text, 5)), []);
});
