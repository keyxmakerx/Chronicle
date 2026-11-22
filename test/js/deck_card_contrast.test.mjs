// deck_card_contrast.test.mjs: the card's name must read at 4.5 to 1 on the
// plate of every border, in the light and the dark page. The page-border plates
// take Chronicle's own tokens; the five forged ones are fixed materials (a
// forged plate looks the same in light and dark), so their ink and field come
// from the MATERIALS block at the top of static/css/decks_card.css. A name
// over a picture sits on a band that must be at least 0.85 opaque.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const chip = require('../../static/js/chip_ink.js');
const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const css = readFileSync(join(root, 'static', 'css', 'decks_card.css'), 'utf8');
const tokens = readFileSync(join(root, 'static', 'css', 'input.css'), 'utf8');

// The first value of a token is the light page's, the next is the dark page's.
function token(name) {
  const all = [...tokens.matchAll(new RegExp('--' + name + ':\\s*(#[0-9a-fA-F]{6})\\b', 'g'))].map((m) => m[1]);
  assert.ok(all.length >= 2, `${name} needs a light and a dark value in input.css`);
  return { light: all[0], dark: all[1] };
}
function material(name) {
  const m = css.match(new RegExp('--dk-' + name + ':\\s*(#[0-9a-fA-F]{6})\\b'));
  assert.ok(m, `--dk-${name} is not a plain colour in the MATERIALS block`);
  return m[1];
}
const need = (got, what) => assert.ok(got >= 4.5, `${what} is ${got.toFixed(2)}:1`);

test('the name reads on the plate of the page borders, light and dark', () => {
  const ink = token('color-text-primary');
  const plate = token('color-bg-secondary');
  for (const theme of ['light', 'dark']) {
    need(chip.contrast(ink[theme], plate[theme]), `name on a double, line, kit or no-border plate (${theme})`);
  }
});

test('the name reads on each forged plate', () => {
  for (const m of ['iron', 'brass', 'steel', 'stone']) {
    need(chip.contrast(material(m + '-ink'), material(m + '-field')), `name on ${m}`);
    need(chip.contrast(material(m + '-ink2'), material(m + '-field')), `words on ${m}`);
  }
});

test('the gold of a rare card reads on every plate', () => {
  const gold = material('gold-ink');
  const goldNight = material('gold-ink-night');
  const plate = token('color-bg-secondary');
  need(chip.contrast(gold, plate.light), 'gold name on the light plate');
  need(chip.contrast(goldNight, plate.dark), 'gold name on the dark plate');
  for (const m of ['iron', 'brass', 'steel', 'stone']) need(chip.contrast(goldNight, material(m + '-field')), `gold name on ${m}`);
});

test('a name over a picture sits on a band that is at least 0.85 opaque and reads on the worst picture', () => {
  const m = css.match(/--dk-band:\s*rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)/);
  assert.ok(m, 'no --dk-band');
  const a = parseFloat(m[4]);
  assert.ok(a >= 0.85, `band opacity ${a}`);
  const hex = (n) => Math.round(n).toString(16).padStart(2, '0');
  const band = '#' + [m[1], m[2], m[3]].map((x) => hex(+x)).join('');
  // the whitest picture shows through the least-opaque band as 1 - a of white
  const blended = '#' + [m[1], m[2], m[3]].map((x) => hex(+x * a + 255 * (1 - a))).join('');
  need(chip.contrast('#ffffff', blended), `white name on ${band} at ${a} over white`);
});

test('outside the MATERIALS and PALETTE blocks the stylesheet has no raw colour', () => {
  const a = css.indexOf('---- MATERIALS');
  const b = css.indexOf('---- end of the fixed colours');
  assert.ok(a >= 0 && b > a, 'the fixed-colour blocks are missing their markers');
  const outside = css.slice(0, a) + css.slice(b);
  const raw = outside.match(/#[0-9a-fA-F]{3,8}\b|\b(?:rgb|rgba|hsl|hsla)\(/g) || [];
  assert.deepEqual(raw, [], 'move raw colours into the MATERIALS block or use a token');
});
