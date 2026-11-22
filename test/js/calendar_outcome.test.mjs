// calendar_outcome.test.mjs — the sheet's pure parts: the heading, and
// the writes Done and Undo send for each answer, worked out before anything
// is sent. The sheet itself runs in calendar_outcome_browser.test.mjs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = (f) => readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', f), 'utf8');

function load() {
  const sandbox = { module: { exports: {} }, Chronicle: { register: function () {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(read('calendar_view.js'), sandbox);
  const CalDate = sandbox.module.exports.CalDate;
  delete sandbox.module;
  vm.runInContext(read('calendar_outcome.js'), sandbox);
  return { CalDate, P: sandbox.Chronicle.calendarOutcome };
}

const { CalDate, P } = load();
const cal = {
  mode: 'fantasy', current_year: 100,
  months: ['Firstfrost', 'Deepwinter'].map((name) => ({ name, days: 30 })),
  weekdays: ['Hearthday', 'Forgeday', 'Veilday', 'Wayday', 'Halfday', 'Hushday', 'Restday'].map((name) => ({ name })),
};
const d = (y, m, dd) => ({ y, m, d: dd });

test('the heading names the days that passed', () => {
  const cases = [
    [d(100, 1, 11), d(100, 1, 12), 'Firstfrost 11 has passed'],
    [d(100, 1, 9), d(100, 1, 12), 'Firstfrost 9 to 11 have passed'],
    [d(100, 1, 29), d(100, 2, 3), 'Firstfrost 29 to Deepwinter 2 have passed'],
    [d(99, 2, 29), d(100, 1, 2), 'Deepwinter 29, 99 to Firstfrost 1 have passed'],
  ];
  for (const [from, to, want] of cases) assert.equal(P.heading(cal, CalDate, from, to), want);
});

test('each answer becomes the write Done sends and the write Undo sends', () => {
  const item = (ev) => ({ key: ev.id, ev, date: d(ev.year, ev.month, ev.day) });
  const one = item({ id: 'a', year: 100, month: 1, day: 10, outcome: 'old' });
  const span = item({ id: 's', year: 100, month: 1, day: 28, end_year: 100, end_month: 1, end_day: 30 });
  const rep = item({ id: 'r', year: 100, month: 1, day: 11, repeating: true });
  const cases = [
    { name: 'unanswered reads as happened, the line left alone', items: [one], answers: {},
      want: { update: [{ id: 'a', changes: { status: 'happened' } }], undo: [{ id: 'a', changes: { status: 'planned', outcome: 'old' } }], occ: [] } },
    { name: 'a new line', items: [one], answers: { a: { choice: 'happened', line: ' Fine ' } },
      want: { update: [{ id: 'a', changes: { status: 'happened', outcome: 'Fine' } }], undo: [{ id: 'a', changes: { status: 'planned', outcome: 'old' } }], occ: [] } },
    { name: 'the same line is not resent', items: [one], answers: { a: { choice: 'didnt', line: 'old' } },
      want: { update: [{ id: 'a', changes: { status: 'called_off' } }], undo: [{ id: 'a', changes: { status: 'planned', outcome: 'old' } }], occ: [] } },
    { name: 'a span moves whole into the next month', items: [span], answers: { s: { choice: 'moved', to: d(100, 1, 30) } },
      want: { update: [{ id: 's', changes: { status: 'moved', year: 100, month: 1, day: 30, end_year: 100, end_month: 2, end_day: 2 } }],
        undo: [{ id: 's', changes: { status: 'planned', outcome: null, year: 100, month: 1, day: 28, end_year: 100, end_month: 1, end_day: 30 } }], occ: [] } },
    { name: 'moved onto its own day is no move', items: [span], answers: { s: { choice: 'moved', to: d(100, 1, 28) } }, want: { update: [], undo: [], occ: [] } },
    { name: 'a repeating date skipped', items: [rep], answers: { r: { choice: 'didnt' } }, want: { update: [], undo: [], occ: [{ id: 'r', date: d(100, 1, 11), body: { action: 'skip' } }] } },
    { name: 'a repeating date moved', items: [rep], answers: { r: { choice: 'moved', to: d(100, 1, 13) } },
      want: { update: [], undo: [], occ: [{ id: 'r', date: d(100, 1, 11), body: { action: 'move', year: 100, month: 1, day: 13 } }] } },
    { name: 'a repeating date that happened sends nothing', items: [rep], answers: {}, want: { update: [], undo: [], occ: [] } },
  ];
  for (const c of cases) {
    assert.deepEqual(JSON.parse(JSON.stringify(P.requests(c.items, c.answers, cal, CalDate))), c.want, c.name);
  }
});
