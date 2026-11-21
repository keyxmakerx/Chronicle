// shop_room_parts_browser.test.mjs — the counter room's parts packs
// (static/js/widgets/shop_room_parts_*.js inside shop_room.js), in a real
// Chromium.
//
// Every starter room built from the packs draws without an error, stands its
// goods where they can be pressed and brings one to the counter. With motion
// off, none of a room's moving parts is left running. Rooms mixed from old
// and new parts draw too: mirrored, open to the air, under domes, and with as
// many props as a theme may hold. The builder lists every starter, applies
// one from a pack, and stops at the number of props the server takes.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';
import { launch as launchBuilder, openArranged } from './_design_library_harness.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, 'static/js/widgets', f), 'utf8');
const ROOM_FILES = ['shop_room_counter.js', 'shop_room_parts_strange.js', 'shop_room_parts_trades.js', 'shop_room_parts_places.js'];
const SCRIPTS = ['shop_room_icons.js', 'shop_room_paper.js', ...ROOM_FILES, 'shop_room.js'].map(read);

const sells = (id, name, icon, price, quantity) => ({ id, relationType: 'sells', targetEntityId: 'e' + id, targetEntityName: name, targetEntityType: 'Goods', targetEntityIcon: 'fa-' + icon, metadata: { price, currency: 'gp', quantity } });
const GOODS = [sells(11, 'Healing Potion', 'flask', 2, 4), sells(12, 'Spellbook', 'book', 25, 1), sells(13, 'Bandages', 'bandage', 1, 6), sells(14, 'Dagger', 'dagger', 2, 2)];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', money: 500, moneyCp: 50000 }] };
const room = (counter) => ({ layout: { version: 1, roomType: 'general', setting: 'room', size: 'm', furniture: 'normal', decorations: 'some', palette: 'oak', seeds: { room: 1, goods: 1, deco: 1 }, look: 'counter', mood: '', effects: null, pieces: [], decor: [], items: {}, portrait: null, lines: ['Evening.'], counter } });

const HOST = `<!doctype html><html data-motion="calm"><head><style>[hidden] { display: none } body { margin: 0 }</style></head><body>
<div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="The Shop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></body></html>`;

async function launch(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  return browser.newContext({ viewport: { width: 1200, height: 800 } });
}

// The widget's own lists, read from the files as a page loads them.
async function lists(ctx) {
  const p = await ctx.newPage();
  await p.setContent('<!doctype html><html><body></body></html>');
  for (const f of ROOM_FILES) await p.addScriptTag({ content: read(f) });
  const got = await p.evaluate(() => ({ starters: window.ShopRoomCounter.starters, props: window.ShopRoomCounter.options.props }));
  await p.close();
  return got;
}

// Opens a shop whose room is this theme and says what happened there.
async function open(ctx, theme) {
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent(HOST);
  await p.addScriptTag({ content: `window.Chronicle = {
    register: function (n, impl) { impl.init(document.getElementById('w')); },
    apiFetch: function (url) {
      var body = /\\/room$/.test(url) ? ${JSON.stringify(room(theme))} : /\\/buyers$/.test(url) ? ${JSON.stringify(BUYERS)} : ${JSON.stringify(GOODS)};
      return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(JSON.parse(JSON.stringify(body))); } });
    }
  };` });
  for (const s of SCRIPTS) await p.addScriptTag({ content: s });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForSelector('.src-svg [data-k="keeper"]');
  const seen = await p.evaluate(() => ({
    wares: document.querySelectorAll('#srcb-wares [data-k^="ware:"]').length,
    moving: document.querySelector('.src-stage').getAnimations({ subtree: true }).filter((a) => a instanceof CSSAnimation && a.playState === 'running').length,
    hint: document.querySelector('.src-hint').textContent
  }));
  await p.evaluate(() => document.querySelector('#srcb-wares [data-k^="ware:"]').dispatchEvent(new MouseEvent('click', { bubbles: true })));
  await p.waitForSelector('[data-k^="held:"]');
  await p.close();
  return { ...seen, errors };
}

test('every starter room from the packs draws, holds still with motion off, and hands over a good', { skip, timeout: 240000 }, async (t) => {
  const ctx = await launch(t);
  const { starters } = await lists(ctx);
  assert.equal(starters.length, 32);
  for (const s of starters.slice(5)) {
    const got = await open(ctx, s);
    assert.deepEqual(got.errors, [], s.name);
    assert.ok(got.wares >= 2, s.name + ' stands its goods on its display');
    assert.equal(got.moving, 0, s.name + ' has nothing moving while motion is off');
    if (s.skeleton.display !== 'shelves' && s.skeleton.display !== 'pegboard' && s.skeleton.display !== 'drawers' && s.skeleton.display !== 'pigeonholes') assert.doesNotMatch(got.hint, /on the shelves/, s.name + ' names its own display in the hint');
  }
});

test('rooms mixed from old and new parts draw', { skip, timeout: 120000 }, async (t) => {
  const ctx = await launch(t);
  const { props } = await lists(ctx);
  const paint = { wood: '#5a3a22', cloth: '#7a2a22', accent: '#e8a040', trim: '#b89a58' };
  const mixes = [
    { name: 'Mirrored', version: 1, skeleton: { shape: 'cavern', opening: 'window-left', display: 'casks', counter: 'bar' }, theme: { wall: 'marble', light: 'sunlight', warmth: 0.5, outside: 'rain', ...paint }, props: props.slice(15, 39) },
    { name: 'Open to the air', version: 1, skeleton: { shape: 'open-air', opening: 'none', display: 'clouds', counter: 'coin-drift' }, theme: { wall: 'glazing', light: 'jellyfish', warmth: 0.2, outside: 'day', ...paint }, props: props.slice(39, 63) },
    { name: 'Under domes', version: 1, skeleton: { shape: 'gondola', opening: 'dragon-eye', display: 'domes', counter: 'ice-block' }, theme: { wall: 'faded-paper', light: 'candelabra', warmth: 0.9, outside: 'dusk', ...paint }, props: props.slice(63, 87) },
    { name: 'An old room with new props', version: 1, skeleton: { shape: 'round', opening: 'doorway', display: 'hoard', counter: 'curved' }, theme: { wall: 'night-sky', light: 'forge', warmth: 0.9, outside: 'night', ...paint }, props: props.filter((p, i) => i % 4 === 0) },
    { name: 'A new room with old parts', version: 1, skeleton: { shape: 'conservatory', opening: 'sun-disc', display: 'pegboard', counter: 'stall-table' }, theme: { wall: 'chart', light: 'moonlight', warmth: 0.9, outside: 'night', ...paint }, props: props.slice(0, 15) }
  ];
  for (const m of mixes) {
    const got = await open(ctx, m);
    assert.deepEqual(got.errors, [], m.name);
    assert.ok(got.wares >= 2, m.name);
    assert.equal(got.moving, 0, m.name);
  }
});

test('the builder offers every starter, applies one from a pack, and stops at 24 props', { skip, timeout: 120000 }, async (t) => {
  const browser = await launchBuilder();
  t.after(() => browser.close());
  const { page: p, errors } = await openArranged(browser);
  t.after(() => assert.deepEqual(errors, []));
  assert.equal(await p.evaluate(() => document.querySelectorAll('.shr-cb [data-cb-start]').length), 32);
  await p.evaluate(() => Array.from(document.querySelectorAll('.shr-cb [data-cb-start]')).find((b) => b.textContent === 'The Taproom').click());
  await p.waitForFunction(() => document.querySelector('[data-cb-path="skeleton.display"]').value === 'casks');
  const seen = await p.evaluate(() => ({
    light: document.querySelector('[data-cb-path="theme.light"]').value,
    ticked: Array.from(document.querySelectorAll('.shr-cb [data-cb-prop]')).filter((c) => c.checked).map((c) => c.getAttribute('data-cb-prop')),
    hold: document.querySelector('[data-cb-path="skeleton.shape"] option[value="hold"]').textContent,
    say: document.querySelector('.shr-cb [data-cb-say]').textContent
  }));
  assert.equal(seen.light, 'firelight');
  assert.deepEqual(seen.ticked.sort(), ['antlers', 'keg', 'smoke', 'tankards']);
  assert.equal(seen.hold, 'Ship’s hold');
  assert.match(seen.say, /The Taproom/);
  // Ticking goes on until the theme is full, and the next tick is handed back with a reason.
  const tick = () => p.evaluate(() => { const c = Array.from(document.querySelectorAll('.shr-cb [data-cb-prop]')).find((x) => !x.checked); c.click(); return c.checked; });
  for (let n = 4; n < 24; n++) assert.equal(await tick(), true, 'prop ' + (n + 1));
  assert.equal(await tick(), false, 'the 25th prop is not taken');
  assert.match(await p.evaluate(() => document.querySelector('.shr-cb [data-cb-say]').textContent), /up to 24 props/);
  assert.equal(await p.evaluate(() => document.querySelectorAll('.shr-cb [data-cb-prop]:checked').length), 24);
});

test('at phone width the builder with every starter and prop stays inside its panel', { skip, timeout: 60000 }, async (t) => {
  const browser = await launchBuilder();
  t.after(() => browser.close());
  const { page: p, errors } = await openArranged(browser, { width: 400, height: 760 });
  t.after(() => assert.deepEqual(errors, []));
  const fits = await p.evaluate(() => { const b = document.querySelector('.shr-panel'); return [b.scrollWidth <= b.clientWidth + 1, document.documentElement.scrollWidth <= innerWidth]; });
  assert.deepEqual(fits, [true, true]);
});
