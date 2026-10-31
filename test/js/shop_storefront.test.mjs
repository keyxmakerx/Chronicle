// shop_storefront.test.mjs — the storefront drawing (static/js/widgets/
// shop_storefront.js) must offer exactly the choices the server accepts
// (internal/plugins/armory/shop_front_model.go), or a pick would fail to save
// or a saved storefront would draw the wrong thing. It must also never let a
// stored value reach the SVG as markup, and the street must group shops by
// town in a stable order.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');

function load() {
  const sandbox = { Chronicle: { register() {}, escapeHtml: (s) => String(s).replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`) } };
  sandbox.window = sandbox;
  vm.createContext(sandbox);
  for (const f of ['shop_storefront.js', 'shop_street.js']) {
    vm.runInContext(readFileSync(join(root, 'static', 'js', 'widgets', f), 'utf8'), sandbox);
  }
  return sandbox;
}
const W = load();
const S = W.ChronicleStorefront;

function goSet(name) {
  const src = readFileSync(join(root, 'internal', 'plugins', 'armory', 'shop_front_model.go'), 'utf8');
  const m = src.match(new RegExp(name + '\\s*=\\s*setOf\\(([^)]*)\\)'));
  assert.ok(m, `no ${name} set in shop_front_model.go`);
  return [...m[1].matchAll(/"([^"]*)"/g)].map((x) => x[1]).sort();
}

test('the choices match the server lists', () => {
  const pairs = [['style', 'shopFrontStyles'], ['awning', 'shopFrontAwnings'], ['sign', 'shopFrontSigns'],
    ['icon', 'shopFrontIcons'], ['ware', 'shopFrontWares'], ['closed', 'shopFrontClosed']];
  for (const [js, go] of pairs) assert.deepEqual([...S.LISTS[js]].sort(), goSet(go), js);
});

test('every ware can be drawn', () => {
  assert.deepEqual([...S.WARES].sort(), [...S.LISTS.ware].sort());
});

test('a stored value never reaches the drawing as markup', () => {
  const svg = S.draw({
    name: '<img src=x onerror=alert(1)>', words: '"><script>x</script>', style: '<g>', colour: '"/><script>',
    trim: 'red;fill:url(x)', awning: 'stripe', awningColour: '#12345g', sign: 'banner', wares: ['gem', '<x>'],
    closed: 'board', closedNote: '<b>shut</b>', refuseNote: '<i>no</i>',
  });
  assert.doesNotMatch(svg, /<script|<img|<b>|<i>|<x>|url\(x\)|#12345g/);
  assert.match(svg, /^<svg[\s\S]*<\/svg>$/);
});

test('clean keeps listed values and drops the rest', () => {
  const c = S.clean({ style: 'stone', colour: '#ABCDEF', sign: 'neon', wares: ['gem', 'dragon', 'herb', 'bread', 'sword'], closed: 'board' });
  assert.equal(c.style, 'stone');
  assert.equal(c.colour, '#abcdef');
  assert.equal(c.sign, 'hang');
  assert.deepEqual([...c.wares], ['gem', 'herb', 'bread']);
});

test('shops are grouped by town, with townless shops last', () => {
  const g = W.ChronicleShopStreet.groupByTown([
    { id: '1', name: 'A' }, { id: '2', name: 'B', townId: 't', townName: 'Thornbury' },
    { id: '3', name: 'C', townId: 's', townName: 'Saltmere' }, { id: '4', name: 'D', townId: 't', townName: 'Thornbury' },
  ]);
  assert.deepEqual(Array.from(g, (t) => t.name), ['Thornbury', 'Saltmere', 'Other shops']);
  assert.deepEqual(Array.from(g[0].shops, (s) => s.id), ['2', '4']);
});
