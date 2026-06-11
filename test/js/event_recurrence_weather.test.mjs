// event_recurrence_weather.test.mjs — drawer wiring contracts for
// C-CAL-EDITOR-EXPANSION PR2 (recurrence select + weather-on-date). Source-level
// seams on event_grid.js.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'event_grid.js'), 'utf8');

test('is_recurring is derived from the Repeats select on save', () => {
  assert.match(src, /body\.is_recurring = !!body\.recurrence_type/, 'is_recurring must derive from recurrence_type');
  assert.match(src, /delete body\.recurrence_interval/, 'interval is dropped when not recurring');
});

test('the custom-interval input is gated to the custom type', () => {
  assert.match(src, /function syncRecurrenceCustom/, 'syncRecurrenceCustom present');
  assert.match(src, /\[data-recurrence-custom\][\s\S]*?sel\.value !== 'custom'/, 'custom panel shows only for the custom type');
});

test('"set weather for this day" PUTs the additive weatherDate = the event date', () => {
  const start = src.indexOf('function setWeatherForDay');
  assert.ok(start >= 0, 'setWeatherForDay present');
  const b = src.slice(start, start + 1000);
  assert.match(b, /\/calendar\/world-state/, 'PUTs the world-state endpoint (no new route)');
  assert.match(b, /method: 'PUT'/, 'uses PUT');
  assert.match(b, /weatherDate: \{ year: currentEvent\.year, month: currentEvent\.month, day: currentEvent\.day \}/,
    'sends weatherDate = the event\'s own date');
});
