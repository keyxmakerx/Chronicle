// wide_screen.spec.mjs — pages beside, side widgets that follow you, "On this
// page" in the margin and the 4K tip on a wide screen. Stands in for steps 1-4
// of the operator check in #1801.
//
// Covers steps 1-4 with a 2560 and a 3840 wide browser. Pressing Ctrl and + on
// the 4K screen is done by narrowing the window, which is what the browser's zoom
// does to the page's width. Left for a person: that a real 4K monitor at 100%
// browser zoom feels right (this checks the numbers, not the eye).
//
// RUN: tools/live-checks.sh test/live/wide_screen.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture, BASE } from './_live.mjs';

after(done);

// A long entry with headings, so the page scrolls and has an outline.
function longEntry() {
  const doc = { type: 'doc', content: [] };
  let html = '';
  for (let i = 1; i <= 12; i++) {
    const h = 'Chapter ' + i;
    doc.content.push({ type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: h }] });
    html += `<h2>${h}</h2>`;
    for (let j = 0; j < 3; j++) {
      const p = 'The harbour town keeps its ledgers in salt-stained books and every ship pays the toll twice. '.repeat(4);
      doc.content.push({ type: 'paragraph', content: [{ type: 'text', text: p }] });
      html += `<p>${p}</p>`;
    }
  }
  return { entry: JSON.stringify(doc), entry_html: html };
}

async function twoLinkedPages(owner, cid) {
  const a = await newPage(owner, cid, 'Location', 'Saltreach');
  const b = await newPage(owner, cid, 'Location', 'Greyhaven');
  const put = await owner.api(`/campaigns/${cid}/entities/${a}/entry`, { method: 'PUT', body: longEntry() });
  assert.ok(put.ok, 'could not write the long entry');
  const rel = await owner.api(`/campaigns/${cid}/entities/${a}/relations`, { method: 'POST', body: { targetEntityId: b, relationType: 'allied with', reverseRelationType: 'allied with' } });
  assert.ok(rel.ok, 'could not relate the pages');
  return { a, b };
}

// The page scrolls inside #main-content, not the window.
const scrollMain = (page, y) => page.evaluate((y) => { document.getElementById('main-content').scrollTop = y; }, y);

test('2560 wide: side widgets follow, On this page sits in the margin', async () => {
  const owner = await person({ tag: 'ws', name: 'Mara Vell', context: { viewport: { width: 2560, height: 1440 } } });
  const cid = await newCampaign(owner, 'Wide Check');
  const { a } = await twoLinkedPages(owner, cid);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${a}`);
  await page.locator('.page-col-stick').waitFor();

  // 1. After scrolling a long way the page's own top is far above, the side column is still in view.
  const frameTop0 = await page.locator('[data-page-frame]').evaluate((e) => e.getBoundingClientRect().top);
  const stuck0 = await page.locator('.page-col-stick').first().evaluate((e) => e.getBoundingClientRect().top);
  await scrollMain(page, 1200);
  await page.waitForTimeout(300);
  const frameTop1 = await page.locator('[data-page-frame]').evaluate((e) => e.getBoundingClientRect().top);
  const stuck1 = await page.locator('.page-col-stick').first().evaluate((e) => e.getBoundingClientRect());
  assert.ok(frameTop1 < frameTop0 - 1000, 'the page did not scroll');
  assert.ok(stuck1.top >= 0 && stuck1.top < 400 && stuck1.bottom > 0, `the side column scrolled away (top ${stuck0} then ${stuck1.top})`);
  await picture(page, 'wide-sticky-side', { check: '#1801', about: 'A long page scrolled at 2560 wide: Attributes still beside the text', mask: ['[data-page-frame] time'] });
  await scrollMain(page, 0);

  // 2. "On this page" stands past the page's right edge.
  const outline = page.locator('.chronicle-outline-margin');
  await outline.waitFor();
  const edge = await page.locator('[data-page-frame]').evaluate((e) => e.getBoundingClientRect().right);
  const o = await outline.evaluate((e) => e.getBoundingClientRect());
  assert.ok(o.left >= edge, `the outline overlaps the page (${o.left} < ${edge})`);
  assert.match(await outline.innerText(), /On this page/i);
  owner.assertClean();
});

test('2560 wide: Alt shows where a page opens; Alt-click opens it beside', async () => {
  const owner = await person({ tag: 'wb', name: 'Mara Vell', context: { viewport: { width: 2560, height: 1440 } } });
  const cid = await newCampaign(owner, 'Wide Beside');
  const { a, b } = await twoLinkedPages(owner, cid);
  const { page } = owner;
  const mainUrl = `${BASE}/campaigns/${cid}/entities/${a}`;
  await owner.go(`/campaigns/${cid}/entities/${a}`);

  // The link in the page's own Connections, not the sidebar's copy.
  const name = page.locator(`[data-widget="relations"] a[href*="/entities/${b}"]:not([data-beside-open])`).first();
  await name.waitFor();
  await name.scrollIntoViewIfNeeded();
  await name.hover();
  await page.keyboard.down('Alt');
  const ghost = page.locator('.beside-ghost.is-on');
  await ghost.waitFor();
  assert.match(await ghost.innerText(), /Greyhaven/);
  const g = await ghost.boundingBox();
  assert.ok(g.x > 2560 / 2 - 50, 'the outline is not over the right part of the screen');
  await picture(page, 'wide-beside-hint', { check: '#1801', about: 'Holding Alt over a Connections name: the dashed outline', mask: ['[data-page-frame] time'] });
  // Pointer inside the outline: it goes faint.
  await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
  await page.waitForTimeout(300);
  const faint = await page.locator('.beside-ghost').evaluate((e) => Number(getComputedStyle(e).opacity));
  await page.keyboard.up('Alt');

  await name.click({ modifiers: ['Alt'] });
  const pane = page.locator('.beside-pane');
  await pane.waitFor();
  assert.equal(page.url(), mainUrl, 'Alt-click opened the link in place');
  await pane.locator('[data-beside-here]').waitFor();
  await pane.locator('[data-beside-close]').waitFor();
  const inner = page.frameLocator('.beside-pane iframe');
  await inner.getByText('Greyhaven').first().waitFor();
  assert.doesNotMatch(await inner.locator('body').innerText(), /refused to connect/i);
  assert.ok(faint < 1, 'the outline did not go faint under the pointer');
  await picture(page, 'wide-beside-open', { check: '#1801', about: 'A page opened beside, with the x and the open-here arrow', mask: ['.beside-pane', '[data-page-frame] time'] });

  // Reload: it comes back. x closes it.
  await page.reload({ waitUntil: 'networkidle' });
  await page.locator('.beside-pane').waitFor();
  await page.locator('[data-beside-close]').click();
  await page.waitForFunction(() => !document.querySelector('.beside-pane'));

  // The icon at the end of a Connections row does the same.
  await page.locator('[data-widget="relations"] a[data-beside-open]').first().click({ force: true });
  await page.locator('.beside-pane').waitFor();
  assert.equal(page.url(), mainUrl);
  owner.assertClean();
});

test('4K: the very-wide tip shows once and never returns', async () => {
  const owner = await person({ tag: 'w4', name: 'Mara Vell', context: { viewport: { width: 3840, height: 2160 } } });
  const cid = await newCampaign(owner, 'Wide Tip');
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');
  const { page } = owner;
  await page.evaluate(() => localStorage.removeItem('chronicle-wide-tip-seen')).catch(() => {});
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const tip = page.locator('.wide-tip');
  await tip.waitFor();
  assert.match(await tip.innerText(), /This screen is very wide/);
  await picture(page, 'wide-4k-tip', { check: '#1801', about: 'The very-wide tip at 4K, lower right', mask: ['[data-page-frame] time'] });

  // Ctrl and + narrows the page: the tip goes.
  await page.setViewportSize({ width: 1920, height: 1080 });
  await tip.waitFor({ state: 'detached' });
  await page.setViewportSize({ width: 3840, height: 2160 });
  await tip.waitFor();

  // Got it: never again, even after a reload.
  await tip.getByRole('button', { name: 'Got it' }).click();
  await tip.waitFor({ state: 'detached' });
  await page.reload({ waitUntil: 'networkidle' });
  await owner.go(`/campaigns/${cid}`);
  await page.waitForTimeout(800);
  assert.equal(await page.locator('.wide-tip').count(), 0, 'the tip came back after Got it');
  owner.assertClean();
});
