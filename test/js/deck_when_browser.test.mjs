// deck_when_browser.test.mjs: the "Comes up when" editor of the Cards tab
// (internal/plugins/decks/static/js/deck_workshop.js) in a real browser, on
// the section the server renders and the choices it sends, with the network
// replaced by a table of answers. It pins what the editor asks of the page:
// a row of every kind of test is built from the choices, a changed blank goes
// out as one card update carrying only that kind's keys, a refused save is
// said in the row that caused it and the row goes back to what is stored.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
let browser;
after(async () => { if (browser) await browser.close(); });

const OPTIONS = {
  heroes: [{ id: 'h-tam', name: 'Tam' }, { id: 'h-ilse', name: 'Ilse' }],
  fields: [{ key: 'hp', label: 'Hit points', maxKey: 'hpmax' }, { key: 'hpmax', label: 'Max hit points' }],
  trackers: [{ id: 'k-doom', name: 'Doom' }],
  tags: [{ id: 7, name: 'Cursed' }],
  layers: ['Haunted'],
  available: true, maxTests: 4,
  ops: [{ key: 'lte', phrase: 'at or below' }, { key: 'gte', phrase: 'at or above' }],
  shares: [{ key: 'half', phrase: 'half of' }, { key: 'all', phrase: '' }],
  testKinds: [{ key: 'number', phrase: 'a number on the page' }, { key: 'tracker', phrase: 'a tracker on the page' },
    { key: 'tag', phrase: 'a tag on the page' }, { key: 'layer', phrase: 'a transformation on the page' }, { key: 'say', phrase: 'the Director says so' },
    { key: 'moon', phrase: 'a moon' }, { key: 'event', phrase: 'an event today' }, { key: 'time', phrase: 'the time of day' }],
  hasCalendar: true, moons: ['Nyx'], eventKinds: [{ slug: 'blood', name: 'Bloodnight' }],
  dayParts: [{ key: 'dawn', phrase: 'dawn' }, { key: 'day', phrase: 'day' }, { key: 'dusk', phrase: 'dusk' }, { key: 'night', phrase: 'night' }],
  moonPhases: [{ key: 'full', phrase: 'full' }, { key: 'new', phrase: 'new' }],
};
const STORED = [
  { test: 'number', open: true, who: 'holder', field: 'hp', op: 'gte', value: 1, sentence: 'Tam’s Hit points is at or above 1', broken: false },
  { test: 'tag', open: false, who: 'holder', tag: 7, has: true, sentence: 'Tam has the tag Cursed', broken: false },
];
const BROKEN = [{ test: 'tag', open: false, who: 'holder', tag: 99, has: true, sentence: 'Tam has the tag that was deleted', broken: true }];

const PAGE = (when) => `<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="file://${root}/static/css/decks_workshop.css">
<body><div data-widget="deck-workshop" data-campaign-id="c1" data-deck-id="d1" data-version="3">
<div id="dkw-panel"><div data-dkw-editor data-card-id="card1">
<section data-dkw-when data-when='${JSON.stringify(when)}' data-state='{"fitsNow":false,"whyNot":"Tam is above half"}' data-hero-deck="true" data-holder="h-tam" data-holder-name="Tam">
<h3>Comes up when</h3><ol data-dkw-whens></ol><div data-dkw-whenadd></div><p data-dkw-now role="status"></p><p class="dkw-err" data-dkw-when-err hidden></p></section>
</div></div></div>
<script>
window.__puts = []; window.__answer = null;
var widgets = {};
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function (r) { document.querySelectorAll('[data-widget]').forEach(function (el) { var d = widgets[el.getAttribute('data-widget')]; if (d && !el.__m) { el.__m = true; d.init(el); } }); },
  apiAction: function (url, o) {
    window.__puts.push({ method: o.method, url: url, body: o.body });
    if (window.__answer && window.__answer.fail) return Promise.reject(new Error(window.__answer.fail));
    return Promise.resolve(window.__answer && window.__answer.deck || { version: 4, cards: [] });
  },
  working: function () {}
};
window.fetch = function (url) {
  var u = String(url);
  if (/when-options/.test(u)) return Promise.resolve(new Response(JSON.stringify(${JSON.stringify(OPTIONS)}), { status: 200 }));
  return Promise.resolve(new Response('', { status: 200 }));
};
</script>
<script src="file://${root}/internal/plugins/decks/static/js/deck_workshop.js"></script>
<script>Chronicle.mountWidgets(document);</script>`;

async function open(when) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 390, height: 800 } });
  const page = await ctx.newPage();
  await page.route('**/page.html', (r) => r.fulfill({ contentType: 'text/html', body: PAGE(when) }));
  await page.goto('file:///x/page.html');
  await page.waitForSelector('.dkw-addsel');
  return page;
}
const puts = (page) => page.evaluate(() => window.__puts.slice());
const blankValue = (page, row, label) => page.evaluate(([r, l]) => document.querySelector(`[data-when-row="${r}"] [aria-label="${l}"]`).value, [row, label]);

test('the stored tests are drawn as rows with their marks, the hint and the Now line', { skip }, async () => {
  const page = await open(STORED);
  const r = await page.evaluate(() => ({
    rows: [...document.querySelectorAll('[data-when-row]')].map((n) => n.querySelector('.dkw-mark').textContent),
    hint: document.querySelector('.dkw-whenhint').textContent,
    add: document.querySelector('.dkw-addsel').getAttribute('aria-label'),
    now: document.querySelector('[data-dkw-now]').textContent,
  }));
  assert.deepEqual(r.rows, ['On the card', 'Only you']);
  assert.equal(r.hint, 'Every condition must hold. Up to four.');
  assert.equal(r.add, 'And another condition');
  assert.match(r.now, /Waiting: Tam is above half\./);
  assert.equal(await blankValue(page, 0, 'How it compares'), 'gte');
  assert.equal(await page.evaluate(() => document.querySelector('[aria-label="The number"]').value), '1');
  await page.context().close();
});

test('a changed blank goes out as one card update with only its kind’s keys', { skip }, async () => {
  const page = await open(STORED);
  await page.selectOption('[data-when-row="0"] [aria-label="How it compares"]', 'lte');
  await page.waitForFunction(() => window.__puts.length === 1);
  const [put] = await puts(page);
  assert.equal(put.method, 'PUT');
  assert.equal(put.url, '/campaigns/c1/decks/d1/cards/card1');
  assert.deepEqual(put.body, { when: [
    { test: 'number', open: true, who: 'holder', field: 'hp', op: 'lte', value: 1 },
    { test: 'tag', open: false, who: 'holder', tag: 7, has: true },
  ] });
  await page.context().close();
});

test('a typed number saves, and text that is not a number says so and saves nothing', { skip }, async () => {
  const page = await open(STORED);
  await page.fill('[aria-label="The number"]', 'lots');
  await page.press('[aria-label="The number"]', 'Tab');
  await page.waitForFunction(() => /Type a number/.test(document.querySelector('[data-when-row="0"]').textContent));
  assert.deepEqual(await puts(page), []);
  await page.fill('[aria-label="The number"]', '2,5');
  await page.press('[aria-label="The number"]', 'Tab');
  await page.waitForFunction(() => window.__puts.length === 1);
  assert.equal((await puts(page))[0].body.when[0].value, 2.5);
  await page.context().close();
});

test('every kind can be added and sends only its own keys', { skip }, async () => {
  const want = {
    number: { test: 'number', open: false, who: 'holder', field: 'hp', op: 'lte', of: 'hpmax', share: 'half' },
    tracker: { test: 'tracker', open: false, who: 'holder', kind: 'k-doom', op: 'gte', value: 1 },
    tag: { test: 'tag', open: false, who: 'holder', tag: 7, has: true },
    layer: { test: 'layer', open: false, who: 'holder', name: 'Haunted', on: true },
    say: { test: 'say', open: false },
    moon: { test: 'moon', open: false, name: 'Nyx', phase: 'full', is: true },
    event: { test: 'event', open: false, name: 'Bloodnight', has: true },
    time: { test: 'time', open: false, time: 'night', is: true },
  };
  for (const [kind, body] of Object.entries(want)) {
    const page = await open([]);
    await page.selectOption('.dkw-addsel', kind);
    await page.waitForFunction(() => window.__puts.length === 1);
    assert.deepEqual((await puts(page))[0].body, { when: [body] }, kind);
    await page.context().close();
  }
});

test('a moon test turns from a phase to days of its cycle, and a day must be whole', { skip }, async () => {
  const moon = [{ test: 'moon', open: true, name: 'Nyx', phase: 'full', is: true, sentence: 'Nyx is full', broken: false }];
  const page = await open(moon);
  assert.equal(await page.evaluate(() => document.querySelector('[aria-label="Whose page"]')), null, 'a calendar test names no page');
  await page.selectOption('[data-when-row="0"] [aria-label="Which phase"]', 'days');
  await page.waitForFunction(() => window.__puts.length === 1);
  assert.deepEqual((await puts(page))[0].body, { when: [{ test: 'moon', open: true, name: 'Nyx', from: 1, to: 1, is: true }] });
  await page.context().close();
  const days = await open([{ test: 'moon', open: false, name: 'Nyx', from: 3, to: 5, is: false, sentence: 'Nyx is not on day 3 to 5 of its cycle', broken: false }]);
  assert.equal(await blankValue(days, 0, 'Which phase'), 'days');
  assert.equal(await blankValue(days, 0, 'Is or is not'), 'false');
  await days.fill('[aria-label="The last day"]', '2.5');
  await days.press('[aria-label="The last day"]', 'Tab');
  await days.waitForFunction(() => /a day of the cycle/.test(document.querySelector('[data-when-row="0"]').textContent));
  assert.deepEqual(await puts(days), []);
  await days.fill('[aria-label="The last day"]', '1');
  await days.press('[aria-label="The last day"]', 'Tab');
  await days.waitForFunction(() => window.__puts.length === 1);
  assert.deepEqual((await puts(days))[0].body.when[0], { test: 'moon', open: false, name: 'Nyx', from: 3, to: 1, is: false });
  await days.context().close();
});

test('a refused save is said in the row and the row goes back to what is stored', { skip }, async () => {
  const page = await open(STORED);
  await page.evaluate(() => { window.__answer = { fail: 'Hit points cannot be compared with that.' }; });
  await page.selectOption('[data-when-row="0"] [aria-label="How it compares"]', 'lte');
  await page.waitForFunction(() => /cannot be compared/.test(document.querySelector('[data-when-row="0"] [data-dkw-err]').textContent));
  assert.equal(await blankValue(page, 0, 'How it compares'), 'gte', 'the row shows the stored test, not the refused one');
  assert.equal(await page.evaluate(() => document.querySelector('[data-when-row="1"] [data-dkw-err]').hidden), true);
  await page.context().close();
});

test('a test that points at nothing says so, and can be removed', { skip }, async () => {
  const page = await open(BROKEN);
  assert.match(await page.evaluate(() => document.querySelector('[data-when-row="0"]').textContent), /This no longer points at anything/);
  await page.click('[data-when-row="0"] [data-act="when-remove"]');
  await page.waitForFunction(() => window.__puts.length === 1);
  assert.deepEqual((await puts(page))[0].body, { when: [] });
  await page.context().close();
});

test('while a test that points at nothing is in the list, a refused change says to remove it first', { skip }, async () => {
  const page = await open([STORED[0], BROKEN[0]]);
  await page.evaluate(() => { window.__answer = { fail: 'test 2: that tag is not in this campaign' }; });
  await page.selectOption('[data-when-row="0"] [aria-label="How it compares"]', 'lte');
  await page.waitForFunction(() => !document.querySelector('[data-when-row="0"] [data-dkw-err]').hidden);
  assert.equal(await page.evaluate(() => document.querySelector('[data-when-row="0"] [data-dkw-err]').textContent),
    'A condition here no longer points at anything. Remove it, then change the others.');
  await page.context().close();
});
