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

// A page whose links block turns to chapters several spreads away.
const jumpPage = (title) => ({
  title,
  blocks: [{ type: 'text', text: 'Read on.' }, { type: 'links', items: [{ title: 'Running the game', chapter: 'running' }, { title: 'Combat', chapter: 'combat' }] }],
});

// A light paper with a crimson accent, the shape of a real bold book's theme.
const BOLD_THEME = { paper: '#f1f2f4', 'paper-alt': '#ffffff', ink: '#15171d', 'ink-soft': '#4b5261', muted: '#6b7280', edge: '#cbd1da', accent: '#c22f39' };
const COVER_URL = '/campaigns/camp-1/systems/demo/book/files/cover.svg';
const FONT_URL = '/campaigns/camp-1/systems/demo/book/files/display.woff2';
const COVER_SVG = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 516"><rect width="400" height="516" fill="#3d0a1c"/><circle cx="200" cy="250" r="90" fill="#ffd27a"/></svg>';

const rollPage = {
  title: 'The power roll',
  blocks: [{ type: 'roll', dice: { count: 2, sides: 10 }, label: 'Power roll', modifier: true, bands: [
    { max: 11, label: 'Tier 1', text: 'A weak result.' }, { max: 16, label: 'Tier 2', text: 'A solid result.' }, { max: null, label: 'Tier 3', text: 'A strong result.' }] },
    { type: 'callout', title: 'Remember', text: 'Edges stack.' }],
};

function bookFixture(opts = {}) {
  if (opts.index) return indexFixture();
  if (opts.house) return houseFixture(opts);
  if (opts.screen) return screenFixture(opts);
  if (opts.contents) {
    const book = bookFixture(Object.assign({}, opts, { contents: false }));
    book.start = 'front';
    book.parts[0].chapters.unshift({ id: 'front', title: 'Handbook', pages: [{ title: 'Contents', wide: true, blocks: [{ type: 'contents' }] }] });
    return book;
  }
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: opts.bold ? 'bold' : 'paper',
    theme: opts.theme || (opts.bold ? BOLD_THEME : {}),
    cover: opts.cover ? COVER_URL : undefined,
    fonts: opts.cover ? [{ family: 'Big Shoulders Display', url: FONT_URL, weight: 900, style: 'normal' }] : undefined,
    tabs: opts.noTabs ? [] : ['basics', 'screen', 'combat'], isDirector: true, start: 'basics',
    terms: {},
    parts: [{
      title: "Player's book",
      chapters: [
        { id: 'basics', title: 'Basics', pages: [opts.jumps ? jumpPage('Start here') : longPage('Start here'), rollPage, longPage('Rolling')] },
        { id: 'screen', title: 'Players Screen', pages: [{ title: 'At the table', wide: true, blocks: [{ type: 'text', text: 'Everything at once.' }] }] },
        { id: 'tests', title: 'Tests', pages: [longPage('Rolling a test'), longPage('Easy and hard')] },
        { id: 'combat', title: 'Combat', pages: [longPage('Turns'), opts.popUp ? { title: 'Pop-up fight', blocks: [{ type: 'widget', widget: 'class-fights' }] } : longPage('Moving')] },
      ],
    }, {
      title: "Director's book", director: true,
      chapters: [{ id: 'running', title: 'Running the game', director: true, pages: [longPage('Pacing'), longPage('Rewards')] }],
    }],
  };
}

// A rules index with choices, and a written page naming one of its entries
// as an entry link and a rule word, shaped like GET .../book and the index
// and entry answers the server sends.
const IX_LIST = { filters: [{ key: 'level', label: 'Level' }, { key: 'keywords', label: 'Keywords' }], sections: 'level', sectionLabel: 'Level', tiers: ['tier1_display'] };
const IX_ITEMS = [
  { id: 'b-strike', name: 'Blade Strike', summary: 'A cut.', fields: [{ label: 'Level', value: '1', key: 'level' }, { label: 'Tier 1 (11 or lower)', value: '2 damage', key: 'tier1_display' }, { label: 'Effect', value: 'The target is [[slowed|slowed down]].', key: 'effect_display' }],
    facets: [{ key: 'level', values: ['1'] }, { key: 'keywords', values: ['Melee', 'Strike'] }] },
  { id: 'c-cleave', name: 'Cleave', summary: 'Two foes.', facets: [{ key: 'level', values: ['2'] }, { key: 'keywords', values: ['Melee'] }] },
  { id: 'd-dash', name: 'Dash', summary: 'Go fast.', facets: [{ key: 'level', values: ['2'] }, { key: 'keywords', values: ['Move'] }] },
];
function indexFixture() {
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: 'bold', theme: BOLD_THEME, isDirector: true, start: 'spells', tabs: [],
    terms: { slowed: { name: 'Slowed', text: 'Your speed is 2.' } },
    parts: [{ title: "Player's book", chapters: [
      { id: 'spells', title: 'Fighting', pages: [{ title: 'Moves', blocks: [{ type: 'text', text: 'A fury opens with [[@abilities/b-strike|Blade Strike]] and leaves a foe [[slowed]].\n\nA dead name: [[@abilities/nope|Nobody]].' }] }, longPage('More')] },
      { id: 'all-abilities', title: 'All abilities', generated: true, pages: [{ title: 'Fury', blocks: [{ type: 'index', category: 'abilities', key: 'class', value: 'Fury', count: 3, list: IX_LIST }] }] },
    ] }],
  };
}

// A book that names a screen chapter: three quick-reference pages that drop
// over the book instead of being pages of it, a long card list among them.
function screenFixture(opts = {}) {
  const conditions = Array.from({ length: 9 }, (_, i) => ({ title: `Condition ${i + 1}`, text: `What condition ${i + 1} does.` }));
  conditions[4].title = 'Grabbed';
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: 'paper',
    tabs: opts.noTabs ? [] : ['screen', 'basics', 'combat'], isDirector: true, start: 'screen', screen: 'screen',
    terms: {},
    parts: [{
      title: "Player's book",
      chapters: [
        { id: 'basics', title: 'Basics', pages: [longPage('Start here'), { title: 'Where next', blocks: [{ type: 'links', items: [{ title: 'Players Screen', chapter: 'screen' }] }] }] },
        { id: 'screen', title: 'Players Screen', pages: [
          { title: 'Your turn', blocks: [{ type: 'cards', items: [{ title: 'Main action', text: 'The heaviest part of your turn.' }] }] },
          opts.longScreen ? longPage('The power roll') : { title: 'The power roll', blocks: [{ type: 'text', text: 'Roll two ten-sided dice.' }] },
          { title: 'Conditions', blocks: [{ type: 'cards', items: conditions }] },
        ] },
        { id: 'combat', title: 'Combat', pages: [longPage('Turns'), longPage('Moving')] },
      ],
    }],
  };
}

// A book with two house-rules chapters, for the leaves on the campaign home.
// The first chapter's second page is Director-only; a player's book (the server
// has already dropped it) still carries the flag in the fixture so the reader
// is shown to hide it by itself.
function houseFixture(opts = {}) {
  const first = {
    title: 'First rule',
    blocks: [
      { type: 'text', text: 'A soul lingers for **three days**. A [[pledge]] holds it longer.' },
      { type: 'callout', title: 'Remember', text: 'Edges stack, and a very long unbroken word like Supercalifragilisticexpialidocious_Supercalifragilistic stays inside.' },
      { type: 'cards', items: [1, 2, 3, 4].map((i) => ({ title: `Remnant ${i}`, summary: `About ${i} days.`, text: `What remnant ${i} does.` })) },
      { type: 'roll', dice: { count: 2, sides: 10 }, label: 'Power roll', modifier: true, bands: [
        { max: 11, label: 'Tier 1', text: 'A weak result.' }, { max: 16, label: 'Tier 2', text: 'A solid result.' }, { max: null, label: 'Tier 3', text: 'A strong result.' }] },
    ],
  };
  return {
    title: 'Handbook', systemName: 'Demo', turn: 'flip', look: opts.bold ? 'bold' : 'paper',
    theme: opts.theme || (opts.bold ? BOLD_THEME : {}),
    isDirector: !opts.player, start: 'basics', tabs: [],
    terms: { pledge: { name: 'Pledge', text: 'A promise sworn on a soul.' } },
    parts: [
      { title: "Player's book", chapters: [{ id: 'basics', title: 'Basics', pages: [longPage('Start here')] }] },
      { title: 'House rules', chapters: [
        { id: 'house_one', title: 'Souls, Death and Remnants', intro: 'What becomes of a soul.', pages: [
          first,
          { title: 'Second page', blocks: [{ type: 'text', text: 'The second page.' }] },
          { title: 'DIR ONLY page', director: true, blocks: [{ type: 'text', text: 'Never shown to a player.' }] },
          { title: 'Third page', blocks: [{ type: 'text', text: 'The third page.' }] },
        ] },
        { id: 'house_two', title: 'Potions', intro: 'Drinking one is a maneuver.', pages: [{ title: 'Brewing', blocks: [{ type: 'text', text: 'Brew slowly.' }] }] },
      ] },
    ],
  };
}

const HOUSE_HREF = '/campaigns/camp-1/systems/demo#house_one';

// The campaign home's House rules block: each rule is a closed <details> with
// the introduction and a leaf.
function leafHarnessHTML() {
  const leaf = (id) => `<div data-widget="rulebook" data-campaign-id="camp-1" data-book-url="${BOOK_URL}" data-leaf="${id}" data-open-href="/campaigns/camp-1/systems/demo#${id}" data-leaf-preview="true"></div>`;
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  html, body { margin: 0; background: #fff; color: #222; font: 14px system-ui, sans-serif; }
  main { padding: 16px; max-width: 760px; }
  .rows { border: 1px solid #ddd; border-radius: 12px; padding: 0 16px; }
  summary { min-height: 32px; cursor: pointer; font-weight: 600; display: flex; align-items: center; }
</style>
<link rel="stylesheet" href="/static/css/rulebook.css">
</head><body><main><div class="rows">
<details id="d1"><summary>Souls, Death and Remnants</summary><div class="cb-rule-body"><p>What becomes of a soul.</p>${leaf('house_one')}</div></details>
<details id="d2"><summary>Potions</summary><div class="cb-rule-body"><p>Drinking one is a maneuver.</p>${leaf('house_two')}</div></details>
</div><p id="after">After the block.</p></main>
<script>
  window.Chronicle = {
    escapeHtml: function (s) { return String(s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); },
    apiFetch: function (u) { return fetch(u); },
    register: function (slug, w) {
      document.querySelectorAll('[data-widget="' + slug + '"]').forEach(function (el) { w.init(el, Object.assign({}, el.dataset)); });
    }
  };
</script>
<script src="/static/js/hovercard.js"></script>
<script src="/static/js/widgets/rulebook.js"></script>
</body></html>`;
}

// The app layout around the Rules page, sized as base.templ and app.templ
// size it: the header and a banner take part of the screen, <main> scrolls.
function harnessHTML(opts = {}) {
  if (opts.house) return leafHarnessHTML();
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
${opts.nested ? '<main id="main-content">' : ''}<main id="main-content"><div id="module-content"><div data-rulebook-fullbleed>
  <div data-widget="rulebook" data-campaign-id="camp-1" data-book-url="${BOOK_URL}"></div>
</div></div></main>${opts.nested ? '</main>' : ''}
<script>
  window.Chronicle = {
    escapeHtml: function (s) { return String(s).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); },
    apiFetch: function (u) { return fetch(u); },
    register: function (slug, w) {
      document.querySelectorAll('[data-widget="' + slug + '"]').forEach(function (el) { w.init(el, Object.assign({}, el.dataset)); });
    }
  };
  ${opts.backCloses ? `window.__bc = []; window.__bcClose = {};
  window.Chronicle.backCloses = function (name, close) {
    window.__bc.push('hold ' + name); window.__bcClose[name] = close;
    return function () { window.__bc.push('release ' + name); };
  };` : ''}
</script>
${opts.index ? '<script src="/static/js/hovercard.js"></script>' : ''}
<script src="/static/js/widgets/rulebook.js"></script>
</body></html>`;
}

async function withBook(fn, opts = {}) {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage(Object.assign({ viewport: opts.viewport || { width: 1280, height: 720 }, reducedMotion: opts.reducedMotion ? 'reduce' : 'no-preference' }, opts.touch ? { hasTouch: true, isMobile: true } : {}));
    const fontRequests = [];
    page.fontRequests = fontRequests;
    page.bookRequests = 0;
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.addInitScript(() => {
      const Real = window.FontFace;
      window.__faceFamilies = [];
      window.FontFace = function (family, src, desc) { window.__faceFamilies.push(family); return new Real(family, src, desc); };
    });
    await page.route('**/*', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/rules') return route.fulfill({ status: 200, contentType: 'text/html', body: harnessHTML(opts) });
      if (p === '/static/css/rulebook.css') return route.fulfill({ status: 200, contentType: 'text/css', body: asset('static/css/rulebook.css') });
      if (p === '/static/js/widgets/rulebook.js') return route.fulfill({ status: 200, contentType: 'text/javascript', body: asset('static/js/widgets/rulebook.js') });
      if (p === '/static/js/hovercard.js') return route.fulfill({ status: 200, contentType: 'text/javascript', body: asset('static/js/hovercard.js') });
      if (p === BOOK_URL) page.bookRequests++;
      if (p === BOOK_URL && opts.failFirst && page.bookRequests === 1) return route.fulfill({ status: 500, body: '' });
      if (p === BOOK_URL) return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(bookFixture(opts)) });
      if (p === BOOK_URL + '/index/abilities') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: IX_ITEMS }) });
      if (p === BOOK_URL + '/entry/abilities/b-strike') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ entry: IX_ITEMS[0], chapter: 'all-abilities', page: 0, pageTitle: 'Fury', list: IX_LIST }) });
      if (p === COVER_URL) return route.fulfill({ status: 200, contentType: 'image/svg+xml', body: COVER_SVG });
      if (p === FONT_URL) { fontRequests.push(p); return route.fulfill({ status: 200, contentType: 'font/woff2', body: 'wOF2' }); }
      return route.fulfill({ status: 404, body: '' });
    });
    // The attribute an older Customize setting wrote; the reader must ignore it.
    if (opts.standard) await page.addInitScript(() => document.addEventListener('DOMContentLoaded', () => document.documentElement.setAttribute('data-cz-book', 'standard')));
    await page.goto('https://chronicle.test/rules' + (opts.hash || ''), opts.referer ? { referer: opts.referer } : {});
    if (!opts.house) await page.waitForSelector('.rb-book');
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

test('after a sidebar link nests the page in a second <main>, the book still fills the content area', async () => {
  for (const viewport of [{ width: 1280, height: 720 }, { width: 412, height: 915 }]) {
    await withBook(async (page) => {
      const m = await page.evaluate(() => {
        const outer = document.querySelector('body > #main-content');
        const rb = document.querySelector('.rb').getBoundingClientRect();
        const o = outer.getBoundingClientRect();
        return { nested: !!outer.querySelector(':scope > #main-content'), rb: [rb.left, rb.width, Math.round(rb.height)], outer: [o.left, o.width, outer.clientHeight], overflow: outer.scrollHeight - outer.clientHeight };
      });
      assert.equal(m.nested, true);
      assert.deepEqual(m.rb, m.outer, `${viewport.width}px: the book spans the outer <main> edge to edge`);
      assert.equal(m.overflow, 0);
    }, { nested: true, viewport });
  }
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

test('a tab opens its own chapter', async () => {
  await withBook(async (page) => {
    // Tests has an opener and two pages, so without a fresh spread per
    // chapter Combat's opener would share a spread with the last Tests page.
    for (const [tab, title] of [['combat', 'Combat'], ['basics', 'Basics'], ['combat', 'Combat']]) {
      await page.click(`[data-tab="${tab}"]`);
      await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
      assert.equal(await page.evaluate(() => document.querySelector('.rb-where b').textContent), title);
    }
  });
});

test('a contents block numbers every other chapter with its page and turns to the one picked', async () => {
  await withBook(async (page) => {
    const rows = await page.$$eval('.rb-contents li', (ls) => ls.map((l) => [...l.querySelectorAll('button > span')].map((s) => s.textContent.trim())));
    assert.deepEqual(rows.map((r) => r[0]), rows.map((_, i) => String(i + 1)));
    assert.ok(!rows.some((r) => r[1] === 'Handbook'), 'the chapter holding the list is not in it');
    assert.ok(rows.some((r) => r[1].startsWith('Running the game')), 'a Director sees Director chapters');
    const combat = rows.find((r) => r[1] === 'Combat');
    await page.click('.rb-contents [data-chapter="combat"]');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where b').textContent), 'Combat');
    const folios = await page.$$eval('.rb-page:not([hidden]) .rb-folio', (fs) => fs.map((f) => 'p. ' + f.textContent));
    assert.ok(folios.includes(combat[2]), `Combat starts on ${combat[2]}, the spread shows ${folios}`);
  }, { contents: true });
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

// --- the cover, the bold look and the Sections bookmark -----------------------

const frontState = (page) => page.evaluate(() => {
  const rb = document.querySelector('.rb'), f = document.querySelector('.rb-front');
  return {
    closed: rb.classList.contains('rb-closed'), shown: !!f && !f.hidden, open: !!f && f.classList.contains('open'),
    pageVisible: getComputedStyle(document.querySelector('.rb-book .rb-page.r')).visibility,
    focusOnFace: document.activeElement === document.querySelector('.rb-front-face'),
    turn: [...document.querySelectorAll('.rb-turnbar button')].map((b) => b.textContent + (b.disabled ? ':off' : '')).join('|'),
    where: (document.querySelector('.rb-turn-where') || {}).textContent,
  };
});

test('a book with a cover opens shut on it every visit, opens on Enter and shuts again from the turn bar', async () => {
  await withBook(async (page) => {
    let st = await frontState(page);
    assert.deepEqual([st.closed, st.shown, st.open, st.pageVisible, st.focusOnFace], [true, true, false, 'hidden', false]);
    // The bold look turns pages from a bar under the book; shut, only Next works.
    assert.deepEqual([st.turn, st.where], ['\u2039 Previous:off|Close the book:off|Next \u203A', 'The cover']);
    assert.equal(await page.evaluate(() => !!document.querySelector('.rb-bar [data-act="shut"]')), false);
    const face = await page.$eval('.rb-front-face', (b) => ({ label: b.getAttribute('aria-label'), alt: b.querySelector('img').alt, src: b.querySelector('img').getAttribute('src') }));
    assert.deepEqual(face, { label: 'Open the Handbook', alt: 'Handbook: the cover', src: '/campaigns/camp-1/systems/demo/book/files/cover.svg' });
    // The cover is fetched after the book draws, so wait for it to load
    // rather than read its size the moment it is added.
    await page.waitForFunction(() => { const img = document.querySelector('.rb-front-face img'); return img.complete && img.naturalWidth > 0; }, null, { timeout: 5000 });
    // The book's fonts are asked for from the package route.
    assert.deepEqual(page.fontRequests, [FONT_URL]);
    // The face is registered under the bare family name the theme asks for;
    // quotes would become part of the name and no heading would match it.
    assert.deepEqual(await page.evaluate(() => window.__faceFamilies), ['Big Shoulders Display']);
    // Nothing behind the cover answers the keyboard.
    await page.keyboard.press('ArrowRight');
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where').textContent), 'Basics');
    assert.equal(await page.evaluate(() => document.querySelector('.rb-front-face').getBoundingClientRect().width > 100), true);

    // With the rest of the book hidden, Tab lands straight on the cover.
    await page.keyboard.press('Tab');
    assert.equal((await frontState(page)).focusOnFace, true);
    await page.keyboard.press('Enter');
    await page.waitForFunction(() => document.querySelector('.rb-front').hidden);
    st = await frontState(page);
    assert.deepEqual([st.closed, st.shown, st.pageVisible], [false, false, 'visible']);
    assert.equal(st.turn, '\u2039 Previous:off|Close the book|Next \u203A');
    assert.equal(st.where, 'Basics \u00B7 Start here');
    assert.equal(await page.evaluate(() => document.activeElement.closest('.rb-scroll') !== null), true);

    await page.click('.rb-turnbar [data-act="shut"]');
    await page.waitForFunction(() => document.querySelector('.rb').classList.contains('rb-closed'));
    st = await frontState(page);
    assert.deepEqual([st.shown, st.open, st.focusOnFace, st.where], [true, false, true, 'The cover']);
    // Next opens a shut book.
    await page.click('.rb-turnbar [data-act="next"]');
    await page.waitForFunction(() => document.querySelector('.rb-front').hidden);
    assert.equal((await frontState(page)).closed, false);

    // Coming back to the page finds the book shut again.
    await page.reload();
    await page.waitForSelector('.rb-book');
    st = await frontState(page);
    assert.deepEqual([st.closed, st.shown], [true, true]);
  }, { cover: true, bold: true });
});

test('the inside of the cover carries the left-hand page and lands on it as the book opens', async () => {
  await withBook(async (page) => {
    await page.click('.rb-front-face');
    // Mid-swing: the cover's inside holds a copy of the left page, with no
    // ids of its own, while the real one waits hidden under it.
    const mid = await page.evaluate(() => {
      const back = document.querySelector('.rb-front-back'), copy = back.querySelector('.rb-page');
      return {
        copy: !!copy && copy.textContent === document.querySelector('.rb-book > .rb-page.l').textContent,
        ids: back.querySelectorAll('[id]').length, inert: copy && copy.hasAttribute('inert'),
        left: getComputedStyle(document.querySelector('.rb-book > .rb-page.l')).visibility,
        right: getComputedStyle(document.querySelector('.rb-book > .rb-page.r')).visibility,
      };
    });
    assert.deepEqual(mid, { copy: true, ids: 0, inert: true, left: 'hidden', right: 'visible' });
    await page.waitForFunction(() => document.querySelector('.rb-front').hidden);
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('.rb-book > .rb-page.l')).visibility), 'visible');
    // Open, the cover is the right-hand page's size, so its inside lies exactly on the left one.
    const fit = await page.evaluate(() => {
      const front = document.querySelector('.rb-front');
      front.hidden = false;
      front.querySelectorAll('*').forEach((n) => { n.style.transition = 'none'; });
      const f = document.querySelector('.rb-front-book').getBoundingClientRect(), r = document.querySelector('.rb-book > .rb-page.r').getBoundingClientRect();
      return [f.left, f.top, f.width, f.height].map((v, i) => Math.abs(v - [r.left, r.top, r.width, r.height][i]) < 1.5);
    });
    assert.deepEqual(fit, [true, true, true, true]);
  }, { cover: true, bold: true });
});

test('the shut cover keeps its shape in any window, so its art is never cropped', async () => {
  for (const viewport of [{ width: 1280, height: 720 }, { width: 1300, height: 1400 }, { width: 2560, height: 1440 }, { width: 412, height: 915 }]) {
    await withBook(async (page) => {
      const r = await page.$eval('.rb-front-face', (f) => { const b = f.getBoundingClientRect(), d = f.closest('.rb-front').getBoundingClientRect(); return { ratio: b.width / b.height, inside: b.top >= d.top && b.bottom <= d.bottom && b.left >= d.left && b.right <= d.right }; });
      assert.ok(Math.abs(r.ratio - 400 / 516) < 0.01, `${viewport.width}x${viewport.height}: ${r.ratio}`);
      assert.ok(r.inside, `${viewport.width}x${viewport.height}: the cover fits its desk`);
    }, { cover: true, bold: true, viewport });
  }
});

test('a link naming a chapter skips the cover, and a book without one has none', async () => {
  await withBook(async (page) => {
    const st = await frontState(page);
    assert.deepEqual([st.closed, st.shown], [false, false]);
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where b').textContent), 'Combat');
  }, { cover: true, hash: '#combat' });
  await withBook(async (page) => {
    assert.equal(await page.evaluate(() => [!!document.querySelector('.rb-front'), !!document.querySelector('[data-act="shut"]')].join()), 'false,false');
    assert.equal(await page.evaluate(() => document.querySelector('.rb').classList.contains('rb-closed')), false);
  });
});

test('with reduced motion the cover is a still that a click dismisses at once', async () => {
  await withBook(async (page) => {
    const t = await page.$eval('.rb-front-board', (b) => getComputedStyle(b).transitionDuration);
    assert.equal(t, '0s');
    await page.click('.rb-front-face');
    await page.waitForFunction(() => document.querySelector('.rb-front').hidden, null, { timeout: 500 });
    assert.equal((await frontState(page)).closed, false);
  }, { cover: true, reducedMotion: true });
});

test('the bold reader has one way around the book: no contents drawer, tab row or second "where" line', async () => {
  await withBook(async (page) => {
    const nav = await page.evaluate(() => ({
      toc: !!document.querySelector('[data-act="toc"]'),
      tabs: !!document.querySelector('.rb-tabs'),
      where: !document.querySelector('.rb-bar .rb-where').hidden,
      ribbon: !!document.querySelector('.rb-bm-tab'),
      turnWhere: document.querySelector('.rb-turn-where').textContent,
    }));
    assert.deepEqual([nav.toc, nav.tabs, nav.where, nav.ribbon], [false, false, false, true]);
    assert.ok(nav.turnWhere.length > 0, 'the bar under the book says where you are');
  }, { bold: true });
  // The paper look keeps its contents drawer and tab row.
  await withBook(async (page) => {
    assert.equal(await page.evaluate(() => !!document.querySelector('[data-act="toc"]')), true);
  });
});

test('look: bold draws dark banners, heavy headings and coloured tiers, whatever the site settings', async () => {
  const probe = (page) => page.evaluate(() => {
    const rb = document.querySelector('.rb');
    const kick = document.querySelector('.rb-page.l .rb-scroll > .rb-kicker');
    const h1 = document.querySelector('.rb-page.l h1, .rb-page.l h2');
    const lum = (c) => { const m = c.match(/[\d.]+/g).map(Number), k = c.startsWith('color(') ? 1 : 255; return (0.2126 * m[0] + 0.7152 * m[1] + 0.0722 * m[2]) / k; };
    return {
      look: rb.getAttribute('data-look'),
      bannerBg: getComputedStyle(kick).backgroundColor,
      bannerLum: lum(getComputedStyle(kick).backgroundColor),
      weight: getComputedStyle(h1).fontWeight, transform: getComputedStyle(h1).textTransform,
      tier: document.querySelector('.rb-tier') ? getComputedStyle(document.querySelector('.rb-tier')).backgroundImage.includes('linear-gradient') : null,
    };
  });
  await withBook(async (page) => {
    let p = await probe(page);
    assert.equal(p.look, 'bold');
    assert.ok(p.bannerLum < 0.15, `banner is dark (${p.bannerLum})`);
    assert.deepEqual([p.weight, p.transform], ['900', 'uppercase']);
    await page.click('[data-act="next"]');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    const tiers = await page.$$eval('.rb-page.l .rb-tier', (ts) => ts.map((t) => getComputedStyle(t).backgroundImage.includes('linear-gradient')));
    assert.deepEqual(tiers, [true, true, true]);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  }, { bold: true });
  // A game system's book keeps its own look whatever the site's settings say.
  await withBook(async (page) => {
    const p = await probe(page);
    assert.equal(p.look, 'bold');
    assert.ok(p.bannerLum < 0.15, `banner is dark (${p.bannerLum})`);
  }, { bold: true, standard: true });
});

test('on a phone the bar is one row; the view switch folds under More, which closes on a pick or Escape', async () => {
  await withBook(async (page) => {
    const st = () => page.evaluate(() => {
      const bar = document.querySelector('.rb-bar').getBoundingClientRect(), more = document.querySelector('.rb-more');
      return { tall: Math.round(bar.height), expanded: more.getAttribute('aria-expanded'), seg: document.querySelector('.rb-seg').checkVisibility() ? 'shown' : 'hidden', wide: document.documentElement.scrollWidth <= innerWidth };
    });
    let s = await st();
    assert.ok(s.tall < 60, `one row: ${s.tall}`);
    assert.deepEqual([s.expanded, s.seg, s.wide], ['false', 'hidden', true]);
    await page.click('.rb-more');
    s = await st();
    assert.deepEqual([s.expanded, s.seg], ['true', 'shown']);
    await page.click('.rb-tools [data-view="player"]');
    s = await st();
    assert.deepEqual([s.expanded, s.seg], ['false', 'hidden']);
    assert.equal(await page.evaluate(() => document.querySelector('[data-view="player"]').getAttribute('aria-pressed')), 'true');
    await page.click('.rb-more');
    await page.keyboard.press('Escape');
    assert.equal((await st()).expanded, 'false');
    assert.equal(await page.evaluate(() => document.activeElement.classList.contains('rb-more')), true);
  }, { bold: true, viewport: { width: 390, height: 844 }, hash: '#basics' });
  // On a wide screen everything stays in the bar and there is no More.
  await withBook(async (page) => {
    assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('.rb-more')).display), 'none');
    assert.equal(await page.evaluate(() => document.querySelector('.rb-seg').checkVisibility()), true);
  }, { bold: true, hash: '#basics' });
});

test('Next under the book is never covered by Chronicle\'s Jot notes tab', async () => {
  for (const viewport of [{ width: 1280, height: 720 }, { width: 390, height: 844 }]) {
    await withBook(async (page) => {
      // The same fixed tab the app layout mounts when Jot notes is on.
      await page.evaluate(() => {
        const t = document.createElement('button');
        t.className = 'jot-tab';
        t.textContent = 'Jot notes';
        const phone = innerWidth < 640;
        t.style.cssText = `position:fixed;right:${phone ? 16 : 24}px;bottom:${phone ? 16 : 24}px;z-index:50;padding:10px 14px;font:700 12.5px system-ui`;
        document.body.appendChild(t);
      });
      const r = await page.evaluate(() => {
        const a = document.querySelector('.rb-turnbar [data-act="next"]').getBoundingClientRect(), b = document.querySelector('.jot-tab').getBoundingClientRect();
        return { apart: a.bottom <= b.top || a.right <= b.left, next: a.bottom, tab: b.top };
      });
      assert.ok(r.apart, JSON.stringify(r));
    }, { bold: true, viewport, hash: '#basics' });
  }
});

// Draw Steel's front page before 2.3.1 put these rules in a global
// stylesheet; once it had rendered they crushed the reader's tiers and hid
// its flaps.
const OLD_WIDGET_CSS = '.rb-tiers{display:flex;gap:8px;margin-top:12px}' +
  '.rb-flap{position:absolute;top:calc(100% - 3px);left:6px;right:6px;z-index:35;padding:12px 14px;opacity:0;visibility:hidden;pointer-events:none}' +
  '.rb-grid{display:grid;grid-template-columns:1.55fr 1fr;grid-template-areas:"hero chars" "conds lair";gap:14px}' +
  '.rb-brand .rb-mark{width:32px;height:32px;background:linear-gradient(135deg,#7c3aed,#6d28d9)}';

test('an older package widget\'s stylesheet cannot restyle the reader\'s tiers', async () => {
  await withBook(async (page) => {
    await page.click('[data-act="next"]');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    const probe = () => page.evaluate(() => {
      const tiers = document.querySelector('.rb-page .rb-tiers'), mark = document.querySelector('.rb-mark') || tiers;
      return { display: getComputedStyle(tiers).display, margin: getComputedStyle(tiers).marginTop, widths: [...tiers.children].map((t) => Math.round(t.getBoundingClientRect().width)), mark: getComputedStyle(mark).width };
    });
    const before = await probe();
    await page.addStyleTag({ content: OLD_WIDGET_CSS });
    assert.deepEqual(await probe(), before);
    assert.equal(before.display, 'grid');
  }, { bold: true });
});

// The words on the open page, in the tab row, and over the book's top edge.
const wordsUnderRibbon = (page) => page.evaluate(() => {
  const bm = document.querySelector('.rb-bm-roll').getBoundingClientRect();
  const hits = [];
  for (const root of document.querySelectorAll('.rb-book, .rb-tabs')) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if (!n.nodeValue.trim() || n.parentElement.closest('.rb-bm, [hidden]') || getComputedStyle(n.parentElement).visibility === 'hidden') continue;
      const r = document.createRange();
      r.selectNodeContents(n);
      // Text cut short with an ellipsis is drawn only inside its box.
      const box = getComputedStyle(n.parentElement).overflow === 'hidden' ? n.parentElement.getBoundingClientRect() : null;
      for (const q0 of r.getClientRects()) {
        const q = box ? { left: Math.max(q0.left, box.left), right: Math.min(q0.right, box.right), top: q0.top, bottom: q0.bottom } : q0;
        if (q.right > q.left && q.bottom > q.top && q.left < bm.right && q.right > bm.left && q.top < bm.bottom && q.bottom > bm.top) hits.push(n.nodeValue.trim().slice(0, 30));
      }
    }
  }
  const book = document.querySelector('.rb-book').getBoundingClientRect();
  return { hits, rightPageMiddle: book.left + 0.75 * book.width, rect: { left: Math.round(bm.left), top: Math.round(bm.top), width: Math.round(bm.width), height: Math.round(bm.height) } };
});

for (const [name, viewport] of [['desktop', { width: 1280, height: 720 }], ['phone', { width: 390, height: 844 }]]) {
  for (const look of ['bold', 'paper']) {
    test(`the closed Sections ribbon never covers a word (${name}, ${look})`, async () => {
      await withBook(async (page) => {
        // A long chapter title runs the whole width of the page header.
        await page.evaluate(() => document.querySelector('.rb').scrollTop);
        for (let i = 0; i < 4; i++) {
          const w = await wordsUnderRibbon(page);
          assert.deepEqual(w.hits, [], `spread ${i}: ${JSON.stringify(w.rect)}`);
          if (name === 'desktop') assert.ok(Math.abs(w.rect.left + w.rect.width / 2 - w.rightPageMiddle) < 20, `centred over the right page: ${JSON.stringify(w)}`);
          assert.equal(w.rect.height, 46);
          const next = await page.$('[data-act="next"]:not([hidden])');
          if (!next) break;
          await next.click();
          await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
        }
      }, { viewport, bold: look === 'bold' });
    });
  }
}

test('on a phone the Sections ribbon sits right of the screen ribbon, never over it', async () => {
  await withBook(async (page) => {
    const measure = () => page.evaluate(() => {
      const a = document.querySelector('.rb-ribbon').getBoundingClientRect(), b = document.querySelector('.rb-bm-roll').getBoundingClientRect();
      return { a: [a.left, a.right, a.top, a.bottom], b: [b.left, b.right, b.top, b.bottom] };
    });
    const check = (r) => {
      const apart = r.a[1] <= r.b[0] || r.b[1] <= r.a[0] || r.a[3] <= r.b[2] || r.b[3] <= r.a[2];
      assert.ok(apart, JSON.stringify(r));
      assert.ok(r.b[1] <= 390 && r.b[0] >= 195, `right side: ${JSON.stringify(r)}`);
    };
    check(await measure());
    // A long screen title, or wider fonts, is cut short rather than run under the bookmark.
    await page.$eval('.rb-ribbon-t', (t) => { t.textContent = 'The players screen of quick rules for the table'; });
    check(await measure());
  }, { screen: true, noTabs: true, viewport: { width: 390, height: 844 }, hash: '#basics' });
});

test('on a phone the screen stacks its leaves at full height and scrolls, never cropping one under the next', async () => {
  await withBook(async (page) => {
    await page.click('.rb-ribbon');
    await page.waitForFunction(() => document.querySelector('.rb').classList.contains('rb-screen-open'));
    const leaves = await page.$$eval('.rb-screen .rb-sleaf', (ls) => ls.map((l) => ({ top: l.offsetTop, h: l.offsetHeight, body: l.querySelector('.rb-scroll').offsetHeight })));
    assert.equal(leaves.length, 3);
    leaves.forEach((l, i) => {
      assert.ok(l.h >= l.body, `leaf ${i} shows all of its body: ${JSON.stringify(l)}`);
      if (i) assert.ok(l.top + 1 >= leaves[i - 1].top + leaves[i - 1].h, `leaf ${i} starts below the one before: ${JSON.stringify(leaves)}`);
    });
    const sc = await page.$eval('.rb-screen', (s) => ({ sh: s.scrollHeight, ch: s.clientHeight }));
    assert.ok(sc.sh > sc.ch, `the screen scrolls: ${JSON.stringify(sc)}`);
  }, { screen: true, longScreen: true, noTabs: true, bold: true, viewport: { width: 390, height: 844 }, hash: '#basics' });
});

test('the editor\'s preview draws a contents page from the chapters it is given, without page numbers', async () => {
  await withBook(async (page) => {
    const draw = (ctx) => page.evaluate((c) => {
      const div = document.createElement('div');
      div.innerHTML = window.ChronicleRulebook.renderPage({ title: 'Contents', blocks: [{ type: 'contents' }] }, c);
      return [...div.querySelectorAll('.rb-toc li')].map((li) => li.textContent);
    }, ctx);
    const parts = [
      { title: "Player's book", chapters: [{ id: 'intro', title: 'Contents' }, { id: 'basics', title: 'Basics' }] },
      { title: "Director's book", director: true, chapters: [{ id: 'running', title: 'Running the game' }] },
    ];
    assert.deepEqual(await draw({}), []);
    assert.deepEqual(await draw({ parts, chapterId: 'intro' }), ['1Basics', '2Running the game']);
    assert.deepEqual(await draw({ parts, chapterId: 'intro', director: false }), ['1Basics']);
  }, { hash: '#basics' });
});

const bmState = (page) => page.evaluate(() => {
  const tab = document.querySelector('.rb-bm-tab'), roll = document.querySelector('.rb-bm-roll');
  return { expanded: tab.getAttribute('aria-expanded'), controls: tab.getAttribute('aria-controls'), height: Math.round(roll.getBoundingClientRect().height), tabFocused: document.activeElement === tab };
});

test('the Sections ribbon pulls down, lists chapters and pages, and rolls up before it turns', async () => {
  await withBook(async (page) => {
    assert.equal(await page.$eval('.rb-bm-tab span', (s) => s.textContent), 'Sections');
    await page.click('.rb-bm-tab');
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height > 200);
    let st = await bmState(page);
    assert.equal(st.expanded, 'true');
    assert.ok(await page.$(`#${st.controls}`));
    // Chapters by part, with a chapter's pages under it when it has several.
    const list = await page.$$eval('.rb-bm-list > li:not(.rb-bm-all)', (lis) => lis.map((li) => li.classList.contains('rb-bm-part') ? `# ${li.textContent}` : li.querySelector(':scope > button span').textContent + (li.querySelector('.rb-bm-sub') ? ` [${[...li.querySelectorAll('.rb-bm-sub button span')].map((s) => s.textContent).join(', ')}]` : '')));
    assert.deepEqual(list, ["# Player's book", 'Basics [Start here, The power roll, Rolling]', 'Players Screen', 'Tests [Rolling a test, Easy and hard]', 'Combat [Turns, Moving]', "# Director's book", 'Running the game [Pacing, Rewards]']);
    assert.equal(await page.$eval('.rb-bm-list [aria-current="true"] span', (s) => s.textContent), 'Basics');

    // Picking a page: the ribbon rolls up, and only then does the leaf turn.
    await page.evaluate(() => {
      window.__atTurn = null;
      new MutationObserver(() => {
        if (window.__atTurn === null && document.querySelector('.rb-leaf')) window.__atTurn = document.querySelector('.rb-bm-roll').getBoundingClientRect().height;
      }).observe(document.querySelector('.rb-book'), { childList: true, subtree: true });
    });
    await page.click('.rb-bm-sub button >> text=Moving');
    await page.waitForFunction(() => window.__atTurn !== null);
    assert.equal(await page.evaluate(() => Math.round(window.__atTurn)), 46, 'rolled up before the turn');
    await page.waitForFunction(() => !document.querySelector('.rb-leaf'));
    assert.equal(await page.evaluate(() => document.querySelector('.rb-where').textContent), 'Combat · Moving');
    st = await bmState(page);
    assert.equal(st.expanded, 'false');
  }, { bold: true });
});

test('a page with a pop-up book on it glows in the Sections list, and Calm stills the glow', async () => {
  await withBook(async (page) => {
    await page.click('.rb-bm-tab');
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height > 200);
    const lit = await page.$$eval('.rb-bm-list .rb-bm-live span', (els) => els.map((e) => e.textContent));
    assert.deepEqual(lit, ['Pop-up fight']);
    const css = () => page.$eval('.rb-bm-live span', (e) => [getComputedStyle(e).animationName, getComputedStyle(e).textShadow !== 'none', getComputedStyle(e, '::after').content]);
    const [anim, glows, star] = await css();
    assert.equal(anim, 'rb-bm-glow');
    assert.ok(glows);
    assert.ok(star.includes('\u2726'), star);
    // The stars sit around the title, out of the flow: the row is one line tall
    // for a short title and the stars are positioned, not inline.
    const pos = await page.$eval('.rb-bm-live', (b) => [getComputedStyle(b, '::before').position, getComputedStyle(b, '::after').position, getComputedStyle(b.querySelector('span'), '::before').position]);
    assert.deepEqual(pos, ['absolute', 'absolute', 'absolute']);
    await page.evaluate(() => document.documentElement.setAttribute('data-motion', 'calm'));
    const [calmAnim, calmGlows] = await css();
    assert.equal(calmAnim, 'none');
    assert.ok(calmGlows, 'still shines');
  }, { popUp: true });
});

test('Escape or a click elsewhere rolls the Sections ribbon up; Escape returns focus to it', async () => {
  await withBook(async (page) => {
    await page.click('.rb-bm-tab');
    await page.waitForFunction(() => document.querySelector('.rb-bm-list').contains(document.activeElement));
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height === 46);
    let st = await bmState(page);
    assert.deepEqual([st.expanded, st.tabFocused], ['false', true]);
    await page.click('.rb-bm-tab');
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height > 200);
    await page.mouse.click(200, 650);
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height === 46);
    st = await bmState(page);
    assert.equal(st.expanded, 'false');
    // Opening the contents closes it, and it does not outlive a view switch.
    await page.click('.rb-bm-tab');
    await page.click('[data-act="toc"]');
    assert.equal((await bmState(page)).expanded, 'false');
  });
});

test('the Sections ribbon stops at 80% of the window and its list scrolls', async () => {
  await withBook(async (page) => {
    await page.click('.rb-bm-tab');
    await page.waitForFunction(() => document.querySelector('.rb-bm-roll').getBoundingClientRect().height > 200);
    await page.waitForTimeout(700);
    const r = await page.evaluate(() => {
      const roll = document.querySelector('.rb-bm-roll');
      return { h: roll.getBoundingClientRect().height, win: window.innerHeight, scroll: roll.scrollHeight, overflow: getComputedStyle(roll).overflowY, bottom: roll.getBoundingClientRect().bottom, rb: document.querySelector('.rb').getBoundingClientRect().bottom };
    });
    assert.ok(r.h <= r.win * 0.8 + 1, JSON.stringify(r));
    assert.ok(r.bottom <= r.rb, JSON.stringify(r));
    assert.equal(r.overflow, 'auto');
    assert.ok(r.scroll > r.h);
  }, { viewport: { width: 1280, height: 520 } });
});

test('with reduced motion the Sections ribbon has no transition and turns at once', async () => {
  await withBook(async (page) => {
    assert.equal(await page.$eval('.rb-bm-roll', (r) => getComputedStyle(r).transitionDuration), '0s');
    await page.click('.rb-bm-tab');
    await page.click('.rb-bm-list > li >> text=Tests');
    await page.waitForFunction(() => document.querySelector('.rb-where').textContent === 'Tests', null, { timeout: 1000 });
  }, { reducedMotion: true });
});

// --- the way back -------------------------------------------------------------

const returnState = (page) => page.evaluate(() => {
  const a = document.querySelector('.rb-return');
  if (!a) return null;
  const next = document.querySelector('.rb-turnbar [data-act="next"]'), cs = getComputedStyle(next);
  return { text: a.textContent, href: a.getAttribute('href'), ownRow: a.getBoundingClientRect().bottom <= next.getBoundingClientRect().top, filled: cs.backgroundColor !== 'rgba(0, 0, 0, 0)', height: next.getBoundingClientRect().height };
});

test('a reader a link sent into the book gets a way back to that page, and working pills are filled', async () => {
  await withBook(async (page) => {
    const st = await returnState(page);
    assert.deepEqual([st.text, st.href, st.ownRow, st.filled], ['\u21A9 Back to the campaign home', '/campaigns/camp-1', true, true]);
    assert.ok(st.height >= 44, String(st.height));
    assert.equal(await page.$eval('.rb-turnbar [data-act="prev"]', (b) => b.textContent), '\u2039 Previous');
  }, { bold: true, viewport: { width: 390, height: 844 }, hash: '#basics', referer: 'https://chronicle.test/campaigns/camp-1' });
  await withBook(async (page) => {
    assert.equal((await returnState(page)).text, '\u21A9 Back to where you were');
  }, { bold: true, hash: '#basics', referer: 'https://chronicle.test/campaigns/camp-1/pages/a?x=1' });
});

test('no way back is offered to a reader who opened the book themselves or came from another site', async () => {
  await withBook(async (page) => { assert.equal(await returnState(page), null); }, { bold: true, referer: 'https://chronicle.test/campaigns/camp-1' });
  await withBook(async (page) => { assert.equal(await returnState(page), null); }, { bold: true, hash: '#basics', referer: 'https://elsewhere.test/campaigns/camp-1' });
  await withBook(async (page) => { assert.equal(await returnState(page), null); }, { bold: true, hash: '#basics' });
});

// ---- leaves: a chapter's page torn out of the book, on the campaign home ----

const openRule = async (page, id) => {
  await page.click(`#${id} > summary`);
  await page.waitForSelector(`#${id} .rb-torn .rb-page`);
};
const leafTitle = (page, id) => page.$eval(`#${id} .rb-torn .rb-page h2`, (h) => h.textContent);

for (const bold of [false, true]) {
  const look = bold ? 'bold' : 'plain';

  test(`a ${look} leaf shows one page of its chapter, with none of the reader's chrome`, async () => {
    await withBook(async (page) => {
      await openRule(page, 'd1');
      const st = await page.evaluate(() => {
        const leaf = document.querySelector('#d1 .rb-torn');
        const q = (s) => leaf.querySelectorAll(s).length;
        const wrap = leaf.querySelector('.rb-tear');
        return {
          pages: q('.rb-page'), chrome: q('.rb-bar, .rb-tabs, .rb-front, .rb-search, .rb-turnbar, .rb-book, .rb-spine, .rb-curl, .rb-drawer, .rb-bm, .rb-ribbon, .rb-screen, .rb-toc'),
          folio: leaf.querySelector('.rb-folio').textContent, head: !!leaf.querySelector('.rb-fhd'), kicker: leaf.querySelector('.rb-kicker').textContent,
          look: leaf.getAttribute('data-look'), shadow: getComputedStyle(wrap).filter,
          clip: leaf.querySelector('.rb-page').style.clipPath, tab: leaf.querySelector('.rb-scroll').hasAttribute('tabindex'),
          bodyBg: getComputedStyle(document.querySelector('#d1 .cb-rule-body p')).backgroundColor,
          sideBg: getComputedStyle(document.querySelector('#d2')).backgroundColor,
        };
      });
      assert.equal(st.pages, 1);
      assert.equal(st.chrome, 0);
      // The chapter's opener is skipped; the folio is the page's number in the book.
      assert.equal(await leafTitle(page, 'd1'), 'First rule');
      assert.equal(st.folio, '4');
      assert.ok(st.head);
      assert.equal(st.kicker, 'Souls, Death and Remnants');
      assert.equal(st.look, bold ? 'bold' : 'paper');
      assert.match(st.shadow, /drop-shadow/);
      assert.match(st.clip, /^polygon\(/);
      assert.ok(st.clip.split(',').length > 8, st.clip);
      assert.equal(st.tab, false);
      // Nothing outside the leaf gains paper.
      assert.equal(st.bodyBg, 'rgba(0, 0, 0, 0)');
      assert.equal(st.sideBg, 'rgba(0, 0, 0, 0)');
      // The page's own blocks work as in the book.
      await page.click('#d1 .rb-fc');
      assert.equal(await page.$eval('#d1 .rb-fold:not([hidden])', (e) => e.textContent), 'Remnant 1What remnant 1 does.');
      await page.click('#d1 [data-act="roll"]');
      await page.waitForFunction(() => document.querySelector('#d1 [data-total]').textContent.startsWith('='));
    }, { house: true, bold });
  });
}

test('a leaf keeps no place and answers no key outside itself', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    await page.keyboard.press('ArrowRight');
    await page.keyboard.press('/');
    assert.equal(await leafTitle(page, 'd1'), 'First rule');
    assert.equal(await page.evaluate(() => localStorage.getItem('chronicle.rulebook.place.camp-1')), null);
    assert.equal(await page.evaluate(() => document.querySelectorAll('.rb-front, .rb-closed').length), 0);
  }, { house: true });
});

test('Next and Previous turn among the chapter\'s pages and stop at its ends', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    const words = () => page.$$eval('#d1 .rb-tnav .rb-tw', (els) => els.map((e) => e.textContent));
    // First page: no Previous. The Director-only page is not turned to by a player.
    assert.deepEqual(await words(), ['Next: Second page', 'Open the book at this page']);
    assert.equal(await page.$eval('#d1 .rb-tcount', (e) => e.textContent), '1 of 3');
    await page.click('#d1 [data-act="leaf-next"]');
    assert.equal(await leafTitle(page, 'd1'), 'Second page');
    assert.deepEqual(await words(), ['Previous', 'Next: Third page', 'Open the book at this page']);
    assert.equal(await page.$eval('#d1 .rb-sr', (e) => e.textContent), 'Page 5 of 8: Souls, Death and Remnants, Second page');
    await page.click('#d1 [data-act="leaf-next"]');
    assert.equal(await leafTitle(page, 'd1'), 'Third page');
    // Last page: no Next, and focus lands on Previous rather than being lost.
    assert.deepEqual(await words(), ['Previous', 'Open the book at this page']);
    assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-act')), 'leaf-prev');
    await page.click('#d1 [data-act="leaf-prev"]');
    await page.click('#d1 [data-act="leaf-prev"]');
    assert.equal(await leafTitle(page, 'd1'), 'First rule');
    assert.deepEqual(await words(), ['Next: Second page', 'Open the book at this page']);
    // The other leaf is a one-page chapter: only the link to the book.
    await openRule(page, 'd2');
    assert.deepEqual(await page.$$eval('#d2 .rb-tnav .rb-tw', (els) => els.map((e) => e.textContent)), ['Open the book at this page']);
  }, { house: true, player: true });
});

test('a Director-only page is absent from a player\'s leaf, and present for the Director', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    const seen = [await leafTitle(page, 'd1')];
    while (await page.$('#d1 [data-act="leaf-next"]')) {
      await page.click('#d1 [data-act="leaf-next"]');
      seen.push(await leafTitle(page, 'd1'));
    }
    assert.deepEqual(seen, ['First rule', 'Second page', 'Third page']);
    assert.ok(!(await page.content()).includes('Never shown to a player'));
  }, { house: true, player: true });
  await withBook(async (page) => {
    await openRule(page, 'd1');
    const seen = [await leafTitle(page, 'd1')];
    while (await page.$('#d1 [data-act="leaf-next"]')) {
      await page.click('#d1 [data-act="leaf-next"]');
      seen.push(await leafTitle(page, 'd1'));
    }
    assert.deepEqual(seen, ['First rule', 'Second page', 'DIR ONLY page', 'Third page']);
  }, { house: true });
});

test('a leaf asks for the book only when its rule first opens, and two leaves ask once', async () => {
  await withBook(async (page) => {
    await page.waitForTimeout(300);
    assert.equal(page.bookRequests, 0);
    assert.equal(await page.$('#d1 .rb-page'), null);
    await openRule(page, 'd1');
    assert.equal(page.bookRequests, 1);
    await openRule(page, 'd2');
    assert.equal(page.bookRequests, 1);
    // Closing and opening again asks for nothing more.
    await page.click('#d1 > summary');
    await page.click('#d1 > summary');
    await page.waitForTimeout(150);
    assert.equal(page.bookRequests, 1);
    assert.equal(await leafTitle(page, 'd1'), 'First rule');
  }, { house: true });
});

test('a failed request shows the error with Retry, and Retry asks again', async () => {
  await withBook(async (page) => {
    await page.click('#d1 > summary');
    await page.waitForSelector('#d1 .rb-state-error');
    assert.match(await page.$eval('#d1 .rb-state-error', (e) => e.textContent), /could not be opened/);
    await page.click('#d1 [data-act="retry"]');
    await page.waitForSelector('#d1 .rb-torn .rb-page');
    assert.equal(page.bookRequests, 2);
  }, { house: true, failFirst: true });
});

test('"Open the book at this page" is a real link into the book at the chapter', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    const a = await page.$eval('#d1 .rb-tnav a', (e) => ({ href: e.getAttribute('href'), text: e.textContent }));
    assert.deepEqual(a, { href: HOUSE_HREF, text: 'Open the book at this page' });
  }, { house: true });
});

test('a leaf holds no sideways scroll at 320px, in either look', async () => {
  for (const bold of [false, true]) {
    await withBook(async (page) => {
      await openRule(page, 'd1');
      await openRule(page, 'd2');
      await page.click('#d1 .rb-fc');
      const st = await page.evaluate(() => {
        const leaf = document.querySelector('#d1 .rb-torn'), r = leaf.getBoundingClientRect();
        return { doc: document.documentElement.scrollWidth - document.documentElement.clientWidth, left: r.left, right: r.right, vw: window.innerWidth };
      });
      assert.ok(st.doc <= 0, `page scrolls sideways by ${st.doc}`);
      assert.ok(st.left >= 0 && st.right <= st.vw, JSON.stringify(st));
      // No text is cut off by the page's own edge.
      const cut = await page.evaluate(() => {
        const pg = document.querySelector('#d1 .rb-torn .rb-page').getBoundingClientRect();
        return [...document.querySelectorAll('#d1 .rb-torn .rb-page *')].filter((e) => e.children.length === 0 && e.textContent.trim() && e.getBoundingClientRect().right > pg.right + 1).length;
      });
      assert.equal(cut, 0);
    }, { house: true, bold, viewport: { width: 320, height: 700 } });
  }
});

test('on a touch screen the leaf\'s quiet words are 44px tall', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    await page.click('#d1 [data-act="leaf-next"]');
    const heights = await page.$$eval('#d1 .rb-tnav .rb-tw', (els) => els.map((e) => e.getBoundingClientRect().height));
    assert.equal(heights.length, 3);
    heights.forEach((h) => assert.ok(h >= 44, String(h)));
  }, { house: true, touch: true, viewport: { width: 390, height: 844 } });
});

test('resting the pointer on a rule shows the shared hover card and only then asks for the book', async () => {
  await withBook(async (page) => {
    await page.waitForTimeout(200);
    assert.equal(page.bookRequests, 0);
    await page.hover('#d1 > summary');
    await page.waitForFunction(() => { const c = document.getElementById('chc'); return c && !c.hidden && c.textContent.includes('A soul lingers'); });
    assert.equal(page.bookRequests, 1);
    const card = await page.$eval('#chc', (c) => c.textContent);
    assert.match(card, /Rulebook/);
    assert.match(card, /Souls, Death and Remnants/);
    assert.match(card, /A soul lingers for three days\. A pledge holds it longer\./);
    assert.match(card, /3 pages · First rule/);
    // The Director-only page is not counted or shown on a preview.
    assert.ok(!card.includes('DIR ONLY'));
    // Opening the rule reuses the same fetch and takes the card away.
    await page.click('#d1 > summary');
    await page.waitForSelector('#d1 .rb-torn .rb-page');
    assert.equal(page.bookRequests, 1);
    await page.waitForFunction(() => document.getElementById('chc').hidden);
  }, { house: true });
});

test('a keyboard focus on a rule shows the preview too, and a touch screen shows none', async () => {
  await withBook(async (page) => {
    await page.keyboard.press('Tab');
    await page.waitForFunction(() => { const c = document.getElementById('chc'); return c && !c.hidden && c.textContent.includes('Rulebook'); });
    assert.equal(page.bookRequests, 1);
  }, { house: true });
  await withBook(async (page) => {
    await page.tap('#d1 > summary');
    await page.waitForSelector('#d1 .rb-torn .rb-page');
    await page.waitForTimeout(500);
    assert.equal(await page.evaluate(() => { const c = document.getElementById('chc'); return !c || c.hidden; }), true);
  }, { house: true, touch: true, viewport: { width: 390, height: 844 } });
});

test('opening a rule grows its page in; reduced motion shows it at once', async () => {
  await withBook(async (page) => {
    await openRule(page, 'd1');
    assert.equal(await page.$eval('#d1 .rb-tear', (e) => getComputedStyle(e).animationName), 'rb-grow');
  }, { house: true });
  await withBook(async (page) => {
    await openRule(page, 'd1');
    assert.equal(await page.$eval('#d1 .rb-tear', (e) => getComputedStyle(e).animationName), 'none');
  }, { house: true, reducedMotion: true });
  await withBook(async (page) => {
    await page.evaluate(() => document.documentElement.setAttribute('data-motion', 'calm'));
    await openRule(page, 'd1');
    assert.equal(await page.$eval('#d1 .rb-tear', (e) => getComputedStyle(e).animationName), 'rb-grow-calm');
  }, { house: true });
});

// ---- the way back after a jump, and an address that follows the book ----

const whereTitle = (page) => page.$eval('.rb-where b', (b) => b.textContent);
const settle = (page) => page.waitForFunction(() => !document.querySelector('.rb-leaf'));
const backWord = (page) => page.evaluate(() => { const b = document.querySelector('.rb-back'); return b ? b.textContent : null; });
const firstFolio = (page) => page.$eval('.rb-page:not(.blank):not(.gone) .rb-folio', (f) => f.textContent);

test('a jump through a links block offers "Back to p. N", and using it turns back to that page', async () => {
  await withBook(async (page) => {
    const left = await firstFolio(page);
    assert.equal(await backWord(page), null);
    await page.click('.rb-link[data-chapter="running"]');
    await settle(page);
    assert.equal(await whereTitle(page), 'Running the game');
    assert.equal(await backWord(page), `Back to p. ${left}`);
    assert.equal(await page.evaluate(() => window.location.hash), '#running');
    await page.click('.rb-back');
    await settle(page);
    assert.equal(await whereTitle(page), 'Basics');
    assert.equal(await backWord(page), null);
    assert.equal(await page.evaluate(() => window.location.hash), '#basics');
  }, { jumps: true });
});

test('the return word is quiet text, not a button', async () => {
  await withBook(async (page) => {
    await page.click('.rb-link[data-chapter="running"]');
    await settle(page);
    const st = await page.$eval('.rb-back', (b) => { const cs = getComputedStyle(b); return { bg: cs.backgroundColor, border: cs.borderTopWidth, deco: cs.textDecorationLine }; });
    assert.deepEqual([st.bg, st.border, st.deco], ['rgba(0, 0, 0, 0)', '0px', 'none']);
    await page.hover('.rb-back');
    assert.equal(await page.$eval('.rb-back', (b) => getComputedStyle(b).textDecorationLine), 'underline');
  }, { jumps: true });
});

test('turning a page by hand ends the offer to go back', async () => {
  await withBook(async (page) => {
    await page.click('.rb-link[data-chapter="running"]');
    await settle(page);
    assert.ok(await backWord(page));
    await page.keyboard.press('ArrowLeft');
    await settle(page);
    assert.equal(await backWord(page), null);
  }, { jumps: true });
});

test('a tab and a search result are jumps too', async () => {
  await withBook(async (page) => {
    await page.click('[data-tab="combat"]');
    await settle(page);
    assert.ok((await backWord(page)).startsWith('Back to p. '));
    await page.click('.rb-back');
    await settle(page);
    await page.fill('[data-search]', 'Moving');
    await page.click('.rb-results [data-hit]');
    await settle(page);
    assert.equal(await whereTitle(page), 'Combat');
    assert.ok((await backWord(page)).startsWith('Back to p. '));
  });
});

test('browser Back after a jump stays in the book and turns back; a reload keeps the page', async () => {
  await withBook(async (page) => {
    await page.evaluate(() => { window.__same = 1; });
    await page.click('.rb-link[data-chapter="running"]');
    await settle(page);
    await page.goBack();
    await settle(page);
    assert.equal(await page.evaluate(() => window.__same), 1, 'the page was not reloaded');
    assert.equal(await whereTitle(page), 'Basics');
    assert.equal(await backWord(page), null);
    await page.goForward();
    await settle(page);
    assert.equal(await whereTitle(page), 'Running the game');
    await page.reload();
    await page.waitForSelector('.rb-book');
    assert.equal(await whereTitle(page), 'Running the game');
  }, { jumps: true });
});

test('with reduced motion a jump still offers the way back and the address follows', async () => {
  await withBook(async (page) => {
    await page.click('.rb-link[data-chapter="combat"]');
    assert.ok(await backWord(page));
    assert.equal(await page.evaluate(() => window.location.hash), '#combat');
    await page.click('.rb-back');
    assert.equal(await whereTitle(page), 'Basics');
  }, { jumps: true, reducedMotion: true });
});

test('a book nobody has turned leaves the address alone', async () => {
  await withBook(async (page) => {
    assert.equal(await page.evaluate(() => window.location.hash), '');
  });
});

test('rulebook:goto from inside a page turns to that chapter as a jump', async () => {
  await withBook(async (page) => {
    const fire = (chapter) => page.evaluate((c) => document.querySelector('.rb-page .rb-scroll').dispatchEvent(new CustomEvent('rulebook:goto', { bubbles: true, cancelable: true, detail: { chapter: c } })), chapter);
    assert.equal(await fire('running'), false, 'a turn the book makes cancels the ask');
    await settle(page);
    assert.equal(await whereTitle(page), 'Running the game');
    assert.ok((await backWord(page)).startsWith('Back to p. '));
    assert.equal(await fire('nope'), true, 'an unknown chapter is left to the asker');
    assert.equal(await whereTitle(page), 'Running the game');
  }, { jumps: true });
});

test('the contents drawer holds the phone Back gesture and lets go when it closes itself', async () => {
  await withBook(async (page) => {
    await page.click('[data-act="toc"]');
    assert.deepEqual(await page.evaluate(() => window.__bc), ['hold rb-contents']);
    await page.keyboard.press('Escape');
    assert.deepEqual(await page.evaluate(() => window.__bc), ['hold rb-contents', 'release rb-contents']);
    await page.click('[data-act="toc"]');
    await page.evaluate(() => window.__bcClose['rb-contents']());
    assert.equal(await page.$eval('.rb-drawer', (d) => d.classList.contains('open')), false);
    assert.equal((await page.evaluate(() => window.__bc)).filter((x) => x === 'release rb-contents').length, 2);
  }, { backCloses: true });
});

test('the rules screen holds the Back gesture too', async () => {
  await withBook(async (page) => {
    await page.click('.rb-ribbon');
    assert.ok((await page.evaluate(() => window.__bc)).includes('hold rb-screen'));
    await page.evaluate(() => window.__bcClose['rb-screen']());
    assert.equal(await page.$eval('.rb', (r) => r.classList.contains('rb-screen-open')), false);
  }, { screen: true, backCloses: true });
});

test('the Sections ribbon links once to the reference lists', async () => {
  await withBook(async (page) => {
    await page.click('[data-act="bm"]');
    const links = await page.$$eval('.rb-bm-all a', (as) => as.map((a) => [a.textContent, a.getAttribute('href')]));
    assert.deepEqual(links, [['All reference lists', '/campaigns/camp-1/systems/demo/browse']]);
  });
});

test('a light paper gets the deep gold and red for text; a dark paper keeps the bright ones', async () => {
  const ink = (page) => page.$eval('.rb', (r) => { const cs = getComputedStyle(r); return [cs.getPropertyValue('--rb-gold-ink').trim(), cs.getPropertyValue('--rb-bad-ink').trim(), cs.getPropertyValue('--rb-gold').trim()]; });
  await withBook(async (page) => { assert.deepEqual(await ink(page), ['#8a5a00', '#a3271b', '#fbbf24']); }, { bold: true });
  await withBook(async (page) => { assert.deepEqual(await ink(page), ['#fbbf24', '#f87171', '#fbbf24']); }, { bold: true, theme: Object.assign({}, BOLD_THEME, { paper: '#141925', ink: '#f2f4fa' }) });
});

test('index tabs narrow a long list: each value counts what it leaves, choices combine, Clear lets go', async () => {
  await withBook(async (page) => {
    const box = page.locator('.rb-page [data-ix][data-ready]');
    await box.waitFor();
    assert.deepEqual(await box.locator('.rb-ix-sec').allTextContents(), ['Level 1', 'Level 2']);
    assert.equal(await box.locator('.rb-ix-row').first().locator('.rb-ix-meta').textContent(), 'Level 1');
    await box.locator('[data-ixf-tab="keywords"]').click();
    const vals = () => box.locator('[data-ixf-val]').evaluateAll((bs) => bs.map((b) => b.getAttribute('aria-label') + (b.disabled ? ' (off)' : '')));
    assert.deepEqual(await vals(), ['Melee, 2 entries', 'Move, 1 entry', 'Strike, 1 entry']);
    await box.locator('[data-ixf-val][data-v="Melee"]').click();
    assert.equal(await box.locator('.rb-ix-row:not([hidden])').count(), 2);
    await box.locator('[data-ixf-tab="level"]').click();
    assert.deepEqual(await vals(), ['Level 1, 1 entry', 'Level 2, 1 entry']);
    await box.locator('[data-ixf-val][data-v="1"]').click();
    assert.deepEqual(await box.locator('.rb-ix-row:not([hidden]) .rb-ix-t > b').allTextContents(), ['Blade Strike']);
    assert.match(await box.locator('.rb-ixf-on').textContent(), /^1 of 3/);
    // A value that would empty the list cannot be picked.
    await box.locator('[data-ixf-tab="keywords"]').click();
    assert.deepEqual(await vals(), ['Melee, 1 entry', 'Move, 0 entries (off)', 'Strike, 1 entry']);
    await page.mouse.click(5, 5);
    assert.equal(await box.locator('.rb-ixf-drawer').isHidden(), true, 'a click outside closes the drawer');
    await box.locator('[data-ixf-clear]').click();
    assert.equal(await box.locator('.rb-ix-row:not([hidden])').count(), 3);
    await box.locator('[data-ix-filter]').fill('zzz');
    assert.equal(await box.locator('.rb-ix-none').isVisible(), true, 'an empty list says so');
  }, { index: true, hash: '#all-abilities' });
});

test('an entry name opens its entry in a card: rule words follow with Back, Hold keeps a slip, Turn opens it in its index', async () => {
  await withBook(async (page) => {
    // An entry the book does not list opens no card.
    await page.locator('.rb-page .rb-ref', { hasText: 'Nobody' }).hover();
    await page.waitForTimeout(900);
    assert.equal(await page.locator('.chc.chc--show').count(), 0);
    const ref = page.locator('.rb-page .rb-ref', { hasText: 'Blade Strike' });
    await ref.hover();
    const card = page.locator('.chc.chc--show');
    await card.waitFor();
    assert.equal(await card.locator('.chc__title').textContent(), 'Blade Strike');
    assert.equal(await card.locator('.rb-tier b').textContent(), 'Tier 1');
    await ref.click();
    await card.locator('.rb-peek .rb-term', { hasText: 'slowed down' }).click();
    assert.equal(await card.locator('.chc__title').textContent(), 'Slowed');
    await card.locator('[data-peek-back]').click();
    assert.equal(await card.locator('.chc__title').textContent(), 'Blade Strike');
    await card.locator('[data-peek-hold]').click();
    await page.waitForFunction(() => document.querySelector('.rb-slip-tab') && document.querySelector('.rb-slip-tab').textContent === 'Blade Strike');
    assert.equal(await page.locator('.chc--show').count(), 0, 'Hold puts the card away');
    await page.locator('.rb-pill[data-act="next"]').click();
    await page.waitForTimeout(1200);
    assert.equal(await page.locator('.rb-slip').count(), 1, 'the slip stays while pages turn');
    await page.locator('[data-slip-drop]').click();
    assert.equal(await page.locator('.rb-slip').count(), 0);
    await page.locator('.rb-pill[data-act="prev"]').click();
    await page.waitForTimeout(1200);
    await page.locator('.rb-page .rb-ref', { hasText: 'Blade Strike' }).click();
    await page.locator('.chc.chc--show [data-peek-turn]').click();
    await page.waitForSelector('.rb-ix-row.open');
    assert.equal(await page.locator('.rb-ix-row.open .rb-ix-t > b').textContent(), 'Blade Strike');

  }, { index: true, hash: '#spells' });
});
