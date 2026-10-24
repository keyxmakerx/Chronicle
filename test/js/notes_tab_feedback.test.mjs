// notes_tab_feedback.test.mjs — pins that a click on a jot panel tab always
// shows something: the open tab reloads with the loading line, and a failed
// load says so, with Try again, instead of claiming there are no jots. notes.js is a browser
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
  const fail = src.indexOf('if (state.loadFailed) {');
  const empty = src.indexOf('var emptyMsg =');
  assert.ok(fail >= 0 && empty > fail, 'the failure must be checked before the empty line');
  const branch = src.slice(fail, empty);
  assert.match(branch, /data-jot-retry/, 'a failed load offers Try again');
  assert.match(branch, /addEventListener\('click', function \(\) \{ loadNotes\(\); \}\)/, 'Try again reloads');
});

test('away from a page the tab waits for the viewer to have jots of their own', () => {
  const i = src.indexOf('function loadCount() {');
  assert.ok(i >= 0, 'loadCount must exist');
  const body = src.slice(i, src.indexOf('\n    }\n', i));
  assert.match(body, /if \(!entityId\) \{[\s\S]*fab\.hidden = true;/,
    'off a page the tab starts hidden: a jot cannot be written there');
  assert.match(body, /fab\.hidden = mine\.length === 0;/,
    'it shows once the viewer has jots to read');
});
