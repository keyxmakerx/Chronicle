// notes_autosave.test.mjs — pins the notes-widget autosave contract
// (cordinator#6). The floating notes widget (notes.js) only persisted edits
// when the user clicked Done; this adds a journal.js-style debounced autosave
// plus blur / navigation / unload flushes. notes.js is a browser IIFE bound to
// TipTap + the live DOM, so — like widget_listener_leaks.test.mjs — these pin
// the wiring by static source contract rather than executing it. Behavioral
// verification (typing → save fires) needs a real browser.
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
const journal = strip(readFileSync(join(root, 'static/js/widgets/journal.js'), 'utf8'));

test('notes autosave mirrors journal.js’ 1.5s debounce delay', () => {
  const notesDelay = src.match(/AUTOSAVE_DELAY\s*=\s*(\d+)/);
  const journalDelay = journal.match(/AUTOSAVE_DELAY\s*=\s*(\d+)/);
  assert.ok(notesDelay, 'notes.js must define AUTOSAVE_DELAY');
  assert.ok(journalDelay, 'journal.js must define AUTOSAVE_DELAY (mirror source)');
  assert.equal(notesDelay[1], '1500');
  assert.equal(notesDelay[1], journalDelay[1], 'notes must mirror journal AUTOSAVE_DELAY');
});

test('the TipTap editor is wired for debounced autosave + blur flush', () => {
  assert.match(src, /onUpdate:\s*function\s*\([^)]*\)\s*\{[^}]*markNoteDirty\(\)/,
    'editor onUpdate must mark the note dirty (debounced autosave)');
  assert.match(src, /onBlur:\s*function\s*\([^)]*\)\s*\{[^}]*flushAutosave\(\)/,
    'editor onBlur must flush the autosave');
});

test('title + checklist inputs schedule autosave on input', () => {
  assert.match(src, /note-title-input[\s\S]{0,60}note-check-text-input/,
    'both the title and checklist text inputs must be covered');
  assert.match(src, /addEventListener\(\s*['"]input['"]\s*,\s*markNoteDirty\s*\)/,
    'input edits must schedule an autosave via markNoteDirty');
});

test('flushAutosave is gated on notesDirty (no redundant / double writes)', () => {
  const i = src.indexOf('function flushAutosave');
  assert.ok(i >= 0, 'flushAutosave must be defined');
  assert.match(src.slice(i, i + 400), /notesDirty/,
    'flushAutosave must only write when there are unsaved changes');
});

test('Done flushes instead of an unconditional save (no double-save)', () => {
  const i = src.indexOf('.note-done-btn'); // the querySelectorAll in bindCardEvents, not the HTML button
  assert.ok(i >= 0, 'the Done button handler must exist');
  const region = src.slice(i, i + 600);
  assert.match(region, /flushAutosave\(\)/, 'Done must flush the autosave');
  assert.doesNotMatch(region, /saveEditingNote\(/,
    'Done must not call saveEditingNote directly — that double-saves after the blur flush');
});

test('beforeunload autosave listener is added and removed (no leak)', () => {
  assert.match(src, /addEventListener\(\s*['"]beforeunload['"]\s*,\s*flushAutosave\s*\)/,
    'must flush on page unload');
  assert.match(src, /removeEventListener\(\s*['"]beforeunload['"]\s*,\s*el\._notesBeforeUnload\s*\)/,
    'the beforeunload listener must be removed on destroy()');
});
