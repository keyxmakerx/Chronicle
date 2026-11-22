// calendar_on_their_day.test.mjs — pins the Director's "waiting for its day"
// rule: it must agree with the server's EffectiveAnnounced and its "strictly
// after today" cut (calendar/model.go, service.go dropUnannouncedFutureEvents),
// or the waiting mark on the month would promise something players never get.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';
import { escapeHtml } from './_escape.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const read = (f) => readFileSync(path.join(widgets, f), 'utf8');

function load() {
  const sandbox = {
    module: { exports: {} }, console,
    Chronicle: { escapeHtml, register() {} },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(read('chronicle_gen.js'), sandbox);
  vm.runInContext(read('calendar_view.js'), sandbox);
  return sandbox.module.exports.CalDate;
}
const CalDate = load();

const cal = {
  name: 'Test', current_year: 100, current_month: 2, current_day: 10, leap_year_every: 0,
  months: ['Frost', 'Thaw', 'Bloom'].map((name, i) => ({ name, days: 30, sort_order: i })),
  weekdays: ['One', 'Two', 'Three'].map((name, i) => ({ name, sort_order: i })),
  event_kinds: [{ id: 1, default_announced: 'on_day' }, { id: 2, default_announced: 'ahead' }, { id: 3, default_announced: '' }],
};
const ev = (over) => Object.assign({ id: 'e', name: 'Fair', year: 100, month: 2, day: 20, visibility: 'everyone' }, over);

test('the effective announce rule is the server one, in the same order', () => {
  const eff = (e) => CalDate.effectiveAnnounced(cal, ev(e));
  assert.equal(eff({}), 'on_day');
  assert.equal(eff({ announced: 'ahead' }), 'ahead');
  assert.equal(eff({ announced: 'on_day', kind_id: 2 }), 'on_day');
  assert.equal(eff({ is_recurring: true, recurrence_type: 'yearly' }), 'ahead');
  assert.equal(eff({ is_recurring: true, recurrence_type: 'weekly' }), 'on_day');
  assert.equal(eff({ announced: 'on_day', is_recurring: true, recurrence_type: 'yearly' }), 'on_day');
  assert.equal(eff({ kind_id: 2 }), 'ahead');
  assert.equal(eff({ kind_id: 1 }), 'on_day');
  assert.equal(eff({ kind_id: 3 }), 'on_day');
  assert.equal(eff({ kind_id: 99 }), 'on_day');
  assert.equal(CalDate.effectiveAnnounced({}, ev({ kind_id: 2 })), 'on_day');
});

test('an event waits only while its day is strictly after today and it is held by "on the day"', () => {
  const waits = (e) => CalDate.waitsForItsDay(cal, ev(e));
  assert.equal(waits({}), true);
  assert.equal(waits({ day: 11 }), true);
  assert.equal(waits({ day: 10 }), false, 'today is its day');
  assert.equal(waits({ day: 9 }), false);
  assert.equal(waits({ month: 3, day: 1 }), true);
  assert.equal(waits({ year: 101, month: 1, day: 1 }), true);
  assert.equal(waits({ year: 99, month: 3, day: 30 }), false);
  assert.equal(waits({ announced: 'ahead' }), false);
  assert.equal(waits({ kind_id: 2 }), false);
  assert.equal(waits({ is_recurring: true, recurrence_type: 'yearly' }), false);
});

test('a hidden event or a draft is not waiting', () => {
  assert.equal(CalDate.waitsForItsDay(cal, ev({ visibility: 'dm_only' })), false);
  assert.equal(CalDate.waitsForItsDay(cal, ev({ _draft: true })), false);
  assert.equal(CalDate.waitsForItsDay(cal, null), false);
});
