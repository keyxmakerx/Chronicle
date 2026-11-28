// dm_screen_leaves.spec.mjs — dragging a DM Screen section out onto a leaf.
//
// On a computer, a rail icon dragged toward the page opens that section on a
// leaf beside the column, lying over the page; a second icon opens a second
// leaf. The leaves come back after a reload. With both out, a third icon
// dropped on a leaf joins it, and one dropped on the column opens there. A
// leaf's bar merges it back into the column, or closes it, and dragging the
// bar onto the column merges it there. A plain click on a rail icon still
// opens the card in the column. On a phone there are no leaves: every section
// is a line, as before.
//
// RUN: tools/live-checks.sh test/live/dm_screen_leaves.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, enableAddons, done, picture } from './_live.mjs';

after(done);

const DESK = { viewport: { width: 1600, height: 900 } };
const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };

async function theme(page, dark) {
  await page.emulateMedia({ colorScheme: dark ? 'dark' : 'light' });
  await page.evaluate((d) => document.documentElement.classList.toggle('dark', d), dark);
  await page.waitForTimeout(100);
}

async function pictures(page, state, size, about) {
  for (const dark of [false, true]) {
    await theme(page, dark);
    await picture(page, `dm-screen-leaves-${state}-${dark ? 'dark' : 'light'}-${size}`, { about });
  }
  await theme(page, false);
}

// Drags a rail icon out toward the page by `dist` pixels, in steps, the way
// a hand does, and lets go.
async function dragOut(page, id, dist) {
  const box = await page.locator(`.dms-rail [data-dms-open="${id}"]`).boundingBox();
  const x = box.x + box.width / 2, y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  for (let i = 1; i <= 12; i++) await page.mouse.move(x - (dist * i) / 12, y + i);
  await page.mouse.up();
  await page.waitForTimeout(450);
}

// What each leaf holds, in order: [['adventures'], ['homebrew', ...]].
function leaves(page) {
  return page.evaluate(() => Array.from(document.querySelectorAll('.dms-leaf:not(.dms-leaf-ghost)'))
    .sort((a, b) => a.dataset.dmsLeaf - b.dataset.dmsLeaf)
    .map((l) => Array.from(l.querySelectorAll('[data-dms-sec]')).map((s) => s.dataset.dmsSec)));
}

function column(page) {
  return page.evaluate(() => Array.from(document.querySelectorAll('[data-dms-cards] > [data-dms-sec]:not([hidden])'))
    .sort((a, b) => (+a.style.order) - (+b.style.order))
    .map((s) => s.dataset.dmsSec));
}

async function openDock(owner, cid) {
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-dms-toggle]').click();
  await page.locator('.dms-dock').waitFor({ state: 'visible' });
  await page.locator('[data-dms-sec="world"]').waitFor({ state: 'visible' });
}

test('a rail icon dragged toward the page opens its section on a leaf, and the leaf merges or closes', async () => {
  const owner = await person({ tag: 'dml', name: 'Mara Vell', context: DESK });
  const cid = await newCampaign(owner, 'Saltreach leaves');
  await enableAddons(owner, cid, '*');
  const { page } = owner;
  await openDock(owner, cid);
  assert.deepEqual(await column(page), ['party', 'world']);

  // A short tug that never passes the column's edge swings shut again.
  await dragOut(page, 'adventures', 40);
  assert.deepEqual(await leaves(page), [], 'a tug short of the hinge opened a leaf');

  // Past the hinge the leaf opens with Adventures on it, over the page.
  await dragOut(page, 'adventures', 420);
  assert.deepEqual(await leaves(page), [['adventures']]);
  const leaf = page.locator('.dms-leaf[data-dms-leaf="0"]');
  assert.equal(await leaf.getAttribute('aria-label'), 'Leaf 2: Adventures');
  const dock = await page.locator('.dms-dock').boundingBox();
  const lb = await leaf.boundingBox();
  assert.ok(Math.abs(lb.x + lb.width - dock.x) < 2, 'the leaf is not hinged on the column edge');
  assert.equal(await page.locator('[data-dms-open="adventures"]').getAttribute('aria-pressed'), 'true');
  assert.deepEqual(await column(page), ['party', 'world'], 'the column changed');

  // A second icon opens a second leaf, further out.
  await dragOut(page, 'homebrew', 700);
  assert.deepEqual(await leaves(page), [['adventures'], ['homebrew']]);
  await page.locator('.dms-leaf[data-dms-leaf="1"] .hbd').waitFor();
  await pictures(page, 'two', 'desk', 'Two leaves out beside the column: Adventures and Homebrew');

  // Both leaves come back after a reload; the dock was out, so it stays out.
  await page.reload();
  await page.locator('.dms-leaf[data-dms-leaf="1"]').waitFor();
  assert.deepEqual(await leaves(page), [['adventures'], ['homebrew']]);

  // With both out, Reveal dropped on the first leaf joins it as its front card.
  const l0 = await page.locator('.dms-leaf[data-dms-leaf="0"]').boundingBox();
  const rv = await page.locator('.dms-rail [data-dms-open="reveal"]').boundingBox();
  await page.mouse.move(rv.x + rv.width / 2, rv.y + rv.height / 2);
  await page.mouse.down();
  for (let i = 1; i <= 15; i++) await page.mouse.move(rv.x - ((rv.x - (l0.x + 100)) * i) / 15, rv.y + 20 * i);
  assert.equal(await page.locator('.dms-leaf[data-dms-leaf="0"]').evaluate((e) => e.classList.contains('dms-hot')), true, 'the leaf under the pointer is not marked');
  await page.mouse.up();
  await page.waitForTimeout(300);
  assert.deepEqual(await leaves(page), [['adventures', 'reveal'], ['homebrew']]);
  // With both out, the column takes a drop too: Notes dropped on it opens
  // there, and no leaf changes.
  const colBox = await page.locator('.dms-col').boundingBox();
  const nt = await page.locator('.dms-rail [data-dms-open="notes"]').boundingBox();
  await page.mouse.move(nt.x + nt.width / 2, nt.y + nt.height / 2);
  await page.mouse.down();
  for (let i = 1; i <= 12; i++) await page.mouse.move(nt.x - ((nt.x - (colBox.x + 120)) * i) / 12, nt.y + 10 * i);
  assert.equal(await page.locator('.dms-col').evaluate((e) => e.classList.contains('dms-hot')), true, 'the column is not marked as a drop target');
  await page.mouse.up();
  await page.waitForTimeout(300);
  assert.deepEqual(await column(page), ['notes', 'party', 'world']);
  assert.deepEqual(await leaves(page), [['adventures', 'reveal'], ['homebrew']]);
  await page.locator('.dms-rail [data-dms-open="notes"]').click();
  assert.deepEqual(await column(page), ['party', 'world']);

  // Both cards on the leaf stay open: the older one is not forced to fold.
  assert.equal(await page.locator('.dms-leaf [data-dms-sec="adventures"]').evaluate((e) => e.classList.contains('dms-folded')), false, 'the older card on the leaf was folded');
  assert.equal(await page.locator('.dms-leaf [data-dms-sec="reveal"]').evaluate((e) => e.classList.contains('dms-folded')), false);
  // A header folds just its own card.
  await page.locator('.dms-leaf [data-dms-sec="adventures"] [data-dms-fold]').click();
  assert.equal(await page.locator('.dms-leaf [data-dms-sec="adventures"]').evaluate((e) => e.classList.contains('dms-folded')), true);
  assert.equal(await page.locator('.dms-leaf [data-dms-sec="reveal"]').evaluate((e) => e.classList.contains('dms-folded')), false);
  await page.locator('.dms-leaf [data-dms-sec="adventures"] [data-dms-fold]').click();

  // A card dragged by its title lands where it is dropped: reveal goes above
  // adventures on the same leaf, then across to the column.
  const dragCard = async (id, to, below) => {
    const h = await page.locator(`[data-dms-sec="${id}"] .dms-sec-t`).boundingBox();
    const t = await page.locator(to).boundingBox();
    const x = h.x + 60, y = h.y + h.height / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    const tx = t.x + t.width / 2, ty = below ? t.y + t.height - 4 : t.y + 4;
    for (let i = 1; i <= 10; i++) await page.mouse.move(x + ((tx - x) * i) / 10, y + ((ty - y) * i) / 10);
    await page.mouse.up();
    await page.waitForTimeout(300);
  };
  await dragCard('reveal', '.dms-leaf [data-dms-sec="adventures"]', false);
  assert.deepEqual(await leaves(page), [['reveal', 'adventures'], ['homebrew']]);
  await dragCard('reveal', '[data-dms-cards] [data-dms-sec="party"]', true);
  assert.deepEqual(await leaves(page), [['adventures'], ['homebrew']]);
  assert.deepEqual(await column(page), ['party', 'reveal', 'world']);
  await dragCard('reveal', '.dms-leaf [data-dms-sec="adventures"]', true);
  assert.deepEqual(await leaves(page), [['adventures', 'reveal'], ['homebrew']]);

  // A folded card dropped on itself stays where it is, and stays folded.
  await page.locator('[data-dms-sec="party"] [data-dms-fold]').click();
  const before = await column(page);
  const gr = await page.locator('[data-dms-sec="party"] [data-dms-grip]').boundingBox();
  const own = await page.locator('[data-dms-sec="party"]').boundingBox();
  const gx = gr.x + gr.width / 2, gy = gr.y + gr.height / 2;
  await page.mouse.move(gx, gy);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) await page.mouse.move(gx + (own.x + own.width / 2 - gx) * i / 8, gy + 2 * i);
  await page.mouse.up();
  await page.waitForTimeout(300);
  assert.deepEqual(await column(page), before, 'a drop on its own card moved it');
  assert.equal(await page.locator('[data-dms-sec="party"]').evaluate((e) => e.classList.contains('dms-folded')), true, 'a drop on its own card unfolded it');
  await page.locator('[data-dms-sec="party"] [data-dms-fold]').click();

  // A plain click on a rail icon still opens the card in the column.
  await page.locator('.dms-rail [data-dms-open="notes"]').click();
  assert.deepEqual(await column(page), ['notes', 'party', 'world']);
  // Clicking the icon of a section on a leaf closes it there.
  await page.locator('.dms-rail [data-dms-open="reveal"]').click();
  await page.waitForTimeout(300);
  assert.deepEqual(await leaves(page), [['adventures'], ['homebrew']]);

  // Merge folds the first leaf into the column; the second slides in.
  await page.locator('.dms-leaf[data-dms-leaf="0"] [data-dms-lmerge]').click();
  await page.waitForTimeout(450);
  assert.deepEqual(await leaves(page), [['homebrew']]);
  assert.deepEqual(await column(page), ['adventures', 'notes', 'party', 'world']);
  const slid = await page.locator('.dms-leaf[data-dms-leaf="0"]').boundingBox();
  assert.ok(Math.abs(slid.x + slid.width - dock.x) < 2, 'the remaining leaf did not slide in to the column');

  // Dragging the leaf's bar onto the column merges it there.
  const bar = await page.locator('.dms-leaf[data-dms-leaf="0"] .dms-leafbar').boundingBox();
  await page.mouse.move(bar.x + 60, bar.y + bar.height / 2);
  await page.mouse.down();
  for (let i = 1; i <= 12; i++) await page.mouse.move(bar.x + 60 + (dock.x + 140 - bar.x - 60) * i / 12, bar.y + 200);
  assert.equal(await page.locator('.dms-col').evaluate((e) => e.classList.contains('dms-hot')), true, 'the column is not marked as the drop target');
  await page.mouse.up();
  await page.waitForTimeout(450);
  assert.deepEqual(await leaves(page), []);
  assert.equal((await column(page))[0], 'homebrew', 'the merged card is not in front');

  // Close folds a leaf away with its cards.
  await dragOut(page, 'party', 420);
  assert.deepEqual(await leaves(page), [['party']]);
  await page.locator('.dms-leaf[data-dms-leaf="0"] [data-dms-lshut]').click();
  await page.waitForTimeout(450);
  assert.deepEqual(await leaves(page), []);
  assert.equal(await page.locator('[data-dms-open="party"]').getAttribute('aria-pressed'), 'false');
  owner.assertClean();
});

test('on a phone the leaves stay away and every section is a line', async () => {
  const owner = await person({ tag: 'dmlp', name: 'Mara Vell', context: PHONE });
  const cid = await newCampaign(owner, 'Saltreach leaves phone');
  await enableAddons(owner, cid, '*');
  const { page } = owner;
  // Leaves saved from a computer in this browser do not show on a phone.
  await owner.go(`/campaigns/${cid}`);
  await page.evaluate((c) => localStorage.setItem('chronicle.dms.leaves.' + c, JSON.stringify([['adventures']])), cid);
  await owner.go(`/campaigns/${cid}`);
  await page.waitForFunction(() => { const h = document.querySelector('[data-widget="topbar-tools"]'); return !!h && !!h._tbDestroy; });
  await page.locator('#tb-tools-tab').tap();
  await page.locator('#tb-shelf').waitFor({ state: 'visible' });
  await page.waitForTimeout(300);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Screen' }).tap();
  await page.locator('[data-dms-sec="world"]').waitFor({ state: 'visible' });
  assert.equal(await page.locator('.dms-leaf').count() === 0 || !(await page.locator('.dms-leaves').isVisible()), true, 'a leaf shows on a phone');
  assert.equal(await page.locator('[data-dms-cards] > [data-dms-sec="adventures"]').isVisible(), true, 'Adventures is not a line on a phone');
  owner.assertClean();
});
