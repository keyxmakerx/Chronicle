// boot_build_reload_browser.test.mjs — a tab opened before a deploy keeps
// its first load's scripts across boosted links, so boot.js loads a boosted
// page from another build in full instead of swapping it in.
//
// Real htmx and boot.js in a real Chromium against a stubbed server. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const page = (build, name) => `<!doctype html><html><head><meta charset="utf-8"><meta name="chronicle-build" content="${build}"></head>
<body hx-boost="true"><nav><a id="go" href="/next">Next</a></nav><main id="main-content"><h1 id="name">${name}</h1></main>
<script src="/static/vendor/htmx.min.js"></script><script src="/static/js/boot.js"></script><script>window.loads = (window.loads || 0) + 1;</script></body></html>`;

async function run(t, nextBuild) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let fullLoads = 0;
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname.startsWith('/static/')) {
      return route.fulfill({ contentType: 'application/javascript', body: readFileSync(join(root, u.pathname)) });
    }
    const boosted = !!req.headers()['hx-boosted'];
    if (u.pathname === '/next' && !boosted) fullLoads++;
    const build = u.pathname === '/next' ? nextBuild : 'b1';
    return route.fulfill({ contentType: 'text/html', headers: { 'X-Chronicle-Build': build }, body: page(build, u.pathname === '/next' ? 'Next' : 'Start') });
  });
  await p.goto('http://chronicle.test/');
  await p.evaluate(() => { window.marker = true; });
  await p.click('#go');
  await p.waitForFunction(() => document.getElementById('name') && document.getElementById('name').textContent === 'Next');
  await p.waitForLoadState('load');
  return { kept: await p.evaluate(() => window.marker === true), fullLoads };
}

test('a boosted link from the same build swaps in place', { skip }, async (t) => {
  const r = await run(t, 'b1');
  assert.equal(r.kept, true, 'the page was not reloaded');
  assert.equal(r.fullLoads, 0);
});

test('a boosted link that lands on a new build loads the page in full', { skip }, async (t) => {
  const r = await run(t, 'b2');
  assert.equal(r.kept, false, 'the page was reloaded, so its scripts are the new build’s');
  assert.equal(r.fullLoads, 1);
});
