// calendar_outcome_browser.test.mjs — what became of the events once
// today moves past them.
//
// Runs calendar_view.js in a real Chromium as the campaign owner against a
// fake server. Moving today forward over events fetches the sheet and asks
// about each passed event: Happened is picked to start with, Didn't and
// Moved (with the new day) are a press away, and a one-off event takes a
// line. Done sends the one-off answers as one batch and a repeating date as
// its override, and Undo sends them back. Not now and Escape send nothing.
// The switch in Calendar settings, a step back in time and a player never
// ask. The month shows a called-off event struck through and a moved one's
// dashed mark on its old day; the event's card changes what became of it.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const API = '/campaigns/c1/calendars/k1';

const calendar = (over = {}) => ({
  id: 'k1', campaign_id: 'c1', mode: 'fantasy', name: 'Therin',
  current_year: 100, current_month: 1, current_day: 9, current_hour: 8, current_minute: 0,
  hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60, ask_when_day_passes: true,
  months: ['Firstfrost', 'Deepwinter', 'Thaw'].map((name, i) => ({ id: i + 1, name, days: 30, sort_order: i, leap_year_days: 0 })),
  weekdays: ['Hearthday', 'Forgeday', 'Veilday', 'Wayday', 'Halfday', 'Hushday', 'Restday'].map((name) => ({ name })),
  moons: [], seasons: [], eras: [],
  ...over,
});

// What GET .../events/passed answers: three one-off events (the first over
// two days) and one date of a repeating event.
const PASSED = [
  { id: 'e1', name: 'Journey through the dry country', visibility: 'dm_only', year: 100, month: 1, day: 9, end_year: 100, end_month: 1, end_day: 10, repeating: false },
  { id: 'e2', name: 'Ambush at the dry well', visibility: 'dm_only', year: 100, month: 1, day: 10, repeating: false },
  { id: 'e3', name: 'Arrival at Lastwell', visibility: 'everyone', year: 100, month: 1, day: 11, repeating: false, outcome: 'Expected at dusk' },
  { id: 'e4', name: 'Sanguin’mor bites the sun', visibility: 'everyone', year: 100, month: 1, day: 11, repeating: true },
];

// The month the page opens on (and reads again after a save): a called-off
// event, a moved one and a planned one.
const MONTH = [
  { id: 'm1', name: 'Ambush at the dry well', visibility: 'everyone', year: 100, month: 1, day: 10, status: 'called_off', outcome: 'Foiled by the party' },
  { id: 'm2', name: 'Arrival at Lastwell', visibility: 'everyone', year: 100, month: 1, day: 12, status: 'moved', moved_from_year: 100, moved_from_month: 1, moved_from_day: 11 },
  { id: 'm3', name: 'Market day', visibility: 'everyone', year: 100, month: 1, day: 20, status: 'planned' },
];

const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const html = (role, cal, month) => `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<link rel="stylesheet" href="/static/css/calendar-view.css">
<style>html,body{margin:0}</style></head><body>
<main id="main-content"><div style="max-width:64rem;margin:0 auto;padding:24px 16px">
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="${role >= 3}" data-can-author-dm-only="${role >= 3}"
  data-role="${role}" data-user-id="u0" data-api-base="${API}" data-outcome-src="/static/js/widgets/calendar_outcome.js">
<div id="calendar-config" class="hidden" data-calendar='${attr(cal)}' data-events='${attr(month)}'></div>
</div></div></main>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script>
<script src="/static/js/motion_rest.js"></script>
<script src="/static/js/widgets/calendar_era_blend.js"></script>
<script src="/static/js/widgets/calendar_view.js"></script>
</body></html>`;

// failBatch: the batch route refuses with a message.
async function page(t, { role = 3, cal = calendar(), month = [], failBatch = false, viewport = { width: 1200, height: 900 }, mobile = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport, isMobile: mobile, hasTouch: mobile });
  const p = await ctx.newPage();
  const errors = [], writes = [], loaded = [], asked = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(role, cal, month) });
    if (u.pathname.startsWith('/static/')) {
      loaded.push(u.pathname);
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    const tail = u.pathname.replace(API, '');
    if (tail === '/events/passed') {
      asked.push(u.search);
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ events: PASSED, truncated: false }) });
    }
    if (req.method() !== 'GET') {
      writes.push({ method: req.method(), tail, body: req.postData() ? JSON.parse(req.postData()) : null });
      if (failBatch && tail === '/events/batch') return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ message: 'The server said no.' }) });
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
    }
    if (tail === '/events' && u.searchParams.get('month') === '1') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(month) });
    return route.fulfill({ contentType: 'application/json', body: '[]' });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('#cal5-stage .day');
  return { p, writes, loaded, asked };
}

const settle = (p) => p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
const waitFor = async (p, list, n) => { for (let i = 0; i < 150 && list.length < n; i++) await p.waitForTimeout(20); return list; };

// Moves today to Firstfrost <day> through the date form.
async function setToday(p, day) {
  await p.click('#cal5-todaypill [data-today-set]');
  await p.waitForSelector('[data-today-form]');
  await p.waitForFunction(() => document.activeElement && document.activeElement.name === 'month'); // as a person would, once it is open
  await p.fill('[data-today-form] [name="day"]', String(day));
  await p.click('[data-today-form] button[type="submit"]');
}

const row = (i) => `.dpsheet [data-dp-row="${i}"]`;

test('moving today past events asks once what became of each, Happened picked to start with', { skip }, async (t) => {
  const { p, asked } = await page(t);
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet[role="dialog"][aria-modal="true"]');
  assert.deepEqual(asked, ['?from=100-1-9&to=100-1-12']);
  assert.equal(await p.textContent('.dpsheet h2'), 'Firstfrost 9 to 11 have passed');
  const rows = await p.$$eval('.dpsheet .dprow', (rs) => rs.map((r) => ({
    name: r.querySelector('.dphd b').textContent,
    when: r.querySelector('.dpwhen').textContent,
    only: !!r.querySelector('.dponly'),
    picked: r.querySelector('[aria-pressed="true"]').textContent,
    line: !!r.querySelector('[data-dp-why]') || !!r.querySelector('[data-dp-line]'),
  })));
  assert.deepEqual(rows, [
    { name: 'Journey through the dry country', when: 'Firstfrost 9 to 10', only: true, picked: 'Happened', line: true },
    { name: 'Ambush at the dry well', when: 'Firstfrost 10', only: true, picked: 'Happened', line: true },
    { name: 'Arrival at Lastwell', when: 'Firstfrost 11', only: false, picked: 'Happened', line: true },
    { name: 'Sanguin’mor bites the sun', when: 'Firstfrost 11 · one date of a repeating event', only: false, picked: 'Happened', line: false },
  ]);
  assert.equal(await p.isVisible(`${row(2)} [data-dp-line]`), true, 'a line already written shows at once');
  await settle(p);
  assert.equal(await p.evaluate(() => document.activeElement.tagName), 'H2', 'the sheet takes the focus at its heading');
});

test('Done sends one batch and the repeating date’s override, and Undo sends them back', { skip }, async (t) => {
  const { p, writes } = await page(t);
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet');
  writes.length = 0; // the today save
  await p.click(`${row(0)} [data-dp-why]`);
  await p.fill(`${row(0)} [data-dp-line]`, 'Took a day longer');
  await p.click(`${row(1)} [data-dp-ch="didnt"]`);
  await p.click(`${row(2)} [data-dp-ch="moved"]`);
  assert.equal(await p.$eval(`${row(2)} [data-dp-to]`, (s) => s.selectedOptions[0].textContent), 'Wayday, Firstfrost 12', 'Moved starts on the new today');
  await p.click(`${row(3)} [data-dp-ch="didnt"]`);
  await p.click('.dpsheet [data-dp-done]');
  await waitFor(p, writes, 2);
  assert.deepEqual(writes, [
    { method: 'POST', tail: '/events/batch', body: { update: [
      { id: 'e1', changes: { status: 'happened', outcome: 'Took a day longer' } },
      { id: 'e2', changes: { status: 'called_off' } },
      { id: 'e3', changes: { status: 'moved', year: 100, month: 1, day: 12 } },
    ] } },
    { method: 'PUT', tail: '/events/e4/occurrences/100/1/11', body: { action: 'skip' } },
  ]);
  await p.waitForSelector('.dpsheet', { state: 'detached' });
  await p.waitForSelector('.toast.on button:has-text("Undo")');
  writes.length = 0;
  await p.click('.toast.on button');
  await waitFor(p, writes, 2);
  assert.deepEqual(writes, [
    { method: 'POST', tail: '/events/batch', body: { update: [
      { id: 'e1', changes: { status: 'planned', outcome: null } },
      { id: 'e2', changes: { status: 'planned', outcome: null } },
      { id: 'e3', changes: { status: 'planned', outcome: 'Expected at dusk', year: 100, month: 1, day: 11 } },
    ] } },
    { method: 'DELETE', tail: '/events/e4/occurrences/100/1/11', body: null },
  ]);
});

test('a span moved to another day keeps its length, and another day can be typed', { skip }, async (t) => {
  const { p, writes } = await page(t);
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet');
  writes.length = 0;
  await p.click(`${row(0)} [data-dp-ch="moved"]`);
  await p.selectOption(`${row(0)} [data-dp-to]`, 'other');
  assert.equal(await p.evaluate(() => document.activeElement.name), 'month', 'Another day… opens its boxes and goes to them');
  await p.selectOption(`${row(0)} [data-dp-other] [name="month"]`, '2');
  await p.fill(`${row(0)} [data-dp-other] [name="day"]`, '40');
  await p.click('.dpsheet [data-dp-done]');
  await p.waitForSelector('.dpsheet .dperr:not([hidden])');
  assert.equal(await p.textContent('.dpsheet .dperr'), 'Deepwinter has 30 days.');
  assert.deepEqual(writes, [], 'nothing goes out while a day cannot be read');
  await p.fill(`${row(0)} [data-dp-other] [name="day"]`, '30');
  await p.click('.dpsheet [data-dp-done]');
  await waitFor(p, writes, 1);
  const ch = writes[0].body.update[0].changes;
  assert.deepEqual({ y: ch.year, m: ch.month, d: ch.day, ey: ch.end_year, em: ch.end_month, ed: ch.end_day }, { y: 100, m: 2, d: 30, ey: 100, em: 3, ed: 1 });
});

test('a refused batch keeps the sheet open and says why', { skip }, async (t) => {
  const { p } = await page(t, { failBatch: true });
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet');
  await p.click('.dpsheet [data-dp-done]');
  await p.waitForSelector('.dpsheet .dperr:not([hidden])');
  assert.equal(await p.textContent('.dpsheet .dperr'), 'The server said no.');
  assert.equal(await p.$eval('.dpsheet [data-dp-done]', (b) => b.disabled), false);
});

test('Not now and Escape send nothing and give the focus back to the today pill', { skip }, async (t) => {
  const { p, writes } = await page(t);
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet');
  writes.length = 0;
  await p.click('.dpsheet .dpacts [data-dp-later]');
  await p.waitForSelector('.dpsheet', { state: 'detached' });
  await setToday(p, 14);
  await p.waitForSelector('.dpsheet');
  writes.length = 0;
  await p.focus(`${row(0)} [data-dp-ch="didnt"]`);
  await p.keyboard.press('Escape');
  await p.waitForSelector('.dpsheet', { state: 'detached' });
  assert.deepEqual(writes, []);
  assert.equal(await p.evaluate(() => !!document.activeElement.closest('#cal5-todaypill')), true);
});

test('Tab stays inside the sheet', { skip }, async (t) => {
  const { p } = await page(t);
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet');
  await settle(p);
  await p.focus('.dpsheet [data-dp-done]');
  await p.keyboard.press('Tab');
  assert.equal(await p.evaluate(() => !!document.activeElement.closest('.dpsheet')), true);
  await p.keyboard.press('Shift+Tab');
  assert.equal(await p.evaluate(() => document.activeElement.matches('[data-dp-done]')), true);
});

test('the switch in Calendar settings, a step back and a player never ask', { skip }, async (t) => {
  {
    const { p, asked, loaded } = await page(t, { cal: calendar({ ask_when_day_passes: false }) });
    await setToday(p, 12);
    await p.waitForSelector('#cal5-todaypill .td:has-text("12")');
    await p.waitForTimeout(150);
    assert.deepEqual(asked, []);
    assert.equal(loaded.includes('/static/js/widgets/calendar_outcome.js'), false, 'switched off, the sheet is never fetched');
  }
  {
    const { p, asked } = await page(t, { cal: calendar({ current_day: 20 }) });
    await setToday(p, 12);
    await p.waitForSelector('#cal5-todaypill .td:has-text("12")');
    await p.waitForTimeout(150);
    assert.deepEqual(asked, [], 'a step back in time asks nothing');
  }
  {
    const { p, loaded } = await page(t, { role: 1 });
    assert.equal(await p.$('#cal5-todaypill [data-today-set]'), null, 'a player cannot move today');
    assert.equal(loaded.includes('/static/js/widgets/calendar_outcome.js'), false);
  }
});

test('on a phone the sheet is a drawer at the bottom edge with touch-sized choices', { skip }, async (t) => {
  const { p } = await page(t, { viewport: { width: 390, height: 844 }, mobile: true });
  await setToday(p, 12);
  await p.waitForSelector('.dpsheet.phone');
  await settle(p);
  const box = await p.evaluate(() => {
    const r = document.querySelector('.dpsheet').getBoundingClientRect(), chip = document.querySelector('.dpchip').getBoundingClientRect();
    return { left: r.left, right: Math.round(r.right), bottom: Math.round(r.bottom), chip: chip.height, scroll: document.documentElement.scrollWidth };
  });
  assert.deepEqual({ left: box.left, right: box.right, bottom: box.bottom }, { left: 0, right: 390, bottom: 844 });
  assert.ok(box.chip >= 44, `choices are at least 44px tall (${box.chip})`);
  assert.ok(box.scroll <= 390, 'no sideways scroll');
});

test('the month strikes through a called-off event and keeps a dashed mark where a moved one was', { skip }, async (t) => {
  const { p } = await page(t, { month: MONTH });
  await p.waitForSelector('#cal5-stage .mk[data-ev="m1"]');
  const marks = await p.$$eval('#cal5-stage .mk[data-ev]', (ms) => ms.map((m) => ({ id: m.dataset.ev, day: m.closest('.day').dataset.key || '', cls: m.className, title: m.title })));
  const find = (id, day) => marks.find((m) => m.id === id && m.day === '100_1_' + day);
  assert.match(find('m1', 10).cls, /\boff\b/);
  assert.match(find('m1', 10).title, /called off/);
  assert.match(find('m2', 11).cls, /\bmoved\b/, 'the old day keeps a dashed mark');
  assert.match(find('m2', 11).title, /moved to/);
  assert.doesNotMatch(find('m2', 12).cls, /\bmoved\b/, 'the new day shows the event as it is');
  assert.doesNotMatch(find('m3', 20).cls, /\b(off|moved)\b/);
});

test('the event card says what became of it, and changes it with a partial save', { skip }, async (t) => {
  const { p, writes } = await page(t, { month: MONTH });
  await p.waitForSelector('#cal5-stage .mk[data-ev="m1"]');
  await p.click('#cal5-stage .day[data-key="100_1_10"]');
  await p.waitForSelector('.evd.off[data-ev="m1"]');
  assert.equal(await p.textContent('.evd.off[data-ev="m1"] .evwhy'), 'Foiled by the party');
  await p.click('.evd[data-ev="m1"]');
  await p.waitForSelector('.evp.open .evout');
  assert.match(await p.textContent('.evp .evost .evst'), /Called off/);
  assert.equal(await p.textContent('.evp .evwh p'), 'Foiled by the party');
  await p.click('.evp [data-evout-open]');
  await p.click('.evp [data-evout-st="happened"]');
  await p.fill('.evp [name="outcome"]', 'Foiled, then it happened anyway');
  writes.length = 0;
  await p.click('.evp [data-evout-save]');
  await waitFor(p, writes, 1);
  assert.deepEqual(writes, [{ method: 'PUT', tail: '/events/m1', body: { status: 'happened', outcome: 'Foiled, then it happened anyway' } }]);
});
