// notes_autosave.test.mjs — pins the notes-widget (notes.js) autosave
// contract: a journal.js-style debounced autosave plus blur / navigation /
// unload flushes, not just a save-on-Done click. notes.js is a browser IIFE
// bound to TipTap + the live DOM, so these pin the wiring by static source
// contract rather than executing it; behavioral verification (typing → save
// fires) needs a real browser.
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

test('every page-leave listener is added and removed on destroy (no leak)', () => {
  assert.match(src, /addEventListener\(\s*['"]beforeunload['"]\s*,\s*flushOnHide\s*\)/,
    'must flush on page unload');
  assert.match(src, /addEventListener\(\s*['"]pagehide['"]\s*,\s*flushOnHide\s*\)/,
    'must flush on pagehide');
  assert.match(src, /document\.addEventListener\(\s*['"]visibilitychange['"]\s*,\s*el\._notesOnVisibility\s*\)/,
    'visibilitychange must use a named handler so it can be removed');
  assert.match(src, /removeEventListener\(\s*['"]beforeunload['"]\s*,\s*el\._notesBeforeUnload\s*\)/,
    'the beforeunload listener must be removed on destroy()');
  assert.match(src, /removeEventListener\(\s*['"]pagehide['"]\s*,\s*el\._notesBeforeUnload\s*\)/,
    'the pagehide listener must be removed on destroy()');
  assert.match(src, /document\.removeEventListener\(\s*['"]visibilitychange['"]\s*,\s*el\._notesOnVisibility\s*\)/,
    'the visibilitychange listener must be removed on destroy()');
});

test('the hide flush saves with keepalive and the autosave PUT carries it', () => {
  assert.match(src, /function flushOnHide\(\)\s*\{\s*flushNow\(true\)/);
  assert.match(src, /function flushAutosave\(\)\s*\{\s*flushNow\(false\)/);
  assert.match(src, /keepalive:\s*!!keepalive/, 'the PUT must pass keepalive to apiFetch');
});

// Behavioural: run the real functions cut out of notes.js against stand-ins,
// since the widget is an IIFE bound to the DOM.
function cut(name) {
  const i = src.indexOf('function ' + name);
  assert.ok(i >= 0, name + ' missing');
  let depth = 0, j = src.indexOf('{', i);
  for (let k = j; k < src.length; k++) {
    if (src[k] === '{') depth++;
    else if (src[k] === '}' && --depth === 0) return src.slice(i, k + 1);
  }
  throw new Error('unbalanced ' + name);
}

function harness(visibility) {
  const body = [cut('flushAutosave'), cut('flushOnHide'), cut('flushNow'), cut('onVisibilityChange')].join('\n');
  const saves = [];
  const factory = new Function('document', 'state', 'saveEditingNote', 'ctl',
    'var autosaveTimer = null; var notesDirty = ctl.dirty;\n' + body +
    '\nreturn { flushAutosave: flushAutosave, flushOnHide: flushOnHide, onVisibilityChange: onVisibilityChange };');
  const api = factory({ visibilityState: visibility }, { editingId: 'n1' },
    (id, keepalive) => saves.push({ id, keepalive }), { dirty: true });
  return { api, saves };
}

test('a hidden tab flushes the dirty edit with keepalive', () => {
  const { api, saves } = harness('hidden');
  api.onVisibilityChange();
  assert.deepEqual(saves, [{ id: 'n1', keepalive: true }]);
  api.onVisibilityChange();
  assert.equal(saves.length, 1, 'a clean note is not saved twice');
});

test('a visible tab does not flush; the debounce flush is not keepalive', () => {
  const visible = harness('visible');
  visible.api.onVisibilityChange();
  assert.equal(visible.saves.length, 0);
  visible.api.flushAutosave();
  assert.deepEqual(visible.saves, [{ id: 'n1', keepalive: false }]);
});

test('an Event passed to the pagehide handler is not read as the keepalive flag', () => {
  const { api, saves } = harness('visible');
  api.flushAutosave({ type: 'blur' });
  assert.equal(saves[0].keepalive, false);
});
