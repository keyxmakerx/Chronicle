// Account settings and Site admin name the campaign you were last in as the
// way back, and only to a member of it.
import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, siteAdmin, newCampaign, done, BASE } from './_live.mjs';

after(done);

test('Account settings and Site admin offer the way back to your campaign', async () => {
  const admin = await siteAdmin();
  const cid = await newCampaign(admin, 'Saltreach Keep');
  const page = admin.page;
  await admin.go(`/campaigns/${cid}`);

  await admin.go('/account');
  const row = page.locator('#sidebar .nav-back');
  assert.match(await row.innerText(), /Back to Saltreach Keep/);
  assert.equal(await row.getAttribute('href'), `/campaigns/${cid}`);
  const box = await row.boundingBox();
  assert.ok(box && box.height >= 44, `the row is ${box && box.height}px tall`);

  await admin.go('/admin');
  assert.match(await page.locator('#admin-nav .nav-back').innerText(), /Back to Saltreach Keep/);
  assert.match(await page.locator('.admin-leave').innerText(), /Back to Saltreach Keep/);
  await page.locator('#admin-nav .nav-back').click();
  await page.waitForURL(new RegExp(`/campaigns/${cid}`));

  // Someone who is not in the campaign never sees its name, whatever the
  // cookie says.
  const stranger = await person({ tag: 'rc-stranger', name: 'Stranger' });
  await stranger.page.context().addCookies([{ name: 'chronicle_last_campaign', value: cid, url: BASE }]);
  await stranger.go('/account');
  assert.equal(await stranger.page.locator('.nav-back').count(), 0);
  assert.equal(await stranger.page.getByText('Saltreach Keep').count(), 0);
  admin.assertClean();
});
