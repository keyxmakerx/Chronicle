// widget_pictures.test.mjs — the Customizer's widget drawings: every kind
// draws an inline SVG on the one 56 by 40 grid with only the stylesheet's
// classes, and a kind the script does not know draws "lines".

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'widget_pictures.js'), 'utf8');
const sandbox = {};
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const draw = sandbox.Chronicle.widgetPicture;
const kinds = [...sandbox.Chronicle._widgetPictureKinds];
const allowed = new Set(['i', 'f', 'a', 's', 'sa', 'b', 'b-s', 'thin', 'ring']);

test('there are 15 to 25 kinds, lines among them', () => {
  assert.ok(kinds.length >= 15 && kinds.length <= 25, `${kinds.length} kinds`);
  assert.ok(kinds.includes('lines'));
});

test('each kind is a hidden 56 by 40 SVG using only the stylesheet classes', () => {
  for (const k of kinds) {
    const svg = draw(k);
    assert.match(svg, /^<svg class="wpic" viewBox="0 0 56 40" width="56" height="40" aria-hidden="true" focusable="false">/, k);
    assert.ok(svg.endsWith('</svg>'), k);
    const classes = [...svg.matchAll(/class="([^"]+)"/g)].slice(1).flatMap((m) => m[1].split(' '));
    assert.ok(classes.length > 2, `${k} draws almost nothing`);
    for (const c of classes) assert.ok(allowed.has(c), `${k} uses class ${c}`);
    // Plain text, as the signed mockup draws it, is ink alone.
    if (k !== 'lines') assert.ok(classes.some((c) => c === 'a' || c === 'sa'), `${k} has no accent mark`);
    assert.doesNotMatch(svg, /fill="#|stroke="#|style=/, `${k} carries its own colour`);
  }
});

test('an unknown or missing kind draws lines, and a name never reaches the markup', () => {
  const lines = draw('lines');
  assert.equal(draw('nope'), lines);
  assert.equal(draw(undefined), lines);
  assert.equal(draw('"><script>'), lines);
  assert.equal(draw('constructor'), lines);
  assert.equal(draw('__proto__'), lines);
});
