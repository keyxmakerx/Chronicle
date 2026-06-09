// gm_panel_transition.test.mjs — C-CAL-SKY-COMPLETION C. Advancing the day/time
// briefly fades the GM card translucent so the living sky animates through it,
// then restores — GPU-only (opacity), and skipped under prefers-reduced-motion.
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
    dataset: {}, style: {}, disabled: false, _click: null,
    setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
    addEventListener(type, fn) { if (type === 'click') this._click = fn; },
    removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    classList: { toggle() {}, add() {}, remove() {} },
    get scrollHeight() { return 100; }, get offsetHeight() { return 1; },
  }, extra || {});
}

function boot(reduceMotion) {
  const advanceBtn = el({ dataset: { gmDays: '1' } });
  const root = el({ dataset: { calV2CampaignId: 'camp-1', calV2CalendarId: 'cal-1', calV2CsrfToken: 'tok' } });
  const panel = el({
    querySelector() { return null; },
    querySelectorAll(sel) { return sel === '[data-gm-advance]' ? [advanceBtn] : []; },
  });
  const sandbox = {
    console,
    matchMedia: () => ({ matches: !!reduceMotion }),
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
  return { panel, advanceBtn };
}

test('advancing the day fades the card translucent', () => {
  const { panel, advanceBtn } = boot(false);
  assert.ok(advanceBtn._click, 'advance button not wired');
  advanceBtn._click();
  assert.equal(panel.style.opacity, '0.42', 'card should go translucent during the sky transition');
});

test('reduced-motion: no fade (card stays opaque)', () => {
  const { panel, advanceBtn } = boot(true);
  advanceBtn._click();
  assert.notEqual(panel.style.opacity, '0.42', 'reduced-motion must skip the translucent fade');
});
