// boot_first_paint_browser.test.mjs — a page that arrives whole, and links
// that change the page in place.
//
// Chronicle.firstPaint hands a widget what the page sent with its mount;
// a [data-whole] frame stays unseen until the mounts the server filled have
// drawn (or a short wait runs out); page_links.js swaps a plain in-campaign
// link into #main-content without a document load, leaves a form's own
// request alone, and lets an outside link load in full.
//
// Runs boot.js and page_links.js with the vendored htmx in a real Chromium
// against a stubbed server (see _browser_env.mjs); skips locally, and fails
// under CI, when no browser is found.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p));
const C = 'c0ffee00-0000-4000-8000-000000000001';

const page = (title, body) => `<!doctype html><html><head><title>${title}</title>
<style>[data-whole]{transition:opacity 20ms}[data-whole]:not([data-whole-ready]){opacity:0}</style>
<script src="/htmx.js"></script><script src="/boot.js"></script><script src="/page_links.js"></script></head>
<body><main id="main-content"><div id="pg">${body}</div></main>
<script>
Chronicle.register('slow', { init: function (el) {
  var d = Chronicle.firstPaint(el);
  el.textContent = d ? 'drawn ' + d.word : 'empty';
  if (el.hasAttribute('data-late')) { setTimeout(function () { Chronicle.drawn(el); }, 300); return; }
  Chronicle.drawn(el);
}});
</script></body></html>`;

const pages = {
  [`/campaigns/${C}/a`]: page('A', `
    <div id="frame" data-whole>
      <div id="mount" data-widget="slow" data-first-paint><template data-first-paint-data>{"word":"seeded"}</template></div>
    </div>
    <a id="inpage" href="/campaigns/${C}/b">to b</a>
    <a id="out" href="/campaigns">out</a>
    <a id="blank" href="/campaigns/${C}/b" target="_blank">new tab</a>
    <form id="f" method="post" action="/campaigns/${C}/post"><button id="post">post</button></form>`),
  [`/campaigns/${C}/b`]: page('B', `<p id="b">page b</p>`),
  [`/campaigns/${C}/late`]: page('Late', `
    <div id="frame" data-whole><div id="mount" data-widget="slow" data-first-paint data-late></div></div>`),
  [`/campaigns/${C}/block`]: page('Block', `
    <div id="frame" data-whole><div id="lazy" hx-get="/slow" hx-trigger="load" hx-swap="innerHTML"></div></div>`),
  [`/campaigns/${C}/post`]: page('Posted', `<p id="posted">posted</p>`),
  '/campaigns': page('Campaigns', `<p id="list">list</p>`),
};

async function launch() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const tab = await (await browser.newContext()).newPage();
  const requests = [];
  await tab.route('http://t.test/**', async (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname;
    if (p === '/slow') {
      await new Promise((r) => setTimeout(r, 300));
      return route.fulfill({ contentType: 'text/html', body: '<p id="filled">filled</p>' });
    }
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/vendor/htmx.min.js') });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/boot.js') });
    if (p === '/page_links.js') return route.fulfill({ contentType: 'text/javascript', body: read('static/js/page_links.js') });
    if (req.resourceType() === 'document' || req.resourceType() === 'xhr' || req.resourceType() === 'fetch') requests.push({ p, type: req.resourceType(), method: req.method() });
    return route.fulfill({ contentType: 'text/html', body: pages[p] || pages['/campaigns'] });
  });
  return { browser, tab, requests };
}

test('a widget takes the first content the page sent and the frame then shows', { skip }, async () => {
  const { browser, tab } = await launch();
  try {
    await tab.goto(`http://t.test/campaigns/${C}/a`);
    await tab.waitForSelector('#frame[data-whole-ready]');
    assert.equal(await tab.textContent('#mount'), 'drawn seeded');
    assert.equal(await tab.locator('#mount template').count(), 0, 'the widget leaves its seed behind');
  } finally { await browser.close(); }
});

test('the frame waits for a mount that has not drawn, then gives up', { skip }, async () => {
  const { browser, tab } = await launch();
  try {
    await tab.goto(`http://t.test/campaigns/${C}/late`);
    await tab.waitForTimeout(100);
    assert.equal(await tab.locator('#frame[data-whole-ready]').count(), 0, 'shown before its widget drew');
    await tab.waitForSelector('#frame[data-whole-ready]');
  } finally { await browser.close(); }
});

test('a plain link swaps in place; a form and an outside link do not', { skip }, async () => {
  const { browser, tab, requests } = await launch();
  try {
    await tab.goto(`http://t.test/campaigns/${C}/a`);
    await tab.evaluate(() => { window.__same = true; });

    await tab.click('#inpage');
    await tab.waitForSelector('#b');
    assert.equal(await tab.evaluate(() => window.__same === true), true, 'the link reloaded the document');
    assert.equal(await tab.title(), 'B');
    assert.equal(new URL(tab.url()).pathname, `/campaigns/${C}/b`);

    await tab.goBack();
    await tab.waitForSelector('#inpage');
    assert.equal(await tab.evaluate(() => window.__same === true), true, 'Back reloaded the document');

    // A form keeps its own request: a full document POST, not an in-place swap.
    const before = requests.length;
    await tab.click('#post');
    await tab.waitForSelector('#posted');
    assert.equal(await tab.evaluate(() => window.__same === true), false, 'the form was swapped into the page');
    assert.ok(requests.slice(before).some((r) => r.type === 'document' && r.method === 'POST'), 'the form did not post as a document');

    // A link out of the campaign loads in full.
    await tab.goto(`http://t.test/campaigns/${C}/a`);
    await tab.evaluate(() => { window.__same = true; });
    await tab.click('#out');
    await tab.waitForSelector('#list');
    assert.equal(await tab.evaluate(() => window.__same === true), false, 'an outside link was swapped in place');
  } finally { await browser.close(); }
});

test('a block that fills itself on arrival keeps its room unseen until it has', { skip }, async () => {
  const { browser, tab } = await launch();
  try {
    await tab.goto(`http://t.test/campaigns/${C}/block`);
    await tab.waitForTimeout(100);
    assert.equal(await tab.locator('#frame[data-whole-ready]').count(), 0, 'shown before its slow block arrived');
    await tab.waitForSelector('#frame[data-whole-ready]');
    assert.equal(await tab.locator('#filled').count(), 1, 'shown without its block');
  } finally { await browser.close(); }
});
