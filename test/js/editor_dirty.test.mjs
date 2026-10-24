// editor_dirty.test.mjs — pins that opening or closing the page editor is
// not an edit. TipTap's setEditable emits an update unless told not to, and
// the editor's update handler marks the page dirty, so a bare Edit press
// showed "Unsaved changes" on a page nobody had touched.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor.js'), 'utf8');

test('every setEditable call in the page editor suppresses the update event', () => {
  const calls = src.match(/\.setEditable\([^)]*\)/g) || [];
  assert.ok(calls.length >= 2, 'expected the enter and exit edit-mode calls');
  for (const c of calls) {
    assert.match(c, /,\s*false\)$/, `${c} would fire an update and mark the page dirty`);
  }
});

test('the bundled TipTap still takes the emitUpdate argument', () => {
  const bundle = readFileSync(join(root, 'static/vendor/tiptap-bundle.min.js'), 'utf8');
  assert.match(bundle, /setEditable\(\w+,\w+=!0\)/);
});
