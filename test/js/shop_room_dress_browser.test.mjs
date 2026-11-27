// shop_room_dress_browser.test.mjs — dressing the counter room by its parts
// (static/js/widgets/shop_room_counter.js, shop_room_builder.js), in a real
// Chromium.
//
// In Arrange the builder shows the room's parts as tabs, each choice a small
// picture of this room dressed that way. Pressing a part of the room opens
// its tab. The room pulls apart into its three layers, each pressable and
// hideable. The owner's picture lends the room its colours.
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



const smithyWith = (f) => { const c = smithy(null); c.props = ['anvil', 'banner']; if (f) f(c); return c; };
const sel = (p) => p.evaluate(() => { const b = document.querySelector('[data-cb-part][aria-selected="true"]'); return b && b.getAttribute('data-cb-part'); });
async function arrange(p) {
  await p.click('[data-mode="arr"]');
  await p.waitForSelector('.shr-cb [data-cb-part]', { state: 'attached' });
}

test('the parts as tabs: each choice is a picture of the room, and a press dresses it', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithyWith());
  await arrange(p);
  assert.equal(await sel(p), 'wall');
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-tile="theme.wall"][data-cb-v="stone"]').getAttribute('aria-pressed')), 'true', 'the wall in force is marked');
  // The room draws the pictures, a few at a time.
  await p.waitForFunction(() => [...document.querySelectorAll('[data-cb-tile="theme.wall"] img')].filter((i) => i.naturalWidth > 0).length >= 3, null, { timeout: 60000 });
  await p.click('[data-cb-tile="theme.wall"][data-cb-v="brick"]');
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-tile="theme.wall"][data-cb-v="brick"]').getAttribute('aria-pressed')), 'true');
  // The tabs move with the arrow keys, as tabs do.
  await p.focus('[data-cb-part="wall"]');
  await p.keyboard.press('ArrowRight');
  assert.equal(await sel(p), 'window');
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-cb-part')), 'window');
  await p.keyboard.press('End');
  assert.equal(await sel(p), 'props');
  // A prop tile puts the prop in the room or takes it out.
  await p.click('[data-cb-tile="props"][data-cb-v="anvil"]');
  await p.waitForFunction(() => !document.querySelector('.src-svg [data-mvk="anvil"]'));
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.some((c) => c.m === 'PUT'));
  const saved = (await p.evaluate(() => window.__calls.find((c) => c.m === 'PUT'))).body.counter;
  assert.equal(saved.theme.wall, 'brick');
  assert.deepEqual(saved.props, ['banner']);
  assert.deepEqual(errors, []);
});

test('pressing a part of the room opens its choices', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithyWith());
  await arrange(p);
  const at = (q, fx, fy) => p.evaluate(([q, fx, fy]) => { const b = document.querySelector(q).getBoundingClientRect(); return [b.left + b.width * fx, b.top + b.height * fy]; }, [q, fx, fy]);
  let w = await at('.src-svg [data-k="window"]', .1, .5);
  await p.mouse.click(w[0], w[1]);
  await p.waitForFunction(() => document.querySelector('[data-cb-part="window"]').getAttribute('aria-selected') === 'true');
  w = await at('.src-svg rect[data-k="shelves"]', .5, .2);
  await p.mouse.click(w[0], w[1]);
  await p.waitForFunction(() => document.querySelector('[data-cb-part="shelves"]').getAttribute('aria-selected') === 'true');
  w = await at('.src-svg [data-mvk="anvil"]', .45, .3);
  await p.mouse.click(w[0], w[1]);
  await p.waitForFunction(() => document.querySelector('[data-cb-part="props"]').getAttribute('aria-selected') === 'true');
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-cb-v')), 'anvil', 'the pressed prop has the keyboard');
  // Shopping presses are back once the owner leaves Arrange.
  await p.click('[data-mode="shop"]');
  assert.match(await p.evaluate(() => document.querySelector('.src-hint').textContent), /Press a good/);
  assert.deepEqual(errors, []);
});

test('the room pulls apart into its layers: press one to dress it, hide one to see through', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithyWith());
  await arrange(p);
  // Leaving Arrange puts the room back together.
  await p.click('[data-cb-layers]');
  await p.waitForSelector('.cm-lay');
  await p.click('[data-mode="shop"]');
  await p.waitForFunction(() => !document.querySelector('.cm-lay'));
  await arrange(p);
  await p.click('[data-cb-layers]');
  await p.waitForSelector('.cm-lay.open');
  assert.equal(await p.evaluate(() => document.querySelectorAll('.cm-lay .cm-ly').length), 3);
  assert.equal(await p.evaluate(() => document.querySelector('.cm-lay.calm') !== null), true, 'calm motion lays them out flat');
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-layers]').textContent), 'Put the room back together');
  // The back holds the wall but not the counter; the front holds the counter and props.
  assert.equal(await p.evaluate(() => !!document.querySelector('.cm-ly1 [data-depth=".55"]') && !document.querySelector('.cm-ly1 [data-depth="1"]')), true);
  assert.equal(await p.evaluate(() => !!document.querySelector('.cm-ly2 [data-depth="1"]') && !document.querySelector('.cm-ly2 [data-depth=".55"]')), true);
  assert.equal(await p.evaluate(() => document.querySelectorAll('.cm-lay [tabindex]:not(button)').length), 0, 'the copies take no keyboard stops');
  await p.click('[data-ly-eye="wall"]', { force: true });
  assert.equal(await p.evaluate(() => document.querySelector('.cm-ly1').classList.contains('hid')), true);
  assert.equal(await p.evaluate(() => document.querySelector('[data-ly-eye="wall"]').getAttribute('aria-pressed')), 'false');
  await p.click('[data-ly="back"]', { force: true });
  assert.equal(await sel(p), 'window');
  await p.click('[data-ly="front"]', { force: true });
  assert.equal(await sel(p), 'counter');
  // A change redraws the layers, and they stay hidden as the owner left them.
  await p.click('[data-cb-tile="skeleton.counter"]:not([aria-pressed="true"])');
  await p.waitForFunction(() => document.querySelector('.cm-ly1') && document.querySelector('.cm-ly1').classList.contains('hid'));
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => !document.querySelector('.cm-lay'));
  assert.equal(await p.evaluate(() => document.querySelector('[data-cb-layers]').textContent), 'Pull the room apart into its layers');
  assert.deepEqual(errors, []);
});

test("the owner's picture lends the room its colours", { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithyWith((c) => { c.theme.picture = { id: A, on: 'wall', fit: 'fill' }; }));
  await arrange(p);
  await p.evaluate(() => { const d = document.querySelector('[data-cb-pic]').closest('details'); d.open = true; });
  await p.waitForSelector('[data-cb-pal]');
  assert.ok(await p.evaluate(() => document.querySelectorAll('[data-cb-palbox] i').length) >= 1, 'the picture’s colours show');
  await p.click('[data-cb-pal]');
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.some((c) => c.m === 'PUT'));
  const th = (await p.evaluate(() => window.__calls.find((c) => c.m === 'PUT'))).body.counter.theme;
  for (const k of ['wood', 'cloth', 'accent', 'trim']) assert.match(th[k], /^#[0-9a-f]{6}$/, k);
  assert.notEqual(th.trim, '#9aa0a8', 'the trim follows the picture');
  assert.ok(th.warmth >= 0 && th.warmth <= 1);
  assert.deepEqual(errors, []);
});
