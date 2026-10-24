// dynamic_surface_late_box_browser.test.mjs — a surface mounted before its box
// renderers register still paints those boxes.
//
// Deferred scripts run after parsing, and Chronicle.register mounts the
// dynamic-surface widget as soon as it registers, so a page's surface is built
// before character_surface.js supplies the box bodies. Runs boot.js and
// dynamic_surface.js in a real Chromium. Needs playwright-core and a Chromium
// (see _browser_env.mjs); skips locally, and fails under CI, when either is
// missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const SCHEMA = JSON.stringify({
  provider: { key: 'late-box-test', seed: { name: 'Wren' } },
  rows: [{ columns: [{ width: 12, boxes: [
    { id: 'early', title: 'Early', block: 'early-box' },
    { id: 'late', title: 'Late', block: 'late-box' },
  ] }] }],
});

const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script>
<script>Chronicle.escapeHtml = function (s) { return String(s); };</script>
<script src="/dynamic_surface.js" defer></script>
<script>
document.addEventListener('DOMContentLoaded', function () {
  Chronicle.surface.registerBox('early-box', function (def, d) { return '<p class="early">' + d.name + '</p>'; });
});
</script></head><body>
<div id="sheet" data-widget="dynamic-surface"><script type="application/json" data-surface-schema>${SCHEMA}</script></div>
</body></html>`;

async function open() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  await page.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/dynamic_surface.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/widgets/dynamic_surface.js')) });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/');
  await page.waitForFunction(() => document.querySelectorAll('#sheet .cs-row').length === 1, null, { timeout: 5000 });
  return { browser, page };
}

test('a box whose renderer registers after the surface mounted is painted', { skip }, async () => {
  const { browser, page } = await open();
  try {
    assert.equal(await page.locator('#sheet .early').count(), 1, 'the early box should paint');
    assert.equal(await page.locator('#sheet .late').count(), 0, 'the late box has no renderer yet');
    await page.evaluate(() => {
      Chronicle.surface.registerBox('late-box', function (def, d) { return '<p class="late">' + d.name + '</p>'; });
    });
    assert.equal(await page.locator('#sheet .late').textContent(), 'Wren');
  } finally {
    await browser.close();
  }
});

test('a late renderer does not paint a surface that was taken off the page', { skip }, async () => {
  const { browser, page } = await open();
  try {
    const painted = await page.evaluate(() => {
      var sheet = document.getElementById('sheet');
      var body = sheet.querySelectorAll('.cs-row .cs-col > *')[1];
      sheet.remove();
      Chronicle.surface.registerBox('late-box', function () { return '<p class="late">x</p>'; });
      return body.querySelectorAll('.late').length;
    });
    assert.equal(painted, 0);
  } finally {
    await browser.close();
  }
});
