// design_suggest.test.mjs — the pure parts of static/js/widgets/design_library_suggest.js:
// what the fold says the file holds, the request it sends, and the note shown
// after an export. The server builds the file; these only word and shape it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(root, 'static/js/widgets/design_library_suggest.js'), 'utf8'), sandbox);
const U = sandbox.DesignSuggest.util;
const plain = (o) => JSON.parse(JSON.stringify(o));

test('the contents list changes with the credit tick and never names goods or prices as included', () => {
  const on = U.contents({ system: 'drawsteel', credit: true, you: 'Alice' });
  const off = U.contents({ system: 'drawsteel', credit: false, you: 'Alice' });
  assert.match(on.join('\n'), /your name \(Alice\)/);
  assert.match(off.join('\n'), /Your name is not included/);
  assert.doesNotMatch(off.join('\n'), /Alice/);
  for (const l of [on, off]) {
    assert.match(l[1], /game system \(drawsteel\)/);
    assert.equal(l[l.length - 1], 'No goods, prices or pictures');
  }
});

test('the request body trims, caps the note at 240 and sends a real boolean', () => {
  const cases = [
    [['  hi  ', true], { note: 'hi', credit: true }],
    [[undefined, 0], { note: '', credit: false }],
    [['x'.repeat(300), 'yes'], { note: 'x'.repeat(240), credit: true }],
  ];
  for (const [[note, credit], want] of cases) assert.deepEqual(plain(U.requestBody(note, credit)), want);
});

test('the exported note names the file, the system and the maintainer', () => {
  const t = U.exportedNote('drawsteel-design-x.json', 'Draw Steel');
  assert.match(t, /^Exported drawsteel-design-x\.json\./);
  assert.match(t, /Send it to the Draw Steel package maintainer/);
  assert.match(t, /From Draw Steel on every Draw Steel campaign after their next release/);
});

test('the file text is pretty JSON that reads back the same', () => {
  const s = { format: 'chronicle-design-suggestion:v1', designs: [{ name: 'A' }], data: { 'designs/a.json': { part: 'oak' } } };
  assert.deepEqual(JSON.parse(U.fileText(s)), s);
  assert.ok(U.fileText(s).endsWith('}\n'));
});
