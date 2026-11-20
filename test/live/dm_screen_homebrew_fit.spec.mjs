// dm_screen_homebrew_fit.spec.mjs — the DM Screen's Homebrew card fits its
// column.
//
// The owner puts two Director-only trackers, one with a long name, on two
// heroes and opens the Homebrew card. On a computer and on a phone nothing in
// the card is wider than the card, Workshop and Player view sit inside it at
// the top, and each tracker shows its whole name as a row, not as a cut-off
// label.
//
// RUN: tools/live-checks.sh test/live/dm_screen_homebrew_fit.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, enableAddons, done, picture } from './_live.mjs';

after(done);

const DESK = { viewport: { width: 1280, height: 900 } };
const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };

async function seed(owner, name) {
  const cid = await newCampaign(owner, name);
  await enableAddons(owner, cid, '*');
  const kind = async (n) => {
    const r = await owner.api(`/campaigns/${cid}/tracker-kinds`, { method: 'POST', body: { name: n, shape: 'count', min: 0, max: 10, visibility: 'directors' } });
    assert.equal(r.status, 201, `the tracker kind ${n} was not made`);
    return r.body.id;
  };
  const kinds = [await kind('It hertz odd'), await kind('Respites since a long rest')];
  for (const hero of ['Orrin Goldwind', 'Saatraaol']) {
    const eid = await newPage(owner, cid, 'Character', hero);
    for (const kindId of kinds) {
      const r = await owner.api(`/campaigns/${cid}/trackers/page/${eid}`, { method: 'POST', body: { kindId } });
      assert.ok(r.ok, `a tracker was not placed on ${hero}: ${r.status}`);
    }
  }
  return cid;
}

// Everything the card must keep inside itself.
async function assertFits(page) {
  const card = page.locator('[data-dms-sec="homebrew"]');
  await card.locator('.tk-row').first().waitFor();
  const r = await card.evaluate((sec) => {
    const box = sec.getBoundingClientRect();
    const wide = Array.from(sec.querySelectorAll('*'))
      .filter((e) => e.getClientRects().length && e.getBoundingClientRect().right > box.right + 1)
      .map((e) => e.className || e.tagName);
    const acts = Array.from(sec.querySelectorAll('.dms-brew-acts > *')).map((e) => e.getBoundingClientRect());
    const names = Array.from(sec.querySelectorAll('.tk-name')).map((e) => ({ t: e.textContent, cut: e.scrollWidth > e.clientWidth + 1 }));
    const rows = Array.from(sec.querySelectorAll('.tk-row')).map((e) => getComputedStyle(e).textTransform);
    return { wide, acts: acts.map((a) => a.right <= box.right + 1 && a.width > 0), names, rows };
  });
  assert.deepEqual(r.wide, [], 'something in the Homebrew card runs past its right edge');
  assert.ok(r.acts.length === 2 && r.acts.every(Boolean), 'Workshop and Player view are not inside the card');
  assert.equal(r.names.length, 4);
  for (const n of r.names) assert.equal(n.cut, false, `the tracker name "${n.t}" is cut off`);
  assert.ok(r.rows.every((t) => t === 'none'), 'a tracker row is drawn as an uppercase label');
}

async function pictures(page, size, locator) {
  for (const dark of [false, true]) {
    await page.emulateMedia({ colorScheme: dark ? 'dark' : 'light' });
    await page.evaluate((d) => document.documentElement.classList.toggle('dark', d), dark);
    await page.waitForTimeout(100);
    await picture(page, `dm-screen-homebrew-${dark ? 'dark' : 'light'}-${size}`, { about: 'The Homebrew card with trackers on two heroes', locator });
  }
}

test('on a computer the Homebrew card fits the screen column', async () => {
  const owner = await person({ tag: 'dmh', name: 'Mara Vell', context: DESK });
  const cid = await seed(owner, 'Saltreach brew');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-dms-toggle]').click();
  await page.locator('.dms-dock').waitFor({ state: 'visible' });
  await page.locator('.dms-rail [data-dms-open="homebrew"]').click();
  await assertFits(page);
  await pictures(page, 'desk', page.locator('.dms-dock'));
  owner.assertClean();
});

test('on a phone the Homebrew line unfolds to a card that fits', async () => {
  const owner = await person({ tag: 'dmhp', name: 'Mara Vell', context: PHONE });
  const cid = await seed(owner, 'Saltreach brew phone');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.waitForFunction(() => { const h = document.querySelector('[data-widget="topbar-tools"]'); return !!h && !!h._tbDestroy; });
  await page.locator('#tb-tools-tab').tap();
  await page.locator('#tb-shelf').waitFor({ state: 'visible' });
  await page.waitForTimeout(300);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Screen' }).tap();
  await page.locator('[data-dms-sec="homebrew"] [data-dms-fold]').tap();
  await assertFits(page);
  await page.locator('[data-dms-sec="homebrew"]').scrollIntoViewIfNeeded();
  await pictures(page, 'phone', page.locator('[data-dms-sec="homebrew"]'));
  owner.assertClean();
});
