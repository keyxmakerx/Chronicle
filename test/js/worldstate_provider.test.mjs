// worldstate_provider.test.mjs — C-CAL-WORLDSTATE-WIDGETS. The worldState
// provider singleton: exactly one /world-state fetch per page regardless of
// how many widgets subscribe; a server-embedded seed means ZERO fetch;
// subscribe fans the seed out (immediately if already loaded); errors fan to
// onError; reduced-motion skips the shared rAF; destroy() tears down and the
// last unsubscribe self-destroys.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const jsPath = join(here, '..', '..', 'static', 'js', 'widgets', 'worldstate_provider.js');

// Boot the provider IIFE in a vm with a minimal window. opts.reduced toggles
// prefers-reduced-motion; rAF is a manual queue so the shared loop is testable.
function boot(opts) {
  opts = opts || {};
  const rafQ = [];
  const sandbox = {
    console,
    matchMedia: () => ({ matches: !!opts.reduced }),
    requestAnimationFrame: (cb) => { rafQ.push(cb); return rafQ.length; },
    cancelAnimationFrame: () => {},
    Promise,
    setTimeout, clearTimeout,
  };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  sandbox.__pump = (n) => { for (let i = 0; i < (n || 1) && rafQ.length; i++) { const cb = rafQ.shift(); cb(16 * (i + 1)); } };
  sandbox.__rafLen = () => rafQ.length;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(jsPath, 'utf8'), sandbox, { filename: 'worldstate_provider.js' });
  return sandbox;
}

// A counting fetch stub that resolves to a fixed seed.
function countingFetch(seed) {
  const calls = { n: 0, urls: [] };
  const impl = (url) => {
    calls.n++; calls.urls.push(url);
    return Promise.resolve({ ok: true, json: () => Promise.resolve(seed) });
  };
  return { impl, calls };
}

const SEED = { timeOfDay: 0.5, season: 'Spring', date: { year: 1492, month: 4, day: 15 }, weather: { type: 'rain', intensity: 1 } };

test('exactly one fetch per page regardless of subscriber count', async () => {
  const w = boot();
  const { impl, calls } = countingFetch(SEED);
  w.ChronicleWorldState._fetch = impl;
  const p = w.ChronicleWorldState.get('camp-1');
  // three independent widgets subscribe + all trigger load
  const got = [];
  p.subscribe((s) => got.push(['a', s]));
  p.subscribe((s) => got.push(['b', s]));
  p.subscribe((s) => got.push(['c', s]));
  await Promise.all([p.load(), p.load(), p.load()]);
  assert.equal(calls.n, 1, 'one network fetch for the whole page');
  assert.equal(p.fetchCount, 1);
  assert.equal(got.length, 3, 'all three subscribers received the seed');
  assert.deepEqual(JSON.parse(JSON.stringify(p.current())), SEED);
});

test('get() returns the same singleton instance per campaign', () => {
  const w = boot();
  const a = w.ChronicleWorldState.get('camp-x');
  const b = w.ChronicleWorldState.get('camp-x');
  assert.equal(a, b, 'memoized per campaign id');
});

test('a server-embedded seed means ZERO fetch', async () => {
  const w = boot();
  const { impl, calls } = countingFetch(SEED);
  w.ChronicleWorldState._fetch = impl;
  const p = w.ChronicleWorldState.get('camp-2');
  let received = null;
  p.subscribe((s) => { received = s; });
  await p.load({ seed: SEED });
  assert.equal(calls.n, 0, 'no fetch when a server seed is provided');
  assert.equal(p.fetchCount, 0);
  assert.deepEqual(JSON.parse(JSON.stringify(received)), SEED);
});

test('subscribe after load fires immediately with the current seed', async () => {
  const w = boot();
  w.ChronicleWorldState._fetch = countingFetch(SEED).impl;
  const p = w.ChronicleWorldState.get('camp-3');
  await p.load();
  let late = null;
  p.subscribe((s) => { late = s; });
  assert.ok(late, 'late subscriber got the already-loaded seed');
});

test('load failure fans to onError and rejects', async () => {
  const w = boot();
  w.ChronicleWorldState._fetch = () => Promise.resolve({ ok: false, status: 503 });
  const p = w.ChronicleWorldState.get('camp-4');
  let err = null;
  p.onError((e) => { err = e; });
  await assert.rejects(() => p.load(), /world-state 503/);
  assert.ok(err, 'onError fired');
  // A late onError still gets the recorded error.
  let late = null;
  p.onError((e) => { late = e; });
  assert.ok(late, 'late onError subscriber got the recorded error');
});

test('shared rAF: one loop drives all frame subscribers; reduced-motion skips it', () => {
  const w = boot();
  const p = w.ChronicleWorldState.get('camp-5');
  let aTicks = 0, bTicks = 0;
  const un = p.onFrame(() => { aTicks++; });
  p.onFrame(() => { bTicks++; });
  assert.equal(w.__rafLen(), 1, 'exactly one rAF scheduled for both frame subs');
  w.__pump(1);
  assert.equal(aTicks, 1); assert.equal(bTicks, 1);
  un(); // removing one keeps the loop while the other remains
  w.__pump(1);
  assert.equal(bTicks, 2, 'remaining frame sub still ticks');

  // Reduced-motion: no rAF; fn runs once statically.
  const wr = boot({ reduced: true });
  const pr = wr.ChronicleWorldState.get('camp-6');
  let rTicks = 0;
  pr.onFrame(() => { rTicks++; });
  assert.equal(wr.__rafLen(), 0, 'no rAF under reduced motion');
  assert.equal(rTicks, 1, 'frame fn ran once statically');
});

test('last unsubscribe self-destroys the instance', () => {
  const w = boot();
  const p = w.ChronicleWorldState.get('camp-7');
  const u1 = p.subscribe(() => {});
  const u2 = p.subscribe(() => {});
  u1();
  assert.equal(w.ChronicleWorldState.get('camp-7'), p, 'still alive with one sub left');
  u2();
  // After the last unsub, get() returns a fresh instance (old one destroyed).
  assert.notEqual(w.ChronicleWorldState.get('camp-7'), p, 'instance destroyed + recreated');
});

test('push() fans a live update to subscribers', async () => {
  const w = boot();
  w.ChronicleWorldState._fetch = countingFetch(SEED).impl;
  const p = w.ChronicleWorldState.get('camp-8');
  const seen = [];
  p.subscribe((s) => seen.push(s.timeOfDay));
  await p.load();
  p.push({ ...SEED, timeOfDay: 0.9 });
  assert.deepEqual(seen, [0.5, 0.9], 'subscriber saw initial + pushed update');
});
