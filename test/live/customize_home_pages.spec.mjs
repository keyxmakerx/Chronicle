// customize_home_pages.spec.mjs — Home pages open in the page editor and a
// change saves through the one save bar. Stands in for steps 2 and 3 of the
// operator check in #1545.
//
// Step 1 (campaigns made before the deploy moved to Material) needs a person:
// it is about what the first boot of the new build did to campaigns that
// already existed, and a scratch server only has new ones.
//
// RUN: tools/live-checks.sh test/live/customize_home_pages.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, picture } from './_live.mjs';

after(done);

test('Home pages: tabs, the real home, and a Players change saved and seen', async () => {
  const owner = await person({ tag: 'cho', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const player = await person({ tag: 'chp', name: 'Tomas Reed' });
  const cid = await newCampaign(owner, 'Home Pages Check');
  await addMember(owner, cid, player, 'player');
  const { page } = owner;

  // 2. Six homes (listed in the Home pages drawer of the rail), with the real home drawn under them.
  await owner.go(`/campaigns/${cid}/customize/home`);
  await page.locator('.cpe-sec').first().waitFor();
  const tabs = (await page.locator('.czs-drawer a[href*="/customize/home/"]').allInnerTexts()).map((t) => t.trim());
  assert.deepEqual(tabs, ['Everyone', 'Players', 'Scribes', 'GM team', 'Guests', 'Overview'], 'the home tabs are not the expected six');
  assert.match(await page.locator('.cpe-page').innerText(), /categories/i, 'the real home (its categories) is not drawn');
  assert.equal(await page.locator('.czs-legacy, button:has-text("Save Layout")').count(), 0, 'the old grid with its own Save Layout is back');
  await picture(page, 'customize-home-pages', { check: '#1545', about: 'Home pages, Everyone tab drawn as the real home', mask: ['.cpe-page'] });

  // A tab never arranged says what it shows. New campaigns start with their
  // own Players home, so the Scribes tab stands in for an unarranged one.
  await owner.go(`/campaigns/${cid}/customize/home/scribe`);
  await page.locator('.cpe-sec').first().waitFor();
  const borrowing = (await page.locator('.czs-home-hint').innerText()).trim();
  assert.match(borrowing, /same as the Everyone home until you change it/i, 'an unarranged tab does not say what it shows');

  await owner.go(`/campaigns/${cid}/customize/home/player`);
  await page.locator('.cpe-sec').first().waitFor();

  // 3. Add a widget on the Players tab; it goes through the save bar.
  await page.getByRole('tab', { name: 'Add widgets' }).click();
  await page.locator('.cpe-gw[data-gw="text_block"]').click();
  await page.locator('.cpe-insp textarea').first().fill('Welcome, adventurers of the Grey Coast.');
  await page.locator('[data-czs-what]').filter({ hasText: '1 change waiting' }).waitFor();
  assert.match(await page.locator('[data-czs-what]').innerText(), /Home pages: Players/);
  await page.waitForTimeout(1500);
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();

  await player.go(`/campaigns/${cid}`);
  await player.page.getByText('Welcome, adventurers of the Grey Coast.').first().waitFor();

  await owner.go(`/campaigns/${cid}/customize/home/player`);
  await page.locator('.cpe-sec').first().waitFor();
  const after = (await page.locator('.czs-home-hint').innerText()).trim();
  assert.doesNotMatch(after, /until you change it/i, 'the Players tab still says it borrows the Everyone home');

  // Undo for step 3: remove the widget on the same tab and save again.
  await page.locator('.cpe-blk:has([data-type="text_block"]), .cpe-tile:has([data-type="text_block"])').first().click();
  await page.getByRole('button', { name: 'Remove from the layout' }).click();
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
  owner.assertClean();
  player.assertClean();
});
