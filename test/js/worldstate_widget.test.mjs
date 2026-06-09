// worldstate_widget.test.mjs — C-CAL-WORLDSTATE-WIDGETS. The worldstate widget
// registers with boot.js (Chronicle.register), reads the campaign worldState
// through the provider singleton (adopting a server seed → zero fetch), drives
// the shared engine via window.__calSetWorldState on each update, flips the
// friendly error state on a provider load failure, and tears down cleanly on
// destroy().

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const providerSrc = readFileSync(join(root, 'static', 'js', 'widgets', 'worldstate_provider.js'), 'utf8');
const widgetSrc = readFileSync(join(root, 'static', 'js', 'widgets', 'worldstate.js'), 'utf8');

const SEED = { timeOfDay: 0.5, season: 'Spring', date: { year: 1492, month: 4, day: 15 } };

// A tiny element stub with the attribute + classList surface the widget uses.
function elem(attrs) {
  const a = Object.assign({}, attrs);
  const errKid = { classList: { _c: new Set(['hidden']), add(x) { this._c.add(x); }, remove(x) { this._c.delete(x); }, contains(x) { return this._c.has(x); } } };
  return {
    _attrs: a, _err: errKid, _worldstate: null,
    getAttribute: (n) => (n in a ? a[n] : null),
    setAttribute: (n, v) => { a[n] = v; },
    removeAttribute: (n) => { delete a[n]; },
    querySelector: (sel) => (sel === '[data-worldstate-error]' ? errKid : null),
  };
}

function boot(opts) {
  opts = opts || {};
  const seedBlob = opts.serverSeed ? { getAttribute: () => JSON.stringify(opts.serverSeed) } : null;
  const calls = { set: [] };
  const sandbox = {
    console,
    matchMedia: () => ({ matches: false }),
    requestAnimationFrame: () => 0, cancelAnimationFrame: () => {},
    Promise, setTimeout, clearTimeout,
    fetch: opts.fetch || null,
    document: {
      getElementById: (id) => (id === 'cal-v2-worldstate' ? seedBlob : null),
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
  vm.runInContext(widgetSrc, sandbox, { filename: 'worldstate.js' });
  return sandbox;
}

test('the widget registers itself as "worldstate"', () => {
  const w = boot();
  assert.ok(w.Chronicle._registry.worldstate, 'registered');
  assert.equal(typeof w.Chronicle._registry.worldstate.init, 'function');
  assert.equal(typeof w.Chronicle._registry.worldstate.destroy, 'function');
});

test('init adopts the server seed (zero fetch) and drives the engine', async () => {
  const w = boot({ serverSeed: SEED, engine: true });
  const impl = w.Chronicle._registry.worldstate;
  const el = elem({ 'data-campaign-id': 'camp-1', 'data-variant': 'hourglass', 'data-worldstate-loading': '1' });
  impl.init(el);
  // let the resolved provider promise flush
  await Promise.resolve(); await Promise.resolve();
  const provider = w.ChronicleWorldState.get('camp-1');
  assert.equal(provider.fetchCount, 0, 'server seed → no fetch');
  assert.equal(w.__calls.set.length >= 1, true, 'engine driven via __calSetWorldState');
  assert.equal(el.getAttribute('data-worldstate-loading'), null, 'loading flag cleared');
});

test('init with no server seed fetches once via the provider', async () => {
  const fetchCalls = { n: 0 };
  const fetch = (url) => { fetchCalls.n++; return Promise.resolve({ ok: true, json: () => Promise.resolve(SEED) }); };
  const w = boot({ engine: true, fetch });
  const impl = w.Chronicle._registry.worldstate;
  impl.init(elem({ 'data-campaign-id': 'camp-9' }));
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  assert.equal(fetchCalls.n, 1, 'one fetch when no server seed');
});

test('a provider load failure flips the friendly error state', async () => {
  const fetch = () => Promise.resolve({ ok: false, status: 500 });
  const w = boot({ fetch });
  const impl = w.Chronicle._registry.worldstate;
  const el = elem({ 'data-campaign-id': 'camp-err', 'data-worldstate-loading': '1' });
  impl.init(el);
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
  assert.equal(el._err.classList.contains('hidden'), false, 'error shown');
  assert.equal(el._err.classList.contains('flex'), true);
});

test('destroy unsubscribes from the provider', async () => {
  const w = boot({ serverSeed: SEED });
  const impl = w.Chronicle._registry.worldstate;
  const el = elem({ 'data-campaign-id': 'camp-d' });
  impl.init(el);
  await Promise.resolve();
  const provider = w.ChronicleWorldState.get('camp-d');
  assert.equal(provider._subs.length, 1, 'subscribed');
  impl.destroy(el);
  // last unsubscribe self-destroys → get() yields a fresh instance
  assert.notEqual(w.ChronicleWorldState.get('camp-d'), provider, 'unsubscribed + destroyed');
  assert.equal(el._worldstate, null, 'widget state cleared');
});
