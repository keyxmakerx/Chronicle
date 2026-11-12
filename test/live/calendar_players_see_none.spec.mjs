// calendar_players_see_none.spec.mjs — a player who can see no calendar gets
// no Calendar row in the sidebar and an honest empty page, and a public
// campaign's finished game night shows no recap to signed-out visitors.
// Stands in for the operator check in #1487.
//
// Covers steps 2, 3, 4 and 5. Step 1 (set every calendar hidden in the
// Calendar app) cannot be clicked: no screen or API writes a calendar's own
// visibility today, only a campaign import carries it, so the hidden calendar
// is made by importing a campaign export edited to say "Director only", and
// step 4 ("share it again") is checked on a campaign whose calendar is shared.
// "View as player" is not used; a real player account is.
//
// RUN: tools/live-checks.sh test/live/calendar_players_see_none.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture, BASE, PASSWORD } from './_live.mjs';

after(done);

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
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
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
  const empty = player.page.locator('.calv5-empty');
  await empty.waitFor();
  assert.match(await empty.innerText(), /No calendars shared with you/);
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

test('the header subtitle for a player who sees none does not claim there are none', { todo: 'the subtitle reads "No calendars yet." for a player whose calendars are all hidden' }, () => {
  assert.equal(hiddenCampaign.subtitleShown, false);
});

test('a public campaign shows no recap box to a signed-out visitor, but does to a member', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
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
