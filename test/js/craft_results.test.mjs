// craft_results.test.mjs — the pictures of made things:
// static/js/widgets/craft_results.js.
//
// Pins that every look is cleaned before it is drawn (the pack inserts the
// markup as HTML, and a look is stored data), that each shape draws with its
// own ids so many can share a page, that materials colour a look in recipe
// order, and that the shapes and default colours match the server's
// (internal/plugins/crafting/result_look.go), which composes stored looks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const widgets = join(root, 'static', 'js', 'widgets');

function load() {
  const window = {};
  const ctx = vm.createContext({ window });
  vm.runInContext(readFileSync(join(widgets, 'craft_pictures.js'), 'utf8'), ctx);
  vm.runInContext(readFileSync(join(widgets, 'craft_results.js'), 'utf8'), ctx);
  return window.CraftResults;
}
const R = load();
// Values from the vm's realm compare by shape once copied into this one.
const plain = (v) => JSON.parse(JSON.stringify(v));

test('every shape draws, and unknown shapes fall back to the bundle', () => {
  assert.deepEqual(plain(R.shapes()).sort(), ['blade', 'bundle', 'potion', 'ring', 'shield', 'vial']);
  for (const shape of R.shapes()) {
    const svg = R.svg({ shape, body: '#336699', trim: '#aa8844', detail: '#eeeeee', mark: { picture: 'leaf', tint: '#62903a' } }, 48);
    assert.match(svg, /^<svg class="crx" viewBox="0 0 120 120" width="48" height="48" aria-hidden="true">/);
    assert.ok(svg.endsWith('</svg>'));
  }
  assert.equal(R.clean({ shape: 'castle' }).shape, 'bundle');
  assert.equal(R.clean({ shape: 'constructor' }).shape, 'bundle');
});

test('nothing from a look reaches the markup unchecked', () => {
  const bad = {
    shape: 'potion" onload="alert(1)', body: 'red" onmouseover="x', trim: 'url(javascript:x)', detail: '#12345',
    mark: { picture: '"><script>', tint: '#000000' }
  };
  const c = R.clean(bad);
  assert.deepEqual(plain(c), { shape: 'bundle', body: '#7a6a9a', trim: '#c9a54a', detail: '#efe6d2', mark: null });
  const svg = R.svg(bad, 40);
  for (const s of ['onload', 'onmouseover', 'javascript', '<script', 'alert']) assert.ok(!svg.includes(s), s);
  assert.equal(R.clean(null).shape, 'bundle');
  assert.equal(R.clean('potion').shape, 'bundle');
  // A mark whose picture the library doesn't have is dropped on its own.
  assert.equal(R.clean({ shape: 'ring', mark: { picture: 'dragon', tint: '#ffffff' } }).mark, null);
  assert.equal(R.svg({ shape: 'ring' }, 40, { label: 'A <ring> "x"' }).includes('aria-label="A ring x"'), true);
});

test('two pictures on one page never share an id', () => {
  const a = R.svg({ shape: 'potion' }, 40), b = R.svg({ shape: 'potion' }, 40);
  const ids = (s) => [...s.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]);
  assert.ok(ids(a).length > 3);
  for (const id of ids(a)) assert.ok(!ids(b).includes(id), id);
  // Every url(#…) a drawing uses is one it defines.
  for (const svg of R.shapes().map((shape) => R.svg({ shape, mark: { picture: 'leaf', tint: '#62903a' } }, 40))) {
    const have = new Set(ids(svg));
    for (const m of svg.matchAll(/url\(#([^)]+)\)/g)) assert.ok(have.has(m[1]), m[1]);
  }
});

test('materials colour a look in recipe order', () => {
  const l = R.fromMaterials('potion', [
    { picture: 'moss', tint: '#c3223a' }, { picture: 'leaf', tint: '#9fc7e8' }, { picture: 'salt', tint: '#e8f2f6' }, { picture: 'root', tint: '#000000' }
  ]);
  assert.deepEqual(plain(l), { shape: 'potion', body: '#c3223a', trim: '#9fc7e8', detail: '#e8f2f6', mark: { picture: 'leaf', tint: '#9fc7e8' } });
  const one = R.fromMaterials('blade', [{ picture: 'ore', tint: '#6a6e78' }, { picture: 'x', tint: 'nope' }]);
  assert.equal(one.body, '#6a6e78');
  assert.equal(one.trim, '#c9a24a');
});

test('the shapes and their colours match the server', () => {
  const go = readFileSync(join(root, 'internal', 'plugins', 'crafting', 'result_look.go'), 'utf8');
  const server = {};
  for (const m of go.matchAll(/"(\w+)":\s*\{"(#[0-9a-f]{6})", "(#[0-9a-f]{6})", "(#[0-9a-f]{6})"\}/g)) server[m[1]] = [m[2], m[3], m[4]];
  assert.deepEqual(Object.keys(server).sort(), plain(R.shapes()).sort());
  for (const [shape, d] of Object.entries(server)) {
    const c = R.clean({ shape });
    assert.deepEqual([c.body, c.trim, c.detail], d, shape);
  }
});
