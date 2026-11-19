// hovercard_longpress_browser.test.mjs — the long press on a link to another
// page inside a page's text belongs to Chronicle: it opens the pinned card with
// Open page, Read it below, Open in new tab and Copy link, the phone's own link
// menu is shut on that link and not on links to other sites, a tap still
// follows the link, and holding then dragging opens nothing.
//
// Runs hovercard.js and entity_tooltip.js in a real Chromium with touch.
// Needs playwright-core and a Chromium (see _browser_env.mjs).

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const file = (p) => readFileSync(join(root, p), 'utf8');

const page = () => `<!doctype html><html><head><style>${file('static/css/paper.css')}body{margin:0;padding:16px;font:16px sans-serif}</style></head><body>
<div class="prose"><p>The party met
<a id="inner" href="/campaigns/c1/entities/e1" data-mention-id="e1" data-entity-preview="/campaigns/c1/entities/e1/preview">Captain Vell</a>
and read <a id="outer" href="https://example.com/x">a site</a>.</p></div>
<ul><li><a id="listed" href="/campaigns/c1/entities/e2" data-entity-preview="/campaigns/c1/entities/e2/preview">In a list</a></li></ul>
<script>
window.__peeked = []; window.__toasts = []; window.__copied = [];
window.Chronicle = { register: function () {}, notify: function (m) { window.__toasts.push(m); },
  peek: { open: function (a) { window.__peeked.push(a); } },
  escapeHtml: function (s) { return s; },
  apiFetch: function () { return Promise.resolve({ ok: true, json: function () { return Promise.resolve({ name: 'Captain Vell', type_name: 'Character', type_label: 'Harbour Watch', entry_excerpt: 'Runs the Watch.' }); } }); } };
Object.defineProperty(navigator, 'clipboard', { value: { writeText: function (t) { window.__copied.push(t); return Promise.resolve(); } } });
</script>
<script>${file('static/js/peek_address.js')}</script>
<script>${file('static/js/hovercard.js')}</script>
<script>${file('static/js/widgets/entity_tooltip.js')}</script>
</body></html>`;

const opened = [];
after(async () => { for (const b of opened) await b.close().catch(() => {}); });

async function open() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  opened.push(browser);
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
  const p = await ctx.newPage();
  await p.setContent(page());
  return { p, browser };
}

// A finger down on the element, held, optionally dragged, then lifted.
async function press(p, sel, { hold = 700, drag = 0 } = {}) {
  const b = await (await p.$(sel)).boundingBox();
  const x = b.x + b.width / 2, y = b.y + b.height / 2;
  const cdp = await p.context().newCDPSession(p);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
  if (drag) await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y + drag }] });
  await p.waitForTimeout(hold);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await p.waitForTimeout(50);
}

const cardState = (p) => p.evaluate(() => {
  const c = document.getElementById('chc');
  return { shown: !!c && !c.hidden, pinned: !!c && c.classList.contains('chc--pinned'),
    rows: c ? [...c.querySelectorAll('.chc__row')].map((r) => r.textContent) : [],
    title: c && c.querySelector('.chc__title') ? c.querySelector('.chc__title').textContent : '',
    heights: c ? [...c.querySelectorAll('.chc__row')].map((r) => Math.round(r.getBoundingClientRect().height)) : [] };
});

test('a long press on a page link in text opens the pinned card with its four rows', { skip }, async () => {
  const { p, browser } = await open();
  await press(p, '#inner');
  const s = await cardState(p);
  assert.ok(s.shown && s.pinned, 'card is open and pinned');
  assert.equal(s.title, 'Captain Vell');
  assert.deepEqual(s.rows, ['Open page', 'Read it below, keep this page', 'Open in new tab', 'Copy link']);
  assert.ok(s.heights.every((h) => h >= 44), 'rows are 44 tall');
  await browser.close();
});

test('Read it below opens the page peek, Copy link says Link copied', { skip }, async () => {
  const { p, browser } = await open();
  await press(p, '#inner');
  await p.tap('[data-chc="copy"]');
  assert.deepEqual(await p.evaluate(() => window.__toasts), ['Link copied']);
  assert.match((await p.evaluate(() => window.__copied))[0], /\/campaigns\/c1\/entities\/e1$/);
  await press(p, '#inner');
  await p.tap('[data-chc="peek"]');
  assert.deepEqual(await p.evaluate(() => window.__peeked), ['/campaigns/c1/entities/e1/peek']);
  await browser.close();
});

test('the phone\'s link menu is suppressed on a page link in text only', { skip }, async () => {
  const { p, browser } = await open();
  const r = await p.evaluate(() => {
    const fire = (id) => {
      const a = document.getElementById(id);
      a.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true }));
      const e = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
      a.dispatchEvent(e);
      return e.defaultPrevented;
    };
    return { inner: fire('inner'), outer: fire('outer'), listed: fire('listed'),
      };
  });
  assert.equal(r.inner, true, 'a page link in text');
  assert.equal(r.outer, false, 'a link to another site keeps the phone\'s menu');
  assert.equal(r.listed, false, 'a link outside page text is left alone');
  // Chromium has no touch callout to read back; the rule's scope is the CSS.
  const css = file('static/css/paper.css');
  assert.match(css, /\.prose:not\(:has\(\[contenteditable="true"\]\)\) a\[data-entity-preview\] \{ -webkit-touch-callout: none; \}/);
  await browser.close();
});

test('a tap still follows the link, and holding then dragging opens nothing', { skip }, async () => {
  const { p, browser } = await open();
  await p.evaluate(() => document.addEventListener('click', (e) => { if (e.target.closest('a')) { window.__clicked = e.target.closest('a').id; e.preventDefault(); } }));
  await p.tap('#inner');
  assert.equal(await p.evaluate(() => window.__clicked), 'inner');
  assert.equal((await cardState(p)).pinned, false, 'a tap pins no card');
  assert.deepEqual((await cardState(p)).rows, [], 'a tap shows no link jobs');
  await press(p, '#inner', { hold: 700, drag: 40 });
  assert.equal((await cardState(p)).pinned, false, 'a drag cancels the press');
  await browser.close();
});
