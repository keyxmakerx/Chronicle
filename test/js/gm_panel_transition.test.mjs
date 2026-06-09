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

// C-CAL-CLOSEOUT §7 — the expanded panel body is capped to the sky band so it
// scrolls internally instead of overflowing DOWN onto the grid below.
function plainEl(props) {
  // Like el() but with offsetHeight/scrollHeight as plain (overridable) data,
  // not getters — so the §7 cap math can be driven from the test.
  return Object.assign({
    dataset: {}, style: {}, offsetHeight: 0, scrollHeight: 0, offsetTop: 0,
    setAttribute() {}, getAttribute() { return null; }, removeAttribute() {},
    addEventListener() {}, removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    classList: { toggle() {}, add() {}, remove() {} },
  }, props);
}

function bootCollapse() {
  const toggle = plainEl({ getAttribute: () => 'true' }); // ships expanded
  const header = plainEl({ offsetHeight: 36 });
  const body = plainEl({ scrollHeight: 500, offsetHeight: 140 });
  const region = { clientHeight: 200 };
  const panel = plainEl({
    parentElement: region,
    offsetTop: 12,
    querySelector(sel) {
      return sel === '[data-gm-panel-toggle]' ? toggle
        : sel === '[data-gm-panel-body]' ? body
        : sel === 'header' ? header : null;
    },
  });
  const root = plainEl({ dataset: { calV2CampaignId: 'c', calV2CalendarId: 'cal' } });
  const sandbox = {
    console, matchMedia: () => ({ matches: false }), setTimeout: () => 0, clearTimeout() {},
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
  return { body };
}

test('expanded body is capped to the band height and scrolls (never released to none)', () => {
  const { body } = bootCollapse();
  // band 200 − top 12 − header 36 − 12 breathing = 140 cap; content 500 → 140px + scroll.
  assert.equal(body.style.maxHeight, '140px', 'body should be capped to the band, not grown to fit content');
  assert.equal(body.style.overflowY, 'auto', 'body should scroll internally when content exceeds the band');
  assert.notEqual(body.style.maxHeight, 'none', 'must NOT release to none (that overflowed onto the grid)');
});

test('source seam: the expand path caps to the band instead of releasing to none', () => {
  assert.match(src, /bodyCap/, 'band-cap helper missing');
  assert.doesNotMatch(src, /maxHeight = 'none'/, 'expanded body must not be released to none');
});
