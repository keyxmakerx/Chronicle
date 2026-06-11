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
  assert.match(css, /\[data-gm-transition="true"\] \.gm-console__strip,\s*\n\.gm-console\[data-gm-transition="true"\] \.gm-console__sheet \{\s*\n\s*opacity: 0\.1/,
    'transition state must fade strip + sheet to near-transparent');
  assert.match(css, /prefers-reduced-motion/, 'reduced-motion block missing');
  assert.match(css, /prefers-reduced-motion[\s\S]*?\[data-gm-transition="true"\] \.gm-console__strip \{ opacity: 1; \}/,
    'reduced-motion must keep the strip fully opaque (no fade)');
});

test('sheets cover the band pane and scroll internally (never unbounded)', () => {
  assert.match(css, /\.gm-console__sheet\s*\{[\s\S]*?top: 44px/, 'sheets must span the sky pane below the strip');
  assert.match(css, /\.gm-console__sheet-body\s*\{[\s\S]*?overflow-y: auto/, 'sheet body must scroll internally');
  assert.match(css, /\.gm-console\s*\{[\s\S]*?pointer-events: none/, 'console root must not block sky clicks');
  assert.doesNotMatch(src, /maxHeight = 'none'/, 'JS must not release any body to an unbounded height');
});

test('the console is DOCKED into the pane edge (r3, cordinator#33)', () => {
  assert.match(css, /\.gm-console__strip \{[\s\S]*?top: 0;[\s\S]*?right: 0;/, 'strip must be flush with the pane corner');
  assert.match(css, /\.gm-console__strip \{[\s\S]*?border-radius: 0 0 0 12px/, 'strip squares off where it meets the edges');
  assert.match(css, /\.gm-console__collapse \{/, 'edge-fused collapse handle missing');
  assert.match(css, /\.gm-console__sheet \{[\s\S]*?left: 0;[\s\S]*?right: 0;[\s\S]*?bottom: 0;/, 'sheets flush to the pane edges');
});

test('collapsed console is fully inert — visibility cut, no click-traps (r3)', () => {
  assert.match(css, /data-gm-collapsed="true"\][\s\S]*?visibility: hidden/, 'collapsed strip/sheets must be visibility:hidden');
  const iconbtnRule = css.match(/\.gm-console__iconbtn \{[^}]*\}/);
  assert.ok(iconbtnRule, 'iconbtn rule present');
  assert.doesNotMatch(iconbtnRule[0], /pointer-events: auto/, 'icon buttons must not re-enable hit-testing (the invisible click-trap class)');
});

test('the console collapses to a pill (data-gm-collapsed contract)', () => {
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__pill \{ display: inline-flex; \}/,
    'collapsed state must show the pill');
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__strip,[\s\S]*?pointer-events: none/,
    'collapsed state must disable the strip + sheets');
  assert.match(src, /data-gm-collapsed/, 'JS must drive the collapsed attribute');
  assert.match(src, /data-gm-sheet-open/, 'JS must wire the section sheet buttons');
});
