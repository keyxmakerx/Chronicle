// customize_look_book.test.mjs — the Rulebook setting in customize_look.js
// offers exactly the looks the server accepts, is covered by
// Save/Discard/Reset (SEC_KEYS), travels in the Save payload, and the book
// reader honours the attribute the layout writes for it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const reader = readFileSync(join(root, 'static', 'js', 'widgets', 'rulebook.js'), 'utf8');
const go = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');
const layout = readFileSync(join(root, 'internal', 'templates', 'layouts', 'appearance.go'), 'utf8');

const a = src.indexOf('var BOOK_LOOKS = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'BOOK_LOOKS marker not found');
const looks = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(BOOK_LOOKS)'));

test('BOOK_LOOKS match the server allow-list, in order', () => {
  const m = go.match(/AppearanceBookLooks\s*=\s*\[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearanceBookLooks not found in appearance.go');
  const ids = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  assert.deepEqual(looks.map((s) => s[0]), ids);
  assert.equal(ids[0], 'book');
  looks.forEach((s) => assert.ok(s[1].length > 0, s[0] + ' needs a display name'));
});

test('the rulebook look is part of Save, Discard and Reset', () => {
  assert.match(src, /book:\['book\.look'\]/, 'SEC_KEYS must list book.look');
  assert.match(src, /sheet:clone\(d\.sheet\), book:clone\(d\.book\)/, 'toInput must send book');
  assert.match(src, /if \(!d\.book\) d\.book = \{ look:'book' \}/, 'fromServer must default to book');
  assert.match(src, /'book\.look', 'bk-look-l'/, 'a control must bind book.look');
});

test('the layout and the reader agree on the attribute', () => {
  assert.match(layout, /attrs\["data-cz-book"\] = "standard"/);
  assert.match(reader, /getAttribute\('data-cz-book'\) === 'standard'/);
});
