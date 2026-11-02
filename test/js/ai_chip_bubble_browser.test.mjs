// ai_chip_bubble_browser.test.mjs — the AI button's bubble opens and stays
// open from its own buttons. Look on the "requests wait" notice and Stay in
// the unsaved-changes warning both remove themselves from the page before the
// page-wide "click outside closes the bubble" check sees the click, so that
// check has to read the click's path, not where the button is now.
//
// Runs boot.js and the ai-chip widget in a real Chromium against a stubbed
// server. Needs playwright-core and a Chromium (see _browser_env.mjs); skips
// locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// The markup layouts/ai_chip.templ renders, trimmed to what the widget reads.
const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script><script src="/ai_chip.js"></script></head><body>
<div class="ai-chip-host" data-widget="ai-chip" data-campaign-id="c1" data-ai-base="ai">
  <button type="button" data-ai-chip aria-expanded="false">AI<span data-ai-chip-label>Your AI</span><span data-ai-chip-n hidden></span></button>
  <div data-ai-note hidden></div>
  <div id="ai-bubble" data-ai-bubble hidden></div>
  <div data-ai-scrim hidden></div>
  <div class="ai-panel" hidden><div data-ai-panel-body></div></div>
</div>
<p id="elsewhere" style="margin-top:400px">elsewhere</p>
</body></html>`;

const BUBBLE = `<div class="aib"><h2 id="ai-bubble-heading">Your AI</h2>
<button type="button" data-ai-copy="/campaigns/c1/ai/copy"><span data-ai-copy-label>Copy for my AI</span></button>
<a class="aib-full" href="/campaigns/c1/ai" data-ai-full>Your AI page</a>
<div data-ai-bubble-warn></div></div>`;

async function open() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage({ viewport: { width: 1000, height: 800 } });
  const state = { waiting: 0 };
  await page.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    const js = (f) => route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, f)) });
    if (p === '/htmx.js') return js('static/vendor/htmx.min.js');
    if (p === '/boot.js') return js('static/js/boot.js');
    if (p === '/ai_chip.js') return js('static/js/widgets/ai_chip.js');
    if (p === '/campaigns/c1/ai/chip') return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ waiting: state.waiting, warnings: 0, stopped: false }) });
    if (p === '/campaigns/c1/ai/bubble') return route.fulfill({ contentType: 'text/html', body: BUBBLE });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/campaigns/c1');
  await page.waitForFunction(() => document.querySelector('.ai-chip-host')._aiChip);
  return { browser, page, state };
}

const bubbleOpen = (page) => page.evaluate(() => !document.querySelector('#ai-bubble').hidden);

test('Look on the notice opens the bubble and it stays open', { skip }, async () => {
  const { browser, page, state } = await open();
  try {
    state.waiting = 3;
    await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    await page.click('[data-ai-note] button:has-text("Look")');
    await page.waitForSelector('[data-ai-copy]');
    assert.equal(await bubbleOpen(page), true);
  } finally { await browser.close(); }
});

test('Stay in the unsaved-changes warning keeps the bubble open', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.click('[data-ai-chip]');
    await page.waitForSelector('[data-ai-copy]');
    await page.evaluate(() => Chronicle.markDirty('test'));
    await page.click('a[data-ai-full]');
    await page.click('[data-ai-bubble-warn] button:has-text("Stay")');
    assert.equal(await bubbleOpen(page), true);
    assert.equal(new URL(page.url()).pathname, '/campaigns/c1', 'Stay must not leave the page');
  } finally { await browser.close(); }
});

test('a click elsewhere on the page still closes the bubble', { skip }, async () => {
  const { browser, page } = await open();
  try {
    await page.click('[data-ai-chip]');
    await page.waitForSelector('[data-ai-copy]');
    await page.click('#elsewhere');
    assert.equal(await bubbleOpen(page), false);
  } finally { await browser.close(); }
});
