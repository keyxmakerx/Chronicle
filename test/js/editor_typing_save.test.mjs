// editor_typing_save.test.mjs — pins that the page editor saves shortly after
// typing stops, not only on its 30-second timer, and that words typed while a
// save is on its way are saved by the next one instead of being marked clean.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor.js'), 'utf8');

function loadScheduler() {
  const start = src.indexOf('var TYPING_SAVE_MS');
  const end = src.indexOf('\n  }\n', src.indexOf('function scheduleTypingSave')) + 4;
  assert.ok(start > 0 && end > start, 'scheduleTypingSave not found');
  const timers = [];
  const saves = [];
  const make = new Function('setTimeout', 'clearTimeout', 'saveContent',
    src.slice(start, end) + '\nreturn { scheduleTypingSave: scheduleTypingSave, ms: TYPING_SAVE_MS };');
  const api = make(
    (fn, ms) => { timers.push({ fn, ms, live: true }); return timers.length; },
    (id) => { if (id && timers[id - 1]) timers[id - 1].live = false; },
    (st) => saves.push(st));
  return { ...api, timers, saves };
}

test('typing schedules one save a short pause after the last key', () => {
  const s = loadScheduler();
  const state = { autosaveInterval: 30, isEditing: true, dirty: true, saving: false, typingTimer: null };
  s.scheduleTypingSave(state);
  s.scheduleTypingSave(state);
  s.scheduleTypingSave(state);
  const live = s.timers.filter((t) => t.live);
  assert.equal(live.length, 1, 'each key restarts the pause');
  assert.ok(s.ms <= 3000, 'saves within a few seconds of typing stopping');
  live[0].fn();
  assert.equal(s.saves.length, 1);
});

test('no typing save while autosave is switched off or a save is running', () => {
  const s = loadScheduler();
  s.scheduleTypingSave({ autosaveInterval: 0, isEditing: true, dirty: true, saving: false });
  assert.equal(s.timers.length, 0);
  const busy = { autosaveInterval: 30, isEditing: true, dirty: true, saving: true, typingTimer: null };
  s.scheduleTypingSave(busy);
  s.timers[0].fn();
  assert.equal(s.saves.length, 0);
});

test('a save only marks the page clean when nothing was typed during it', () => {
  assert.match(src, /var savedSeq = state\.editSeq;/);
  assert.match(src, /if \(state\.editSeq !== savedSeq\) \{\s*scheduleTypingSave\(state\);\s*return;/);
  assert.match(src, /state\.editSeq\+\+;[\s\S]{0,200}scheduleTypingSave\(state\);/);
});
