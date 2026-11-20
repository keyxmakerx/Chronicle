// brochure_home.spec.mjs — the Brochure block on Home: a small box that opens
// the new-player page folded out over the app (twelve panels, three to a row,
// no table cut off on a phone, checklist boxes, hover cards that land above
// the dialog), for a player and not only the owner.
//
// RUN: tools/live-checks.sh test/live/brochure_home.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { person, newCampaign, newPage, addMember, api, done, BASE } from './_live.mjs';

after(done);

const FIXTURE = new URL('./fixtures/new-player-brochure.html', import.meta.url);

// Pastes the HTML into the page editor the way a person pasting a written
// leaflet would, then saves with Done.
async function paste(page, html) {
  await page.waitForFunction(() => document.querySelector('[data-widget="editor"]')?.classList.contains('chronicle-editor--editing'));
  await page.locator('[data-widget="editor"] .ProseMirror').click();
  await page.evaluate((h) => {
    const dt = new DataTransfer();
    dt.setData('text/html', h);
    document.querySelector('[data-widget="editor"] .ProseMirror').dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
  }, html);
  await page.waitForTimeout(500);
  await Promise.all([
    page.waitForResponse((r) => r.request().method() === 'PUT' && r.url().includes('/entry') && r.ok()),
    page.locator('[data-widget="editor"]').getByRole('button', { name: 'Done' }).click(),
  ]);
}

test('Brochure on Home: small box, full view, tables, checklist, hover card, Esc', async () => {
  const owner = await person({ tag: 'bro-o', name: 'Key Maker', context: { viewport: { width: 1280, height: 900 } } });
  const player = await person({ tag: 'bro-p', name: 'Tomas Reed', context: { viewport: { width: 1280, height: 900 } } });
  const cid = await newCampaign(owner, 'Brochure Check');
  await addMember(owner, cid, player, 'player');

  let category = 'Location';
  const saltreach = await newPage(owner, cid, category, 'Saltreach');
  const brochure = await newPage(owner, cid, category, 'New player brochure');
  await paste(owner.page, readFileSync(FIXTURE, 'utf8').replaceAll('__CAMPAIGN__', cid).replaceAll('__PAGE__', saltreach));
  // New pages start hidden from players; the brochure and the page it names are for them.
  const open = await api(owner.page, `/campaigns/${cid}/entities/bulk-visibility`, {
    method: 'POST', body: JSON.stringify({ entity_ids: [saltreach, brochure], private: false }), headers: { 'Content-Type': 'application/json' },
  });
  assert.ok(open.ok, `opening the pages to players answered ${open.status}`);

  // Place the block on the Players home the way the Customizer saves it.
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: [
    { id: 'b1', type: 'brochure', config: { entity_id: brochure, content: 'Read this before your first night.' } },
  ] }] }] };
  const put = await api(owner.page, `/campaigns/${cid}/dashboard-layout?role=player`, {
    method: 'PUT', body: JSON.stringify(layout), headers: { 'Content-Type': 'application/json' },
  });
  assert.ok(put.ok, `saving the home answered ${put.status}: ${JSON.stringify(put.body)}`);

  // The small box: title, the owner's words, a count, a quiet word.
  await player.go(`/campaigns/${cid}`);
  const box = player.page.locator('[data-block="brochure"] .cb-bro-box');
  await box.waitFor();
  const text = await box.innerText();
  assert.match(text, /New player brochure/);
  assert.match(text, /Read this before your first night\./);
  assert.match(text, /12 panels inside/);
  assert.match(text, /Open the brochure/);
  assert.equal(await player.page.locator('dialog[data-brochure-dialog]').evaluate((d) => d.open), false, 'the full view starts closed');

  // Opening: a modal dialog over the app, twelve panels, three to a row.
  await box.click();
  const dialog = player.page.locator('dialog[data-brochure-dialog]');
  await dialog.waitFor({ state: 'visible' });
  assert.equal(await player.page.locator('.cb-bro-panel').count(), 12);
  const cols = await player.page.evaluate(() => {
    const tops = [...document.querySelectorAll('.cb-bro-panel')].slice(0, 3).map((p) => Math.round(p.getBoundingClientRect().top));
    return tops;
  });
  assert.equal(new Set(cols).size, 1, `the first three panels are not on one row: ${cols}`);
  assert.equal(await player.page.locator('.cb-bro-cover h1').textContent(), 'Therin: Shattered Heroes');
  assert.equal(await player.page.locator('.cb-bro-cover hr').count(), 0, 'the cover keeps a horizontal rule');

  // Tables keep every column and none is cut off.
  const tables = await player.page.locator('.cb-bro-panel table').count();
  assert.equal(tables, 3);
  const clipped = await player.page.evaluate(() => [...document.querySelectorAll('.cb-bro-panel table')].map((t) => {
    const w = t.closest('.cb-bro-tw');
    return { cols: t.rows[0].cells.length, clipped: w.scrollWidth > w.clientWidth + 1 };
  }));
  assert.deepEqual(clipped.map((c) => c.cols), [4, 4, 3]);
  assert.ok(clipped.every((c) => !c.clipped), 'a table is cut off at desktop width');

  // The checklist is drawn as boxes, one ticked, and they are display-only.
  const boxes = await player.page.evaluate(() => [...document.querySelectorAll('.cb-bro-panel li[data-type="taskItem"]')].map((li) => {
    const b = getComputedStyle(li, '::before');
    return { checked: li.dataset.checked, w: b.width, content: b.content };
  }));
  assert.equal(boxes.length, 5);
  assert.equal(boxes.filter((b) => b.checked === 'true').length, 1);
  assert.ok(boxes.every((b) => b.w === '16px' && b.content !== 'none'), 'a checklist box is not drawn');

  // A hover card on a page mention appears above the dialog.
  const mention = player.page.locator('.cb-bro-panel a[data-entity-preview]').first();
  await mention.scrollIntoViewIfNeeded();
  await mention.hover();
  const card = player.page.locator('#chc.chc--show');
  await card.waitFor();
  assert.match(await card.innerText(), /Saltreach/);
  const top = await player.page.evaluate(() => {
    const c = document.getElementById('chc');
    const r = c.getBoundingClientRect();
    const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
    return { inDialog: !!c.closest('dialog'), onTop: !!hit && c.contains(hit) };
  });
  assert.ok(top.inDialog, 'the hover card is not seated in the dialog');
  assert.ok(top.onTop, 'the hover card is under the dialog');

  // Esc closes it, the card first and then the dialog; focus returns to the box.
  await player.page.keyboard.press('Escape');
  await player.page.waitForTimeout(250);
  if (await dialog.evaluate((d) => d.open)) await player.page.keyboard.press('Escape');
  await player.page.waitForFunction(() => !document.querySelector('dialog[data-brochure-dialog]').open);
  assert.ok(await box.evaluate((el) => el === document.activeElement), 'focus did not return to the box');

  // The close icon closes it too.
  await box.click();
  await dialog.waitFor({ state: 'visible' });
  await player.page.getByRole('button', { name: 'Close the brochure' }).click();
  await player.page.waitForFunction(() => !document.querySelector('dialog[data-brochure-dialog]').open);

  // A phone: panels stack, each says its place, and no table runs past its fold.
  const phone = await person({ tag: 'bro-ph', email: player.email, password: player.password, register: false, context: { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true } });
  await phone.go(`/campaigns/${cid}`);
  await phone.page.locator('[data-block="brochure"] .cb-bro-box').click();
  await phone.page.locator('dialog[data-brochure-dialog]').waitFor({ state: 'visible' });
  const stacked = await phone.page.evaluate(() => {
    const lefts = new Set([...document.querySelectorAll('.cb-bro-panel')].map((p) => Math.round(p.getBoundingClientRect().left)));
    const eyebrow = getComputedStyle(document.querySelectorAll('.cb-bro-panel')[4], '::before').content;
    const bodyW = document.querySelector('.cb-bro-body').clientWidth;
    const tooWide = [...document.querySelectorAll('.cb-bro-panel table')].filter((t) => t.closest('.cb-bro-tw').getBoundingClientRect().right > document.querySelector('.cb-bro-body').getBoundingClientRect().right + 1).length;
    return { oneColumn: lefts.size === 1, eyebrow, bodyW, tooWide, pageOverflow: document.documentElement.scrollWidth > innerWidth };
  });
  assert.ok(stacked.oneColumn, 'panels do not stack on a phone');
  assert.match(stacked.eyebrow, /Panel 5 of 12/);
  assert.equal(stacked.tooWide, 0, 'a table runs past its panel on a phone');
  assert.equal(stacked.pageOverflow, false, 'the phone page scrolls sideways');

  owner.assertClean();
  player.assertClean();
  phone.assertClean();
  void BASE;
});
