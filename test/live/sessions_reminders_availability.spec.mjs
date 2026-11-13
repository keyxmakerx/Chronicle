// sessions_reminders_availability.spec.mjs — the player reminder banner, RSVPs,
// the availability grid and its group heatmap behave on a real campaign with
// real player accounts, including the America/Havana spring-forward week.
// Stands in for the operator check in #736.
//
// Covers: the banner shows for an unanswered night, clears once answered, and
// survives sidebar navigation without being rebuilt; the timezone banner offers
// the browser's zone and accepting it stores it; the "not answered" notice
// clears on saving an empty grid; the roster badge and nudge show only while
// somebody is silent and the nudge reaches the bell; an alternating-week
// pattern shows on alternate weeks in the heatmap and the every-week layer is
// hatched under it; the overlay loads in the DST week with a Havana profile
// zone; a player who only sets the availability page's zone gets it used; a
// member who joins after a night exists gets RSVP buttons.
//
// Left for a person: the banner on a real phone, and the real 60 second poll
// (the check reloads instead of waiting for it).
//
// RUN: tools/live-checks.sh test/live/sessions_reminders_availability.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture, BASE } from './_live.mjs';

after(done);

// Two weeks that straddle Cuba's spring-forward (second Sunday of March).
const DST_WEEKS = ['2026-03-02', '2026-03-09'];

const banner = (page) => page.locator('.cta-host [data-cta]');
let w; // the world, built once

async function createNight(owner, cid, name) {
  return owner.page.evaluate(async ({ cid, name }) => {
    const body = new FormData(); body.append('name', name); body.append('scheduled_date', '2031-06-14');
    const r = await Chronicle.apiFetch(`/campaigns/${cid}/sessions`, { method: 'POST', body, headers: { 'HX-Request': 'true' } });
    return ((r.headers.get('HX-Redirect') || '').match(/sessions\/([0-9a-f-]{36})/) || [])[1];
  }, { cid, name });
}

test('the player banner: timezone offer, unanswered night, answer clears it, no flash on navigation', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const havana = await person({ tag: 'hav', name: 'Ines Cobo', context: { timezoneId: 'America/Havana' } });
  const empty = await person({ tag: 'emp', name: 'Tobin Hale' });
  const alt = await person({ tag: 'alt', name: 'Wren Ode' });
  const cid = await newCampaign(owner, 'Reminder Check');
  for (const p of [havana, empty, alt]) await addMember(owner, cid, p, 'player');
  await enableAddons(owner, cid, '*');
  w = { owner, havana, empty, alt, cid };

  // Timezone offer: the browser's own zone, and accepting it stores it.
  await havana.go(`/campaigns/${cid}`);
  const tz = banner(havana.page);
  await tz.waitFor();
  assert.equal(await tz.getAttribute('data-cta'), 'timezone');
  const accept = tz.getByRole('button', { name: /Havana time/ });
  await accept.waitFor();
  await picture(havana.page, 'sessions-timezone-banner', { check: '#736', about: 'Player banner offering the browser zone', mask: ['header'] });
  await accept.click();
  await tz.waitFor({ state: 'detached' });
  await havana.go(`/campaigns/${cid}/availability`);
  assert.equal(await havana.page.locator('[data-availability-root]').getAttribute('data-tz'), 'America/Havana', 'the accepted zone was not stored');

  // An unanswered night makes the RSVP banner.
  w.sid = await createNight(owner, cid, 'The Salt Road');
  assert.ok(w.sid, 'the game night was not created');
  await havana.go(`/campaigns/${cid}`);
  await tz.waitFor();
  assert.equal(await tz.getAttribute('data-cta'), 'rsvp');
  assert.match(await tz.innerText(), /waiting on your answer/);
  await picture(havana.page, 'sessions-rsvp-banner', { check: '#736', about: 'Player banner for an unanswered game night', mask: ['header', '.cta-bar'] });

  // The banner sits outside the page swap, so a sidebar click keeps the same element.
  await havana.page.evaluate(() => { window.__ctaHost = document.querySelector('.cta-host'); window.__ctaBar = document.querySelector('.cta-host [data-cta]'); });
  await havana.page.locator('aside a').filter({ hasText: 'Maps' }).first().click();
  await havana.page.waitForURL(/\/maps/);
  const same = await havana.page.evaluate(() => document.querySelector('.cta-host') === window.__ctaHost && document.querySelector('.cta-host [data-cta]') === window.__ctaBar);
  assert.ok(same, 'the banner was rebuilt by a sidebar navigation');

  // Answering clears it.
  await tz.getByRole('link', { name: 'Answer now' }).click();
  await havana.page.waitForURL(/\/sessions\//);
  // Wait for the saved answer, not for the word "Going" on the page, which
  // can be there before the answer is stored.
  const answered = havana.page.waitForResponse((r) => /\/sessions\/[^/]+\/rsvp$/.test(new URL(r.url()).pathname) && r.request().method() === 'POST');
  await havana.page.getByRole('button', { name: /Going/ }).click();
  assert.ok((await answered).ok(), 'the answer was not saved');
  await havana.go(`/campaigns/${cid}`);
  await havana.page.locator('.cta-host').waitFor({ state: 'attached' });
  await havana.page.waitForLoadState('networkidle');
  assert.equal(await banner(havana.page).count(), 0, 'the banner stayed after the player answered');
  havana.assertClean();
});

test('a member who joins after the night was made still gets RSVP buttons', async () => {
  const late = await person({ tag: 'late', name: 'Pell Rook' });
  await addMember(w.owner, w.cid, late, 'player');
  await late.go(`/campaigns/${w.cid}/sessions/${w.sid}`);
  for (const label of [/Going/, /Maybe/, /Can't/]) {
    assert.ok(await late.page.getByRole('button', { name: label }).count(), `no ${label} button for a late joiner`);
  }
  await late.page.getByRole('button', { name: /Going/ }).click();
  await late.page.locator('#attendee-list').getByText('Pell Rook').waitFor();
  w.late = late;
  late.assertClean();
});

test('the unanswered notice, roster badge and nudge come and go as people answer', async () => {
  const { owner, empty, alt, havana, late, cid } = w;
  // A player who never saved sees the notice; saving an empty grid clears it.
  await empty.go(`/campaigns/${cid}/availability`);
  const notice = empty.page.locator('[data-avail-unanswered]');
  await notice.waitFor({ state: 'visible' });
  await empty.page.getByRole('button', { name: /Save/ }).first().click();
  await empty.page.getByText('Saved.').waitFor();
  await notice.waitFor({ state: 'hidden' });

  // The Director's roster: badge and nudge while somebody is silent.
  await owner.go(`/campaigns/${cid}/availability?tab=overlay`);
  await owner.page.locator('.ov-weekgrid').waitFor();
  const badges = owner.page.locator('.avail-silent');
  assert.ok(await badges.count() >= 3, 'silent members carry no badge');
  const emptyChip = owner.page.locator('.avail-chip').filter({ hasText: 'Tobin Hale' });
  assert.equal(await emptyChip.locator('.avail-silent').count(), 0, 'a member who saved an empty grid still shows as silent');
  const nudge = owner.page.getByRole('button', { name: /Ask the \d+ who have not answered/ });
  await nudge.click();
  await owner.page.getByText(/^Asked /).waitFor();
  await picture(owner.page, 'sessions-availability-roster', { check: '#736', about: 'Roster chips with not-answered badges and the nudge', mask: ['header'] });

  // The nudge lands in a silent player's bell; the answered one gets nothing.
  const bell = async (who) => (await who.api('/notifications')).body.map((n) => n.type);
  assert.ok((await bell(alt)).includes('availability_nudge'), 'the silent player got no nudge');
  assert.ok(!(await bell(empty)).includes('availability_nudge'), 'a player who answered was nudged');
  await alt.go(`/campaigns/${cid}`);
  assert.ok(await alt.page.locator('[data-notif-badge] [data-count]').count(), 'the bell has no count');

  // Once everyone has answered, the badges and the nudge are gone.
  for (const who of [owner, havana, alt, late]) {
    const r = await who.api(`/campaigns/${cid}/availability/mine`, { method: 'PUT', body: { tz: 'UTC', blocks: [] } });
    assert.ok(r.ok, `saving answered ${r.status}`);
  }
  await owner.go(`/campaigns/${cid}/availability?tab=overlay`);
  await owner.page.locator('.ov-weekgrid').waitFor();
  assert.equal(await owner.page.locator('.avail-silent').count(), 0);
  assert.equal(await owner.page.getByRole('button', { name: /who have not answered/ }).count(), 0, 'the nudge stays with nobody silent');
  owner.assertClean();
});

test('an alternating-week pattern shows on alternate weeks, hatched over the every-week layer', async () => {
  const { owner, alt, cid } = w;
  await alt.go(`/campaigns/${cid}/availability`);
  const mine = (await alt.api(`/campaigns/${cid}/availability/mine`)).body;
  assert.ok(mine.weekALabel && mine.weekBLabel, 'no alternating-week dates offered');
  // Thursday evening on track A only, Saturday every week.
  const save = await alt.api(`/campaigns/${cid}/availability/mine`, { method: 'PUT', body: { tz: 'UTC', blocks: [
    { dayOfWeek: 4, startMinute: 1140, endMinute: 1320, state: 'available', weekCadence: 1 },
    { dayOfWeek: 6, startMinute: 600, endMinute: 780, state: 'available', weekCadence: 0 },
  ] } });
  assert.ok(save.ok);
  const freeThursday = async (weekStart) => {
    const o = (await owner.api(`/campaigns/${cid}/availability/overlay?week=${weekStart}`)).body;
    const day = o.days.find((d) => d.weekday === 4);
    return { thursday: day.hours[19].free, saturday: o.days.find((d) => d.weekday === 6).hours[11].free };
  };
  const weekA = mine.weekALabel; // a Sunday; the Monday after starts the week
  const next = (iso, days) => new Date(Date.parse(iso + 'T00:00:00Z') + days * 86400000).toISOString().slice(0, 10);
  const a = await freeThursday(next(weekA, 1));
  const b = await freeThursday(next(weekA, 8));
  assert.notEqual(a.thursday, b.thursday, 'the alternating Thursday did not alternate');
  assert.equal(a.thursday + b.thursday, 1, 'Thursday is free on exactly one of the two weeks');
  assert.equal(a.saturday, 1); assert.equal(b.saturday, 1, 'the every-week Saturday is missing on a week');

  // Editing the alternate track hatches the every-week layer underneath.
  await alt.go(`/campaigns/${cid}/availability`);
  await alt.page.getByRole('button', { name: /^Week of / }).first().click();
  await alt.page.locator('.avail-cell[data-base="1"]:not([data-state])').first().waitFor();
  await picture(alt.page, 'sessions-availability-alternate', { check: '#736', about: 'Editing an alternate week: the every-week hours hatched underneath', mask: ['header', '[data-tz-label]'] });
  alt.assertClean();
});

test('the group heatmap loads in the Havana spring-forward week, and a zone set on the page is used', async () => {
  const { owner, havana, empty, cid } = w;
  // Profile zone is America/Havana (accepted from the banner above): save there.
  const save = await havana.api(`/campaigns/${cid}/availability/mine`, { method: 'PUT', body: { tz: 'America/Havana', blocks: [
    { dayOfWeek: 0, startMinute: 0, endMinute: 1440, state: 'available', weekCadence: 0 },
    { dayOfWeek: 6, startMinute: 1200, endMinute: 1440, state: 'preferred', weekCadence: 0 },
  ] } });
  assert.ok(save.ok);
  for (const week of DST_WEEKS) {
    for (const who of [owner, havana]) {
      const t0 = Date.now();
      const r = await who.api(`/campaigns/${cid}/availability/overlay?week=${week}`);
      assert.equal(r.status, 200, `overlay for ${week} answered ${r.status}`);
      assert.equal(r.body.days.length, 7);
      assert.ok(r.body.days.every((d) => d.hours.length === 24), 'a day has the wrong number of hours');
      assert.ok(Date.now() - t0 < 8000, `overlay for ${week} was slow (a DST hang)`);
    }
  }

  // The same weeks through the page itself: the grid draws.
  await havana.page.route('**/availability/overlay*', (route) => {
    const u = new URL(route.request().url()); u.searchParams.set('week', DST_WEEKS[0]); route.continue({ url: u.toString() });
  });
  await havana.go(`/campaigns/${cid}/availability?tab=overlay`);
  await havana.page.locator('.ov-weekgrid .ov-daycol').first().waitFor();
  assert.equal(await havana.page.locator('.ov-weekgrid .ov-daycol').count(), 7);
  await havana.page.unroute('**/availability/overlay*');
  await picture(havana.page, 'sessions-availability-havana-week', { check: '#736', about: 'Group heatmap for a player whose zone is America/Havana', mask: ['header', '[data-week-label]', '[data-ov-grid]'] });

  // A player who sets only the availability page's zone gets it used.
  await empty.go(`/campaigns/${cid}/availability`);
  await empty.page.locator('select').filter({ has: empty.page.locator('option[value="Europe/Lisbon"]') }).first().selectOption('Europe/Lisbon');
  // An empty grid stores no zone, so one hour is painted before saving.
  // A drag over two cells; a single click paints and then un-paints (see below).
  await empty.page.waitForLoadState('networkidle');
  const first = empty.page.locator('.avail-cell').nth(20 * 7 + 3);
  await first.scrollIntoViewIfNeeded();
  await first.hover(); await empty.page.mouse.down();
  await empty.page.locator('.avail-cell').nth(21 * 7 + 3).hover(); await empty.page.mouse.up();
  await empty.page.getByRole('button', { name: /Save/ }).first().click();
  await empty.page.getByText('Saved.').waitFor();
  const mine = (await empty.api(`/campaigns/${cid}/availability/mine`)).body;
  assert.equal(mine.tz, 'Europe/Lisbon', 'the zone picked on the page was not used');
  havana.assertClean();
});

test('one click on an empty grid cell leaves it painted', { todo: 'a single click paints on mousedown and the click handler toggles it straight back off' }, async () => {
  const { empty, cid } = w;
  await empty.go(`/campaigns/${cid}/availability`);
  await empty.page.waitForLoadState('networkidle');
  const cell = empty.page.locator('.avail-cell').nth(10 * 7 + 1);
  await cell.scrollIntoViewIfNeeded();
  await cell.click();
  assert.equal(await cell.getAttribute('data-state'), 'available');
});
