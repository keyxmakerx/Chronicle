// topbar_tools_shelf.spec.mjs — on a phone the top bar keeps the menu, the
// campaign name, search, one Tools tab and the picture. The Tools tab hangs a
// shelf with every tool and its name; an entry presses the real button. Above
// a phone the bar is as it was and there is no tab.
//
// RUN: tools/live-checks.sh test/live/topbar_tools_shelf.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 360, height: 740 }, isMobile: true, hasTouch: true };
const NAME = 'Saltreach of the Shattered Isles';

// The bar's own tool buttons, by the label each one carries on the bar.
const BAR_TOOLS = {
  ai: '#app-topbar [data-ai-chip]',
  brush: '#app-topbar [data-brush-toggle]',
  table: '#app-topbar [data-tt-tab]',
  dms: '#app-topbar [data-dms-toggle]',
  eye: '#app-topbar button[data-pv-toggle]',
  bell: '#app-topbar [data-notif-toggle]',
};

const shown = (page, sel) => page.locator(sel).evaluate((el) => {
  const box = el.getBoundingClientRect();
  return getComputedStyle(el).visibility === 'visible' && box.width > 0 && box.height > 0;
});

const shelfNames = (page) => page.locator('#tb-shelf .tb-tool:not([hidden]) .tb-tool-name').allTextContents();

// The widget loads when the tab comes into sight, so after a page load the
// tab is there a moment before it answers.
async function openShelf(page) {
  await page.waitForFunction(() => {
    const host = document.querySelector('[data-widget="topbar-tools"]');
    return !!host && !!host._tbDestroy;
  });
  await page.locator('#tb-tools-tab').tap();
  await page.locator('#tb-shelf').waitFor({ state: 'visible' });
  await page.waitForFunction(() => document.getElementById('tb-shelf').classList.contains('is-open'));
  await page.waitForTimeout(300);
}

test('phone: the name shows its first word and nothing in the bar is cut off', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  const first = await page.locator('[data-topbar-name]').evaluate((span) => {
    const cs = getComputedStyle(span);
    const c = document.createElement('canvas').getContext('2d');
    c.font = `${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`;
    return { word: span.textContent.trim().split(/\s+/)[0], need: c.measureText(span.textContent.trim().split(/\s+/)[0]).width, room: span.clientWidth };
  });
  assert.ok(first.room >= first.need, `"${first.word}" needs ${Math.round(first.need)}px and the name has ${first.room}px`);

  // Everything on the bar sits inside it and none of it overlaps.
  const boxes = await page.evaluate(() => {
    const bar = document.getElementById('app-topbar');
    const b = bar.getBoundingClientRect();
    const vis = [...bar.querySelectorAll('#nav-toggle, button:has([data-topbar-name]), #topbar-search-trigger, #tb-tools-tab, #user-menu-btn')]
      .filter((el) => el.getClientRects().length)
      .map((el) => { const r = el.getBoundingClientRect(); return { id: el.id || 'name', l: r.left, r: r.right, t: r.top, bt: r.bottom }; });
    return { bar: { l: b.left, r: b.right, w: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth }, vis };
  });
  assert.ok(boxes.bar.w <= boxes.bar.cw, 'the page scrolls sideways');
  for (const v of boxes.vis) assert.ok(v.l >= boxes.bar.l - 0.5 && v.r <= boxes.bar.r + 0.5, `${v.id} is cut off by the bar's edge`);
  const row = [...boxes.vis].sort((a, b) => a.l - b.l);
  for (let i = 1; i < row.length; i++) assert.ok(row[i].l >= row[i - 1].r - 0.5, `${row[i - 1].id} and ${row[i].id} overlap`);
  assert.deepEqual(row.map((v) => v.id), ['nav-toggle', 'name', 'topbar-search-trigger', 'tb-tools-tab', 'user-menu-btn']);

  // The tools themselves are off the bar until kept.
  for (const [name, sel] of Object.entries(BAR_TOOLS)) assert.equal(await shown(page, sel), false, `${name} is still on the bar`);
  owner.assertClean();
});

test('phone: the Tools tab opens the shelf, the DM Screen entry opens the DM Screen, Escape closes', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  const tab = page.locator('#tb-tools-tab');
  assert.equal(await tab.getAttribute('aria-expanded'), 'false');
  assert.equal(await tab.getAttribute('aria-controls'), 'tb-shelf');
  await openShelf(page);
  assert.equal(await tab.getAttribute('aria-expanded'), 'true');
  assert.deepEqual(await shelfNames(page), ['Your AI', 'Customize', 'The table', 'GM Screen', 'View as player', 'Notifications', 'Find']);

  // Every entry is a real button a finger can hit, and the shelf lies over the page.
  const sizes = await page.locator('#tb-shelf .tb-tool:not([hidden])').evaluateAll((els) => els.map((e) => { const r = e.getBoundingClientRect(); return [Math.round(r.width), Math.round(r.height)]; }));
  for (const [w, h] of sizes) assert.ok(w >= 44 && h >= 44, `an entry is ${w} by ${h}`);
  const lifted = await page.evaluate(() => getComputedStyle(document.getElementById('app-topbar')).zIndex);
  assert.equal(lifted, '70', 'the bar lifts over the page while the shelf is open');

  // Escape closes and gives focus back to the tab.
  await page.keyboard.press('Escape');
  await page.locator('#tb-shelf').waitFor({ state: 'hidden' });
  assert.equal(await tab.getAttribute('aria-expanded'), 'false');
  assert.equal(await page.evaluate(() => document.activeElement && document.activeElement.id), 'tb-tools-tab');
  await page.waitForFunction(() => getComputedStyle(document.getElementById('app-topbar')).zIndex === '20');

  // A tap outside closes it too.
  await openShelf(page);
  await page.locator('#main-content').tap({ position: { x: 180, y: 500 } });
  await page.locator('#tb-shelf').waitFor({ state: 'hidden' });

  // The DM Screen entry does what the real button does.
  await openShelf(page);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'GM Screen' }).tap();
  await page.locator('#tb-shelf').waitFor({ state: 'hidden' });
  await page.waitForFunction(() => document.querySelector('[data-dms-toggle]').getAttribute('aria-expanded') === 'true');
  await page.locator('.dms-dock').waitFor({ state: 'visible' });
  owner.assertClean();
});

test('phone: the table entry opens the table, and the AI entry its bubble under the bar', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.waitForSelector('html[data-tt-sheet] .tt-sheet .tt-grab');

  await openShelf(page);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'The table' }).tap();
  await page.waitForFunction(() => document.querySelector('.tt-sheet').dataset.h !== 'rest');

  await owner.go(`/campaigns/${cid}`);
  await openShelf(page);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Your AI' }).tap();
  const bubble = page.locator('[data-ai-bubble]');
  await bubble.waitFor({ state: 'visible' });
  await page.waitForTimeout(400);
  const at = await bubble.evaluate((el) => {
    const r = el.getBoundingClientRect();
    const bar = document.getElementById('app-topbar').getBoundingClientRect();
    return { top: r.top, left: r.left, right: r.right, bar: bar.bottom, vw: document.documentElement.clientWidth };
  });
  assert.ok(at.top >= at.bar - 1, 'the bubble hangs under the bar');
  assert.ok(at.left >= 0 && at.right <= at.vw, 'the bubble stays on the screen');

  // The bell's list opens from its entry, inside the screen.
  await owner.go(`/campaigns/${cid}`);
  await openShelf(page);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Notifications' }).tap();
  const panel = page.locator('[data-notif-panel]');
  await panel.waitFor({ state: 'visible' });
  const p = await panel.evaluate((el) => { const r = el.getBoundingClientRect(); return { l: r.left, r: r.right, vw: document.documentElement.clientWidth }; });
  assert.ok(p.l >= 0 && p.r <= p.vw, 'the notification list stays on the screen');
  owner.assertClean();
});

test('phone: keeping a tool puts its button back on the bar, and it stays after a reload', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  await openShelf(page);
  await page.locator('#tb-shelf [data-tb-choose]').tap();
  await page.locator('#tb-shelf.is-choosing').waitFor();
  await page.locator('#tb-shelf .tb-tool', { hasText: 'GM Screen' }).tap();
  await page.locator('#tb-shelf .tb-tool', { hasText: 'View as player' }).tap();
  // Two at most: a third is refused and the shelf says so.
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Notifications' }).tap({ force: true });
  assert.equal(await page.locator('#tb-shelf .tb-tool.is-kept').count(), 2);
  assert.match(await page.locator('[data-tb-hint]').textContent(), /Two kept/);
  assert.equal(await shown(page, BAR_TOOLS.dms), true, 'the kept DM Screen button is on the bar');
  assert.equal(await shown(page, BAR_TOOLS.eye), true, 'the kept eye is on the bar');
  assert.equal(await shown(page, BAR_TOOLS.bell), false, 'a tool that was not kept stays off the bar');

  await page.locator('#tb-shelf [data-tb-choose]').tap(); // Done
  assert.equal(await page.locator('#tb-shelf.is-choosing').count(), 0);

  await owner.go(`/campaigns/${cid}`);
  assert.equal(await shown(page, BAR_TOOLS.dms), true, 'the kept DM Screen button survives a reload');
  assert.equal(await shown(page, BAR_TOOLS.eye), true);
  assert.equal(await shown(page, BAR_TOOLS.ai), false);

  // Two kept tools take the name's room (each is a 44-wide touch target),
  // but nothing runs past the bar or the page's edge.
  const fit = await page.evaluate(() => {
    const bar = document.getElementById('app-topbar').getBoundingClientRect();
    const out = [...document.querySelectorAll('#app-topbar button')].filter((b) => getComputedStyle(b).visibility === 'visible' && b.getClientRects().length)
      .filter((b) => { const r = b.getBoundingClientRect(); return r.left < bar.left - 0.5 || r.right > bar.right + 0.5; });
    return { out: out.length, sideways: document.documentElement.scrollWidth > document.documentElement.clientWidth };
  });
  assert.deepEqual(fit, { out: 0, sideways: false });

  // A kept tool's own button works from the bar.
  await page.locator(BAR_TOOLS.dms).tap();
  await page.locator('.dms-dock').waitFor({ state: 'visible' });
  owner.assertClean();
});

test('phone: viewing as a player, the shelf and the banner both lead back', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  await openShelf(page);
  await Promise.all([
    page.waitForResponse((r) => r.url().endsWith('/toggle-view-mode')),
    page.locator('#tb-shelf .tb-tool', { hasText: 'View as player' }).tap(),
  ]);
  await page.waitForSelector('[data-pv-banner]');
  const back = page.locator('[data-pv-banner] .pv-back');
  assert.equal((await back.textContent()).trim(), 'Back to your view');
  await openShelf(page);
  assert.ok((await shelfNames(page)).includes('Back to your view'));
  assert.ok(!(await shelfNames(page)).includes('Your AI'), 'an owner viewing as a player has no AI entry');
  await page.keyboard.press('Escape');
  await page.locator('#tb-shelf').waitFor({ state: 'hidden' });

  await Promise.all([page.waitForResponse((r) => r.url().endsWith('/toggle-view-mode')), back.tap()]);
  await page.waitForFunction(() => !document.querySelector('[data-pv-banner]'));
  owner.assertClean();
});

test('phone: a player gets the Tools tab with a smaller set', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, NAME);
  const player = await person({ tag: 'tbp', name: 'Pip Dane', context: PHONE });
  await addMember(owner, cid, player, 'player');
  await player.go(`/campaigns/${cid}`);
  await openShelf(player.page);
  assert.deepEqual(await shelfNames(player.page), ['The table', 'Notifications', 'Find']);
  player.assertClean();
});

test('computer: no Tools tab, and every tool is on the bar as before', async () => {
  const owner = await person({ tag: 'tbt', name: 'Mara Vell' });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  assert.equal(await page.locator('#tb-tools-tab').isVisible(), false);
  for (const [name, sel] of Object.entries(BAR_TOOLS)) assert.equal(await shown(page, sel), true, `${name} is missing from the bar`);
  owner.assertClean();
});
