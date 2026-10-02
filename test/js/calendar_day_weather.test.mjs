// calendar_day_weather.test.mjs — pins how calendar_view.js draws a day's
// weather reading: the cell's corner mark and the day card's line, including
// the faint future-day form only a Director ever receives, a reading's own
// colour (so acid rain reads apart from rain), and escaping of stored text.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'calendar_view.js'), 'utf8');

function load() {
  const sandbox = { module: { exports: {} }, Chronicle: { register: function () {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.module.exports;
}

const rain = { preset_label: 'Rain', icon: 'rain', color: '#5b8fc7', temperature_celsius: 12, wind: { speed_tier: 'light', direction: 'W' } };

test('no reading draws nothing', () => {
  const { weatherMarkHTML, weatherFactHTML } = load();
  assert.equal(weatherMarkHTML(null, false), '');
  assert.equal(weatherFactHTML(null, false), '');
});

test('corner mark uses the glyph, the reading colour and the label', () => {
  const { weatherMarkHTML } = load();
  const html = weatherMarkHTML(rain, false);
  assert.match(html, /class="wxm"/);
  assert.match(html, /fa-cloud-rain/);
  assert.match(html, /color:#5b8fc7/);
  assert.match(html, /title="Rain"/);
});

test('a future day is faint on the grid and tagged on the card', () => {
  const { weatherMarkHTML, weatherFactHTML } = load();
  assert.match(weatherMarkHTML(rain, true), /class="wxm fut"/);
  const card = weatherFactHTML(rain, true);
  assert.match(card, /wxf fut/);
  assert.match(card, /Players see this on the day/);
  assert.doesNotMatch(weatherFactHTML(rain, false), /Players see this/);
});

test('day card line reads label, temperature and wind in words', () => {
  const { weatherFactHTML } = load();
  const card = weatherFactHTML(rain, false);
  assert.match(card, /Rain · 12°C/);
  assert.match(card, /Light wind from the west/);
});

test('supernatural weather keeps its own colour on a shared glyph', () => {
  const { weatherMarkHTML } = load();
  const acid = { preset_label: 'Acid Rain', icon: 'rain', color: '#9acd32' };
  const html = weatherMarkHTML(acid, false);
  assert.match(html, /fa-cloud-rain/);
  assert.match(html, /color:#9acd32/);
});

test('unknown icon falls back, stored text is escaped, bad colour is stripped', () => {
  const { weatherMarkHTML, weatherFactHTML } = load();
  const odd = { preset_label: '<b>x</b>', icon: 'lava', color: 'red;background:url(x)' };
  const mark = weatherMarkHTML(odd, false);
  assert.match(mark, /fa-cloud-sun/);
  assert.doesNotMatch(mark, /<b>/);
  assert.doesNotMatch(mark, /;background/);
  assert.doesNotMatch(weatherFactHTML(odd, false), /<b>/);
});
