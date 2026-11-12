// design_library.test.mjs — the pure parts of static/js/widgets/design_library.js:
// reading a pasted share code, naming game systems, and wording the import
// preview. The server reads every code again; these only fail fast and say why.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const sandbox = { atob, TextDecoder };
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(root, 'static/js/widgets/design_library.js'), 'utf8'), sandbox);
const U = sandbox.DesignLibrary.util;
const plain = (o) => JSON.parse(JSON.stringify(o));

const PREFIX = 'chronicle-design:v1:';
const code = (o, pad) => PREFIX + Buffer.from(JSON.stringify(o)).toString('base64url') + (pad || '');
const good = { kind: 'room-theme', system_id: 'drawsteel', name: 'Cinderhold Forge', maker_name: 'Alice', data: { version: 1 } };

test('the prefix is the one the server writes', () => {
  assert.equal(U.PREFIX, PREFIX);
  const go = readFileSync(join(root, 'internal/plugins/designs/model.go'), 'utf8');
  assert.ok(go.includes('const CodePrefix = "' + PREFIX + '"'), 'model.go CodePrefix');
});

test('a good code is read, however it was pasted', () => {
  const want = { ok: true, body: { kind: 'room-theme', name: 'Cinderhold Forge', makerName: 'Alice', systemId: 'drawsteel' } };
  const c = code(good);
  for (const [name, text] of [['plain', c], ['wrapped', c.slice(0, 30) + '\n  ' + c.slice(30, 60) + '\r\n' + c.slice(60)], ['padded', code(good, '==')], ['spaces around', '  ' + c + '\n']]) {
    const r = plain(U.parseCode(text));
    assert.equal(r.ok, true, name);
    assert.deepEqual(r.body, want.body, name);
  }
  const uni = plain(U.parseCode(code(Object.assign({}, good, { name: 'Séance ✦ 夜', maker_name: 'Zoë' }))));
  assert.equal(uni.body.name, 'Séance ✦ 夜');
  assert.equal(uni.body.makerName, 'Zoë');
});

test('a bad code says what is wrong, in words', () => {
  const tests = [
    ['empty', '', /Paste a design code first/],
    ['spaces only', '   \n', /Paste a design code first/],
    ['no prefix', 'hello', /not a Chronicle design code/],
    ['another version', 'chronicle-design:v2:abc', /not a Chronicle design code/],
    ['not base64', PREFIX + '@@@@', /damaged/],
    ['not JSON', PREFIX + Buffer.from('hello').toString('base64url'), /damaged/],
    ['JSON but not an object', code([1, 2]), /damaged/],
    ['no kind', code({ name: 'x' }), /damaged/],
    ['no name', code({ kind: 'room-theme', name: '  ' }), /damaged/],
    ['not UTF-8', PREFIX + Buffer.from([0xff, 0xfe, 0xfd]).toString('base64url'), /damaged/],
    ['too large', PREFIX + 'A'.repeat(23000), /too large/]
  ];
  for (const [name, text, msg] of tests) {
    const r = U.parseCode(text);
    assert.equal(r.ok, false, name);
    assert.match(r.error, msg, name);
  }
});

test('game systems and kinds are named for people', () => {
  assert.equal(U.systemName('dnd5e'), 'D&D 5e');
  assert.equal(U.systemName('drawsteel'), 'Draw Steel');
  assert.equal(U.systemName(''), 'no game system');
  assert.equal(U.systemName('custom:https://example.test/x'), 'a custom game system');
  assert.equal(U.systemName('night-shift'), 'Night shift');
  assert.equal(U.kindLabel('room-theme'), 'Room theme');
  assert.equal(U.kindLabel('crafting-circle'), 'Magic circle');
  assert.equal(U.kindLabel('some-new-kind'), 'Some new kind');
});

test('the preview says the system differs, then what the campaign lacks', () => {
  assert.deepEqual(plain(U.previewNotes({ sameSystem: true, lacks: [] }, 'drawsteel')), []);
  const notes = plain(U.previewNotes({ sameSystem: false, systemId: 'dnd5e', lacks: ['This campaign has no silver.'] }, 'drawsteel'));
  assert.deepEqual(notes.map((n) => n.tone), ['warn', 'warn']);
  assert.equal(notes[0].text, 'Made for D&D 5e. This campaign uses Draw Steel. Parts it uses may be named differently here.');
  assert.equal(notes[1].text, 'This campaign has no silver.');
  assert.deepEqual(plain(U.previewNotes({ sameSystem: true, lacks: ['a', 'b'] }, 'x')).map((n) => n.text), ['a', 'b']);
});

test('links and owner search hits', () => {
  assert.equal(U.shareUrl('https://chronicle.test/', '/designs/link/abc'), 'https://chronicle.test/designs/link/abc');
  assert.equal(U.shareUrl('https://chronicle.test', ''), '');
  const hit = { systems: ['drawsteel', 'dnd5e'] };
  assert.deepEqual(plain(U.onSystem(hit, 'dnd5e')), ['dnd5e']);
  assert.deepEqual(plain(U.onSystem(hit, 'other')), []);
  assert.deepEqual(plain(U.onSystem(null, 'x')), []);
});
