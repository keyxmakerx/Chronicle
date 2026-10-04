// notes_tab_feedback.test.mjs — pins that a click on a jot panel tab always
// shows something: the open tab reloads with the loading line, and a failed
// load says so instead of claiming there are no jots. notes.js is a browser
// IIFE, so these pin the wiring by source contract, as notes_live.test.mjs does.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}

const src = strip(readFileSync(join(root, 'static/js/widgets/notes.js'), 'utf8'));

test('clicking the open tab reloads it with the loading line', () => {
  const i = src.indexOf('tabBtns.forEach(function (btn) {');
  assert.ok(i >= 0, 'tab click handler must exist');
  const handler = src.slice(i, src.indexOf('\n    });\n', i));
  assert.match(handler, /=== state\.tab && !state\.editingId && !state\.versionsNoteId\)/,
    'only the open tab reloads, and never under an open editor or history');
  assert.match(handler, /state\.loading = true;\s*renderNotes\(\);\s*setTimeout\(loadNotes, \d+\);/,
    'the loading line must show before the reload');
});

test('a failed load is not shown as "no jots"', () => {
  assert.match(src, /state\.loadFailed = true;/, 'the catch must mark the failure');
  assert.match(src, /state\.loadFailed = false;/, 'a good load must clear it');
  assert.match(src, /var emptyMsg = state\.loadFailed \?/, 'the empty line must check the failure first');
});
