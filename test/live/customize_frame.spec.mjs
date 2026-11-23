// customize_frame.spec.mjs: two Customize walks that share one owner, so each
// pays for its own steps and not for new accounts.
//
// The rebuilt Customize: frame, draft that survives a reload, Save, Discard with
// Undo, two tabs, Find, the old address and the phone width. Stands in for the
// operator check in #1366, steps 1-8. Step 8 checks the rail and save bar at a
// phone viewport; a real thumb swipe on a real phone is left for a person.
//
// Who may change the sheet and DM Screen look: the owner's "Who can change"
// rows, a player's and a co-DM's "Your look", and narrowing access. Stands in
// for the operator check in #1683, steps 1, 3 and 4 with the DM Screen. Left
// for a person: the Character sheet choice in steps 2 and 4, because "Your
// look" only offers a sheet when the campaign's game system draws one, and a
// scratch server has no game-system package installed (turning one on answers
// 400). The same switch drives both parts, so narrowing is proven here on the
// DM Screen. The screen lists Character sheet, Pages, Hover cards and DM
// Screen, not the two rows the issue names. A co-DM is the owner or a member
// given DM access; a plain scribe is not offered the DM Screen look, and the
// screen says so.
//
// The order matters: the phone walk drafts a rename and throws it away before
// the frame walk keeps one, the co-DM walk comes first in its campaign so the
// members list holds only that one member, and the Saved-note walk uses a
// category the frame walk leaves alone.
//
// RUN: tools/live-checks.sh test/live/customize_frame.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, picture, once, resetErrors, BASE } from './_live.mjs';

after(done);

const WIDE = { width: 1500, height: 1000 };
const LAPTOP = { width: 1280, height: 800 };

// One owner, and the people the Who-can-change walks add, made once. Each
// campaign is made by the first walk that needs it.
const accounts = once(async () => ({
  owner: await person({ tag: 'czf', name: 'Mara Vell' }),
  player: await person({ tag: 'wcq', name: 'Tomas Reed' }),
  scribe: await person({ tag: 'wcs', name: 'Ona Cole' }),
  codm: await person({ tag: 'wcu', name: 'Ines Cole' }),
}));
const frameCampaign = once(async () => newCampaign((await accounts()).owner, 'Frame Check'));
const whoCampaign = once(async () => newCampaign((await accounts()).owner, 'Who Can Change'));

beforeEach(async () => { resetErrors(...Object.values(await accounts())); });

const bar = (page) => page.locator('[data-czs-what]');

async function categoryHref(page, cid, name) {
  await page.goto(`${BASE}/campaigns/${cid}/customize/categories`, { waitUntil: 'networkidle' });
  return page.locator('main a[href*="/customize/categories/"]').filter({ hasText: name }).first().getAttribute('href');
}

const dmsKit = (who, cid) => who.go(`/campaigns/${cid}`).then(() => who.page.evaluate(() => document.documentElement.getAttribute('data-dms-kit')));

// The members page carries each member's id on their role form; DM access is
// granted by id.
async function memberId(owner, cid, name) {
  await owner.go(`/campaigns/${cid}/members`);
  const attr = await owner.page.locator(`xpath=(//*[contains(normalize-space(.), "${name}")]//*[@hx-put])[last()]`).first().getAttribute('hx-put');
  return attr.match(/members\/([0-9a-f-]{36})/)[1];
}

// A hash-only move would keep the last room open, so start from a full load.
async function openPeople(owner, cid) {
  await owner.go(`/campaigns/${cid}/customize`);
  await owner.go(`/campaigns/${cid}/customize/look#people`);
}

async function setAccess(owner, cid, label) {
  await openPeople(owner, cid);
  await owner.page.locator('.pp-row').filter({ hasText: 'DM Screen' }).click();
  await owner.page.locator('label.opt', { hasText: label }).filter({ has: owner.page.locator('input[name="ac-dmscreen"]') }).click();
  await owner.page.waitForTimeout(1500);
  await owner.page.locator('[data-czs-save]').click();
  await owner.page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
}

test('phone width: the tabs are a swipeable row and the save bar fits', async () => {
  // The same account on a phone: signing in again costs a page load, not a sign-up.
  const { owner: me } = await accounts();
  const cid = await frameCampaign();
  const owner = await person({ email: me.email, password: me.password, name: me.name, register: false, context: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } });
  const { page } = owner;
  const href = await categoryHref(page, cid, 'Locations');
  await owner.go(href.replace('/layout', '/settings'));
  await page.locator('#czc-plural').fill('Places of Note');
  await bar(page).filter({ hasText: '1 change' }).waitFor();

  const tabs = page.locator('.czs-tab').first();
  await tabs.waitFor();
  // Two rows: "Everything", the name and Find on the first, the tabs on a
  // second row that scrolls sideways; every control 44 tall, nothing clipped.
  const rail = await page.evaluate(() => {
    const t = document.querySelector('.czs-tab');
    const row = t.closest('.czs-tabrow');
    const s = getComputedStyle(row);
    const box = (q) => { const b = document.querySelector(q).getBoundingClientRect(); return { l: b.left, r: b.right, t: b.top, b: b.bottom, h: b.height, w: b.width }; };
    return {
      overflowX: s.overflowX, flexDir: s.flexDirection, w: innerWidth,
      back: box('[data-czs-back]'), name: box('.czs-catname'), find: box('.czs-top .czs-find'), tab: box('.czs-tab'),
      tabHeights: [...document.querySelectorAll('.czs-tab')].map((x) => x.getBoundingClientRect().height),
    };
  });
  assert.notEqual(rail.flexDir, 'column', 'the tabs are still a column at phone width');
  assert.ok(['auto', 'scroll'].includes(rail.overflowX), `the tabs row does not scroll sideways (${rail.overflowX})`);
  assert.ok(rail.tab.t >= rail.back.b - 1, 'the tabs share a row with "Everything"');
  assert.ok(rail.name.r <= rail.find.l + 1 && rail.find.r <= rail.w, 'the name runs under Find or Find is off the screen');
  assert.ok(rail.find.h >= 44 && rail.find.w >= 44 && rail.back.h >= 44, 'Find or "Everything" is under 44 tall');
  assert.ok(rail.tabHeights.every((h) => h >= 44), `a tab is under 44 tall (${rail.tabHeights.join(', ')})`);

  const fit = await page.evaluate(() => {
    const b = document.querySelector('[data-czs-save]').closest('[data-czs-savebar]').getBoundingClientRect();
    return { left: b.left, right: b.right, bottom: b.bottom, w: innerWidth, h: innerHeight, pageScrollsSideways: document.documentElement.scrollWidth > innerWidth };
  });
  assert.ok(fit.left >= 0 && fit.right <= fit.w + 1, `the save bar runs off the screen (${fit.left}..${fit.right} of ${fit.w})`);
  assert.ok(fit.bottom <= fit.h + 1, 'the save bar is below the screen');
  assert.ok(!fit.pageScrollsSideways, 'the page scrolls sideways at phone width');
  await picture(page, 'customize-phone', { check: '#1366', about: 'Customize at phone width with the save bar', mask: ['.sidebar', '[data-sidebar]'] });
  await page.locator('[data-czs-discard]').click();
  await page.locator('[data-czs-toasts]').getByText('Changes thrown away.').waitFor();
  owner.assertClean();
  await owner.ctx.close();
});

test('Customize frame, drafts, save, discard, two tabs, find, old address', async () => {
  const { owner } = await accounts();
  const cid = await frameCampaign();
  const { page } = owner;
  await page.setViewportSize(LAPTOP);

  // 1. The wall: a picture for the shared parts, each kind of page, each home and each app.
  await owner.go(`/campaigns/${cid}/customize`);
  for (const pic of ['Site frame', 'Colours and type', 'Hover cards', 'Locations', 'Everyone', 'Maps']) {
    assert.ok(await page.locator('.czw-tile .czw-name', { hasText: pic }).count(), `no picture for ${pic}`);
  }
  assert.equal(await page.locator('.czs-top').count(), 0, 'the wall has a bar of tabs');
  await picture(page, 'customize-frame', { check: '#1366', about: 'Customize wall: a picture of everything that can be changed', mask: ['.sidebar', '[data-sidebar]'] });
  // Pressing a picture opens its workshop; the bar names it and "Everything" comes back.
  // Focus lands on the workshop's name going in and on the picture coming back.
  await page.locator('.czw-tile', { hasText: 'Colours and type' }).click();
  await page.locator('.czs-top [data-czs-back]').waitFor();
  assert.match(page.url(), /\/customize\/look#style$/);
  await page.waitForFunction(() => document.activeElement && document.activeElement.matches('.czs-catname'));
  await page.locator('.czs-top [data-czs-back]').click();
  await page.locator('[data-czw]').waitFor();
  await page.waitForFunction(() => document.activeElement && document.activeElement.matches('.czw-tile[data-czw-key="colours"]'));

  // 2. A draft survives a reload: rename a category.
  const href = await categoryHref(page, cid, 'Locations');
  await owner.go(href.replace('/layout', '/settings'));
  const plural = page.locator('#czc-plural');
  await plural.fill('Places of Note');
  await bar(page).filter({ hasText: '1 change' }).waitFor();
  // The draft is sent a moment after the edit; wait for it before reloading.
  await page.waitForResponse((r) => r.url().includes('/customize/draft') && r.request().method() === 'PUT', { timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  await page.reload({ waitUntil: 'networkidle' });
  await bar(page).filter({ hasText: '1 change' }).waitFor();
  // Inside a category the bar's "Everything" carries the Categories dot.
  assert.ok(await page.locator('[data-czs-back] .czs-dot:not([hidden])').count(), 'the bar shows no dot for the changed area');
  await picture(page, 'customize-draft-bar', { check: '#1366', about: 'Save bar with one change waiting', mask: ['.sidebar', '[data-sidebar]'] });

  // 3. Save: a Saved note, the bar goes, the sidebar shows the new name.
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
  await page.locator('[data-czs-what]').filter({ hasText: 'waiting' }).waitFor({ state: 'hidden' });
  await page.reload({ waitUntil: 'networkidle' });
  await page.locator('aside, nav').getByText('Places of Note').first().waitFor();

  // 4. Discard and Undo on Look. The starting looks are in the panel that the
  // dock's Looks opens.
  await owner.go(`/campaigns/${cid}/customize/look`);
  await page.locator('#lk-looks-btn').click();
  await page.locator('#lk-panel.open #lk-looks:not([hidden])').waitFor();
  const parchment = page.locator('label.lk[data-hc="look:parchment"]');
  const radio = parchment.locator('input');
  await parchment.click();
  assert.ok(await radio.isChecked(), 'the Parchment starter did not take');
  await bar(page).filter({ hasText: 'waiting' }).waitFor();
  await page.locator('[data-czs-discard]').click();
  const toast = page.locator('[data-czs-toasts]');
  await toast.getByText('Changes thrown away.').waitFor();
  assert.ok(!(await radio.isChecked()), 'the preview did not go back after Discard');
  await toast.getByRole('button', { name: 'Undo' }).click();
  await page.waitForFunction(() => document.querySelector('label.lk[data-hc="look:parchment"] input').checked);
  await bar(page).filter({ hasText: 'waiting' }).waitFor();
  await page.locator('[data-czs-discard]').click(); // leave no draft behind
  await toast.getByText('Changes thrown away.').waitFor();

  // 5. Two tabs: the second tab's save must not silently overwrite the first.
  const cat = await categoryHref(page, cid, 'Items');
  const settings = cat.replace('/layout', '/settings');
  await owner.go(settings);
  const tab2 = await owner.ctx.newPage();
  await tab2.goto(BASE + settings, { waitUntil: 'networkidle' });
  await page.locator('#czc-plural').fill('Things A');
  await bar(page).filter({ hasText: '1 change' }).waitFor();
  await page.waitForTimeout(1500);
  await tab2.locator('#czc-plural').fill('Things B');
  const bar2 = tab2.locator('[data-czs-what]');
  await bar2.filter({ hasText: 'Draft changed elsewhere' }).waitFor({ timeout: 10000 });
  assert.equal((await tab2.locator('[data-czs-save]').textContent()).trim(), 'Reload', 'the second tab does not offer Reload');
  await tab2.close();
  await page.locator('[data-czs-discard]').click();
  await toast.getByText('Changes thrown away.').waitFor();

  // 6. Find: "/" then "colour"; Colours is first, Enter opens it.
  await owner.go(`/campaigns/${cid}/customize`);
  await page.locator('main').click({ position: { x: 5, y: 5 } });
  await page.keyboard.press('/');
  const find = page.getByRole('combobox', { name: 'Find a setting' });
  await find.waitFor();
  await find.fill('colour');
  const first = page.locator('#fnd-res a').first();
  await first.waitFor();
  assert.match(await first.innerText(), /Colours/, 'Colours is not the first result');
  await picture(page, 'customize-find', { check: '#1366', about: 'Find a setting, typed "colour"', mask: ['.sidebar', '[data-sidebar]'] });
  await page.keyboard.press('Enter');
  await page.waitForURL(/\/customize\/look#colours/);

  // 7. The old address lands on Customize > Categories.
  await page.goto(`${BASE}/campaigns/${cid}/entity-types`, { waitUntil: 'networkidle' });
  assert.match(page.url(), /\/customize\/categories/);
  owner.assertClean();
});

test('the Saved note stays long enough to read', async () => {
  const { owner } = await accounts();
  const cid = await frameCampaign();
  const { page } = owner;
  await page.setViewportSize(LAPTOP);
  const href = await categoryHref(page, cid, 'Items');
  await owner.go(href.replace('/layout', '/settings'));
  await page.locator('#czc-plural').fill('Far Places');
  await bar(page).filter({ hasText: '1 change' }).waitFor();
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor({ timeout: 3000 });
  await page.waitForTimeout(1000);
  assert.ok(await page.locator('[data-czs-toasts]').getByText(/^Saved/).isVisible(), 'the note is still there a second later');
});

// First in this campaign: the members page then lists only the co-DM, so the id
// read from it is theirs, and the access is back to Me and co-DMs at the end.
test('a co-DM picks a DM Screen look; narrowing access takes it away and widening brings it back', async () => {
  const { owner, codm } = await accounts();
  const cid = await whoCampaign();
  await owner.page.setViewportSize(WIDE);
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
  // The pick reloads the page, so the button can be missing for a moment.
  await codm.page.waitForFunction(() => document.querySelector('[data-look-part="dmscreen"] button[data-value="ornate"]')?.getAttribute('aria-pressed') === 'true');
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

test('owner sees who can change; DM Screen never offers Everyone', async () => {
  const { owner } = await accounts();
  const cid = await whoCampaign();
  await owner.page.setViewportSize(WIDE);
  await openPeople(owner, cid);
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

test('opening a row on the People screen raises no page error', async () => {
  const { owner } = await accounts();
  const cid = await whoCampaign();
  await owner.page.setViewportSize(WIDE);
  await openPeople(owner, cid);
  await owner.page.locator('.pp-row').first().click();
  await owner.page.waitForTimeout(500);
  owner.assertClean();
});

test('a player is offered no DM Screen look and no link they cannot use', async () => {
  const { owner, player } = await accounts();
  const cid = await whoCampaign();
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

test('a plain scribe is not offered the DM Screen look, and the People screen says why', async () => {
  const { owner, scribe } = await accounts();
  const cid = await whoCampaign();
  await owner.page.setViewportSize(WIDE);
  await addMember(owner, cid, scribe, 'scribe');
  await scribe.go(`/campaigns/${cid}/my-look`);
  assert.equal(await scribe.page.locator('[data-look-part="dmscreen"]').count(), 0, 'a plain scribe is offered the DM Screen look');

  await openPeople(owner, cid);
  await owner.page.locator('#pp-rows .pp-row').first().waitFor();
  assert.match(await owner.page.locator('#pp-rows').locator('xpath=..').innerText(), /A co-DM is a member you gave DM access\. Scribes pick what players pick\./);
  assert.doesNotMatch(await owner.page.locator('main').innerText(), /co-DM is a scribe/i, 'the screen still says a scribe is a co-DM');
  // Viewing as a co-DM lists the DM Screen; viewing as a player does not.
  await owner.page.locator('[data-viewas="codm"]').click();
  assert.ok(await owner.page.locator('#pp-rows .pp-row', { hasText: 'DM Screen' }).count(), 'viewing as a co-DM does not list the DM Screen');
  await owner.page.locator('[data-viewas="player"]').click();
  assert.equal(await owner.page.locator('#pp-rows .pp-row', { hasText: 'DM Screen' }).count(), 0, 'viewing as a player lists the DM Screen');
});

