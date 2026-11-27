// shell_phone.spec.mjs: the app shell and the top bar at phone width, walked
// with one phone account for the whole file, so each walk pays for its own
// steps and not for a new sign-up. Each walk makes its own campaign, and sets
// the screen size it needs.
//
// The shell: the sidebar stays out of the way and the menu button opens it, and
// a long page scrolls in one place. Stands in for the first two boxes of the
// operator check in #737. Left for a person: the sidebar glow's flicker while
// moving through it (the third box). It is a few frames of paint on a real
// phone's GPU, which a scratch browser's software renderer does not reproduce,
// so there is nothing honest to measure here, and the approved glow must be
// compared by eye.
//
// The top bar: on a phone it keeps the menu, the campaign name, search, one
// Tools tab and the picture. The Tools tab hangs a shelf with every tool and its
// name; an entry presses the real button. Above a phone the bar is as it was and
// there is no tab. The walk that keeps tools on the bar runs after the others,
// so what it keeps is never on the bar of a walk that expects it bare.
//
// RUN: tools/live-checks.sh test/live/shell_phone.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, addMember, done, picture, once, resetErrors } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };
const BAR_SIZE = { width: 360, height: 740 };
const NAME = 'Saltreach of the Shattered Isles';

// One phone owner and one phone player; the computer walk signs the owner in
// again without touch.
const accounts = once(async () => ({
  owner: await person({ tag: 'ph', name: 'Mara Vell', context: PHONE }),
  player: await person({ tag: 'tbp', name: 'Pip Dane', context: { viewport: BAR_SIZE, isMobile: true, hasTouch: true } }),
}));

beforeEach(async () => { resetErrors(...Object.values(await accounts())); });

const sidebarBox = (page) => page.locator('#sidebar').evaluate((e) => {
  const r = e.getBoundingClientRect();
  const s = getComputedStyle(e);
  return { left: r.left, right: r.right, visible: s.visibility !== 'hidden' && s.display !== 'none' };
});

// The bar's own tool buttons, by the label each one carries on the bar.
const BAR_TOOLS = {
  ai: '#app-topbar [data-ai-chip]',
  brush: '#app-topbar [data-brush-toggle]',
  table: '#app-topbar [data-tt-tab]',
  store: '#app-topbar [data-store-toggle]',
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

test('phone: the sidebar is out of the way and the menu button opens and closes it', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize(PHONE.viewport);
  const cid = await newCampaign(owner, 'Phone Shell');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  const toggle = page.locator('#nav-toggle');
  await toggle.waitFor({ state: 'visible' });
  const closed = await sidebarBox(page);
  assert.ok(closed.right <= 1 || !closed.visible, `the sidebar sits over the page at phone width (${closed.left}..${closed.right})`);
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the page scrolls sideways');
  await picture(page, 'shell-phone-closed', { check: '#737', about: 'A campaign home at phone width, menu closed', mask: ['main'] });

  await toggle.tap();
  await page.waitForFunction(() => document.getElementById('nav-toggle').getAttribute('aria-expanded') === 'true');
  await page.waitForFunction(() => { const r = document.getElementById('sidebar').getBoundingClientRect(); return r.left >= -1 && r.right > 100; });
  const open = await sidebarBox(page);
  assert.ok(open.right <= 390 + 1, 'the open sidebar is wider than the screen');
  await picture(page, 'shell-phone-open', { check: '#737', about: 'The sidebar open at phone width', mask: ['main'] });

  // The Table bar steps down while the menu is open, so the menu's bottom
  // row is the thing under a finger, not the bar.
  const footHit = await page.evaluate(() => {
    const links = [...document.querySelectorAll('#sidebar a')].filter((a) => a.getClientRects().length);
    const last = links.reduce((m, a) => (a.getBoundingClientRect().bottom > m.getBoundingClientRect().bottom ? a : m));
    const r = last.getBoundingClientRect();
    const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return { text: last.textContent.trim(), covered: !last.contains(hit), hit: hit && (hit.className || hit.tagName) };
  });
  assert.ok(!footHit.covered, `the menu's bottom row "${footHit.text}" is covered by ${footHit.hit}`);

  // Esc closes it and hands focus back to the button.
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => document.getElementById('nav-toggle').getAttribute('aria-expanded') === 'false');

  // Choosing a place closes it behind you.
  await toggle.tap();
  await page.waitForFunction(() => document.getElementById('nav-toggle').getAttribute('aria-expanded') === 'true');
  await page.locator('#sidebar a[href$="/entities"]').first().tap();
  await page.waitForURL(/\/entities/);
  await page.waitForFunction(() => document.getElementById('nav-toggle').getAttribute('aria-expanded') === 'false');
  owner.assertClean();
});

test('phone: a long page scrolls in one place', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize(PHONE.viewport);
  const cid = await newCampaign(owner, 'Phone Scroll');
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');
  const p = 'The harbour town keeps its ledgers in salt-stained books and every ship pays the toll twice. '.repeat(4);
  const entry = { type: 'doc', content: Array.from({ length: 40 }, () => ({ type: 'paragraph', content: [{ type: 'text', text: p }] })) };
  const put = await owner.api(`/campaigns/${cid}/entities/${eid}/entry`, { method: 'PUT', body: { entry: JSON.stringify(entry), entry_html: entry.content.map(() => `<p>${p}</p>`).join('') } });
  assert.ok(put.ok);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${eid}`);

  // Every element that scrolls vertically while it has more to show.
  const scrollers = await page.evaluate(() => {
    const out = [];
    const root = document.scrollingElement;
    if (root.scrollHeight > root.clientHeight + 2) out.push('document');
    for (const e of document.querySelectorAll('body *')) {
      const s = getComputedStyle(e);
      if (/(auto|scroll)/.test(s.overflowY) && e.scrollHeight > e.clientHeight + 2 && e.clientHeight > 200) out.push(e.tagName.toLowerCase() + (e.id ? '#' + e.id : ''));
    }
    return out;
  });
  assert.equal(scrollers.length, 1, 'the page scrolls in more than one place: ' + scrollers.join(', '));

  // Dragging a finger on the page moves the page, and the top bar stays where it is.
  const bar = page.locator('#nav-toggle');
  const before = await bar.boundingBox();
  await page.mouse.move(200, 600);
  await page.mouse.wheel(0, 900);
  await page.waitForTimeout(400);
  const moved = await page.evaluate(() => { const m = document.getElementById('main-content'); return Math.max(m ? m.scrollTop : 0, document.scrollingElement.scrollTop); });
  assert.ok(moved > 300, 'scrolling did not move the page');
  const after = await bar.boundingBox();
  assert.equal(Math.round(after.y), Math.round(before.y), 'the top bar scrolled away');
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the page scrolls sideways');
  owner.assertClean();
});

test('phone: All pages fits at 360 wide without sliding sideways', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize({ width: 360, height: 780 });
  const cid = await newCampaign(owner, 'Phone Lists');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities`);
  await page.locator('#main-content').waitFor();
  // The shell check above reads only the document; the list scrolls inside
  // #main-content, so a toolbar wider than the screen shows up here.
  assert.ok(await page.evaluate(() => { const m = document.getElementById('main-content'); return m.scrollWidth <= m.clientWidth; }), '#main-content slides sideways on All pages');
  const bounds = await page.locator('[data-app-opening] a[href*="entities/new"]').evaluate((e) => e.getBoundingClientRect().right);
  assert.ok(bounds <= 360, `New page ends at ${bounds}, past the screen`);

  // With a long name and tags the rows, the gallery and Select mode must
  // still fit.
  const eid = await newPage(owner, cid, 'Location', 'The Tidewrights Guild of Saltreach and the Outer Moorings Unbroken'.replace(/ /g, (m, i) => (i > 40 ? '' : m)));
  const tag = await owner.api(`/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Coast', color: '#0d9488' } });
  await owner.api(`/campaigns/${cid}/entities/${eid}/tags`, { method: 'PUT', body: { tagIds: [tag.body.id] } });
  const fits = () => page.evaluate(() => { const m = document.getElementById('main-content'); return m.scrollWidth <= m.clientWidth; });
  await owner.go(`/campaigns/${cid}/entities`);
  assert.ok(await fits(), '#main-content slides sideways on a list with a long name');
  await page.getByRole('tab', { name: /gallery/i }).click().catch(() => {});
  assert.ok(await fits(), '#main-content slides sideways on the gallery');
  owner.assertClean();
});

test('phone: the menu scrolls as one list, an open category included', async () => {
  const PHONE_SHORT = { viewport: { width: 390, height: 664 }, isMobile: true, hasTouch: true };
  const { owner } = await accounts();
  await owner.page.setViewportSize(PHONE_SHORT.viewport);
  const cid = await newCampaign(owner, 'Phone Menu');
  let eid;
  for (let i = 0; i < 16; i++) eid = await newPage(owner, cid, 'Character', 'Hero ' + String(i).padStart(2, '0'));
  const { page } = owner;
  // A page of the category paints its panel already open in the menu.
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await page.locator('#nav-toggle').tap();
  await page.waitForFunction(() => document.getElementById('nav-toggle').getAttribute('aria-expanded') === 'true');
  await page.waitForTimeout(500);
  assert.ok(await page.locator('#sidebar-drill.is-open').count(), 'the category panel is not open on its page');

  // Campaign settings sits pinned at the foot: a tap reaches it with no
  // scrolling, and it is still there once the menu has scrolled.
  const settingsHit = () => page.evaluate(() => {
    const a = document.querySelector('#sidebar .nav-manage a'), r = a.getBoundingClientRect();
    return r.bottom <= innerHeight + 1 && a.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
  });
  assert.ok(await settingsHit(), 'Campaign settings needs scrolling to reach on a phone');

  // A real finger drag, as touch events: Playwright's wheel is not a swipe.
  const cdp = await page.context().newCDPSession(page);
  const swipe = async (dy) => {
    const x = 120, y = 520;
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
    for (let i = 1; i <= 12; i++) await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y - (dy * i) / 12 }] });
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(400);
  };
  const scrolled = () => page.evaluate(() => document.getElementById('sidebar').scrollTop);
  for (let i = 0; i < 6; i++) await swipe(400);
  assert.ok(await scrolled() > 300, 'a swipe on the open category did not move the menu');
  assert.ok(await settingsHit(), 'Campaign settings scrolled away with the menu');

  // Only the menu scrolls in the drawer, and its last page row comes clear
  // of the foot.
  const inner = await page.evaluate(() => [...document.querySelectorAll('#sidebar *')].filter((e) => /(auto|scroll)/.test(getComputedStyle(e).overflowY) && e.scrollHeight > e.clientHeight + 2).map((e) => e.id || e.className));
  assert.deepEqual(inner, [], 'parts of the menu scroll on their own: ' + inner.join(', '));
  const last = await page.evaluate(() => {
    const rows = [...document.querySelectorAll('#sidebar-drill a')].filter((a) => a.getClientRects().length);
    const a = rows[rows.length - 1], r = a.getBoundingClientRect();
    return a.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
  });
  assert.ok(last, 'the open category\'s last row cannot be reached');
  await picture(page, 'shell-phone-menu-scrolled', { about: 'The phone menu scrolled to the end of an open category', mask: ['main'] });

  // Back to the categories: the list is there, and still scrolls.
  await page.locator('[data-drill-back]').tap();
  await page.waitForTimeout(400);
  assert.ok(await page.locator('#sidebar-nav-list [data-drill-open]').first().isVisible(), 'the categories did not come back');

  // Scrolled down, the close button still answers a tap (no row button
  // shows through the brand row), and a category tapped from there opens
  // with its top in view under the brand row.
  await swipe(600);
  const closeHit = await page.evaluate(() => {
    const b = document.querySelector('#sidebar .nav-x-close'), r = b.getBoundingClientRect();
    return b.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
  });
  assert.ok(closeHit, 'something in the menu covers its close button once scrolled');
  await page.locator('#sidebar-nav-list [data-drill-open]').last().tap();
  await page.locator('#sidebar-cat-content [data-drill-back]').waitFor();
  await page.waitForTimeout(400);
  const backHit = await page.evaluate(() => {
    const b = document.querySelector('#sidebar-cat-content [data-drill-back]'), r = b.getBoundingClientRect();
    return b.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
  });
  assert.ok(backHit, 'a category opened from a scrolled menu starts out of view');
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the page scrolls sideways');
  owner.assertClean();
});

test('phone: the name shows its first word and nothing in the bar is cut off', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
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
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  const tab = page.locator('#tb-tools-tab');
  assert.equal(await tab.getAttribute('aria-expanded'), 'false');
  assert.equal(await tab.getAttribute('aria-controls'), 'tb-shelf');
  await openShelf(page);
  assert.equal(await tab.getAttribute('aria-expanded'), 'true');
  assert.deepEqual(await shelfNames(page), ['Your AI', 'Customize', 'The table', 'Store', 'GM Screen', 'View as player', 'Notifications', 'Find']);

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
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
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

test('phone: viewing as a player, the shelf and the banner both lead back', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
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
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
  const cid = await newCampaign(owner, NAME);
  const { player } = await accounts();
  await addMember(owner, cid, player, 'player');
  await player.go(`/campaigns/${cid}`);
  await openShelf(player.page);
  assert.deepEqual(await shelfNames(player.page), ['The table', 'Notifications', 'Find']);
  player.assertClean();
});

test('phone: keeping a tool puts its button back on the bar, and it stays after a reload', async () => {
  const { owner } = await accounts();
  await owner.page.setViewportSize(BAR_SIZE);
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

test('computer: no Tools tab, and every tool is on the bar as before', async () => {
  // The same account on a computer: signing in again costs a page load, not a sign-up.
  const { owner: me } = await accounts();
  const owner = await person({ email: me.email, password: me.password, name: me.name, register: false });
  const cid = await newCampaign(owner, NAME);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  assert.equal(await page.locator('#tb-tools-tab').isVisible(), false);
  for (const [name, sel] of Object.entries(BAR_TOOLS)) assert.equal(await shown(page, sel), true, `${name} is missing from the bar`);
  owner.assertClean();
  await owner.ctx.close();
});
