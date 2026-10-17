// page_editor.test.mjs — pins page_editor.js's rules: a value the server
// would refuse waits instead of being sent, each save sends only its own
// key, and E never takes a letter that is being typed.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'page_editor.js'), 'utf8');

// Without boot.js the script only exposes its helpers.
const sandbox = {};
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const P = sandbox.Chronicle._pageEditor;
const plain = (v) => JSON.parse(JSON.stringify(v));

test('the server counts bytes, and so do the checks', () => {
  const cases = [['', 0], ['ab', 2], ['é', 2], ['龍', 3], ['🐉', 4], ['a🐉b', 6]];
  for (const [s, want] of cases) assert.equal(P.utf8Len(s), want, s);
});

test('other names split on commas the way the server stores them', () => {
  const cases = [
    ['', []],
    ['the Grey Gull', ['the Grey Gull']],
    [' Gull ,  Grey   Gull,, ', ['Gull', 'Grey Gull']],
    ['Gull, gull, GULL', ['Gull']],
  ];
  for (const [text, want] of cases) assert.deepEqual(plain(P.splitAliases(text)), want, text);
});

test('other names the server would refuse wait', () => {
  const eleven = Array.from({ length: 11 }, (_, i) => 'Name ' + i);
  const cases = [
    ['none', [], ''],
    ['two letters', ['Al'], ''],
    ['one letter, as it is being typed', ['Gull', 'T'], 'Each other name needs at least 2 letters.'],
    ['one wide letter is enough bytes', ['龍'], ''],
    ['too long', ['x'.repeat(201)], 'An other name can be at most 200 characters.'],
    ['ten', eleven.slice(0, 10), ''],
    ['eleven', eleven, 'A page can have up to 10 other names.'],
  ];
  for (const [name, list, want] of cases) assert.equal(P.aliasProblem(list), want, name);
});

test('a name is one line, and a page needs one', () => {
  assert.equal(P.cleanName('  Mara\n  Vell  '), 'Mara Vell');
  assert.equal(P.nameProblem(''), 'A page needs a name.');
  assert.equal(P.nameProblem('Mara'), '');
  assert.equal(P.nameProblem('é'.repeat(101)), 'A name can be at most 200 characters.');
});

test('each save sends only its own key, and empty clears', () => {
  const cases = [
    ['name', 'Mara', { name: 'Mara' }],
    ['label', 'Captain', { type_label: 'Captain' }],
    ['label', '', { type_label: null }],
    ['parent', 'p1', { parent_id: 'p1' }],
    ['parent', '', { parent_id: null }],
    ['aka', 'x', null],
  ];
  for (const [key, v, want] of cases) assert.deepEqual(plain(P.metadataBody(key, v)), want, key + '=' + v);
});

test('E outlines only when nothing is being typed; Esc always steps back', () => {
  const k = (key, mods = {}) => ({ key, ctrlKey: false, metaKey: false, altKey: false, ...mods });
  const cases = [
    ['e, not typing', k('e'), false, 'mark'],
    ['E, not typing', k('E'), false, 'mark'],
    ['e while typing', k('e'), true, ''],
    ['Ctrl+E', k('e', { ctrlKey: true }), false, ''],
    ['Cmd+E', k('e', { metaKey: true }), false, ''],
    ['Esc while typing', k('Escape'), true, 'back'],
    ['Esc', k('Escape'), false, 'back'],
    ['another letter', k('x'), false, ''],
  ];
  for (const [name, e, typing, want] of cases) assert.equal(P.keyAction(e, typing), want, name);
});

test('typing means a text field or anything editable', () => {
  const el = (o) => ({ isContentEditable: false, closest: () => null, ...o });
  assert.equal(P.typingIn(el({ isContentEditable: true })), true);
  assert.equal(P.typingIn(el({ closest: (s) => (s.includes('input') ? {} : null) })), true);
  assert.equal(P.typingIn(el({})), false);
  assert.equal(P.typingIn(null), false);
});
