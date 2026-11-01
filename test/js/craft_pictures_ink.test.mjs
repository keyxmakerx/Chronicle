// craft_pictures_ink.test.mjs — the ink-sketch style of the crafting pictures
// (static/js/widgets/craft_pictures.js), which the paper journal and the paper
// shop draw with.
//
// Pins that every drawing comes out in ink with no colour shading left over,
// that every url(#…) it uses is defined in its own markup (many share a page),
// that the shelf size drops the hatching and wobble that would turn to mud,
// and that drawing in ink leaves the shaded style untouched afterwards.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const window = {};
vm.runInContext(readFileSync(join(root, 'static/js/widgets/craft_pictures.js'), 'utf8'), vm.createContext({ window }));
const CP = window.CraftPictures;
const keys = CP.LIST.map((p) => p.key);

function refsDefined(markup) {
  const ids = new Set([...markup.matchAll(/id="([^"]+)"/g)].map((m) => m[1]));
  return [...markup.matchAll(/url\(#([^)]+)\)/g)].map((m) => m[1]).filter((r) => !ids.has(r));
}

test('every drawing has an ink version whose references all resolve', () => {
  assert.ok(keys.length >= 40, 'the whole library: ' + keys.length);
  for (const k of keys) {
    for (const small of [false, true]) {
      const s = CP.inner(k, '#b8482e', { style: 'ink', small });
      assert.deepEqual(refsDefined(s), [], k + (small ? ' (small)' : '') + ' references something it does not define');
      assert.doesNotMatch(s, /rgb\(0 0 0 \//, k + ' keeps a black shadow fill instead of hatching');
    }
  }
});

test('the shelf size has no hatching lines and no wobble; the full size has both', () => {
  const big = CP.inner('flask', '#3f7fb8', { style: 'ink' }), small = CP.inner('flask', '#3f7fb8', { style: 'ink', small: true });
  assert.match(big, /feTurbulence/);
  assert.match(big, /patternTransform="rotate/);
  assert.doesNotMatch(small, /feTurbulence/);
  assert.doesNotMatch(small, /patternTransform/);
  assert.match(small, /stroke-width="2"/, 'the shelf size has the bolder line');
});

test('two ink drawings on one page never share an id', () => {
  const a = CP.inner('gem', '#8a4fb0', { style: 'ink' }), b = CP.inner('gem', '#8a4fb0', { style: 'ink' });
  const ids = (s) => [...s.matchAll(/id="([^"]+)"/g)].map((m) => m[1]);
  const shared = ids(a).filter((i) => ids(b).includes(i));
  assert.deepEqual(shared, []);
});

test('drawing in ink leaves the shaded style as it was', () => {
  const before = CP.inner('vial', '#6a9a3a').replace(/cpx\d+-/g, 'U');
  CP.inner('vial', '#6a9a3a', { style: 'ink', small: true });
  assert.equal(CP.inner('vial', '#6a9a3a').replace(/cpx\d+-/g, 'U'), before);
});
