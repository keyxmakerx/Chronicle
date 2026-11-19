// topbar_menus_over_page.spec.mjs — a menu opened from the top bar lies over
// the page under it, even where the page raises a toolbar of its own, as the
// rulebook does. A raised strip at the top of the page stands in for that
// toolbar; the campaign menu must still be what a finger lands on.
//
// RUN: tools/live-checks.sh test/live/topbar_menus_over_page.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };

test('the campaign menu opens over a raised page toolbar on a phone', async () => {
  const owner = await person({ tag: 'tbm', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, 'Saltreach');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.evaluate(() => {
    const strip = document.createElement('div');
    strip.id = 'raised-strip';
    strip.style.cssText = 'position:relative;z-index:60;height:160px;margin:-12px -12px 0;background:#402';
    document.getElementById('main-content').prepend(strip);
  });
  await page.locator('#app-topbar button:has([data-topbar-name])').tap();
  const menu = page.locator('#app-topbar .tb-pop', { hasText: 'Saltreach' });
  await menu.waitFor({ state: 'visible' });
  await page.waitForTimeout(400);
  const on = await menu.evaluate((el) => {
    const bar = document.getElementById('app-topbar').getBoundingClientRect();
    const box = el.getBoundingClientRect();
    const strip = document.getElementById('raised-strip').getBoundingClientRect();
    const y = Math.max(bar.bottom, strip.top) + 12;
    const hit = document.elementFromPoint(box.left + box.width / 2, y);
    return { overlaps: box.bottom > y && strip.bottom > y, reached: !!hit && el.contains(hit) };
  });
  assert.ok(on.overlaps, 'the menu and the raised strip share a spot to test');
  assert.ok(on.reached, 'the menu lies over the raised strip');

  // Closed again, the bar goes back under the page's full-screen layers.
  await page.locator('#main-content').tap({ position: { x: 200, y: 400 } });
  await menu.waitFor({ state: 'hidden' });
  await page.waitForFunction(() => getComputedStyle(document.getElementById('app-topbar')).zIndex === '20');
});
