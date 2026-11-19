// boot_leave_guard_browser.test.mjs — what boot.js keeps for someone leaving
// a page: unsaved work is asked about before a boosted link swaps it away,
// the next page opens at its top and Back returns to the old offset, and on
// a phone Back closes an open sheet and leaves the address alone.
//
// Runs boot.js and theme.js with the vendored htmx in a real Chromium against
// a stubbed server (see _browser_env.mjs); skips locally, and fails under CI,
// when no browser is found.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p));

const page = (name, body) => `<!doctype html><html><head><title>${name}</title>
<script src="/htmx.js"></script><script src="/theme.js"></script><script src="/boot.js"></script><script src="/peek_address.js"></script><script src="/peek_panel.js"></script>
<style>html,body{height:100%;margin:0}#main-content{height:100vh;overflow-y:auto}.tall{height:3000px}</style></head>
<body hx-boost="true" hx-target="#main-content" hx-select="#main-content > *" hx-swap="innerHTML show:window:top">
<main id="main-content"><div id="pg">${body}</div></main></body></html>`;

const pages = {
  '/': page('home', `<form id="f" data-track-changes="new-page" action="/x" method="post"><input id="name" name="name"></form>
    <a id="go" href="/other">other</a><div class="tall"></div><a id="deep" href="/other">deep</a>`),
  '/other': page('other', `<p id="other">other page</p><div class="tall"></div>`),
};

async function launch(viewport) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ viewport });
  const tab = await ctx.newPage();
  await tab.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/vendor/htmx.min.js') });
    if (p === '/theme.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/theme.js') });
    if (p === '/peek_address.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/peek_address.js') });
    if (p === '/peek_panel.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/peek_panel.js') });
    if (p.endsWith('/peek')) return route.fulfill({ contentType: 'text/html', body: '<div data-peek-root>peeked</div>' });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/boot.js') });
    return route.fulfill({ contentType: 'text/html', body: pages[p] || pages['/'] });
  });
  return { browser, tab };
}

test('a boosted link with an unsaved form asks first and does not swap', { skip }, async () => {
  const { browser, tab } = await launch({ width: 1000, height: 700 });
  try {
    await tab.goto('http://t.test/');
    await tab.fill('#name', 'Brother Ansel');
    await tab.click('#go');
    await tab.waitForSelector('#f .ask-slip.is-on', { state: 'attached' });
    assert.equal(await tab.locator('#other').count(), 0, 'the page must not have swapped');
    assert.equal(new URL(tab.url()).pathname, '/');
    // Keep editing leaves everything as it was.
    await tab.click('#f [data-keep]');
    assert.equal(await tab.inputValue('#name'), 'Brother Ansel');
    // Discarding goes on to the page.
    await tab.click('#go');
    await tab.waitForSelector('#f .ask-slip.is-on', { state: 'attached' });
    await tab.click('#f [data-drop]');
    await tab.waitForSelector('#other');
    assert.equal(await tab.evaluate(() => Chronicle.isDirty()), false);
  } finally { await browser.close(); }
});

test('a clean page navigates without asking', { skip }, async () => {
  const { browser, tab } = await launch({ width: 1000, height: 700 });
  try {
    await tab.goto('http://t.test/');
    await tab.click('#go');
    await tab.waitForSelector('#other');
    assert.equal(await tab.locator('.ask-slip').count(), 0);
  } finally { await browser.close(); }
});

test('a new page opens at its top and Back returns to the old offset', { skip }, async () => {
  const { browser, tab } = await launch({ width: 1000, height: 700 });
  try {
    await tab.goto('http://t.test/');
    await tab.evaluate(() => { document.getElementById('main-content').scrollTop = 1800; });
    await tab.evaluate(() => document.getElementById('deep').click());
    await tab.waitForSelector('#other');
    assert.equal(await tab.evaluate(() => document.getElementById('main-content').scrollTop), 0);
    await tab.goBack();
    await tab.waitForFunction(() => document.getElementById('deep'));
    await tab.waitForFunction(() => Math.abs(document.getElementById('main-content').scrollTop - 1800) < 5);
  } finally { await browser.close(); }
});

test('on a phone, Back closes an open sheet and leaves the address alone', { skip }, async () => {
  const { browser, tab } = await launch({ width: 390, height: 800 });
  try {
    await tab.goto('http://t.test/');
    const url = tab.url();
    await tab.evaluate(() => {
      window.shut = 0;
      window.release = Chronicle.backCloses('peek', () => { window.shut++; });
    });
    assert.equal(await tab.evaluate(() => history.state && history.state.sheet), 'peek');
    await tab.goBack();
    await tab.waitForFunction(() => window.shut === 1);
    assert.equal(tab.url(), url);
    assert.equal(await tab.evaluate(() => !!document.getElementById('f')), true, 'no page restore');

    // Closed by its own control: the step is given back and close is not called again.
    await tab.evaluate(() => {
      window.shut = 0;
      window.release = Chronicle.backCloses('peek', () => { window.shut++; });
    });
    await tab.evaluate(() => { window.release(); window.release(); });
    await tab.waitForFunction(() => !(history.state && history.state.sheet));
    assert.equal(await tab.evaluate(() => window.shut), 0);
    assert.equal(tab.url(), url);
  } finally { await browser.close(); }
});

test('on a phone, a link followed from an open sheet leaves no extra Back step', { skip }, async () => {
  const { browser, tab } = await launch({ width: 390, height: 800 });
  try {
    await tab.goto('http://t.test/');
    const before = await tab.evaluate(() => history.length);
    await tab.evaluate(() => {
      window.shut = 0;
      window.release = Chronicle.backCloses('nav-drawer', () => { window.shut++; window.release(); });
    });
    await tab.evaluate(() => document.getElementById('go').click());
    await tab.waitForSelector('#other');
    assert.equal(await tab.evaluate(() => window.shut), 1, 'the sheet closed on the way');
    assert.equal(await tab.evaluate(() => history.length), before + 1, 'the new page took the sheet step');
    await tab.goBack();
    await tab.waitForFunction(() => document.getElementById('f'));
    assert.equal(new URL(tab.url()).pathname, '/');
  } finally { await browser.close(); }
});

test('on a desktop window the helper does nothing', { skip }, async () => {
  const { browser, tab } = await launch({ width: 1200, height: 800 });
  try {
    await tab.goto('http://t.test/');
    const before = await tab.evaluate(() => history.length);
    await tab.evaluate(() => { Chronicle.backCloses('peek', () => {})(); });
    assert.equal(await tab.evaluate(() => history.length), before);
  } finally { await browser.close(); }
});

test('the page peek on a phone: opening pushes a step, Back closes it, the address stays', { skip }, async () => {
  const { browser, tab } = await launch({ width: 390, height: 800 });
  try {
    await tab.goto('http://t.test/');
    const url = tab.url();
    await tab.evaluate(() => Chronicle.peek.open('/campaigns/c1/entities/e1'));
    assert.equal(await tab.evaluate(() => Chronicle.peek.isOpen()), true);
    assert.equal(await tab.evaluate(() => history.state && history.state.sheet), 'peek');
    assert.equal(tab.url(), url);
    await tab.goBack();
    await tab.waitForFunction(() => !Chronicle.peek.isOpen());
    assert.equal(tab.url(), url);
    assert.equal(await tab.evaluate(() => !!document.getElementById('f')), true, 'the page is still there');

    // Closed by its own control: the step is given back.
    await tab.evaluate(() => Chronicle.peek.open('/campaigns/c1/entities/e1'));
    await tab.evaluate(() => Chronicle.peek.close());
    await tab.waitForFunction(() => !(history.state && history.state.sheet));
    assert.equal(tab.url(), url);
  } finally { await browser.close(); }
});
