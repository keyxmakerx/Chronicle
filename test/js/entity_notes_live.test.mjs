// entity_notes_live.test.mjs — pins the Player Notes widget's live and
// delete behaviour (entity_notes.js). The widget is a browser IIFE, so these
// pin the wiring by source contract, as notes_live.test.mjs does.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}

const src = strip(readFileSync(join(root, 'static/js/widgets/entity_notes.js'), 'utf8'));

function body(name) {
  const i = src.indexOf('function ' + name + '(');
  assert.ok(i >= 0, name + ' must be defined');
  return src.slice(i, src.indexOf('\n      }\n', i));
}

test('a live update changes rows, never the composer', () => {
  const r = body('refresh');
  assert.ok(!/compSlot/.test(r), 'refresh must not touch the composer slot');
  assert.ok(!/el\.innerHTML/.test(r), 'refresh must not redraw the whole widget');
  assert.match(r, /data-en-id/, 'refresh must match rows by note id');
});

test('a live nudge refetches instead of trusting the message', () => {
  const c = body('connect');
  assert.match(c, /entity_note\.created/);
  assert.match(c, /loadNotes\(\)/);
  assert.ok(!/msg\.(note|bodyHtml|body)\b/.test(c), 'the socket message must not be drawn');
  assert.match(c, /setTimeout\(connect/, 'a dropped socket must reconnect');
});

test('delete waits for Undo, then goes out even if the page closes', () => {
  assert.match(src, /UNDO_MS = 5000/);
  assert.match(body('del'), /setTimeout\(function \(\) \{ commitDelete\(id, false\); \}, UNDO_MS\)/);
  assert.match(body('commitDelete'), /keepalive: !!keepalive/);
  assert.match(src, /addEventListener\('pagehide', onPageHide\)/);
  const cleanup = src.slice(src.indexOf('el.__entityNotesCleanup = function'));
  assert.match(cleanup.slice(0, 200), /flushDeletes\(\)/, 'unmount must carry out a waiting delete');
});

test('editing warns when the note changed or went elsewhere', () => {
  const c = body('checkComposerAgainstServer');
  assert.match(c, /changed in another window/);
  assert.match(c, /deleted in another window/);
});
