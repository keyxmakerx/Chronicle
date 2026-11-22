// table_moments_browser.test.mjs: the Table's count when a card's moment is
// waiting (internal/plugins/rolls/static/js/table.js) in a real browser, with
// the real script and stylesheet and the network replaced by a table of
// answers. It pins: the count is the rolls not yet seen plus the moments the
// server says are waiting; looking at the log clears only the rolls; a new
// moment is fetched again on the live message; a notice that slides in does
// not take focus; and a Table that is not a Director's never asks for a moment.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const tableCSS = readFileSync(join(root, 'internal/plugins/rolls/static/css/table.css'), 'utf8');
const tableJS = readFileSync(join(root, 'internal/plugins/rolls/static/js/table.js'), 'utf8');
let browser;
after(async () => { if (browser) await browser.close(); });

const panel = (rows, at = '') => `<div class="dkm" data-dkm data-at="${at}"><ul data-dkm-index hidden>${rows.map((r) =>
  `<li data-id="${r.id}" data-slide="${!!r.slide}" data-deck="${r.deck || 'Omens'}" data-reading="${r.reading || 'A card is waiting'}"></li>`).join('')}</ul>${at ? `<section class="dkm-panel" data-moment="${at}"><h2 tabindex="-1">A card’s moment</h2><button type="button" data-dkm-close>Close</button></section>` : ''}</div>`;

const PAGE = (director) => `<!doctype html><html><head><meta charset="utf-8"><style>:root{--dur-slide:360ms}header{display:flex;justify-content:flex-end}[hidden]{display:none!important}${tableCSS}</style></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1" data-director="${director}" data-decks="true" data-gm-term="Director">
<button type="button" class="tt-tab" data-tt-tab aria-expanded="false" aria-controls="tt-drawer"><span data-tt-count hidden></span></button></div></header>
<main id="main-content"><button id="elsewhere">Elsewhere</button></main>
<script>
window.__sockets = []; window.__asked = []; window.__moments = window.__moments || [];
window.WebSocket = function () { var s = { l: {}, addEventListener: function (t, f) { this.l[t] = f; }, close: function () {} }; window.__sockets.push(s); return s; };
function respond(body, html) { return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); }, text: function () { return Promise.resolve(html || ''); } }); }
window.Chronicle = {
  register: function (n, o) { window.__reg = o; },
  apiFetch: function (u) {
    window.__asked.push(String(u));
    if (/decks\\/moments\\/panel/.test(u)) {
      var at = (String(u).match(/at=([^&]+)/) || [])[1] || '';
      return respond(null, window.__panel(window.__moments, decodeURIComponent(at)));
    }
    return respond(/settings/.test(u) ? {} : { rolls: [], more: false });
  },
  escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  tableDice: { motion: function () { return 'off'; }, svg: function () { return ''; }, throwInto: function () { return Promise.resolve(); } },
  dice: { roll: function () { return Promise.resolve([]); }, catalogue: function () { return Promise.resolve({ sets: [], crits: { succ: [], fail: [] } }); } }
};
window.__panel = ${panel.toString()};
</script>
<script>${tableJS}</script>
<script>window.__reg.init(document.querySelector('.tt-host'));</script>
</body></html>`;

async function open(director, moments = []) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 800 }, reducedMotion: 'reduce' });
  const p = await ctx.newPage();
  await p.route('http://chronicle.test/', (r) => r.fulfill({ contentType: 'text/html', body: PAGE(director) }));
  await p.addInitScript((m) => { window.__moments = m; }, moments);
  await p.goto('http://chronicle.test/');
  return { p, close: () => ctx.close() };
}
const count = (p) => p.evaluate(() => { const c = document.querySelector('[data-tt-count]'); return c.hidden ? '' : c.textContent; });
const asks = (p) => p.evaluate(() => window.__asked.filter((u) => /decks\/moments/.test(u)));

test('the count is the moments waiting plus the rolls not yet seen, and the log clears only the rolls', { skip }, async () => {
  const { p, close } = await open('true', [{ id: 'm1' }, { id: 'm2' }]);
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').textContent === '2');
  await p.evaluate(() => document.querySelector('.tt-host')._table.bump());
  assert.equal(await count(p), '3');
  await p.click('[data-tt-tab]');
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').textContent === '2');
  assert.equal(await count(p), '2', 'looking at the rolls must not clear a moment');
  const wait = await p.evaluate(() => ({ head: document.querySelector('[data-tt-wait] h3').textContent, rows: document.querySelectorAll('[data-tt-moment]').length }));
  assert.deepEqual(wait, { head: 'Waiting on you', rows: 2 });
  await close();
});

test('a live message fetches the list again and the count follows it', { skip }, async () => {
  const { p, close } = await open('true', [{ id: 'm1' }, { id: 'm2' }]);
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').textContent === '2');
  await p.evaluate(() => { window.__moments = [{ id: 'm2' }]; window.__sockets[0].l.message({ data: JSON.stringify({ type: 'decks.moment' }) }); });
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').textContent === '1');
  await p.evaluate(() => { window.__moments = []; window.dispatchEvent(new CustomEvent('chronicle:decks-moment')); });
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').hidden);
  await close();
});

test('a new moment slides its notice in without taking focus, and closing keeps the count', { skip }, async () => {
  const { p, close } = await open('true', []);
  await p.waitForFunction(() => window.__asked.some((u) => /moments\/panel/.test(u)));
  await p.focus('#elsewhere');
  await p.evaluate(() => { window.__moments = [{ id: 'm9', slide: true, deck: 'Omens' }]; window.dispatchEvent(new CustomEvent('chronicle:decks-moment')); });
  await p.waitForSelector('#tt-moment [data-moment="m9"]');
  assert.equal(await p.evaluate(() => document.activeElement.id), 'elsewhere', 'the notice took focus');
  assert.equal(await count(p), '1');
  await p.click('#tt-moment [data-dkm-close]');
  await p.waitForFunction(() => !document.getElementById('tt-moment'));
  assert.equal(await count(p), '1', 'the moment is still waiting');
  await close();
});

test('a moment that was waiting when the page opened is counted, not slid in', { skip }, async () => {
  const { p, close } = await open('true', [{ id: 'm1', slide: true }]);
  await p.waitForFunction(() => document.querySelector('[data-tt-count]').textContent === '1');
  assert.equal(await p.evaluate(() => !!document.getElementById('tt-moment')), false);
  await close();
});

test('a Table that is not a Director’s never asks for a moment', { skip }, async () => {
  const { p, close } = await open('false', [{ id: 'm1' }]);
  await p.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-moment')));
  await p.evaluate(() => window.__sockets[0].l.message({ data: JSON.stringify({ type: 'decks.moment' }) }));
  await p.click('[data-tt-tab]');
  await p.waitForTimeout(300);
  assert.deepEqual(await asks(p), []);
  assert.equal(await count(p), '');
  await close();
});
