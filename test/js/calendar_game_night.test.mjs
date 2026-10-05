// calendar_game_night.test.mjs — pins the game night editor's pure part:
// the next nights it previews step exactly as the server's
// computeNextOccurrence does, the form is checked before it is sent, a new
// night sends the New Session form's fields, and a change sends only what
// changed (the sessions update is partial: absent keeps, null clears).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_game_night.js'), 'utf8');

function load() {
  const sandbox = { window: {}, console, Intl, Date };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.Chronicle.calendarGameNight;
}

const plain = (v) => JSON.parse(JSON.stringify(v));

test('the next nights step as the server steps them', () => {
  const G = load();
  const cases = [
    ['one night', ['2026-10-08', '', 0, '', 5, ''], ['2026-10-08']],
    ['every week', ['2026-10-08', 'weekly', 0, '', 3, ''], ['2026-10-08', '2026-10-15', '2026-10-22']],
    ['every other week', ['2026-10-08', 'biweekly', 0, '', 3, ''], ['2026-10-08', '2026-10-22', '2026-11-05']],
    ['every three weeks', ['2026-10-08', 'custom', 3, '', 3, ''], ['2026-10-08', '2026-10-29', '2026-11-19']],
    ['every month rolls over like Go AddDate', ['2026-01-31', 'monthly', 0, '', 3, ''], ['2026-01-31', '2026-03-03', '2026-04-03']],
    ['stops at the last night', ['2026-10-08', 'weekly', 0, '2026-10-20', 5, ''], ['2026-10-08', '2026-10-15']],
    ['from a later day', ['2026-10-08', 'biweekly', 0, '', 2, '2026-10-10'], ['2026-10-22', '2026-11-05']],
    ['not a date', ['soon', 'weekly', 0, '', 3, ''], []],
  ];
  for (const [name, args, want] of cases) assert.deepEqual(plain(G.nextNights(...args)), want, name);
});

test('the form is checked before it is sent', () => {
  const G = load();
  const ok = { name: 'Game night', date: '2026-10-08', time: '19:00', repeat: '', every: 3, end: 'never', until: '', summary: '' };
  const cases = [
    ['fine', {}, ''],
    ['no name', { name: '  ' }, 'Give the game night a name.'],
    ['no day', { date: '' }, 'Pick the day it starts.'],
    ['bad time', { time: '7pm' }, 'Pick a start time.'],
    ['no time is fine', { time: '' }, ''],
    ['too few weeks', { repeat: 'custom', every: 1 }, 'Repeat every 2 to 52 weeks.'],
    ['end before start', { repeat: 'weekly', end: 'until', until: '2026-10-01' }, 'The last night can’t be before the first.'],
    ['end with no date', { repeat: 'weekly', end: 'until', until: '' }, 'Pick the last day it repeats.'],
    ['long note', { summary: 'x'.repeat(501) }, 'Keep the note under 500 characters.'],
  ];
  for (const [name, over, want] of cases) assert.equal(G.problem({ ...ok, ...over }), want, name);
});

test('a new night sends the New Session form’s fields', () => {
  const G = load();
  const s = { name: ' Ashen Road ', date: '2026-10-08', time: '19:00', repeat: 'custom', every: 3, end: 'until', until: '2026-12-31', summary: ' Bring dice ' };
  assert.deepEqual(plain(G.createFields(s, 'America/Chicago')), {
    name: 'Ashen Road', scheduled_date: '2026-10-08', scheduled_time: '19:00', scheduled_tz: 'America/Chicago',
    summary: 'Bring dice', is_recurring: '1', recurrence_type: 'custom', recurrence_interval: '3', recurrence_end_date: '2026-12-31',
  });
  assert.deepEqual(plain(G.createFields({ ...s, repeat: '', time: '', summary: '' }, 'America/Chicago')), {
    name: 'Ashen Road', scheduled_date: '2026-10-08',
  }, 'a one-off with no time sends no zone and no repeat');
});

test('a change sends only what changed', () => {
  const G = load();
  const night = { sessionId: 's1', name: 'Ashen Road', date: '2026-10-22', seriesDate: '2026-10-08', time: '19:00', tz: 'America/Chicago',
    recurring: true, recurrenceType: 'weekly', summary: 'Bring dice' };
  const before = G.stateFor(night);
  assert.equal(before.date, '2026-10-08', 'a series is changed from its first night');
  assert.equal(before.repeat, 'weekly');
  assert.deepEqual(plain(G.updateBody(before, { ...before }, 'America/Chicago')), {}, 'nothing changed, nothing sent');
  assert.deepEqual(plain(G.updateBody(before, { ...before, repeat: 'biweekly' }, 'America/Chicago')), {
    is_recurring: true, recurrence_type: 'biweekly',
  }, 'every other week');
  assert.deepEqual(plain(G.updateBody(before, { ...before, repeat: 'custom', every: 3 }, 'America/Chicago')), {
    is_recurring: true, recurrence_type: 'custom', recurrence_interval: 3,
  });
  assert.deepEqual(plain(G.updateBody(before, { ...before, repeat: '' }, 'America/Chicago')), {
    is_recurring: false, recurrence_type: null,
  }, 'stop repeating');
  assert.deepEqual(plain(G.updateBody(before, { ...before, end: 'until', until: '2026-12-31' }, 'America/Chicago')), {
    recurrence_end_date: '2026-12-31',
  });
  assert.deepEqual(plain(G.updateBody(before, { ...before, time: '20:00', summary: '' }, 'America/Chicago')), {
    summary: null, scheduled_time: '20:00', scheduled_tz: 'America/Chicago',
  }, 'a cleared note is null; a new time carries its zone');
  const until = G.stateFor({ ...night, recurrenceEndDate: '2026-12-31' });
  assert.deepEqual(plain(G.updateBody(until, { ...until, end: 'never' }, '')), { recurrence_end_date: null }, 'never ends clears the end');
});
