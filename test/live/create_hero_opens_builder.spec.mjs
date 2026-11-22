// create_hero_opens_builder.spec.mjs — one way to make a hero.
//
// The Characters page's Create hero opens the hero builder, for the campaign
// owner making a hero for the table and for a player making their own. The
// page offers no other new-hero form: the old one-sentence form is gone, and
// a sentence left in the browser does not bring it back.
//
// RUN: tools/live-checks.sh test/live/create_hero_opens_builder.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

async function pressCreateHero(who, cid, tag) {
  const { page } = who;
  // A sentence a browser kept from the old Characters page form.
  await who.go(`/campaigns/${cid}/characters`);
  await page.evaluate((cid) => localStorage.setItem('chronicle:hero-builder:' + cid, JSON.stringify({ name: 'Kept', values: {}, open: true })), cid);
  await who.go(`/campaigns/${cid}/characters`);
  assert.equal(await page.locator('[data-hero-card="new"], [data-make]').count(), 0, tag + ': no new-hero sentence on the Characters page');
  const button = page.locator('[data-create-hero]');
  await button.waitFor();
  await Promise.all([page.waitForURL(new RegExp(`/campaigns/${cid}/characters/new$`)), button.click()]);
  await page.locator('.hc-app').waitFor();
  await picture(page, 'create-hero-' + tag);
  who.assertClean();
}

test('Create hero on the Characters page opens the builder, for the owner and for a player', async () => {
  const owner = await person({ tag: 'hero-owner' });
  const player = await person({ tag: 'hero-player' });
  const cid = await newCampaign(owner, 'One way to make a hero');
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, player, 'player');
  await pressCreateHero(owner, cid, 'owner');
  await pressCreateHero(player, cid, 'player');
});
