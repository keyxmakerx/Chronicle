// calendar_wizard_generate_back.spec.mjs — Back from Review keeps a
// generated calendar. In New calendar > Generate one, the recipe, the
// calendar and the name typed into it are what Back returns to, not a fresh
// random calendar.
//
// RUN: tools/live-checks.sh test/live/calendar_wizard_generate_back.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, enableAddons, done, go } from './_live.mjs';

after(done);

// Calendars > New calendar > Generate one.
async function openGenerate(page, cid) {
  await go(page, `/campaigns/${cid}/calendars`);
  await page.getByRole('button', { name: 'New calendar' }).first().click();
  await page.getByRole('button', { name: /Generate one/ }).click();
  await page.waitForSelector('input[x-model="cal.name"]');
}

const sheet = (page) => page.evaluate(() => {
  const root = document.querySelector('[data-engine-src]');
  const pressed = root && root.querySelector('.calv5-chips [aria-pressed="true"]');
  const name = root && root.querySelector('input[x-model="cal.name"]');
  return { recipe: pressed ? pressed.textContent.trim() : '', name: name ? name.value : '', months: [...(root ? root.querySelectorAll('input[x-model="mo.name"]') : [])].map((i) => i.value).join('|') };
});

test('Back from Review returns to the same generated calendar and name', async () => {
  const owner = await person({ tag: 'gen', name: 'Mara Vell' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Generated Back');
  await enableAddons(owner, cid, '*');
  await openGenerate(page, cid);
  await page.locator('.calv5-chips button', { hasText: 'Moon-reckoned' }).click();
  const name = page.locator('input[x-model="cal.name"]');
  await name.fill('Ashfall Reckoning');
  const before = await sheet(page);
  assert.equal(before.recipe, 'Moon-reckoned');

  await page.locator('#calv5-generate-submit').click();
  await page.waitForSelector('#calv5-review-submit');
  await page.getByRole('button', { name: 'Back', exact: true }).click();
  await page.waitForSelector('input[x-model="cal.name"]');
  const back = await sheet(page);
  assert.deepEqual(back, before, 'Back showed a different calendar');

  // A fresh visit from Start begins again.
  await openGenerate(page, cid);
  assert.notEqual((await sheet(page)).name, 'Ashfall Reckoning', 'a fresh visit kept the old sheet');
});
