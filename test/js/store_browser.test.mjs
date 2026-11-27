// store_browser.test.mjs — the Store sheet (static/js/widgets/store.js) in a
// real Chromium, against an in-page fake of the design gallery's routes: it
// opens from the top bar as a modal sheet, browses by kind and game system,
// opens a thing and adds it, reports one, escapes typed text, fits a phone and
// closes back to the button. Skips locally without playwright-core (see
// _browser_env.mjs); CI must run it.

import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const JS = readFileSync(join(root, 'static/js/widgets/store.js'), 'utf8');
const CSS = readFileSync(join(root, 'static/css/store.css'), 'utf8');

const card = (id, kind, name, extra) => Object.assign({ id, kind, systemId: 'drawsteel', name, note: '', makerName: 'Ines', summary: '', publishedAt: '2026-11-20T10:00:00Z', mine: false, sameSystem: true, lacks: [], reported: false }, extra || {});
const ROOMS = [card('g1', 'room-theme', 'The Gilded Kettle', { note: 'A warm forge counter with copper pots.' }), card('g2', 'room-theme', 'Salt Market Stall'), card('g3', 'room-theme', '<img src=x onerror="window.__pwned=1">')];
const CRAFT = [card('g9', 'crafting-circle', 'Moonlit Circle', { sameSystem: false, systemId: 'dnd5e', lacks: ['This campaign has no silver dust.'] })];

const PAGE = `<!doctype html><html data-motion="full"><head><style>[hidden]{display:none}body{margin:0;font-family:system-ui,sans-serif}
:root{--color-bg-primary:#fff;--color-bg-secondary:#f9fafb;--color-bg-tertiary:#f3f4f6;--color-card-bg:#fff;--color-text-primary:#111827;--color-text-body:#374151;--color-text-muted:#6b7280;--color-border:#e5e7eb;--color-accent:#6366f1;--color-warn:#b45309;--color-danger:#b91c1c;--surface-overlay:rgb(6 7 11/.55);--dur-standard:200ms;--dur-micro:120ms;--ease-out:ease-out;--ease-in:ease-in;--shape-edge:1px;--shape-rule:1px;--shape-r-panel:12px;--shape-r-ctl:8px;--shape-r-small:6px;--shape-r-tag:4px;--elev-resting:none;--elev-hover:none;--elev-dragged:none}
.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}${CSS}</style></head>
<body><header style="height:56px"><div id="w" data-widget="store" data-campaign="c1" data-csrf="t" data-css=""><button type="button" data-store-toggle aria-expanded="false">Store</button></div></header><main style="height:1200px">Page</main></body></html>`;

const FAKE = `
window.__srv = { calls: [], rooms: ${JSON.stringify(ROOMS)}, craft: ${JSON.stringify(CRAFT)}, notes: [] };
function done(v) { return new Promise(function (r) { setTimeout(function () { r(JSON.parse(JSON.stringify(v))); }, 5); }); }
window.Chronicle = {
  register: function (n, impl) { if (n === 'store') impl.init(document.getElementById('w')); },
  escapeHtml: function (t) { return String(t == null ? '' : t).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  working: function (b, on) { if (on) b.setAttribute('data-working', ''); else b.removeAttribute('data-working'); },
  notify: function (m) { window.__srv.notes.push(m); },
  apiAction: function (url, o) {
    var s = window.__srv, m = (o && o.method) || 'GET', p = url.replace('/campaigns/c1', '').split('?')[0], q = url.split('?')[1] || '';
    s.calls.push(m + ' ' + p + (q ? '?' + q : '') + (o && o.body ? ' ' + JSON.stringify(o.body) : ''));
    if (m === 'GET' && p === '/designs/gallery') {
      var list = /kind=crafting/.test(q) ? s.craft : /kind=rooms/.test(q) ? s.rooms : s.rooms.concat(s.craft);
      var term = (q.match(/q=([^&]*)/) || [])[1];
      if (term) { term = decodeURIComponent(term).toLowerCase(); list = list.filter(function (c) { return c.name.toLowerCase().indexOf(term) >= 0; }); }
      return done({ entries: list, total: list.length, page: 1, perPage: 12, systemId: 'drawsteel' });
    }
    if (m === 'POST' && /\\/copy$/.test(p)) return done({ id: 'd1' });
    if (m === 'POST' && /\\/report$/.test(p)) return done({});
    return Promise.reject(new Error('unexpected ' + m + ' ' + p));
  }
};`;

let browser;
before(async () => { if (!skip) browser = await pw.chromium.launch({ executablePath: chromePath }); });
after(async () => { if (browser) await browser.close(); });

async function openPage(opts) {
  const page = await browser.newPage(Object.assign({ viewport: { width: 1280, height: 860 } }, opts || {}));
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.setContent(PAGE);
  await page.addScriptTag({ content: FAKE });
  await page.addScriptTag({ content: JS });
  page.errors = errors;
  return page;
}
const calls = (page) => page.evaluate(() => window.__srv.calls.slice());

test('the button opens a modal sheet on For you, with each kind and its count', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.waitForSelector('.st-dlg.st-in');
  assert.equal(await page.evaluate(() => document.querySelector('.st-dlg').open && document.querySelector('.st-dlg').matches(':modal')), true);
  assert.equal(await page.getAttribute('[data-store-toggle]', 'aria-expanded'), 'true');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'st-q');
  await page.waitForSelector('#st-h-crafting');
  const heads = await page.$$eval('.st-h', (h) => h.map((x) => x.textContent));
  assert.deepEqual(heads, ['New shop rooms for Draw Steel', 'New crafting looks for Draw Steel']);
  const rail = await page.$$eval('.st-rail .st-k', (b) => b.map((x) => x.textContent.replace(/\s+/g, ' ').trim()));
  assert.deepEqual(rail, ['For you', 'Shop rooms, 3', 'Crafting looks, 1', 'Draw Steel', 'Any game system']);
  assert.deepEqual((await calls(page)).sort(), ['GET /designs/gallery?page=1&kind=crafting', 'GET /designs/gallery?page=1&kind=rooms']);
  assert.deepEqual(page.errors, []);
  await page.close();
});

test('typed names are shown as text, never as markup', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.waitForSelector('[data-id="g3"]');
  assert.equal(await page.$('.st-main img'), null);
  assert.equal(await page.evaluate(() => window.__pwned), undefined);
  assert.match(await page.textContent('[data-id="g3"] .st-cn'), /^<img src=x/);
  await page.close();
});

test('a thing opens in place and Add copies it once, then says where it went', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.click('[data-id="g1"]');
  await page.waitForSelector('#st-item-h');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'st-item-h');
  assert.match(await page.textContent('.st-item'), /Shop room · made for Draw Steel · by Ines/);
  await page.click('[data-st="add"]');
  await page.waitForSelector('.st-item .st-ok');
  assert.match(await page.textContent('.st-item .st-ok'), /Added to your designs\. Find it in your designs when you arrange a shop room\./);
  assert.equal(await page.$('[data-st="add"]'), null);
  assert.ok((await calls(page)).includes('POST /designs/gallery/g1/copy'));
  assert.deepEqual(await page.evaluate(() => window.__srv.notes), ['“The Gilded Kettle” was added to your designs.']);
  await page.click('[data-st="back"]');
  await page.waitForSelector('[data-id="g1"] .st-ok');
  await page.close();
});

test('another game system and what the campaign lacks are said on the thing', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.click('[data-id="g9"]');
  const know = await page.$$eval('.st-know', (n) => n.map((x) => x.textContent));
  assert.equal(know.length, 2);
  assert.match(know[0], /^Made for D&D 5e\. This campaign uses Draw Steel\./);
  await page.close();
});

test('a kind, any game system and a search each ask the gallery the right way', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.click('.st-rail [data-v="rooms"]');
  await page.waitForSelector('.st-count');
  assert.match(await page.textContent('.st-count'), /3 things for Draw Steel/);
  await page.click('.st-rail [data-v="all"]');
  await page.waitForFunction(() => window.__srv.calls.some((c) => c === 'GET /designs/gallery?page=1&system=all&kind=rooms'));
  await page.fill('#st-q', 'kettle');
  await page.waitForFunction(() => window.__srv.calls.some((c) => /q=kettle/.test(c)));
  const last = (await calls(page)).pop();
  assert.equal(last, 'GET /designs/gallery?page=1&system=all&q=kettle');
  await page.waitForSelector('.st-sh .st-h');
  assert.equal(await page.textContent('.st-sh .st-h'), 'Results for “kettle”');
  await page.close();
});

test('a report sends one of the three reasons with its note', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.click('[data-id="g2"]');
  await page.click('[data-st="report-open"]');
  await page.check('input[name="st-reason"][value="spam"]');
  await page.fill('#st-rnote', 'Links to a shop');
  await page.click('[data-st="report-send"]');
  await page.waitForSelector('text=You reported this');
  assert.ok((await calls(page)).includes('POST /designs/gallery/g2/report {"reason":"spam","note":"Links to a shop"}'));
  await page.close();
});

test('Escape rolls the sheet up and gives focus back to the button', { skip }, async () => {
  const page = await openPage();
  await page.click('[data-store-toggle]');
  await page.waitForSelector('.st-dlg.st-in');
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => !document.querySelector('.st-dlg').open);
  assert.equal(await page.getAttribute('[data-store-toggle]', 'aria-expanded'), 'false');
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-store-toggle')), true);
  assert.equal(await page.evaluate(() => document.documentElement.classList.contains('st-open')), false);
  await page.close();
});

test('on a phone the sheet fits the window and its choices are 44 tall', { skip }, async () => {
  const page = await openPage({ viewport: { width: 390, height: 800 } });
  await page.click('[data-store-toggle]');
  await page.waitForSelector('#st-h-crafting');
  const fit = await page.evaluate(() => {
    const s = document.querySelector('.st-sheet').getBoundingClientRect(), main = document.querySelector('.st-main');
    return { left: Math.round(s.left), width: Math.round(s.width), over: main.scrollWidth > main.clientWidth };
  });
  assert.deepEqual(fit, { left: 0, width: 390, over: false });
  const heights = await page.$$eval('.st-rail .st-k', (b) => b.map((x) => x.getBoundingClientRect().height));
  assert.ok(heights.every((h) => h >= 44), JSON.stringify(heights));
  await page.close();
});

test('under Off the sheet shows at once, with no move', { skip }, async () => {
  const page = await openPage();
  await page.evaluate(() => document.documentElement.setAttribute('data-motion', 'off'));
  await page.click('[data-store-toggle]');
  assert.equal(await page.getAttribute('.st-dlg', 'data-motion'), 'off');
  const t = await page.evaluate(() => getComputedStyle(document.querySelector('.st-sheet')).transitionDuration);
  assert.match(t, /^0s/);
  await page.close();
});

test("a person's own Calm in My view keeps the sheet from travelling", { skip }, async () => {
  const page = await openPage();
  await page.evaluate(() => document.documentElement.setAttribute('data-view-motion', 'calm'));
  await page.click('[data-store-toggle]');
  assert.equal(await page.getAttribute('.st-dlg', 'data-motion'), 'calm');
  await page.waitForSelector('.st-dlg.st-in');
  const t = await page.evaluate(() => getComputedStyle(document.querySelector('.st-sheet')).transform);
  assert.match(t, /^matrix\(1, 0, 0, 1, [-\d.]+, 0\)$/);
  await page.close();
});

test('the first open waits for the stylesheet, so the sheet never shows unstyled', { skip }, async () => {
  const page = await openPage();
  let release;
  const held = new Promise((r) => { release = r; });
  await page.route('https://store.test/store.css', async (route) => { await held; await route.fulfill({ contentType: 'text/css', body: '.st-dlg{--st-loaded:1}' }); });
  await page.evaluate(() => {
    document.getElementById('w').setAttribute('data-css', 'https://store.test/store.css');
    document.querySelectorAll('link[data-store-css]').forEach((l) => l.remove());
  });
  await page.click('[data-store-toggle]');
  await page.waitForTimeout(150);
  assert.equal(await page.evaluate(() => !!document.querySelector('.st-dlg[open]')), false);
  assert.equal(await page.getAttribute('[data-store-toggle]', 'data-working'), '');
  await page.click('[data-store-toggle]');
  release();
  await page.waitForSelector('.st-dlg.st-in');
  assert.equal(await page.evaluate(() => document.querySelectorAll('.st-dlg').length), 1);
  assert.equal(await page.getAttribute('[data-store-toggle]', 'data-working'), null);
  await page.close();
});

test('a stylesheet that fails opens nothing unstyled, says so, and the next press tries again', { skip }, async () => {
  const page = await openPage();
  let fail = true;
  await page.route('https://store.test/store.css', (route) => (fail ? route.abort() : route.fulfill({ contentType: 'text/css', body: '.st-dlg{--st-loaded:1}' })));
  await page.evaluate(() => document.getElementById('w').setAttribute('data-css', 'https://store.test/store.css'));
  await page.click('[data-store-toggle]');
  await page.waitForFunction(() => window.__srv.notes.length === 1);
  assert.equal(await page.evaluate(() => !!document.querySelector('.st-dlg[open]')), false);
  assert.match(await page.evaluate(() => window.__srv.notes[0]), /^The Store could not load\./);
  assert.equal(await page.evaluate(() => document.querySelectorAll('link[data-store-css]').length), 0);
  assert.equal(await page.getAttribute('[data-store-toggle]', 'data-working'), null);
  fail = false;
  await page.click('[data-store-toggle]');
  await page.waitForSelector('.st-dlg.st-in');
  await page.close();
});
