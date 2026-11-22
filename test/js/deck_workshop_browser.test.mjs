// deck_workshop_browser.test.mjs: the Director's deck page
// (internal/plugins/decks/static/js/deck_workshop.js and static/css/decks_workshop.css)
// in a real browser, on markup shaped like workshop_deck.templ and
// workshop_table.templ, with the server replaced by a record of what was sent
// and the fragments it would answer with. It pins that a refused Draw fetches
// the table row again (so the count, Draw and the stepper say what is true),
// that a save refused as stale leaves the name field holding the newer name,
// and that on a 390 wide phone the controls the look check measured are at
// least 44 by 44 while a desktop keeps its size. The walk through the real
// pages is test/live/deck_workshop.spec.mjs.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const CSS = readFileSync(join(root, 'static', 'css', 'decks_workshop.css'), 'utf8');
const JS = readFileSync(join(root, 'internal', 'plugins', 'decks', 'static', 'js', 'deck_workshop.js'), 'utf8');
const ORIGIN = 'http://t.test';
let browser;
after(async () => { if (browser) await browser.close(); });

// tableHTML is the table row as the server draws it, for a pile of n.
const tableHTML = ({ pile, version, name }) => `<div id="dkw-table"><span hidden data-dkw-version="${version}" data-dkw-deckname="${name}"></span>
<section class="dkw-table" data-dkw-table data-pile="${pile}" data-maxdraw="3">
<div class="dkw-col dkw-pile"><span class="dkw-quiet" data-dkw-left>${pile} left</span>
<div class="dkw-draw"><span class="dkw-step" role="group" aria-label="How many to draw">
<button type="button" class="dkw-stepbtn" data-act="count-down" aria-label="Draw one fewer" disabled>−</button>
<output class="dkw-count" data-dkw-count>1</output>
<button type="button" class="dkw-stepbtn" data-act="count-up" aria-label="Draw one more">+</button></span>
<button type="button" class="btn-primary" data-act="draw" ${pile === 0 ? 'disabled' : ''}>Draw</button></div></div>
<p class="dkw-err" role="alert" data-dkw-err hidden></p></section></div>`;
const panelHTML = ({ version, name }) => `<span hidden data-dkw-version="${version}" data-dkw-deckname="${name}"></span>
<div class="dkw-rules" data-dkw-rules><p class="dkw-err" role="alert" data-dkw-err hidden></p></div>`;

const PAGE = (server) => `<!doctype html><meta charset="utf-8"><style>${CSS}
.btn-primary{padding:8px 16px;font-size:14px;line-height:20px;border:0}
.toggle{position:relative;display:inline-flex;width:44px;height:24px;border:0;padding:0}
.dkw-sentence .dw-blank{font-size:16px}</style>
<body><div class="dkw" data-widget="deck-workshop" data-campaign-id="c1" data-deck-id="d1" data-version="${server.version}" data-tab="rules">
<header class="dkw-head"><p class="dkw-crumb"><a href="/campaigns/c1/decks/workshop">Card decks</a></p>
<div class="dkw-headrow"><h1 class="dkw-h1" tabindex="-1" data-dkw-heading><input class="dkw-title" type="text" value="${server.name}" data-dkw-name aria-label="The deck's name"></h1>
<span class="dkw-saved" data-dkw-saved></span></div><p class="dkw-err" role="alert" data-dkw-err hidden></p></header>
${tableHTML(server)}
<nav class="dkw-tabs" role="tablist"><button type="button" role="tab" id="dkw-tab-rules" class="dkw-tab" aria-selected="true" data-tab="rules" data-url="/campaigns/c1/decks/workshop/d1/rules">Rules</button></nav>
<div id="dkw-panel" class="dkw-panel" role="tabpanel">${panelHTML(server)}
<p class="dkw-sentence"><select class="dw-blank is-filled" data-rule="draws"><option>only the Directors</option></select></p>
<div class="dkw-switch"><button type="button" class="toggle" role="switch" aria-checked="false" aria-label="Rare"><span class="toggle-knob"></span></button><span>Rare</span></div>
<label class="dkw-switch"><input type="checkbox" role="switch" data-chosen><span>Pip</span></label></div>
</div>
<script>
var widgets = {};
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function () { document.querySelectorAll('[data-widget]').forEach(function (el) { var d = widgets[el.getAttribute('data-widget')]; if (d && !el.__m) { el.__m = true; d.init(el); } }); },
  apiAction: function (url, o) {
    window.__record({ method: o.method, url: url });
    return fetch('/__api', { method: 'POST', body: JSON.stringify({ method: o.method, url: url }) }).then(function (r) {
      return r.json().then(function (b) { if (!r.ok) throw new Error(b.message); return b; });
    });
  },
  working: function () {}
};
</script><script>${JS}</script><script>Chronicle.mountWidgets();</script>`;

// open serves the page and a server whose answers the test can change: `server` is the deck as it is now,
// `refuse` maps a write's method and URL to the refusal it gets.
async function open(server, refuse, { width = 1280, coarse = false } = {}) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ reducedMotion: 'reduce', viewport: { width, height: 800 }, hasTouch: coarse, isMobile: coarse });
  const page = await ctx.newPage();
  page.log = [];
  await page.exposeFunction('__record', (r) => { page.log.push(r); });
  await page.route(`${ORIGIN}/**`, (r) => {
    const u = new URL(r.request().url());
    if (u.pathname === '/page.html') return r.fulfill({ contentType: 'text/html', body: PAGE(server) });
    if (u.pathname === '/__api') {
      const sent = JSON.parse(r.request().postData());
      const msg = refuse(sent);
      return r.fulfill({ status: msg ? 409 : 200, contentType: 'application/json', body: JSON.stringify(msg ? { message: msg } : {}) });
    }
    if (/\/table$/.test(u.pathname)) { page.log.push({ fetched: 'table' }); return r.fulfill({ contentType: 'text/html', body: tableHTML(server) }); }
    if (/\/rules$/.test(u.pathname)) { page.log.push({ fetched: 'rules' }); return r.fulfill({ contentType: 'text/html', body: panelHTML(server) }); }
    return r.fulfill({ status: 404, body: '' });
  });
  await page.goto(`${ORIGIN}/page.html`);
  return page;
}

test('a Draw the server refuses fetches the table row again, so the count, Draw and the line say what is true', { skip }, async () => {
  const server = { pile: 6, version: 3, name: 'Omens' };
  const page = await open(server, (s) => (/\/draw$/.test(s.url) ? 'Nothing left in this deck.' : ''));
  assert.equal((await page.textContent('[data-dkw-left]')).trim(), '6 left');
  // The pile was emptied elsewhere; only the server knows.
  server.pile = 0;
  await page.click('[data-act=draw]');
  await page.waitForFunction(() => /Nothing left in this deck/.test(document.querySelector('#dkw-table [data-dkw-err]').textContent));
  assert.deepEqual(page.log.filter((e) => e.fetched).map((e) => e.fetched), ['table'], 'the table row was not fetched again');
  assert.equal((await page.textContent('[data-dkw-left]')).trim(), '0 left');
  assert.equal(await page.isDisabled('[data-act=draw]'), true, 'Draw is still live on an empty pile');
  assert.equal(await page.evaluate(() => document.querySelector('#dkw-table [data-dkw-err]').hidden), false);
  assert.equal(await page.evaluate(() => document.activeElement && document.activeElement.closest('#dkw-table') !== null), true, 'focus fell out of the table row');
  await page.context().close();
});

test('after a save refused as stale the name field holds the newer name, and the title follows', { skip }, async () => {
  const server = { pile: 2, version: 4, name: 'Newer name' };
  // The page was opened at version 3 under the old name; the server has moved on.
  const page = await open({ ...server, version: 4 }, (s) => (s.method === 'PUT' ? 'This deck changed since you opened it. Reload and try again.' : ''));
  await page.evaluate(() => { document.querySelector('[data-dkw-name]').value = 'Old name'; document.querySelector('[data-dkw-name]').defaultValue = 'Old name'; });
  await page.fill('[data-dkw-name]', 'My late name');
  await page.dispatchEvent('[data-dkw-name]', 'change');
  await page.waitForFunction(() => /changed somewhere else/.test(document.querySelector('#dkw-panel [data-dkw-err]').textContent));
  assert.equal(await page.inputValue('[data-dkw-name]'), 'Newer name');
  assert.equal(await page.title(), 'Newer name');
  assert.deepEqual(page.log.filter((e) => e.fetched).map((e) => e.fetched).sort(), ['rules', 'table']);
  await page.context().close();
});

// A control is a tap target when its own box, or its row for a check box, is 44 square, or when a 44 square around
// its centre still lands on it.
const reach = (page, sel) => page.evaluate((s) => {
  const el = document.querySelector(s);
  const own = el.getBoundingClientRect();
  const row = el.type === 'checkbox' && el.closest('label');
  const box = (row || el).getBoundingClientRect();
  if (box.width >= 43.5 && box.height >= 43.5) return true;
  const cx = own.left + own.width / 2, cy = own.top + own.height / 2;
  return [[-1, -1], [1, -1], [-1, 1], [1, 1]].every(([dx, dy]) => {
    const hit = document.elementFromPoint(cx + dx * 21.5, cy + dy * 21.5);
    return !!hit && (el.contains(hit) || hit === el || (!!row && row.contains(hit)));
  });
}, sel);

const TARGETS = [
  ['the Draw button', '[data-act=draw]'],
  ['a stepper button', '[data-act=count-up]'],
  ['a Rules select', '.dkw-sentence .dw-blank'],
  ['the Rare switch', '.dkw-switch .toggle'],
  ['a Players check box', '.dkw-switch input[type=checkbox]'],
  ['the breadcrumb link', '.dkw-crumb a'],
  ['a tab', '.dkw-tab'],
];

test('at 390 wide the controls the look check measured are 44 by 44 tap targets', { skip }, async () => {
  const page = await open({ pile: 3, version: 1, name: 'Omens' }, () => '', { width: 390 });
  for (const [what, sel] of TARGETS) assert.equal(await reach(page, sel), true, `${what} is under 44 by 44 at 390 wide`);
  await page.context().close();
});

test('on a touch screen wider than a phone they are 44 too, and a mouse keeps the small size', { skip }, async () => {
  const touch = await open({ pile: 3, version: 1, name: 'Omens' }, () => '', { width: 1024, coarse: true });
  for (const [what, sel] of TARGETS) assert.equal(await reach(touch, sel), true, `${what} is under 44 by 44 on a touch screen`);
  await touch.context().close();

  const mouse = await open({ pile: 3, version: 1, name: 'Omens' }, () => '', { width: 1280 });
  const sizes = await mouse.evaluate(() => ({
    crumb: document.querySelector('.dkw-crumb a').getBoundingClientRect().height,
    select: document.querySelector('.dkw-sentence .dw-blank').getBoundingClientRect().height,
    draw: document.querySelector('[data-act=draw]').getBoundingClientRect().height,
  }));
  assert.ok(sizes.crumb < 30 && sizes.select < 44 && sizes.draw < 44, `a mouse's controls grew: ${JSON.stringify(sizes)}`);
  await mouse.context().close();
});
