// gm_panel_transition.test.mjs — the GM console's translucent-transition +
// pull-out containment contracts (C-CAL-SKYPANE-DETACH §B; supersedes the
// in-band full-pane sheet from C-CAL-WORLDSTATE-GM-OVERHAUL):
//   1. Every world mutation sets data-gm-transition on the console root; the
//      CSS owns the fade (opacity 0.16) so the sky animates through the card.
//   2. prefers-reduced-motion neutralizes the fade IN CSS (opacity stays 1) —
//      the JS stays branch-free.
//   3. The pull-out card is height-capped + internally scrollable in CSS (it
//      floats OVER the command bar above while open, but never grows
//      unbounded), pulls out of the strip's top edge, and collapses to a tab.
//   4. Esc + tap-outside dismiss the open card (JS document-level handlers).
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
  assert.match(css, /\.gm-console\[data-gm-transition="true"\] \.gm-console__card \{ opacity: 0\.1/,
    'transition state must fade the card to near-transparent');
  assert.match(css, /prefers-reduced-motion/, 'reduced-motion block missing');
  assert.match(css, /prefers-reduced-motion[\s\S]*?\[data-gm-transition="true"\] \.gm-console__card \{ opacity: 1; \}/,
    'reduced-motion must keep the card fully opaque (no fade)');
});

test('the pull-out card is height-capped + scrolls internally (never unbounded)', () => {
  assert.match(css, /\.gm-console__card \{[\s\S]*?max-height: min\(/, 'card must be height-capped');
  assert.match(css, /\.gm-console__sheet-body \{[\s\S]*?overflow-y: auto/, 'sheet body must scroll internally');
  assert.match(css, /\.gm-console \{[\s\S]*?pointer-events: none/, 'console root must not block sky/strip clicks');
  assert.doesNotMatch(src, /maxHeight = 'none'/, 'JS must not release any body to an unbounded height');
});

test('the console PULLS OUT of the strip top edge, over content above (§B)', () => {
  // The card anchors to the strip's top edge (bottom:100%) and grows UPWARD,
  // overlaying the command bar above with zero reflow.
  assert.match(css, /\.gm-console__card \{[\s\S]*?bottom: 100%/, 'card bottom anchors on the strip top edge (grows up over content above)');
  assert.match(css, /\.gm-console__card \{[\s\S]*?flex-direction: column-reverse/, 'control bar pinned at the hinge; the sheet unfolds above it');
  assert.match(css, /\.gm-console__pill \{[\s\S]*?bottom: 100%/, 'the tab rides the strip top edge');
  assert.match(css, /\.gm-console__collapse \{/, 'close ✕ handle missing');
});

test('collapsed console is fully inert — visibility cut, no click-traps', () => {
  assert.match(css, /data-gm-collapsed="true"\][\s\S]*?visibility: hidden/, 'the collapsed card must be visibility:hidden');
  const iconbtnRule = css.match(/\.gm-console__iconbtn \{[^}]*\}/);
  assert.ok(iconbtnRule, 'iconbtn rule present');
  assert.doesNotMatch(iconbtnRule[0], /pointer-events: auto/, 'icon buttons must not re-enable hit-testing (the invisible click-trap class)');
});

test('the console collapses to a tab (data-gm-collapsed contract)', () => {
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__pill \{ display: inline-flex; \}/,
    'collapsed state must show the tab');
  assert.match(css, /\.gm-console\[data-gm-collapsed="true"\] \.gm-console__card \{[\s\S]*?pointer-events: none/,
    'collapsed state must disable the card');
  assert.match(src, /data-gm-collapsed/, 'JS must drive the collapsed attribute');
  assert.match(src, /data-gm-sheet-open/, 'JS must wire the section sheet buttons');
});

// --- §B dismiss: Esc + tap-outside (new document-level handlers) ------------

function bootDismiss() {
  const docHandlers = {};
  const toggle = el(); toggle._attrs['data-gm-panel-toggle'] = '';
  const pill = el();
  const inside = el();
  const outside = el();
  const panel = el({
    _attrs: { 'data-gm-collapsed': 'false', 'data-gm-sheet': '' },
    querySelector(sel) { return sel === '[data-gm-panel-toggle]' ? toggle : sel === '[data-gm-panel-open]' ? pill : null; },
    querySelectorAll() { return []; },
    contains(node) { return node === panel || node === inside; },
  });
  const root = el({ dataset: { calV2CampaignId: 'c', calV2CalendarId: 'cal', calV2CsrfToken: 't' } });
  const sandbox = {
    console, matchMedia: () => ({ matches: false }), setTimeout: () => 0, clearTimeout() {},
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-gm-panel]' ? panel : sel === '[data-cal-v2-root]' ? root : null),
      addEventListener(type, fn) { docHandlers[type] = fn; },
      removeEventListener() {},
    },
  };
  sandbox.window = sandbox;
  sandbox.Chronicle = { apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }), notify() {} };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'gm_panel.js' });
  return { panel, pill, inside, outside, docHandlers };
}

test('tap-outside dismisses the open card; clicks inside keep it open', () => {
  const h = bootDismiss();
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'false', 'starts open (markup said open)');
  assert.ok(h.docHandlers.click, 'document click (tap-outside) handler must be wired');
  // A click on the tab/card (inside the panel) must NOT self-dismiss.
  h.docHandlers.click({ target: h.inside });
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'false', 'inside click keeps it open');
  // A click anywhere else collapses it.
  h.docHandlers.click({ target: h.outside });
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'true', 'tap-outside collapses the card');
  // Once collapsed, an outside click is a no-op (no double-fire hazard).
  h.docHandlers.click({ target: h.outside });
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'true', 'collapsed stays collapsed');
});

test('Esc dismisses the open card', () => {
  const h = bootDismiss();
  assert.ok(h.docHandlers.keydown, 'document keydown (Esc) handler must be wired');
  // A non-Esc key does nothing.
  h.docHandlers.keydown({ key: 'a' });
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'false', 'other keys leave it open');
  h.docHandlers.keydown({ key: 'Escape' });
  assert.equal(h.panel.getAttribute('data-gm-collapsed'), 'true', 'Esc collapses the card');
});
