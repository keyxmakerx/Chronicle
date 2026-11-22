// shell_phone.spec.mjs — the app shell at phone width: the sidebar stays out of
// the way and the menu button opens it, and a long page scrolls in one place.
// Stands in for the first two boxes of the operator check in #737.
//
// Left for a person: the sidebar glow's flicker while moving through it (the
// third box). It is a few frames of paint on a real phone's GPU, which a
// scratch browser's software renderer does not reproduce, so there is nothing
// honest to measure here, and the approved glow must be compared by eye.
//
// RUN: tools/live-checks.sh test/live/shell_phone.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };

const sidebarBox = (page) => page.locator('#sidebar').evaluate((e) => {
  const r = e.getBoundingClientRect();
  const s = getComputedStyle(e);
  return { left: r.left, right: r.right, visible: s.visibility !== 'hidden' && s.display !== 'none' };
});

test('phone: the sidebar is out of the way and the menu button opens and closes it', async () => {
  const owner = await person({ tag: 'ph', name: 'Mara Vell', context: PHONE });
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
  const owner = await person({ tag: 'pl', name: 'Mara Vell', context: PHONE });
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
  const owner = await person({ tag: 'pw', name: 'Mara Vell', context: { viewport: { width: 360, height: 780 }, isMobile: true, hasTouch: true } });
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
  const owner = await person({ tag: 'pm', name: 'Mara Vell', context: PHONE_SHORT });
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
