// shop_room_moves_browser.test.mjs — moving the shelves and props by hand
// in the counter room (static/js/widgets/shop_room_counter.js,
// shop_room_builder.js), in a real Chromium.
//
// While placing by hand the owner drags the shelves or a placed prop, or
// focuses it and nudges it with the arrow keys; the goods on the shelves go
// with them. The builder sizes the picked piece and puts it back, and Save
// room sends the moves in the theme. A saved move draws the room that way
// for everyone.
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
const SCRIPTS = ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_counter.js', 'shop_room_parts_strange.js', 'shop_room_parts_trades.js', 'shop_room_parts_places.js', 'shop_room_builder.js', 'shop_room.js'].map(read);
// A 1x1 JPEG stands in for every picture the room asks for.
const JPEG = Buffer.from('/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAAAAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AKp//2Q==', 'base64');
const A = '0b9c2f4e-1111-4222-8333-444455556666', B = '1b9c2f4e-1111-4222-8333-444455556666', NEW = '4b9c2f4e-1111-4222-8333-444455556666';
const LIST = { items: [A, B].map((id, i) => ({ id, original_name: 'pic' + i + '.jpg', mime_type: 'image/jpeg', url: '/media/' + id, thumbnail_url: '/media/' + id + '/thumb/300' }))
  .concat([{ id: '3b9c2f4e-1111-4222-8333-444455556666', original_name: 'song.mp3', mime_type: 'audio/mpeg', url: '/media/3b9c2f4e-1111-4222-8333-444455556666' }]), total: 3, page: 1, per_page: 24 };
const GOODS = [{ id: 11, relationType: 'sells', targetEntityId: 'e11', targetEntityName: 'Healing Potion', targetEntityType: 'Goods', targetEntityIcon: 'fa-flask', metadata: { price: 2, currency: 'gp', quantity: 4 } }];
const smithy = (picture, opening) => ({ name: 'Mine', version: 1, skeleton: { shape: 'square', opening: opening || 'window-right', display: 'shelves', counter: 'straight' },
  theme: Object.assign({ wall: 'stone', wood: '#4a3220', cloth: '#5a3a22', accent: '#ff9a50', trim: '#9aa0a8', light: 'lamp', warmth: 0.7, outside: 'dusk' }, picture ? { picture } : {}), props: [] });
const room = (counter) => ({ layout: { version: 1, roomType: 'forge', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: [], counter } });
const HOST = '<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body><div id="w" data-widget="shop_room" data-can-arrange="true" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="The Shop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></body></html>';

async function launch(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  return browser;
}

// Opens the shop at the counter on a page served from a stand-in site, so
// "/media/<id>" is a real address the page asks for.
async function open(browser, counter, { before = '' } = {}) {
  const p = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  const errors = [], asked = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.route('http://shop.test/**', (r) => {
    const u = r.request().url();
    if (u.includes('/media/')) { asked.push(new URL(u).pathname); return r.fulfill({ body: JPEG, contentType: 'image/jpeg' }); }
    return r.fulfill({ body: HOST, contentType: 'text/html' });
  });
  await p.goto('http://shop.test/');
  await p.addScriptTag({ content: `window.__calls = []; window.Chronicle = {
    register: function (n, impl) { if (n === 'shop_room') impl.init(document.getElementById('w')); },
    escapeHtml: function (t) { return String(t).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
    apiFetch: function (url, o) {
      var m = (o && o.method) || 'GET', b = o && o.body;
      window.__calls.push({ m: m, url: url, body: b instanceof FormData ? Object.fromEntries([...b.entries()].map(function (e) { return [e[0], typeof e[1] === 'string' ? e[1] : 'file']; })) : b ? JSON.parse(JSON.stringify(b)) : null });
      var body = /\\/room$/.test(url) ? (m === 'PUT' ? { layout: b } : ${JSON.stringify(room(counter))}) : /\\/buyers$/.test(url) ? { canBuyNow: false, buyers: [] }
        : /\\/media\\/list/.test(url) ? ${JSON.stringify(LIST)} : url === '/media/upload' ? { id: '${NEW}', url: '/media/${NEW}', thumbnail_url: '/media/${NEW}/thumb/300', mime_type: 'image/jpeg' } : /\\/rel$/.test(url) ? ${JSON.stringify(GOODS)} : {};
      return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(JSON.parse(JSON.stringify(body))); } });
    }
  };` });
  for (const s of SCRIPTS) await p.addScriptTag({ content: s });
  if (before) await p.addScriptTag({ content: before });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForSelector('.src-svg [data-k="keeper"]');
  return { p, errors, asked };
}


const withMoves = (moves) => { const c = smithy(null); c.props = ['anvil', 'banner']; if (moves) c.moves = moves; return c; };
const tf = (p, k) => p.evaluate((k) => { const e = document.querySelector('.src-svg [data-mvk="' + k + '"]'); return e && e.getAttribute('transform'); }, k);
const ware = (p) => p.evaluate(() => { const e = document.querySelector('#srcb-wares [data-k^="ware:"]'); const b = e && e.getBoundingClientRect(); return b ? [Math.round(b.left), Math.round(b.top)] : null; });

test('a saved move draws the shelves and a prop where the owner put them, goods and all', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p: plainP } = await open(browser, withMoves(null));
  assert.equal(await tf(plainP, 'display'), null, 'no move, no transform');
  const before = await ware(plainP);
  const { p, errors } = await open(browser, withMoves({ display: [120, -20, 1.2], anvil: [-40, 0, 1], banner: [0, 0, 1], ghost: [5, 5, 1] }));
  assert.match(await tf(p, 'display'), /scale\(1\.2\)/);
  assert.match(await tf(p, 'banner') || 'none', /none/, 'a move to nowhere is dropped');
  const after = await ware(p);
  assert.ok(after[0] > before[0] + 60, 'the goods moved right with the shelves: ' + before + ' -> ' + after);
  assert.equal(await p.evaluate(() => document.querySelectorAll('[data-mvon]').length), 0, 'only the owner placing by hand can move them');
  assert.deepEqual(errors, []);
});

test('the owner drags the shelves, nudges a prop, sizes and puts back, and Save room sends it', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, withMoves(null));
  await p.click('[data-mode="arr"]');
  await p.waitForSelector('.shr-cb [data-cb-mode="place"]', { state: 'attached' });
  await p.click('[data-cb-mode="place"]');
  await p.waitForSelector('.src-svg [data-mvk="display"][data-mvon]');
  const box = await p.evaluate(() => { const b = document.querySelector('.src-svg rect[data-k="shelves"]').getBoundingClientRect(); return { x: b.left + b.width / 2, y: b.top + b.height / 2, w: b.width }; });
  await p.mouse.move(box.x, box.y); await p.mouse.down(); await p.mouse.move(box.x + 60, box.y, { steps: 6 }); await p.mouse.move(box.x + 120, box.y + 10, { steps: 6 }); await p.mouse.up();
  await p.waitForFunction(() => /translate/.test(document.querySelector('.src-svg [data-mvk="display"]').getAttribute('transform') || ''));
  await p.waitForSelector('#cm-mvring', { state: 'attached' });
  await p.waitForSelector('[data-cb-size]');
  // The anvil, focused, steps right with the arrow keys, and the room does not turn.
  await p.focus('.src-svg [data-mvk="anvil"][data-mvon]');
  await p.keyboard.press('ArrowRight'); await p.waitForFunction(() => !!document.querySelector('.src-svg [data-mvk="anvil"][transform]'));
  await p.keyboard.press('Shift+ArrowUp');
  await p.waitForFunction(() => /translate\(\S+ \S+\)/.test(document.querySelector('.src-svg [data-mvk="anvil"]').getAttribute('transform')) && document.activeElement && document.activeElement.getAttribute('data-mvk') === 'anvil');
  assert.equal(await p.evaluate(() => !!document.querySelector('.src-svg rect[data-k="shelves"]')), true, 'still at the counter');
  await p.$eval('[data-cb-size]', (r) => { r.value = '1.5'; r.dispatchEvent(new Event('input', { bubbles: true })); });
  await p.waitForFunction(() => /scale\(1\.5\)/.test(document.querySelector('.src-svg [data-mvk="anvil"]').getAttribute('transform') || ''));
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.some((c) => c.m === 'PUT'));
  const moves = (await p.evaluate(() => window.__calls.find((c) => c.m === 'PUT'))).body.counter.moves;
  assert.deepEqual(Object.keys(moves), ['anvil', 'display']);
  assert.deepEqual(moves.anvil, [10, -40, 1.5]);
  assert.ok(Math.abs(moves.display[0] - Math.round(moves.display[0])) === 0 && moves.display[0] > 40, 'the shelves moved right in room units: ' + moves.display);
  assert.equal(moves.display[2], 1);
  // A prop is dragged by its drawing too, not only nudged.
  await p.click('[data-mode="arr"]');
  await p.waitForSelector('.shr-cb [data-cb-mode="place"]', { state: 'attached' });
  await p.click('[data-cb-mode="place"]');
  await p.waitForSelector('.src-svg [data-mvk="anvil"][data-mvon]');
  const before = await tf(p, 'anvil');
  const ab = await p.evaluate(() => { const b = document.querySelector('.src-svg [data-mvk="anvil"]').getBoundingClientRect(); return { x: b.left + b.width * .45, y: b.top + b.height * .3 }; });
  assert.equal(await p.evaluate((q) => document.elementsFromPoint(q.x, q.y).map((e) => e.closest('[data-mvk]')).filter(Boolean).map((e) => e.getAttribute('data-mvk'))[0], ab), 'anvil', 'the anvil lies under the press');
  await p.mouse.move(ab.x, ab.y); await p.mouse.down(); await p.mouse.move(ab.x + 40, ab.y - 20, { steps: 5 }); await p.mouse.up();
  await p.waitForFunction((was) => document.querySelector('.src-svg [data-mvk="anvil"]').getAttribute('transform') !== was, before);
  // A drag the browser cancels puts the piece back and saves nothing.
  await p.waitForSelector('.src-svg [data-mvk="banner"][data-mvon]');
  const was = await tf(p, 'banner');
  const bb = await p.evaluate(() => { const b = document.querySelector('.src-svg [data-mvk="banner"]').getBoundingClientRect(); return { x: b.left + b.width / 2, y: b.top + b.height / 2 }; });
  const calls = await p.evaluate(() => window.__calls.length);
  await p.mouse.move(bb.x, bb.y); await p.mouse.down(); await p.mouse.move(bb.x - 50, bb.y + 20, { steps: 5 });
  await p.evaluate(() => document.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true })));
  await p.mouse.up();
  assert.equal(await tf(p, 'banner'), was, 'back where it was');
  assert.equal(await p.evaluate((n) => window.__calls.slice(n).some((c) => c.m === 'PUT'), calls), false);
  // Put back: the anvil, picked, has a button that puts it where it was.
  await p.focus('.src-svg [data-mvk="anvil"][data-mvon]');
  await p.waitForSelector('[data-cb-unmove]:not([hidden])');
  await p.click('[data-cb-unmove]');
  await p.waitForFunction(() => !document.querySelector('.src-svg [data-mvk="anvil"]').getAttribute('transform'));
  assert.deepEqual(errors, []);
});
