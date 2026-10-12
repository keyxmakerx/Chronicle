// calendar_panels_browser.test.mjs — the calendar's panels stay on screen.
//
// Runs calendar_view.js in a real Chromium, inside a page shaped like the
// app's (a fixed-height shell whose main column scrolls), and opens a game
// night's full page from a day low in the month. The page must end at the
// screen's edge and scroll inside itself: a panel that runs past the edge
// can't be scrolled into view while the pointer is over it.
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

async function page(t, viewport) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport });
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
    if (u.pathname.endsWith('/sessions/nights')) {
      body = u.searchParams.get('from') <= NIGHT.date && NIGHT.date <= u.searchParams.get('to') ? [NIGHT] : [];
    } else if (u.pathname.endsWith('/sessions/s1/page')) body = NIGHT_PAGE;
    else if (u.pathname.endsWith('/events')) body = { data: [], total: 0 };
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
  });
  // A real-world calendar shows today's month; today is held to the fixture's.
  await p.clock.setFixedTime(new Date('2026-10-12T12:00:00Z'));
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('.day[data-key="2026_10_29"]');
  return p;
}

const settle = (p) => p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));

for (const [label, viewport] of [['a desktop', { width: 1280, height: 800 }], ['a short laptop', { width: 1280, height: 640 }]]) {
  test(`a game night's full page ends at the screen's edge and scrolls inside, on ${label}`, { skip }, async (t) => {
    const p = await page(t, viewport);
    await p.click('.day[data-key="2026_10_29"]');
    await p.waitForSelector('#cal5-wing.open .evd .qt0');
    await settle(p);
    await p.click('#cal5-wing .evd .qt0');
    await p.waitForSelector('#cal5-evp.open .gnlinks');
    await settle(p);
    const m = await p.evaluate(() => {
      const evp = document.querySelector('#cal5-evp'), ein = evp.querySelector('.ein');
      ein.scrollTop = ein.scrollHeight;
      const links = evp.querySelector('.gnlinks').getBoundingClientRect();
      return { vh: innerHeight, bottom: evp.getBoundingClientRect().bottom, scrolls: ein.scrollHeight > ein.clientHeight, linksBottom: links.bottom, einBottom: ein.getBoundingClientRect().bottom };
    });
    assert.ok(m.bottom <= m.vh, `the page runs to ${m.bottom}px on a ${m.vh}px screen`);
    assert.ok(m.scrolls, 'the page is taller than the room, so it scrolls inside');
    assert.ok(m.linksBottom <= m.einBottom + 1, 'scrolled to its end, Linked pages is in view');
  });
}
