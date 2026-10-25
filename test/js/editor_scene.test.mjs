// editor_scene.test.mjs — pins how a Foundry scene mention reads and saves.
// A picked scene keeps its id; a typed one goes by name and lists every
// folder when several scenes share it; viewers who can't see the list (403)
// get no status at all. The saved shape must match what the Go sanitizer
// keeps (internal/sanitize/scene_mention_test.go).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor_scene.js'), 'utf8');

const LIST = [
  { id: 'aaaaaaaaaaaaaaa1', name: 'Sunken Crypt', folder: 'Act 2 / Caves' },
  { id: 'aaaaaaaaaaaaaaa2', name: "Dragon's Lair", folder: '' },
  { id: 'aaaaaaaaaaaaaaa3', name: 'Sunken Crypt', folder: 'Act 3' },
  { id: 'aaaaaaaaaaaaaaa4', name: 'Crypt Entrance', folder: 'Act 2' },
];

function load(fetchImpl) {
  const window = {
    TipTap: { Node: { create: (spec) => ({ spec }) } },
    Chronicle: { apiFetch: fetchImpl },
  };
  vm.runInNewContext(src, { window, Chronicle: window.Chronicle, TipTap: window.TipTap });
  return window.Chronicle;
}

const flush = () => new Promise((r) => setTimeout(r, 0));
const plain = (v) => JSON.parse(JSON.stringify(v));

test('a picked scene is found by id, even after a rename', () => {
  const C = load();
  const st = C.FoundryScenes._sceneStatus(LIST, 'aaaaaaaaaaaaaaa2', 'Old Lair');
  assert.equal(st.present, true);
  assert.equal(st.scene.name, "Dragon's Lair");
  assert.equal(C.FoundryScenes._statusTitle(st, 'Old Lair'), "In Foundry, at the top level, now called Dragon's Lair");
});

test('a typed name matches every scene that shares it and lists their folders', () => {
  const C = load();
  const st = C.FoundryScenes._sceneStatus(LIST, null, '  sunken   crypt ');
  assert.equal(st.present, true);
  assert.equal(st.same.length, 2);
  assert.equal(C.FoundryScenes._statusTitle(st, 'Sunken Crypt'), 'In Foundry 2 times: in Act 2 / Caves; in Act 3');
});

test('a name with no scene reads as needed; no list means no status', () => {
  const C = load();
  const st = C.FoundryScenes._sceneStatus(LIST, null, 'Frost Giant Hall');
  assert.equal(st.present, false);
  assert.equal(C.FoundryScenes._statusTitle(st, 'Frost Giant Hall'), 'No scene with this name in Foundry yet');
  assert.equal(C.FoundryScenes._sceneStatus(null, 'aaaaaaaaaaaaaaa1', 'Sunken Crypt'), null);
  assert.equal(C.FoundryScenes._statusTitle(null, 'x'), '');
});

test('picker rows keep Foundry order and offer "needed" only without an exact match', () => {
  const C = load();
  const rows = plain(C.FoundryScenes._pickerItems(LIST, 'crypt'));
  assert.deepEqual(rows.map((r) => r.sceneId), ['aaaaaaaaaaaaaaa1', 'aaaaaaaaaaaaaaa3', 'aaaaaaaaaaaaaaa4', null]);
  assert.deepEqual(rows[3], { kind: 'scene', sceneId: null, name: 'crypt', needed: true });
  const exact = plain(C.FoundryScenes._pickerItems(LIST, 'Sunken Crypt'));
  assert.ok(exact.every((r) => !r.needed), 'an exact name offers no "needed" row');
  assert.equal(C.FoundryScenes._pickerItems(LIST, 'crypt', 1).length, 2, 'max caps the scene rows, not the needed row');
  assert.deepEqual(plain(C.FoundryScenes._pickerItems(LIST, '   ')), []);
  assert.deepEqual(plain(C.FoundryScenes._pickerItems(null, 'crypt')), [], 'players get no scene rows');
});

test('saved HTML is a span with a checked scene id', () => {
  const C = load();
  assert.deepEqual(plain(C.FoundryScenes._renderSpec({ sceneId: 'aaaaaaaaaaaaaaa1', name: 'Sunken Crypt' })),
    ['span', { 'data-type': 'foundry-scene', class: 'scene-mention', 'data-scene-id': 'aaaaaaaaaaaaaaa1' }, 'Sunken Crypt']);
  for (const bad of [null, '', 'a"b', 'x'.repeat(65), '../x']) {
    assert.deepEqual(plain(C.FoundryScenes._renderSpec({ sceneId: bad, name: 'N' })),
      ['span', { 'data-type': 'foundry-scene', class: 'scene-mention' }, 'N'], String(bad));
  }
});

test('the list is fetched once per campaign; a 403 leaves it empty', async () => {
  const calls = [];
  const C = load((url) => {
    calls.push(url);
    if (url.includes('/c2/')) return Promise.resolve({ ok: false, status: 403 });
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ reported: true, scenes: LIST }) });
  });
  let notified = 0;
  C.FoundryScenes.subscribe(() => { notified++; });
  C.FoundryScenes.want('c1');
  C.FoundryScenes.want('c1');
  C.FoundryScenes.want('c2');
  assert.equal(C.FoundryScenes.get('c1'), null, 'nothing while loading');
  await flush(); await flush();
  assert.deepEqual(calls, ['/campaigns/c1/foundry/scenes', '/campaigns/c2/foundry/scenes']);
  assert.equal(C.FoundryScenes.get('c1').length, 4);
  assert.equal(C.FoundryScenes.get('c2'), null);
  assert.equal(C.FoundryScenes.status('c2', 'aaaaaaaaaaaaaaa1', 'Sunken Crypt'), null);
  assert.equal(notified, 2);
});

test('the node parses only checked ids from saved HTML', () => {
  const C = load();
  const attrs = C.SceneMention.spec.addAttributes();
  const el = (id, text) => ({ getAttribute: () => id, textContent: text });
  assert.equal(attrs.sceneId.parseHTML(el('aaaaaaaaaaaaaaa1')), 'aaaaaaaaaaaaaaa1');
  assert.equal(attrs.sceneId.parseHTML(el('bad id')), null);
  assert.equal(attrs.sceneId.parseHTML(el(null)), null);
  assert.equal(attrs.name.parseHTML(el(null, '  Sunken Crypt ')), 'Sunken Crypt');
});
