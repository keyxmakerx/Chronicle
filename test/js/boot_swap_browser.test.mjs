// boot_swap_browser.test.mjs — boot.js keeps widgets alive across htmx swaps
// that do not replace them, and mounts the new content of an outerHTML swap.
//
// Runs boot.js with the vendored htmx in a real Chromium against a stubbed
// server. Needs playwright-core and a Chromium (see _browser_env.mjs); skips
// locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script></head><body>
<div id="host"><div data-widget="w" id="oldw"></div></div>
<div id="main"><div data-widget="w" id="inmain"></div></div>
<button id="outer" hx-get="/swap" hx-target="#host" hx-swap="outerHTML">swap</button>
<button id="err" hx-get="/err" hx-target="#main">err</button>
<script>
window.log = [];
Chronicle.register('w', {
  init: function (el) { log.push('init ' + el.id); },
  destroy: function (el) { log.push('destroy ' + el.id); },
});
</script></body></html>`;

async function open() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  await page.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/swap') return route.fulfill({ contentType: 'text/html', body: '<div id="host"><div data-widget="w" id="neww"></div></div>' });
    if (p === '/err') return route.fulfill({ status: 404, contentType: 'text/html', body: 'nope' });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/');
  await page.waitForFunction(() => window.log.length === 2);
  await page.evaluate(() => { window.log.length = 0; });
  return { browser, page };
}

test('an error response leaves the target\'s widgets mounted', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.click('#err');
    await page.waitForTimeout(300);
    assert.deepEqual(await page.evaluate(() => window.log), []);
  } finally { await browser.close(); }
});

test('an outerHTML swap destroys the old widget and mounts the new one', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.click('#outer');
    await page.waitForFunction(() => document.getElementById('neww'));
    await page.waitForTimeout(300);
    assert.deepEqual(await page.evaluate(() => window.log), ['destroy oldw', 'init neww']);
  } finally { await browser.close(); }
});
