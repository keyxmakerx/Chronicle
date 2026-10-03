// map_shadow.test.mjs — pins the shadow module's pure parts: box parsing, the
// two strengths, and the motion rules (ease to a stop, idle and hidden stops,
// reduced motion, and that a stopped loop schedules nothing).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'map_shadow.js'), 'utf8');

function load() {
  const saved = { window: globalThis.window, document: globalThis.document, module: globalThis.module };
  delete globalThis.window;
  delete globalThis.document;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  globalThis.module = saved.module;
  if (saved.window !== undefined) globalThis.window = saved.window;
  if (saved.document !== undefined) globalThis.document = saved.document;
  return api;
}
const S = load();

// A fake clock with a manual rAF queue and timers.
function harness(over = {}) {
  const h = {
    t: 0, queue: [], timers: [], frames: 0, active: true,
    reduced: false,
  };
  Object.assign(h, over);
  const env = {
    raf: (fn) => { h.queue.push(fn); return h.queue.length; },
    caf: () => { h.queue = []; },
    now: () => h.t,
    setTimer: (fn, ms) => { h.timers.push({ at: h.t + ms, fn }); return h.timers.length; },
    clearTimer: () => { h.timers = []; },
    get reduced() { return h.reduced; },
    isActive: () => h.active,
    onFrame: () => { h.frames++; },
  };
  h.motion = S.createMotion(env);
  // Advance time by ms in 16ms frames, firing timers and one queued frame each step.
  h.run = (ms) => {
    const end = h.t + ms;
    while (h.t < end) {
      h.t += 16;
      const due = h.timers.filter((x) => x.at <= h.t);
      h.timers = h.timers.filter((x) => x.at > h.t);
      due.forEach((x) => x.fn());
      const q = h.queue; h.queue = [];
      q.forEach((fn) => fn(h.t));
    }
  };
  return h;
}

test('levelFor: the two stored strengths', () => {
  assert.equal(S.levelFor(0.5), 'hint');
  assert.equal(S.levelFor(0.85), 'almost');
  assert.equal(S.levelFor(0), 'hint');
});

test('boxFromPoints orders corners given in any order and rejects other shapes', () => {
  assert.deepEqual(S.boxFromPoints([{ x: 60, y: 70 }, { x: 20, y: 30 }]), { minX: 20, maxX: 60, minY: 30, maxY: 70 });
  assert.equal(S.boxFromPoints([{ x: 1, y: 1 }]), null);
  assert.equal(S.boxFromPoints([{ x: 1, y: 1 }, { x: 2, y: 2 }, { x: 3, y: 3 }]), null);
  assert.equal(S.boxFromPoints([{ x: 1, y: 1 }, { x: 'a', y: 2 }]), null);
  assert.equal(S.boxFromPoints(null), null);
});

test('no shadows: nothing starts', () => {
  const h = harness({ active: false });
  h.motion.start();
  assert.equal(h.motion.running(), false);
});

test('drifts while there is input, then eases to a stop and the loop ends', () => {
  const h = harness();
  h.motion.start();
  h.run(2000);
  assert.ok(h.motion.state().factor > 0.99, 'reaches full drift');
  // Keep input flowing so idle does not trigger.
  for (let i = 0; i < 60; i++) { h.run(500); h.motion.input(); }
  assert.ok(h.motion.running());
  // Now stop all input: after IDLE_MS it must ease to rest within ~a second or two.
  h.run(S.IDLE_MS + 3000);
  assert.equal(h.motion.state().factor, 0);
  assert.equal(h.motion.running(), false, 'no rAF scheduled once stopped');
  const framesAtRest = h.frames;
  h.run(5000);
  assert.equal(h.frames, framesAtRest, 'no idle frames after stopping');
});

test('easing is gradual: partway through the stop the drift is neither full nor zero', () => {
  const h = harness();
  h.motion.start();
  h.run(2000);
  h.motion.setHidden(true);
  h.run(300);
  const f = h.motion.state().factor;
  assert.ok(f > 0.05 && f < 0.7, 'mid-ease factor ' + f);
  h.run(2500);
  assert.equal(h.motion.running(), false);
});

test('hidden tab stops the drift; visible again eases back in; input while hidden does not restart', () => {
  const h = harness();
  h.motion.start();
  h.run(1500);
  h.motion.setHidden(true);
  h.run(3000);
  assert.equal(h.motion.running(), false);
  h.motion.input();
  assert.equal(h.motion.running(), false, 'hidden tab ignores input');
  h.motion.setHidden(false);
  assert.equal(h.motion.running(), true);
  h.run(2000);
  assert.ok(h.motion.state().factor > 0.9);
});

test('input after the idle stop wakes it', () => {
  const h = harness();
  h.motion.start();
  h.run(S.IDLE_MS + 4000);
  assert.equal(h.motion.running(), false);
  h.motion.input();
  assert.equal(h.motion.running(), true);
});

test('reduced motion never moves and never schedules a frame', () => {
  const h = harness({ reduced: true });
  h.motion.start();
  h.motion.input();
  h.motion.setHidden(false);
  h.run(5000);
  assert.equal(h.frames, 0);
  assert.equal(h.motion.running(), false);
});

test('destroy cancels the loop and timers', () => {
  const h = harness();
  h.motion.start();
  h.run(500);
  h.motion.destroy();
  const frames = h.frames;
  h.run(2000);
  assert.equal(h.frames, frames);
  assert.equal(h.motion.running(), false);
});

test('loop stops when the last shadow layer goes away, and restarts when one is added', () => {
  const h = harness();
  h.motion.start();
  h.run(1500);
  assert.equal(h.motion.running(), true);
  h.active = false; // zero layers registered
  h.run(3000);
  assert.equal(h.motion.running(), false);
  const frames = h.frames;
  h.run(3000);
  assert.equal(h.frames, frames, 'no frames with zero layers');
  h.active = true; // a layer is added
  h.motion.start();
  assert.equal(h.motion.running(), true);
});
