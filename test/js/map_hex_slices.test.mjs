// map_hex_slices.test.mjs — pins how Realistic hex art is scheduled so painting
// never holds the page: the time-limited job queue and the model that can be
// built a few rows at a time (map_hex_art.js).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'internal', 'plugins', 'maps', 'static', 'js', 'map_hex_art.js'), 'utf8');

function loadArt() {
  const saved = { window: globalThis.window, module: globalThis.module };
  delete globalThis.window;
  globalThis.module = { exports: {} };
  vm.runInThisContext(src);
  const api = globalThis.module.exports;
  globalThis.module = saved.module;
  if (saved.window !== undefined) globalThis.window = saved.window;
  return api;
}
const HA = loadArt();

// A clock the test moves by hand: each unit of work costs the ticks it names.
function clock() {
  let t = 0;
  return { now: () => t, tick: (n) => { t += n; } };
}

test('JobQueue: a slice stops once the budget is spent and the rest waits', () => {
  const c = clock();
  const done = [];
  const q = new HA.JobQueue((j) => { c.tick(4); done.push(j.key); return true; });
  ['a', 'b', 'c', 'd', 'e'].forEach((k) => q.add({ key: k }));
  assert.equal(q.run(10, c.now), 2);          // a, b, c ran: 12 ticks is past 10
  assert.deepEqual(done, ['a', 'b', 'c']);
  assert.equal(q.run(10, c.now), 0);
  assert.deepEqual(done, ['a', 'b', 'c', 'd', 'e']);
});

test('JobQueue: a budget of zero still runs the front job, so the queue cannot stall', () => {
  const c = clock();
  const done = [];
  const q = new HA.JobQueue((j) => { done.push(j.key); return true; });
  q.add({ key: 'a' });
  q.add({ key: 'b' });
  assert.equal(q.run(0, c.now), 1);
  assert.deepEqual(done, ['a']);
});

test('JobQueue: keys are deduplicated and first goes to the front', () => {
  const order = [];
  const q = new HA.JobQueue((j) => { order.push(j.key); return true; });
  assert.equal(q.add({ key: 'a' }), true);
  assert.equal(q.add({ key: 'a' }), false);
  q.add({ key: 'b' });
  q.add({ key: 'c' }, true);
  assert.equal(q.length, 3);
  q.run(1000, () => 0);
  assert.deepEqual(order, ['c', 'a', 'b']);
  // A finished key can be queued again.
  assert.equal(q.add({ key: 'a' }), true);
});

test('JobQueue: a job that ran out of time keeps its place, and "wait" ends the slice', () => {
  const c = clock();
  let calls = 0;
  let blocked = true;
  const q = new HA.JobQueue((j) => {
    if (j.key === 'long') { calls++; c.tick(5); return calls >= 3; }
    if (j.key === 'gate') return blocked ? 'wait' : true;
    return true;
  });
  q.add({ key: 'long' });
  q.add({ key: 'gate' });
  q.add({ key: 'after' });
  assert.equal(q.run(10, c.now), 3);          // long is unfinished, nothing behind it ran
  assert.equal(q.run(10, c.now), 3);
  assert.equal(q.run(10, c.now), 2);          // long finished; gate now waits
  assert.equal(q.run(10, c.now), 2);
  blocked = false;
  assert.equal(q.run(10, c.now), 0);
  assert.equal(calls, 3);
});

test('JobQueue: clear drops the queue and frees the keys', () => {
  const q = new HA.JobQueue(() => true);
  q.add({ key: 'a' });
  q.clear();
  assert.equal(q.length, 0);
  assert.equal(q.add({ key: 'a' }), true);
});

// iso3 paints on canvases; this stub keeps the pixels the model writes so two
// runs can be compared.
function withCanvasStub(fn) {
  const made = [];
  const saved = globalThis.document;
  globalThis.document = {
    createElement() {
      const cv = { width: 0, height: 0 };
      const ctx = {
        createImageData: (w, h) => ({ data: new Uint8ClampedArray(w * h * 4) }),
        putImageData(img) { cv.img = img; },
        drawImage() {}, save() {}, restore() {}, scale() {}, fillRect() {},
        createRadialGradient: () => ({ addColorStop() {} }),
      };
      cv.getContext = () => ctx;
      made.push(cv);
      return cv;
    },
  };
  try { return fn(made); } finally { globalThis.document = saved; }
}

test('iso3ModelJob: building in small steps gives the same pixels as in one go', () => {
  const cases = [['forest', 3], ['mountain', 1], ['water', 2], ['town', 4]];
  for (const [t, v] of cases) {
    const whole = withCanvasStub((made) => {
      HA.iso3Model(t, v, 0.5)({ bl: false, br: true });
      return Array.from(made[0].img.data);
    });
    let steps = 0;
    const sliced = withCanvasStub((made) => {
      const job = HA.iso3ModelJob(t, v, 0.5);
      // A clock that has always run out makes every step stop after one row.
      while (!job.step(-1)) steps++;
      job.make({ bl: false, br: true });
      return Array.from(made[0].img.data);
    });
    assert.ok(steps > 10, t + ': the model was built in many steps, got ' + steps);
    assert.deepEqual(sliced, whole, t + ' v' + v);
  }
});

test('iso3ModelJob: one model serves every edge variant', () => {
  withCanvasStub((made) => {
    const job = HA.iso3ModelJob('hills', 5, 0.5);
    assert.equal(job.step(Infinity), true);
    const a = job.make({ bl: true, br: true });
    const b = job.make({ bl: false, br: false });
    assert.ok(a.canvas && b.canvas && a.canvas !== b.canvas);
  });
});
