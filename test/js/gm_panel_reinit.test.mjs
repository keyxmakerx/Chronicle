// gm_panel_reinit.test.mjs — C-CAL-GM-PANEL-REWORK C. Boots gm_panel.js in a
// node vm against a panel DOM stub and asserts: (1) it re-inits on boosted nav
// (htmx:afterSettle/htmx:load registered — the QA2 class); (2) the per-element
// guard flag is set so re-init is idempotent; (3) the new "clear world-events"
// button commits {clearEvents:true} through the PUT seam.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'gm_panel.js');
const src = readFileSync(jsPath, 'utf8');

function el(extra) {
  return Object.assign({
    dataset: {}, style: {}, disabled: false,
    _click: null,
    setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
    addEventListener(type, fn) { if (type === 'click') this._click = fn; },
    removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    classList: { toggle() {}, add() {}, remove() {} },
    get scrollHeight() { return 100; }, get offsetHeight() { return 1; },
  }, extra || {});
}

function boot() {
  const clearBtn = el();
  const root = el({ dataset: { calV2CampaignId: 'camp-1', calV2CalendarId: 'cal-1', calV2CsrfToken: 'tok' } });
  const panel = el({
    querySelector(sel) {
      if (sel === '[data-gm-events-clear]') return clearBtn;
      return null; // toggle/body/etc. absent → their wiring is skipped
    },
    querySelectorAll() { return []; },
  });

  const fetchCalls = [];
  const docEvents = {};
  const sandbox = {
    console,
    matchMedia: () => ({ matches: false }),
    setTimeout: () => 0, clearTimeout() {},
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-gm-panel]' ? panel : sel === '[data-cal-v2-root]' ? root : null),
      addEventListener: (type, fn) => { (docEvents[type] = docEvents[type] || []).push(fn); },
      removeEventListener() {},
    },
  };
  sandbox.window = sandbox;
  sandbox.Chronicle = {
    apiFetch: (url, opts) => { fetchCalls.push({ url, opts }); return Promise.resolve({ ok: true, json: () => Promise.resolve({}) }); },
    notify() {},
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'gm_panel.js' });
  return { panel, clearBtn, fetchCalls, docEvents };
}

test('re-inits on boosted nav (htmx:afterSettle + htmx:load registered)', () => {
  const { docEvents } = boot();
  assert.ok(docEvents['htmx:afterSettle'] && docEvents['htmx:afterSettle'].length, 'htmx:afterSettle not registered');
  assert.ok(docEvents['htmx:load'] && docEvents['htmx:load'].length, 'htmx:load not registered');
});

test('per-element guard is set after init (idempotent re-init)', () => {
  const { panel, docEvents } = boot();
  assert.equal(panel.dataset.gmWired, '1', 'gmWired guard not set on init');
  // A second init (e.g. an htmx:afterSettle fire) must early-return on the same
  // panel — calling it must not throw and the guard stays set.
  docEvents['htmx:afterSettle'][0]();
  assert.equal(panel.dataset.gmWired, '1');
});

test('clear-world-events commits {clearEvents:true} via the PUT seam', () => {
  const { clearBtn, fetchCalls } = boot();
  assert.ok(clearBtn._click, 'clear button not wired');
  clearBtn._click();
  assert.equal(fetchCalls.length, 1, 'clear should PUT once');
  assert.match(fetchCalls[0].url, /\/calendar\/world-state/);
  assert.equal(fetchCalls[0].opts.method, 'PUT');
  assert.equal(fetchCalls[0].opts.body.clearEvents, true);
});
