// event_peek.test.mjs — C-CAL-CLOSEOUT PR A §1. The hover quick-peek card:
// hovering an event chip shows a compact preview (title/time/category/tier),
// the peek hides on click (so the drawer takes over), no-hover/touch devices
// skip it entirely, and the click→drawer path is left untouched. Listeners
// bind to the shell root (not document) so they don't leak on boosted nav.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'internal', 'plugins', 'calendar', 'static', 'js', 'calendar_v2_shell.js');
const src = readFileSync(jsPath, 'utf8');

// makeEl builds a minimal DOM-element stand-in: classList (with toggle),
// dataset, style, getBoundingClientRect, querySelector, and event capture.
function makeEl(extra) {
  const el = Object.assign({ style: {}, dataset: {} }, extra);
  el._classes = new Set(extra && extra._hidden === false ? [] : ['hidden']);
  el._ev = {};
  el._q = el._q || {};
  el.classList = {
    add: (c) => el._classes.add(c),
    remove: (c) => el._classes.delete(c),
    contains: (c) => el._classes.has(c),
    toggle: (c, on) => {
      if (on === undefined) on = !el._classes.has(c);
      if (on) el._classes.add(c); else el._classes.delete(c);
    },
  };
  el.getBoundingClientRect = el.getBoundingClientRect ||
    (() => ({ left: 10, right: 50, top: 10, bottom: 24, width: 40, height: 14 }));
  el.addEventListener = (t, fn) => { (el._ev[t] = el._ev[t] || []).push(fn); };
  el.removeEventListener = () => {};
  el.querySelector = (sel) => el._q[sel] || null;
  el.contains = el.contains || (() => false);
  return el;
}

function boot(eventsJSON, hoverCapable) {
  const title = makeEl(), time = makeEl(), cat = makeEl(), tier = makeEl();
  const peek = makeEl();
  peek._q = {
    '[data-peek-title]': title, '[data-peek-time]': time,
    '[data-peek-category]': cat, '[data-peek-tier]': tier,
  };
  const root = makeEl({ dataset: { calV2Events: eventsJSON } });
  const byId = { 'cal-v2-event-peek': peek };
  const docEvents = {};
  const sandbox = {
    console,
    document: {
      readyState: 'complete',
      querySelector: (sel) => (sel === '[data-cal-v2-root]' ? root : null),
      querySelectorAll: () => [],
      getElementById: (id) => byId[id] || null,
      addEventListener: (t, fn) => { (docEvents[t] = docEvents[t] || []).push(fn); },
      removeEventListener() {},
    },
  };
  sandbox.window = sandbox;
  sandbox.window.innerWidth = 1000;
  sandbox.window.innerHeight = 800;
  sandbox.window.matchMedia = () => ({ matches: !hoverCapable }); // '(hover: none)'
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox, { filename: 'calendar_v2_shell.js' });
  return { root, peek, title, time, cat, tier };
}

// A fake event-chip whose .closest() resolves the peek's chip selector.
function chip(id) {
  return {
    dataset: { eventId: id },
    contains: () => false,
    getBoundingClientRect: () => ({ left: 100, right: 140, top: 50, bottom: 64, width: 40, height: 14 }),
  };
}
function over(target) { return { target: { closest: (s) => (s.indexOf('data-event-card') >= 0 ? target : null) } }; }

test('hovering an event chip shows the peek filled with title/time/category/tier', () => {
  const ev = { id: 'e1', name: 'Battle of Dawn', start_hour: 9, start_minute: 5, category: 'combat', tier: 'major' };
  const { root, peek, title, time, cat, tier } = boot(JSON.stringify([ev]), true);
  assert.ok(root._ev['mouseover'] && root._ev['mouseover'].length, 'mouseover must bind on the shell root');
  root._ev['mouseover'][0](over(chip('e1')));
  assert.equal(peek.classList.contains('hidden'), false, 'peek should be visible on hover');
  assert.equal(title.textContent, 'Battle of Dawn');
  assert.equal(time.textContent, '09:05');
  assert.equal(cat.textContent, 'Combat');   // title-cased category slug
  assert.equal(tier.textContent, 'Major');    // title-cased tier slug
});

test('an all-day event reads "All day"; a chip with no match leaves the peek hidden', () => {
  const { root, peek, time } = boot(JSON.stringify([{ id: 'e1', name: 'Festival', all_day: true }]), true);
  root._ev['mouseover'][0](over(chip('e1')));
  assert.equal(time.textContent, 'All day');
  // Unknown id → no event → peek stays hidden.
  const fresh = boot(JSON.stringify([{ id: 'e1', name: 'X' }]), true);
  fresh.root._ev['mouseover'][0](over(chip('does-not-exist')));
  assert.equal(fresh.peek.classList.contains('hidden'), true);
});

test('clicking dismisses the peek so the drawer takes over', () => {
  const { root, peek } = boot(JSON.stringify([{ id: 'e1', name: 'X' }]), true);
  root._ev['mouseover'][0](over(chip('e1')));
  assert.equal(peek.classList.contains('hidden'), false);
  assert.ok(root._ev['click'] && root._ev['click'].length, 'click must be wired to dismiss the peek');
  root._ev['click'][0]({});
  assert.equal(peek.classList.contains('hidden'), true, 'peek must hide on click');
});

test('mousing out of the chip hides the peek (but staying inside it does not)', () => {
  const { root, peek } = boot(JSON.stringify([{ id: 'e1', name: 'X' }]), true);
  const c = chip('e1');
  root._ev['mouseover'][0](over(c));
  assert.equal(peek.classList.contains('hidden'), false);
  // relatedTarget still inside the chip → keep showing.
  c.contains = () => true;
  root._ev['mouseout'][0]({ target: { closest: () => c }, relatedTarget: {} });
  assert.equal(peek.classList.contains('hidden'), false, 'must not hide while still over the chip');
  // moving away → hide.
  c.contains = () => false;
  root._ev['mouseout'][0]({ target: { closest: () => c }, relatedTarget: {} });
  assert.equal(peek.classList.contains('hidden'), true);
});

test('touch / no-hover devices skip the peek wiring (fall through to the drawer)', () => {
  const { root } = boot(JSON.stringify([{ id: 'e1', name: 'X' }]), false); // hover:none
  assert.ok(!root._ev['mouseover'], 'no-hover devices must not wire the hover peek');
});

test('source seams: peek is wired (not document-leaked), guarded for touch, click→drawer intact', () => {
  assert.match(src, /function wireEventPeek/, 'wireEventPeek missing');
  assert.match(src, /\(hover: none\)/, 'touch/no-hover guard missing');
  assert.match(src, /cal-v2-event-peek/, 'peek element id missing');
  // Bound on root (dies with the shell node), not document → no leak on boosted nav.
  assert.match(src, /root\.addEventListener\(\s*['"]mouseover['"]/, 'peek hover must bind on root, not document');
});
