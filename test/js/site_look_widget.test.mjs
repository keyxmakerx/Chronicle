// site_look_widget.test.mjs — the Site look preview's decisions: what the
// preview shows for a given form state, what is allowed to move, and the
// reduced-motion rules of the shared header-motion code it reuses. The widget
// is loaded the way the page loads it, with a stub Chronicle.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const widgets = join(here, '..', '..', 'static', 'js', 'widgets');

function load(file, sandbox) {
  vm.runInContext(readFileSync(join(widgets, file), 'utf8'), sandbox);
}

function sandbox(attrs, mqReduce) {
  const registered = {};
  const root = { hasAttribute: (n) => n in attrs, getAttribute: (n) => attrs[n] ?? null };
  const ctx = vm.createContext({
    window: { matchMedia: () => ({ matches: !!mqReduce }) },
    document: { documentElement: root },
    Chronicle: { register: (n, w) => { registered[n] = w; } },
    requestAnimationFrame() { return 1; }, cancelAnimationFrame() {},
  });
  ctx.window.Chronicle = ctx.Chronicle;
  load('header_motion.js', ctx);
  load('site_look.js', ctx);
  return { ctx, registered };
}

const site = (attrs = {}, mq = false) => sandbox(attrs, mq).ctx.Chronicle;

test('the widget registers under its mount name', () => {
  assert.ok(sandbox({}).registered['site-look']);
});

test('initial and displayName fall back to Chronicle', () => {
  const C = site();
  assert.equal(C.siteLook.initial('  dragon hold'), 'D');
  assert.equal(C.siteLook.initial('   '), 'C');
  assert.equal(C.siteLook.displayName(''), 'Chronicle');
});

test('view applies the real page fallbacks', () => {
  const v = site().siteLook.view;
  const cases = [
    [{ look: '', background: 'plain', hasPicture: false, move: true }, { bg: 'plain', look: false, moving: false }],
    [{ look: 'ember', background: 'look', hasPicture: false, move: false }, { bg: 'look', look: true, moving: false }],
    [{ look: '', background: 'look', hasPicture: false, move: true }, { bg: 'plain', look: false, moving: false }],
    [{ look: 'ember', background: 'picture', hasPicture: false, move: true }, { bg: 'plain', look: true, moving: false }],
    [{ look: 'ember', background: 'picture', hasPicture: true, move: true }, { bg: 'picture', look: true, moving: true }],
    [{ look: 'arcane', background: 'look', hasPicture: false, move: true }, { bg: 'look', look: true, moving: true }],
  ];
  for (const [state, want] of cases) assert.deepEqual(JSON.parse(JSON.stringify(v(state))), want, JSON.stringify(state));
});

test('motionPlan moves only what is on, and nothing when reduced', () => {
  const C = site();
  const plan = (v, r) => Array.from(C.siteLook.motionPlan(v, r));
  assert.deepEqual(plan({ bg: 'look', look: true, moving: true }, false), ['flow', 'top']);
  assert.deepEqual(plan({ bg: 'picture', look: false, moving: true }, false), ['pan']);
  assert.deepEqual(plan({ bg: 'picture', look: true, moving: true }, false), ['pan', 'top']);
  assert.deepEqual(plan({ bg: 'look', look: true, moving: false }, false), []);
  assert.deepEqual(plan({ bg: 'look', look: true, moving: true }, true), []);
});

test('reduced motion: device setting, campaign switch and personal Calmer all stop it', () => {
  const rows = [
    [{}, false, false],
    [{}, true, true],
    [{ 'data-cz-reduce': '' }, false, true],
    [{ 'data-view-motion': 'calm' }, false, true],
    [{ 'data-view-motion': 'full' }, false, false],
  ];
  for (const [attrs, mq, want] of rows) {
    assert.equal(site(attrs, mq).headerMotion.reduced(), want, JSON.stringify(attrs) + ' mq=' + mq);
  }
});
