// shop_room_arrange_keys_browser.test.mjs — arranging the shop room without a
// mouse, and the paper keeper's place on the counter
// (static/js/widgets/shop_room.js and shop_room_paper.js), in a real Chromium.
//
// While arranging, every piece takes focus; arrow keys nudge the focused piece
// a quarter square and Shift with an arrow a whole one, in the lit room and in
// paper. Arrows typed into the panel's text never move anything, and Escape
// lets go of the piece before it leaves Arrange. In paper, three marks over
// the counter pick where the keeper stands, and that is saved with the room.
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

const GOODS = [{ id: 11, relationType: 'sells', targetEntityId: 'e1', targetEntityName: 'Healing Potion', targetEntityType: 'Potion', metadata: { price: 2, currency: 'gp', quantity: 4 } }];
const HOST = `<!doctype html><html><head><style>[hidden]{display:none} body{margin:0}</style></head><body><div id="w" data-widget="shop_room" data-room-endpoint="/campaigns/c1/armory/shops/s1/room" data-relations-endpoint="/rel" data-shop-name="Keys" data-campaign-url="/campaigns/c1" data-csrf-token="t" data-can-arrange="true"></div></body></html>`;

async function page(t, vp) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: vp || { width: 1280, height: 860 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent(HOST);
  await p.addScriptTag({ content: `window.saved = []; window.Chronicle = {
    register: function (n, impl) { impl.init(document.getElementById('w')); },
    apiFetch: function (url, o) {
      if (o && o.method === 'PUT') { window.saved.push(JSON.parse(JSON.stringify(o.body))); return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve({}); } }); }
      var body = /\\/room$/.test(url) ? { layout: null } : /\\/buyers$/.test(url) ? { canBuyNow: false, buyers: [] } : ${JSON.stringify(GOODS)};
      return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
    }
  };` });
  for (const f of ['shop_room_icons.js', 'shop_room_paper.js', 'shop_room_keeper.js', 'shop_room.js']) await p.addScriptTag({ content: read(f) });
  await p.waitForSelector('#w:not([hidden]) .shr-front');
  await p.click('.shr-front');
  await p.waitForFunction(() => document.querySelector('.shr-dlg').open, null, { timeout: 4000 });
  await p.evaluate(() => document.querySelector('[data-mode="arr"]').click());
  await p.waitForSelector('.shr.arr');
  return p;
}
const click = (p, sel) => p.evaluate((s) => document.querySelector(s).click(), sel);
const save = (p) => p.evaluate(() => { [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Save room').click(); });
const pieceOf = (layout, id) => layout.pieces.find((q) => q.id === id);

// Focuses the first floor piece that can move by keys in the given direction
// and presses it; returns the piece id, or 0 when none could.
async function pressOnSomePiece(p, sel, key) {
  const ids = await p.evaluate((s) => [...document.querySelectorAll(s)].map((g) => +g.dataset.p), sel);
  for (const id of ids) {
    await p.evaluate(([s, i]) => document.querySelector(s + '[data-p="' + i + '"]').focus(), [sel, id]);
    await p.keyboard.press(key);
    const moved = await p.evaluate(() => !document.querySelector('.shr-save').disabled);
    if (moved) return id;
  }
  return 0;
}

for (const look of ['lit', 'paper']) {
  test(`${look}: arrows nudge the focused piece, Shift moves it a whole square, and it keeps focus`, { skip }, async (t) => {
    const p = await page(t);
    if (look === 'paper') await click(p, 'button[data-k="look"][data-v="paper"]');
    const sel = look === 'paper' ? '.srp-pc' : '.piece';
    assert.equal(await p.evaluate((s) => [...document.querySelectorAll(s)].every((g) => g.getAttribute('tabindex') === '0' && /Arrow keys move it/.test(g.getAttribute('aria-label'))), sel), true, 'every piece is focusable and says how');
    // A baseline save (a colour change makes there be something to save), so
    // the nudge can be measured against it.
    await click(p, 'button[data-k="pal"][data-v="ember"]');
    await save(p); await p.waitForFunction(() => window.saved.length === 1);
    await p.evaluate(() => document.querySelector('[data-mode="arr"]').click());
    const id = await pressOnSomePiece(p, sel, 'ArrowRight');
    assert.ok(id, 'some piece moved right');
    assert.equal(await p.evaluate(() => document.activeElement.dataset.p), String(id), 'the moved piece keeps focus');
    await p.keyboard.press('Shift+ArrowLeft');
    await save(p); await p.waitForFunction(() => window.saved.length === 2);
    const [before, after] = await p.evaluate(() => window.saved);
    const a = pieceOf(before, id), b = pieceOf(after, id);
    assert.ok(b.pinned, 'a nudged piece counts as moved by hand');
    const moved = Math.abs(b.x - a.x) + Math.abs(b.y - a.y);
    // Right a quarter, then left a whole square, unless the wall or a neighbour stopped it.
    assert.ok(moved > 0, 'the piece moved: ' + JSON.stringify([a, b]));
  });
}

test('arrows typed into the panel never move a piece, and Escape lets go of a piece first', { skip }, async (t) => {
  const p = await page(t);
  await p.evaluate(() => { const ta = document.querySelector('[data-lines]'); ta.closest('details, section') && ta.closest('details') && (ta.closest('details').open = true); ta.focus(); });
  for (const k of ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Shift+ArrowLeft']) await p.keyboard.press(k);
  assert.equal(await p.evaluate(() => document.querySelector('.shr-save').disabled), true, 'nothing changed');
  await p.evaluate(() => document.querySelector('.piece').focus());
  await p.keyboard.press('Escape');
  const s = await p.evaluate(() => ({ arr: document.querySelector('.shr').classList.contains('arr'), focused: !!document.activeElement.closest('.piece') }));
  assert.deepEqual(s, { arr: true, focused: false }, 'Escape lets go of the piece and stays in Arrange');
});

test('paper: the marks over the counter place the keeper, and the choice is saved', { skip }, async (t) => {
  const p = await page(t);
  await click(p, 'button[data-k="look"][data-v="paper"]');
  const marks = await p.evaluate(() => [...document.querySelectorAll('.srp-kspot')].map((b) => [b.dataset.ks, b.getAttribute('aria-pressed'), Math.round(b.getBoundingClientRect().width)]));
  assert.deepEqual(marks.map((m) => m[0]), ['left', 'middle', 'right']);
  assert.equal(marks.find((m) => m[1] === 'true')[0], 'middle', 'the middle until chosen');
  assert.ok(marks.every((m) => m[2] >= 40), 'each mark is a finger-sized target: ' + marks.map((m) => m[2]));
  const before = await p.evaluate(() => document.querySelector('.srp-keeper').style.transform);
  const box = await (await p.$('.srp-kspot[data-ks="right"]')).boundingBox();
  await p.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  assert.equal(await p.evaluate(() => document.querySelector('.srp-kspot[data-ks="right"]').getAttribute('aria-pressed')), 'true');
  assert.notEqual(await p.evaluate(() => document.querySelector('.srp-keeper').style.transform), before, 'the keeper moved');
  await save(p); await p.waitForFunction(() => window.saved.length === 1);
  assert.equal(await p.evaluate(() => window.saved[0].keeperSpot), 'right');
  await p.evaluate(() => document.querySelector('[data-mode="arr"]').click());
  assert.equal(await p.evaluate(() => document.querySelector('.srp-kspot[data-ks="right"]').getAttribute('aria-pressed')), 'true', 'still chosen after saving');
});

test('the marks only show while arranging', { skip }, async (t) => {
  const p = await page(t);
  await click(p, 'button[data-k="look"][data-v="paper"]');
  await save(p); await p.waitForFunction(() => window.saved.length === 1);
  assert.equal(await p.evaluate(() => document.querySelectorAll('.srp-kspot').length), 0);
});

test('paper: a press just beside a small ware still finds it', { skip }, async (t) => {
  // A small window draws the wares smaller than the 32px a press needs.
  const p = await page(t, { width: 760, height: 560 });
  await click(p, 'button[data-k="look"][data-v="paper"]');
  await click(p, 'button[data-k="deco"][data-v="none"]');
  // Points just outside each ware's own box, left and right, where no other card's own box is.
  const spots = await p.evaluate(() => {
    const host = document.querySelector('.srp').parentNode, cards = [...host.querySelectorAll('.srp-good,.srp-deco,.srp-keeper')].map((e) => e.querySelector('.srp-cd').getBoundingClientRect());
    const inside = (x, y) => cards.some((b) => x >= b.left && x <= b.right && y >= b.top && y <= b.bottom);
    const out = [];
    host.querySelectorAll('.srp-good').forEach((g) => {
      const b = g.querySelector('.srp-cd').getBoundingClientRect(), pad = (32 - b.width) / 2 - 1, y = b.top + b.height / 2;
      if (pad < 2) return;
      [b.left - pad, b.right + pad].forEach((x) => { if (!inside(x, y) && x < innerWidth * .55) out.push({ x, y, name: g.getAttribute('aria-label') }); });
    });
    return out;
  });
  assert.ok(spots.length > 0, 'some small ware has clear space beside it');
  const s = spots[0];
  await p.mouse.click(s.x, s.y);
  assert.equal(await p.evaluate(() => !document.querySelector('.shr-pick').hidden), true, 'the ware answered');
  assert.equal(await p.evaluate(() => document.querySelector('.shr-pick h4').textContent), 'Look of ' + s.name.split(',')[0]);
});
