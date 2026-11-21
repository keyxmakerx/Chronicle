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
const html = (role) => `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/calendar-view.css"></head><body>
<div class="cal5-page" style="margin:0 auto;padding:24px 16px">
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="false" data-can-author-dm-only="false"
  data-role="${role}" data-user-id="u1" data-game-nights="true" data-api-base="/campaigns/c1/calendars/k1">
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
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(o.role || 1) });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    let body = [];
    if (u.pathname === '/campaigns/c1/sessions/nights') {
      const from = u.searchParams.get('from'), to = u.searchParams.get('to');
      body = (o.nights || NIGHTS).filter((n) => n.date >= from && n.date <= to);
    } else if (u.pathname === '/campaigns/c1/sessions/all') body = o.all || ALL;
    else if (u.pathname === '/campaigns/c1/sessions/feed-settings') body = { enabled: !!o.feed };
    else if (u.pathname.startsWith('/campaigns/c1/sessions/feed/token')) {
      (o.posts || []).push(u.pathname);
      body = { url: u.pathname.endsWith('/replace') ? TOKEN.replace('eV6', 'xY9') : TOKEN };
    } else if (u.pathname === '/campaigns/c1/sessions/event-candidates') {
      (o.asked || []).push(u.pathname);
      body = o.candidates || [];
    } else if (u.pathname.startsWith('/campaigns/c1/sessions/event-dismissals/')) {
      const id = decodeURIComponent(u.pathname.split('/').pop());
      (o.dismissed || []).push(route.request().method() + ' ' + id);
      if (route.request().method() === 'DELETE') {
        if (o.restoreStatus) return route.fulfill({ status: o.restoreStatus, contentType: 'application/json', body: '{"error":"no"}' });
        // The server puts it back: the next read of the strip lists it again.
        if (o.candidates && o.original) o.candidates = o.original.filter((r) => o.candidates.some((c) => c.eventId === r.eventId) || r.eventId === id);
        return route.fulfill({ status: 204, body: '' });
      }
      if (o.dismissStatus) return route.fulfill({ status: o.dismissStatus, contentType: 'application/json', body: '{"error":"no"}' });
      // The server keeps it: the next read of the strip leaves it out.
      if (o.candidates) o.candidates = o.candidates.filter((r) => r.eventId !== id);
      return route.fulfill({ status: 204, body: '' });
    } else if (u.pathname === '/campaigns/c1/sessions/from-event') {
      (o.made || []).push(route.request().postDataJSON());
      if (o.makeStatus) return route.fulfill({ status: o.makeStatus, contentType: 'application/json', body: JSON.stringify({ error: o.makeError || 'no' }) });
      if (o.afterMake) o.nights.push(o.afterMake);
      body = { id: 's-new', name: 'made', scheduledDate: o.madeDate || '' };
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

const CANDS = [
  { eventId: 'e1', name: 'Session 1', when: '14 Hammer, 1492', page: 'The Sunken Chapel' },
  { eventId: 'e2', name: 'Session 2', when: '21 Hammer, 1492' },
];

test('a player is not offered the strip and nothing is asked for it', { skip }, async (t) => {
  const asked = [];
  const p = await page(t, 1280, { role: 1, candidates: CANDS, asked });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .wrow');
  assert.equal(await p.$('#cal5-gnpop .gnstrip'), null);
  assert.deepEqual(asked, []);
});

test('the strip names Session events with no game night, and makes one on a press', { skip }, async (t) => {
  const made = [];
  const p = await page(t, 1280, { role: 2, candidates: CANDS, made });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  const strips = await p.$$eval('#cal5-gnpop .gnstrip', (els) => els.map((e) => e.innerText.replace(/\s+/g, ' ').trim()));
  assert.equal(strips.length, 2);
  assert.match(strips[0], /Session 1 is on your calendar with no game night\. 14 Hammer, 1492 · The Sunken Chapel Make a game night Not this one/);
  assert.ok(strips[1].includes('21 Hammer, 1492') && !strips[1].includes('·'), 'no page, no separator');
  // The strip sits above "Coming up", and its controls are quiet words, not bordered buttons.
  assert.ok(await p.$eval('#cal5-gnpop', (pop) => pop.firstElementChild.classList.contains('gnstrip')));
  assert.equal(await p.$eval('.gnstrip [data-gn-make]', (b) => getComputedStyle(b).borderTopWidth), '0px');
  assert.equal(made.length, 0, 'nothing is made until it is pressed');
  await p.click('#cal5-gnpop [data-gn-make="e1"]');
  await p.waitForFunction(() => document.querySelectorAll('#cal5-gnpop .gnstrip').length === 1);
  assert.deepEqual(made, [{ eventId: 'e1' }], 'only the event is named; its words come from the server');
  assert.match(await p.textContent('#cal5-gnpop .gnstrip'), /Session 2/);
  assert.match(await p.textContent('.toast'), /Made a game night for Session 1\. It has no real date yet/);
  assert.equal(await p.evaluate(() => document.activeElement.hasAttribute('data-gn-make')), true, 'focus moves to the next control');
});

test('Not this one is kept on the server, not in this browser', { skip }, async (t) => {
  const dismissed = [];
  const p = await page(t, 1280, { role: 2, candidates: CANDS.slice(), dismissed });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  await p.click('#cal5-gnpop [data-gn-skip="e1"]');
  assert.equal(await p.$$eval(ROW, (e) => e.length), 1, 'the row goes at once');
  for (let i = 0; i < 50 && !dismissed.length; i++) await new Promise((r) => setTimeout(r, 20));
  assert.deepEqual(dismissed, ['PUT e1'], 'the press is sent to the server');
  // Reopening reads the strip from the server again, which now leaves it out.
  await p.click('#cal5-gnbtn');
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .wrow');
  assert.doesNotMatch(await p.textContent('#cal5-gnpop'), /Session 1 is on your calendar/);
  assert.match(await p.textContent('#cal5-gnpop'), /Session 2 is on your calendar/);
  assert.equal(await p.evaluate(() => Object.keys(localStorage).filter((k) => /gn-skip/.test(k)).length), 0, 'nothing is remembered in the browser');
});

const ROW = '#cal5-gnpop .gnstrip:not(.gnstrip-undo)';

for (const [w, h] of [[1280, 900], [360, 800]]) {
  test(`Not this one leaves Undo in the panel, in the window at ${w}x${h}, and Undo keeps the panel open`, { skip }, async (t) => {
    const dismissed = [];
    const p = await page(t, w, { role: 2, candidates: CANDS.slice(), original: CANDS, dismissed });
    await p.setViewportSize({ width: w, height: h });
    await p.click('#cal5-gnbtn');
    await p.waitForSelector(ROW);
    await p.click('#cal5-gnpop [data-gn-skip="e1"]');
    await p.waitForSelector('#cal5-gnpop [data-gn-undo="e1"]');
    assert.match(await p.textContent('#cal5-gnpop [data-gn-undo-row="e1"]'), /Hidden for everyone\. Undo/);
    assert.equal(await p.$('.toast.on'), null, 'no toast for this action');
    const box = await p.$eval('#cal5-gnpop [data-gn-undo="e1"]', (b) => {
      const r = b.getBoundingClientRect();
      return { t: r.top, b: r.bottom, l: r.left, r: r.right, vw: innerWidth, vh: innerHeight };
    });
    assert.ok(box.t >= 0 && box.b <= box.vh && box.l >= 0 && box.r <= box.vw, 'Undo is inside the window: ' + JSON.stringify(box));
    assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-gn-undo')), 'e1', 'focus moves to Undo');
    await p.click('#cal5-gnpop [data-gn-undo="e1"]');
    await p.waitForFunction(() => document.querySelectorAll('#cal5-gnpop .gnstrip:not(.gnstrip-undo)').length === 2);
    assert.equal(await p.$eval('#cal5-gnpop', (el) => el.hidden), false, 'the panel stays open');
    assert.deepEqual(dismissed, ['PUT e1', 'DELETE e1']);
    assert.equal(await p.$('#cal5-gnpop .gnstrip-undo'), null);
    const names = await p.$$eval(ROW + ' b', (els) => els.map((e) => e.textContent));
    assert.equal(names[0], 'Session 1', 'back in its old place');
    assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-gn-make')), 'e1', 'focus returns to the row');
  });
}

test('the Undo line folds away after about three seconds and focus moves to the next row', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS.slice(), dismissed: [] });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector(ROW);
  await p.click('#cal5-gnpop [data-gn-skip="e1"]');
  await p.waitForSelector('#cal5-gnpop [data-gn-undo="e1"]');
  const t0 = Date.now();
  await p.waitForSelector('#cal5-gnpop [data-gn-undo-row="e1"]', { state: 'detached', timeout: 6000 });
  const took = Date.now() - t0;
  assert.ok(took > 2000 && took < 4500, 'the line lasted about three seconds, took ' + took);
  assert.equal(await p.$$eval(ROW, (e) => e.length), 1);
  assert.equal(await p.evaluate(() => !!document.activeElement.closest('.gnstrip') && document.activeElement.closest('.gnstrip').textContent.includes('Session 2')), true);
});

test('with motion Off the Undo line still waits three seconds, then goes without a fold', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS.slice(), dismissed: [] });
  await p.evaluate(() => document.documentElement.setAttribute('data-motion', 'off'));
  await p.click('#cal5-gnbtn');
  await p.waitForSelector(ROW);
  await p.click('#cal5-gnpop [data-gn-skip="e1"]');
  await p.waitForSelector('#cal5-gnpop [data-gn-undo="e1"]');
  await p.waitForSelector('#cal5-gnpop [data-gn-undo-row="e1"]', { state: 'detached', timeout: 6000 });
  assert.equal(await p.$$eval(ROW, (e) => e.length), 1);
});

test('an Undo the server refuses says so in the line and leaves the row hidden', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS.slice(), dismissed: [], restoreStatus: 500 });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector(ROW);
  await p.click('#cal5-gnpop [data-gn-skip="e1"]');
  await p.click('#cal5-gnpop [data-gn-undo="e1"]');
  await p.waitForFunction(() => /Could not undo that/.test(document.querySelector('#cal5-gnpop [data-gn-undo-row="e1"]').textContent));
  assert.equal(await p.$$eval(ROW, (e) => e.length), 1, 'Session 1 is not shown as restored');
  assert.ok(await p.$('#cal5-gnpop [data-gn-undo="e1"]'), 'Undo can be tried again');
  assert.equal(await p.$eval('#cal5-gnpop', (el) => el.hidden), false);
});

test('an old browser-only skip no longer hides anything, so every device agrees', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS });
  await p.evaluate(() => localStorage.setItem('chronicle:gn-skip:c1', JSON.stringify(['e1'])));
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  assert.equal(await p.$$eval('#cal5-gnpop .gnstrip', (e) => e.length), 2);
});

test('a refused Not this one puts the row back and says so', { skip }, async (t) => {
  const dismissed = [];
  const p = await page(t, 1280, { role: 2, candidates: CANDS, dismissed, dismissStatus: 500 });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  await p.click('#cal5-gnpop [data-gn-skip="e1"]');
  await p.waitForSelector('.toast.on');
  assert.match(await p.textContent('.toast'), /Could not hide that one/);
  assert.equal(await p.$$eval('#cal5-gnpop .gnstrip', (e) => e.length), 2, 'the row is back');
});

test('a night with no real date says so, with its in-world day, instead of a dash', { skip }, async (t) => {
  const all = [{ key: 'upcoming', label: 'Upcoming', nights: [
    { sessionId: 's-story', name: 'Session 1', noRealDate: true, when: 'No real date yet · 14 Hammer, 1492', href: '/campaigns/c1/sessions/s-story' },
    { sessionId: 's-real', name: 'Friday', day: '2026-10-30', date: '2026-10-30', when: 'Fri, Oct 30' }] }];
  const p = await page(t, 1280, { all });
  await p.click('#cal5-gnbtn');
  await p.click('#cal5-gnpop [data-gn-all]');
  await p.waitForSelector('#cal5-gnpop [data-gn-next]');
  const row = await p.$eval('#cal5-gnpop a.wrow', (a) => ({ text: a.innerText.replace(/\s+/g, ' ').trim(), tile: a.querySelector('.wtile').textContent.trim(), href: a.getAttribute('href') }));
  assert.match(row.text, /Session 1 No real date yet · 14 Hammer, 1492/);
  assert.notEqual(row.tile, '–', 'no bare dash');
  assert.equal(row.href, '/campaigns/c1/sessions/s-story', 'it opens its own page, where the owner sets the date');
  assert.doesNotMatch(await p.textContent('#cal5-gnpop'), /–/);
});

test('a refused make says why and keeps the strip', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS, makeStatus: 422, makeError: 'the game night could not be made' });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  await p.click('#cal5-gnpop [data-gn-make="e1"]');
  await p.waitForSelector('.toast.on');
  assert.match(await p.textContent('.toast'), /could not be made/);
  assert.equal(await p.$$eval('#cal5-gnpop .gnstrip', (e) => e.length), 2);
  assert.equal(await p.$eval('#cal5-gnpop [data-gn-make="e1"]', (b) => b.disabled), false, 'it can be pressed again');
});

test('the strip fits a phone', { skip }, async (t) => {
  const p = await page(t, 360, { role: 2, candidates: CANDS });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  // The strip itself must sit inside the panel and not overflow it sideways.
  const r = await p.$eval('#cal5-gnpop', (pop) => {
    const pb = pop.getBoundingClientRect();
    const out = [...pop.querySelectorAll('.gnstrip, .gnstrip *')].filter((e) => e.getBoundingClientRect().right > pb.right + 1 || e.getBoundingClientRect().left < pb.left - 1);
    return { over: pop.scrollWidth > pop.clientWidth + 1, out: out.map((e) => e.className) };
  });
  assert.deepEqual(r, { over: false, out: [] });
  const h = await p.$$eval('#cal5-gnpop .gnstrip .lnk', (bs) => bs.map((b) => b.getBoundingClientRect().height));
  assert.ok(h.every((x) => x >= 32), 'controls have room to press: ' + h);
});

test('a night made for a real-life calendar event does not claim it has no real date', { skip }, async (t) => {
  const p = await page(t, 1280, { role: 2, candidates: CANDS, made: [], madeDate: '2026-10-29' });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  await p.click('#cal5-gnpop [data-gn-make="e1"]');
  await p.waitForSelector('.toast');
  const toast = await p.textContent('.toast');
  assert.match(toast, /Made a game night for Session 1\./);
  assert.doesNotMatch(toast, /no real date/);
});

test('a night made from an event shows in the open panel at once, without a reload', { skip }, async (t) => {
  const fresh = night('2026-11-20', 'Session 1 night', false, '');
  const p = await page(t, 1280, { role: 2, candidates: CANDS, made: [], madeDate: '2026-11-20', nights: NIGHTS.slice(), afterMake: fresh });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  assert.doesNotMatch(await p.textContent('#cal5-gnpop'), /Session 1 night/);
  await p.click('#cal5-gnpop [data-gn-make="e1"]');
  await p.waitForSelector('#cal5-gnpop [data-gn-go="s-2026-11-20"]');
  assert.equal(await p.$eval('#cal5-gnpop', (x) => x.hidden), false, 'the panel stays open');
  assert.doesNotMatch(await p.textContent('#cal5-gnpop'), /Nothing planned yet/);
});

const MANY = [1, 2, 3, 4, 5].map((n) => ({ eventId: 'e' + n, name: 'Session ' + n, when: n + ' Hammer, 1492' }));

// A button that is drawn but sits under the Look/Edit deck, or off the edge of
// the screen, cannot be pressed; asking the page what is at its centre is the
// only check that sees both.
async function stripButtonsReachable(p) {
  // The Look/Edit deck belongs to the editor script, which this page does not
  // load; a stand-in with its class and layer sits across where a tall list
  // reaches, so the list has to be above it to pass.
  await p.evaluate(() => {
    const d = document.createElement('div');
    d.className = 'deck';
    d.style.cssText = 'position:absolute;left:0;right:0;top:230px;height:90px';
    document.querySelector('.cal-v5 .cal').appendChild(d);
  });
  await p.click('#cal5-gnbtn');
  await p.waitForSelector('#cal5-gnpop .gnstrip');
  const bad = await p.$$eval('#cal5-gnpop .gnstrip .lnk', (bs) => bs.flatMap((b) => {
    b.scrollIntoView({ block: 'nearest' });
    const r = b.getBoundingClientRect();
    const inside = r.left >= 0 && r.right <= window.innerWidth && r.top >= 0 && r.bottom <= window.innerHeight;
    const at = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    const hit = at === b || b.contains(at);
    return inside && hit ? [] : [b.textContent + ' inside=' + inside + ' hit=' + hit + ' left=' + Math.round(r.left) + ' right=' + Math.round(r.right)];
  }));
  assert.deepEqual(bad, []);
  const pop = await p.$eval('#cal5-gnpop', (e) => { const r = e.getBoundingClientRect(); return { l: r.left, r: r.right, w: window.innerWidth }; });
  assert.ok(pop.l >= 0 && pop.r <= pop.w, 'the panel is inside the screen: ' + JSON.stringify(pop));
}

test('every strip button is inside the screen and on top, on a phone', { skip }, async (t) => {
  await stripButtonsReachable(await page(t, 360, { role: 2, candidates: MANY }));
});

test('every strip button is inside the screen and on top, on a desktop', { skip }, async (t) => {
  await stripButtonsReachable(await page(t, 1280, { role: 2, candidates: MANY }));
});

test('the count of events past the three shown agrees with its number', { skip }, async (t) => {
  for (const [cands, want] of [[MANY.slice(0, 4), '1 more Session event has no game night.'], [MANY, '2 more Session events have no game night.']]) {
    const p = await page(t, 1280, { role: 2, candidates: cands });
    await p.click('#cal5-gnbtn');
    await p.waitForSelector('#cal5-gnpop .gnstrip');
    assert.equal((await p.textContent('#cal5-gnpop .gnstrip-more')).trim(), want);
  }
});
