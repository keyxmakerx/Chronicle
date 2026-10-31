// shop_room_walk_browser.test.mjs — the shop's door and the room behind it
// (static/js/widgets/shop_room.js), in a real Chromium.
//
// Closed, the shop is a door in the middle of its block with the name and
// "Step inside" under it, never squashed by a host's button styles (Foundry
// styles every button). Pressing it walks into a room that fills the window;
// the basket in the room's corner peeks on hover and opens the wares on a
// press; Escape closes the basket first, then leaves.
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

const GOODS = [
  { id: 11, relationType: 'sells', targetEntityId: 'e1', targetEntityName: 'Healing Potion', targetEntityType: 'Potion', metadata: { price: 2, currency: 'gp', quantity: 4 } },
  { id: 12, relationType: 'sells', targetEntityId: 'e2', targetEntityName: 'Antitoxin', targetEntityType: 'Potion', metadata: { price: 5, currency: 'gp', quantity: 0 } },
];
const BUYERS = { canBuyNow: true, buyers: [{ id: 'c1', name: 'Brannoc', own: true, kind: 'coins', moneyKey: 'gp', moneyCp: 5000 }] };

// A host page: Foundry's own rule for every button, in a cascade layer as
// Foundry ships it, so the door has to hold its shape against it.
const HOST = `<!doctype html><html data-motion="__MOTION__"><head><style>
@layer foundry { button { height: 28px; line-height: 26px; background: #fff; border: 1px solid #999; } }
[hidden] { display: none }
body { margin: 0; width: 900px; }
</style></head><body><div style="padding:20px"><div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="Hessa Ord's Workshop" data-campaign-url="/campaigns/c1" data-csrf-token="t"></div></div></body></html>`;

async function page(t, motion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1200, height: 800 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent(HOST.replace('__MOTION__', motion || ''));
  await p.addScriptTag({ content: `window.Chronicle = {
    register: function (n, impl) { impl.init(document.getElementById('w')); },
    apiFetch: function (url) {
      var body = /\\/room$/.test(url) ? { layout: null } : /\\/buyers$/.test(url) ? ${JSON.stringify(BUYERS)} : ${JSON.stringify(GOODS)};
      return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
    }
  };` });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room.js']) await p.addScriptTag({ content: read(f) });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  return p;
}

test('closed, the door stands centred with its name and "Step inside" under it', { skip }, async (t) => {
  const p = await page(t);
  const s = await p.evaluate(() => {
    const f = document.querySelector('.shr-front'), w = document.getElementById('w').getBoundingClientRect(), r = f.getBoundingClientRect();
    const dr = f.querySelector('.shr-dr').getBoundingClientRect(), nm = f.querySelector('.nm').getBoundingClientRect(), hint = f.querySelector('.hint').getBoundingClientRect();
    return { offCentre: Math.abs((r.left + r.right) / 2 - (w.left + w.right) / 2), doorH: dr.height, frontH: r.height, nameBelow: nm.top >= dr.bottom - 1, hintBelow: hint.top >= nm.bottom - 1,
      name: f.querySelector('.nm').textContent, hint: f.querySelector('.hint').textContent, open: document.querySelector('.shr-dlg').open };
  });
  assert.ok(s.offCentre < 2, 'door is centred, off by ' + s.offCentre);
  assert.equal(s.doorH, 144);
  assert.ok(s.frontH > 180, 'the host button height does not squash it: ' + s.frontH);
  assert.ok(s.nameBelow && s.hintBelow, 'name, then "Step inside", under the door');
  assert.equal(s.name, "Hessa Ord's Workshop");
  assert.equal(s.hint, 'Step inside');
  assert.equal(s.open, false);
});

test('pressing the door walks into a room that fills the window', { skip }, async (t) => {
  const p = await page(t);
  await p.click('.shr-front');
  assert.equal(await p.evaluate(() => document.querySelector('.shr-front').classList.contains('opening')), true, 'the door swings first');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open, null, { timeout: 3000 });
  const s = await p.evaluate(() => {
    const d = document.querySelector('.shr-dlg').getBoundingClientRect(), st = document.querySelector('.shr-stage').getBoundingClientRect(), sc = document.querySelector('.shr-scene').getBoundingClientRect();
    return { w: d.width, h: d.height, inStage: sc.width <= st.width + 1 && sc.height <= st.height + 1, fills: Math.max(sc.width / st.width, sc.height / st.height) };
  });
  assert.equal(s.w, 1200); assert.equal(s.h, 800);
  assert.ok(s.inStage, 'the scene fits in the room');
  assert.ok(s.fills > 0.99, 'and grows to the room on one side');
  await p.waitForFunction(() => !document.querySelector('.shr-hole'), null, { timeout: 3000 });
});

test('the basket peeks on hover, opens the wares on a press, and counts what is added', { skip }, async (t) => {
  const p = await page(t, 'off');
  await p.click('.shr-front');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open);
  await p.hover('.shr-cart');
  assert.match(await p.textContent('.shr-peek.on'), /Your basket is empty/);
  await p.click('.shr-cart');
  assert.equal(await p.isVisible('.shr-menu'), true);
  assert.equal(await p.evaluate(() => document.querySelector('.shr-peek').classList.contains('on')), false, 'the peek steps aside for the menu');
  assert.equal(await p.getAttribute('.shr-cart', 'aria-expanded'), 'true');
  assert.equal(await p.textContent('.shr-mh h3 span'), '2');
  assert.equal(await p.isDisabled('.shr-row[data-row="12"] .shr-add'), true, 'sold out cannot be added');
  await p.click('.shr-row[data-row="11"] [data-add]');
  await p.click('.shr-row[data-row="11"] [data-add]');
  assert.equal(await p.textContent('.shr-cnt'), '2');
  assert.match(await p.textContent('.shr-bk'), /2 in basket/);
  assert.match(await p.textContent('.shr-peek'), /2 × Healing Potion[\s\S]*Total[\s\S]*4 gp/);
});

test('Escape closes the basket first, then leaves through the Leave control', { skip }, async (t) => {
  const p = await page(t, 'off');
  await p.click('.shr-front');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open);
  await p.click('.shr-cart');
  await p.keyboard.press('Escape');
  assert.equal(await p.isVisible('.shr-menu'), false);
  assert.equal(await p.evaluate(() => document.querySelector('.shr-dlg').open), true, 'still inside');
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => !document.querySelector('.shr-dlg').open);
  assert.equal(await p.getAttribute('.shr-front', 'aria-expanded'), 'false');
  assert.equal(await p.evaluate(() => document.activeElement && document.activeElement.classList.contains('shr-front')), true, 'focus is back on the door');
});

test('leaving shrinks the room back into the door under full motion', { skip }, async (t) => {
  const p = await page(t);
  await p.click('.shr-front');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open, null, { timeout: 3000 });
  await p.click('.shr-out');
  assert.equal(await p.evaluate(() => document.querySelector('.shr-dlg').classList.contains('leaving')), true);
  await p.waitForFunction(() => !document.querySelector('.shr-dlg').open, null, { timeout: 2000 });
  assert.equal(await p.evaluate(() => document.querySelector('.shr-front').classList.contains('opening')), false, 'the door swings shut');
});

test('a redraw keeps the flames and glows going rather than starting them over', { skip }, async (t) => {
  const p = await page(t);
  await p.click('.shr-front');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open, null, { timeout: 3000 });
  await p.waitForTimeout(1200);
  const loops = () => p.evaluate(() => document.querySelector('.shr-iso').getAnimations({ subtree: true })
    .filter((a) => a.effect.getComputedTiming().iterations === Infinity).map((a) => a.currentTime));
  const before = await loops();
  assert.ok(before.length > 0, 'the lit room has looping animations');
  // Picking the same effects level redraws the whole room.
  await p.click('.shr-fx button[data-fx="full"]');
  const after = await loops();
  assert.ok(Math.min(...after) >= Math.min(...before), 'loops carry on from where they were: ' + Math.min(...before) + ' then ' + Math.min(...after));
});
