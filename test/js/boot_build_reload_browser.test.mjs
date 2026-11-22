// boot_build_reload_browser.test.mjs — a tab opened before a deploy keeps
// its first load's scripts across boosted links, live refreshes and Back, so
// boot.js loads a page from another build in full instead of swapping it in.
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

// The page's live refresh fetches it with htmx.ajax into #main-content, which
// htmx does not count as boosted; the restart's dropped socket starts one.
async function live(t, nextBuild) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let served = 0, fullLoads = 0;
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname.startsWith('/static/')) {
      return route.fulfill({ contentType: 'application/javascript', body: readFileSync(join(root, u.pathname)) });
    }
    const refresh = !!req.headers()['x-page-live'];
    if (!refresh && served++) fullLoads++;
    const build = served > 1 || refresh ? nextBuild : 'b1';
    return route.fulfill({ contentType: 'text/html', headers: { 'X-Chronicle-Build': build }, body: page(build, refresh ? 'Fresh' : 'Start') });
  });
  await p.goto('http://chronicle.test/hero');
  await p.evaluate(() => { window.marker = true; });
  await p.evaluate(() => window.htmx.ajax('GET', '/hero', { target: document.getElementById('main-content'), select: '#main-content > *', swap: 'innerHTML', headers: { 'HX-Boosted': 'true', 'X-Page-Live': '/hero' } }).catch(() => {}));
  await p.waitForFunction(() => window.marker !== true || document.getElementById('name').textContent === 'Fresh');
  await p.waitForLoadState('load');
  return { kept: await p.evaluate(() => window.marker === true), fullLoads };
}

test('a live refresh from the same build swaps in place', { skip }, async (t) => {
  const r = await live(t, 'b1');
  assert.equal(r.kept, true);
  assert.equal(r.fullLoads, 0);
});

test('a live refresh that lands on a new build loads the page in full', { skip }, async (t) => {
  const r = await live(t, 'b2');
  assert.equal(r.kept, false, 'the page was reloaded, so its scripts are the new build’s');
  assert.equal(r.fullLoads, 1);
});

// Back re-fetches the page (no history cache) without a beforeSwap.
async function back(t, laterBuild) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let deployed = false, fullLoads = 0;
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname.startsWith('/static/')) {
      return route.fulfill({ contentType: 'application/javascript', body: readFileSync(join(root, u.pathname)) });
    }
    if (!req.headers()['hx-request'] && u.pathname === '/') fullLoads++;
    const build = deployed ? laterBuild : 'b1';
    return route.fulfill({ contentType: 'text/html', headers: { 'X-Chronicle-Build': build }, body: page(build, u.pathname === '/next' ? 'Next' : 'Start') });
  });
  await p.goto('http://chronicle.test/');
  await p.evaluate(() => { window.htmx.config.historyCacheSize = 0; });
  await p.click('#go');
  await p.waitForFunction(() => document.getElementById('name').textContent === 'Next');
  await p.evaluate(() => { window.marker = true; });
  deployed = true;
  await p.goBack();
  await p.waitForFunction(() => window.marker !== true || document.getElementById('name').textContent === 'Start');
  await p.waitForFunction(() => document.getElementById('name').textContent === 'Start');
  await p.waitForLoadState('load');
  return { kept: await p.evaluate(() => window.marker === true), fullLoads };
}

test('Back within the same build restores in place', { skip }, async (t) => {
  const r = await back(t, 'b1');
  assert.equal(r.kept, true);
  assert.equal(r.fullLoads, 1);
});

test('Back that lands on a new build reloads the page in full', { skip }, async (t) => {
  const r = await back(t, 'b2');
  assert.equal(r.kept, false, 'the page was reloaded, so its scripts are the new build’s');
  assert.equal(r.fullLoads, 2);
});
