// annals_phone_scroll.spec.mjs — Playwright coverage for the Annals look on a
// phone: the content area still scrolls, and the title rows still wrap.
//
// A harness page, as in page_editor.spec.mjs: the real page_look_annals.css
// read off disk, with the app shell transcribed from layouts/app.templ, the
// page header from entities/show.templ and the home hero from
// campaigns/dashboard_blocks.templ. The shell's main column is a .min-w-0
// child of a .flex row with the page title deep inside it, so a title rule
// that matches too loosely wraps the shell, which then grows to the page's
// full height and leaves nothing to scroll.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/annals_phone_scroll.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const annalsCSS = readFileSync(join(repoRoot, 'static/css/page_look_annals.css'), 'utf8');

// The Tailwind utilities the transcribed markup leans on.
const UTILITIES = `
  html, body { margin: 0; } .h-full { height: 100%; } .h-screen { height: 100vh; }
  .flex { display: flex; } .flex-col { flex-direction: column; } .flex-wrap { flex-wrap: wrap; }
  .flex-1 { flex: 1 1 0%; } .min-w-0 { min-width: 0; } .relative { position: relative; }
  .overflow-hidden { overflow: hidden; } .overflow-y-auto { overflow-y: auto; }
  .items-start { align-items: flex-start; } .items-center { align-items: center; }
  .justify-between { justify-content: space-between; } .gap-2 { gap: .5rem; }
  .h-14 { height: 3.5rem; } h1 { margin: 0; font-size: 1.875rem; }
`;

const pageHeader = `
  <div class="flex flex-wrap items-start justify-between" data-t="page-row">
    <div class="min-w-0">
      <div class="flex items-center gap-2"><h1 class="cb-title">Brother Quill of the Long Road</h1><button type="button">star</button></div>
    </div>
    <div><button type="button">Edit page</button><button type="button">Share</button></div>
  </div>`;

const homeHero = `
  <div class="cb-hero flex items-center justify-between" data-t="hero-row">
    <div class="min-w-0"><h1 class="cb-title">The Shattered Heroes of Therin</h1></div>
    <a href="#">Settings</a>
  </div>`;

function harnessHTML(header) {
  return `<!doctype html>
<html lang="en" class="h-full" data-cz-page="annals">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<style>${UTILITIES}</style><style>${annalsCSS}</style></head>
<body class="h-full">
  <div class="flex h-screen overflow-hidden" data-t="shell">
    <div id="app-main" class="relative flex-1 flex flex-col overflow-hidden min-w-0">
      <header class="h-14">Top bar</header>
      <main id="main-content" class="flex-1 overflow-y-auto">
        <div class="max-w-7xl mx-auto">${header}<div style="height:3000px">The rest of the page.</div></div>
      </main>
    </div>
  </div>
</body></html>`;
}

async function swipeUp(cdp, x, y, dy) {
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
  for (let i = 1; i <= 12; i++) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y - (dy * i) / 12 }] });
    await new Promise((r) => setTimeout(r, 16));
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
}

for (const [name, header, row] of [['a page', pageHeader, 'page-row'], ['the campaign home', homeHero, 'hero-row']]) {
  test(`Annals on a phone: ${name} scrolls, and its title row wraps`, async () => {
    const browser = await chromium.launch();
    try {
      const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
      const page = await ctx.newPage();
      await page.setContent(harnessHTML(header));
      const got = await page.evaluate((row) => {
        const main = document.getElementById('main-content');
        const css = (sel) => getComputedStyle(document.querySelector(`[data-t="${sel}"]`)).flexWrap;
        return { shell: css('shell'), row: css(row), main: main.clientHeight, inner: main.scrollHeight, view: innerHeight };
      }, row);
      assert.equal(got.shell, 'nowrap', 'the app shell must not wrap');
      assert.equal(got.row, 'wrap', 'the title row wraps on a phone');
      assert.ok(got.main < got.view, `the content area fits the screen (${got.main}px of ${got.view}px)`);
      assert.ok(got.inner > got.main, 'the content area has more to scroll to');

      const cdp = await ctx.newCDPSession(page);
      await swipeUp(cdp, 200, 600, 300);
      await page.waitForTimeout(300);
      const top = await page.evaluate(() => document.getElementById('main-content').scrollTop);
      assert.ok(top > 100, `a finger swipe scrolls the page (scrollTop ${top})`);
    } finally {
      await browser.close();
    }
  });
}
