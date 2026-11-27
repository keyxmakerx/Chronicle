// shop_room_picture_browser.test.mjs — the owner's own picture in the counter
// room (static/js/widgets/shop_room_counter.js, shop_room_builder.js), in a
// real Chromium.
//
// The picture is drawn from its media id, never inside a baked layer (a baked
// layer is an image and cannot load another file), on the back wall or in the
// opening's view with the drawn skyline standing down. A host may hand the
// room its own address for the picture. In Arrange the owner picks one of the
// campaign's pictures (other files are left out), moves it, uploads a new one
// and takes it away, and Save room sends it in the theme. The view outside
// (a scene in place of the town) is picked and saved the same way.
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

const drawn = (p) => p.evaluate(() => [...document.querySelectorAll('.src-svg [data-pic]')].map((e) => ({
  href: e.getAttribute('href') || (e.ownerSVGElement.querySelector(e.getAttribute('fill').replace(/^url\((#[^)]+)\)$/, '$1') + ' image') || {}).getAttribute?.('href'),
  baked: !!e.closest('[data-bake]'), inOpening: !!e.closest('[clip-path="url(#cm-oclip)"]'), wall: !!e.closest('[data-pic-wall]'), aspect: e.getAttribute('preserveAspectRatio')
})));

test('the picture covers the back wall, outside every baked layer', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors, asked } = await open(browser, smithy({ id: A, on: 'wall', fit: 'fill' }));
  const pics = await drawn(p);
  assert.equal(pics.length, 1);
  assert.deepEqual(pics[0], { href: '/media/' + A, baked: false, inOpening: false, wall: true, aspect: 'xMidYMid slice' });
  await p.waitForFunction((a) => performance.getEntriesByType('resource').some((r) => r.name.endsWith('/media/' + a)), A);
  assert.ok(asked.includes('/media/' + A), 'the page asked for the picture by its id');
  assert.deepEqual(errors, []);
});

test('through the window the picture is the view, and the drawn skyline stands down', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithy({ id: A, on: 'outside', fit: 'fit' }));
  const pics = await drawn(p);
  assert.equal(pics.length, 1);
  assert.equal(pics[0].inOpening, true);
  assert.equal(pics[0].baked, false);
  assert.equal(pics[0].aspect, 'xMidYMid meet');
  const skyline = await p.evaluate(() => document.querySelectorAll('[clip-path="url(#cm-oclip)"] path[fill="#2a1e30"]').length);
  assert.equal(skyline, 0, 'no drawn buildings stand in front of the picture');
  const { p: plainP } = await open(browser, smithy(null));
  assert.ok(await plainP.evaluate(() => document.querySelectorAll('[clip-path="url(#cm-oclip)"] path[fill="#2a1e30"]').length) > 0, 'without a picture the skyline is drawn');
  assert.deepEqual(errors, []);
});

test('a host can hand the room its own address for the picture; an unsafe one draws nothing', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p } = await open(browser, smithy({ id: A, on: 'wall', fit: 'repeat' }), { before: 'window.ShopRoomCounter.pictureSrc = function (id) { return "/signed/" + id + "?sig=1&x=2"; };' });
  const pics = await drawn(p);
  assert.equal(pics.length, 1);
  assert.equal(pics[0].href, '/signed/' + A + '?sig=1&x=2');
  const { p: bad } = await open(browser, smithy({ id: A, on: 'wall', fit: 'fill' }), { before: 'window.ShopRoomCounter.pictureSrc = function () { return "javascript:alert(1)"; };' });
  assert.equal((await drawn(bad)).length, 0);
});

test('the owner picks, moves, uploads and takes away the picture, and Save room sends it', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithy(null));
  await p.click('[data-mode="arr"]');
  await p.waitForSelector('.shr-cb [data-cb-pic]', { state: 'attached' });
  await p.evaluate(() => { document.querySelector('[data-cb-pic]').closest('details').open = true; });
  await p.click('[data-cb-pics]');
  await p.waitForSelector('[data-cb-use]');
  assert.deepEqual(await p.$$eval('[data-cb-use]', (e) => e.map((x) => x.getAttribute('data-cb-use'))), [A, B], 'only the pictures are offered');
  await p.click('[data-cb-use="' + B + '"]');
  await p.waitForFunction((b) => [...document.querySelectorAll('.src-svg [data-pic]')].some((e) => e.getAttribute('href') === '/media/' + b), B);
  await p.click('[data-cb-picon="outside"]');
  await p.waitForFunction(() => !!document.querySelector('[clip-path="url(#cm-oclip)"] [data-pic]'));
  await p.setInputFiles('[data-cb-up]', { name: 'mine.jpg', mimeType: 'image/jpeg', buffer: JPEG });
  await p.waitForFunction((n) => [...document.querySelectorAll('.src-svg [data-pic]')].some((e) => e.getAttribute('href') === '/media/' + n), NEW);
  const up = await p.evaluate(() => window.__calls.find((c) => c.url === '/media/upload'));
  assert.deepEqual(up.body, { file: 'file', usage_type: 'attachment', campaign_id: 'c1' });
  // The same picture uploaded again comes back as the file the campaign has, and is listed once.
  await p.setInputFiles('[data-cb-up]', { name: 'mine.jpg', mimeType: 'image/jpeg', buffer: JPEG });
  await p.waitForFunction(() => window.__calls.filter((c) => c.url === '/media/upload').length === 2);
  await p.waitForSelector('[data-cb-use]');
  assert.equal(await p.$$eval('[data-cb-use="' + NEW + '"]', (e) => e.length), 1);
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.some((c) => c.m === 'PUT'));
  const put = await p.evaluate(() => window.__calls.find((c) => c.m === 'PUT'));
  assert.deepEqual(put.body.counter.theme.picture, { id: NEW, on: 'outside', fit: 'fill' }, 'the new picture keeps where the last one was');
  await p.click('[data-mode="arr"]');
  await p.waitForSelector('.shr-cb [data-cb-pic]', { state: 'attached' });
  await p.evaluate(() => { document.querySelector('[data-cb-pic]').closest('details').open = true; });
  await p.click('[data-cb-picoff]');
  await p.waitForFunction(() => !document.querySelector('.src-svg [data-pic]'));
  assert.deepEqual(errors, []);
});

// The view outside is part of the same backdrop: picked in Arrange, drawn in
// the opening in place of the town, and saved without a word for the town.
test("the shop floor's door shows the window's sky, view and picture", { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const pane = async (counter) => {
    const { p, errors } = await open(browser, counter);
    await p.click('.src-turn');
    await p.waitForSelector('[data-k="door"] [clip-path="url(#srcf-paneclip)"]');
    const h = await p.evaluate(() => document.querySelector('[data-k="door"] [clip-path="url(#srcf-paneclip)"]').innerHTML);
    assert.deepEqual(errors, []);
    return h;
  };
  const night = (scene, picture) => { const c = smithy(picture || null); c.theme.outside = 'night'; if (scene) c.theme.scene = scene; return c; };
  const town = await pane(night(''));
  const castle = await pane(night('castle'));
  assert.match(town, /stop-color="#0b1430"/, 'the night the theme names, not the shop’s own time');
  assert.notEqual(castle, town, 'the view outside follows the theme');
  const dusk = smithy(null); dusk.theme.scene = 'castle';
  assert.match(await pane(dusk), /stop-color="#2a2a5a"/, 'and so does the sky');
  assert.match(await pane(night('', { id: A, on: 'outside', fit: 'fill' })), /data-pic/, 'a picture through the window is through the door too');
});

test('the owner picks the view outside, and Save room sends it', { skip, timeout: 120000 }, async (t) => {
  const browser = await launch(t);
  const { p, errors } = await open(browser, smithy(null));
  const view = () => p.evaluate(() => document.querySelector('[clip-path="url(#cm-oclip)"]').innerHTML);
  const town = await view();
  await p.click('[data-mode="arr"]');
  const sel = '.shr-cb select[data-cb-path="theme.scene"]';
  await p.waitForSelector(sel, { state: 'attached' });
  await p.evaluate((s) => { document.querySelector(s).closest('details').open = true; }, sel);
  assert.equal(await p.$eval(sel, (e) => e.value), 'town', 'a theme with no view shows the town');
  await p.selectOption(sel, 'castle');
  await p.waitForFunction((was) => document.querySelector('[clip-path="url(#cm-oclip)"]').innerHTML !== was, town);
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.some((c) => c.m === 'PUT'));
  assert.equal((await p.evaluate(() => window.__calls.find((c) => c.m === 'PUT'))).body.counter.theme.scene, 'castle');
  await p.click('[data-mode="arr"]');
  await p.waitForSelector(sel, { state: 'attached' });
  await p.evaluate((s) => { document.querySelector(s).closest('details').open = true; }, sel);
  await p.selectOption(sel, 'town');
  await p.waitForFunction((was) => document.querySelector('[clip-path="url(#cm-oclip)"]').innerHTML === was, town);
  await p.click('button:has-text("Save room")');
  await p.waitForFunction(() => window.__calls.filter((c) => c.m === 'PUT').length === 2);
  assert.equal('scene' in (await p.evaluate(() => window.__calls.filter((c) => c.m === 'PUT')[1])).body.counter.theme, false, 'the town is saved as no view');
  assert.deepEqual(errors, []);
});
