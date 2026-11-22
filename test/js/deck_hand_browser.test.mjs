// deck_hand_browser.test.mjs: the player's hand widget
// (internal/plugins/decks/static/js/deck_hand.js) in a real browser, on
// fragments written the way hand*.templ writes them, with the network
// replaced by a table of answers. It pins what the contract asks of the page:
// a face is requested only by a tap, once, through the one request that turns
// the copy; a refused turn leaves the card face down and says why; a live
// message fetches the hand again, once for a burst; the opened card's
// controls, and what each does; a turned-down ask is said once.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const decks = join(root, 'internal', 'plugins', 'decks', 'static', 'js');
let browser;
after(async () => { if (browser) await browser.close(); });

const BASE = '/campaigns/c1/decks';
const down = (copy) => `<div class="dkh-slot" data-dkh-slot data-copy="${copy}" data-deck="d1" data-deck-name="Omens">
<div class="dk-wrap dk-sz-hand dk-s-tall dk-t-flip" data-dk-card data-widget="deck-cards" data-turn="flip" data-shape="tall" data-state="down">
<span class="dk-turner"><div class="dk-card dk-back dk-s-tall" aria-hidden="true"></div><span class="dk-faceslot"></span></span>
<button type="button" class="dk-turn" aria-label="Turn over card 1 of 1, Omens" data-face-src=""></button>
<span class="dk-sr dk-live" role="status" aria-live="polite"></span></div><span class="dkh-chip dkh-chip-new">New</span></div>`;
const up = (copy, chip = '') => `<div class="dkh-slot" data-dkh-slot data-copy="${copy}" data-deck="d1" data-deck-name="Omens">
<div class="dk-wrap dk-sz-hand dk-s-tall" data-dk-card data-state="up"><div class="dk-card dk-plate dk-s-tall" aria-label="Moon, Bane"><b class="dk-name">Moon</b></div></div>
<button type="button" class="dkh-hit" data-dkh-open aria-label="Open Moon"></button>${chip}</div>`;
const rowOf = (slots, held) => `<div class="dkh-row"${held ? ' data-dkh-held' : ''}>${slots.map((s) => `<div class="dkh-item">${s}</div>`).join('')}</div>`;
// bodyOf draws a hand (held) and, after it, cards lying in play.
const bodyOf = (held, play = []) => `<div class="dkh" data-widget="deck-hand" data-dkh-root data-src="${BASE}/hand" data-base="${BASE}" data-campaign-id="c1">
<div class="dkh-body" data-dkh-body>${rowOf(held, true)}${play.length ? rowOf(play, false) : ''}</div>
<div class="dkh-notes" data-dkh-notes role="status"></div><p class="dkh-err" role="alert" data-dkh-err hidden></p></div>`;
const body = (...slots) => bodyOf(slots);
const FACE = '<div class="dk-card dk-plate dk-s-tall" aria-label="Moon, Bane"><b class="dk-name">Moon</b></div>';
const opened = ({ ask = false, waiting = false, shown = false, discard = false } = {}) => `<div class="dkh-over" role="dialog" data-dkh-over data-copy="c1" data-deck="d1"><div class="dkh-sheet">
<button type="button" class="dkh-x" data-dkh-close aria-label="Close this card">x</button>
<div class="dkh-big" data-dkh-big><div class="dk-wrap dk-sz-open" data-dk-card data-state="up"><div class="dk-card dk-plate">Moon</div></div></div>
<p class="dkh-state" data-dkh-state>${waiting ? 'Waiting for your Director' : shown ? 'Shown to the table' : ''}</p>
<div class="dkh-acts">${waiting ? '' : `<button type="button" class="dkh-btn" data-dkh-act="play" ${ask ? 'data-dkh-ask' : ''}>${ask ? 'Ask to play' : 'Play it'}</button>`}
${shown ? '' : '<button type="button" class="dkh-btn" data-dkh-act="show">Show the table</button>'}
${discard ? '<button type="button" class="dkh-btn" data-dkh-act="discard">Throw it away</button>' : ''}
<button type="button" class="dkh-btn" data-dkh-close>Keep it</button></div><p class="dkh-err" role="alert" data-dkh-err hidden></p></div></div>`;

const PAGE = `<!doctype html><meta charset="utf-8"><link rel="stylesheet" href="file://${root}/static/css/decks_card.css"><link rel="stylesheet" href="file://${root}/static/css/decks_hand.css">
<body><div id="host">__BODY__</div>
<script>
window.__calls = []; window.__answers = {};
window.fetch = function (url, opts) {
  opts = opts || {};
  var key = (opts.method || 'GET') + ' ' + String(url).replace(/^file:\\/\\/\\/?/, '/');
  window.__calls.push(key + (opts.body ? ' ' + opts.body : ''));
  var a = window.__answers[key];
  if (typeof a === 'function') a = a();
  if (!a) a = { status: 404, body: '{"message":"not found"}' };
  return Promise.resolve(new Response(a.body, { status: a.status || 200 }));
};
var widgets = {};
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; Chronicle.mountWidgets(document, n); },
  mountWidgets: function (r, only) {
    Object.keys(widgets).forEach(function (n) {
      if (only && n !== only) return;
      (r.querySelectorAll ? r : document).querySelectorAll('[data-widget="' + n + '"]').forEach(function (el) {
        if (el.__m) return; el.__m = true; widgets[n].init(el);
      });
    });
  },
  apiFetch: function (url, o) { o = o || {}; return fetch(url, { method: o.method, headers: o.headers, body: o.body && typeof o.body === 'object' ? JSON.stringify(o.body) : o.body }); },
  working: function (b, on) { if (on) b.setAttribute('data-working', ''); else b.removeAttribute('data-working'); }
};
Object.defineProperty(navigator, 'hardwareConcurrency', { get: function () { return 8; } });
</script>
<script src="file://${root}/internal/plugins/decks/static/js/deck_cards.js"></script>
<script src="file://${root}/internal/plugins/decks/static/js/deck_hand.js"></script>
<script>Chronicle.mountWidgets(document);</script>`;

async function open(html, answers = {}) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 390, height: 800 } });
  const page = await ctx.newPage();
  await page.route('**/page.html', (r) => r.fulfill({ contentType: 'text/html', body: PAGE.replace('__BODY__', html) }));
  await page.goto('file:///x/page.html');
  await page.evaluate((a) => { Object.assign(window.__answers, a); }, answers);
  return page;
}
const calls = (page) => page.evaluate(() => window.__calls.slice());
const html200 = (b) => ({ status: 200, body: b });

test('a face is asked for only by a tap, once, through the turn request', { skip }, async () => {
  const page = await open(body(down('c1')), { [`POST ${BASE}/hand/d1/copies/c1/turn`]: html200(FACE) });
  await page.waitForTimeout(300);
  assert.deepEqual(await calls(page), [], 'the page asked for something before any tap');
  assert.equal(await page.evaluate(() => document.body.innerHTML.includes('Moon')), false, 'the face is in the page before the tap');
  await page.click('.dk-turn');
  await page.waitForFunction(() => document.querySelector('[data-dk-card]').getAttribute('data-state') === 'up');
  const made = (await calls(page)).filter((c) => c.startsWith('POST'));
  assert.deepEqual(made, [`POST ${BASE}/hand/d1/copies/c1/turn`]);
  assert.match(await page.evaluate(() => document.body.textContent), /Moon/);
  await page.context().close();
});

test('a refused turn leaves the card face down, says why and puts focus back', { skip }, async () => {
  const page = await open(body(down('c1')), { [`POST ${BASE}/hand/d1/copies/c1/turn`]: { status: 409, body: '{"message":"that card is already face up"}' } });
  await page.click('.dk-turn');
  await page.waitForSelector('[data-dkh-err]:not([hidden])');
  const r = await page.evaluate(() => ({
    state: document.querySelector('[data-dk-card]').getAttribute('data-state'),
    err: document.querySelector('[data-dkh-err]').textContent,
    enabled: !document.querySelector('.dk-turn').disabled,
    focused: document.activeElement === document.querySelector('.dk-turn'),
  }));
  assert.equal(r.state, 'down');
  assert.match(r.err, /already face up/);
  assert.ok(r.enabled && r.focused, JSON.stringify(r));
  await page.context().close();
});

test('a burst of live messages fetches the hand once, and the page shows the answer', { skip }, async () => {
  const page = await open(body(down('c1')), { [`GET ${BASE}/hand`]: html200(body(down('c1'), down('c2'))) });
  await page.evaluate(() => { for (let i = 0; i < 5; i++) window.dispatchEvent(new CustomEvent('chronicle:decks-changed')); });
  await page.waitForFunction(() => document.querySelectorAll('[data-dkh-slot]').length === 2);
  assert.deepEqual((await calls(page)).filter((c) => c === `GET ${BASE}/hand`).length, 1);
  await page.context().close();
});

test('the opened card has the controls the server drew, and Keep it changes nothing', { skip }, async () => {
  const page = await open(body(up('c1')), { [`GET ${BASE}/hand/d1/copies/c1`]: html200(opened({ discard: true })) });
  await page.click('[data-dkh-open]');
  await page.waitForSelector('[data-dkh-over]');
  const names = await page.$$eval('.dkh-acts button', (bs) => bs.map((b) => b.textContent.trim()));
  assert.deepEqual(names, ['Play it', 'Show the table', 'Throw it away', 'Keep it']);
  const before = await calls(page);
  await page.click('.dkh-acts [data-dkh-close]');
  assert.equal(await page.$('[data-dkh-over]'), null);
  assert.deepEqual(await calls(page), before, 'Keep it sent something');
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-dkh-open')), true, 'focus did not return to the card');
  await page.context().close();
});

test('asking sends the play verb, then the card says it is waiting and cannot be asked again', { skip }, async () => {
  const page = await open(body(up('c1')), { [`GET ${BASE}/hand/d1/copies/c1`]: html200(opened({ ask: true })) });
  await page.click('[data-dkh-open]');
  await page.waitForSelector('[data-dkh-act="play"]');
  assert.equal((await page.textContent('[data-dkh-act="play"]')).trim(), 'Ask to play');
  // The server's answer to the open request now says waiting.
  await page.evaluate(({ b, waiting }) => {
    window.__answers['POST ' + b + '/d1/copies/c1/play'] = { status: 200, body: '{}' };
    window.__answers['GET ' + b + '/hand/d1/copies/c1'] = { status: 200, body: waiting };
    window.__answers['GET ' + b + '/hand'] = { status: 200, body: document.querySelector('#host').innerHTML };
  }, { b: BASE, waiting: opened({ ask: true, waiting: true }) });
  await page.click('[data-dkh-act="play"]');
  await page.waitForFunction(() => /Waiting for your Director/.test(document.querySelector('[data-dkh-state]').textContent));
  assert.equal(await page.$('[data-dkh-act="play"]'), null, 'the card can be asked again');
  assert.ok((await calls(page)).includes(`POST ${BASE}/d1/copies/c1/play {}`));
  await page.context().close();
});

test('a refused move changes nothing and says why', { skip }, async () => {
  const page = await open(body(up('c1')), {
    [`GET ${BASE}/hand/d1/copies/c1`]: html200(opened()),
    [`POST ${BASE}/d1/copies/c1/show`]: { status: 409, body: '{"message":"this deck does not let you show cards"}' },
  });
  await page.click('[data-dkh-open]');
  await page.click('[data-dkh-act="show"]');
  await page.waitForSelector('.dkh-sheet [data-dkh-err]:not([hidden])');
  assert.match(await page.textContent('.dkh-sheet [data-dkh-err]'), /does not let you show/);
  assert.equal(await page.evaluate(() => document.querySelector('[data-dkh-state]').textContent.trim()), '');
  assert.equal(await page.evaluate(() => document.activeElement.getAttribute('data-dkh-act')), 'show');
  await page.context().close();
});

test('a play a Director let through is not read as turned down', { skip }, async () => {
  const waitingChip = '<span class="dkh-chip dkh-chip-wait">Waiting</span>';
  // The card keeps its id and lands in play in front of the hero.
  const page = await open(body(up('c1', waitingChip)), { [`GET ${BASE}/hand`]: html200(bodyOf([], [up('c1')])) });
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await page.waitForFunction(() => !document.querySelector('.dkh-chip-wait'));
  await page.waitForTimeout(300);
  assert.equal(await page.$$eval('[data-dkh-notes] .dkh-note', (n) => n.length), 0, 'an approved play was called a refusal');
  await page.context().close();
});

test('a turned-down ask is said once and the card is askable again', { skip }, async () => {
  const waitingChip = '<span class="dkh-chip dkh-chip-wait">Waiting</span>';
  const page = await open(body(up('c1', waitingChip)), { [`GET ${BASE}/hand`]: html200(body(up('c1'))) });
  const fire = () => page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await fire();
  await page.waitForSelector('[data-dkh-notes] .dkh-note');
  await page.waitForTimeout(300);
  await fire();
  await page.waitForTimeout(400);
  assert.equal(await page.$$eval('[data-dkh-notes] .dkh-note', (n) => n.length), 1, 'said more than once');
  assert.match(await page.textContent('[data-dkh-notes]'), /turned that down/);
  assert.equal(await page.$$eval('.dkh-chip-wait', (n) => n.length), 0);
  await page.context().close();
});

// A hand the way hand_panel.templ writes it: the section's label, the fanned
// row, and the one alert at the end. `after` is whatever follows the row.
const handOf = (n, { after = '', label = 'Your hand' } = {}) => `<div class="dkh" data-widget="deck-hand" data-dkh-root data-src="${BASE}/hand" data-base="${BASE}" data-campaign-id="c1">
<div class="dkh-body" data-dkh-body><section class="dkh-sec"><h3 class="dkh-lab">${label}</h3>
<div class="dkh-row dkh-fan" role="list" data-dkh-held>${Array.from({ length: n }, (_, i) => `<div class="dkh-item" role="listitem">${down('c' + (i + 1))}</div>`).join('')}</div>${after}</section></div>
<div class="dkh-notes" data-dkh-notes role="status"></div><p class="dkh-err" role="alert" data-dkh-err hidden></p></div>`;
const SPACER = '<div style="height:1400px"></div>';

test('an offline tap says its line right under the row, inside the screen, and a refetch leaves one line', { skip }, async () => {
  const page = await open(handOf(12, { after: SPACER }), { [`POST ${BASE}/hand/d1/copies/c3/turn`]: { status: 500, body: '' } });
  await page.click('[data-copy="c3"] .dk-turn');
  await page.waitForSelector('[data-dkh-err]:not([hidden])');
  const r = await page.evaluate(() => {
    const e = document.querySelector('[data-dkh-err]'), b = e.getBoundingClientRect();
    return { n: document.querySelectorAll('[data-dkh-err]').length, role: e.getAttribute('role'), text: e.textContent, under: e.previousElementSibling === document.querySelector('.dkh-row'), top: b.top, bottom: b.bottom, h: innerHeight };
  });
  assert.equal(r.n, 1);
  assert.equal(r.role, 'alert');
  assert.match(r.text, /That did not go through\. Check your connection and try again\./);
  assert.ok(r.under, 'the line is not directly under the row that holds the tapped card');
  assert.ok(r.top >= 0 && r.bottom <= r.h, `the line is off screen: ${r.top}..${r.bottom} of ${r.h}`);
  // The hand is read again: the line is not stranded in the body that was replaced, nor twice.
  await page.evaluate(([b, html]) => { window.__answers['GET ' + b + '/hand'] = { status: 200, body: html }; }, [BASE, handOf(12, { after: SPACER })]);
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await page.waitForFunction(() => window.__calls.some((c) => c === 'GET /campaigns/c1/decks/hand'));
  await page.waitForTimeout(400);
  const after = await page.evaluate(() => ({ n: document.querySelectorAll('[data-dkh-err]').length, inBody: !!document.querySelector('[data-dkh-body] [data-dkh-err]'), home: document.querySelector('[data-dkh-err]').parentNode.hasAttribute('data-dkh-root') }));
  assert.equal(after.n, 1, 'the alert was duplicated or lost');
  assert.equal(after.inBody, false, 'the alert is stranded in the redrawn body');
  assert.ok(after.home);
  // The next tap that fails says it again, under the row.
  await page.click('[data-copy="c5"] .dk-turn');
  await page.waitForSelector('[data-dkh-err]:not([hidden])');
  assert.equal(await page.evaluate(() => document.querySelector('[data-dkh-err]').previousElementSibling === document.querySelector('.dkh-row')), true);
  await page.context().close();
});

test('a line said below the fold is brought into view', { skip }, async () => {
  const page = await open(handOf(4, { after: SPACER }), { [`GET ${BASE}/hand`]: { status: 500, body: '' } });
  // A refresh that fails says its line at the alert's own place, far down the page.
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await page.waitForSelector('[data-dkh-err]:not([hidden])');
  await page.waitForTimeout(100);
  const r = await page.evaluate(() => { const b = document.querySelector('[data-dkh-err]').getBoundingClientRect(); return { top: b.top, bottom: b.bottom, h: innerHeight }; });
  assert.ok(r.top >= 0 && r.bottom <= r.h, `the line is off screen: ${JSON.stringify(r)}`);
  await page.context().close();
});

test('a hand of 12 on a phone scrolls inside its row, fades where more waits and counts in its label', { skip }, async () => {
  const page = await open(handOf(12), {});
  const info = () => page.evaluate(() => {
    const row = document.querySelector('.dkh-row');
    return { more: row.getAttribute('data-dkh-more'), sw: row.scrollWidth, cw: row.clientWidth, x: row.scrollLeft, label: document.querySelector('.dkh-lab').textContent.replace(/\s+/g, ' ').trim(), mask: getComputedStyle(row).maskImage, page: document.documentElement.scrollWidth - innerWidth };
  });
  let r = await info();
  assert.ok(r.sw > r.cw + 100, `the row does not overflow: ${r.sw} in ${r.cw}`);
  assert.equal(r.more, 'end');
  assert.match(r.mask, /gradient/, 'no fade on the trailing edge');
  assert.equal(r.label, 'Your hand · 12');
  assert.ok(r.page <= 0, `the page scrolls sideways by ${r.page}px`);
  // Touch and keyboard both move along the row, and the leading edge fades once it has.
  await page.focus('[data-copy="c12"] .dk-turn');
  await page.waitForFunction(() => document.querySelector('.dkh-row').getAttribute('data-dkh-more') === 'start');
  r = await info();
  assert.ok(r.x > 0, 'focus at the last card did not scroll the row');
  await page.evaluate(() => { const row = document.querySelector('.dkh-row'); row.scrollLeft = (row.scrollWidth - row.clientWidth) / 2; });
  await page.waitForFunction(() => document.querySelector('.dkh-row').getAttribute('data-dkh-more') === 'both');
  assert.ok((await info()).page <= 0);
  await page.context().close();
});

test('a small hand keeps its fan with no fade and no count, and a live redraw adds the count when the hand grows', { skip }, async () => {
  const page = await open(handOf(3), { [`GET ${BASE}/hand`]: html200(handOf(12)) });
  const r = await page.evaluate(() => ({ more: document.querySelector('.dkh-row').hasAttribute('data-dkh-more'), label: document.querySelector('.dkh-lab').textContent.trim() }));
  assert.equal(r.more, false);
  assert.equal(r.label, 'Your hand');
  await page.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:decks-changed')));
  await page.waitForFunction(() => document.querySelectorAll('[data-dkh-slot]').length === 12);
  await page.waitForFunction(() => /Your hand · 12/.test(document.querySelector('.dkh-lab').textContent.replace(/\s+/g, ' ')));
  assert.equal(await page.evaluate(() => document.querySelectorAll('[data-dkh-count]').length), 1);
  await page.context().close();
});

test('the opened card is dimmed by the site scrim, not a mix of the text colour', { skip }, async () => {
  const page = await open(body(up('c1')), { [`GET ${BASE}/hand/d1/copies/c1`]: html200(opened()) });
  await page.addStyleTag({ content: ':root { --surface-overlay: rgb(6 7 11 / 0.55); }' });
  await page.click('[data-dkh-open]');
  await page.waitForSelector('[data-dkh-over]');
  assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('[data-dkh-over]')).backgroundColor), 'rgba(6, 7, 11, 0.55)');
  await page.context().close();
});
