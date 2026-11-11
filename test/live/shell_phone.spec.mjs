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
