// all_pages_and_create_sheet.spec.mjs — All pages is one plain list the owner
// can pick from (and undo), the player sees none of the owner's marks, and the
// Create page sheet's sentence is true to what a player then sees.
//
// RUN: tools/live-checks.sh test/live/all_pages_and_create_sheet.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, newPage, api, go, done } from './_live.mjs';

after(done);

const rowNames = (page) => page.locator('.el-row .el-name').allInnerTexts();

// Makes a page through the sheet opened from All pages. wantShown asks for the
// sentence's toggle to be pressed; the sentence it ends on is returned so the
// caller can compare it with what a player sees.
async function viaSheet(owner, cid, name, { show = false } = {}) {
  const { page } = owner;
  await go(page, `/campaigns/${cid}/entities`);
  await page.locator('[data-app-opening] a[href*="entities/new"]').click();
  await page.waitForSelector('dialog.np-sheet[open] #name');
  assert.match(new URL(page.url()).pathname, new RegExp(`/campaigns/${cid}/entities$`), 'the sheet opens where you are');
  await page.fill('#name', name);
  const select = page.locator('dialog.np-sheet select[name=entity_type_id]');
  const first = await select.evaluate((el) => [...el.options].find((o) => o.value)?.value || '');
  await select.selectOption(first);
  if (show) await page.locator('dialog.np-sheet [data-np-who-word]').click();
  const sentence = (await page.locator('dialog.np-sheet [data-np-who-text]').innerText()).trim();
  await Promise.all([page.waitForURL(/\/entities\/[0-9a-f-]{36}/), page.locator('dialog.np-sheet button[type=submit]').click()]);
  await page.waitForLoadState('networkidle');
  return sentence;
}

test('owner picks two pages, tags them, and Undo takes the tag back', async () => {
  const owner = await person({ tag: 'apo', name: 'Owner Olly' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'All Pages Check');
  for (const n of ['Aldermoor', 'Brinehold', 'Cinderquay']) await newPage(owner, cid, 'Location', n);
  const tag = (await api(page, `/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Coast', color: '#6b7280' } })).body;
  assert.ok(tag && tag.id, 'a tag to apply');

  await go(page, `/campaigns/${cid}/entities`);
  assert.deepEqual(await rowNames(page), ['Aldermoor', 'Brinehold', 'Cinderquay'], 'one plain A to Z list');

  // The box is in the margin, shown by pointing at the row.
  for (const n of [1, 2]) {
    await page.locator(`.el-row:nth-child(${n})`).hover();
    await page.locator(`.el-row:nth-child(${n}) .el-pick input`).check();
  }
  assert.match(await page.locator('.bulk-action-bar').innerText(), /2 selected/);
  await page.locator('[data-bulk-action="add-tags"]').click();
  await page.waitForSelector('.bulk-picker [role="option"]');
  await page.keyboard.press('Enter');
  await page.waitForLoadState('networkidle');
  await page.waitForFunction(() => document.querySelectorAll('.el-tag').length === 2);

  await page.getByRole('button', { name: 'Undo' }).first().click();
  await page.waitForLoadState('networkidle');
  await page.waitForFunction(() => document.querySelectorAll('.el-tag').length === 0);
  owner.assertClean();
});

test('a player sees no hidden page and none of the owner marks', async () => {
  const owner = await person({ tag: 'apo2', name: 'Owner Ola' });
  const player = await person({ tag: 'app2', name: 'Pip Player' });
  const cid = await newCampaign(owner, 'All Pages Player');
  await newPage(owner, cid, 'Location', 'Open Harbour');
  const shown = await viaSheet(owner, cid, 'Shown On Purpose', { show: true });
  assert.match(shown, /Everyone/, 'the sentence says everyone after Show it to players');
  const left = await viaSheet(owner, cid, 'Left As Offered');
  await addMember(owner, cid, player, 'player');

  // The sentence is the claim; the player's list is the proof.
  await go(player.page, `/campaigns/${cid}/entities`);
  const seen = await rowNames(player.page);
  assert.ok(seen.includes('Shown On Purpose'), 'a page the sentence said everyone sees, the player sees');
  assert.equal(seen.includes('Left As Offered'), !/won.t see/.test(left), 'the left-as-offered sentence matches what the player sees');
  assert.equal(await player.page.locator('.el-pick, .el-select-word, [data-dm-only]').count(), 0, 'no tick boxes, Select word or hidden mark for a player');
  assert.equal(await player.page.locator('[data-app-opening] a[href*="entities/new"]').count(), 0, 'no New page for a player');
  player.assertClean();
  owner.assertClean();
});
