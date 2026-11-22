// ai_undo_browser.test.mjs — Undo on an applied request in Your AI's "What it
// did": the Undo word asks first in the row, Keep it folds the question away,
// Undo it posts once and the row then says "Undone <when>", and a refusal
// shows inline with Undo still offered. Checked at desktop and phone width,
// where nothing may run off the page and each touch target is 44px.
//
// Runs Alpine, its collapse plugin and htmx in a real Chromium against a
// stubbed server, with the markup ai_page.templ's aiUndoCell renders. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const CONFIRM = 'Puts back “Lyra” and “The Vault” as they were before. “Sable Quay”, which the AI created, goes to the Trash.';
const READY = (why = '') => `<div id="aic-undo-7" class="aic-undo" x-data="{ asking: false }" :data-undo-open="asking ? '' : null">${why}<button type="button" class="quiet-word aic-undo-btn" @click="asking = !asking" :aria-expanded="asking">Undo</button><div class="aic-ask" x-show="asking" x-cloak x-collapse.duration.200ms><p>${CONFIRM}</p><button type="button" class="btn-secondary" @click="asking = false">Keep it</button> <button type="button" class="btn-secondary" hx-post="/campaigns/c1/ai/requests/7/undo" hx-target="#aic-undo-7" hx-swap="outerHTML" hx-disabled-elt="this"><span class="aic-idle">Undo it</span> <span class="aic-busy">Putting back…</span></button></div></div>`;
const DONE = `<div id="aic-undo-7" class="aic-undo" x-data="{ asking: false }"><p class="aic-undone" role="status">Undone today 2:05 pm</p></div>`;
const REFUSED = READY('<p class="aic-why aic-undo-why" role="alert">Nothing was put back. “Lyra” has been changed since the AI applied it. Undo won\'t overwrite later work; for a page, open its History to choose an earlier version.</p>');

// A row of What it did as the page lays it out: time, icon, line, result, and
// the Undo cell under the line.
const PAGE = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/ai_page.css">
<style>[x-cloak]{display:none!important} body{margin:0;font-family:sans-serif} .quiet-word{background:none;border:0;padding:0;cursor:pointer;text-decoration:underline}
.btn-secondary{padding:4px 10px;border:1px solid #999;background:#fff}
.aic-busy{display:none} [disabled] .aic-idle{display:none} [disabled] .aic-busy{display:inline}</style>
<script src="/htmx.js"></script><script src="/collapse.js" defer></script><script src="/alpine.js" defer></script></head><body>
<section class="aic-hist" id="ai-activity"><div class="aic-calls">
<div class="aic-call" data-call data-kind="ask">
<time class="aic-call-t">2:01 pm</time><i class="fa-solid fa-wand-magic-sparkles aic-call-i"></i>
<span class="aic-call-w">Asked for 3 changes: “Mill news”</span><span class="aic-call-r">1 to add, 2 to change</span>
${READY()}
</div></div></section></body></html>`;

async function open(width, mode, opts = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ viewport: { width, height: 800 }, hasTouch: !!opts.touch, isMobile: !!opts.touch });
  const page = await ctx.newPage();
  const seen = { posts: 0 };
  await page.route('http://t.test/**', async (route) => {
    const p = new URL(route.request().url()).pathname;
    const file = (f, type = 'text/javascript') => route.fulfill({ contentType: type, body: readFileSync(join(root, f)) });
    if (p === '/htmx.js') return file('static/vendor/htmx.min.js');
    if (p === '/alpine.js') return file('static/vendor/alpine.min.js');
    if (p === '/collapse.js') return file('static/vendor/alpine-collapse.min.js');
    if (p === '/ai_page.css') return file('static/css/ai_page.css', 'text/css');
    if (p === '/campaigns/c1/ai/requests/7/undo') {
      seen.posts++;
      await new Promise((r) => setTimeout(r, 150));
      return route.fulfill({ contentType: "text/html; charset=utf-8", body: mode === 'refused' ? REFUSED : DONE });
    }
    return route.fulfill({ contentType: 'text/html; charset=utf-8', body: PAGE });
  });
  await page.goto('http://t.test/');
  await page.waitForFunction(() => window.Alpine && document.querySelector('.aic-undo')._x_dataStack);
  return { browser, page, seen };
}

const visible = (page, sel) => page.evaluate((s) => { const e = document.querySelector(s); return !!e && e.offsetParent !== null && e.getBoundingClientRect().height > 0; }, sel);
const noSideScroll = (page) => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth);

for (const [label, width, touch] of [['1280', 1280, false], ['360', 360, true]]) {
  test(`Undo asks first, and Keep it folds the question away (${label})`, { skip }, async () => {
    const { browser, page, seen } = await open(width, 'done', { touch });
    try {
      assert.equal(await visible(page, '.aic-undo-btn'), true);
      assert.equal(await visible(page, '.aic-ask'), false, 'the question is closed until Undo is pressed');
      await page.click('.aic-undo-btn');
      await page.waitForSelector('.aic-ask', { state: 'visible' });
      assert.match(await page.textContent('.aic-ask p'), /Puts back “Lyra” and “The Vault” as they were before\. “Sable Quay”, which the AI created, goes to the Trash\./);
      // An open question holds back the page's 30-second refresh.
      assert.equal(await page.evaluate(() => !!document.querySelector('[data-undo-open]')), true);
      await page.click('.aic-ask button:has-text("Keep it")');
      await page.waitForSelector('.aic-ask', { state: 'hidden' });
      assert.equal(await page.evaluate(() => !!document.querySelector('[data-undo-open]')), false);
      assert.equal(seen.posts, 0, 'nothing was sent');
      assert.equal(await noSideScroll(page), true);
    } finally { await browser.close(); }
  });

  test(`Undo it posts once and the row says Undone (${label})`, { skip }, async () => {
    const { browser, page, seen } = await open(width, 'done', { touch });
    try {
      await page.click('.aic-undo-btn');
      await page.waitForSelector('.aic-ask', { state: 'visible' });
      // Two quick presses are one request: the button is disabled in flight.
      await page.evaluate(() => { const b = [...document.querySelectorAll('.aic-ask button')].find((x) => x.textContent.includes('Undo it')); b.click(); b.click(); });
      await page.waitForSelector('.aic-undone');
      assert.match(await page.textContent('.aic-undone'), /Undone today 2:05 pm/);
      assert.equal(seen.posts, 1);
      assert.equal(await page.evaluate(() => document.querySelectorAll('.aic-undo button').length), 0, 'no button once undone');
      assert.equal(await noSideScroll(page), true);
    } finally { await browser.close(); }
  });

  test(`A refusal shows inline with Undo still offered (${label})`, { skip }, async () => {
    const { browser, page } = await open(width, 'refused', { touch });
    try {
      await page.click('.aic-undo-btn');
      await page.click('.aic-ask button:has-text("Undo it")');
      await page.waitForSelector('.aic-undo-why');
      assert.match(await page.textContent('.aic-undo-why'), /“Lyra” has been changed since the AI applied it/);
      assert.match(await page.textContent('.aic-undo-why'), /History/);
      assert.equal(await visible(page, '.aic-undo-btn'), true);
      assert.equal(await visible(page, '.aic-ask'), false, 'the question closed again');
      assert.equal(await page.evaluate(() => document.querySelectorAll('.aic-undone').length), 0);
      assert.equal(await noSideScroll(page), true);
    } finally { await browser.close(); }
  });
}

test('A row with nothing kept says No undo, with no button or icon', { skip }, async () => {
  const { browser, page } = await open(360, 'done', { touch: true });
  try {
    await page.evaluate(() => { document.querySelector('.aic-undo').outerHTML = '<div class="aic-undo"><p class="aic-undo-none">No undo</p></div>'; });
    assert.equal(await visible(page, '.aic-undo-none'), true);
    assert.equal(await page.evaluate(() => document.querySelectorAll('.aic-undo button, .aic-undo i').length), 0);
    assert.equal(await noSideScroll(page), true);
  } finally { await browser.close(); }
});

test('Each Undo target is 44px on touch', { skip }, async () => {
  const { browser, page } = await open(360, 'done', { touch: true });
  try {
    const r = await page.evaluate(() => { const b = document.querySelector('.aic-undo-btn').getBoundingClientRect(); return [b.width, b.height]; });
    assert.ok(r[0] >= 44 && r[1] >= 44, `Undo is ${r[0]} x ${r[1]}px`);
    await page.click('.aic-undo-btn');
    await page.waitForSelector('.aic-ask', { state: 'visible' });
    const hs = await page.evaluate(() => [...document.querySelectorAll('.aic-ask button')].map((b) => b.getBoundingClientRect().height));
    assert.ok(hs.every((x) => x >= 44), `buttons are ${hs.join(', ')}px tall`);
  } finally { await browser.close(); }
});
