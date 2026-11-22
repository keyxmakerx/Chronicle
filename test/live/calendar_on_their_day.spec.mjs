// calendar_on_their_day.spec.mjs — an event can reveal itself when its day
// comes. The owner fills the year with "On their day" and keeps it: a player
// reads none of the future ones, and as the date moves forward the player
// sees exactly those dated on or before it. The owner's month says a waiting
// event waits, in its title, and draws a hidden one faded. The chosen days' row sets one to show now, or hides
// it again, and a player sees each change at once.
//
// RUN: tools/live-checks.sh test/live/calendar_on_their_day.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

const listOf = (body) => (Array.isArray(body) ? body : (body && body.data) || []);
const LONG = { timeout: 150000 };
const stamp = (y, m, d) => y * 10000 + m * 100 + d;
const startOf = (e) => stamp(e.year, e.month, e.day);

async function yearEvents(who, base, year, months) {
  const byId = new Map();
  for (let m = 1; m <= months; m++) {
    const r = await who.api(`${base}/events?year=${year}&month=${m}`);
    if (r.ok) listOf(r.body).forEach((e) => byId.set(e.id, e));
  }
  return [...byId.values()];
}

async function harptos(owner, player, tag) {
  const cid = await newCampaign(owner, tag);
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const made = await owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} });
  assert.equal(made.status, 201);
  const cal = made.body.calendar;
  const months = (cal.months || made.body.months || []).length || 16;
  const base = `/campaigns/${cid}/calendars/${cal.id}`;
  await owner.go(`${base}/view`);
  const utc = owner.page.getByRole('button', { name: 'Use UTC time' });
  if (await utc.count()) { await utc.first().click(); await owner.go(`${base}/view`); }
  const fresh = await owner.api(base);
  return { cid, cal: fresh.ok && fresh.body && fresh.body.current_year ? fresh.body : cal, months, base };
}

test('a fill kept "On their day" stays hidden until its day, then shows by itself', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const { cal, months, base } = await harptos(owner, player, 'On Their Day');
  const { page } = owner;
  const y = cal.current_year;
  const today = stamp(y, cal.current_month, cal.current_day);

  await page.locator('#cal5-edittab').click();
  const yearBtn = page.locator('[data-deck-year]');
  await yearBtn.waitFor();
  await yearBtn.click();
  await page.getByRole('button', { name: 'Fill the rest of the year' }).click();
  const keep = page.locator('[data-yt="keep"]');
  await keep.waitFor();

  // The new row defaults to today's behaviour, and changing it keeps the same draft.
  await page.locator('[data-yt="fold:makes"]').click();
  const holding = page.locator('[data-yt="see:later"]');
  assert.equal(await holding.getAttribute('aria-pressed'), 'true');
  const before = await page.locator('.yt-barwrap').innerText();
  await page.locator('[data-yt="see:day"]').click();
  assert.equal(await page.locator('[data-yt="see:day"]').getAttribute('aria-pressed'), 'true');
  const after_ = await page.locator('.yt-barwrap').innerText();
  assert.match(after_, /Players see each event on its day\./);
  assert.equal(after_.replace(' Players see each event on its day.', ''), before, 'choosing when players see it changed the draft');

  await keep.click();
  await keep.waitFor({ state: 'detached', ...LONG });
  await page.locator('#cal5-live').filter({ hasText: /^Kept/ }).waitFor(LONG);

  const kept = await yearEvents(owner, base, y, months);
  assert.ok(kept.length > 0, 'Keep saved no events');
  for (const e of kept) {
    assert.equal(e.visibility, 'everyone', `${e.name} was not saved to show on its day`);
    assert.equal(e.announced, 'on_day');
    assert.equal(JSON.parse(e.payload).type, 'generated');
  }

  // Before its day, a player reads nothing dated after today.
  const early = await yearEvents(player, base, y, months);
  assert.deepEqual(early.filter((e) => startOf(e) > today).map((e) => e.name), [], 'a player can read an event before its day');
  assert.ok(kept.some((e) => startOf(e) > today), 'the fill made nothing after today');

  // The owner's month marks a waiting event, and says so in words.
  await page.locator('#cal5-looktab').click();
  const wait = page.locator('.cal-v5 .mk.wait');
  for (let i = 0; i < 4 && !(await wait.count()); i++) { await page.locator('#cal5-next').click(); await page.waitForTimeout(400); }
  assert.ok(await wait.count(), 'no waiting event is marked on the owner’s month');
  assert.equal(await page.locator('.cal-v5 .mk.wait.dir').count(), 0, 'a waiting mark is also drawn as hidden');
  assert.match(await wait.first().getAttribute('title'), /players see it on the day/);
  await picture(page, 'calendar-waiting-marks', { check: '#2441', about: 'Owner month with events waiting for their day', locator: page.locator('#cal5-cal') });

  // The date moves forward: the player sees exactly what is dated on or before it.
  const cur = cal.current_month;
  const month = cur + Math.max(1, Math.floor((months - cur) / 2));
  assert.ok(month > cur && month < months, `no room to move the date forward from month ${cur} of ${months}`);
  const moved = await owner.api(base, { method: 'PUT', body: { name: cal.name, current_year: y, current_month: month, current_day: 1 } });
  assert.ok(moved.ok, `moving the date answered ${moved.status}`);
  const newDay = stamp(y, month, 1);
  const owned = await yearEvents(owner, base, y, months);
  const now = await yearEvents(player, base, y, months);
  const want = owned.filter((e) => startOf(e) <= newDay).map((e) => e.id).sort();
  assert.ok(want.length > early.length, 'moving the date forward revealed nothing');
  assert.ok(owned.some((e) => startOf(e) > newDay), 'nothing is left waiting after the move');
  assert.deepEqual(now.map((e) => e.id).sort(), want, 'the player does not see exactly what is dated on or before the new date');
  owner.assertClean();
});

test('Show now, Show on the day and Hide on chosen days change what a player reads at once', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const { cal, base } = await harptos(owner, player, 'Chosen Days');
  const { page } = owner;
  const y = cal.current_year, m = cal.current_month, d = cal.current_day;
  // Day 28 is after today and exists in every month of the preset.
  const day = d < 27 ? 28 : null;
  assert.ok(day, 'the calendar is too near the end of its month for this check');
  const made = await owner.api(`${base}/events`, { method: 'POST', body: { name: 'Quiet arrival', year: y, month: m, day, all_day: true, visibility: 'dm_only', announced: 'on_day' } });
  assert.equal(made.status, 201, `creating the event answered ${made.status}`);
  const id = made.body.id;
  const seen = async () => listOf((await player.api(`${base}/events?year=${y}&month=${m}`)).body).some((e) => e.id === id);
  assert.equal(await seen(), false, 'a hidden event is readable');

  await owner.go(`${base}/view`);
  await page.locator('#cal5-edittab').click();
  const cell = page.locator(`.day[data-key="${y}_${m}_${day}"]`);
  await cell.waitFor();
  await cell.click();
  const bar = page.locator('.bbar, [data-bb="hide"]').first();
  await bar.waitFor();
  const words = await page.locator('[data-bb="hide"], [data-bb="onday"], [data-bb="now"]').allInnerTexts();
  assert.deepEqual(words.map((w) => w.trim()), ['Hide', 'Show on the day', 'Show now']);
  assert.equal(await page.locator('[data-bb="reveal"]').count(), 0, 'the old Reveal word is still there');

  const say = page.locator('#cal5-live');
  await page.locator('[data-bb="onday"]').click();
  await say.filter({ hasText: /Set 1 event to show on their day/ }).waitFor();
  let got = (await owner.api(`${base}/events/${id}`)).body;
  assert.equal(got.visibility, 'everyone');
  assert.equal(got.announced, 'on_day');
  assert.equal(await seen(), false, 'an event set to show on its day is readable before its day');
  assert.ok(await page.locator(`.cal-v5 .mk.wait[data-ev="${id}"]`).count(), 'the waiting mark is missing');

  await page.locator('[data-bb="now"]').click();
  await say.filter({ hasText: /Showed 1 event now/ }).waitFor();
  got = (await owner.api(`${base}/events/${id}`)).body;
  assert.equal(got.announced, 'ahead');
  assert.equal(await seen(), true, 'Show now did not show it to the player');
  assert.equal(await page.locator(`.cal-v5 .mk.wait[data-ev="${id}"]`).count(), 0, 'a shown event still carries the waiting mark');

  await page.locator('[data-bb="hide"]').click();
  await say.filter({ hasText: /Hid 1 event/ }).waitFor();
  got = (await owner.api(`${base}/events/${id}`)).body;
  assert.equal(got.visibility, 'dm_only');
  assert.equal(await seen(), false, 'Hide left it readable');
  assert.ok(await page.locator(`.cal-v5 .mk.dir[data-ev="${id}"]`).count(), 'a hidden event is not marked as hidden');
  owner.assertClean();
});
