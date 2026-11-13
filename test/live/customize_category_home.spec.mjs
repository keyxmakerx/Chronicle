// customize_category_home.spec.mjs — Category home in the Customizer saves with
// the one save bar. Stands in for the operator check in #1551/#1552.
//
// Covers steps 1-4. The category landing page is read back as the owner.
//
// RUN: tools/live-checks.sh test/live/customize_category_home.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture, BASE } from './_live.mjs';

after(done);

test('Category home: edit, one save bar, landing page, home-widget list', async () => {
  const owner = await person({ tag: 'cch', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const cid = await newCampaign(owner, 'Category Home Check');
  const { page } = owner;
  await newPage(owner, cid, 'Location', 'Highmoor');

  // 1. The Category home tab is a page editor, with no "saves alone" note.
  await owner.go(`/campaigns/${cid}/customize/categories`);
  const href = await page.locator('main a[href*="/customize/categories/"]').filter({ hasText: 'Locations' }).last().getAttribute('href');
  const home = href.replace('/layout', '/home');
  await owner.go(home);
  await page.locator('.cpe-sec').first().waitFor();
  const text = await page.locator('.cpe-page').innerText();
  assert.match(text, /Locations/, 'no category heading');
  assert.match(text, /pinned pages/i, 'no Pinned pages block');
  assert.ok(await page.locator('.cpe-page input, .cpe-page [data-type="search_bar"]').count(), 'no search box');
  assert.equal(await page.getByText(/saves with its own button|saves alone/i).count(), 0, 'the "still saves with its own button" note is there');
  await picture(page, 'customize-category-home', { check: '#1552', about: 'Category home as a page editor', mask: ['.cpe-page'] });

  // 2. The heading's Description and a Text Block, through one change line.
  await page.locator('.cpe-blk:has([data-type="category_header"])').first().click();
  await page.locator('#cpe-desc').fill('Every place the party has set foot in.');
  // The description is taken on blur; the panel redraws, so open the gallery after.
  await page.locator('#cpe-desc').blur();
  await page.locator('[data-czs-what]').filter({ hasText: 'waiting' }).waitFor();
  await page.getByRole('tab', { name: 'Add widgets' }).click();
  await page.locator('.cpe-gw[data-gw="text_block"]').click();
  await page.locator('.cpe-insp textarea').first().fill('Mind the tide-toll.');
  await page.locator('[data-czs-what]').filter({ hasText: 'waiting' }).waitFor();
  assert.match(await page.locator('[data-czs-what]').innerText(), /1 change waiting: Locations home/);
  await page.waitForFunction(() => !document.querySelector('.cpe-page .cpe-wait'));
  assert.match(await page.locator('.cpe-page').innerText(), /Every place the party has set foot in/, 'the description is not in the preview');

  // 3. Save, then the landing page shows both.
  await page.waitForTimeout(1500);
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
  await owner.go(`/campaigns/${cid}/location`);
  const landing = await page.locator('main').innerText();
  assert.match(landing, /Every place the party has set foot in/, 'the landing page lacks the description');
  assert.match(landing, /Mind the tide-toll/, 'the landing page lacks the text block');

  // 4. The Home pages widget list holds home widgets only.
  await owner.go(`/campaigns/${cid}/customize/home`);
  await page.locator('.cpe-gw').first().waitFor();
  const widgets = await page.locator('.cpe-gw').allInnerTexts();
  const joined = widgets.join(' | ');
  assert.match(joined, /Welcome Banner/);
  assert.match(joined, /Quick Actions/);
  assert.doesNotMatch(joined, /Category Header|Search Bar/, 'category-only widgets leaked into Home pages');
  owner.assertClean();
});
