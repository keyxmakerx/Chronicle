// customize_accent_rules.test.mjs — the campaign colour's refusals as the
// picker applies them: amber is refused and red or green is moved. The rules
// live in customize_look.js's closure, so its colour block is evaluated on
// its own. internal/colour's TestNearAmber and TestOffState hold the same
// cases, so the picker and the server cannot drift apart.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const start = src.indexOf('/* ---------- Colour');
const end = src.indexOf('/* ---------- Data');
assert.ok(start > 0 && end > start, 'colour block markers not found');

const api = vm.runInNewContext(
  '(function(){ function clamp(v, a, b){ return Math.max(a, Math.min(b, v)); }\n' +
  src.slice(start, end) + '\nreturn { nearAmber: nearAmber, offState: offState, accentTame: accentTame }; })()');

test('amber range is refused and its neighbours are not', () => {
  for (const [hex, want] of [
    ['#f59e0b', true], ['#f97316', true], ['#a16207', true], ['#b45309', true],
    ['#c2410c', false], ['#9a4a26', false], ['#84cc16', false], ['#6366f1', false],
    ['#8a8350', false], ['#5c3d00', false],
  ]) assert.equal(api.nearAmber(hex), want, hex);
});

test('red and green move off the state colours, others stay put', () => {
  for (const [hex, moved] of [
    ['#dc2626', true], ['#16a34a', true], ['#f43f5e', true],
    ['#6366f1', false], ['#10b981', false], ['#c2410c', false],
  ]) {
    const o = api.offState(hex);
    assert.equal(o.moved, moved, hex);
    if (!moved) assert.equal(o.hex, hex);
  }
});

test('the server and the picker land on the same moved colour', () => {
  // internal/colour: OffState("#dc2626") and OffState("#16a34a")
  assert.equal(api.offState('#dc2626').hex, '#b05f56');
  assert.equal(api.offState('#16a34a').hex, '#6b9472');
});
