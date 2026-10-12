// rulebook.spec.mjs — Playwright coverage for the Rules page reader: the book
// fits the content area, a turn to a wide page is a page turn, and switching
// between Player and Director view keeps the reader's place on the page.
//
// Drives the real static/js/widgets/rulebook.js and static/css/rulebook.css
// in a stand-in for the app layout: a header, a banner, and a scrolling
// <main id="main-content"> holding the same fullbleed wrapper
// system_pages.templ renders. Only Chronicle's widget runtime is stubbed, and
// the book JSON is served from a fixture shaped like GET .../book.
//
// RUN: NODE_PATH="$(npm root -g)" PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers \
//        node --test test/e2e/rulebook.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const asset = (p) => readFileSync(join(repoRoot, p), 'utf8');

const BOOK_URL = '/campaigns/camp-1/systems/demo/book';

// A long page, so the page itself scrolls inside the book.
const longPage = (title) => ({
  title,
  blocks: [{ type: 'text', text: Array.from({ length: 40 }, (_, i) => `Paragraph ${i + 1} of ${title}.`).join('\n\n') }],
});

function bookFixture(opts = {}) {
  if (opts.screen) return screenFixture();
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: 'paper',
    tabs: ['basics', 'screen', 'combat'], isDirector: true, start: 'basics',
    terms: {},
    parts: [{
      title: "Player's book",
      chapters: [
        { id: 'basics', title: 'Basics', pages: [longPage('Start here'), longPage('Rolling')] },
        { id: 'screen', title: 'Players Screen', pages: [{ title: 'At the table', wide: true, blocks: [{ type: 'text', text: 'Everything at once.' }] }] },
        { id: 'tests', title: 'Tests', pages: [longPage('Rolling a test'), longPage('Easy and hard')] },
        { id: 'combat', title: 'Combat', pages: [longPage('Turns'), longPage('Moving')] },
      ],
    }],
  };
}

// A book that names a screen chapter: three quick-reference pages that drop
// over the book instead of being pages of it, a long card list among them.
function screenFixture() {
  const conditions = Array.from({ length: 9 }, (_, i) => ({ title: `Condition ${i + 1}`, text: `What condition ${i + 1} does.` }));
  conditions[4].title = 'Grabbed';
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: 'paper',
    tabs: ['screen', 'basics', 'combat'], isDirector: true, start: 'screen', screen: 'screen',
    terms: {},
    parts: [{
      title: "Player's book",
      chapters: [
        { id: 'basics', title: 'Basics', pages: [longPage('Start here'), { title: 'Where next', blocks: [{ type: 'links', items: [{ title: 'Players Screen', chapter: 'screen' }] }] }] },
        { id: 'screen', title: 'Players Screen', pages: [
          { title: 'Your turn', blocks: [{ type: 'cards', items: [{ title: 'Main action', text: 'The heaviest part of your turn.' }] }] },
          { title: 'The power roll', blocks: [{ type: 'text', text: 'Roll two ten-sided dice.' }] },
          { title: 'Conditions', blocks: [{ type: 'cards', items: conditions }] },
        ] },
        { id: 'combat', title: 'Combat', pages: [longPage('Turns'), longPage('Moving')] },
      ],
    }],
  };
}

// The app layout around the Rules page, sized as base.templ and app.templ
// size it: the header and a banner take part of the screen, <main> scrolls.
function harnessHTML() {
  return `<!doctype html><html><head><meta charset="utf-8">
<style>
  html, body { margin: 0; height: 100%; }
  body { display: flex; flex-direction: column; }
  .hdr { flex: 0 0 auto; height: 56px; }
  .banner { flex: 0 0 auto; height: 49px; }
  #main-content { flex: 1 1 0; min-height: 0; overflow-y: auto; padding: 16px 20px; }
  [data-rulebook-fullbleed] { margin: -16px -20px; min-height: 100%; }
</style>
<link rel="stylesheet" href="/static/css/rulebook.css">
</head><body>
<div class="hdr"></div><div class="banner"></div>
<main id="main-content"><div id="module-content"><div data-rulebook-fullbleed>
  <div data-widget="rulebook" data-campaign-id="camp-1" data-book-url="${BOOK_URL}"></div>
</div></div></main>
<script>
  window.Chronicle = {
    escapeHtml: function (s) { return String(s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); },
    apiFetch: function (u) { return fetch(u); },
    register: function (slug, w) {
      document.querySelectorAll('[data-widget="' + slug + '"]').forEach(function (el) { w.init(el, Object.assign({}, el.dataset)); });
    }
  };
</script>
<script src="/static/js/widgets/rulebook.js"></script>
</body></html>`;
}

async function withBook(fn, opts = {}) {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.route('**/*', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/rules') return route.fulfill({ status: 200, contentType: 'text/html', body: harnessHTML() });
      if (p === '/static/css/rulebook.css') return route.fulfill({ status: 200, contentType: 'text/css', body: asset('static/css/rulebook.css') });
      if (p === '/static/js/widgets/rulebook.js') return route.fulfill({ status: 200, contentType: 'text/javascript', body: asset('static/js/widgets/rulebook.js') });
      if (p === BOOK_URL) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(bookFixture(opts)) });
      return route.fulfill({ status: 404, body: '' });
    });
    // Customize > Rulebook "Chronicle's look" reaches the page as this attribute.
    if (opts.standard) await page.addInitScript(() => document.addEventListener('DOMContentLoaded', () => document.documentElement.setAttribute('data-cz-book', 'standard')));
    await page.goto('https://chronicle.test/rules' + (opts.hash || ''));
    await page.waitForSelector('.rb-tabs');
    await fn(page);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
}

const mainOverflow = (page) => page.evaluate(() => {
  const m = document.getElementById('main-content');
  return m.scrollHeight - m.clientHeight;
});

test('the book fills the content area, so it never scrolls the page under it', async () => {
  await withBook(async (page) => {
    assert.equal(await mainOverflow(page), 0);
    const sizes = await page.evaluate(() => [document.querySelector('.rb').offsetHeight, document.getElementById('main-content').clientHeight]);
    assert.equal(sizes[0], sizes[1]);
    // Opening the contents moves focus into the drawer; nothing may scroll.
    await page.click('[data-act="toc"]');
    await page.waitForTimeout(400);
    assert.equal(await page.evaluate(() => document.getElementById('main-content').scrollTop), 0);
  });
});

test('a turn to and from a wide page turns a leaf instead of sliding', async () => {
  await withBook(async (page) => {
    for (const [tab, dir] of [['screen', 'fwd'], ['basics', 'back']]) {
      await page.click(`[data-tab="${tab}"]`);
      const leaf = await page.evaluate(() => {
        const l = document.querySelector('.rb-leaf');
        return l && { cls: l.className, cover: !!document.querySelector('.rb-cover'), halves: l.querySelectorAll('.rb-half').length };
      });
      assert.ok(leaf, 'a leaf is turning');
      assert.match(leaf.cls, /\bflip\b/);
      assert.doesNotMatch(leaf.cls, /\bslide\b/);
      assert.match(leaf.cls, new RegExp(`\\b${dir}\\b`));
      assert.equal(leaf.halves, 2);
      assert.ok(leaf.cover, 'the half not yet turned still shows the old spread');
      await page.waitForFunction(() => !document.querySelector('.rb-leaf') && !document.querySelector('.rb-cover'));
    }
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where').textContent), 'Basics');
  });
});

test('switching between Player and Director view keeps the place on the page', async () => {
  await withBook(async (page) => {
    await page.click('[data-tab="combat"]');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    const tops = () => page.evaluate(() => [...document.querySelectorAll('.rb-book > .rb-page .rb-scroll')].map((s) => s.scrollTop));
    await page.evaluate(() => document.querySelectorAll('.rb-book > .rb-page .rb-scroll').forEach((s) => { s.scrollTop = 150; }));
    const before = await tops();
    assert.ok(before.some((t) => t > 0), 'a page scrolled');
    await page.click('[data-view="player"]');
    assert.deepEqual(await tops(), before);
    assert.equal(await mainOverflow(page), 0);
  });
});

test("in Chronicle's look a tab still opens its own chapter", async () => {
  await withBook(async (page) => {
    // Tests has an opener and two pages, so without a fresh spread per
    // chapter Combat's opener would share a spread with the last Tests page.
    for (const [tab, title] of [['combat', 'Combat'], ['basics', 'Basics'], ['combat', 'Combat']]) {
      await page.click(`[data-tab="${tab}"]`);
      await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
      assert.equal(await page.evaluate(() => document.querySelector('.rb-where b').textContent), title);
    }
  }, { standard: true });
});

const screenState = (page) => page.evaluate(() => {
  const rb = document.querySelector('.rb'), sc = document.querySelector('.rb-screen'), rib = document.querySelector('.rb-ribbon');
  return {
    open: rb.classList.contains('rb-screen-open'), hidden: sc.hidden, leaves: sc.querySelectorAll('.rb-sleaf').length,
    expanded: rib.getAttribute('aria-expanded'), focusIn: sc.contains(document.activeElement), ribbonFocused: document.activeElement === rib,
  };
});

test('a book that starts on its screen opens the screen over the first page, never as a page', async () => {
  await withBook(async (page) => {
    let st = await screenState(page);
    assert.equal(st.open, true);
    assert.equal(st.leaves, 3);
    assert.equal(st.expanded, 'true');
    const tabs = await page.$$eval('.rb-tab', (ts) => ts.map((t) => t.textContent));
    assert.deepEqual(tabs, ['Basics', 'Combat']);
    // The screen's pages are not pages of the book.
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where b').textContent), 'Basics');
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.querySelector('.rb-screen').hidden);
    st = await screenState(page);
    assert.equal(st.open, false);
    assert.equal(st.expanded, 'false');
  }, { screen: true });
});

test('the ribbon, the P key and a link all drop the screen, and Escape returns to the ribbon', async () => {
  await withBook(async (page) => {
    // A chapter in the address wins over a start on the screen.
    assert.equal((await screenState(page)).open, false);
    await page.click('.rb-ribbon');
    let st = await screenState(page);
    assert.equal(st.open, true);
    await page.waitForFunction(() => document.querySelector('.rb-screen').contains(document.activeElement));
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.querySelector('.rb-screen').hidden);
    assert.equal((await screenState(page)).ribbonFocused, true);
    await page.keyboard.press('p');
    assert.equal((await screenState(page)).open, true);
    await page.keyboard.press('p');
    await page.waitForFunction(() => document.querySelector('.rb-screen').hidden);
    // A link to the screen chapter opens the screen.
    await page.click('[data-act="next"]');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    await page.click('.rb-link[data-chapter="screen"]');
    assert.equal((await screenState(page)).open, true);
    // A long card list gets a find box.
    await page.fill('.rb-screen [data-cfind]', 'grab');
    const shown = await page.$$eval('.rb-screen .rb-sleaf:last-child .rb-fc', (cs) => cs.filter((c) => !c.hidden).map((c) => c.textContent));
    assert.deepEqual(shown, ['Grabbed']);
    // A click on the book behind folds it away.
    const book = await page.$eval('.rb-book', (b) => { const r = b.getBoundingClientRect(); return { x: r.left + 20, y: r.bottom - 6 }; });
    await page.mouse.click(book.x, book.y);
    await page.waitForFunction(() => document.querySelector('.rb-screen').hidden);
  }, { screen: true, hash: '#basics' });
});
