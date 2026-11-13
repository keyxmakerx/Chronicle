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

  // 1. Frame: a rail with the five areas and a card for each.
  await owner.go(`/campaigns/${cid}/customize`);
  for (const area of ['Overview', 'Look', 'Categories', 'Home pages', 'Maps']) {
    await page.locator('.czs-tab', { hasText: area }).first().waitFor();
  }
  for (const card of ['Look', 'Categories', 'Home pages', 'Maps']) {
    assert.ok(await page.locator('main').getByText(card, { exact: true }).count(), `no card for ${card}`);
  }
  await picture(page, 'customize-frame', { check: '#1366', about: 'Customize overview: rail and cards', mask: ['.sidebar', '[data-sidebar]'] });

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
  assert.ok(await page.locator('.czs-tab[data-czs-area="categories"] .czs-dot:not([hidden])').count(), 'the rail shows no dot for the changed area');
  await picture(page, 'customize-draft-bar', { check: '#1366', about: 'Save bar with one change waiting', mask: ['.sidebar', '[data-sidebar]'] });

  // 3. Save: a Saved note, the bar goes, the sidebar shows the new name.
  // The note is written down the moment it appears: the category page reads
  // itself again after a save and that wipes the note at once (#2143).
  await page.evaluate(() => {
    window.__toasts = [];
    new MutationObserver(() => {
      const t = document.querySelector('[data-czs-toasts]');
      if (t && t.innerText) window.__toasts.push(t.innerText);
    }).observe(document.body, { subtree: true, childList: true, characterData: true });
  });
  await page.locator('[data-czs-save]').click();
  await page.waitForFunction(() => window.__toasts.some((t) => /^Saved/.test(t)));
  await page.locator('[data-czs-what]').filter({ hasText: 'waiting' }).waitFor({ state: 'hidden' });
  await page.reload({ waitUntil: 'networkidle' });
  await page.locator('aside, nav').getByText('Places of Note').first().waitFor();

  // 4. Discard and Undo on Look.
  await owner.go(`/campaigns/${cid}/customize/look`);
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
  const first = page.locator('#czs-find-res a').first();
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

test('phone width: the rail is a swipeable row and the save bar fits', async () => {
  const owner = await person({ tag: 'czp', name: 'Mara Vell', context: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } });
  const cid = await newCampaign(owner, 'Frame Phone');
  const { page } = owner;
  const href = await categoryHref(page, cid, 'Locations');
  await owner.go(href.replace('/layout', '/settings'));
  await page.locator('#czc-plural').fill('Places of Note');
  await bar(page).filter({ hasText: '1 change' }).waitFor();

  const tabs = page.locator('.czs-tab').first();
  await tabs.waitFor();
  const rail = await page.evaluate(() => {
    const t = document.querySelector('.czs-tab');
    const row = t.closest('.czs-tabs-top');
    const s = getComputedStyle(row);
    return { overflowX: s.overflowX, flexDir: s.flexDirection, display: s.display, scrolls: row.scrollWidth >= row.clientWidth };
  });
  assert.notEqual(rail.flexDir, 'column', 'the rail is still a column at phone width');
  assert.ok(['auto', 'scroll'].includes(rail.overflowX), `the rail row does not scroll sideways (${rail.overflowX})`);

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

test('the Saved note stays long enough to read', { todo: 'after Save the category page reads itself again into the main area, which holds the note, so "Saved." is gone at once' }, async () => {
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
