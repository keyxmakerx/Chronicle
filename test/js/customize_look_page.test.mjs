// customize_look_page.test.mjs — the Pages setting in customize_look.js
// offers exactly the page looks the server accepts, is covered by
// Save/Discard/Reset (SEC_KEYS), and travels in the Save payload.

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

const a = src.indexOf('var PAGE_LOOKS = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'PAGE_LOOKS marker not found');
const looks = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(PAGE_LOOKS)'));

test('PAGE_LOOKS match the server allow-list, in order', () => {
  const m = go.match(/AppearancePageLooks\s*=\s*\[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearancePageLooks not found in appearance.go');
  const ids = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  assert.deepEqual(looks.map((l) => l[0]), ids);
  // Annals is the Material page look; the owner sees that name.
  assert.deepEqual(looks.find((l) => l[0] === 'annals'), ['annals', 'Material']);
});

test('the page look has its own section and is part of Save, Discard and Reset', () => {
  assert.match(src, /\{ id:'page', name:'Pages'/, 'SECTIONS must list Pages');
  assert.match(src, /page:\['page\.look'\]/, 'SEC_KEYS must list page.look');
  assert.match(src, /sheet:clone\(d\.sheet\), page:clone\(d\.page\)/, 'toInput must send page');
  assert.match(src, /if \(!d\.page\) d\.page = \{ look:'' \}/, 'fromServer must keep an unset page look unset');
  assert.match(src, /opts\('pg-look', 'page\.look'/, 'a control must bind page.look');
});
