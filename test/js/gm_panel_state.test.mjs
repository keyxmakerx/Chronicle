// gm_panel_state.test.mjs — the r3 single-writer state machine
// (cordinator#33 item 1): collapse/open/sheet transitions must keep ALL state
// surfaces reconciled — data-gm-collapsed, data-gm-sheet, per-sheet [hidden],
// and button aria — through any click sequence, and init must NORMALIZE a
// desynced markup state (the "collapsed yet a sheet shows behind the pill"
// bug class).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'gm_panel.js'), 'utf8');

function el(extra) {
  return Object.assign({
    dataset: {}, style: {}, disabled: false, _click: null, _attrs: {},
    setAttribute(k, v) { this._attrs[k] = String(v); },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(this._attrs, k) ? this._attrs[k] : null; },
    removeAttribute(k) { delete this._attrs[k]; },
    hasAttribute(k) { return Object.prototype.hasOwnProperty.call(this._attrs, k); },
    addEventListener(type, fn) { if (type === 'click') this._click = fn; },
    removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    classList: { toggle() {}, add() {}, remove() {} },
  }, extra || {});
}

// Boot with a panel carrying toggle + pill + two sheet buttons + two sheets.
// startAttrs lets a test begin from a DESYNCED state (init must normalize it).
function boot(startAttrs, opts) {
  const toggle = el({ _attrs: {} });
  toggle._attrs['data-gm-panel-toggle'] = '';
  const pill = el();
  const openWeather = el(); openWeather._attrs['data-gm-sheet-open'] = 'weather';
  const openTime = el(); openTime._attrs['data-gm-sheet-open'] = 'time';
  const sheetWeather = el(); sheetWeather._attrs['data-gm-sheet-panel'] = 'weather';
  if (!(opts && opts.weatherSheetOpen)) sheetWeather._attrs.hidden = '';
  const sheetTime = el(); sheetTime._attrs['data-gm-sheet-panel'] = 'time'; sheetTime._attrs.hidden = '';
  const panel = el({
    _attrs: Object.assign({ 'data-gm-collapsed': 'false', 'data-gm-sheet': '' }, startAttrs || {}),
    querySelector(sel) {
      if (sel === '[data-gm-panel-toggle]') return toggle;
      if (sel === '[data-gm-panel-open]') return pill;
      return null;
    },
    querySelectorAll(sel) {
      if (sel === '[data-gm-sheet-open]') return [openWeather, openTime];
      if (sel === '[data-gm-sheet-panel]') return [sheetWeather, sheetTime];
      return [];
    },
  });
  const root = el({ dataset: { calV2CampaignId: 'c', calV2CalendarId: 'cal', calV2CsrfToken: 't' } });
  const sandbox = {
    console,
    matchMedia: () => ({ matches: false }),
    setTimeout: () => 0, clearTimeout() {},
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-gm-panel]' ? panel : sel === '[data-cal-v2-root]' ? root : null),
      addEventListener() {}, removeEventListener() {},
    },
  };
  sandbox.window = sandbox;
  sandbox.Chronicle = { apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }), notify() {} };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'gm_panel.js' });
  return { panel, toggle, pill, openWeather, openTime, sheetWeather, sheetTime };
}

// The reconciliation invariant after ANY transition.
function assertConsistent(h, where) {
  const collapsed = h.panel.getAttribute('data-gm-collapsed') === 'true';
  const sheet = h.panel.getAttribute('data-gm-sheet') || '';
  if (collapsed) assert.equal(sheet, '', where + ': a collapsed console must hold no open sheet');
  assert.equal(!h.sheetWeather.hasAttribute('hidden'), sheet === 'weather', where + ': weather sheet hidden-state matches');
  assert.equal(!h.sheetTime.hasAttribute('hidden'), sheet === 'time', where + ': time sheet hidden-state matches');
  assert.equal(h.openWeather.getAttribute('aria-expanded'), String(sheet === 'weather'), where + ': weather button aria matches');
  assert.equal(h.openTime.getAttribute('aria-expanded'), String(sheet === 'time'), where + ': time button aria matches');
}

test('full interaction walk keeps every state surface reconciled', () => {
  const h = boot();
  assertConsistent(h, 'init');
  h.openWeather._click(); assertConsistent(h, 'open weather');
  assert.equal(h.panel.getAttribute('data-gm-sheet'), 'weather');
  h.openTime._click(); assertConsistent(h, 'switch to time');
  h.toggle._click(); assertConsistent(h, 'collapse with a sheet open');
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'true');
  h.pill._click(); assertConsistent(h, 'reopen from pill');
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'false');
  h.openWeather._click(); h.openWeather._click(); assertConsistent(h, 'sheet toggled closed');
  assert.equal(h.panel.getAttribute('data-gm-sheet'), '');
});

test('init NORMALIZES a desynced collapsed-with-open-sheet state', () => {
  // The reported bug: collapsed pill showing while a sheet is visible behind
  // it. Boot directly INTO that broken markup (sheet un-hidden, root says
  // collapsed+weather) — init's normalize pass must reconcile it.
  const h = boot({ 'data-gm-collapsed': 'true', 'data-gm-sheet': 'weather' }, { weatherSheetOpen: true });
  assertConsistent(h, 'normalized init');
  assert.equal(h.panel.getAttribute('data-gm-sheet'), '', 'collapsed boot must close the sheet');
});
