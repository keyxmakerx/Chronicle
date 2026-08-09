// notes_share_picker.test.mjs — pins the share-with-players picker contract
// between internal/widgets/notes/handler.go (memberRef) and the notes widget.
//
// The regression: GET /campaigns/:id/notes/members has only ever shipped
// {user_id, username, role}, but notes.js read m.id / m.name. escapeHtml
// (boot.js) turns undefined into '', so every checkbox rendered blank with
// value="", the `m.id !== currentUserId` filter excluded nobody (undefined is
// never equal to a user id), and ticking a row PUT sharedWith:[""] — a note
// shared with no one. Nothing was broken loudly enough to notice.
//
// notes.js is a browser IIFE bound to TipTap + the live DOM, so — following
// notes_autosave.test.mjs and widget_listener_leaks.test.mjs — this pins the
// binding by source contract rather than by executing it. What makes it more
// than a regex is that the expected key names are not written here: they are
// read out of the Go struct tags, so renaming either side goes red.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// Strip comments so prose naming the fields can't satisfy the guard.
function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
}

const src = strip(readFileSync(join(root, 'static/js/widgets/notes.js'), 'utf8'));
const handler = readFileSync(join(root, 'internal/widgets/notes/handler.go'), 'utf8');

// The JSON keys MembersAPI actually puts on the wire, read from the Go struct.
const memberRefKeys = (() => {
  const body = handler.match(/type memberRef struct \{([\s\S]*?)\n\}/);
  assert.ok(body, 'handler.go must declare `type memberRef struct`');
  return [...body[1].matchAll(/json:"([^",]+)/g)].map((m) => m[1]);
})();

// Source region of a named function, so assertions can't be satisfied by an
// unrelated part of a 1700-line file.
function region(name, len) {
  const i = src.indexOf('function ' + name);
  assert.ok(i >= 0, name + ' must be defined in notes.js');
  return src.slice(i, i + len);
}

test('memberRef is the {user_id, username, role} shape the picker binds to', () => {
  assert.deepEqual(memberRefKeys, ['user_id', 'username', 'role']);
});

test('fetchMembers filters the current user on the id key memberRef ships', () => {
  const [idKey] = memberRefKeys;
  const r = region('fetchMembers', 700);
  assert.match(r, new RegExp('m\\.' + idKey + '\\s*!==\\s*currentUserId'),
    'fetchMembers must exclude the viewer by m.' + idKey +
    ' — comparing a key the payload does not carry excludes nobody');
});

test('the picker renders the checkbox value + label from memberRef keys', () => {
  const [idKey, nameKey] = memberRefKeys;
  const r = region('loadShareMembers', 1400);
  assert.match(r, new RegExp('escapeAttr\\(m\\.' + idKey + '\\)'),
    'the checkbox value must be the member ' + idKey + ' — anything else PUTs empty ids');
  assert.match(r, new RegExp('escapeHtml\\(m\\.' + nameKey + '\\b'),
    'the visible label must come from m.' + nameKey);
  assert.match(r, new RegExp('currentShared\\.indexOf\\(m\\.' + idKey + '\\)'),
    'already-shared rows must be matched by ' + idKey);
});

test('no member field outside the memberRef contract is read', () => {
  const both = region('fetchMembers', 700) + region('loadShareMembers', 1400);
  for (const phantom of [...both.matchAll(/\bm\.([A-Za-z_$][\w$]*)/g)].map((m) => m[1])) {
    assert.ok(memberRefKeys.includes(phantom),
      'the share picker reads m.' + phantom + ', which MembersAPI does not send ' +
      '(memberRef ships ' + memberRefKeys.join('/') + ')');
  }
});
