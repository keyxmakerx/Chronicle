// rulebook_hues.test.mjs — every chapter colour of the bold look carries
// white page numbers and buttons, so each must hold 4.5:1 with white.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'rulebook.js'), 'utf8');
const m = /var CHAPTER_HUES = \[([^\]]+)\]/.exec(src);
assert.ok(m, 'CHAPTER_HUES not found');
const hues = m[1].match(/#[0-9a-fA-F]{6}/g);

const lum = (hex) => {
  const c = [1, 3, 5].map((i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
const withWhite = (hex) => 1.05 / (lum(hex) + 0.05);

test('the list is read in full', () => {
  assert.ok(hues.length >= 8, String(hues.length));
});

for (const hex of new Set(hues)) {
  test(`${hex} holds 4.5:1 with white`, () => {
    assert.ok(withWhite(hex) >= 4.5, `${hex} is ${withWhite(hex).toFixed(2)}:1`);
  });
}
