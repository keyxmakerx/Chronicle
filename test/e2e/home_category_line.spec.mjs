// home_category_line.spec.mjs — Playwright coverage for the Categories block
// under the Annals look: the categories run as one wrapping line of words, so
// a long list stays a few lines tall on a phone, and each word is still tall
// enough for a finger.
//
// A harness page, as in annals_phone_scroll.spec.mjs: the real
// page_look_annals.css read off disk, with the block's markup transcribed
// from dashCategoryGrid in campaigns/dashboard_blocks.templ.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/home_category_line.spec.mjs

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

// The Tailwind utilities the transcribed markup leans on, as the Standard
// look draws them: a grid of bordered tiles.
const UTILITIES = `
  html, body { margin: 0; font: 16px/1.5 sans-serif; } a { color: inherit; text-decoration: none; }
  .grid { display: grid; } .grid-cols-2 { grid-template-columns: repeat(2, minmax(0, 1fr)); } .gap-3 { gap: .75rem; }
  .block { display: block; } .flex { display: flex; } .items-center { align-items: center; }
  .card { border: 1px solid #ccc; background: #fff; border-radius: 12px; } .px-4 { padding-inline: 1rem; } .py-3 { padding-block: .75rem; }
  .mb-1 { margin-bottom: .25rem; } .w-7 { width: 1.75rem; } .h-7 { height: 1.75rem; }
  .truncate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } .text-sm { font-size: .875rem; } .text-xs { font-size: .75rem; }
`;

const NAMES = ['Characters', 'Locations', 'Factions', 'Items', 'Creatures', 'Quests', 'Lore', 'Shops', 'Heroes', 'Maps', 'Races', 'Timelines', 'Calendars', 'Adventures'];

function word(name, n) {
  return `<a href="#" class="cb-word card px-4 py-3 block group" aria-label="${name}, ${n} pages"${n === 0 ? ' data-empty' : ''}>
    <div class="cb-word-main flex items-center mb-1"><span class="cb-swatch w-7 h-7"></span><span class="cb-word-name text-sm truncate">${name}</span></div>
    <div class="cb-word-meta text-xs"><span class="cb-word-n">${n}</span><span class="cb-word-unit"> pages</span></div>
  </a>`;
}

function harnessHTML(names) {
  return `<!doctype html>
<html lang="en" data-cz-page="annals">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<style>${UTILITIES}</style><style>${annalsCSS}</style></head>
<body><main id="main-content" style="padding: 24px">
  <div class="cb-contents"><h2 class="cb-head">Categories</h2>
    <div class="cb-rows cb-line grid grid-cols-2 gap-3">${names.map((n, i) => word(n, i === 8 ? 0 : i + 1)).join('')}</div>
  </div>
</main></body></html>`;
}

async function measure(names, viewport, touch) {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ viewport, hasTouch: touch, isMobile: touch });
    const page = await ctx.newPage();
    await page.setContent(harnessHTML(names));
    return await page.evaluate(() => {
      const words = [...document.querySelectorAll('.cb-word')].map((e) => e.getBoundingClientRect());
      return {
        line: document.querySelector('.cb-line').getBoundingClientRect().height,
        lines: new Set(words.map((r) => Math.round(r.top))).size,
        shortest: Math.min(...words.map((r) => r.height)),
        widest: Math.max(...words.map((r) => r.right)),
        view: innerWidth,
        sideways: document.documentElement.scrollWidth > innerWidth + 1,
        unit: getComputedStyle(document.querySelector('.cb-word-unit')).display,
      };
    });
  } finally {
    await browser.close();
  }
}

test('Annals on a phone: fourteen categories run as a few lines of words', async () => {
  const got = await measure(NAMES, { width: 390, height: 844 }, true);
  assert.ok(got.lines > 1 && got.lines <= 5, `fourteen words wrap onto a few lines (got ${got.lines})`);
  assert.ok(got.line <= 5 * 44 + 8, `the line stays short (${got.line}px)`);
  assert.ok(got.shortest >= 44, `each word is tall enough for a finger (${got.shortest}px)`);
  assert.equal(got.unit, 'none', 'the line shows the bare count; the link names it in full');
  assert.equal(got.sideways, false, 'nothing runs off the side');
});

test('Annals on a phone: a name longer than the line wraps inside it', async () => {
  const got = await measure(['A Category With A Name Far Longer Than Any Phone Is Wide, Truly', 'Lore'], { width: 390, height: 844 }, true);
  assert.ok(got.widest <= got.view, `the long name stays on the screen (${got.widest}px of ${got.view}px)`);
  assert.equal(got.sideways, false, 'nothing runs off the side');
});

test('Annals on a computer: the same line with a pointer-sized height', async () => {
  const got = await measure(NAMES, { width: 1280, height: 800 }, false);
  assert.ok(got.lines <= 2, `fourteen words fit two lines (got ${got.lines})`);
  assert.ok(got.shortest >= 32, `each word keeps a 32px hit area (${got.shortest}px)`);
});
