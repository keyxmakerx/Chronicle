// creature_fold.test.mjs: the pure helpers of the creature fold-out, which
// decide between the game system's stat block and the page's own fields.
// The file is loaded in a bare vm context, the way the server ships it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'internal', 'plugins', 'encounters', 'static', 'js', 'creature_fold.js'), 'utf8');

function load() {
  const sandbox = { window: {}, console };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return sandbox.window.Chronicle.CreatureFold;
}

const plain = (o) => JSON.parse(JSON.stringify(o));

test('a stat block is used only for the page types it draws', () => {
  const F = load();
  const blocks = [{ widget: 'statblock', systemId: 'drawsteel', typeIds: [10, 1] }];
  assert.equal(F.statBlockFor(blocks, 10).widget, 'statblock');
  assert.equal(F.statBlockFor(blocks, 7), null);
  assert.equal(F.statBlockFor([], 10), null);
  assert.equal(F.statBlockFor(null, 10), null);
  assert.equal(F.statBlockFor([{ widget: 'x' }], 10), null, 'a block without type ids draws for nothing');
});

test('the fields list keeps filled scalar values in the type order', () => {
  const F = load();
  const fields = [
    { key: 'armor_class', label: 'Armor Class', type: 'text' },
    { key: 'hit_points', label: 'Hit Points', type: 'text' },
    { key: 'speed', label: 'Speed', type: 'text' },
    { key: 'notes', label: 'Notes', type: 'richtext' },
    { key: 'str', label: 'STR', type: 'number' },
    { key: 'traits', label: 'Traits', type: 'text' }
  ];
  const data = { hit_points: '9', armor_class: '13', speed: '  ', notes: '<p>x</p>', str: 12, traits: ['a'] };
  assert.deepEqual(plain(F.fieldRows(fields, data)), [
    { label: 'Armor Class', value: '13' },
    { label: 'Hit Points', value: '9' },
    { label: 'STR', value: '12' }
  ]);
  assert.deepEqual(plain(F.fieldRows(null, data)), []);
  assert.deepEqual(plain(F.fieldRows(fields, null)), []);
});
