// tailwind_tokens.test.mjs — every semantic colour token (fg, surface, page,
// edge) must take an opacity modifier. A token written as a bare var() has no
// <alpha-value> slot, so `bg-surface/90` silently emits no CSS and the element
// loses its background.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

function loadConfig() {
  const src = readFileSync(new URL('../../tailwind.config.js', import.meta.url), 'utf8');
  const module = { exports: {} };
  // The plugins are not needed to read the colour table.
  vm.runInNewContext(src, { module, exports: module.exports, require: () => ({}) });
  return module.exports;
}

const colors = loadConfig().theme.extend.colors;
const tokens = [
  ['fg', colors.fg],
  ['surface', colors.surface],
  ['page', { DEFAULT: colors.page }],
  ['edge', colors.edge],
];

for (const [group, shades] of tokens) {
  for (const [shade, value] of Object.entries(shades)) {
    const name = shade === 'DEFAULT' ? group : `${group}-${shade}`;

    test(`${name} takes an opacity modifier`, () => {
      assert.equal(typeof value, 'function', `${name} must be wrapped in token()`);
      const mixed = value({ opacityValue: '0.9' });
      assert.match(mixed, /^color-mix\(in srgb, var\(--[\w-]+\) calc\(0\.9 \* 100%\), transparent\)$/);
    });

    test(`${name} without a modifier is the bare variable`, () => {
      assert.match(value({}), /^var\(--[\w-]+\)$/);
      assert.match(value({ opacityValue: 'var(--tw-bg-opacity)' }), /^var\(--[\w-]+\)$/);
    });
  }
}
