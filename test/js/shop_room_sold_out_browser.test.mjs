// shop_room_sold_out_browser.test.mjs — a buy someone else beat you to
// (static/js/widgets/shop_room.js), in a real Chromium.
//
// When the server refuses a basket because another shopper took the stock,
// the room reads the wares again, drops what sold out from the basket (or
// cuts it to what is left), names it in plain words instead of the server's
// API text, and the keeper says so. Any other refusal still shows as sent.
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

const sells = (id, name, price, quantity) => ({ id, relationType: 'sells', targetEntityId: 'e' + id, targetEntityName: name, targetEntityType: 'Goods', targetEntityIcon: 'fa-flask', metadata: { price, currency: 'gp', quantity } });
const BEFORE = [sells(11, 'Healing Potion', 2, 4), sells(13, 'Fever tea', 1, null), sells(15, 'Toolbox', 4, 2)];
// What the shelves hold once the other shopper has bought.
const AFTER = [sells(11, 'Healing Potion', 2, 0), sells(13, 'Fever tea', 1, null), sells(15, 'Toolbox', 4, 1)];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', money: 500, moneyCp: 50000 }] };
const ROOM = { layout: { version: 1, roomType: 'apothecary', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: [] } };

const HOST = `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body>
<div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="Hessa Ord's Workshop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></body></html>`;

async function shop(t, refusal, after = AFTER) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent(HOST);
  // The fixtures go in as data, so no test value is ever built into script text.
  await p.evaluate((fx) => {
    window.__posts = []; window.__reads = 0;
    window.Chronicle = {
      register: function (n, impl) { impl.init(document.getElementById('w')); },
      apiFetch: function (url, o) {
        if (o && o.method === 'POST') { window.__posts.push(o.body); return Promise.resolve({ ok: false, status: fx.refusal.status, json: function () { return Promise.resolve({ type: 'bad_request', message: fx.refusal.message }); } }); }
        if (url === '/rel') window.__reads++;
        var body = /\/room$/.test(url) ? fx.room : /\/buyers$/.test(url) ? fx.buyers : window.__posts.length ? fx.after : fx.before;
        return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(JSON.parse(JSON.stringify(body))); } });
      }
    };
  }, { refusal, room: ROOM, buyers: BUYERS, before: BEFORE, after });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_keeper.js', 'shop_room_counter.js', 'shop_room.js']) await p.addScriptTag({ content: read(f) });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForSelector('.src-svg [data-k="keeper"]');
  await p.click('.shr-cart');
  for (const id of ['11', '13', '15', '15']) await p.click('[data-add="' + id + '"]');
  return p;
}
const count = (p) => p.evaluate(() => document.querySelector('.shr-cnt').textContent);
const note = (p) => p.evaluate(() => { const n = document.querySelector('.shr-bk .shr-note'); return n ? n.textContent : ''; });

test('a buy someone beat you to reads the shelves again, settles the basket and says so plainly', { skip, timeout: 60000 }, async (t) => {
  const p = await shop(t, { status: 400, message: 'insufficient stock: 0 available, 1 requested' });
  assert.equal(await count(p), '4');
  const reads = await p.evaluate(() => window.__reads);
  await p.click('[data-buy]');
  await p.waitForFunction(() => { const n = document.querySelector('.shr-bk .shr-note'); return n && /last/.test(n.textContent); });
  assert.equal(await note(p), 'Someone just bought the last Healing Potion. Only 1 Toolbox left. Nothing was charged, and your basket is updated.');
  assert.ok(await p.evaluate((n) => window.__reads > n, reads), 'the wares are read again');
  assert.equal(await count(p), '2', 'the potion is gone and the toolbox cut to the one left');
  assert.doesNotMatch(await note(p), /insufficient/);
  // With no manner and no line of the GM's, the keeper still says it.
  await p.waitForFunction(() => /someone else already bought the last one/.test(document.querySelector('.shr-say').textContent));
  // The settled basket buys what is left.
  await p.click('[data-buy]');
  await p.waitForFunction(() => window.__posts.length === 2);
  assert.deepEqual(await p.evaluate(() => window.__posts[1].items), [{ relationId: 13, quantity: 1 }, { relationId: 15, quantity: 1 }]);
});

test('one thing sold out names it; any other refusal is shown as the server sent it', { skip, timeout: 60000 }, async (t) => {
  const p = await shop(t, { status: 400, message: 'this item is out of stock' });
  await p.evaluate(() => { document.querySelector('[data-empty]').click(); });
  await p.click('[data-add="11"]');
  await p.click('[data-buy]');
  await p.waitForFunction(() => { const n = document.querySelector('.shr-bk .shr-note'); return n && /last/.test(n.textContent); });
  assert.equal(await note(p), 'Someone just bought the last Healing Potion. Nothing was charged, and your basket is updated.');
  assert.equal(await count(p), '');

  const q = await shop(t, { status: 400, message: 'Brannoc can’t carry that much.' });
  await q.click('[data-buy]');
  await q.waitForFunction(() => { const n = document.querySelector('.shr-bk .shr-note'); return n && n.textContent.length > 0; });
  assert.equal(await note(q), 'Brannoc can’t carry that much.');
  assert.equal(await count(q), '4', 'the basket is left alone');
});

test('a cut-only basket says what is left, a listing taken off sale is not called bought, and the keeper stays quiet', { skip, timeout: 60000 }, async (t) => {
  const offSale = sells(11, 'Healing Potion', 2, 4); offSale.metadata.in_stock = false;
  const p = await shop(t, { status: 400, message: 'insufficient stock: 1 available, 2 requested' }, [offSale, sells(13, 'Fever tea', 1, null), sells(15, 'Toolbox', 4, 1)]);
  await p.click('[data-buy]');
  await p.waitForFunction(() => { const n = document.querySelector('.shr-bk .shr-note'); return n && /left/.test(n.textContent); });
  assert.equal(await note(p), 'Healing Potion is no longer for sale. Only 1 Toolbox left. Nothing was charged, and your basket is updated.');
  assert.equal(await count(p), '2');
  assert.doesNotMatch(await p.evaluate(() => document.querySelector('.shr-say') ? document.querySelector('.shr-say').textContent : ''), /someone else already bought/);

  const q = await shop(t, { status: 400, message: 'insufficient stock: 1 available, 2 requested' }, [sells(11, 'Healing Potion', 2, 4), sells(13, 'Fever tea', 1, null), sells(15, 'Toolbox', 4, 1)]);
  await q.click('[data-buy]');
  await q.waitForFunction(() => { const n = document.querySelector('.shr-bk .shr-note'); return n && /left/.test(n.textContent); });
  assert.equal(await note(q), 'Someone just bought some of these. Only 1 Toolbox left. Nothing was charged, and your basket is updated.');
  assert.equal(await count(q), '3');
  assert.doesNotMatch(await q.evaluate(() => document.querySelector('.shr-say') ? document.querySelector('.shr-say').textContent : ''), /someone else already bought/);
});
