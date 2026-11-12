// shop_rolls_browser.test.mjs — rolls at a shop's counter (static/js/widgets/shop_rolls.js
// inside shop_room.js), in a real Chromium against a fake of the shop routes.
//
// Haggle and Look closer show on the wares only when the owner switched them on; the sheet rolls
// through the server, shows the server's dice, and the price or note it changed; a note goes in
// as text; the owner switches rolls on in Arrange and they go into the saved room; and the sheet
// fits a phone above the phone shop's bottom bar.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally, and fails under CI,
// when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, 'static/js/widgets', f), 'utf8');

const sells = (id, name, price) => ({ id, relationType: 'sells', targetEntityId: 'e' + id, targetEntityName: name, targetEntityType: 'Goods', targetEntityIcon: 'fa-flask', metadata: { price, currency: 'gp', quantity: 3 } });
const GOODS = [sells(11, 'Healing draught', 50), sells(12, 'Old map', 5)];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', money: 500, moneyCp: 50000 }, { id: 'c2', name: 'Vex', own: true, kind: 'coins', moneyKey: 'gp', money: 9, moneyCp: 900 }] };
const layout = (rolls) => ({ layout: Object.assign({ version: 1, roomType: 'apothecary', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'lit', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: [] }, rolls ? { rolls } : {}) });
const TEST = { dice: '2d10', style: 'tiers', against: 'usual', haggle: { stat: 'presence', label: 'Presence', value: 2, mod: 2, missing: false }, look: { stat: 'reason', label: 'Reason', value: 1, mod: 1, missing: false } };

const HOST = (arrange) => `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body>
<div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="The Gilded Kettle" data-campaign-url="/campaigns/c1" data-csrf-token="t"${arrange ? ' data-can-arrange="true"' : ''}></div></body></html>`;

// The fake server keeps today's rolls, so a roll changes what the next read says.
const FAKE = (room, rollResult) => `window.__calls = []; window.__state = { goods: {} };
window.Chronicle = {
  escapeHtml: function (s) { return String(s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); },
  register: function (n, impl) { impl.init(document.getElementById('w')); },
  apiFetch: function (url, o) {
    var m = (o && o.method) || 'GET', body = o && o.body;
    if (body && typeof body !== 'string') body = JSON.stringify(body);
    window.__calls.push(m + ' ' + url + (body ? ' ' + body : ''));
    var ok = function (j) { return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(j); } }); };
    if (/\\/rolls\\?character=/.test(url)) return ok({ haggle: ${!!room.layout.rolls?.haggle}, look: ${!!room.layout.rolls?.look}, test: ${JSON.stringify(TEST)}, stats: [], goods: window.__state.goods });
    if (/\\/rolls$/.test(url) && m === 'POST') {
      var r = ${JSON.stringify(rollResult)}, b = JSON.parse(body), g = window.__state.goods[b.relationId] = window.__state.goods[b.relationId] || {};
      if (b.action === 'haggle') g.haggle = { tier: r.tier, pct: r.haggle.pct, price: r.haggle.price, was: r.haggle.was };
      else g.look = { tier: r.tier, note: r.look.note, extra: r.look.extra, today: true };
      return ok(r);
    }
    if (/\\/rolls$/.test(url)) return ok({ haggle: false, look: false, stats: [{ key: 'presence', label: 'Presence' }, { key: 'reason', label: 'Reason' }], defaults: { haggle: 'presence', look: 'reason' }, goods: {} });
    if (/\\/secrets$/.test(url)) return ok({ 12: { note: 'A coast is drawn in lemon juice.', extra: '' } });
    if (/\\/room$/.test(url) && m === 'PUT') return ok({ layout: JSON.parse(body) });
    if (/\\/room$/.test(url)) return ok(${JSON.stringify(room)});
    if (/\\/buyers$/.test(url)) return ok(${JSON.stringify(BUYERS)});
    if (/\\/today$/.test(url)) return ok({ pool: [], today: [] });
    return ok(${JSON.stringify(GOODS)});
  }
};`;

async function shop(t, { rolls, result, width = 1200, height = 800, arrange = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width, height } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent(HOST(arrange));
  await p.addScriptTag({ content: FAKE(layout(rolls), result || {}) });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_counter.js', 'shop_rolls.js', 'shop_room.js']) await p.addScriptTag({ content: read(f) });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  return p;
}
async function wares(p) {
  await p.click('.shr-front');
  await p.click('[data-cart="1"]');
  await p.waitForSelector('.shr-menu:not([hidden]) .shr-row');
}
const calls = (p) => p.evaluate(() => window.__calls);
const HAGGLED = { action: 'haggle', dice: [{ sides: 10, value: 9 }, { sides: 10, value: 8 }], modifier: 2, total: 19, tier: 3, label: 'Presence test', outcome: '20% off', haggle: { pct: -20, price: 40, was: 50, currency: 'gp' }, rollId: 'r1' };

test('with rolls off the wares show no Haggle or Look closer', { skip }, async (t) => {
  const p = await shop(t);
  await wares(p);
  assert.equal(await p.locator('[data-haggle], [data-look]').count(), 0);
});

test('Haggle rolls through the server, shows its dice and the new price, and the row keeps it', { skip }, async (t) => {
  const p = await shop(t, { rolls: { haggle: true, look: true, stubborn: 'usual', haggleStat: '', lookStat: '', great: 20, middling: 10, poor: 10 }, result: HAGGLED });
  await wares(p);
  await p.click('[data-haggle="11"]');
  await p.waitForSelector('.srl [data-srl="roll"]:not([disabled])');
  assert.equal(await p.textContent('.srl [data-srl="roll"]'), 'Presence test (2d10 + 2)');
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-srl')), 'roll', 'the roll button has focus');
  assert.equal(await p.locator('.srl [data-srl="char"] option').count(), 2, 'a player with two characters picks who rolls');
  await p.click('.srl [data-srl="roll"]');
  await p.waitForSelector('.srl .srl-after:not([hidden])');
  const sheet = await p.locator('.srl').innerText();
  assert.match(sheet, /9 \+ 8 \+ 2 = 19/);
  assert.match(sheet, /A great deal/);
  assert.match(sheet, /now 40 gp for you today/);
  assert.match(sheet, /Logged at the Table/);
  assert.deepEqual(await p.$$eval('.srl .srl-nums span', (s) => s.map((x) => x.textContent)), ['9', '8'], 'with no dice drawn here the numbers show');
  assert.ok((await calls(p)).includes('POST /campaigns/c1/armory/shops/s1/rolls {"relationId":11,"characterId":"c1","action":"haggle"}'));
  await p.click('.srl [data-srl="close"]');
  assert.equal(await p.locator('.srl').count(), 0);
  await p.waitForSelector('.shr-row[data-row="11"] .shr-price s');
  assert.match(await p.locator('.shr-row[data-row="11"]').innerText(), /50 gp\s+40 gp/);
  assert.equal(await p.locator('[data-haggle="11"]').count(), 0, 'one haggle per good a day');
  assert.match(await p.locator('.shr-row[data-row="11"]').innerText(), /Haggled today/);
});

test('Look closer shows the owner\'s note as text, never markup', { skip }, async (t) => {
  const note = 'Stamped <b>Ashgate</b> on the back';
  const p = await shop(t, { rolls: { look: true }, result: { action: 'look', dice: [{ sides: 10, value: 7 }, { sides: 10, value: 6 }], modifier: 1, total: 14, tier: 2, label: 'Reason test', outcome: 'Noticed something', look: { note, extra: '' }, rollId: 'r2' } });
  await wares(p);
  assert.equal(await p.locator('[data-haggle]').count(), 0, 'haggling is off in this shop');
  await p.click('[data-look="12"]');
  await p.waitForSelector('.srl [data-srl="roll"]:not([disabled])');
  await p.click('.srl [data-srl="roll"]');
  await p.waitForSelector('.srl .srl-after:not([hidden])');
  assert.equal(await p.textContent('.srl [data-srl-note]'), note);
  assert.equal(await p.locator('.srl [data-srl-note] b').count(), 0);
  await p.keyboard.press('Escape');
  await p.waitForSelector('.shr-row[data-row="12"] .shr-seen');
  assert.equal(await p.textContent('.shr-row[data-row="12"] .shr-seen'), 'You noticed: ' + note);
  assert.equal(await p.locator('.shr-row[data-row="12"] .shr-seen b').count(), 0);
});

test('the sheet\'s dice switch writes the Table\'s own setting', { skip }, async (t) => {
  const p = await shop(t, { rolls: { haggle: true }, result: HAGGLED });
  await p.evaluate(() => { const d = document.createElement('div'); d.className = 'tt-host'; d.setAttribute('data-widget', 'table'); d.setAttribute('data-campaign-id', 'c1'); d.setAttribute('data-user-id', 'u1'); document.body.appendChild(d);
    const m = {}; Object.defineProperty(window, 'localStorage', { value: { getItem: (k) => (k in m ? m[k] : null), setItem: (k, v) => { m[k] = String(v); } } }); });
  await wares(p);
  await p.click('[data-haggle="11"]');
  await p.click('.srl [data-srl-look="none"]');
  assert.equal(await p.evaluate(() => localStorage.getItem('chronicle.table.look.c1.u1')), 'none');
  assert.equal(await p.getAttribute('.srl [data-srl-look="none"]', 'aria-pressed'), 'true');
});

test('the owner switches rolls on in Arrange and they go into the saved room', { skip }, async (t) => {
  const p = await shop(t, { arrange: true });
  await p.click('.shr-front');
  await p.click('[data-mode="arr"]');
  await p.click('[data-sec="rolls"]');
  await p.click('[data-rsw="haggle"]');
  await p.selectOption('[data-rstat="haggle"]', 'reason');
  await p.fill('[data-rpct="great"]', '25');
  await p.dispatchEvent('[data-rpct="great"]', 'change');
  await p.click('[data-rstub="hard"]');
  await p.click('[data-rsw="look"]');
  await p.waitForSelector('[data-rnote="12"]');
  assert.match(await p.locator('.shr-rnotes').innerText(), /Has a hidden note/);
  await p.click('[data-saveroom="1"]');
  await p.waitForFunction(() => window.__calls.some((c) => c.startsWith('PUT /campaigns/c1/armory/shops/s1/room')));
  const put = (await calls(p)).find((c) => c.startsWith('PUT /campaigns/c1/armory/shops/s1/room'));
  const saved = JSON.parse(put.slice(put.indexOf('{')));
  assert.deepEqual(saved.rolls, { haggle: true, look: true, stubborn: 'hard', haggleStat: 'reason', lookStat: '', great: 25, middling: 10, poor: 10 });
  assert.equal(JSON.stringify(saved).includes('lemon'), false, 'hidden notes never go into the room');
});

test('at 390px the sheet fits above a 60px bottom bar and nothing scrolls sideways', { skip }, async (t) => {
  const p = await shop(t, { rolls: { haggle: true }, result: HAGGLED, width: 390, height: 780 });
  await wares(p);
  await p.evaluate(() => document.documentElement.style.setProperty('--srl-bottom', '60px'));
  await p.click('[data-haggle="11"]');
  await p.click('.srl [data-srl="roll"]');
  await p.waitForSelector('.srl .srl-after:not([hidden])');
  const box = await p.evaluate(() => { const r = document.querySelector('.srl').getBoundingClientRect(); return { l: r.left, r: r.right, b: r.bottom, t: r.top }; });
  assert.ok(box.l >= 0 && box.r <= 390, 'inside the screen: ' + JSON.stringify(box));
  assert.equal(Math.round(780 - box.b), 60, 'clear of the bottom bar');
  assert.ok(box.t > 0, 'the whole sheet is on screen');
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth), true);
});

test('with Realistic 3D picked, the site\'s dice are thrown to land on the server\'s numbers', { skip }, async (t) => {
  const p = await shop(t, { rolls: { haggle: true }, result: HAGGLED });
  await p.evaluate(() => {
    const d = document.createElement('div'); d.className = 'tt-host'; d.setAttribute('data-widget', 'table'); d.setAttribute('data-campaign-id', 'c1'); d.setAttribute('data-user-id', 'u1'); document.body.appendChild(d);
    const m = { 'chronicle.table.look.c1.u1': 'rune' }; Object.defineProperty(window, 'localStorage', { value: { getItem: (k) => (k in m ? m[k] : null), setItem: (k, v) => { m[k] = String(v); } } });
    window.__thrown = [];
    Chronicle.dice = { roll: (groups, opts) => { window.__thrown.push(groups); setTimeout(() => opts.onSettle([]), 50); return Promise.resolve([]); } };
  });
  await wares(p);
  await p.click('[data-haggle="11"]');
  assert.equal(await p.getAttribute('.srl [data-srl-look="rune"]', 'aria-pressed'), 'true');
  await p.click('.srl [data-srl="roll"]');
  await p.waitForSelector('.srl .srl-after:not([hidden])');
  const thrown = await p.evaluate(() => window.__thrown);
  assert.deepEqual(thrown[0][0].dice, [{ sides: 10, value: 9, crit: 'none' }, { sides: 10, value: 8, crit: 'none' }]);
});
