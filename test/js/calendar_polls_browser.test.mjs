// calendar_polls_browser.test.mjs — date polls on the Game nights calendar.
//
// Runs calendar_view.js in a real Chromium against a stubbed server: a
// poll's dates show on their days, members answer in the day card, whoever
// runs the game picks a date there, a new poll is started by tapping days,
// and a link to a poll opens its day.
//
// Needs playwright-core and a Chromium. It looks in the usual places
// (NODE_PATH, /opt/node-tools, PW_CHROMIUM or /opt/pw-browsers) and skips,
// saying so, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function findPlaywright() {
  for (const base of [import.meta.url, '/opt/node-tools/', process.env.NODE_PATH ? process.env.NODE_PATH + '/' : null]) {
    if (!base) continue;
    try {
      const req = createRequire(base.startsWith('file:') || base.startsWith('/') ? (base.startsWith('/') ? base + 'x.js' : base) : base);
      return req('playwright-core');
    } catch (e) { /* try the next place */ }
  }
  return null;
}
function findChromium() {
  if (process.env.PW_CHROMIUM && existsSync(process.env.PW_CHROMIUM)) return process.env.PW_CHROMIUM;
  try {
    for (const d of readdirSync('/opt/pw-browsers')) {
      const c = join('/opt/pw-browsers', d, 'chrome-linux', 'chrome');
      if (d.startsWith('chromium-') && existsSync(c)) return c;
    }
  } catch (e) { /* none */ }
  return null;
}

const pw = findPlaywright();
const chromePath = findChromium();
const skip = !pw || !chromePath ? 'playwright-core or Chromium not found' : false;

const DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
const CAL = {
  id: 'k1', campaign_id: 'c1', mode: 'reallife', name: 'Real life', tracks_real_time: true,
  current_year: 2026, current_month: 10, current_day: 12, hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
  months: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
    .map((name, i) => ({ name, days: DAYS[i], leap_year_days: i === 1 ? 1 : 0 })),
  weekdays: ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((name) => ({ name })),
  moons: [], seasons: [], eras: [],
};
const ROSTER = ['Ana', 'Bo', 'Cy', 'Di', 'Ed', 'Flo'].map((name, i) => ({ userId: 'u' + i, name, answer: ['going', 'maybe', 'cant', '', 'going', 'maybe'][i] }));
const NIGHT = {
  sessionId: 's1', name: 'The Ashen Road', summary: 'We pick up at the ford.', date: '2026-10-29', time: '19:00', tz: 'America/Chicago',
  recurring: true, recurrenceType: 'weekly', organizerId: 'u0', organizerName: 'Ana', past: false,
  tally: { going: 2, maybe: 2, cant: 1, noAnswer: 1 }, roster: ROSTER, mine: ROSTER[0], canExclude: true,
};
const NIGHT_PAGE = { recap: '', recapHtml: '', links: [{ entityId: 'e1', slug: 'ford', name: 'The Ford' }, { entityId: 'e2', slug: 'bren', name: 'Bren' }] };

const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const HTML = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/calendar-view.css">
<style>html,body{margin:0}</style></head><body>
<div style="display:flex;height:100vh;overflow:hidden"><div style="width:240px"></div>
<div style="position:relative;flex:1;display:flex;flex-direction:column;overflow:hidden;min-width:0">
<div style="height:56px;flex:none"></div>
<main id="main-content" style="flex:1;overflow-y:auto;padding:16px 20px"><div style="max-width:64rem;margin:0 auto;padding:24px 16px">
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="true" data-can-author-dm-only="true"
  data-role="3" data-user-id="u0" data-game-nights="true" data-api-base="/campaigns/c1/calendars/k1">
<div id="calendar-config" class="hidden" data-calendar='${attr(CAL)}' data-events="[]" data-zone="America/Chicago"></div>
</div></div></main></div></div>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script>
<script src="/static/js/widgets/calendar_era_blend.js"></script>
<script src="/static/js/widgets/calendar_view.js"></script>
<script src="/static/js/widgets/calendar_game_night.js"></script>
</body></html>`;

const POLL = {
  id: 'p1', title: 'Next arc', note: '', askedBy: 'Ana', canPick: true, members: 6, tz: 'America/Chicago',
  options: [
    { id: 'o1', date: '2026-10-17', dateLabel: 'Sat, Oct 17', timeLabel: '7:00 PM – 11:00 PM', yes: 1, maybe: 0, no: 1, mine: '' },
    { id: 'o2', date: '2026-10-24', dateLabel: 'Sat, Oct 24', timeLabel: '7:00 PM – 11:00 PM', yes: 3, maybe: 1, no: 0, mine: 'yes' },
  ],
};

async function page(t, query = '') {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  const errors = [], posts = [];
  let polls = [structuredClone(POLL)];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: HTML });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    let body = [];
    if (req.method() === 'POST') {
      posts.push({ path: u.pathname + u.search, body: req.postDataJSON() });
      if (u.pathname.endsWith('/confirm')) { polls = []; body = { sessionId: 's9', link: '/campaigns/c1/sessions/s9' }; }
      else if (u.pathname === '/campaigns/c1/proposals') body = { id: 'p2', link: '/campaigns/c1/proposals/p2' };
      else body = { status: 'ok' };
    } else if (u.pathname === '/campaigns/c1/proposals/open') body = polls;
    else if (u.pathname.endsWith('/events')) body = { data: [], total: 0 };
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
  });
  await p.clock.setFixedTime(new Date('2026-10-12T12:00:00Z'));
  await p.goto('http://chronicle.test/' + query);
  await p.waitForSelector('.day[data-key="2026_10_24"] .mk.plm', { state: 'attached' });
  return { p, posts };
}

const settle = (p) => p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));

test('a poll date shows on its day, and a member answers it in the day card', { skip }, async (t) => {
  const { p, posts } = await page(t);
  assert.ok(await p.$('.day[data-key="2026_10_24"] .mk.plm.mine'), 'a date I answered is marked as answered');
  assert.ok(await p.$('.day[data-key="2026_10_17"] .mk.plm:not(.mine)'));
  await p.click('.day[data-key="2026_10_17"]');
  await p.waitForSelector('#cal5-wing .pollb');
  await settle(p);
  const box = await p.textContent('#cal5-wing .pollb');
  assert.match(box, /Next arc/);
  assert.match(box, /1 can · 0 maybe · 1 can’t · 4 no answer yet/);
  assert.match(box, /Sat Oct 24 · most can/, 'the other date is offered, marked as the one most can make');
  await p.click('#cal5-wing .pollb [data-poll-ans="yes"]');
  await p.waitForFunction(() => /2 can · 0 maybe · 1 can’t/.test(document.querySelector('#cal5-wing .pollb').textContent));
  assert.deepEqual(posts.at(-1), { path: '/campaigns/c1/proposals/p1/options/o1/respond', body: { response: 'yes' } });
  assert.equal(await p.getAttribute('#cal5-wing .pollb [data-poll-ans="yes"]', 'aria-pressed'), 'true');
  // Another date opens its own day.
  await p.click('#cal5-wing .pollb [data-poll-go]');
  await p.waitForSelector('#cal5-wing .pollb.hl[data-ev="poll:p1:o2"]');
});

test('whoever runs the game picks a date in the day card, after saying so twice', { skip }, async (t) => {
  const { p, posts } = await page(t);
  await p.click('.day[data-key="2026_10_24"]');
  await p.waitForSelector('#cal5-wing [data-poll-pick]');
  await settle(p);
  await p.click('#cal5-wing [data-poll-pick]');
  await p.waitForSelector('#cal5-wing .plpick.ask');
  assert.match(await p.textContent('#cal5-wing .plpick.ask'), /closes the poll and plans a game night on Sat Oct 24/);
  assert.equal(posts.length, 0, 'nothing is sent until it is confirmed');
  await p.click('#cal5-wing [data-poll-pick-yes]');
  await p.waitForFunction(() => !document.querySelector('.mk.plm'));
  assert.deepEqual(posts.at(-1), { path: '/campaigns/c1/proposals/p1/confirm?tz=America%2FChicago', body: { optionId: 'o2' } });
});

test('a poll is started from "Plan a game night" by tapping days', { skip }, async (t) => {
  const { p, posts } = await page(t);
  await p.click('.day[data-key="2026_10_20"]');
  await p.waitForSelector('#cal5-wing [data-gn-choose]');
  await settle(p);
  await p.click('#cal5-wing [data-gn-choose]');
  await p.click('#cal5-wing [data-poll-new]');
  await p.waitForSelector('#cal5-pollbar:not([hidden]) .pbar');
  await p.waitForFunction(() => !document.querySelector('#cal5-wing.open'));
  await p.click('.day[data-key="2026_10_27"]');
  await p.click('.day[data-key="2026_10_22"]');
  await p.click('.day[data-key="2026_10_5"]'); // passed: refused
  await p.click('.day[data-key="2026_10_22"]'); // tapped again: taken off
  assert.deepEqual(await p.$$eval('.day.ppick', (d) => d.map((x) => x.dataset.key)), ['2026_10_20', '2026_10_27']);
  await p.fill('#cal5-pollbar input[name="title"]', 'Session zero');
  await p.click('#cal5-pollbar [type="submit"]');
  await p.waitForFunction(() => document.querySelector('#cal5-pollbar').hidden);
  const sent = posts.find((x) => x.path === '/campaigns/c1/proposals');
  assert.deepEqual(sent.body, {
    title: 'Session zero', note: '', tz: 'America/Chicago',
    options: [{ date: '2026-10-20', startMinute: 1140, endMinute: 1380 }, { date: '2026-10-27', startMinute: 1140, endMinute: 1380 }],
  });
  assert.equal(await p.$$eval('.day.ppick', (d) => d.length), 0);
});

test('a link to a poll opens its day with the poll picked out', { skip }, async (t) => {
  const { p } = await page(t, '?poll=p1&date=2026-10-24');
  await p.waitForSelector('#cal5-wing.open .pollb.hl[data-ev="poll:p1:o2"]');
});
