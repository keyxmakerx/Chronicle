// gm_panel_transition.test.mjs — the GM console's translucent-transition +
// containment contracts (C-CAL-WORLDSTATE-GM-OVERHAUL; supersedes the #438
// opacity flash and the #441 in-band body cap):
//   1. Every world mutation sets data-gm-transition on the console root; the
//      CSS owns the fade (opacity 0.16) so the sky animates through the card.
//   2. prefers-reduced-motion neutralizes the fade IN CSS (opacity stays 1) —
//      the JS stays branch-free.
//   3. The card itself is height-capped + internally scrollable in CSS (it
//      may float over the grid while open, but never grows unbounded), and
//      collapses to a pill.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'gm_panel.js');
const cssPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'css', 'gm_panel.css');
const src = readFileSync(jsPath, 'utf8');
const css = readFileSync(cssPath, 'utf8');

function el(extra) {
  return Object.assign({
    dataset: {}, style: {}, disabled: false, _click: null, _attrs: {},
    setAttribute(k, v) { this._attrs[k] = v; },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(this._attrs, k) ? this._attrs[k] : null; },
    removeAttribute(k) { delete this._attrs[k]; },
    addEventListener(type, fn) { if (type === 'click') this._click = fn; },
    removeEventListener() {},
    querySelector() { return null; }, querySelectorAll() { return []; },
    classList: { toggle() {}, add() {}, remove() {} },
  }, extra || {});
}

function boot() {
  const advanceBtn = el({ dataset: { gmDays: '1' } });
  const root = el({ dataset: { calV2CampaignId: 'camp-1', calV2CalendarId: 'cal-1', calV2CsrfToken: 'tok' } });
  const panel = el({
    querySelector() { return null; },
    querySelectorAll(sel) { return sel === '[data-gm-advance]' ? [advanceBtn] : []; },
  });
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
  return { panel, advanceBtn };
}

test('advancing the day marks the console for the translucent transition', () => {
  const { panel, advanceBtn } = boot();
  assert.ok(advanceBtn._click, 'advance button not wired');
  advanceBtn._click();
  assert.equal(panel.getAttribute('data-gm-transition'), 'true',
    'world mutations must set data-gm-transition (the CSS fades the card so the sky shows through)');
});

test('the transition fade lives in CSS and reduced-motion neutralizes it', () => {
  assert.match(css, /\[data-gm-transition="true"\] \.gm-console__card\s*\{\s*opacity: 0\.1/,
    'transition state must fade the card to near-transparent');
  assert.match(css, /prefers-reduced-motion/, 'reduced-motion block missing');
  assert.match(css, /prefers-reduced-motion[^}]*\{[\s\S]*?\[data-gm-transition="true"\] \.gm-console__card \{ opacity: 1; \}/,
    'reduced-motion must keep the card fully opaque (no fade)');
});

test('the console card is height-capped + internally scrollable (never unbounded)', () => {
  assert.match(css, /\.gm-console__card\s*\{[\s\S]*?max-height: min\(/, 'card must be height-capped');
  assert.match(css, /\.gm-console__body\s*\{[\s\S]*?overflow-y: auto/, 'body must scroll internally');
  assert.doesNotMatch(src, /maxHeight = 'none'/, 'JS must not release the body to an unbounded height');
});

test('the console collapses to a pill (data-gm-collapsed contract)', () => {
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__pill \{ display: inline-flex; \}/,
    'collapsed state must show the pill');
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__card\s*\{[\s\S]*?pointer-events: none/,
    'collapsed state must disable the card');
  assert.match(src, /data-gm-collapsed/, 'JS must drive the collapsed attribute');
});
