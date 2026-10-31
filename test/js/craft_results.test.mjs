// craft_results.test.mjs — the pictures of made things:
// static/js/widgets/craft_results.js and its lifelike style,
// craft_results_real.js.
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
  vm.runInContext(readFileSync(join(widgets, 'craft_results_real.js'), 'utf8'), ctx);
  return window.CraftResults;
}
const R = load();
// Values from the vm's realm compare by shape once copied into this one.
const plain = (v) => JSON.parse(JSON.stringify(v));

test('every shape draws, and unknown shapes fall back to the bundle', () => {
  assert.deepEqual(plain(R.shapes()).sort(), ['blade', 'bundle', 'potion', 'ring', 'shield', 'vial']);
  for (const style of ['real', 'drawn']) for (const shape of R.shapes()) {
    const svg = R.svg({ shape, body: '#336699', trim: '#aa8844', detail: '#eeeeee', mark: { picture: 'leaf', tint: '#62903a' } }, 48, { style });
    assert.match(svg, /^<svg class="crx" viewBox="0 0 120 120" width="48" height="48" aria-hidden="true">/);
    assert.ok(svg.endsWith('</svg>'));
    assert.ok(svg.length > 2000, style + ' ' + shape + ' draws something');
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
  for (const style of ['real', 'drawn']) {
    for (const shape of R.shapes()) {
      const svg = R.svg({ ...bad, shape }, 40, { style });
      for (const s of ['onload', 'onmouseover', 'javascript', '<script', 'alert']) assert.ok(!svg.includes(s), style + ' ' + shape + ' ' + s);
    }
  }
  assert.equal(R.clean(null).shape, 'bundle');
  assert.equal(R.clean('potion').shape, 'bundle');
  // A mark whose picture the library doesn't have is dropped on its own.
  assert.equal(R.clean({ shape: 'ring', mark: { picture: 'dragon', tint: '#ffffff' } }).mark, null);
  assert.equal(R.svg({ shape: 'ring' }, 40, { label: 'A <ring> "x"' }).includes('aria-label="A ring x"'), true);
});

test('two pictures on one page never share an id', () => {
  const ids = (s) => [...s.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]);
  for (const style of ['real', 'drawn']) {
    const a = R.svg({ shape: 'potion' }, 40, { style }), b = R.svg({ shape: 'potion' }, 40, { style });
    assert.ok(ids(a).length > 3);
    for (const id of ids(a)) assert.ok(!ids(b).includes(id), id);
    // Every url(#…) a drawing uses is one it defines, and no id is written twice.
    for (const svg of R.shapes().map((shape) => R.svg({ shape, mark: { picture: 'leaf', tint: '#62903a' } }, 40, { style }))) {
      const have = ids(svg);
      assert.equal(new Set(have).size, have.length, style + ' duplicate id');
      for (const m of svg.matchAll(/url\(#([^)]+)\)/g)) assert.ok(have.includes(m[1]), style + ' ' + m[1]);
    }
  }
});

test('the lifelike style is the default, and the illustrated one stays', () => {
  assert.deepEqual(plain(R.styles()).sort(), ['drawn', 'real']);
  const drawn = R.svg({ shape: 'ring' }, 40, { style: 'drawn' }), real = R.svg({ shape: 'ring' }, 40);
  assert.ok(real.includes('feTurbulence') || real.includes('feSpecularLighting'), 'real is the default');
  assert.ok(!drawn.includes('feSpecularLighting'));
  // An unknown style name draws the default; a style missing a shape is refused.
  assert.ok(R.svg({ shape: 'ring' }, 40, { style: 'nope' }).includes('feSpecularLighting'));
  R.addStyle('half', { potion: () => '' });
  assert.ok(!plain(R.styles()).includes('half'));
});

test("the campaign's Customize choice picks the style", () => {
  for (const [attr, filtered] of [['illustrated', false], ['lifelike', true], [null, true], ['bogus', true]]) {
    // getElementById answers so CraftPictures sees its stylesheet as already in.
    const window = {}, document = { getElementById: () => ({}), documentElement: { getAttribute: (k) => (k === 'data-craft-art' ? attr : null) } };
    const ctx = vm.createContext({ window, document });
    for (const f of ['craft_pictures.js', 'craft_results.js', 'craft_results_real.js']) {
      vm.runInContext(readFileSync(join(widgets, f), 'utf8'), ctx);
    }
    const svg = window.CraftResults.svg({ shape: 'ring' }, 40);
    assert.equal(svg.includes('feSpecularLighting'), filtered, String(attr));
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
