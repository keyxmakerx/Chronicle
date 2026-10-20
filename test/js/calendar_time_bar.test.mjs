// calendar_time_bar.test.mjs — the sky's time bar looks privately at a
// moment and only Set time writes it. Steps carry across days, a look at
// the world's own now ends the look, and the die is as big as the day.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load() {
  const defs = {};
  const sandbox = {
    module: { exports: {} }, console, Promise, URLSearchParams, Intl, Math,
    window: { location: { search: '' } },
    Chronicle: { register(name, def) { defs[name] = def; }, apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve([]) }) },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return defs.calendar_view;
}

const world = (over) => Object.assign({
  name: 'World', mode: 'fantasy', current_year: 100, current_month: 1, current_day: 28, current_hour: 20, current_minute: 0,
  hours_per_day: 24, minutes_per_hour: 60, months: [{ name: 'Deep', days: 28 }, { name: 'Thaw', days: 28 }],
}, over);

function view(cal) {
  const v = Object.create(load());
  const drawn = [];
  Object.assign(v, {
    cal, el: { querySelector: () => null, querySelectorAll: () => [] },
    eventsByMonth: {}, fetchMonth: () => Promise.resolve(), _announce(msg) { drawn.push(msg); },
    _skyDay() { drawn.push(this._skyPrev ? [this._skyPrev.y, this._skyPrev.m, this._skyPrev.d, this._skyPrev.h, this._skyPrev.min].join('-') : 'now'); },
  });
  return { v, drawn };
}

test('steps carry across the end of a month', () => {
  const tests = [
    { name: '+1 hour', by: 60, want: '100-1-28-21-0' },
    { name: 'long rest into the next month', by: 480, want: '100-2-1-4-0' },
    { name: '+1 day', by: 1440, want: '100-2-1-20-0' },
    { name: 'back across midnight', by: -21 * 60, want: '100-1-27-23-0' },
  ];
  for (const tt of tests) {
    const { v, drawn } = view(world());
    v._lookStep(tt.by);
    assert.equal(drawn.at(-1), tt.want, tt.name);
  }
});

test('looking at the world\'s own now ends the look', () => {
  const { v, drawn } = view(world());
  v._lookStep(60);
  v._lookStep(-60);
  assert.deepEqual(drawn, ['100-1-28-21-0', 'now']);
  assert.equal(v._skyPrev, null);
});

test('a look never moves the world\'s today', () => {
  const { v } = view(world());
  v._lookStep(3000);
  v._lookMinute(5);
  assert.equal(v.cal.current_day, 28);
  assert.equal(v.cal.current_hour, 20);
});

test('the die rolls any hour of the calendar\'s own day', () => {
  for (const hpd of [24, 10]) {
    const { v } = view(world({ hours_per_day: hpd }));
    const seen = new Set();
    for (let i = 0; i < 400; i++) { v._rollHour(); seen.add(v._lookAt().h); }
    assert.equal(seen.size, hpd, 'every hour from 0 to ' + (hpd - 1));
    assert.ok([...seen].every((h) => h >= 0 && h < hpd));
  }
});

test('the gap reads in days and hours, ahead or back', () => {
  const { v } = view(world());
  assert.equal(v._lookGapWords({ y: 100, m: 2, d: 2, h: 23, min: 0 }), '2 days 3 hours ahead');
  assert.equal(v._lookGapWords({ y: 100, m: 1, d: 28, h: 19, min: 30 }), '30 min back');
  assert.equal(v._lookGapWords({ y: 100, m: 1, d: 27, h: 20, min: 0 }), '1 day back');
});

test('clock words fit the calendar\'s day', () => {
  assert.equal(view(world()).v._clockWords(19, 0), '7pm');
  assert.equal(view(world({ hours_per_day: 10 })).v._clockWords(7, 5), 'Hour 7:05');
});
