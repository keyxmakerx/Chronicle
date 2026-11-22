// deck_new_browser.test.mjs: the New deck page's three ways to start a deck
// (internal/plugins/decks/static/js/deck_workshop.js, initNew) in a real
// browser, on markup shaped like workshop_new.templ, with the network replaced
// by a record of what was sent. It pins that the tab that is open decides what
// goes out, that the arrow keys move between tabs as they do on a deck's page,
// that each choice group takes one choice, and that what a new deck left off
// is carried to the deck's page and said once.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
let browser;
after(async () => { if (browser) await browser.close(); });

const tab = (k, name, on) => `<button type="button" role="tab" id="dkw-start-${k}" class="dkw-tab" aria-selected="${on}" aria-controls="dkw-startbody-${k}" tabindex="${on ? 0 : -1}" data-tab="${k}">${name}</button>`;
const PAGE = (opts) => `<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="file://${root}/static/css/decks_workshop.css">
<body><div class="dkw" data-widget="deck-workshop" data-dkw-new-page data-campaign-id="c1">
<form class="dkw-newform" data-dkw-new novalidate>
<input id="dkw-name" name="name" value="Omens">
<div class="dkw-chips" role="radiogroup" aria-label="Who the deck is for">
<button type="button" class="dkw-chip" role="radio" aria-checked="true" data-kind="hero">A hero</button>
<button type="button" class="dkw-chip" role="radio" aria-checked="false" data-kind="party">The party</button></div>
<div data-dkw-hero></div>
<nav class="dkw-tabs" role="tablist" aria-label="How to start the deck">${tab('style', 'A style', true)}${tab('copy', 'Copy a deck', false)}${tab('table', 'From a rolling table', false)}</nav>
<div id="dkw-startbody-style" class="dkw-panel" role="tabpanel" data-start="style">
<div class="dkw-styles" role="radiogroup" aria-label="A style to start from">
<button type="button" class="dkw-style" role="radio" aria-checked="true" data-style="plain" data-look='{"border":"double"}'>Plain</button>
<button type="button" class="dkw-style" role="radio" aria-checked="false" data-style="iron" data-look='{"border":"metal"}'>Iron</button></div></div>
<div id="dkw-startbody-copy" class="dkw-panel" role="tabpanel" data-start="copy" hidden>${opts.decks ? `<ul role="radiogroup" aria-label="A deck to copy" data-dkw-decks>
<li role="none"><button type="button" class="dkw-deck dkw-choice" role="radio" aria-checked="false" data-deck="d1">First</button></li>
<li role="none"><button type="button" class="dkw-deck dkw-choice" role="radio" aria-checked="false" data-deck="d2">Second</button></li></ul>` : '<p data-dkw-none>No decks to copy yet.</p>'}</div>
<div id="dkw-startbody-table" class="dkw-panel" role="tabpanel" data-start="table" hidden><ul role="radiogroup" aria-label="A rolling table" data-dkw-tables>
<li role="none"><button type="button" class="dkw-choice" role="radio" aria-checked="false" data-table="loot">Loot</button></li>
<li role="none"><button type="button" class="dkw-choice" role="radio" aria-checked="false" data-table="omens">Omens</button></li></ul></div>
<p class="dkw-err" role="alert" data-dkw-err hidden></p>
<div class="dkw-actions"><p data-hint="style">style hint</p><p data-hint="copy" hidden>copy hint</p><p data-hint="table" hidden>table hint</p>
<button type="submit">Make the deck</button></div>
</form></div>
<script>
var widgets = {};
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function () { document.querySelectorAll('[data-widget]').forEach(function (el) { var d = widgets[el.getAttribute('data-widget')]; if (d && !el.__m) { el.__m = true; d.init(el); } }); },
  apiAction: function (url, o) {
    window.__record({ method: o.method, url: url, body: o.body });
    if (/\\/from$/.test(url)) return Promise.resolve({ deck: { id: 'new1' }, report: { notes: ['3 lines only come up on certain days.'] } });
    return Promise.resolve({ id: 'new0' });
  },
  working: function () {}
};
</script>
<script src="file://${root}/internal/plugins/decks/static/js/deck_workshop.js"></script>
<script>Chronicle.mountWidgets();</script>`;

async function open(opts = { decks: true }) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 390, height: 800 } });
  const page = await ctx.newPage();
  // Recorded outside the page, so it survives the navigation a made deck causes.
  page.log = [];
  await page.exposeFunction('__record', (r) => { page.log.push(r); });
  await page.route('**/page.html', (r) => r.fulfill({ contentType: 'text/html', body: PAGE(opts) }));
  // A navigation after a made deck is recorded, not followed.
  await page.route('**/campaigns/**', (r) => r.fulfill({ contentType: 'text/html', body: '<!doctype html><title>deck</title>' }));
  await page.goto('file:///x/page.html');
  return page;
}
const sent = (page) => page.log.slice();
const waitSent = async (page) => { for (let i = 0; i < 100 && page.log.length < 1; i++) await new Promise((r) => setTimeout(r, 50)); assert.equal(page.log.length, 1); };
const visible = (page) => page.evaluate(() => [...document.querySelectorAll('[data-start]')].filter((p) => !p.hidden).map((p) => p.getAttribute('data-start')));

test('the open tab decides what goes out', { skip }, async () => {
  const page = await open();
  await page.click('[data-style="iron"]');
  await page.click('[type=submit]');
  await waitSent(page);
  let [s] = await sent(page);
  assert.equal(s.url, '/campaigns/c1/decks');
  assert.deepEqual(s.body, { look: { border: 'metal' }, name: 'Omens', kind: 'hero' });
  await page.context().close();

  const p2 = await open();
  await p2.click('#dkw-start-copy');
  assert.deepEqual(await visible(p2), ['copy']);
  await p2.click('[data-deck="d2"]');
  await p2.click('[type=submit]');
  await waitSent(p2);
  [s] = await sent(p2);
  assert.equal(s.url, '/campaigns/c1/decks/from');
  assert.deepEqual(s.body, { deckId: 'd2', name: 'Omens', kind: 'hero' });
  await p2.context().close();

  const p3 = await open();
  await p3.click('[data-kind="party"]');
  await p3.click('#dkw-start-table');
  await p3.click('[data-table="loot"]');
  await p3.click('[type=submit]');
  await waitSent(p3);
  [s] = await sent(p3);
  assert.deepEqual(s.body, { tableId: 'loot', name: 'Omens', kind: 'party' });
  await p3.context().close();
});

test('nothing chosen on the open tab says what to choose and sends nothing', { skip }, async () => {
  const page = await open();
  await page.click('#dkw-start-copy');
  await page.click('[type=submit]');
  assert.match(await page.textContent('[data-dkw-err]'), /Choose a deck to copy/);
  await page.click('#dkw-start-table');
  assert.equal(await page.evaluate(() => document.querySelector('[data-dkw-err]').hidden), true, 'changing tab clears the line');
  await page.click('[type=submit]');
  assert.match(await page.textContent('[data-dkw-err]'), /Choose a rolling table/);
  assert.deepEqual(await sent(page), []);
  await page.context().close();
});

test('arrow keys, Home and End move between the tabs and show that tab', { skip }, async () => {
  const page = await open();
  await page.focus('#dkw-start-style');
  const state = () => page.evaluate(() => ({
    focus: document.activeElement.id,
    selected: [...document.querySelectorAll('[role=tab]')].filter((t) => t.getAttribute('aria-selected') === 'true').map((t) => t.dataset.tab),
    tabbable: [...document.querySelectorAll('[role=tab]')].filter((t) => t.tabIndex === 0).map((t) => t.dataset.tab),
  }));
  for (const [key, want] of [['ArrowRight', 'copy'], ['ArrowRight', 'table'], ['ArrowRight', 'style'], ['ArrowLeft', 'table'], ['Home', 'style'], ['End', 'table']]) {
    await page.keyboard.press(key);
    const s = await state();
    assert.deepEqual(s, { focus: 'dkw-start-' + want, selected: [want], tabbable: [want] }, key);
    assert.deepEqual(await visible(page), [want], key);
  }
  await page.context().close();
});

test('each choice group takes one choice, and a keyboard alone can make a deck', { skip }, async () => {
  const page = await open();
  await page.click('#dkw-start-copy');
  await page.click('[data-deck="d1"]');
  await page.click('[data-deck="d2"]');
  assert.deepEqual(await page.evaluate(() => [...document.querySelectorAll('[data-deck]')].map((b) => b.getAttribute('aria-checked'))), ['false', 'true']);
  await page.context().close();

  // Keyboard only: Tab to the strip, arrow to Copy a deck, Tab into the group, arrow, Space, then Enter in the name field.
  const k = await open();
  await k.focus('#dkw-start-style');
  await k.keyboard.press('ArrowRight');
  await k.keyboard.press('Tab');
  assert.equal(await k.evaluate(() => document.activeElement.getAttribute('data-deck')), 'd1');
  await k.keyboard.press('ArrowDown');
  assert.equal(await k.evaluate(() => document.activeElement.getAttribute('data-deck')), 'd2');
  await k.keyboard.press('Space');
  assert.equal(await k.evaluate(() => document.querySelector('[data-deck="d2"]').getAttribute('aria-checked')), 'true');
  await k.focus('#dkw-name');
  await k.keyboard.press('Enter');
  await waitSent(k);
  assert.equal((await sent(k))[0].body.deckId, 'd2');
  await k.context().close();
});

test('what a new deck left off is carried to its page and said once', { skip }, async () => {
  const page = await open();
  await page.click('#dkw-start-copy');
  await page.click('[data-deck="d1"]');
  await page.click('[type=submit]');
  await page.waitForFunction(() => /decks\/workshop\/new1/.test(location.href));
  const stored = await page.evaluate(() => sessionStorage.getItem('dkw-left-new1'));
  assert.deepEqual(JSON.parse(stored), ['3 lines only come up on certain days.']);
  await page.context().close();
});
