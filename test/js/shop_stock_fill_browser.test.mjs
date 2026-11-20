// shop_stock_fill_browser.test.mjs: the Fill-the-shelves sheet in a real
// Chromium with a stubbed API. Nothing is saved until Keep; pin, leave out
// and hand edits survive Roll again; the Keep body carries what the Director
// left; a good with no price is flagged and kept as zero for them to set.
// Needs playwright-core and a Chromium (see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');
const GEN = read('static/js/widgets/shop_stock_gen.js');
const FILL = read('static/js/widgets/shop_stock_fill.js');
const EVERYDAY = read('static/shop-goods/everyday.json');

const SOURCES = {
  foodPrice: 2, listed: [],
  items: [{ id: 'it1', name: 'Heirloom lamp', price: 30, currency: 'gp' }],
  system: [{ slug: 'ax', name: 'Odd axe', kind: 'Trinket', rarity: 1, price: 0, hasPrice: false }],
  systemName: 'Test System', rarityLabel: 'echelon',
};

const STUB = `
  window.calls = []; window.dirty = []; window.kept = null; window.sheetClosed = null;
  window.Chronicle = {
    notify: (m, t) => { (window.notes = window.notes || []).push({ m, t }); },
    markDirty: (id) => window.dirty.push('dirty:' + id),
    markClean: (id) => window.dirty.push('clean:' + id),
    apiFetch: (url, opts) => {
      window.calls.push({ url, method: (opts && opts.method) || 'GET', body: opts && opts.body });
      let body = {};
      if (url.endsWith('/fill-sources')) body = ${JSON.stringify(SOURCES)};
      else if (url.endsWith('/stock')) body = { batch: 'b1', relations: [11, 12], skipped: 0 };
      else if (url.endsWith('/food-price')) body = { foodPrice: opts.body.foodPrice };
      else if (url.endsWith('/room')) body = { roomType: 'forge' };
      else if (url.endsWith('/roll-tables')) body = { tables: [] };
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    },
  };
`;

async function openSheet(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (r) => {
    const u = r.request().url();
    if (u.endsWith('/static/shop-goods/everyday.json')) return r.fulfill({ contentType: 'application/json', body: EVERYDAY });
    return r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body><div id="host"></div></body></html>' });
  });
  await p.goto('http://chronicle.test/campaigns/c1/entities/shop1');
  await p.addScriptTag({ content: STUB });
  await p.addScriptTag({ content: GEN });
  await p.addScriptTag({ content: FILL });
  await p.evaluate(() => {
    window.sheet = window.Chronicle.shopFill.open(document.getElementById('host'), {
      campaignUrl: '/campaigns/c1', shopId: 'shop1', words: {}, spots: 2,
      onKept: (r) => { window.kept = r; }, onClose: (k) => { window.sheetClosed = k; },
    });
  });
  await p.waitForSelector('.sf-keep');
  return p;
}
const rows = (p) => p.$$eval('.sf-g', (n) => n.map((r) => r.getAttribute('data-good')));
const stockPosts = (p) => p.evaluate(() => window.calls.filter((c) => c.method === 'POST'));

test('the draft is shown and nothing is saved until Keep', { skip }, async (t) => {
  const p = await openSheet(t);
  assert.ok((await rows(p)).length >= 1);
  assert.match(await p.textContent('.sf-say'), /goods? for a forge/i, 'the shop kind comes from its room');
  assert.deepEqual(await stockPosts(p), []);
  await p.click('.sf-roll');
  assert.deepEqual(await stockPosts(p), [], 'rolling again saves nothing');
});

test('a pinned good survives Roll again; a left-out good never returns', { skip }, async (t) => {
  const p = await openSheet(t);
  const [first, second] = await rows(p);
  await p.click('[data-good="' + first + '"] .sf-pin');
  await p.click('[data-good="' + second + '"] .sf-out');
  for (let i = 0; i < 4; i++) {
    await p.click('.sf-roll');
    const now = await rows(p);
    assert.ok(now.includes(first), 'pinned good lost on roll ' + i);
    assert.ok(!now.includes(second), 'left-out good came back on roll ' + i);
  }
});

test('Keep posts what the Director left, and says what it kept', { skip }, async (t) => {
  const p = await openSheet(t);
  const key = (await rows(p))[0];
  await p.fill('[data-good="' + key + '"] .sf-name', 'Renamed good');
  await p.dispatchEvent('[data-good="' + key + '"] .sf-name', 'change');
  const price = '[data-good="' + key + '"] .sf-price';
  await p.fill(price, '7.5');
  await p.dispatchEvent(price, 'change');
  await p.click('.sf-keep');
  await p.waitForFunction(() => window.kept);
  const [post] = await stockPosts(p);
  assert.ok(post.url.endsWith('/armory/shops/shop1/stock'));
  const edited = post.body.goods.find((g) => g.name === 'Renamed good');
  assert.ok(edited, 'the typed name is sent');
  assert.equal(edited.price, 7.5);
  assert.ok(post.body.goods.every((g) => typeof g.price === 'number' && g.currency));
  assert.equal(await p.evaluate(() => window.kept.batch), 'b1');
  assert.equal(await p.evaluate(() => window.sheetClosed), true);
  assert.deepEqual(await p.evaluate(() => window.dirty.filter((d) => d.startsWith('clean')).length), 1);
});

test('a system good with no price is flagged and sent as zero, never guessed', { skip }, async (t) => {
  const p = await openSheet(t);
  // Draft everything so the unpriced system good is certainly in it.
  await p.fill('.sf-num', '80');
  await p.dispatchEvent('.sf-num', 'change');
  await p.click('.sf-more');
  const key = await p.$eval('[data-good*="Odd axe" i], [data-good*="odd axe" i]', (n) => n.getAttribute('data-good'));
  assert.equal(await p.$eval('[data-good="' + key + '"] .sf-price', (i) => i.value), '');
  assert.match(await p.textContent('.sf-hint'), /no price yet/);
  await p.click('.sf-keep');
  await p.waitForFunction(() => window.kept);
  const [post] = await stockPosts(p);
  const axe = post.body.goods.find((g) => g.name === 'Odd axe');
  assert.equal(axe.price, 0);
  assert.equal(axe.today, true, 'a find goes on today’s shelf');
  const lamp = post.body.goods.find((g) => g.name === 'Heirloom lamp');
  assert.equal(lamp.price, 30, 'a hand-set price is never rescaled');
  assert.equal(lamp.itemId, 'it1');
});

test('changing a day’s food saves it once and reprices the everyday goods', { skip }, async (t) => {
  const p = await openSheet(t);
  await p.fill('.sf-num', '80');
  await p.dispatchEvent('.sf-num', 'change');
  await p.click('.sf-more');
  const priceOf = () => p.$$eval('.sf-g', (rs) => rs.map((r) => [r.getAttribute('data-good'), r.querySelector('.sf-price').value]));
  const before = Object.fromEntries(await priceOf());
  await p.fill('.sf-foodin', '4');
  await p.dispatchEvent('.sf-foodin', 'change');
  await p.waitForFunction(() => window.calls.some((c) => c.method === 'PUT'));
  const put = await p.evaluate(() => window.calls.find((c) => c.method === 'PUT'));
  assert.deepEqual(put.body, { foodPrice: 4 });
  const after = Object.fromEntries(await priceOf());
  const ration = Object.keys(before).find((k) => /trail rations, one day/i.test(k));
  if (ration) assert.equal(Number(after[ration]), Number(before[ration]) * 2, 'food 2 -> 4 doubles an everyday price');
  const lamp = Object.keys(after).find((k) => /heirloom/i.test(k));
  assert.equal(after[lamp], before[lamp], 'a hand-set price does not move');
});

test('Escape closes the sheet and Discard saves nothing', { skip }, async (t) => {
  const p = await openSheet(t);
  await p.keyboard.press('Escape');
  await p.waitForFunction(() => window.sheetClosed === false);
  assert.equal(await p.$('.shop-draft'), null);
  assert.deepEqual(await stockPosts(p), []);
});
