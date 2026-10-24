// customize_look_rooms.test.mjs — the Look editor's four rooms group the
// eleven sections, every section has exactly one room, and an address
// fragment (a section, a room, or the retired "access") opens the right one.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
const pages = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'customize_pages.go'), 'utf8');

function grab(name) {
  const a = src.indexOf('var ' + name + ' = [');
  assert.ok(a > 0, name + ' marker not found');
  const b = src.indexOf('];', a);
  return JSON.parse(vm.runInNewContext('JSON.stringify(' + src.slice(src.indexOf('[', a), b + 1) + ')'));
}
const rooms = grab('ROOMS');
const sections = [...src.slice(src.indexOf('var SECTIONS = ['), src.indexOf('var ROOMS = [')).matchAll(/\{ id:'([a-z]+)'/g)].map((m) => m[1]);

test('the rooms are the four signed ones, in order', () => {
  assert.deepEqual(rooms.map((r) => r[1]), ['Site frame', 'Colours and type', 'Pages and sheets', 'People']);
  assert.deepEqual(rooms[0][3], ['brand', 'header', 'sidebar', 'nav']);
  assert.deepEqual(rooms[1][3], ['colours', 'type', 'buttons', 'motion']);
  assert.deepEqual(rooms[2][3], ['page', 'hover', 'sheet']);
  assert.deepEqual(rooms[3][3], []);
});

test('every section belongs to exactly one room, and access is not a section', () => {
  const owned = rooms.flatMap((r) => r[3]).sort();
  assert.deepEqual(owned, [...sections].sort());
  assert.ok(!sections.includes('access'));
});

function hashTarget() {
  const a = src.indexOf('function hashTarget(');
  const b = src.indexOf('\n    }', a) + 6;
  const ctx = { ROOMS: rooms, ROOM_OF: {} };
  rooms.forEach((r) => r[3].forEach((s) => { ctx.ROOM_OF[s] = r[0]; }));
  return vm.runInNewContext('(' + src.slice(a, b).replace('function hashTarget', 'function') + ')', ctx);
}

test('a hash opens the section\'s room, a room hash opens the room, junk opens nothing', () => {
  const t = hashTarget();
  const cases = [
    ['#brand', 'frame', 'brand'], ['nav', 'frame', 'nav'], ['#type', 'style', 'type'], ['#motion', 'style', 'motion'],
    ['#page', 'pages', 'page'], ['#hover', 'pages', 'hover'], ['#sheet', 'pages', 'sheet'],
    ['#people', 'people', null], ['#access', 'people', null], ['#style', 'style', null], ['#frame', 'frame', null],
  ];
  for (const [h, room, sec] of cases) assert.deepEqual({ ...t(h) }, { room, sec }, h);
  assert.equal(t('#nope'), null);
  assert.equal(t(''), null);
});

test('every Find entry for the Look opens something in the editor', () => {
  const ids = [...pages.slice(pages.indexOf('var lookSections'), pages.indexOf('}\n\n', pages.indexOf('var lookSections'))).matchAll(/\{"([a-z]+)", "/g)].map((m) => m[1]);
  const t = hashTarget();
  // The Rulebook entry belongs to its own page, not to this editor.
  for (const id of ids.filter((i) => i !== 'book')) assert.ok(t('#' + id), id + ' must open a room');
});

test('rooms keep the tablist pattern and arrow keys', () => {
  assert.match(src, /role="tab" class="tab" id="tab-' \+ r\[0\]/);
  assert.match(src, /role="tabpanel" id="room-'/);
  assert.match(src, /ArrowRight'\) n = \(i \+ 1\) % ids\.length/);
  assert.match(src, /selectRoom\(ids\[n\], true\)/);
});
