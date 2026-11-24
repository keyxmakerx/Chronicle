// account_campaign_settings.spec.mjs — Account settings' "Your campaigns":
// each campaign the person belongs to has its own page there, reached from
// the rail and from the picture menu, with Your look and the dice in place.
//
// Checks: the rail lists the campaign and marks the open one; a look pick made
// there saves through the owner's limits and shows on the old Your look page
// too; a dice choice there is the one the Table reads (same browser storage);
// the owner gets Customize and the brush instead of picks; a stranger is
// refused; leaving from the page goes back to Account settings.
//
// RUN: tools/live-checks.sh test/live/account_campaign_settings.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, picture } from './_live.mjs';

after(done);

// Opens Pages to every member, through Customize's People room.
async function openPagesToEveryone(owner, cid) {
  await owner.go(`/campaigns/${cid}/customize`);
  await owner.go(`/campaigns/${cid}/customize/look#people`);
  await owner.page.locator('.pp-row').filter({ hasText: 'Pages' }).first().click();
  await owner.page.locator('input[name="ac-page"][value="everyone"]').check({ force: true });
  await owner.page.waitForTimeout(1500);
  await owner.page.locator('[data-czs-save]').click();
  await owner.page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
}

test('a player finds and changes one campaign\'s settings under Account settings', async () => {
  const owner = await person({ tag: 'acs-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Ashen Reach');
  await openPagesToEveryone(owner, cid);
  const player = await person({ tag: 'acs-player', name: 'Mara Vell' });
  await addMember(owner, cid, player, 'player');

  // The picture menu inside the campaign leads to the page.
  await player.go(`/campaigns/${cid}`);
  const href = await player.page.locator(`a[href="/account/campaigns/${cid}"]`).first().getAttribute('href');
  assert.equal(href, `/account/campaigns/${cid}`, 'the picture menu offers this campaign\'s settings');

  // The account page's rail lists the campaign.
  await player.go('/account');
  await player.page.locator('.acct-rail').getByRole('link', { name: 'Ashen Reach' }).click();
  await player.page.waitForURL(new RegExp(`/account/campaigns/${cid}$`));
  const rail = player.page.locator('.acct-rail a[aria-current="page"]');
  assert.equal((await rail.textContent()).trim(), 'Ashen Reach', 'the open campaign is marked in the rail');

  // Your look, in place.
  const pages = player.page.locator('[data-look-part="page"]');
  await pages.waitFor();
  const pick = pages.locator('button[data-value]:not([data-value=""])').first();
  const value = await pick.getAttribute('data-value');
  await Promise.all([player.page.waitForEvent('load'), pick.click()]);
  assert.equal(await player.page.locator(`[data-look-part="page"] button[data-value="${value}"]`).getAttribute('aria-pressed'), 'true', 'the pick is saved and shown');
  await player.go(`/campaigns/${cid}/my-look`);
  assert.equal(await player.page.locator(`[data-look-part="page"] button[data-value="${value}"]`).getAttribute('aria-pressed'), 'true', 'Your look shows the same pick');

  // Dice: the same storage the Table reads.
  await player.go(`/account/campaigns/${cid}`);
  const rune = player.page.locator('[data-acct-look="rune"]');
  await rune.waitFor();
  await rune.click();
  await player.page.locator('[data-acct-look="rune"][aria-pressed="true"]').waitFor();
  const stored = await player.page.evaluate(() => Object.keys(localStorage).filter((k) => k.startsWith('chronicle.table.look.')).map((k) => localStorage.getItem(k)));
  assert.deepEqual(stored, ['rune'], 'the Table\'s look key holds the choice');
  await player.page.locator('[data-acct-look-reset]').waitFor();

  // The calendar link is made when asked for.
  await player.page.locator('[data-acct-cal-show]').click();
  const link = await player.page.locator('[data-acct-cal-url]').inputValue();
  assert.match(link, /\/sessions\/feed\/[A-Za-z0-9_-]+\.ics$/, 'a private calendar link is shown');

  await picture(player.page, 'account-campaign-player-light', { about: 'A campaign\'s page in Account settings, as a player', locator: player.page.locator('.acct') });
  player.assertClean();

  // Phone, dark.
  const phone = await person({ email: player.email, password: player.password, register: false, context: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, colorScheme: 'dark' } });
  await phone.go('/account');
  await phone.page.locator('#my-view').getByRole('button', { name: 'Dark', exact: true }).click();
  await phone.go(`/account/campaigns/${cid}`);
  await phone.page.locator('[data-acct-look]').first().waitFor();
  const wide = await phone.page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  assert.ok(wide <= 1, `no sideways scroll on a phone (over by ${wide}px)`);
  await picture(phone.page, 'account-campaign-player-phone-dark', { about: 'The same page on a phone, dark', locator: phone.page.locator('.acct') });
  await phone.go('/account');
  await phone.page.locator('#my-view').getByRole('button', { name: 'Light', exact: true }).click();

  // The owner gets Customize and the brush, not picks.
  await owner.go(`/account/campaigns/${cid}`);
  await owner.page.locator(`a[href="/campaigns/${cid}/customize/look"]`).waitFor();
  assert.equal(await owner.page.locator('[data-look-part]').count(), 0, 'the owner has no personal picks');
  await owner.page.locator('[data-acct-brush] [data-brush="hidden"]').click();
  await owner.page.locator('[data-acct-brush] [data-brush="hidden"][aria-pressed="true"]').waitFor();
  await owner.go(`/campaigns/${cid}`);
  assert.equal(await owner.page.locator('[data-widget="customize-brush"] [data-brush-toggle]').count(), 0, 'the brush is put away in the header');
  await owner.go(`/account/campaigns/${cid}`);
  await owner.page.locator('[data-acct-brush] [data-brush="shown"]').click();
  await owner.page.locator('[data-acct-brush] [data-brush="shown"][aria-pressed="true"]').waitFor();
  await picture(owner.page, 'account-campaign-owner-light', { about: 'A campaign\'s page in Account settings, as its owner', locator: owner.page.locator('.acct') });
  owner.assertClean();

  // A stranger is refused.
  const stranger = await person({ tag: 'acs-stranger' });
  const res = await stranger.page.goto(`${process.env.LIVE_BASE || 'http://localhost:8080'}/account/campaigns/${cid}`);
  assert.ok([403, 404].includes(res.status()), `a stranger is refused (got ${res.status()})`);

  // Leaving from the page goes back to Account settings.
  await player.go(`/account/campaigns/${cid}`);
  await player.page.getByRole('button', { name: /^Leave Ashen Reach/ }).first().click();
  await Promise.all([
    player.page.waitForURL(/\/account$/),
    player.page.locator('[data-leave-box] .btn-danger').click(),
  ]);
  assert.equal(await player.page.locator('.acct-rail').getByRole('link', { name: 'Ashen Reach' }).count(), 0, 'the campaign is gone from the rail');
  await picture(player.page, 'account-page-rail-light', { about: 'Account settings with its rail' });
});
