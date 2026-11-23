// calendar_on_their_day.spec.mjs: three calendar walks that share one owner and
// one player, so each pays for its own steps and not for new accounts.
//
// On their day: an event can reveal itself when its day comes. The owner fills
// the year with "On their day" and keeps it: a player reads none of the future
// ones, and as the date moves forward the player sees exactly those dated on or
// before it. The owner's month says a waiting event waits, in its title, and
// draws a hidden one faded. The chosen days' row sets one to show now, or hides
// it again, and a player sees each change at once.
//
// Year track: the Edit drawer's year track: an owner fills the rest of the year
// and keeps it, what is saved is director-only and marked as generated, a
// player sees none of it, Undo takes it all away again, and a second fill
// replaces the first instead of stacking on it.
//
// Players who see none: a player who can see no calendar gets no Calendar row
// in the sidebar and an honest empty page, and a public campaign's finished
// game night shows no recap to signed-out visitors. Stands in for the operator
// check in #1487, steps 2, 3, 4 and 5. Step 1 (set every calendar hidden in the
// Calendar app) cannot be clicked: no screen or API writes a calendar's own
// visibility today, only a campaign import carries it, so the hidden calendar
// is made by importing a campaign export edited to say "Director only", and
// step 4 ("share it again") is checked on a campaign whose calendar is shared.
// "View as player" is not used; a real player account is.
//
// RUN: tools/live-checks.sh test/live/calendar_on_their_day.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture, once, resetErrors, BASE } from './_live.mjs';

after(done);

const listOf = (body) => (Array.isArray(body) ? body : (body && body.data) || []);
const LONG = { timeout: 150000 };
const stamp = (y, m, d) => y * 10000 + m * 100 + d;
const startOf = (e) => stamp(e.year, e.month, e.day);

// Every event of the calendar's current year as `who` is allowed to read it. A
// repeating event is listed under every month it lands in, so each is counted once.
async function yearEvents(who, base, year, months) {
  const byId = new Map();
  for (let m = 1; m <= months; m++) {
    const r = await who.api(`${base}/events?year=${year}&month=${m}`);
    if (r.ok) listOf(r.body).forEach((e) => byId.set(e.id, e));
  }
  return [...byId.values()];
}

// The one owner, one player and campaign the Harptos walks share. Each walk
// makes its own calendar in it, so what one keeps is never read by another.
const world = once(async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Calendar Walks');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  return { owner, player, cid };
});

beforeEach(async () => { const w = await world(); resetErrors(w.owner, w.player); });

async function harptos(owner, cid) {
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
  const { owner, player, cid } = await world();
  const { cal, months, base } = await harptos(owner, cid);
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
  const { owner, player, cid } = await world();
  const { cal, base } = await harptos(owner, cid);
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

test('fill the rest of the year, keep it, see it director-only, and undo it', async () => {
  const { owner, player, cid } = await world();
  const { cal, months, base } = await harptos(owner, cid);
  const { page } = owner;
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'the new calendar already has events');

  await page.locator('#cal5-edittab').click();
  const yearBtn = page.locator('[data-deck-year]');
  await yearBtn.waitFor();
  assert.equal(await yearBtn.getAttribute('aria-expanded'), 'false');
  await yearBtn.click();
  assert.equal(await yearBtn.getAttribute('aria-expanded'), 'true');

  await page.getByRole('button', { name: 'Fill the rest of the year' }).click();
  const keep = page.locator('[data-yt="keep"]');
  await keep.waitFor();
  assert.match(await page.locator('.yt-barwrap').innerText(), /Nothing is saved until you keep it\./);
  assert.equal(await page.locator('[data-deck-year]').isDisabled(), true, 'The year can still be pressed while a draft exists');
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'the draft was saved before Keep');
  await picture(page, 'year-track-draft', { check: '#2222', about: 'Edit drawer with a fill draft', mask: ['header'] });

  // Every write the page makes, so Keep and Undo can be shown to be one
  // request each and not one per event.
  const writes = [];
  page.on('response', (r) => {
    const q = r.request();
    if (q.method() !== 'GET') writes.push(`${q.method()} ${new URL(r.url()).pathname.replace(base, '').replace(/[0-9a-f-]{36}/g, ':id')} ${r.status()}`);
  });
  // Keep is done when the draft bar has gone. An Undo button alone does not
  // say so: the one from an earlier Keep can still be on screen.
  const keptAll = async () => {
    const live = page.locator('#cal5-live');
    await keep.waitFor({ state: 'detached' });
    await live.filter({ hasText: /^Kept/ }).waitFor();
    const mine = writes.splice(0);
    assert.deepEqual(mine, ['POST /events/batch 200'], `Keep was not one request: ${mine.join(', ')}`);
  };
  await keep.click();
  await keptAll();
  const undo = page.getByRole('button', { name: 'Undo' });
  await undo.waitFor();
  assert.match(await page.locator('body').innerText(), /Kept \d+ events?/);

  const kept = await yearEvents(owner, base, cal.current_year, months);
  assert.ok(kept.length > 0, 'Keep saved no events');
  for (const e of kept) {
    assert.equal(e.visibility, 'dm_only', `${e.name} was saved visible to players`);
    const p = JSON.parse(e.payload);
    assert.equal(p.type, 'generated');
    assert.ok(['events', 'sky'].includes(p.lane), `unexpected lane ${p.lane}`);
    assert.ok(p.sig, 'no sig');
  }
  // A fill covers today to the year's end: nothing it keeps starts before the
  // calendar's current day or in another year, however the comet's span fell.
  const today = (await owner.api(base)).body;
  const stampOf = (y, m, d) => y * 10000 + m * 100 + d;
  const firstDay = stampOf(today.current_year, today.current_month, today.current_day);
  for (const e of kept) {
    assert.equal(e.year, today.current_year, `${e.name} starts in ${e.year}, not the current year`);
    assert.ok(stampOf(e.year, e.month, e.day) >= firstDay, `${e.name} starts ${e.year}-${e.month}-${e.day}, before today ${firstDay}`);
  }
  const seen = await yearEvents(player, base, cal.current_year, months);
  assert.equal(seen.length, 0, 'a player can read generated events');

  writes.length = 0;
  await undo.click();
  await page.locator('#cal5-live').filter({ hasText: 'Undone.' }).waitFor();
  assert.deepEqual(writes, ['POST /events/batch 200'], `Undo was not one request: ${writes.join(', ')}`);
  writes.length = 0;
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'Undo left events behind');

  // A second fill replaces the first one's events instead of stacking on them.
  const fill = page.getByRole('button', { name: 'Fill the rest of the year' });
  await fill.click();
  await keep.waitFor();
  await keep.click();
  await keptAll();
  const first = await yearEvents(owner, base, cal.current_year, months);
  assert.ok(first.length > 0, 'the first fill saved no events');
  await fill.click();
  await keep.waitFor();
  const said = await page.locator('.yt-barwrap').innerText();
  const m = /It replaces (\d+) events? from an earlier fill\./.exec(said);
  assert.ok(m, `the second fill does not say what it replaces: ${said}`);
  assert.equal(Number(m[1]), first.length, 'the second fill does not replace all of the first');
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, first.length, 'the second draft changed what is saved before Keep');
  await keep.click();
  await keptAll();
  const second = await yearEvents(owner, base, cal.current_year, months);
  const firstIds = new Set(first.map((e) => e.id));
  assert.equal(second.filter((e) => firstIds.has(e.id)).length, 0, 'events of the first fill are still there after the second');
  assert.ok(second.length > 0, 'the second fill saved no events');

  // Undoing the second fill puts the first back, still director-only and still marked.
  await undo.click();
  await page.locator('#cal5-live').filter({ hasText: 'Undone.' }).waitFor();
  const back = await yearEvents(owner, base, cal.current_year, months);
  assert.equal(back.length, first.length, 'Undo did not put the first fill back');
  for (const e of back) {
    assert.equal(e.visibility, 'dm_only', `${e.name} came back visible to players`);
    assert.equal(JSON.parse(e.payload).type, 'generated');
  }
  assert.equal((await yearEvents(player, base, cal.current_year, months)).length, 0, 'a player can read generated events after Undo');
  owner.assertClean();
});

const calendarRow = (page) => page.locator('aside a, nav a').filter({ hasText: /^\s*Calendar\s*$/ });

// Export `cid`, optionally hide its calendar, and import it as a new campaign.
async function importCopy(owner, cid, name, visibility) {
  const { page } = owner;
  const json = JSON.parse(await (await page.request.get(BASE + `/campaigns/${cid}/export`)).text());
  json.calendar.visibility = visibility;
  json.campaign.name = name;
  await owner.go('/campaigns/import');
  await page.setInputFiles('#import-file', { name: 'campaign.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(json)) });
  await page.locator('form:has(#import-file) button[type=submit]').first().click();
  await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}(?!\/import)/, { timeout: 20000 });
  await page.waitForLoadState('networkidle');
  const id = (page.url().match(/\/campaigns\/([0-9a-f-]{36})/) || [])[1];
  assert.ok(id, 'the import did not land on a campaign');
  return id;
}

let hiddenCampaign = {};

test('a player with no visible calendar loses the sidebar row and sees an honest empty page', async () => {
  const { owner, player } = await world();
  const base = await newCampaign(owner, 'Calendar source');
  await enableAddons(owner, base, '*');
  const made = await owner.api(`/campaigns/${base}/calendars/presets/blank`, { method: 'POST', body: {} });
  assert.equal(made.status, 201);

  const hidden = await importCopy(owner, base, 'Hidden Calendar', 'dm_only');
  const shared = await importCopy(owner, base, 'Shared Calendar', 'everyone');
  await addMember(owner, hidden, player, 'player');
  await addMember(owner, shared, player, 'player');

  // Step 2: hidden everywhere, so no Calendar row; Game nights is untouched.
  await player.go(`/campaigns/${hidden}`);
  assert.equal(await calendarRow(player.page).count(), 0, 'a Calendar row is still in the player sidebar');
  assert.ok(await player.page.locator('aside a, nav a').filter({ hasText: 'Game nights' }).count(), 'the sidebar lost more than the Calendar row');
  await picture(player.page, 'calendar-player-sidebar-hidden', { check: '#1487', about: 'Player sidebar with every calendar hidden: no Calendar row', mask: ['header'] });

  // Step 3: the page says nothing is shared, not that none was set up.
  await player.go(`/campaigns/${hidden}/calendars`);
  const empty = player.page.locator('[data-app-opening="empty"]');
  await empty.waitFor();
  assert.match(await empty.innerText(), /shares a calendar with you/i);
  assert.doesNotMatch(await empty.innerText(), /No calendars yet|set up/);
  // The header's one-line subtitle still reads "No calendars yet." for a
  // player, which says the same thing the empty box was fixed not to.
  const subtitle = player.page.getByText('No calendars yet.', { exact: true });
  hiddenCampaign = { subtitleShown: await subtitle.count() > 0 };
  await picture(player.page, 'calendar-player-none-shared', { check: '#1487', about: 'Calendars page for a player with none shared', mask: ['header'] });

  // The owner still sees it, labelled Director only.
  await owner.go(`/campaigns/${hidden}/calendars`);
  assert.match(await owner.page.locator('.calv5-ccard').first().innerText(), /Director only/);

  // Step 4: a shared calendar brings the row back.
  await player.go(`/campaigns/${shared}`);
  assert.equal(await calendarRow(player.page).count(), 1, 'the Calendar row did not come back when a calendar is shared');
  player.assertClean();
});

test('the header subtitle for a player who sees none does not claim there are none', () => {
  assert.equal(hiddenCampaign.subtitleShown, false);
});

test('a public campaign shows no recap box to a signed-out visitor, but does to a member', async () => {
  const { owner, player } = await world();
  const guest = await person({ tag: 'guest', signIn: false });
  const cid = await newCampaign(owner, 'Open Table');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  await owner.go(`/campaigns/${cid}/settings`);
  const pub = await owner.api(`/campaigns/${cid}`, { method: 'PUT', form: { name: 'Open Table', is_public: 'true' } });
  assert.ok(pub.ok, `making the campaign public answered ${pub.status}`);

  // A finished night: the Session Recap box is open for members once it is played.
  // The create call answers with a redirect header naming the new night.
  const sid = await owner.page.evaluate(async (cid) => {
    const body = new FormData(); body.append('name', 'The Drowned Gate'); body.append('scheduled_date', '2026-01-10');
    const r = await Chronicle.apiFetch(`/campaigns/${cid}/sessions`, { method: 'POST', body, headers: { 'HX-Request': 'true' } });
    const m = (r.headers.get('HX-Redirect') || '').match(/sessions\/([0-9a-f-]{36})/); return m && m[1];
  }, cid);
  assert.ok(sid, 'could not find the game night id');
  const done_ = await owner.api(`/campaigns/${cid}/sessions/${sid}`, { method: 'PUT', body: { status: 'completed' } });
  assert.ok(done_.ok, `completing the night answered ${done_.status}`);

  await owner.go(`/campaigns/${cid}/sessions/${sid}`);
  assert.ok(await owner.page.locator('#session-recap').count(), 'the Director does not see the recap box');
  await player.go(`/campaigns/${cid}/sessions/${sid}`);
  assert.ok(await player.page.locator('#session-recap').count(), 'a member does not see the recap box');

  const res = await guest.page.goto(`${BASE}/campaigns/${cid}/sessions/${sid}`, { waitUntil: 'networkidle' });
  assert.equal(res.status(), 200, 'a public campaign night did not open for a visitor');
  assert.match(await guest.page.locator('body').innerText(), /The Drowned Gate/);
  assert.equal(await guest.page.locator('#session-recap').count(), 0, 'a signed-out visitor sees the recap box');
  assert.doesNotMatch(await guest.page.locator('main').innerText(), /Recap/);
  await picture(guest.page, 'calendar-public-night-no-recap', { check: '#1487', about: 'Finished game night seen signed out: no recap box', mask: ['header'] });
});
