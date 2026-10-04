// calendar_lock_forecast.test.mjs — pins locked days and the player forecast:
// the Lock button's label, a locked day's edit-mode mark, the weather calendar
// treating a locked day as pinned, and the forecast's grid mark and day-card
// wording (including the Director's "Players see" line).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const widgets = path.join(here, '..', '..', 'static', 'js', 'widgets');
const read = (f) => readFileSync(path.join(widgets, f), 'utf8');

const cal = {
  name: 'Test', current_year: 100, current_month: 1, current_day: 1, leap_year_every: 0,
  months: ['Frost', 'Thaw', 'Bloom', 'Sun', 'Harvest', 'Dusk'].map((name, i) => ({ name, days: 30, sort_order: i })),
  weekdays: ['One', 'Two', 'Three', 'Four', 'Five', 'Six'].map((name, i) => ({ name, sort_order: i })),
  moons: [], seasons: [], eras: [],
};

function loadAll() {
  const sandbox = {
    module: { exports: {} }, console,
    Chronicle: { register(n, def) { if (n === 'calendar_view') sandbox.def = def; } },
    document: { activeElement: null, querySelector: () => null, addEventListener() {} },
    window: {},
  };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(read('chronicle_gen.js'), sandbox);
  vm.runInContext(read('calendar_view.js'), sandbox);
  const viewExports = sandbox.module.exports;
  sandbox.module = { exports: {} };
  vm.runInContext(read('calendar_editor.js'), sandbox);
  const editorExports = sandbox.module.exports;
  sandbox.module = { exports: {} };
  vm.runInContext(read('calendar_weather_sheet.js'), sandbox);
  return { sandbox, view: viewExports, editor: editorExports, sheet: sandbox.module.exports };
}

// ---- Lock button ----
test('lock button: Lock, Unlock, or disabled', () => {
  const { editor } = loadAll();
  const { lockState, lockButtonHTML } = editor;
  assert.deepEqual({ ...lockState([null, null]) }, { label: 'Lock', icon: 'fa-lock', disabled: true, unlocking: false });
  assert.equal(lockState([]).disabled, true);
  assert.equal(lockState([{ locked: false }, null]).label, 'Lock');
  assert.equal(lockState([{ locked: true }, { locked: false }]).label, 'Lock');
  const un = lockState([{ locked: true }, null, { locked: true }]);
  assert.equal(un.label, 'Unlock');
  assert.equal(un.icon, 'fa-lock-open');
  assert.match(lockButtonHTML(un), /data-bb="lock"/);
  assert.match(lockButtonHTML(un), /fa-solid fa-lock-open/);
  assert.match(lockButtonHTML(lockState([null])), /disabled/);
});

// ---- Locked day mark ----
test('a locked day shows a monochrome lock with accessible text; others do not', () => {
  const { view } = loadAll();
  const w = { icon: 'rain', source: 'generated', locked: true };
  const html = view.weatherPaintHTML(w, false);
  assert.match(html, /fa-lock lk" aria-hidden="true"/);
  assert.match(html, /<span class="fcsr">locked<\/span>/);
  assert.doesNotMatch(html, /🔒/);
  const plain = view.weatherPaintHTML({ icon: 'rain', source: 'generated' }, false);
  assert.doesNotMatch(plain, /fa-lock/);
  assert.match(plain, /aria-hidden="true"/);
  assert.doesNotMatch(view.weatherPaintHTML(w, false, true), /fa-lock/);
});

// ---- Weather calendar ----
const lockedDay = { year: 100, month: 2, day: 3, source: 'generated', locked: true, preset_id: 'snow', preset_label: 'Snow', icon: 'snow', color: '#e8eef6' };
const paintedDay = { year: 100, month: 2, day: 4, source: 'manual', preset_id: 'snow', preset_label: 'Snow', icon: 'snow', color: '#e8eef6' };

function makePanel(sb, stored) {
  const { Panel } = sb.sheet;
  const p = Object.create(Panel.prototype);
  p.view = { cal, weatherByYear: { 100: stored }, say() {}, _paintMonth() {}, wxPreview: null, eventsOnDay: () => [], mainMoon: () => null };
  p._climate = 'temperate';
  p._continuity = 0.55;
  p._skyOwn = true;
  p._kinds = [];
  p._presets = {};
  sb.sandbox.ChronicleGen.weather.presets([]).forEach((x) => { p._presets[x.id] = x; });
  p.state = { y: 100, m: 2, draft: {}, pins: {}, sel: {}, added: [] };
  return p;
}

test('a locked day stays through a roll and reads as pinned; a painted one stays too', () => {
  const sb = loadAll();
  const p = makePanel(sb, { '2_3': lockedDay, '2_4': paintedDay });
  assert.equal(p._pinned('100_2_3'), true);
  assert.equal(p._pinned('100_2_4'), false);
  const out = p.roll('pinned');
  assert.equal(out.error, undefined, String(out.error));
  assert.equal(out.rolled.length, 28);
  assert.ok(!('100_2_3' in p.state.draft) && !('100_2_4' in p.state.draft));
  const ch = p.changes();
  assert.ok(ch.put.every((d) => d.day !== 3 && d.day !== 4 && d.source === 'generated'));
  assert.equal(ch.lock.length, 0);
});

test('painting a locked day keeps its pin: the write is locked again after', () => {
  const p = makePanel(loadAll(), { '2_3': lockedDay });
  p.paint('rain', ['100_2_3']);
  const ch = p.changes();
  assert.equal(ch.put.length, 1);
  assert.equal(ch.put[0].source, 'manual');
  assert.deepEqual([...ch.lock.map((d) => d.day)], [3]);
  p.togglePins(['100_2_3']);
  assert.equal(p.changes().lock.length, 0);
});

test('the grid ghost is not drawn over a locked day', () => {
  const { sandbox } = loadAll();
  const stub = { weatherByYear: { 100: { '2_3': lockedDay, '2_5': { year: 100, month: 2, day: 5, source: 'generated', icon: 'rain' } } },
    wxPreview: { '100_2_3': { icon: 'clear' }, '100_2_5': { icon: 'clear' } } };
  const mark = sandbox.def._paintMarkHTML;
  assert.doesNotMatch(mark.call(stub, 100, 2, 3, false), /ghost/);
  assert.match(mark.call(stub, 100, 2, 5, false), /ghost/);
});

test('players get the forecast mark only on future days with no reading', () => {
  const { sandbox } = loadAll();
  const d = sandbox.def;
  const stub = { canAuthorDmOnly: false, weatherByYear: { 100: {} }, cal, forecastByDay: { '100_2_9': fc },
    weatherOnDay: d.weatherOnDay, forecastOn: d.forecastOn };
  assert.match(d.forecastMarkOn.call(stub, 100, 2, 9, true), /Forecast: Rain likely/);
  assert.equal(d.forecastMarkOn.call(stub, 100, 2, 9, false), '');
  assert.equal(d.forecastMarkOn.call({ ...stub, canAuthorDmOnly: true }, 100, 2, 9, true), '');
  stub.weatherByYear[100]['2_9'] = { icon: 'clear' };
  assert.equal(d.forecastMarkOn.call(stub, 100, 2, 9, true), '');
});

// ---- Forecast ----
const fc = { year: 100, month: 2, day: 9, lead: 3, confidence: 0.8, icon: 'rain', words: 'Rain likely', temp_low: 8, temp_high: 14, precip_chance: 70 };

test('forecast mark: icon, words, dashed class, faded when unsure, escaped, accessible', () => {
  const { view } = loadAll();
  const sure = view.forecastMarkHTML(fc);
  assert.match(sure, /class="wxm fc"/);
  assert.match(sure, /fa-cloud-rain/);
  assert.match(sure, /<span class="fcsr">Forecast: Rain likely<\/span>/);
  assert.match(view.forecastMarkHTML({ ...fc, confidence: 0.54 }), /wxm fc faded/);
  assert.doesNotMatch(view.forecastMarkHTML({ ...fc, confidence: 0.55 }), /faded/);
  const bad = view.forecastMarkHTML({ ...fc, words: '<img src=x onerror=1>' });
  assert.doesNotMatch(bad, /<img/);
  assert.match(bad, /&lt;img/);
  assert.equal(view.forecastMarkHTML(null), '');
});

test('forecast detail wording: temperatures and chance', () => {
  const { view } = loadAll();
  const t = view.forecastDetailText;
  assert.equal(t(fc), '8°C to 14°C · 70% chance of rain');
  assert.equal(t({ ...fc, temp_low: null, temp_high: null }), '70% chance of rain');
  assert.equal(t({ ...fc, icon: 'snow' }), '8°C to 14°C · 70% chance of snow');
  assert.equal(t({ ...fc, icon: 'storm' }), '8°C to 14°C · 70% chance of storms');
  assert.equal(t({ ...fc, precip_chance: 0 }), '8°C to 14°C');
  assert.equal(t({ ...fc, icon: 'clear', precip_chance: 30 }), '8°C to 14°C · 30% chance of rain');
});

test('forecast day card says it is a forecast; the Director sees what players see', () => {
  const { view } = loadAll();
  const card = view.forecastFactHTML(fc);
  assert.match(card, /<b>Rain likely<\/b>/);
  assert.match(card, /8°C to 14°C · 70% chance of rain/);
  assert.match(card, /A forecast, not a promise\. The real weather shows on the day\./);
  assert.match(view.playersSeeHTML(fc), /Players see: Rain likely/);
  assert.equal(view.playersSeeHTML(null), '');
  assert.match(view.playersSeeHTML({ ...fc, words: '<b>x' }), /&lt;b&gt;x/);
});
