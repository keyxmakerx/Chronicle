// sky_dials.test.mjs — the one list of the sky's dials (SkyLooks.DIALS in
// static/js/widgets/sky_looks.js).
//
// Every weather is a set of targets for these dials, so a change between any
// two weathers needs no transition of its own (#945). These tests pin that the
// list is the single source the rest of the sky reads: an empty sky is the
// dials at rest, a blend moves every dial and nothing else, and the dials the
// pane glides are exactly the ones with an ease.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const dir = join(here, '..', '..', 'static', 'js', 'widgets');

function load() {
  const sandbox = { console, Math, JSON, Object, Array, Number, String, Float32Array, Uint8Array, Uint8ClampedArray, Error };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['sky_world.js', 'sky_looks.js']) vm.runInContext(readFileSync(join(dir, f), 'utf8'), sandbox, { filename: f });
  return sandbox.SkyLooks;
}

test('every dial has a name, a resting value, an ease in seconds or none, and a plain description', () => {
  const L = load();
  const seen = new Set();
  for (const d of L.DIALS) {
    assert.ok(/^[a-z]+$/.test(d.n), 'name ' + d.n);
    assert.ok(!seen.has(d.n), 'no dial listed twice: ' + d.n); seen.add(d.n);
    assert.equal(typeof d.rest, 'number', d.n + ' rests at a number');
    assert.ok(d.ease === null || d.ease > 0, d.n + ' eases over a positive time or not at all');
    assert.ok(d.what && d.what.length > 8, d.n + ' says what it does');
  }
});

test('an empty sky is every dial at rest', () => {
  const L = load(), R = L.blank();
  for (const d of L.DIALS) assert.equal(R[d.n], d.rest, d.n);
});

test('a blend moves every dial in proportion, and a weather sets every dial', () => {
  const L = load();
  const A = L.resolve(L.spec({ preset_id: 'clear', preset_label: 'Clear' }));
  const B = L.resolve(L.spec({ preset_id: 'thunderstorm', preset_label: 'Thunderstorm' }));
  const M = L.blend(A, B, .25);
  for (const n of L.DIAL_NAMES) {
    assert.equal(typeof A[n], 'number', 'clear sets ' + n);
    assert.ok(Math.abs(M[n] - (A[n] * .75 + B[n] * .25)) < 1e-12, n + ' is a quarter of the way');
  }
});

test('the dials the pane glides between weathers are the ones with an ease', () => {
  const src = readFileSync(join(dir, 'sky_pane.js'), 'utf8');
  assert.match(src, /LOOKS\.DIALS\.filter\(function \(d\) \{ return d\.ease; \}\)/, 'the pane takes its glide list from the dials');
  assert.doesNotMatch(src, /\['cloud', 'dark'/, 'and keeps no list of its own');
});
