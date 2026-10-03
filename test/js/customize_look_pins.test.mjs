// customize_look_pins.test.mjs — the eight looks' values in customize_look.js
// match test/js/fixtures/look_pins.json, which the Go table in
// internal/plugins/campaigns/look_presets.go is pinned to as well, so a new
// campaign seeded from Go renders as the editor's preview promised.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const a = src.indexOf('var LOOKS = [');
const b = src.indexOf('function look(id)');
const k = src.indexOf('var LOOK_KEYS = [');
const k2 = src.indexOf('];', k);
assert.ok(a > 0 && b > a && k > 0 && k2 > k, 'LOOKS / LOOK_KEYS markers not found');

const api = JSON.parse(vm.runInNewContext(src.slice(a, b) + src.slice(k, k2 + 2) + ';JSON.stringify({ LOOKS: LOOKS, KEYS: LOOK_KEYS })'));
const get = (o, path) => path.split('.').reduce((x, s) => x[s], o);

test('LOOKS match the pinned fixture', () => {
  const pins = JSON.parse(readFileSync(join(here, 'fixtures', 'look_pins.json'), 'utf8'));
  assert.deepEqual(api.LOOKS.map((l) => l.id), Object.keys(pins));
  for (const l of api.LOOKS) {
    const flat = {};
    api.KEYS.forEach((key) => { flat[key] = get(l, key); });
    assert.deepEqual(flat, pins[l.id], l.id);
  }
});
