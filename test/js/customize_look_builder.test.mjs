// customize_look_builder.test.mjs — the hero builder look in customize_look.js
// offers exactly the skins the server accepts, sits in Character sheets, is
// covered by Save, Discard and Reset (SEC_KEYS), and travels in the Save payload.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const server = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'appearance.go'), 'utf8');

const a = src.indexOf('var BUILDER_LOOKS = [');
const a2 = src.indexOf('];', a);
assert.ok(a > 0 && a2 > a, 'BUILDER_LOOKS marker not found');
const looks = JSON.parse(vm.runInNewContext(src.slice(a, a2 + 2) + ';JSON.stringify(BUILDER_LOOKS)'));

test('BUILDER_LOOKS match the server list, Plain first', () => {
  const m = server.match(/AppearanceBuilderLooks = \[\]string\{([^}]*)\}/);
  assert.ok(m, 'AppearanceBuilderLooks not found');
  const ids = m[1].split(',').map((x) => x.trim().replace(/"/g, ''));
  assert.deepEqual(looks.map((l) => l[0]), ids);
  assert.equal(ids[0], 'plain');
  assert.deepEqual(looks.map((l) => l[1]), ['Plain', 'Paper', 'Ornate']);
});

test('the control is in Character sheets and covered by Save, Discard and Reset', () => {
  assert.match(src, /sheet:\['sheet\.style', 'sheet\.craftArt', 'builder\.look', 'access\.sheet'\]/, 'SEC_KEYS must list builder.look');
  assert.match(src, /builder:clone\(d\.builder\)/, 'toInput must send the look');
  assert.match(src, /if \(!d\.builder \|\| !d\.builder\.look\) d\.builder = \{ look:'plain' \};/, 'a missing look reads as Plain');
  assert.match(src, /opts\('sh-builder', 'builder\.look', 'sh-builder-l', BUILDER_LOOKS\)/, 'a control must bind builder.look');
  assert.match(src, /'Hero builder look'/);
  assert.match(src, /How the hero builder looks to your players\./);
});
