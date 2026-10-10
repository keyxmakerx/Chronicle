// customize_look_sheet.test.mjs — the Character sheets setting in
// customize_look.js offers exactly the styles the server accepts, is
// covered by Save/Discard/Reset (SEC_KEYS), and travels in the Save payload.

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

const a = src.indexOf('var SHEET_STYLES = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'SHEET_STYLES marker not found');
const styles = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(SHEET_STYLES)'));

test('SHEET_STYLES match the server allow-list, in order', () => {
  const m = go.match(/AppearanceSheetStyles\s*=\s*\[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearanceSheetStyles not found in appearance.go');
  const ids = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  assert.deepEqual(styles.map((s) => s[0]), ids);
  assert.equal(ids[0], 'modern');
  styles.forEach((s) => assert.ok(s[1].length > 0, s[0] + ' needs a display name'));
});

test('the sheet style is part of Save, Discard and Reset', () => {
  assert.match(src, /sheet:\['sheet\.style'\]/, 'SEC_KEYS must list sheet.style');
  assert.match(src, /hover:clone\(d\.hover\), sheet:clone\(d\.sheet\)/, 'toInput must send sheet');
  assert.match(src, /if \(!d\.sheet\) d\.sheet = \{ style:'modern' \}/, 'fromServer must default to modern');
  assert.match(src, /data-k="sheet\.style"|'sheet\.style', 'sh-style-l'/, 'a control must bind sheet.style');
});
