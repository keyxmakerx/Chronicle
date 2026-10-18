// customize_look_tlpop.test.mjs — the "Popups on time lines" setting in
// customize_look.js offers exactly the popups the server accepts, is covered
// by Save/Discard/Reset (SEC_KEYS), and travels in the Save payload.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const go = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');

const a = src.indexOf('var TL_POPUPS = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'TL_POPUPS marker not found');
const popups = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(TL_POPUPS)'));

test('TL_POPUPS match the server allow-list, in order', () => {
  const m = go.match(/AppearanceTimelinePopups\s*=\s*\[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearanceTimelinePopups not found in appearance.go');
  const ids = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  assert.deepEqual(popups.map((p) => p[0]), ids);
  assert.deepEqual(popups.map((p) => p[1]), ['Classic card', 'Fading corner']);
});

test('the popup is in Hover cards and part of Save, Discard and Reset', () => {
  assert.match(src, /hover:\['hover\.look', 'hover\.timelinePopup'\]/, 'SEC_KEYS must list hover.timelinePopup');
  assert.match(src, /hover:clone\(d\.hover\)/, 'toInput must send hover, which carries the popup');
  assert.match(src, /if \(!d\.hover\.timelinePopup\) d\.hover\.timelinePopup = 'classic';/, 'fromServer must fill a missing popup as the classic card');
  assert.match(src, /data-k="hover\.timelinePopup"/, 'a control must bind hover.timelinePopup');
  assert.match(src, /fld\('h-tl', 'Popups on time lines'/, 'the field must carry the signed name');
});
