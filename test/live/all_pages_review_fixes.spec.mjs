// all_pages_review_fixes.spec.mjs — what the design review of All pages, the
// Create sheet and the page's add controls asked for, pinned in a browser.
//
// RUN: tools/live-checks.sh test/live/all_pages_review_fixes.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done } from './_live.mjs';

after(done);

const json = { 'Content-Type': 'application/json' };

test('rows carry the tag colours, and the pickers match Search', async () => {
  const owner = await person({ tag: 'rv1', name: 'Rev One' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Review Fixes');
  const eid = await newPage(owner, cid, 'Location', 'Aldermoor');
  const tag = await owner.api(`/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Coast', color: '#0d9488' }, headers: json });
  await owner.api(`/campaigns/${cid}/entities/${eid}/tags`, { method: 'PUT', body: { tagIds: [tag.body.id] }, headers: json });

  await owner.go(`/campaigns/${cid}/entities`);
  const chip = await page.locator('.el-row .el-tag').first().evaluate((e) => ({ bg: getComputedStyle(e).backgroundColor, ink: e.style.getPropertyValue('--chip-l') }));
  assert.match(chip.bg, /rgba\(13, 148, 136/, 'the chip is tinted with the tag colour: ' + chip.bg);
  assert.ok(chip.ink, 'the chip has its readable word colour');

  const edge = (sel) => page.locator(sel).first().evaluate((e) => getComputedStyle(e).borderTopWidth);
  assert.notEqual(await edge('#entity-list-search'), '0px', 'the find field is boxed');
  assert.notEqual(await edge('#entity-list-category'), '0px', 'the category picker is bordered');
  assert.notEqual(await edge('#entity-list-sort'), '0px', 'the order picker is bordered');
  owner.assertClean();
});

test('phone: the bar of changes clears the bottom dock and every word is a full target', async () => {
  const owner = await person({ tag: 'rv2', name: 'Rev Two', context: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Review Bar');
  for (const n of ['Aldermoor', 'Brinehold']) await newPage(owner, cid, 'Location', n);
  await owner.go(`/campaigns/${cid}/entities`);
  await page.locator('.el-select-word').click();
  await page.locator('.el-row .el-pick input').nth(0).check();
  await page.locator('.el-row .el-pick input').nth(1).check();
  await page.waitForSelector('.bulk-action-bar');
  assert.equal((await page.locator('.el-select-word').innerText()).trim(), 'Done', 'the word offers the way out while pages are picked');
  const m = await page.evaluate(() => {
    const bar = document.querySelector('.bulk-action-bar').getBoundingClientRect();
    const dock = document.querySelector('[data-tt-sheet], #tt-drawer, .tt-dock');
    const rest = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--bottom-bar')) || 0;
    return {
      bottom: bar.bottom, viewport: innerHeight, rest,
      heights: [...document.querySelectorAll('.bulk-action-bar button')].map((b) => b.getBoundingClientRect().height),
      dock: !!dock,
    };
  });
  assert.ok(m.bottom <= m.viewport - m.rest + 0.5, `the bar ends at ${m.bottom}, under the dock (${m.viewport - m.rest})`);
  assert.ok(m.heights.every((h) => h >= 43.5), 'every word is 44 tall: ' + m.heights.join(','));
  owner.assertClean();
});

test('a page with nothing shows no add controls at rest, even reached by a link; one with tags offers quiet words', async () => {
  const owner = await person({ tag: 'rv3', name: 'Rev Three' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Review Rest');
  const plain = await newPage(owner, cid, 'Location', 'Quiet Cellar');
  const tagged = await newPage(owner, cid, 'Location', 'Ravenhold');
  const tag = await owner.api(`/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Coast', color: '#0d9488' }, headers: json });
  await owner.api(`/campaigns/${cid}/entities/${tagged}/tags`, { method: 'PUT', body: { tagIds: [tag.body.id] }, headers: json });

  const visible = () => page.evaluate(() => [...document.querySelectorAll('.tp-add-btn, .rel-add-btn, .pl-add')]
    .filter((e) => e.offsetParent !== null).map((e) => e.textContent.trim()));

  await owner.go(`/campaigns/${cid}/entities/${plain}`);
  await page.locator('[data-widget="tag-picker"] .tp-add-btn').waitFor({ state: 'attached' });
  assert.deepEqual(await visible(), [], 'nothing at rest after a full load');

  // Arrive through an in-page link: first content now comes with the page.
  await owner.go(`/campaigns/${cid}/entities`);
  await page.evaluate(() => { window.__still = true; });
  await page.locator(`#main-content a[href$="/entities/${plain}"]`).first().click();
  await page.waitForURL((u) => u.pathname.endsWith(plain));
  await page.locator('[data-widget="tag-picker"] .tp-add-btn').waitFor({ state: 'attached' });
  assert.ok(await page.evaluate(() => window.__still === true), 'the link changed the page in place');
  assert.deepEqual(await visible(), [], 'nothing at rest after an in-page link');

  await owner.go(`/campaigns/${cid}/entities/${tagged}`);
  await page.locator('[data-widget="tag-picker"] .tp-add-btn').waitFor();
  const quiet = await page.locator('.tp-add-btn').evaluate((e) => ({ border: getComputedStyle(e).borderTopStyle, before: getComputedStyle(e, '::before').content }));
  assert.equal(quiet.border, 'none', 'the add word is not a dashed chip');
  assert.match(quiet.before, /\+/, 'it carries the plus');
  owner.assertClean();
});

test('the Create sheet heading is a small label and its fields share one style', async () => {
  const owner = await person({ tag: 'rv4', name: 'Rev Four' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Review Sheet');
  await owner.go(`/campaigns/${cid}/entities`);
  await page.locator('[data-app-opening] a[href*="entities/new"]').first().click();
  await page.waitForSelector('dialog.np-sheet[open] #name');
  const head = await page.locator('.np-sheet-title').evaluate((e) => ({ size: parseFloat(getComputedStyle(e).fontSize), caps: getComputedStyle(e).textTransform, hasTitle: e.classList.contains('cb-title') }));
  assert.ok(head.size <= 14 && head.caps === 'uppercase' && !head.hasTitle, 'a small capitals label: ' + JSON.stringify(head));
  await page.locator('dialog.np-sheet .np-more-word').click();
  const border = (sel) => page.locator(sel).first().evaluate((e) => getComputedStyle(e).borderTopWidth);
  for (const sel of ['dialog.np-sheet #entity_type_id', 'dialog.np-sheet #type_label', 'dialog.np-sheet [name=parent_id] ~ div input']) {
    assert.notEqual(await border(sel), '0px', sel + ' is a bordered field');
  }
  assert.equal(await page.getByText('Nest this page under another page').count(), 0, 'the old hierarchy sentence is gone');
  owner.assertClean();
});
