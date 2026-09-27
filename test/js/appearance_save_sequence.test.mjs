// appearance_save_sequence.test.mjs — pins that appearance_editor.js's Save
// button issues its PUTs one at a time, never concurrently.
//
// Every Update* service method the Save button calls (UpdateAccentSurface,
// UpdateAccentColor, UpdateBranding, ...) is a read-modify-write on the
// campaign's whole settings blob: find, change one field, write the whole
// thing back, with no locking. Two of them in flight together race — the
// one whose write lands second overwrites the first's change with a copy of
// the settings read before it happened. Chaining the PUTs removes the race
// by construction: each step's write is on disk before the next step reads.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'appearance_editor.js'), 'utf8');

// runSaveStepsSequentially has no DOM/Chronicle dependency, so the widget's
// browser IIFE only needs a no-op Chronicle.register stub (it never calls
// init() in this test) plus the module.exports guard the source itself
// checks for.
function loadRunSaveStepsSequentially() {
  const sandbox = { module: { exports: {} }, Chronicle: { register: function () {} }, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const fn = sandbox.module.exports.runSaveStepsSequentially;
  assert.equal(typeof fn, 'function', 'appearance_editor.js must export runSaveStepsSequentially for this test');
  return fn;
}

// namesOf(arr) -- arr is built inside the vm sandbox, a different V8 realm
// than this test file, so its Array/Object prototypes differ from this
// realm's even when the contents match; assert.deepStrictEqual would fail
// on that prototype mismatch alone. Reducing to a sorted, comma-joined
// string of names (primitives, realm-agnostic) sidesteps it.
function namesOf(arr) {
  return Array.from(arr).sort().join(',');
}

// mockStep(name, opts) -- opts.ok (default true), opts.delayMs (default 0),
// opts.reject (default false). Records into `log` (pushed the instant run()
// is CALLED, before any delay) and into `order` (pushed when it RESOLVES),
// and tracks the live concurrency count in `concurrency` so a test can
// assert it never exceeds 1.
function mockStep(name, log, order, concurrency, opts) {
  opts = opts || {};
  return {
    name: name,
    run: function () {
      log.push(name);
      concurrency.active++;
      concurrency.max = Math.max(concurrency.max, concurrency.active);
      return new Promise(function (resolve, reject) {
        setTimeout(function () {
          concurrency.active--;
          order.push(name);
          if (opts.reject) {
            reject(new Error(name + ' failed'));
          } else {
            resolve({ ok: opts.ok !== false });
          }
        }, opts.delayMs || 0);
      });
    }
  };
}

test('steps never run concurrently, even when an earlier one is slower', async () => {
  const log = [];
  const order = [];
  const concurrency = { active: 0, max: 0 };
  const runSaveStepsSequentially = loadRunSaveStepsSequentially();

  // Step 1 is the slowest: if steps ran concurrently (the pre-fix
  // Promise-per-field-fired-at-once shape), step 2 and 3 would both start
  // (and finish) before step 1 resolves, so `order` would NOT come out
  // strictly in call order, and concurrency.max would be > 1.
  const steps = [
    mockStep('surface1', log, order, concurrency, { delayMs: 30 }),
    mockStep('surface2', log, order, concurrency, { delayMs: 5 }),
    mockStep('brandName', log, order, concurrency, { delayMs: 5 }),
  ];

  const result = await runSaveStepsSequentially(steps);

  assert.deepEqual(log, ['surface1', 'surface2', 'brandName'], 'each step is not CALLED until the previous one resolved');
  assert.deepEqual(order, ['surface1', 'surface2', 'brandName'], 'steps resolve in call order');
  assert.equal(concurrency.max, 1, 'at most one step is ever in flight at a time');
  assert.equal(namesOf(result.succeeded), 'brandName,surface1,surface2');
  assert.equal(result.failed.length, 0);
});

test('a failed step does not abort the rest, and both outcomes are reported', async () => {
  const log = [];
  const order = [];
  const concurrency = { active: 0, max: 0 };
  const runSaveStepsSequentially = loadRunSaveStepsSequentially();

  const steps = [
    mockStep('accentColor', log, order, concurrency, { ok: true }),
    mockStep('accentSurface1', log, order, concurrency, { ok: false }), // server 4xx/5xx
    mockStep('accentSurface2', log, order, concurrency, { reject: true }), // network error
    mockStep('fontFamily', log, order, concurrency, { ok: true }),
  ];

  const result = await runSaveStepsSequentially(steps);

  assert.deepEqual(log, ['accentColor', 'accentSurface1', 'accentSurface2', 'fontFamily'],
    'every step still runs after an earlier one fails');
  assert.equal(namesOf(result.succeeded), 'accentColor,fontFamily');
  assert.equal(namesOf(result.failed), 'accentSurface1,accentSurface2',
    'both a non-ok response and a rejected promise count as failed');
});

test('no steps resolves immediately with both lists empty', async () => {
  const runSaveStepsSequentially = loadRunSaveStepsSequentially();
  const result = await runSaveStepsSequentially([]);
  assert.equal(result.succeeded.length, 0);
  assert.equal(result.failed.length, 0);
});
