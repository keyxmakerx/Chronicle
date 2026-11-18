// dates_browser.test.mjs — the same instant read in a browser set to en-US
// and to en-GB, through the real script: markup from the server's fallback,
// a saved time zone winning over the device's, and a swapped-in fragment.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const SCRIPT = readFileSync(join(root, 'static/js/dates.js'), 'utf8');

const PAGE = (htmlAttrs) => `<!doctype html><html ${htmlAttrs}><body>
<time id="a" datetime="2026-12-01T15:00:00Z" data-date="weekday">Tuesday 1 December</time>
<time id="b" datetime="2026-11-18T15:00:00Z" data-date="short">18 Nov 2026</time>
<time id="c" datetime="2026-11-18T15:00:00Z" data-date-title title="">Edited 2 days ago</time>
<div id="swap"></div>
<script src="/dates.js"></script></body></html>`;

async function read(locale, htmlAttrs, timezoneId = 'UTC') {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const ctx = await browser.newContext({ locale, timezoneId });
    const page = await ctx.newPage();
    await page.route('http://t.test/**', (route) => {
      if (new URL(route.request().url()).pathname === '/dates.js') {
        return route.fulfill({ contentType: 'text/javascript', body: SCRIPT });
      }
      return route.fulfill({ contentType: 'text/html', body: PAGE(htmlAttrs) });
    });
    await page.goto('http://t.test/');
    const out = await page.evaluate(() => {
      const t = (id) => document.getElementById(id);
      const first = { a: t('a').textContent, b: t('b').textContent, c: t('c').title, cText: t('c').textContent };
      document.getElementById('swap').innerHTML = '<time datetime="2026-11-18T15:00:00Z" data-date="long">18 November 2026</time>';
      document.dispatchEvent(new CustomEvent('htmx:afterSwap', { detail: {} }));
      const swapEl = document.getElementById('swap');
      swapEl.dispatchEvent(new CustomEvent('htmx:afterSwap', { bubbles: true }));
      first.swapped = swapEl.textContent;
      return first;
    });
    return out;
  } finally {
    await browser.close();
  }
}

test('en-US reads month first, en-GB day first, the same instant', { skip }, async () => {
  const us = await read('en-US', '');
  const gb = await read('en-GB', '');
  assert.equal(us.a, 'Tuesday, December 1');
  assert.equal(gb.a, 'Tuesday 1 December');
  assert.equal(us.b, 'Nov 18, 2026');
  assert.equal(gb.b, '18 Nov 2026');
  assert.equal(us.c, 'Nov 18, 2026, 3:00 PM');
  assert.equal(gb.c, '18 Nov 2026, 15:00');
  assert.equal(us.cText, 'Edited 2 days ago');
});

test('a saved time zone wins over the device zone', { skip }, async () => {
  const tokyo = await read('en-GB', 'data-view-tz="Asia/Tokyo"', 'America/Chicago');
  assert.equal(tokyo.c, '19 Nov 2026, 0:00');
});

test('a swapped-in fragment is rewritten too', { skip }, async () => {
  const gb = await read('en-GB', '');
  assert.equal(gb.swapped, '18 November 2026');
});
