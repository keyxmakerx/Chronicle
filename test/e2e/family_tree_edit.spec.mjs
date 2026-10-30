// family_tree_edit.spec.mjs — Playwright coverage for the Family tree block's
// Director Edit tab: add a parent, add a partner, create a new page, remove a
// link, save, a save that fails, Escape, and a phone width. Players never see
// the tab.
//
// Drives the real family_tree.css, family_tree.js, boot.js (and its htmx) in a
// stand-in page, with the block's markup rendered by the real Templ component
// (committed under test/e2e/fixtures/family_tree/; the Go test
// TestFamilyBlocksMatchBrowserFixtures fails if it drifts from the server's). Only the network is stubbed: the picker's
// candidate list and the save endpoint answer from an in-memory family that
// starts as the very JSON the block carried and applies each save the way the
// server would (removes, then adds, new pages first). Any other request fails
// the test.
//
// RUN: NODE_PATH="$(npm root -g)" PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers \
//        node --test test/e2e/family_tree_edit.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');

const EDIT_URL = '/campaigns/c1/entities/maud/family';
const PICK_URL = EDIT_URL + '/candidates';

// The real blocks, rendered by the real component and kept current by a Go test.
const blocks = { director: read('test/e2e/fixtures/family_tree/director.html'), player: read('test/e2e/fixtures/family_tree/player.html') };

function reducedMotionGuardCSS() {
  const css = read('static/css/input.css');
  const start = css.indexOf('@media (prefers-reduced-motion: reduce) {');
  assert.notEqual(start, -1, 'reduced-motion guard not found in static/css/input.css');
  let depth = 0, end = start;
  for (let i = start; i < css.length; i++) {
    if (css[i] === '{') depth++;
    else if (css[i] === '}') { depth--; if (depth === 0) { end = i + 1; break; } }
  }
  return css.slice(start, end);
}

// A page of prose around the block, so "nothing below shifts" has something to shift.
function harnessHTML(who) {
  return `<!doctype html><html data-motion="off"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>body{margin:0;font-family:system-ui,sans-serif;background:#fff;color:#111} main{max-width:880px;margin:0 auto;padding:16px} ${reducedMotionGuardCSS()}</style></head>
<body><main><h1>Maud</h1><p>Prose before the block.</p>${blocks[who]}<p id="after">Prose after the block.</p></main>
<script src="/static/vendor/htmx.min.js" defer></script>
<script src="/static/js/boot.js" defer></script>
<script src="/entities-js/family_tree.js" defer></script></body></html>`;
}

// ---- the stand-in server -------------------------------------------------

const PAGES = [
  { id: 'harrow', name: 'Harrow Valcrest', sub: 'Founder' },
  { id: 'ysolde', name: 'Ysolde Valcrest', sub: 'Ledger-keeper' },
  { id: 'secret', name: 'Veiled Seer', sub: 'Character', dm: true },
  ...Array.from({ length: 30 }, (_, i) => ({ id: 'x' + i, name: 'Extra page ' + i, sub: 'Place' }))
];

class Server {
  constructor(html) {
    // The block's data rides in its data-json attribute, HTML-escaped.
    const m = /<section class="fm"[^>]*\sdata-json="([^"]*)"/.exec(html);
    const unescape = (s) => s.replace(/&#34;|&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&');
    this.tree = m ? JSON.parse(unescape(m[1])) : null;
    this.requests = [];
    this.saves = [];
    this.failNext = null;
    this.made = 0;
    this.unexpected = [];
  }
  candidates(q) {
    const known = new Set(this.tree.people.map((p) => p.id));
    const hit = PAGES.filter((p) => !known.has(p.id) && (!q || p.name.toLowerCase().includes(q.toLowerCase())));
    return { pages: hit.slice(0, 20), more: hit.length > 20, max: 20 };
  }
  person(id) { return this.tree.people.find((p) => p.id === id); }
  save(body) {
    this.saves.push(body);
    if (this.failNext) {
      const f = this.failNext; this.failNext = null;
      return { status: 500, json: { saved: false, created: [], failed: f, written: [], rolledBack: true, tree: this.tree } };
    }
    const created = [];
    const real = {};
    for (const np of body.newPages) {
      const id = 'made' + (++this.made);
      real[np.ref] = id; created.push({ ref: np.ref, id, name: np.name });
      this.tree.people.push({ id, name: np.name, sub: 'Character', url: '/campaigns/c1/entities/' + id, parents: [], partners: [], read: true });
    }
    const r = (x) => real[x] || x;
    const order = [...body.edits.filter((e) => e.op === 'remove'), ...body.edits.filter((e) => e.op === 'add')];
    for (const e of order) {
      const a = r(e.a), b = r(e.b);
      for (const id of [a, b]) {
        if (!this.person(id)) {
          const pg = PAGES.find((p) => p.id === id);
          this.tree.people.push({ id, name: pg.name, sub: pg.sub, url: '/campaigns/c1/entities/' + id, parents: [], partners: [], read: false, ...(pg.dm ? { dm: true } : {}) });
        }
      }
      if (e.edge === 'parent') {
        const c = this.person(a);
        c.parents = c.parents.filter((p) => p.id !== b);
        if (e.op === 'add') c.parents.push({ id: b, kind: e.link });
      } else {
        const pa = this.person(a), pb = this.person(b);
        pa.partners = pa.partners.filter((x) => x !== b); pb.partners = pb.partners.filter((x) => x !== a);
        if (e.op === 'add') { pa.partners.push(b); pb.partners.push(a); }
      }
    }
    return { status: 200, json: { saved: true, created, failed: [], written: [], rolledBack: false, tree: this.tree } };
  }
}

async function install(page, server, who) {
  await page.route('**/*', async (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname, q = new URL(req.url()).searchParams.get('q') || '';
    if (p === '/page') return route.fulfill({ status: 200, contentType: 'text/html; charset=utf-8', body: harnessHTML(who) });
    if (p.startsWith('/static/js/') || p.startsWith('/static/vendor/') || p.startsWith('/static/css/')) {
      const type = p.endsWith('.css') ? 'text/css' : 'text/javascript';
      return route.fulfill({ status: 200, contentType: type, body: read(p.slice(1)) });
    }
    if (p === '/entities-js/family_tree.js') {
      return route.fulfill({ status: 200, contentType: 'text/javascript', body: read('internal/plugins/entities/static/js/family_tree.js') });
    }
    if (p === PICK_URL && req.method() === 'GET') {
      server.requests.push('GET ' + p + (q ? '?q=' + q : ''));
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(server.candidates(q)) });
    }
    if (p === EDIT_URL && req.method() === 'POST') {
      server.requests.push('POST ' + p);
      const out = server.save(JSON.parse(req.postData()));
      return route.fulfill({ status: out.status, contentType: 'application/json', body: JSON.stringify(out.json) });
    }
    server.unexpected.push(`${req.method()} ${p}`);
    return route.fulfill({ status: 404, body: '' });
  });
}

async function withPage(opts, fn) {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await (await browser.newContext({ viewport: opts.viewport || { width: 1100, height: 900 }, hasTouch: !!opts.touch })).newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    const server = new Server(blocks[opts.who]);
    await install(page, server, opts.who);
    await page.goto('https://chronicle.test/page');
    await page.waitForFunction(() => document.querySelector('.fm-hs-it'));
    await fn(page, server);
    assert.deepEqual(server.unexpected, [], 'no unexpected requests');
    assert.deepEqual(errors, [], 'no uncaught JS exceptions');
  } finally {
    await browser.close();
  }
}

const bar = (page) => page.locator('[data-fm-savemsg]');
const dot = (page, id) => page.locator(`[data-fm-rows] .fm-b[data-id="${id}"]`);
async function openEdit(page) {
  await page.locator('[data-fm-open]').click();
  await page.locator('[data-fm-sheet].fm-show').waitFor();
  await page.locator('[data-fm-edit]').click();
  await page.locator('[data-fm-savebar]:not([hidden])').waitFor();
}
async function pickFrom(page, slot, searchText, choose) {
  await page.locator(`[data-slot="${slot}"]`).click();
  await page.locator('.fm-pick').waitFor();
  if (searchText) await page.locator('.fm-pick input').fill(searchText);
  await page.locator(`.fm-pl button[data-pid="${choose}"]`).waitFor();
  await page.locator(`.fm-pl button[data-pid="${choose}"]`).click();
}

test('players never see the Edit tab, the save bar or an empty place', { timeout: 30000 }, async () => {
  await withPage({ who: 'player' }, async (page, server) => {
    await page.locator('[data-fm-open]').click();
    await page.locator('[data-fm-sheet].fm-show').waitFor();
    assert.equal(await page.locator('[data-fm-edit]').count(), 0);
    assert.equal(await page.locator('[data-fm-savebar]').count(), 0);
    assert.equal(await page.locator('[data-slot]').count(), 0);
    assert.equal(await page.locator('.fm-rm').count(), 0);
    assert.equal(await page.locator('section.fm').getAttribute('data-edit-url'), '');
    assert.deepEqual(server.requests, [], 'a player\'s block never calls the edit routes');
  });
});

test('the Director edits: add a parent and a partner, create a page, remove a link, undo, save', { timeout: 60000 }, async () => {
  await withPage({ who: 'director' }, async (page, server) => {
    const afterBefore = (await page.locator('#after').boundingBox()).y;
    await openEdit(page);
    assert.equal(await bar(page).textContent(), 'No changes yet');
    assert.equal(await page.locator('[data-fm-save]').isDisabled(), true, 'nothing to save yet');
    assert.equal(await page.locator('[data-fm-legend]').isHidden(), true, 'the save bar takes the legend\'s place');
    for (const s of ['parent', 'child', 'partner']) assert.equal(await page.locator(`[data-slot="${s}"]`).count(), 1, `a place to add a ${s}`);
    assert.equal(await page.locator('[data-fm-edit]').textContent(), 'Done');

    // A parent, as an adoptive one: the picker is bounded and says more match.
    await page.locator('[data-slot="parent"]').click();
    await page.locator('.fm-pick').waitFor();
    await page.locator('.fm-pl button').first().waitFor();
    assert.ok(await page.locator('.fm-pl button').count() <= 20, 'the picker never lists more than 20 pages');
    assert.match(await page.locator('.fm-pl').textContent(), /More pages match/);
    assert.equal(server.requests[0], 'GET ' + PICK_URL, 'the first list is asked for with no search text');
    assert.match(await page.locator('.fm-pl button[data-pid="secret"]').textContent(), /DM/, 'a page players cannot open is marked');
    await page.locator('.fm-seg button[data-v="adopted"]').click();
    await page.locator('.fm-pick input').fill('harr');
    await page.waitForFunction(() => document.querySelectorAll('.fm-pl button').length === 1);
    assert.equal(server.requests.at(-1), 'GET ' + PICK_URL + '?q=harr', 'typing narrows the list by asking the server');
    await page.locator('.fm-pl button[data-pid="harrow"]').click();
    await dot(page, 'harrow').waitFor();
    assert.equal(await page.locator('.fm-pick').count(), 0, 'the picker closes on a choice');
    assert.equal(await bar(page).textContent(), '1 change not saved');

    // A partner.
    await pickFrom(page, 'partner', 'yso', 'ysolde');
    await dot(page, 'ysolde').waitFor();
    assert.equal(await bar(page).textContent(), '2 changes not saved');

    // A page that does not exist yet.
    await page.locator('[data-slot="child"]').click();
    await page.locator('.fm-pick').waitFor();
    await page.locator('.fm-pick input').fill('Odo');
    assert.match(await page.locator('.fm-mk').textContent(), /Create a new page .Odo./);
    await page.locator('.fm-mk').click();
    await dot(page, 'new:1').waitFor();
    assert.equal(await bar(page).textContent(), '3 changes not saved');

    // Taking a link off. The mark is a sibling of the circle, shown on hover and focus.
    const tam = page.locator('[data-fm-rows] .fm-dot', { has: page.locator('.fm-b[data-id="tam"]') });
    await dot(page, 'tam').hover();
    await tam.locator('.fm-rm').click();
    assert.equal(await dot(page, 'tam').count(), 0, 'Tam is no longer a child here');
    assert.equal(await bar(page).textContent(), '4 changes not saved');
    await page.locator('[data-fm-undo]').click();
    await dot(page, 'tam').waitFor();
    assert.equal(await bar(page).textContent(), '3 changes not saved', 'undo takes the last change back');
    await dot(page, 'tam').hover();
    await tam.locator('.fm-rm').click();
    assert.equal(await bar(page).textContent(), '4 changes not saved');

    // The resting strip still shows what is saved, and says there is more to save.
    assert.match(await page.locator('[data-fm-cnt]').textContent(), /4 changes not saved/);
    assert.equal(await page.locator('.fm-hs-it[data-id="tam"]').count(), 1, 'the strip keeps the saved family until Save');
    assert.equal(await page.locator('.fm-hs-it[data-id="harrow"]').count(), 0);

    assert.equal((await page.locator('#after').boundingBox()).y, afterBefore, 'opening and editing never moved the page below');

    // Save: one request carrying only the difference.
    await page.locator('[data-fm-save]').click();
    await page.waitForFunction(() => /^Saved/.test(document.querySelector('[data-fm-savemsg]').textContent));
    assert.equal(server.saves.length, 1);
    const sent = server.saves[0];
    assert.deepEqual(sent.newPages, [{ ref: 'new:1', name: 'Odo' }]);
    const sorted = (l) => l.map((e) => JSON.stringify(e)).sort();
    assert.deepEqual(sorted(sent.edits), sorted([
      { op: 'add', edge: 'parent', a: 'maud', b: 'harrow', link: 'adopted' },
      { op: 'add', edge: 'partner', a: 'maud', b: 'ysolde' },
      { op: 'add', edge: 'parent', a: 'new:1', b: 'maud', link: 'birth' },
      { op: 'remove', edge: 'parent', a: 'tam', b: 'maud' }
    ]));
    assert.equal(await page.locator('[data-fm-save]').isDisabled(), true);
    assert.equal(await dot(page, 'made1').count(), 1, 'the made-up page is the real one now');
    assert.equal(await dot(page, 'tam').count(), 0);
    assert.equal(await page.locator('[data-fm-savebar]').evaluate((e) => e.classList.contains('fm-err')), false);

    // Done and close: the strip now shows the new family; nothing below the block moved.
    await page.locator('[data-fm-edit]').click();
    await page.locator('[data-fm-close]').click();
    await page.locator('[data-fm-sheet].fm-show').waitFor({ state: 'hidden' });
    for (const id of ['harrow', 'ysolde', 'made1']) assert.equal(await page.locator(`.fm-hs-it[data-id="${id}"]`).count(), 1, `${id} in the strip`);
    assert.equal(await page.locator('.fm-hs-it[data-id="tam"]').count(), 0);
  });
});

test('a save that does not land says which link, keeps the edits, and tries again', { timeout: 60000 }, async () => {
  await withPage({ who: 'director' }, async (page, server) => {
    await openEdit(page);
    await pickFrom(page, 'partner', 'yso', 'ysolde');
    const tam = page.locator('[data-fm-rows] .fm-dot', { has: page.locator('.fm-b[data-id="tam"]') });
    await dot(page, 'tam').hover();
    await tam.locator('.fm-rm').click();
    assert.equal(await bar(page).textContent(), '2 changes not saved');

    server.failNext = [{ index: 1, reason: 'the link could not be written' }];
    await page.locator('[data-fm-save]').click();
    await page.waitForFunction(() => /^Nothing was saved/.test(document.querySelector('[data-fm-savemsg]').textContent));
    const msg = await bar(page).textContent();
    assert.match(msg, /Could not do: (adding Maud and Ysolde Valcrest as partners|removing Maud as a parent of Tam)/);
    assert.match(msg, /press Save to try again/);
    assert.equal(await page.locator('[data-fm-savebar]').evaluate((e) => e.classList.contains('fm-err')), true);
    assert.equal(await dot(page, 'ysolde').count(), 1, 'the change is still on screen');
    assert.equal(await dot(page, 'tam').count(), 0);
    assert.equal(await page.locator('[data-fm-save]').isDisabled(), false, 'Save is ready to try again');

    await page.locator('[data-fm-save]').click();
    await page.waitForFunction(() => /^Saved/.test(document.querySelector('[data-fm-savemsg]').textContent));
    assert.equal(server.saves.length, 2);
    assert.deepEqual(server.saves[1].edits.length, 2, 'the retry sends the same two changes');
  });
});

test('Escape closes the picker, then the card, then the sheet; unsaved edits wait', { timeout: 60000 }, async () => {
  await withPage({ who: 'director' }, async (page) => {
    await openEdit(page);
    await pickFrom(page, 'partner', 'yso', 'ysolde');

    await page.locator('[data-slot="parent"]').click();
    await page.locator('.fm-pick').waitFor();
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('.fm-pick').count(), 0, 'first Escape closes only the picker');
    assert.equal(await page.locator('[data-fm-sheet].fm-show').count(), 1, 'the sheet is still open');
    assert.equal(await page.evaluate(() => document.activeElement && document.activeElement.getAttribute('data-slot')), 'parent', 'focus goes back to the place that opened it');

    await dot(page, 'mum').click();
    await page.locator('.fm-body.fm-docked').waitFor();
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('.fm-body.fm-docked').count(), 0, 'then the person\'s card');
    await page.keyboard.press('Escape');
    await page.locator('[data-fm-sheet].fm-show').waitFor({ state: 'hidden' });

    assert.match(await page.locator('[data-fm-cnt]').textContent(), /1 change not saved/, 'closing keeps the edit and the strip says so');
    await page.locator('[data-fm-open]').click();
    await page.locator('[data-fm-sheet].fm-show').waitFor();
    assert.equal(await bar(page).textContent(), '1 change not saved');
    assert.equal(await dot(page, 'ysolde').count(), 1);
  });
});

test('a phone: remove marks need no hover, the picker fits, targets are thumb-sized', { timeout: 60000 }, async () => {
  await withPage({ who: 'director', viewport: { width: 390, height: 800 }, touch: true }, async (page) => {
    await openEdit(page);
    const vw = 390;
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'no horizontal page scroll');

    const rm = page.locator('.fm-rm').first();
    assert.equal(await rm.evaluate((e) => getComputedStyle(e).opacity), '1', 'remove marks are always shown on touch');
    const slot = (await page.locator('[data-slot="parent"]').boundingBox());
    assert.ok(slot.width >= 44 && slot.height >= 44, `a place is ${slot.width}x${slot.height}, wants at least 44`);
    for (const sel of ['[data-fm-edit]', '[data-fm-save]', '[data-fm-undo]']) {
      const b = await page.locator(sel).boundingBox();
      assert.ok(b.height >= 44, `${sel} is ${b.height}px tall`);
    }

    await page.locator('[data-slot="parent"]').tap();
    await page.locator('.fm-pick').waitFor();
    const pk = await page.locator('.fm-pick').boundingBox();
    assert.ok(pk.x >= 0 && pk.x + pk.width <= vw, `the picker (${pk.x}..${pk.x + pk.width}) fits a ${vw}px phone`);
    assert.equal(await page.locator('.fm-pick input').evaluate((e) => parseFloat(getComputedStyle(e).fontSize) >= 16), true, 'no zoom-on-focus');
    const seg = await page.locator('.fm-seg button').first().boundingBox();
    assert.ok(seg.height >= 40, `the link-type buttons are ${seg.height}px`);
    await page.locator('.fm-pl button[data-pid="harrow"]').tap();
    await dot(page, 'harrow').waitFor();
    assert.equal(await bar(page).textContent(), '1 change not saved');
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'still no horizontal page scroll');

    await page.locator('[data-fm-save]').tap();
    await page.waitForFunction(() => /^Saved/.test(document.querySelector('[data-fm-savemsg]').textContent));
  });
});
