// calendar_card_in_window_browser.test.mjs — a day's card, and the game
// night's page that opens over it, stay inside the window when a strip sits
// above the page (the timezone notice, the RSVP strip). A day low in the
// month is then cut off by the window's bottom edge, and a card lined up
// with that day's bottom ran past the edge with it, so its last section was
// reachable only by scrolling the whole page.
//
// Runs calendar_view.js in a real Chromium inside a copy of the app's shell
// (a strip, then the page's own scroll). Needs playwright-core and a Chromium
// (see _browser_env.mjs); skips locally, and fails under CI, when either is
// missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
const CAL = {
  id: 'k1', campaign_id: 'c1', mode: 'reallife', name: 'Real world', tracks_real_time: true,
  current_year: 2026, current_month: 11, current_day: 27, hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
  months: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
    .map((name, i) => ({ name, days: DAYS[i], leap_year_days: i === 1 ? 1 : 0 })),
  weekdays: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].map((name) => ({ name })),
  moons: [], seasons: [], eras: [],
};
// A night with a long roster, so its page is taller than the room left.
const roster = Array.from({ length: 9 }, (_, i) => ({ userId: 'u' + i, name: 'Player ' + i, answer: '' }));
const NIGHT = {
  sessionId: 's1', name: 'Heist night', summary: 'The vault under the bridge.', date: '2026-11-28', time: '19:00', tz: 'UTC', recurring: false,
  organizerId: 'u0', organizerName: 'Dana', past: false, tally: { going: 0, maybe: 0, cant: 0, noAnswer: 9 }, roster,
  mine: { userId: 'u1', name: 'Pip', answer: '' }, canExclude: false,
};

const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
// The shell the app draws: beside the sidebar, a top bar and a strip above the page, which
// scrolls on its own (#main-content), all inside one window-high column.
const html = (strip) => `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/calendar-view.css">
<style>body{margin:0}#col{margin-left:256px;display:flex;flex-direction:column;height:100vh;overflow:hidden;position:relative}#bar{height:48px;flex:none}#strip{height:${strip}px;flex:none;background:#eee}#row{flex:1;display:flex;min-height:0}#main-content{flex:1;overflow-y:auto}</style></head><body>
<div id="col"><div id="bar"></div><div id="strip"></div><div id="row"><main id="main-content">
<div class="cal5-page" style="margin:0 auto;padding:24px 16px"><div style="height:140px">The breadcrumbs and whatever else sits above the calendar</div>
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="false" data-can-author-dm-only="false"
  data-role="1" data-user-id="u1" data-game-nights="true" data-api-base="/campaigns/c1/calendars/k1">
<div id="calendar-config" class="hidden" data-calendar='${attr(CAL)}' data-events="[]" data-zone="UTC"></div>
</div></div></main></div></div>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/motion_rest.js"></script>
<script src="/static/js/boot.js"></script>
<script src="/static/js/widgets/calendar_era_blend.js"></script>
<script src="/static/js/widgets/calendar_view.js"></script>
</body></html>`;

async function open(t, strip) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1280, height: 800 }, reducedMotion: 'reduce' });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(strip) });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    let body = [];
    if (u.pathname === '/campaigns/c1/sessions/nights') body = [NIGHT];
    else if (u.pathname.endsWith('/events')) body = { data: [], total: 0 };
    else if (u.pathname.endsWith('/page')) body = { entries: [], links: [], recap: '' };
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
  });
  await p.clock.setFixedTime(new Date('2026-11-27T15:00:00Z'));
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('.day[data-key="2026_11_28"] .gndie', { state: 'attached' });
  return p;
}

// Opened the way a person opens it, without the page scrolling first.
const press = (p, sel) => p.evaluate((s) => document.querySelector(s).click(), sel);
const box = (p, sel) => p.$eval(sel, (e) => { const r = e.getBoundingClientRect(); return { top: r.top, bottom: r.bottom }; });

for (const strip of [0, 49]) {
  test(`the day's card and the night's page fit the window (strip ${strip}px)`, { skip }, async (t) => {
    const p = await open(t, strip);
    await press(p, '.day[data-key="2026_11_28"]');
    await p.waitForSelector('#cal5-wing.open');
    await p.waitForTimeout(300);
    const main = await box(p, '#main-content');
    const day = await box(p, '.day[data-key="2026_11_28"]');
    if (strip) assert.ok(day.bottom > main.bottom, 'the case under test: the day runs past the window ' + JSON.stringify([day, main]));
    const wing = await box(p, '#cal5-wing');
    assert.ok(wing.bottom <= main.bottom + 0.5, `the day's card ends at ${wing.bottom}, past the window's ${main.bottom}`);
    assert.ok(wing.top >= main.top - 0.5, `the day's card starts at ${wing.top}, above the page's ${main.top}`);

    await press(p, '#cal5-wing .gnb[data-ev^="gn:s1"]');
    await p.waitForSelector('#cal5-evp.open');
    await p.waitForTimeout(300);
    const page = await box(p, '#cal5-evp');
    assert.ok(page.bottom <= main.bottom + 0.5, `the night's page ends at ${page.bottom}, past the window's ${main.bottom}`);
    assert.equal(await p.$eval('#main-content', (m) => m.scrollTop), 0, 'the page was scrolled to reach the card');
  });
}
