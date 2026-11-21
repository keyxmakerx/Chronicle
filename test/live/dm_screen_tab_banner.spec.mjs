// dm_screen_tab_banner.spec.mjs — the tucked GM Screen tab and the top banner.
//
// An owner with no time zone set sees the "Times aren't in your zone yet"
// banner under the top bar. The tucked tab hangs below that banner, not in
// its row; once the banner is dismissed it sits under the top bar again, and
// the banner's controls stay whole beside an open dock.
//
// RUN: tools/live-checks.sh test/live/dm_screen_tab_banner.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done, picture, go } from './_live.mjs';

after(done);

const DESK = { viewport: { width: 1280, height: 800 } };
const WIDE_ENOUGH_TO_WRAP = { viewport: { width: 900, height: 800 } };

function boxes(page) {
  return page.evaluate(() => {
    const r = (s) => { const e = document.querySelector(s); if (!e || e.hidden) return null; const b = e.getBoundingClientRect(); return { top: b.top, bottom: b.bottom, left: b.left, right: b.right }; };
    return { tab: r('.dms-tab'), cta: r('.cta-host'), row: r('#beside-row'), bar: r('#app-topbar') };
  });
}

const overlaps = (a, b) => a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;

async function owner(context) {
  const who = await person({ tag: 'dmstab', context });
  const id = await newCampaign(who, 'Tab and banner');
  await go(who.page, `/campaigns/${id}`);
  await who.page.waitForSelector('.cta-host .cta-bar', { timeout: 15000 });
  await who.page.waitForSelector('.dms-tab:not([hidden])');
  return who;
}

test('the tab hangs below the banner, then under the top bar once it is dismissed', async () => {
  const who = await owner(DESK);
  const { page } = who;
  const a = await boxes(page);
  assert.ok(!overlaps(a.tab, a.cta), 'the tab sits in the banner');
  assert.ok(Math.abs(a.tab.top - (a.row.top + 12)) <= 1, `tab top ${a.tab.top}, row top ${a.row.top}`);

  await page.click('[data-cta-dismiss]');
  await page.waitForFunction(() => !document.querySelector('.cta-host .cta-bar'));
  await page.waitForFunction(() => {
    const t = document.querySelector('.dms-tab').getBoundingClientRect().top;
    return Math.abs(t - (document.getElementById('app-topbar').getBoundingClientRect().bottom + 12)) <= 1;
  });
  who.assertClean();
});

test('an open dock leaves the banner whole', async () => {
  const who = await owner(DESK);
  const { page } = who;
  await page.click('.dms-tab');
  await page.waitForSelector('.dms-dock:not([hidden])');
  await page.waitForTimeout(500);
  const bar = await page.evaluate(() => {
    const b = document.querySelector('.cta-bar').getBoundingClientRect();
    const dock = document.querySelector('.dms-dock').getBoundingClientRect();
    const kids = [...document.querySelectorAll('.cta-bar button, .cta-bar a')].filter((e) => e.offsetParent).map((e) => e.getBoundingClientRect());
    return { right: b.right, dockLeft: dock.left, kids: kids.map((k) => ({ left: k.left, right: k.right })) };
  });
  assert.ok(bar.right <= bar.dockLeft + 1, 'the banner runs under the dock');
  assert.ok(bar.kids.length >= 2 && bar.kids.every((k) => k.right <= bar.dockLeft + 1), 'a banner control is covered');
  who.assertClean();
});

test('a narrower window, where the banner wraps, still clears the tab', async () => {
  const who = await owner(WIDE_ENOUGH_TO_WRAP);
  const a = await boxes(who.page);
  assert.ok(!overlaps(a.tab, a.cta), 'the tab sits in the banner');
  assert.ok(Math.abs(a.tab.top - (a.row.top + 12)) <= 1);
  who.assertClean();
});
