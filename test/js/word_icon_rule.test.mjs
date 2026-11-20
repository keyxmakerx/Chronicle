// word_icon_rule.test.mjs: an icon that repeats a button's word is drawn only
// when the campaign asks for icons everywhere, and the sidebar never takes
// the mark, because its icons are landmarks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

test('a word icon is hidden unless the campaign shows icons everywhere', () => {
  const css = read('static/css/input.css').replace(/\s+/g, ' ');
  assert.ok(css.includes(':root:not([data-cz-icons="all"]) .word-ic { display: none; }'));
});

test('the sidebar keeps its icons at every step', () => {
  assert.ok(!read('internal/templates/layouts/nav.templ').includes('word-ic'));
});

test('the Customizer says what Everywhere does', () => {
  assert.ok(read('static/js/widgets/customize_look.js').includes('Everywhere also keeps the icon on buttons and menu rows that have a word.'));
});
