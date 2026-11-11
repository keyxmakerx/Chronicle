// customize_who_can_change.spec.mjs — who may change the sheet and DM Screen
// look: the owner's "Who can change" rows, a player's and a co-DM's "Your look",
// and narrowing access. Stands in for the operator check in #1683.
//
// Covers steps 1, 3 and 4 with the DM Screen. Left for a person: the Character
// sheet choice in steps 2 and 4, because "Your look" only offers a sheet when the
// campaign's game system draws one, and a scratch server has no game-system
// package installed (turning one on answers 400). The same switch drives both
// parts, so narrowing is proven here on the DM Screen.
//
// Findings against the issue's wording (see the todo below): the screen now
// lists Character sheet, Pages, Hover cards and DM Screen, not two rows; and
// "co-DM" means the owner or a member given DM access, so a plain scribe is
// not offered the DM Screen look.
//
// RUN: tools/live-checks.sh test/live/customize_who_can_change.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, picture, BASE } from './_live.mjs';

after(done);

const dmsKit = (who, cid) => who.go(`/campaigns/${cid}`).then(() => who.page.evaluate(() => document.documentElement.getAttribute('data-dms-kit')));

// The members page carries each member's id on their role form; DM access is
// granted by id.
async function memberId(owner, cid, name) {
  await owner.go(`/campaigns/${cid}/members`);
  const attr = await owner.page.locator(`xpath=(//*[contains(normalize-space(.), "${name}")]//*[@hx-put])[last()]`).first().getAttribute('hx-put');
  return attr.match(/members\/([0-9a-f-]{36})/)[1];
}

async function setAccess(owner, cid, label) {
  // A hash-only move would keep the last room open; start from a full load.
  await owner.go(`/campaigns/${cid}/customize`);
  await owner.go(`/campaigns/${cid}/customize/look#people`);
  await owner.page.locator('.pp-row').filter({ hasText: 'DM Screen' }).click();
  await owner.page.locator('label.opt', { hasText: label }).filter({ has: owner.page.locator('input[name="ac-dmscreen"]') }).click();
  await owner.page.waitForTimeout(1500);
  await owner.page.locator('[data-czs-save]').click();
  await owner.page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
}

test('owner sees who can change; DM Screen never offers Everyone', async () => {
  const owner = await person({ tag: 'wco', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const cid = await newCampaign(owner, 'Who Can Change');
  await owner.go(`/campaigns/${cid}/customize/look#people`);
  const rows = owner.page.locator('#pp-rows .pp-row');
  await rows.first().waitFor();
  const text = await rows.allInnerTexts();
  const row = (name) => text.find((t) => t.startsWith(name)) || '';
  assert.match(row('Character sheet'), /Everyone/);
  assert.match(row('DM Screen'), /Me and co-DMs/);
  await picture(owner.page, 'customize-who-can-change', { check: '#1683', about: 'Look > People: who can change each part' });

  await owner.page.locator('.pp-row').filter({ hasText: 'DM Screen' }).click();
  const labels = await owner.page.locator('input[name="ac-dmscreen"] + span').allInnerTexts();
  assert.deepEqual(labels, ['Only me', 'Me and co-DMs'], 'the DM Screen offers a wider setting than co-DMs');
});

test('opening a row on the People screen raises no page error', { todo: 'clicking a People row throws "Cannot read properties of undefined" in fzRects (customize_look.js): the People room has no focus-zone entry for the part it opens' }, async () => {
  const owner = await person({ tag: 'wcv', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const cid = await newCampaign(owner, 'Who Can Change Errors');
  await owner.go(`/campaigns/${cid}/customize/look#people`);
  await owner.page.locator('.pp-row').first().click();
  await owner.page.waitForTimeout(500);
  owner.assertClean();
});

test('a player is offered no DM Screen look and no link they cannot use', async () => {
  const owner = await person({ tag: 'wcp', name: 'Mara Vell' });
  const player = await person({ tag: 'wcq', name: 'Tomas Reed' });
  const cid = await newCampaign(owner, 'Who Can Change Player');
  await addMember(owner, cid, player, 'player');
  await player.go(`/campaigns/${cid}/my-look`);
  const body = await player.page.locator('main').innerText();
  assert.doesNotMatch(body, /DM Screen/, 'a player is offered the DM Screen look');
  assert.equal(await player.page.locator('[data-look-part="dmscreen"]').count(), 0);
  await player.go(`/campaigns/${cid}`);
  assert.equal(await player.page.getByRole('link', { name: 'Your look' }).count(), 0, 'a player with nothing to pick still sees Your look');
  // Writing a DM Screen pick anyway is refused.
  const res = await player.api(`/campaigns/${cid}/my-look/dmscreen`, { method: 'PUT', body: { value: 'ornate' } });
  assert.ok(res.status >= 400 && res.status < 500, `a player's DM Screen pick answered ${res.status}`);
});

test('a plain scribe is offered the DM Screen look', { todo: 'the screen says a co-DM is a scribe, but only the owner and members given DM access are offered it (isCoDM in look_access.go)' }, async () => {
  const owner = await person({ tag: 'wcr', name: 'Mara Vell' });
  const scribe = await person({ tag: 'wcs', name: 'Ines Cole' });
  const cid = await newCampaign(owner, 'Who Can Change Scribe');
  await addMember(owner, cid, scribe, 'scribe');
  await scribe.go(`/campaigns/${cid}/my-look`);
  await scribe.page.locator('[data-look-part="dmscreen"]').waitFor({ timeout: 3000 });
});

test('a co-DM picks a DM Screen look; narrowing access takes it away and widening brings it back', async () => {
  const owner = await person({ tag: 'wct', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const codm = await person({ tag: 'wcu', name: 'Ines Cole' });
  const cid = await newCampaign(owner, 'Who Can Change CoDM');
  await addMember(owner, cid, codm, 'scribe');
  const id = await memberId(owner, cid, 'Ines Cole');
  const grant = await owner.api(`/campaigns/${cid}/dm-grants`, { method: 'PUT', body: { user_ids: [id] } });
  assert.ok(grant.ok, 'could not give DM access');

  // 3. Both the link and the choice are there; a pick changes the DM Screen only.
  await codm.go(`/campaigns/${cid}`);
  await codm.page.getByRole('link', { name: 'Your look' }).click();
  await codm.page.locator('[data-look-part="dmscreen"]').waitFor();
  assert.equal(await codm.page.locator('[data-look-part="sheet"]').count(), 0, 'no game system, so no sheet choice');
  const before = await dmsKit(codm, cid);
  const pageKit = await codm.page.evaluate(() => document.documentElement.getAttribute('data-cz-kit'));
  await codm.go(`/campaigns/${cid}/my-look`);
  const pick = codm.page.locator('[data-look-part="dmscreen"] button[data-value="ornate"]');
  await pick.click();
  await codm.page.waitForFunction(() => document.querySelector('[data-look-part="dmscreen"] button[data-value="ornate"]').getAttribute('aria-pressed') === 'true');
  assert.equal(await dmsKit(codm, cid), 'ornate', 'the DM Screen does not wear the picked look');
  assert.notEqual(before, 'ornate');
  assert.equal(await codm.page.evaluate(() => document.documentElement.getAttribute('data-cz-kit')), pageKit, 'the pages around the DM Screen changed');
  assert.notEqual(await dmsKit(owner, cid), 'ornate', 'the owner is wearing the co-DM\'s pick');
  await picture(codm.page, 'customize-your-look-codm', { check: '#1683', about: 'Your look for a co-DM: the DM Screen choice', mask: ['.nav-foot'] });
  await codm.go(`/campaigns/${cid}/my-look`);

  // 4. Only me: the choice and the link go, and the pick stops applying.
  await setAccess(owner, cid, 'Only me');
  await codm.go(`/campaigns/${cid}/my-look`);
  assert.equal(await codm.page.locator('[data-look-part="dmscreen"]').count(), 0, 'the co-DM can still pick after Only me');
  assert.equal(await codm.page.getByRole('link', { name: 'Your look' }).count(), 0, 'Your look is still linked');
  assert.notEqual(await dmsKit(codm, cid), 'ornate', 'the pick still applies after Only me');

  // Back to Me and co-DMs: the earlier pick returns.
  await setAccess(owner, cid, 'Me and co-DMs');
  assert.equal(await dmsKit(codm, cid), 'ornate', 'the earlier pick did not come back');
  // Page errors are not asserted here: the People-row error is its own todo above.
});
