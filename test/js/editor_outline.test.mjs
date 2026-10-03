// editor_outline.test.mjs — pins when the "On this page" outline shows and
// which headings it lists. Runs the real widget source in a vm sandbox.
//
// The rule that matters for players: the server strips GM-only text before
// the editor loads, so a heading that was entirely secret arrives empty and
// must not leave a blank row (or count toward showing the outline).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor_outline.js'), 'utf8');

function load() {
  const window = {};
  vm.runInNewContext(src, { window });
  return window.Chronicle.EditorOutline;
}

test('lists non-empty headings in order, keeping their source index', () => {
  const O = load();
  const got = O.collect([
    { level: 2, text: 'History' },
    { level: 2, text: '   ' },
    { level: 3, text: ' The  Old\nWall ' },
  ]);
  assert.deepEqual(JSON.parse(JSON.stringify(got)), [
    { index: 0, level: 2, text: 'History' },
    { index: 2, level: 3, text: 'The Old Wall' },
  ]);
});

const cases = [
  { name: 'no headings', items: [], show: false },
  { name: 'two headings', items: ['A', 'B'], show: false },
  { name: 'three headings', items: ['A', 'B', 'C'], show: true },
  { name: 'three, one emptied by secret stripping', items: ['A', '', 'C'], show: false },
];
for (const c of cases) {
  test(`shows only with ${3}+ real headings: ${c.name}`, () => {
    const O = load();
    const entries = O.collect(c.items.map((t) => ({ level: 2, text: t })));
    assert.equal(O.shouldShow(entries), c.show);
  });
}

test('editor.js only attaches the outline when the mount asks for it', () => {
  const editorSrc = readFileSync(join(root, 'static/js/widgets/editor.js'), 'utf8');
  assert.match(editorSrc, /config\.outline === true && Chronicle\.EditorOutline/);
  const show = readFileSync(join(root, 'internal/plugins/entities/show.templ'), 'utf8');
  assert.match(show, /data-outline="true"/);
});
