// calendar_panels_browser.test.mjs — the calendar's panels stay on screen.
//
// Runs calendar_view.js in a real Chromium, inside a page shaped like the
// app's (a fixed-height shell whose main column scrolls), and opens a game
// night's full page from a day low in the month. The page must end at the
// screen's edge and scroll inside itself: a panel that runs past the edge
// can't be scrolled into view while the pointer is over it.
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
<header id="app-topbar" style="height:56px;flex:none;position:relative;isolation:isolate;z-index:20;background:#fff"></header>
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

async function page(t, viewport, opts = {}) {
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
    if (opts.route) {
      const own = opts.route(u, route.request());
      if (own !== undefined) return route.fulfill({ contentType: 'application/json', body: JSON.stringify(own) });
    }
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

// Escape pressed while a day's card is still folding open closes it every
// time, rather than queueing a close an interrupted opening could drop.
test('Escape while a day card is opening closes it', { skip }, async (t) => {
  const p = await page(t, { width: 1280, height: 800 });
  for (let i = 0; i < 20; i++) {
    await p.click('.day[data-key="2026_10_29"]');
    await p.waitForTimeout(60);
    await p.keyboard.press('Escape');
    await settle(p);
    assert.equal(await p.$eval('#cal5-wing', (w) => w.classList.contains('open')), false, `run ${i + 1}: the card stayed open`);
  }
});

// A calendar message sits inside the window even when the card runs past
// the window's bottom edge, so its Undo can always be seen and pressed.
const showToast = (p) => p.evaluate(() => {
  document.querySelector('[data-widget="calendar_view"]').calendarView._toast('Hidden for everyone.', { label: 'Undo', run() {} });
});
const undoAt = (p) => p.evaluate(() => {
  const b = document.querySelector('#cal5-cal .toast button').getBoundingClientRect();
  const hit = document.elementFromPoint(b.left + b.width / 2, b.top + b.height / 2);
  return { top: b.top, bottom: b.bottom, h: innerHeight, pressable: !!hit && hit.matches('.toast button') };
});

test('a calendar message and its Undo stay inside the window', { skip }, async (t) => {
  const p = await page(t, { width: 1280, height: 640 });
  const calBottom = await p.$eval('#cal5-cal', (c) => c.getBoundingClientRect().bottom);
  assert.ok(calBottom > 640, `the fixture's card must run past the window (bottom ${calBottom})`);
  await showToast(p);
  await settle(p);
  const u = await undoAt(p);
  assert.ok(u.bottom <= u.h, `Undo's bottom ${u.bottom} is below the window's ${u.h}`);
  assert.ok(u.pressable, 'Undo is the thing under its own centre');
});

// An editor's calendar has the Look/Edit bar along its bottom; the message
// rises clear of it, and of the drawer while editing.
test('a calendar message clears the Look/Edit bar and its drawer', { skip }, async (t) => {
  const p = await page(t, { width: 1280, height: 800 });
  await p.evaluate(() => {
    const cal = document.querySelector('#cal5-cal');
    const deck = document.createElement('div');
    deck.className = 'deck';
    deck.innerHTML = '<div class="dkdrawer"><div class="dkr" style="height:64px">Tools</div></div>' +
      '<div class="dkstrip"><button type="button" class="dktab">Look</button><button type="button" class="dktab">Edit</button></div>';
    cal.appendChild(deck);
    cal.classList.add('decked');
  });
  for (const editing of [false, true]) {
    await p.evaluate((on) => {
      document.querySelector('#cal5-cal').classList.toggle('editing', on);
      document.querySelector('#cal5-cal .deck').classList.toggle('up', on);
    }, editing);
    await settle(p);
    await showToast(p);
    await settle(p);
    const m = await p.evaluate(() => {
      const deck = document.querySelector('#cal5-cal .deck');
      const dr = deck.querySelector('.dkdrawer');
      const top = deck.classList.contains('up') ? dr.getBoundingClientRect().top : deck.getBoundingClientRect().top;
      return { toast: document.querySelector('#cal5-cal .toast').getBoundingClientRect().bottom, top };
    });
    assert.ok(m.toast <= m.top - 8, `${editing ? 'editing' : 'looking'}: the message ends at ${m.toast}, the bar starts at ${m.top}`);
  }
});

// On a phone the message moves to the top of the screen, below the app's
// top bar, where Undo can be tapped.
test('on a phone a calendar message sits below the app top bar', { skip }, async (t) => {
  const p = await page(t, { width: 390, height: 800 });
  await showToast(p);
  await settle(p);
  const u = await undoAt(p);
  const bar = await p.$eval('#app-topbar', (b) => b.getBoundingClientRect().bottom);
  assert.ok(u.top >= bar, `Undo starts at ${u.top}, under the top bar ending at ${bar}`);
  assert.ok(u.pressable, 'Undo is the thing under its own centre');
});

// Hours saved as UTC by a player set to Chicago: asked once, at the top of
// the calendar, in the player's own clock. The page's clock is in October,
// so Chicago is on daylight time, 5 hours behind UTC.
test("hours saved in another zone are asked about once, in the viewer's clock", { skip }, async (t) => {
  const posts = [];
  let mine = {
    tz: 'UTC', answered: true, accountTz: 'America/Chicago', zoneConfirmed: false,
    blocks: [1, 2, 3, 4, 5].map((d) => ({ dayOfWeek: d, startMinute: 1080, endMinute: 1440, state: 'available', weekCadence: 0 })),
  };
  const p = await page(t, { width: 1280, height: 800 }, {
    route(u, req) {
      if (!u.pathname.endsWith('/availability/mine/zone') && !u.pathname.endsWith('/availability/mine')) return undefined;
      if (req.method() === 'POST') {
        posts.push(JSON.parse(req.postData()));
        mine = { ...mine, tz: 'America/Chicago', zoneConfirmed: true };
        return { status: 'ok' };
      }
      return mine;
    },
  });
  const strip = p.locator('#cal5-zstrip');
  await strip.waitFor();
  const words = (await strip.innerText()).replace(/\s+/g, ' ');
  assert.match(words, /Are your hours in Chicago time\?/);
  assert.match(words, /You painted weekday evenings from 6pm, saved as UTC time\. Your calendar shows that as 1pm to 7pm\./);

  await strip.getByRole('button', { name: 'Other answers' }).click();
  const ask = p.locator('.zask');
  await ask.waitFor();
  assert.equal(await ask.locator('[data-zone-answer]').count(), 3, 'the hours panel offers all three answers');

  await ask.getByRole('button', { name: /Chicago time: keep my boxes/ }).click();
  await p.waitForFunction(() => document.querySelector('#cal5-zstrip').hidden && !document.querySelector('.zask'));
  assert.deepEqual(posts, [{ mode: 'relabel', tz: 'America/Chicago' }]);
});

test("hours already in the viewer's zone ask nothing", { skip }, async (t) => {
  const p = await page(t, { width: 1280, height: 800 }, {
    route(u) {
      if (!u.pathname.endsWith('/availability/mine')) return undefined;
      return { tz: 'America/Chicago', answered: true, accountTz: 'America/Chicago', zoneConfirmed: false, blocks: [{ dayOfWeek: 1, startMinute: 1080, endMinute: 1440, state: 'available', weekCadence: 0 }] };
    },
  });
  await p.waitForTimeout(400);
  assert.equal(await p.$eval('#cal5-zstrip', (el) => el.hidden), true);
});
