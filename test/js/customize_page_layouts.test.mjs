// customize_page_layouts.test.mjs — the Page layout strip's helpers: a new
// layout's key is short, stable and never "main" or taken, the draft holds
// named layouts in the shape Save stores them, and the Look tab's drawer
// lists the Look editor's own rooms.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const src = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_page_editor.js'), 'utf8');
const sandbox = { Chronicle: { register() {} } };
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const L = sandbox.Chronicle._cpeLayouts;
const plain = (v) => JSON.parse(JSON.stringify(v));

test('a new layout gets a key the server accepts', () => {
  const key = /^[a-z][a-z0-9-]{0,31}$/;
  const cases = [
    ['Hero', {}, 'hero'],
    ['Hero', { hero: true }, 'hero-2'],
    ['Hero', { hero: true, 'hero-2': true }, 'hero-3'],
    ['Main', {}, 'main-2'],
    ['NPC sheet!', {}, 'npc-sheet'],
    ['3 columns', {}, 'layout-3-columns'],
    ['Ünïcode only ✨', {}, 'n-code-only'],
    ['', {}, 'layout'],
    ['x'.repeat(60), {}, 'x'.repeat(28)],
  ];
  for (const [name, taken, want] of cases) {
    const got = L.layoutKey(name, taken);
    assert.equal(got, want, name);
    assert.match(got, key, name);
  }
});

test('named layouts in the draft take the stored shape', () => {
  assert.deepEqual(plain(L.plCanon(null)), { mainName: '', claimed: '', extra: [] });
  const got = plain(L.plCanon({
    mainName: 'NPC', claimed: 'hero', junk: 1,
    extra: [{ key: 'hero', name: 'Hero', layout: { rows: [{ id: 'r', columns: [{ id: 'c', width: '12', blocks: [{ id: 'b', type: 'game_system_sheet', config: {} }] }] }] } }]
  }));
  assert.deepEqual(got, {
    mainName: 'NPC', claimed: 'hero',
    extra: [{ key: 'hero', name: 'Hero', layout: { rows: [{ id: 'r', columns: [{ id: 'c', width: 12, blocks: [{ id: 'b', type: 'game_system_sheet' }] }] }] } }]
  });
});

test("the Look tab's drawer lists the Look editor's rooms, in order", () => {
  const look = readFileSync(join(root, 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');
  const a = look.indexOf('var ROOMS = [');
  const rooms = vm.runInNewContext('(' + look.slice(look.indexOf('[', a), look.indexOf('];', a) + 1) + ')');
  const pages = readFileSync(join(root, 'internal', 'plugins', 'campaigns', 'customize_pages.go'), 'utf8');
  const block = pages.slice(pages.indexOf('var lookRooms = '), pages.indexOf('\n}\n', pages.indexOf('var lookRooms = ')));
  const goRooms = [...block.matchAll(/\{"([a-z]+)", "([^"]+)"\}/g)].map((m) => [m[1], m[2]]);
  assert.deepEqual(goRooms, plain(rooms.map((r) => [r[0], r[1]])));
  assert.equal(goRooms.length, 4);
});
