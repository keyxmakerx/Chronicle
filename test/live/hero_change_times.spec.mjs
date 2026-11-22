// hero_change_times.spec.mjs — when players may change their hero's picks.
//
// A Director sets the times on the Characters page: after making a hero,
// after a new level, or both, and can open one hero's picks for some days.
// The page says the rule in a sentence and each claimed hero's state. A
// player sees none of it and may not set it.
//
// RUN: tools/live-checks.sh test/live/hero_change_times.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

test('Changing heroes: a Director sets the times and opens one hero; a player cannot', async () => {
  const owner = await person({ tag: 'hc-owner' });
  const player = await person({ tag: 'hc-player' });
  const cid = await newCampaign(owner, 'Changing heroes');
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, player, 'player');

  const made = await player.api(`/campaigns/${cid}/characters/new`, { method: 'POST', body: { name: 'Wren' } });
  assert.ok(made.ok, 'player makes a hero; answered ' + made.status + ' ' + JSON.stringify(made.body));

  const { page } = owner;
  await owner.go(`/campaigns/${cid}/characters`);
  const section = page.locator('[data-hero-changes]');
  await section.waitFor();
  assert.match(await section.locator('[data-hc-summary]').innerText(), /^Players can change their hero's picks any time\./);
  assert.equal(await section.locator('[data-hc-row]').count(), 0, 'no hero list while any time');

  await section.locator('summary', { hasText: 'Change when' }).click();
  await section.locator('input[name="when"][value="set"]').check();
  await section.locator('input[name="after_level"]').check();
  await section.locator('input[name="level_days"]').fill('8');
  await section.locator('input[name="from_level"]').fill('2');
  await picture(page, 'hero-change-times-form');
  await section.locator('button[type="submit"]', { hasText: 'Save' }).click();
  await page.locator('[data-hc-summary]', { hasText: 'for 8 days after each new level' }).waitFor();

  const row = page.locator('[data-hero-changes] [data-hc-row]', { hasText: 'Wren' });
  await row.waitFor();
  assert.match(await row.innerText(), /Locked until level 2/);
  await row.locator('input[name="days"]').fill('5');
  await row.locator('button', { hasText: 'Open picks' }).click();
  await page.locator('[data-hero-changes] [data-hc-row]', { hasText: 'reopened by a Director' }).waitFor();
  assert.match(await page.locator('[data-hero-changes] [data-hc-row]', { hasText: 'Wren' }).innerText(), /Open 5 days more/);
  await picture(page, 'hero-change-times-list');

  const bad = await owner.api(`/campaigns/${cid}/characters/hero-changes`, { method: 'POST', form: { when: 'set' } });
  assert.ok(bad.ok, 'a rule that opens nothing is answered with the page and a line, not an error');

  await player.go(`/campaigns/${cid}/characters`);
  assert.equal(await player.page.locator('[data-hero-changes]').count(), 0, 'a player sees no Director section');
  const refused = await player.api(`/campaigns/${cid}/characters/hero-changes`, { method: 'POST', form: { when: 'any' } });
  assert.equal(refused.status, 403, 'a player may not set the times');
  owner.assertClean();
  player.assertClean();
});
