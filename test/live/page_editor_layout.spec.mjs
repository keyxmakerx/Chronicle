// page_editor_layout.spec.mjs — the page editor in Customize > Categories >
// Page layout: a real page drawn in the editor, drag to place and resize,
// the player view, tiles on phone, a draft that survives reload, Save, and the
// old config address. Stands in for the operator check in #1383.
//
// Covers steps 1-7. The drags use real mouse moves; the editor's own drag is
// pointer-based and takes a moment to settle on a spot, so each drag rests
// over its target before letting go. The wording in the issue is older than
// the screen: "Large" is Tall and "Only the DM sees it" is "GM team only".
// Whether the drag *feels* right is left for a person.
//
// RUN: tools/live-checks.sh test/live/page_editor_layout.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture, BASE } from './_live.mjs';

after(done);

// A block or tile by its widget type; new ones get random ids.
const ofType = (page, type, scope = '.cpe-page') => page.locator(`${scope} :is(.cpe-blk, .cpe-tile):has([data-type="${type}"])`).first();

// The block types in each column or tile grid of each section, in order.
const layoutOf = (page) => page.evaluate(() => [...document.querySelectorAll('.cpe-page .cpe-sec')].map((s) => ({
  tag: s.querySelector('.cpe-sec-tag').textContent.trim(),
  parts: [...s.querySelectorAll('.cpe-col, .cpe-tiles')].map((c) => [...c.querySelectorAll(':scope > .cpe-blk, :scope > .cpe-tile')]
    .map((b) => { const l = b.querySelector('[data-type]'); return l ? l.dataset.type : (b.dataset.id || ''); })),
})));

async function drag(page, from, to, off = { x: 0, y: 0 }) {
  await from.scrollIntoViewIfNeeded();
  const a = await from.boundingBox();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(a.x + a.width / 2 + 12, a.y + a.height / 2 + 12, { steps: 3 });
  const b = await to.boundingBox();
  // The save bar floats over the bottom of the canvas and would catch the drop.
  const x = b.x + b.width / 2 + off.x, y = Math.min(b.y + b.height / 2 + off.y, page.viewportSize().height - 140);
  await page.mouse.move(x, y, { steps: 12 });
  // The gap only moves once the pointer rests on a spot.
  await page.waitForTimeout(350);
  await page.mouse.move(x + 2, y + 2, { steps: 2 });
  await page.waitForTimeout(300);
  await page.mouse.up();
  await page.waitForTimeout(500);
}

test('page layout editor: place, size, player view, tiles, draft, save', async () => {
  const owner = await person({ tag: 'ple', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const cid = await newCampaign(owner, 'Layout Check');
  const { page } = owner;
  await newPage(owner, cid, 'Location', 'Highmoor');
  const second = await newPage(owner, cid, 'Location', 'Saltreach');

  // 1. A real page in the editor, with the gallery and the page picker.
  await owner.go(`/campaigns/${cid}/customize/categories`);
  // The rail's drawer lists each category too; the card grid is in main.
  await owner.go(await page.locator('main a[href*="/customize/categories/"]').filter({ hasText: 'Locations' }).last().getAttribute('href'));
  await page.locator('.cpe-sec').first().waitFor();
  assert.match(await page.locator('.cpe-page').innerText(), /highmoor|saltreach/i, 'the editor does not draw a real location');
  assert.ok(await page.locator('.cpe-live:not(:has(.cpe-wait))').count(), 'the widgets are still grey boxes');
  assert.ok(await page.locator('.cpe-gw').count() > 5, 'no widget gallery');
  await page.locator('#cpe-page').waitFor();
  const layoutUrl = page.url();
  await picture(page, 'page-editor-layout', { check: '#1383', about: 'Locations page layout: a real page with the widget gallery', mask: ['.cpe-page', '.sidebar', '[data-sidebar]', '#cpe-page'] });

  const before = await layoutOf(page);
  assert.equal(before[0].parts.length, 2, 'the first section is not two columns');

  // 2. Drag a widget from the gallery, move one, drag a divider; Undo.
  const col2 = page.locator('.cpe-page .cpe-sec').first().locator('.cpe-col').nth(1);
  await drag(page, page.locator('.cpe-gw[data-gw="text_block"]'), col2);
  let now = await layoutOf(page);
  assert.ok(now[0].parts[1].includes('text_block'), 'the gallery widget did not land in the second column: ' + JSON.stringify(now[0]));
  await page.locator('[data-czs-what]').filter({ hasText: 'Locations page layout' }).waitFor();

  const col1 = page.locator('.cpe-page .cpe-sec').first().locator('.cpe-col').first();
  await drag(page, ofType(page, 'details'), col1.locator('.cpe-blk').first(), { x: 0, y: 4 });
  now = await layoutOf(page);
  assert.ok(now[0].parts[0].includes('details'), 'an existing widget did not move to the first column: ' + JSON.stringify(now[0]));

  const widthsBefore = await page.locator('.cpe-page .cpe-sec').first().locator('.cpe-cols').evaluate((e) => getComputedStyle(e).gridTemplateColumns);
  await page.locator('.cpe-page .cpe-sec').first().hover();
  const div = page.locator('.cpe-page .cpe-sec').first().locator('.cpe-div').first();
  const d = await div.boundingBox();
  await page.mouse.move(d.x + d.width / 2, d.y + 40);
  await page.mouse.down();
  await page.mouse.move(d.x - 120, d.y + 40, { steps: 8 });
  await page.mouse.up();
  const widthsAfter = await page.locator('.cpe-page .cpe-sec').first().locator('.cpe-cols').evaluate((e) => getComputedStyle(e).gridTemplateColumns);
  assert.notEqual(widthsAfter, widthsBefore, 'dragging the column divider changed nothing');

  // Undo steps back through the three changes.
  for (let i = 0; i < 3; i++) await page.locator('[data-act="undo"]').click();
  assert.deepEqual(await layoutOf(page), before, 'Undo did not put the layout back');
  for (let i = 0; i < 3; i++) await page.locator('[data-act="redo"]').click();

  // 3. Tall + GM team only on the text block; the player view hides it.
  await ofType(page, 'text_block').click();
  await page.getByRole('button', { name: 'Tall', exact: true }).click();
  await page.getByRole('button', { name: /GM team only/ }).last().click();
  assert.ok(await ofType(page, 'text_block').isVisible());
  await page.locator('[data-act="as"][data-v="player"]').click();
  await page.waitForFunction(() => !document.querySelector('.cpe-page .cpe-blk [data-type="text_block"]'));
  await page.locator('[data-act="as"][data-v="dm"]').click();
  await ofType(page, 'text_block').waitFor();

  // 4. A Tiles section with two widgets, one resized; Phone shows two across.
  // Added at the top, where the floating save bar can't cover the drop.
  await page.locator('.cpe-ins button[data-act="addsec"]').first().click();
  await page.locator('[data-sk="tiles"]').click();
  const tilesSec = page.locator('.cpe-page .cpe-sec').filter({ hasText: /Tiles/ }).first();
  await tilesSec.waitFor();
  // Selecting a widget switches the panel to its settings; the gallery is a tab away.
  // The canvas scrolls itself near its edges, so aim from its middle.
  await page.locator('.cpe-canvas').evaluate((e) => { e.scrollTop = 0; });
  await page.getByRole('tab', { name: 'Add widgets' }).click();
  await drag(page, page.locator('.cpe-gw[data-gw="text_block"]'), tilesSec.locator('.cpe-tiles'));
  await page.getByRole('tab', { name: 'Add widgets' }).click();
  await drag(page, page.locator('.cpe-gw[data-gw="text_block"]'), tilesSec.locator('.cpe-tiles'));
  assert.equal(await tilesSec.locator('.cpe-tile').count(), 2, 'two widgets did not land in the tiles section');
  const tiles = tilesSec.locator('.cpe-tile');
  await tiles.first().waitFor();
  assert.ok(await tiles.count() >= 1, 'no tile landed');
  const tile = tiles.first();
  await tile.hover();
  const sizeBefore = await tile.locator('.cpe-tbar-s').innerText();
  const r = await tile.locator('.cpe-rsz').boundingBox();
  await page.mouse.move(r.x + r.width / 2, r.y + r.height / 2);
  await page.mouse.down();
  await page.mouse.move(r.x + 220, r.y + 180, { steps: 10 });
  await page.mouse.up();
  const sizeAfter = await tile.locator('.cpe-tbar-s').innerText();
  assert.match(sizeAfter, /^\d×\d$/, 'tile sizes are not set sizes');
  assert.notEqual(sizeAfter, sizeBefore, 'pulling the corner did not resize the tile');
  await page.locator('[data-act="dev"][data-v="phone"]').click();
  const across = await page.locator('.cpe-page .cpe-tiles').first().evaluate((e) => getComputedStyle(e).gridTemplateColumns.split(' ').length);
  assert.equal(across, 2, 'on Phone the tiles are not two across');
  await page.locator('[data-act="dev"][data-v="desktop"]').click();

  // 5. Reload before saving: the draft is still there.
  const arranged = await layoutOf(page);
  await page.waitForTimeout(1500);
  await page.reload({ waitUntil: 'networkidle' });
  await page.locator('.cpe-sec').first().waitFor();
  await page.locator('.cpe-live:not(:has(.cpe-wait))').first().waitFor();
  assert.deepEqual(await layoutOf(page), arranged, 'the draft did not survive a reload');
  await page.locator('[data-czs-what]').filter({ hasText: 'Locations page layout' }).waitFor();

  // 6. Save, then the page itself wears the new arrangement.
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
  const reader = await owner.ctx.newPage();
  await reader.goto(`${BASE}/campaigns/${cid}/entities/${second}`, { waitUntil: 'networkidle' });
  // The saved arrangement opens with the tiles section I added at the top.
  assert.ok(await reader.locator('[data-page-frame]').count(), 'the page did not draw after Save');
  assert.equal(await reader.locator('[data-page-frame] > .tile-grid').count(), 1, 'the saved Tiles section is not on the page');
  assert.equal(await reader.locator('[data-page-frame] > .tile-grid > *').count(), 2, 'the Tiles section does not hold its two widgets');
  await reader.close();

  // 7. The old config address lands on the Page layout tab.
  const etid = layoutUrl.match(/categories\/(\d+)\//)[1];
  await page.goto(`${BASE}/campaigns/${cid}/entity-types/${etid}/config`, { waitUntil: 'networkidle' });
  assert.match(page.url(), new RegExp(`/customize/categories/${etid}/layout`));
  owner.assertClean();
});
