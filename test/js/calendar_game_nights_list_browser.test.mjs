// calendar_game_nights_list_browser.test.mjs — the real-world calendar's
// game nights: the d20 sits in the middle of its day, and the header's
// "Game nights" list (in place of Moons) opens a night on its day, lists
// every night, and gives a member their calendar link. Game nights have no
// page of their own, so nothing here links to one.
//
// Runs calendar_view.js in a real Chromium against a stubbed server. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
const CAL = {
  id: 'k1', campaign_id: 'c1', mode: 'reallife', name: 'Game nights', tracks_real_time: true,
  current_year: 2026, current_month: 10, current_day: 25, hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
  months: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
    .map((name, i) => ({ name, days: DAYS[i], leap_year_days: i === 1 ? 1 : 0 })),
  weekdays: ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((name) => ({ name })),
  moons: [], seasons: [], eras: [],
};
const night = (date, name, past, answer) => ({
  sessionId: 's-' + date, name, summary: '', date, time: '19:00', tz: 'America/Chicago', recurring: false, organizerId: 'u0',
  organizerName: 'Ana', past, tally: { going: 1, maybe: 0, cant: 0, noAnswer: 1 }, roster: [], mine: { userId: 'u1', name: 'Bo', answer }, canExclude: false,
});
const NIGHTS = [night('2026-10-22', 'The Ashen Road', true, 'yes'), night('2026-10-29', 'The Ashen Road', false, 'yes'), night('2026-11-12', 'Bren’s one-shot', false, '')];

const ALL = [
  { key: 'upcoming', label: 'Upcoming', nights: [
    { sessionId: 's-2026-10-29', name: 'The Ashen Road', day: '2026-10-01', date: '2026-10-29', when: 'Thu, Oct 1', repeat: 'Every week' }] },
  { key: 'played', label: 'Played', nights: [{ sessionId: 's-2026-10-22', name: 'Session zero', day: '2026-10-22', date: '2026-10-22', when: 'Thu, Oct 22' }] },
  { key: 'cancelled', label: 'Cancelled', nights: [{ sessionId: 's-off', name: 'Snowed in', when: '', href: '/campaigns/c1/sessions/s-off' }] },
];
const TOKEN = '/sessions/feed/eV6Ihdg6aJUrpKydCFGzgv1HPHI66ya2DlFX4KC7tO8.ics';

const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const HTML = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/calendar-view.css"></head><body>
<div class="cal5-page" style="margin:0 auto;padding:24px 16px">
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="false" data-can-author-dm-only="false"
  data-role="1" data-user-id="u1" data-game-nights="true" data-api-base="/campaigns/c1/calendars/k1">
<div id="calendar-config" class="hidden" data-calendar='${attr(CAL)}' data-events="[]" data-zone="America/Chicago"></div>
</div></div>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/motion_rest.js"></script>
<script src="/static/js/boot.js"></script>
<script src="/static/js/widgets/calendar_era_blend.js"></script>
<script src="/static/js/widgets/calendar_view.js"></script>
</body></html>`;

async function page(t, width, o = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width, height: 1000 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: HTML });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    let body = [];
    if (u.pathname === '/campaigns/c1/sessions/nights') {
      const from = u.searchParams.get('from'), to = u.searchParams.get('to');
      body = NIGHTS.filter((n) => n.date >= from && n.date <= to);
    } else if (u.pathname === '/campaigns/c1/sessions/all') body = ALL;
    else if (u.pathname === '/campaigns/c1/sessions/feed-settings') body = { enabled: !!o.feed };
    else if (u.pathname.startsWith('/campaigns/c1/sessions/feed/token')) {
      (o.posts || []).push(u.pathname);
      body = { url: u.pathname.endsWith('/replace') ? TOKEN.replace('eV6', 'xY9') : TOKEN };
    } else if (u.pathname.endsWith('/events')) body = { data: [], total: 0 };
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
  });
  await p.clock.setFixedTime(new Date('2026-10-25T15:00:00Z'));
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('.day[data-key="2026_10_29"] .gndie .mk.gnm .gnart', { state: 'attached' });
  return p;
}

test('a game night is a drawn d20 in the middle of its day, not one of the marks', { skip }, async (t) => {
  const p = await page(t, 1280);
  assert.equal(await p.$('.day[data-key="2026_10_29"] .marks .gnm'), null);
  const off = await p.$eval('.day[data-key="2026_10_29"]', (day) => {
    const d = day.getBoundingClientRect(), m = day.querySelector('.gndie .mk').getBoundingClientRect();
    return [Math.abs((m.left + m.width / 2) - (d.left + d.width / 2)), Math.abs((m.top + m.height / 2) - (d.top + d.height / 2))];
  });
  assert.ok(off[0] < 1 && off[1] < 1, 'the die is in the middle of its day (off by ' + off.join(', ') + 'px)');
  // The game night editor's drawer is also class "gnd"; the die's rules must not reach it.
  const drawer = await p.evaluate(() => {
    const d = document.createElement('div');
    d.className = 'drawer ded gnd';
    document.querySelector('.cal-v5').appendChild(d);
    const cs = getComputedStyle(d);
    return [cs.pointerEvents, cs.position];
  });
  assert.notEqual(drawer[0], 'none', 'the game night editor still takes clicks');
  assert.notEqual(await p.$eval('.gnart', (a) => getComputedStyle(a).backgroundImage), 'none', 'its lines carry the rainbow');
});

test('the header lists game nights instead of moons, and a row opens its night', { skip }, async (t) => {
  const p = await page(t, 1280);
  assert.equal(await p.$eval('#cal5-moonbtn', (b) => b.hidden), true);
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .wrow');
  const rows = await p.$$eval('#cal5-gnpop .wrow .wt', (els) => els.map((e) => e.textContent));
  assert.deepEqual(rows, ['The Ashen Road', 'Bren’s one-shot', 'The Ashen Road'], 'coming up in date order, then the last one played');
  assert.match(await p.textContent('#cal5-gnpop'), /Going[\s\S]*No answer yet[\s\S]*Last played[\s\S]*Played/);
  await p.click('#cal5-gnpop [data-gn-go="s-2026-11-12"]');
  assert.equal(await p.$eval('#cal5-gnpop', (x) => x.hidden), true);
  await p.waitForSelector('#cal5-wing .gnb.hl[data-ev="gn:s-2026-11-12:2026-11-12"]');
  assert.match(await p.textContent('#cal5-mtitle'), /November/);
});

test('All game nights lists every night in the panel, and goes back', { skip }, async (t) => {
  const p = await page(t, 1280);
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop [data-gn-all]');
  assert.equal(await p.$('#cal5-gnpop a[href$="/sessions"]'), null, 'no link to a Game nights page');
  assert.equal(await p.$('#cal5-gnpop [data-gn-feed]'), null, 'no calendar link while the feed is off');
  await p.click('#cal5-gnpop [data-gn-all]');
  await p.waitForSelector('#cal5-gnpop [data-gn-next]');
  const text = await p.textContent('#cal5-gnpop');
  assert.match(text, /Upcoming[\s\S]*The Ashen Road[\s\S]*Every week[\s\S]*Played[\s\S]*Session zero[\s\S]*Cancelled[\s\S]*Snowed in/);
  assert.equal(await p.$eval('#cal5-gnpop a.wrow', (a) => a.getAttribute('href')), '/campaigns/c1/sessions/s-off', 'a night not on the grid opens its own page');
  assert.equal(await p.$eval('#cal5-gnpop', (x) => x.hidden), false, 'the panel stays open');
  await p.click('#cal5-gnpop [data-gn-next]');
  await p.waitForSelector('#cal5-gnpop [data-gn-all]');
  assert.match(await p.textContent('#cal5-gnpop'), /Coming up/);
  await p.click('#cal5-gnpop [data-gn-all]');
  await p.click('#cal5-gnpop [data-gn-go="s-2026-10-22"]');
  await p.waitForSelector('#cal5-wing .gnb.hl[data-ev="gn:s-2026-10-22:2026-10-22"]');
});

test('Add to your calendar makes the link once, copies it, and replaces it after asking', { skip }, async (t) => {
  const posts = [];
  const p = await page(t, 1280, { feed: true, posts });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop [data-gn-feed]');
  assert.equal(posts.length, 0, 'no link is made until it is asked for');
  await p.click('#cal5-gnpop [data-gn-feed]');
  await p.waitForFunction(() => document.querySelector('#cal5-gnpop [data-gn-feed-url]').value.endsWith('.ics'));
  assert.equal(await p.$eval('#cal5-gnpop [data-gn-feed-url]', (i) => i.value), 'http://chronicle.test' + TOKEN);
  await p.click('#cal5-gnpop [data-gn-replace]');
  assert.equal(await p.$eval('#cal5-gnpop [data-gn-confirm]', (x) => x.hidden), false);
  await p.click('#cal5-gnpop [data-gn-replace-yes]');
  await p.waitForFunction(() => /xY9/.test(document.querySelector('#cal5-gnpop [data-gn-feed-url]').value));
  assert.deepEqual(posts, ['/campaigns/c1/sessions/feed/token', '/campaigns/c1/sessions/feed/token/replace']);
});

test('the page grows on a wide screen', { skip }, async (t) => {
  const p = await page(t, 1920);
  const w = await p.$eval('.cal-v5 .cal', (c) => c.getBoundingClientRect().width);
  assert.ok(w > 1100, 'the month is wider than the old 1024px column (' + w + 'px)');
});
