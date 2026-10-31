// boot_history_browser.test.mjs — Back and Forward destroy the widgets of the
// page being left, as well as mounting the restored page's.
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

const page = (id, link) => `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script></head><body hx-boost="true">
<div data-widget="w" id="${id}"></div>
<a id="go" href="${link}">go</a>
<script>
if (!window.log) {
  window.log = [];
  Chronicle.register('w', {
    init: function (el) { log.push('init ' + el.id); },
    destroy: function (el) { log.push('destroy ' + el.id); },
  });
}
</script></body></html>`;

test('Back destroys the page it leaves before mounting the restored one', { skip }, async () => {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const tab = await browser.newPage();
    await tab.route('http://t.test/**', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
      if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
      if (p === '/b') return route.fulfill({ contentType: 'text/html', body: page('wb', '/') });
      return route.fulfill({ contentType: 'text/html', body: page('wa', '/b') });
    });
    await tab.goto('http://t.test/');
    await tab.waitForFunction(() => window.log && window.log.length === 1);

    await tab.click('#go');
    await tab.waitForFunction(() => document.getElementById('wb') && window.log.includes('init wb'));
    await tab.goBack();
    await tab.waitForFunction(() => document.getElementById('wa') && window.log.length >= 5);
    await tab.waitForTimeout(200);
    assert.deepEqual(await tab.evaluate(() => window.log), ['init wa', 'destroy wa', 'init wb', 'destroy wb', 'init wa']);
  } finally { await browser.close(); }
});
