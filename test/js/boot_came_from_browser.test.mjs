// boot_came_from_browser.test.mjs — Chronicle.cameFrom() names the page the
// reader just left, even when a boosted link swapped the new page in place
// and document.referrer still names the last page loaded in full. The book's
// way back (rulebook.js returnLink) reads it to say "Back to the campaign
// home" when the book was opened from Home.
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

// Each page's widget records what cameFrom said when it mounted, which is
// when the book places its way back.
const page = (id, link) => `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script></head><body hx-boost="true">
<div data-widget="w" id="${id}"></div>
<a id="go" href="${link}">go</a>
<script>
if (!window.seen) {
  window.seen = [];
  Chronicle.register('w', { init: function (el) { seen.push(el.id + ' ' + Chronicle.cameFrom()); } });
}
</script></body></html>`;

test('cameFrom follows boosted links, and says nothing after Back', { skip }, async () => {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const tab = await browser.newPage();
    await tab.route('http://t.test/**', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
      if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
      if (p === '/campaigns') return route.fulfill({ contentType: 'text/html', body: page('list', '/campaigns/c1') });
      if (p === '/campaigns/c1') return route.fulfill({ contentType: 'text/html', body: page('home', '/campaigns/c1/systems/rules#ch-1') });
      return route.fulfill({ contentType: 'text/html', body: page('book', '/campaigns') });
    });
    // The campaign list is loaded in full, then Home is too, by its address:
    // the browser's referrer is the only thing known about where Home came from.
    await tab.goto('http://t.test/campaigns');
    await tab.goto('http://t.test/campaigns/c1', { referer: 'http://t.test/campaigns' });
    await tab.waitForFunction(() => window.seen && window.seen.length === 1);
    assert.equal(await tab.evaluate(() => window.seen[0]), 'home http://t.test/campaigns');

    // Opening the book from Home is an in-page swap: the referrer still says
    // the campaign list, but the page left is Home.
    await tab.click('#go');
    await tab.waitForFunction(() => window.seen.length === 2);
    assert.equal(await tab.evaluate(() => document.referrer), 'http://t.test/campaigns');
    assert.equal(await tab.evaluate(() => window.seen[1]), 'book http://t.test/campaigns/c1');

    // Back restores Home; it was not reached from the book, so nothing is said.
    await tab.goBack();
    await tab.waitForFunction(() => document.getElementById('home') && window.seen.length === 3);
    assert.equal(await tab.evaluate(() => window.seen[2]), 'home ');
  } finally { await browser.close(); }
});
