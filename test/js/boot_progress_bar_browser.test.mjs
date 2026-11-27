// boot_progress_bar_browser.test.mjs — the thin progress bar at the top
// (body.is-loading, set by boot.js) shows while an htmx request runs and
// always goes away once it ends: also when the element that asked was
// removed mid-request (a lazy block on a page that was swapped or refreshed),
// and when the request fails.
//
// Runs boot.js with the vendored htmx in a real Chromium against a stubbed
// server. Needs playwright-core and a Chromium (see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script></head><body>
<div id="main"><div id="lazy" hx-get="/slow" hx-trigger="load"></div></div>
<button id="err" hx-get="/err" hx-target="#out">err</button><div id="out"></div>
</body></html>`;

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  t.after(() => browser.close());
  const page = await browser.newPage();
  await page.route('http://t.test/**', async (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/slow') { await new Promise((r) => setTimeout(r, 400)); return route.fulfill({ contentType: 'text/html', body: '<b>filled</b>' }); }
    if (p === '/err') return route.fulfill({ status: 500, contentType: 'text/html', body: 'nope' });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/');
  return page;
}

const loading = (page) => page.evaluate(() => document.body.classList.contains('is-loading'));

test('the bar shows while a lazy block loads and goes once it lands', { skip }, async (t) => {
  const page = await open(t);
  await page.waitForFunction(() => document.body.classList.contains('is-loading'));
  await page.waitForFunction(() => document.querySelector('#lazy b'));
  await page.waitForTimeout(50);
  assert.equal(await loading(page), false);
});

test('a lazy block removed mid-request does not leave the bar running', { skip }, async (t) => {
  const page = await open(t);
  await page.waitForFunction(() => document.body.classList.contains('is-loading'));
  // What a page swap or a live refresh does to #main-content.
  await page.evaluate(() => { document.getElementById('main').innerHTML = '<p>new page</p>'; });
  await page.waitForTimeout(700);
  assert.equal(await loading(page), false);
});

test('a failed request ends the bar too', { skip }, async (t) => {
  const page = await open(t);
  await page.waitForFunction(() => document.querySelector('#lazy b'));
  await page.click('#err');
  await page.waitForTimeout(300);
  assert.equal(await loading(page), false);
});
