// local_name.test.mjs — Chronicle.localName (static/js/boot.js) must rename
// exactly what the server's fieldrules.Rename does, or a shop's coins and a
// page's fields would disagree about a word's local name.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'boot.js'), 'utf8');

// boot.js wires the whole page on load, so only the helper is run here.
function loadLocalName() {
  const start = src.indexOf('Chronicle.localName = function');
  assert.ok(start >= 0, 'Chronicle.localName is defined in boot.js');
  const end = src.indexOf('\n  };', start);
  const sandbox = { Chronicle: {} };
  vm.createContext(sandbox);
  vm.runInContext(src.slice(start, end + 4), sandbox);
  return sandbox.Chronicle.localName;
}
const localName = loadLocalName();

test('localName matches the server rule', () => {
  const words = { mark: 'Grain', marks: 'Grains', copper: 'Rock' };
  const cases = [
    ['Mark', { text: 'Grain', usual: 'Mark' }],
    ['MARK', { text: 'Grain', usual: 'MARK' }],
    ['Marks', { text: 'Grains', usual: 'Marks' }], // an exact plural key beats the singular
    ['Coppers', { text: 'Rock', usual: 'Coppers' }], // plural made with "s" finds the singular
    ['Silver', { text: 'Silver', usual: '' }],
    ['', { text: '', usual: '' }],
    [null, { text: '', usual: '' }],
  ];
  for (const [input, want] of cases) {
    assert.deepEqual({ ...localName(words, input) }, want, String(input));
  }
  assert.deepEqual({ ...localName(null, 'Mark') }, { text: 'Mark', usual: '' });
  assert.deepEqual({ ...localName({ constructor: 'x' }, 'constructor') }, { text: 'x', usual: 'constructor' });
});
