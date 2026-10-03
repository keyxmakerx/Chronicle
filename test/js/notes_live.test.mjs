// notes_live.test.mjs — pins the jot panel's live updates (notes.js): the
// socket lives only while the panel is open, and a live reload never draws
// over a jot being edited. notes.js is a browser IIFE, so these pin the
// wiring by source contract, as notes_autosave.test.mjs does.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// Strip comments so prose mentioning the call names can't satisfy the guard.
function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}

const src = strip(readFileSync(join(root, 'static/js/widgets/notes.js'), 'utf8'));

function body(name) {
  const i = src.indexOf('function ' + name + '(');
  assert.ok(i >= 0, name + ' must be defined');
  return src.slice(i, src.indexOf('\n    }\n', i));
}

test('the socket opens with the panel and closes with it', () => {
  assert.match(body('openPanel'), /live\.start\(\)/, 'openPanel must start live updates');
  assert.match(body('closePanel'), /live\.stop\(\)/, 'closePanel must stop live updates');
  const d = src.indexOf('destroy: function');
  assert.match(src.slice(d), /_notesLive\.stop\(\)/, 'destroy must stop live updates');
});

test('a live reload waits while someone is at work in the panel', () => {
  const i = src.indexOf('function busy()');
  assert.ok(i >= 0, 'busy() must exist');
  const b = src.slice(i, i + 300);
  for (const s of ['state.editingId', 'state.versionsNoteId', 'notesList.contains']) {
    assert.ok(b.includes(s), 'busy() must check ' + s);
  }
});

test('a live reload is quiet and never empties the list on failure', () => {
  const l = body('loadNotes');
  assert.match(l, /if \(!quiet\) \{\s*state\.loading = true;/, 'only a normal load shows the loading line');
  assert.match(l, /if \(quiet\) return;/, 'a failed live reload keeps the list');
  assert.match(l, /quiet && state\.editingId !==/, 'a live reload that lands mid-edit must not draw');
});

test('only this campaign’s note events trigger a reload', () => {
  assert.match(src, /msg\.campaignId !== campaignId[^\n]*msg\.type\.indexOf\('note\.'\) !== 0/);
});
