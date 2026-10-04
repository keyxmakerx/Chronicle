// calendar_sky_day.test.mjs — the calendar hands its sky the day to draw.
//
// boot.js mounts every calendar on the same widget object, so what one
// calendar remembered about its sky must never stop the next one's fresh
// sky from being given its day. A change of date or time redraws the sky
// even when neither day has events.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load() {
  const defs = {}, days = [];
  function Dock(o) { this.o = o; }
  Dock.prototype.setDay = function (cal) { days.push(this.o.calendarId + ' ' + [cal.current_year, cal.current_month, cal.current_day, cal.current_hour].join('-')); };
  Dock.prototype.destroy = function () {};
  const SkyPane = { Dock };
  const sandbox = {
    module: { exports: {} }, console, Promise, URLSearchParams, Intl, SkyPane,
    window: { location: { search: '' }, SkyPane },
    Chronicle: { register(name, def) { defs[name] = def; }, apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve([]) }) },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return { def: defs.calendar_view, days };
}

// Mounts one calendar's sky on the shared widget object, as init does.
function mount(v, calendarId, cal) {
  const stub = () => ({ hidden: true, querySelector: () => null, querySelectorAll: () => [] });
  const parts = { '#cal5-skywrap': stub(), '#cal5-skybtn': stub() };
  Object.assign(v, {
    el: { querySelector: (s) => parts[s] || null, querySelectorAll: () => [] },
    calEl: { nextElementSibling: null }, calendarId, campaignId: 'c1', cal,
    eventsByMonth: { [cal.current_year + '_' + cal.current_month]: [] },
  });
  v._dockSky();
}

const world = (over) => Object.assign({ name: 'World', mode: 'fantasy', current_year: 1523, current_month: 1, current_day: 1, current_hour: 0, current_minute: 0, months: [{ days: 30 }] }, over);

test('a second calendar mounted on the same widget still gets its day', () => {
  const { def, days } = load();
  const v = Object.create(def);
  mount(v, 'real', world({ mode: 'reallife', current_year: 2026, current_month: 10, current_day: 4 }));
  mount(v, 'world', world());
  assert.deepEqual([...days], ['real 2026-10-4-0', 'world 1523-1-1-0']);
});

test('a new date or hour redraws the sky on days with no events', () => {
  const { def, days } = load();
  const v = Object.create(def);
  mount(v, 'world', world());
  v.cal = world({ current_day: 2 });
  v._skyDay();
  v.cal = world({ current_day: 2, current_hour: 6 });
  v._skyDay();
  v._skyDay();
  assert.deepEqual([...days], ['world 1523-1-1-0', 'world 1523-1-2-0', 'world 1523-1-2-6']);
});
