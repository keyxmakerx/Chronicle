// customize_frame.spec.mjs — the rebuilt Customize: frame, draft that survives
// a reload, Save, Discard with Undo, two tabs, Find, the old address and the
// phone width. Stands in for the operator check in #1366.
//
// Covers steps 1-8. Step 8 checks the rail and save bar at a phone viewport; a
// real thumb swipe on a real phone is left for a person.
//
// RUN: tools/live-checks.sh test/live/customize_frame.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done, picture, BASE } from './_live.mjs';

after(done);

const bar = (page) => page.locator('[data-czs-what]');

async function categoryHref(page, cid, name) {
  await page.goto(`${BASE}/campaigns/${cid}/customize/categories`, { waitUntil: 'networkidle' });
  return page.locator('main a[href*="/customize/categories/"]').filter({ hasText: name }).first().getAttribute('href');
}

test('Customize frame, drafts, save, discard, two tabs, find, old address', async () => {
  const owner = await person({ tag: 'czf', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Frame Check');
  const { page } = owner;

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

test('phone width: the tabs are a swipeable row and the save bar fits', async () => {
  const owner = await person({ tag: 'czp', name: 'Mara Vell', context: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } });
  const cid = await newCampaign(owner, 'Frame Phone');
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
  owner.assertClean();
});

test('the Saved note stays long enough to read', async () => {
  const owner = await person({ tag: 'czs', name: 'Ines Morrow' });
  const cid = await newCampaign(owner, 'Saved Note Check');
  const { page } = owner;
  const href = await categoryHref(page, cid, 'Locations');
  await owner.go(href.replace('/layout', '/settings'));
  await page.locator('#czc-plural').fill('Far Places');
  await bar(page).filter({ hasText: '1 change' }).waitFor();
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor({ timeout: 3000 });
  await page.waitForTimeout(1000);
  assert.ok(await page.locator('[data-czs-toasts]').getByText(/^Saved/).isVisible(), 'the note is still there a second later');
});
