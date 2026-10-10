// map_live.test.mjs — pins the live-refresh core for open map pages: bursts
// collapse into one refetch, an edit in progress holds the refetch back, a
// failed or malformed refetch leaves the screen alone, and a change that lands
// mid-fetch is not lost.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const live = createRequire(import.meta.url)('../../static/js/map_live.js');

// A manual clock stands in for setTimeout so the debounce is exact.
function clock() {
  let now = 0, seq = 0;
  const timers = new Map();
  return {
    setTimer(fn, ms) { const id = ++seq; timers.set(id, { at: now + ms, fn }); return id; },
    clearTimer(id) { timers.delete(id); },
    async advance(ms) {
      const end = now + ms;
      for (;;) {
        const due = [...timers].filter(([, t]) => t.at <= end).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        timers.delete(due[0]);
        now = due[1].at;
        due[1].fn();
        // Let promise continuations run before the next timer.
        for (let i = 0; i < 5; i++) await Promise.resolve();
      }
      now = end;
    },
    pending() { return timers.size; }
  };
}

function harness(over = {}) {
  const c = clock();
  const calls = { fetch: 0, applied: [] };
  const state = { busy: false, result: () => Promise.resolve([{ id: 'a' }]) };
  const r = live.create({
    setTimer: c.setTimer, clearTimer: c.clearTimer,
    kinds: {
      markers: {
        fetch() { calls.fetch++; return state.result(); },
        apply(list) { calls.applied.push(list); },
        busy() { return state.busy; }
      },
      drawings: { fetch: () => Promise.resolve([]), apply() {}, busy: () => false }
    },
    ...over
  });
  return { c, r, calls, state };
}

test('a burst of notices becomes one refetch after the debounce', async () => {
  const { c, r, calls } = harness();
  r.notify('markers'); r.notify('markers'); r.notify('markers');
  await c.advance(299);
  assert.equal(calls.fetch, 0);
  await c.advance(1);
  assert.equal(calls.fetch, 1);
  assert.deepEqual(calls.applied, [[{ id: 'a' }]]);
});

test('shadows share the drawings refetch and tokens are ignored', async () => {
  assert.equal(live.kindOf('shadows'), 'drawings');
  assert.equal(live.kindOf('markers'), 'markers');
  const { c, r, calls } = harness();
  r.notify('tokens');
  r.notify('constructor');
  await c.advance(1000);
  assert.equal(calls.fetch, 0);
  assert.equal(c.pending(), 0);
});

test('a failed refetch keeps what is on screen', async () => {
  const cases = [
    ['rejects', () => Promise.reject(new Error('HTTP 500'))],
    ['throws', () => { throw new Error('boom'); }],
    ['not a list', () => Promise.resolve({ error: 'x' })],
    ['null', () => Promise.resolve(null)]
  ];
  for (const [name, result] of cases) {
    const { c, r, calls, state } = harness();
    state.result = result;
    r.notify('markers');
    await c.advance(300);
    assert.equal(calls.fetch, 1, name);
    assert.equal(calls.applied.length, 0, `${name}: nothing applied`);
  }
});

test('an apply that throws does not break later refreshes', async () => {
  const c = clock();
  let n = 0;
  const r = live.create({
    setTimer: c.setTimer, clearTimer: c.clearTimer,
    kinds: { markers: { fetch: () => Promise.resolve([]), apply() { n++; throw new Error('draw failed'); } } }
  });
  r.notify('markers'); await c.advance(300);
  r.notify('markers'); await c.advance(300);
  assert.equal(n, 2);
});

test('no refetch while an edit is in progress; it runs once the edit ends', async () => {
  const { c, r, calls, state } = harness();
  state.busy = true;
  r.notify('markers');
  await c.advance(300 * 5);
  assert.equal(calls.fetch, 0);
  state.busy = false;
  await c.advance(300);
  assert.equal(calls.fetch, 1);
  assert.equal(calls.applied.length, 1);
});

test('a notice during a fetch triggers one more afterwards', async () => {
  const { c, r, calls, state } = harness();
  let release;
  state.result = () => new Promise((res) => { release = () => res([{ id: 'late' }]); });
  r.notify('markers');
  await c.advance(300);
  assert.equal(calls.fetch, 1);
  r.notify('markers');
  await c.advance(300);
  assert.equal(calls.fetch, 1, 'not started while the first is in flight');
  state.result = () => Promise.resolve([{ id: 'fresh' }]);
  release();
  for (let i = 0; i < 5; i++) await Promise.resolve();
  await c.advance(300);
  assert.equal(calls.fetch, 2);
  assert.deepEqual(calls.applied.at(-1), [{ id: 'fresh' }]);
});

test('destroy stops pending work and applies nothing late', async () => {
  const { c, r, calls, state } = harness();
  let release;
  state.result = () => new Promise((res) => { release = () => res([{ id: 'x' }]); });
  r.notify('markers');
  await c.advance(300);
  r.destroy();
  release();
  for (let i = 0; i < 5; i++) await Promise.resolve();
  assert.equal(calls.applied.length, 0);
  r.notify('markers');
  assert.equal(c.pending(), 0);
});

test('a shadow notice refreshes pins as well as drawings', () => {
  assert.deepEqual(live.kindsFor('shadows'), ['drawings', 'markers']);
  assert.deepEqual(live.kindsFor('drawings'), ['drawings']);
  assert.deepEqual(live.kindsFor('markers'), ['markers']);
});
