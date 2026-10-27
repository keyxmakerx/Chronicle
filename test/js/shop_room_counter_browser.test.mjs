// shop_room_counter_browser.test.mjs — the counter look
// (static/js/widgets/shop_room_counter.js inside shop_room.js), in a real
// Chromium.
//
// Pressing a good brings it to the counter cloth and it goes on your pile
// only when you say so; Escape puts it back. Turning round shows the shop
// floor, where a good picked up is held until you add it. The case opens
// from the counter and Escape closes it. Buying sends what is on the pile,
// and the pile becomes a parcel.
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
const GOODS = [
  sells(11, 'Healing Potion', 'flask', 2, 4), sells(12, 'Antitoxin', 'vial', 5, 0), sells(13, 'Fever tea', 'leaf', 1, null), sells(14, 'Bandages', 'bandage', 1, 6),
  sells(15, 'Toolbox', 'box', 4, 2), sells(16, 'Ruby ring', 'ring', 50, 1), sells(17, 'Spellbook', 'book', 25, 1)
];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', money: 500, moneyCp: 50000 }] };
const ROOM = { layout: { version: 1, roomType: 'apothecary', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: ['Evening.'] } };

const HOST = `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body>
<div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="Hessa Ord's Workshop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></body></html>`;

async function shop(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent(HOST);
  await p.addScriptTag({ content: `window.__posts = []; window.Chronicle = {
    register: function (n, impl) { impl.init(document.getElementById('w')); },
    apiFetch: function (url, o) {
      if (o && o.method === 'POST') { window.__posts.push(o.body); return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve({ status: 'bought', spent: 3, currency: 'gp', moneyLeft: 497 }); } }); }
      var body = /\\/room$/.test(url) ? ${JSON.stringify(ROOM)} : /\\/buyers$/.test(url) ? ${JSON.stringify(BUYERS)} : ${JSON.stringify(GOODS)};
      return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
    }
  };` });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_counter.js', 'shop_room.js']) await p.addScriptTag({ content: read(f) });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForSelector('.src-svg [data-k="keeper"]');
  return p;
}
const count = (p) => p.evaluate(() => document.querySelector('.shr-cnt').textContent);
const pile = (p) => p.evaluate(() => Array.from(document.querySelectorAll('#srcb-pile [data-k]'), (e) => e.getAttribute('data-k')));
// Presses a piece by its data-k, the way a pointer would, wherever the camera has put it.
const press = (p, k) => p.evaluate((k) => document.querySelector('.src-svg [data-k="' + k + '"]').dispatchEvent(new MouseEvent('click', { bubbles: true })), k);
const settle = (p) => p.waitForTimeout(120);

test('the counter stands its goods on the shelves and shuts the finer ones in the case', { skip }, async (t) => {
  const p = await shop(t);
  const s = await p.evaluate(() => ({
    counter: document.querySelector('.shr').classList.contains('counter'), iso: getComputedStyle(document.querySelector('.shr-iso')).visibility,
    shelf: Array.from(document.querySelectorAll('#srcb-wares [data-k]'), (e) => e.getAttribute('data-k')), caseShut: !!document.querySelector('[data-k="case"]'),
    scene: (() => { const r = document.querySelector('.shr-scene').getBoundingClientRect(); return Math.round(r.width / r.height * 100) / 100; })()
  }));
  assert.equal(s.counter, true);
  assert.equal(s.iso, 'hidden', 'the lit room stays underneath, hidden');
  assert.deepEqual(s.shelf, ['ware:11', 'ware:12', 'ware:14'], 'the unlimited tea waits in a barrel on the floor');
  assert.equal(s.caseShut, true);
  assert.equal(s.scene, 1.71, 'the scene keeps the counter look\'s 12 by 7 shape');
});

test('a good comes to the cloth and goes on the pile only when you say so', { skip }, async (t) => {
  const p = await shop(t);
  await press(p, 'ware:11'); await settle(p);
  assert.ok(await p.$('[data-k="held:11"]'), 'the potion is on the cloth');
  assert.equal(await count(p), '', 'nothing in the basket yet');
  await p.click('.src-hint [data-act="take"]'); await settle(p);
  assert.equal(await count(p), '1');
  assert.deepEqual(await pile(p), ['pile:11']);
  // Escape puts the next one back without buying it, and leaves the shop open.
  await press(p, 'ware:14'); await settle(p);
  await p.keyboard.press('Escape'); await settle(p);
  assert.equal(await p.$('[data-k^="held:"]'), null);
  assert.equal(await count(p), '1');
  assert.equal(await p.evaluate(() => document.querySelector('.shr-dlg').open), true);
  // A sold-out good cannot be fetched.
  await press(p, 'ware:12'); await settle(p);
  assert.equal(await p.$('[data-k^="held:"]'), null);
});

test('turning round shows the shop floor, where a good is held until you add it', { skip }, async (t) => {
  const p = await shop(t);
  await p.keyboard.press('ArrowLeft'); await p.waitForFunction(() => /shop floor/.test(document.querySelector('.src-svg').getAttribute('aria-label')));
  const floor = await p.evaluate(() => Array.from(document.querySelectorAll('#srcf-wares [data-k], #srcf-barrels [data-k^="ware:"]'), (e) => e.getAttribute('data-k')));
  assert.deepEqual(floor.sort(), ['ware:13', 'ware:15'], 'the tea in a barrel, the toolbox in the cabinet');
  await press(p, 'ware:15'); await settle(p);
  assert.ok(await p.$('[data-k="held:15"]'), 'picked up');
  await p.keyboard.press('Escape'); await p.waitForTimeout(300);
  assert.equal(await p.$('[data-k^="held:"]'), null, 'Escape puts it back');
  assert.equal(await count(p), '');
  await press(p, 'ware:15'); await settle(p);
  await p.click('.src-hint [data-act="take"]'); await settle(p);
  assert.equal(await count(p), '1');
  await p.click('.src-turn-right'); await p.waitForFunction(() => /seen from the counter/.test(document.querySelector('.src-svg').getAttribute('aria-label')));
  assert.deepEqual(await pile(p), ['pile:15'], 'the pile on the counter holds what you chose on the floor');
});

test('the case opens from the counter and Escape closes it', { skip }, async (t) => {
  const p = await shop(t);
  await press(p, 'case'); await p.waitForFunction(() => /case/.test(document.querySelector('.src-svg').getAttribute('aria-label')));
  const inCase = await p.evaluate(() => Array.from(document.querySelectorAll('#srcc-wares [data-k]'), (e) => e.getAttribute('data-k')).sort());
  assert.deepEqual(inCase, ['ware:16', 'ware:17']);
  await press(p, 'ware:16'); await settle(p);
  await p.click('.src-hint [data-act="take"]'); await settle(p);
  assert.equal(await count(p), '1');
  await p.keyboard.press('Escape'); await p.waitForFunction(() => /seen from the counter/.test(document.querySelector('.src-svg').getAttribute('aria-label')));
});

test('buying sends what is on the pile, and the pile becomes a parcel', { skip }, async (t) => {
  const p = await shop(t);
  await press(p, 'ware:11'); await settle(p); await p.click('.src-hint [data-act="take"]'); await settle(p);
  await press(p, 'ware:11'); await settle(p); await p.click('.src-hint [data-act="take"]'); await settle(p);
  await p.click('.shr-cart'); await p.click('[data-buy]');
  await p.waitForFunction(() => window.__posts.length === 1);
  assert.deepEqual(await p.evaluate(() => window.__posts[0]), { buyerEntityId: 'c1', items: [{ relationId: 11, quantity: 2 }] });
  await p.waitForSelector('.srcb-parcel');
  assert.equal(await count(p), '');
});
