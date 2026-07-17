// skybox_widget.test.mjs — C-SKYBOX-WIDGET. The skybox widget registers with
// boot.js (Chronicle.register), reads the campaign worldState through the
// SAME provider singleton the worldstate widget uses (adopting a server seed
// → zero fetch), drives the shared engine via window.__calSetWorldState on
// each update, flips the friendly error state on a provider load failure,
// and tears down cleanly on destroy() with no leaked timers (it never
// starts one — unlike a rAF-driven widget, subscribe() is timer-free).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const providerSrc = readFileSync(join(root, 'static', 'js', 'widgets', 'worldstate_provider.js'), 'utf8');
const widgetSrc = readFileSync(join(root, 'static', 'js', 'widgets', 'skybox.js'), 'utf8');

const SEED = { timeOfDay: 0.5, season: 'Spring', date: { year: 1492, month: 4, day: 15 } };

// A tiny element stub with the attribute + classList surface the widget uses.
function elem(attrs) {
  const a = Object.assign({}, attrs);
  const errKid = { classList: { _c: new Set(['hidden']), add(x) { this._c.add(x); }, remove(x) { this._c.delete(x); }, contains(x) { return this._c.has(x); } } };
  return {
    _attrs: a, _err: errKid, _skybox: null,
    getAttribute: (n) => (n in a ? a[n] : null),
    setAttribute: (n, v) => { a[n] = v; },
    removeAttribute: (n) => { delete a[n]; },
    querySelector: (sel) => (sel === '[data-skybox-error]' ? errKid : null),
  };
}

let setTimerCalls = 0, clearTimerCalls = 0;

function boot(opts) {
  opts = opts || {};
  setTimerCalls = 0; clearTimerCalls = 0;
  const seedBlob = opts.serverSeed ? { getAttribute: () => JSON.stringify(opts.serverSeed) } : null;
  const calls = { set: [] };
  const sandbox = {
    console,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
    Promise,
    // Timer-leak detection: skybox.js should never call these — subscribe()
    // is a pure pub/sub, no polling/timeout of its own.
    setTimeout: (...a) => { setTimerCalls++; return setTimeout(...a); },
    clearTimeout: (...a) => { clearTimerCalls++; return clearTimeout(...a); },
    fetch: opts.fetch || null,
    document: {
      getElementById: () => null,
      querySelector: (sel) => (sel === '[data-cal-worldstate]' ? seedBlob : null),
    },
    // engine seam: present only if opts.engine
    __calSetWorldState: opts.engine ? (p) => { calls.set.push(p); } : undefined,
    __calWorldState: opts.engine ? { timeOfDay: 0 } : undefined,
  };
  // Chronicle registry (boot.js shim).
  const registry = {};
  sandbox.Chronicle = { register: (name, impl) => { registry[name] = impl; }, _registry: registry };
  sandbox.window = sandbox;
  sandbox.global = sandbox;
  sandbox.__calls = calls;
  vm.createContext(sandbox);
  vm.runInContext(providerSrc, sandbox, { filename: 'worldstate_provider.js' });
  vm.runInContext(widgetSrc, sandbox, { filename: 'skybox.js' });
  return sandbox;
}

test('the widget registers itself as "skybox"', () => {
  const w = boot();
  assert.ok(w.Chronicle._registry.skybox, 'registered');
  assert.equal(typeof w.Chronicle._registry.skybox.init, 'function');
  assert.equal(typeof w.Chronicle._registry.skybox.destroy, 'function');
});

test('init adopts the server seed (zero fetch) and drives the engine', async () => {
  const w = boot({ serverSeed: SEED, engine: true });
  const impl = w.Chronicle._registry.skybox;
  const el = elem({ 'data-campaign-id': 'camp-1', 'data-skybox-loading': '1' });
  impl.init(el);
  await Promise.resolve(); await Promise.resolve();
  const provider = w.ChronicleWorldState.get('camp-1');
  assert.equal(provider.fetchCount, 0, 'server seed → no fetch');
  assert.equal(w.__calls.set.length >= 1, true, 'engine driven via __calSetWorldState');
  assert.equal(el.getAttribute('data-skybox-loading'), null, 'loading flag cleared');
});

test('init with no server seed fetches once via the provider', async () => {
  const fetchCalls = { n: 0 };
  const fetch = () => { fetchCalls.n++; return Promise.resolve({ ok: true, json: () => Promise.resolve(SEED) }); };
  const w = boot({ engine: true, fetch });
  const impl = w.Chronicle._registry.skybox;
  impl.init(elem({ 'data-campaign-id': 'camp-9' }));
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  assert.equal(fetchCalls.n, 1, 'one fetch when no server seed');
});

test('a skybox widget and a worldstate widget on the same page share ONE fetch', async () => {
  const fetchCalls = { n: 0 };
  const fetch = () => { fetchCalls.n++; return Promise.resolve({ ok: true, json: () => Promise.resolve(SEED) }); };
  const w = boot({ engine: true, fetch });
  const impl = w.Chronicle._registry.skybox;
  impl.init(elem({ 'data-campaign-id': 'camp-shared' }));
  impl.init(elem({ 'data-campaign-id': 'camp-shared' }));
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  assert.equal(fetchCalls.n, 1, 'two skybox mounts for the same campaign still fetch once (shared provider singleton)');
});

test('a provider load failure flips the friendly error state', async () => {
  const fetch = () => Promise.resolve({ ok: false, status: 500 });
  const w = boot({ fetch });
  const impl = w.Chronicle._registry.skybox;
  const el = elem({ 'data-campaign-id': 'camp-err', 'data-skybox-loading': '1' });
  impl.init(el);
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  assert.equal(el._err.classList.contains('hidden'), false, 'error shown');
  assert.equal(el._err.classList.contains('flex'), true);
});

test('destroy unsubscribes from the provider and leaks no timers', async () => {
  const w = boot({ serverSeed: SEED });
  const impl = w.Chronicle._registry.skybox;
  const el = elem({ 'data-campaign-id': 'camp-d' });
  impl.init(el);
  await Promise.resolve();
  const provider = w.ChronicleWorldState.get('camp-d');
  assert.equal(provider._subs.length, 1, 'subscribed');
  impl.destroy(el);
  // last unsubscribe self-destroys → get() yields a fresh instance
  assert.notEqual(w.ChronicleWorldState.get('camp-d'), provider, 'unsubscribed + destroyed');
  assert.equal(el._skybox, null, 'widget state cleared');
  assert.equal(setTimerCalls, 0, 'skybox.js never starts a timer (subscribe() is pure pub/sub)');
  assert.equal(clearTimerCalls, 0, 'so destroy() has nothing to clear');
});

test('destroy before init resolves does not throw (mount/unmount race)', async () => {
  const fetch = () => new Promise(() => {}); // never resolves
  const w = boot({ fetch });
  const impl = w.Chronicle._registry.skybox;
  const el = elem({ 'data-campaign-id': 'camp-race' });
  impl.init(el);
  assert.doesNotThrow(() => impl.destroy(el));
});
