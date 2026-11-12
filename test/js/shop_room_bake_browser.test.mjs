// shop_room_bake_browser.test.mjs — how the shop room's lit layers are drawn
// (bake in static/js/widgets/shop_room_counter.js), in a real Chromium.
//
// A layer is shown as soon as a drawing at the scene's own size is ready,
// then replaced by one at twice that. The sharp drawings are kept in the
// browser's cache storage, so a second visit shows them without drawing.
// Where there is no cache storage the room still opens.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, 'static/js/widgets', f), 'utf8');

const sells = (id, name, icon, price, quantity) => ({ id, relationType: 'sells', targetEntityId: 'e' + id, targetEntityName: name, targetEntityType: 'Goods', targetEntityIcon: 'fa-' + icon, metadata: { price, currency: 'gp', quantity } });
const GOODS = [sells(11, 'Healing Potion', 'flask', 2, 4), sells(12, 'Antitoxin', 'vial', 5, 3), sells(14, 'Bandages', 'bandage', 1, 6)];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', money: 500, moneyCp: 50000 }] };
const ROOM = { layout: { version: 1, roomType: 'apothecary', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: [] } };

const BOOT = `window.Chronicle = {
  register: function (n, impl) { impl.init(document.getElementById('w')); },
  apiFetch: function (url) {
    var body = /\\/room$/.test(url) ? ${JSON.stringify(ROOM)} : /\\/buyers$/.test(url) ? ${JSON.stringify(BUYERS)} : ${JSON.stringify(GOODS)};
    return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
  }
};`;
const tag = (s) => '<script>' + s.replace(/<\/script/g, '<\\/script') + '</script>';
const PAGE = `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body>
<div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="Hessa Ord's Workshop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div>
${tag(BOOT)}${['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_counter.js', 'shop_room.js'].map((f) => tag(read(f))).join('')}</body></html>`;

// A secure origin, so the page has cache storage as it would behind HTTPS.
const ORIGIN = 'https://shop.test/';
async function browser(t) {
  const b = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => b.close());
  const ctx = await b.newContext({ viewport: { width: 1200, height: 800 } });
  await ctx.route(ORIGIN, (r) => r.fulfill({ contentType: 'text/html; charset=utf-8', body: PAGE }));
  return ctx;
}
async function enter(p) {
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForFunction(() => { const st = document.querySelector('.src-stage'); return st && st.querySelector('.src-svg') && !st.classList.contains('src-baking') && document.querySelectorAll('.src-svg [data-bake] > image').length > 0; });
}
// Each baked layer on screen: how many times the scene's size its picture is.
const sharpness = (p) => p.evaluate(() => Promise.all(Array.from(document.querySelectorAll('.src-svg [data-bake] > image'), (im) => new Promise((ok) => {
  const i = new Image(); i.onload = () => ok(Math.round(i.naturalWidth / +im.getAttribute('width'))); i.onerror = () => ok(0); i.src = im.getAttribute('href');
}))));
async function allSharp(p) {
  for (let i = 0; i < 120; i++) { const s = await sharpness(p); if (s.length && s.every((k) => k === 2)) return; await p.waitForTimeout(500); }
  assert.fail('the layers never became sharp');
}
// Waits until the store has stopped taking in drawings: the views out of sight are drawn and kept after the one on screen.
async function storeSettled(p, atLeast) {
  let last = -1, same = 0;
  for (let i = 0; i < 120 && same < 4; i++) { const n = await stored(p); same = n === last && n >= atLeast ? same + 1 : 0; last = n; await p.waitForTimeout(500); }
  return last;
}
const stored = (p) => p.evaluate(() => caches.open('chronicle-shop-room-v1').then((c) => c.keys()).then((k) => k.length));

test('a room opens on quick drawings, which are then replaced by sharp ones and kept', { skip }, async (t) => {
  const ctx = await browser(t), p = await ctx.newPage(), errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.goto(ORIGIN);
  await enter(p);
  const layers = (await sharpness(p)).length;
  assert.ok(layers >= 3, 'the wall, the shelves and the counter are baked');
  await allSharp(p);
  assert.deepEqual(await sharpness(p), Array(layers).fill(2), 'every layer on screen ends at twice the scene size');
  assert.ok(await storeSettled(p, layers) >= layers, 'the sharp drawings are kept');
  assert.equal(await p.evaluate(() => !!document.querySelector('.src-svg [data-k="keeper"]')), true, 'the room is still the room');
  assert.deepEqual(errors, []);
});

test('a second visit shows the kept drawings and draws nothing', { skip }, async (t) => {
  const ctx = await browser(t), p = await ctx.newPage();
  await p.goto(ORIGIN);
  await enter(p);
  await allSharp(p);
  const n = await p.evaluate(() => document.querySelectorAll('.src-svg [data-bake] > image').length);
  const before = await storeSettled(p, n);

  const again = await ctx.newPage();
  // Counts the layers this visit has to draw: each one is an SVG handed to an image.
  await again.addInitScript(() => { window.__drawn = 0; const d = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src'); Object.defineProperty(HTMLImageElement.prototype, 'src', { set(v) { if (String(v).indexOf('data:image/svg+xml') === 0) window.__drawn++; d.set.call(this, v); }, get() { return d.get.call(this); } }); });
  await again.goto(ORIGIN);
  await enter(again);
  assert.deepEqual(await sharpness(again), Array(n).fill(2), 'sharp from the first moment');
  assert.equal(await again.evaluate(() => window.__drawn), 0, 'nothing on screen was drawn again');
  assert.ok(await stored(again) >= before, 'the kept drawings are still kept');
});

test('the store keeps only the newest layers', { skip }, async (t) => {
  const ctx = await browser(t), p = await ctx.newPage();
  await p.goto(ORIGIN);
  await p.evaluate(async () => { const c = await caches.open('chronicle-shop-room-v1'); for (let i = 0; i < 30; i++) await c.put('/shop-room-bake/old' + i, new Response(new Blob(['x']))); });
  await enter(p);
  await allSharp(p);
  await p.waitForFunction(() => caches.open('chronicle-shop-room-v1').then((c) => c.keys()).then((k) => k.length === 24 && !k.some((r) => /old0$/.test(r.url))), null, { timeout: 30000, polling: 500 });
});

test('with no cache storage the room still opens and sharpens', { skip }, async (t) => {
  const ctx = await browser(t), p = await ctx.newPage(), errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.addInitScript(() => { Object.defineProperty(window, 'caches', { value: undefined }); });
  await p.goto(ORIGIN);
  await enter(p);
  await allSharp(p);
  assert.deepEqual(errors, []);
});
