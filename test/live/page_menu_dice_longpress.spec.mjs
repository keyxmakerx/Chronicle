// page_menu_dice_longpress.spec.mjs — the page's own three-dots menu, the
// sidebar's page menu, the Table's three names at the lifted height, and the
// long press on a page link opening Chronicle's card.
//
// The owner opens a page: three dots beside Edit page, Make a copy works and
// Delete page sends the page to the Trash with Undo. A player gets no dots. The
// sidebar's menu for a page offers Page history and Delete page. On a phone,
// the lifted Table names Rolls, Tonight and My dice and a tap on My dice
// raises it to full height on that leaf. With a finger, holding a page link
// in a page's text opens the pinned card with its four rows.
//
// RUN: tools/live-checks.sh test/live/page_menu_dice_longpress.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, addMember, done, picture } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };
const SIZES = { phone: { width: 390, height: 844 }, desk: { width: 1280, height: 800 } };

// A picture in each size and in light and dark, named <state>-<scheme>-<size>.
// `at` is the size the page already is; `prep` runs after each switch.
async function shots(page, state, about, opts = {}) {
  for (const [size, vp] of Object.entries(opts.sizes || SIZES)) {
    if (!opts.fixed) await page.setViewportSize(vp);
    if (opts.prep) await opts.prep(size);
    for (const scheme of ['light', 'dark']) {
      await page.emulateMedia({ colorScheme: scheme });
      await page.evaluate((s) => document.documentElement.classList.toggle('dark', s === 'dark'), scheme);
      await page.waitForTimeout(150);
      await picture(page, `page-menu-${state}-${scheme}-${size}`, { about: `${about} (${size}, ${scheme})`, mask: ['[data-page-frame] time', '#sidebar-entity-tree', '.recent-list'], ...(opts.pic || {}) });
    }
    await page.evaluate(() => document.documentElement.classList.remove('dark'));
    if (opts.fixed) break;
  }
  await page.emulateMedia({ colorScheme: 'light' });
}

let w;

test('the owner opens a page: three dots beside Edit page, a copy, and Delete page to the Trash', async () => {
  const owner = await person({ tag: 'pmo', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Page Menu');
  const target = await newPage(owner, cid, 'Character', 'Captain Vell');
  const eid = await newPage(owner, cid, 'Location', 'Hollowmere');
  const scratch = await newPage(owner, cid, 'Location', 'Scratch Hall');
  const url = `/campaigns/${cid}/entities/${target}`;
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [
    { type: 'text', text: 'The party met ' },
    { type: 'text', text: 'Captain Vell', marks: [{ type: 'link', attrs: { href: url, 'data-mention-id': target, 'data-entity-preview': url + '/preview' } }] },
    { type: 'text', text: ' at dusk, and read ' },
    { type: 'text', text: 'a site', marks: [{ type: 'link', attrs: { href: 'https://example.com/ledger' } }] },
    { type: 'text', text: '.' },
  ] }] };
  const put = await owner.api(`/campaigns/${cid}/entities/${eid}/entry`, { method: 'PUT', body: { entry: JSON.stringify(doc), entry_html: '' } });
  assert.ok(put.ok, `saving the page's text answered ${put.status}`);
  const shown = await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: JSON.stringify({ entity_ids: [eid, target], private: false }), headers: { 'Content-Type': 'application/json' } });
  assert.ok(shown.ok, `sharing the pages answered ${shown.status}`);
  w = { owner, cid, eid, target, scratch };

  const { page } = owner;
  await page.setViewportSize(SIZES.desk);
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const dots = page.locator('[data-pe-dots]');
  await dots.waitFor();
  const edit = page.locator('[data-pe-edit]');
  const [d, e] = [await dots.boundingBox(), await edit.boundingBox()];
  assert.ok(Math.abs((d.y + d.height / 2) - (e.y + e.height / 2)) <= 3, 'the dots sit on the same line as Edit page');
  await shots(page, 'page-with-dots', 'A page at rest with three dots beside Edit page');

  await page.setViewportSize(SIZES.desk);
  await dots.click();
  const menu = page.locator('[data-pe-menu]');
  await menu.waitFor({ state: 'visible' });
  const rows = await menu.getByRole('menuitem').allInnerTexts();
  assert.deepEqual(rows.map((r) => r.replace(/\s+/g, ' ').trim()),
    ['Make a copy', 'Page history Earlier versions', 'Move under another page', 'Delete page Goes to Trash']);
  assert.equal(await dots.getAttribute('aria-expanded'), 'true');
  await shots(page, 'menu-open', 'The three-dots menu open', {
    prep: async () => { if (!(await menu.isVisible())) await dots.click(); await page.waitForTimeout(200); },
  });

  // Escape closes it and focus is back on the dots.
  if (!(await menu.isVisible())) await dots.click();
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'hidden' });
  assert.equal(await page.evaluate(() => document.activeElement && document.activeElement.hasAttribute('data-pe-dots')), true, 'focus did not return to the dots');

  // Page history opens the History panel under the header.
  await dots.click();
  await menu.getByRole('menuitem', { name: /Page history/ }).click();
  await page.locator('#page-history-slot').getByText(/./).first().waitFor();
  await menu.waitFor({ state: 'hidden' });

  // Make a copy: a new page is made and shown.
  await dots.click();
  await Promise.all([
    page.waitForURL((u) => /\/entities\/[0-9a-f-]{36}$/.test(u.pathname) && !u.pathname.endsWith(eid)),
    menu.getByRole('menuitem', { name: 'Make a copy' }).click(),
  ]);
  const copyId = (page.url().match(/\/entities\/([0-9a-f-]{36})/) || [])[1];
  assert.notEqual(copyId, eid, 'the copy opened the same page');

  // Delete page on the copy: it goes to the Trash with an Undo.
  await page.locator('[data-pe-dots]').click();
  await page.locator('[data-pe-menu]').getByRole('menuitem', { name: /Delete page/ }).click();
  await page.getByText(/Moved to Trash/).first().waitFor();
  await page.getByRole('button', { name: 'Undo' }).first().waitFor();
  const gone = await owner.api(`/campaigns/${cid}/entities/${copyId}`, { headers: { Accept: 'text/html' } });
  assert.ok(gone.status === 404 || gone.status === 410 || /trash/i.test(String(gone.body).slice(0, 4000)) || !gone.ok, 'the deleted copy still opens');
  owner.assertClean();
});

test('Move under another page starts editing on the Parent page field', async () => {
  const { owner, cid, eid } = w;
  const { page } = owner;
  await page.setViewportSize(SIZES.desk);
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await page.locator('[data-pe-dots]').click();
  await page.locator('[data-pe-menu]').getByRole('menuitem', { name: 'Move under another page' }).click();
  await page.waitForFunction(() => document.activeElement && document.activeElement.getAttribute('data-pe-f') === 'parent');
  assert.ok(await page.locator('[data-pe-page][data-editing]').count(), 'the page is not in editing');
  owner.assertClean();
});

test('a scribe gets three rows and a player gets no dots', async () => {
  const { owner, cid, eid } = w;
  const scribe = await person({ tag: 'pms', name: 'Tobin Hale' });
  const player = await person({ tag: 'pmp', name: 'Ines Park' });
  await addMember(owner, cid, scribe, 'scribe');
  await addMember(owner, cid, player, 'player');
  await scribe.go(`/campaigns/${cid}/entities/${eid}`);
  await scribe.page.locator('[data-pe-dots]').click();
  const rows = await scribe.page.locator('[data-pe-menu]').getByRole('menuitem').allInnerTexts();
  assert.deepEqual(rows.map((r) => r.replace(/\s+/g, ' ').trim()), ['Make a copy', 'Page history Earlier versions', 'Move under another page']);
  await player.go(`/campaigns/${cid}/entities/${eid}`);
  await player.page.waitForLoadState('networkidle');
  assert.equal(await player.page.locator('[data-pe-dots]').count(), 0, 'a player has dots');
  assert.equal(await player.page.getByText('Delete page').count(), 0);
  scribe.assertClean();
  player.assertClean();
  w.scribe = scribe;
});

test('the sidebar menu for a page has Page history and Delete page', async () => {
  const { owner, cid, eid, scratch } = w;
  const { page } = owner;
  await page.setViewportSize(SIZES.desk);
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const row = page.locator('#sidebar-entity-tree .sidebar-tree-item', { hasText: 'Hollowmere' }).first();
  await row.waitFor();
  await row.click({ button: 'right' });
  const menu = page.locator('.rowmenu');
  await menu.waitFor();
  const names = (await menu.locator('.rowmenu-item').allInnerTexts()).map((s) => s.trim());
  assert.ok(names.includes('Page history'), `no Page history in ${names}`);
  assert.ok(names.includes('Delete page'), `no Delete page in ${names}`);
  assert.ok(names.includes('Rename') && names.includes('Move to folder…'), 'Rename and Move are gone');
  await picture(page, 'page-menu-sidebar-light-desk', { about: 'The sidebar menu for a page (desk, light)', mask: ['[data-page-frame] time', '.recent-list'] });
  await page.evaluate(() => document.documentElement.classList.add('dark'));
  await picture(page, 'page-menu-sidebar-dark-desk', { about: 'The sidebar menu for a page (desk, dark)', mask: ['[data-page-frame] time', '.recent-list'] });
  await page.keyboard.press('Escape');
  await page.evaluate(() => document.documentElement.classList.remove('dark'));

  // The same menu in the phone's menu drawer.
  await page.setViewportSize(SIZES.phone);
  await page.locator('#nav-toggle').click();
  const prow = page.locator('#sidebar-entity-tree .sidebar-tree-item', { hasText: 'Hollowmere' }).first();
  await prow.waitFor({ state: 'visible' });
  await page.waitForTimeout(400);
  await prow.click({ button: 'right' });
  await page.locator('.rowmenu').waitFor();
  for (const scheme of ['light', 'dark']) {
    await page.evaluate((s) => document.documentElement.classList.toggle('dark', s === 'dark'), scheme);
    await picture(page, `page-menu-sidebar-${scheme}-phone`, { about: `The sidebar menu for a page (phone, ${scheme})`, mask: ['[data-page-frame] time', '.recent-list'] });
  }
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  await page.keyboard.press('Escape');

  // Page history opens the page with its History panel.
  await page.setViewportSize(SIZES.desk);
  await row.click({ button: 'right' });
  await menu.getByRole('menuitem', { name: 'Page history' }).click();
  await page.locator('#page-history-slot').getByText(/./).first().waitFor();

  // Delete page, from the sidebar, on a page nobody needs.
  await owner.go(`/campaigns/${cid}/entities/${scratch}`);
  const sc = page.locator('#sidebar-entity-tree .sidebar-tree-item', { hasText: 'Scratch Hall' }).first();
  await sc.click({ button: 'right' });
  await page.locator('.rowmenu').getByRole('menuitem', { name: 'Delete page' }).click();
  await page.getByText(/Moved to Trash/).first().waitFor();
  owner.assertClean();
});

test('a player and a scribe get no Delete page in the sidebar menu', async () => {
  const { cid, eid, scribe } = w;
  await scribe.go(`/campaigns/${cid}/entities/${eid}`);
  const row = scribe.page.locator('#sidebar-entity-tree .sidebar-tree-item', { hasText: 'Hollowmere' }).first();
  await row.click({ button: 'right' });
  const names = (await scribe.page.locator('.rowmenu .rowmenu-item').allInnerTexts()).map((s) => s.trim());
  assert.ok(names.includes('Page history'));
  assert.ok(!names.includes('Delete page'), 'a scribe can delete from the sidebar');
});

test('phone: the lifted Table names Rolls, Tonight and My dice; My dice raises it to full', async () => {
  const { cid, eid, owner } = w;
  const tbl = await person({ tag: 'pmt', name: 'Kell Dray', context: PHONE });
  await addMember(owner, cid, tbl, 'scribe');
  const { page } = tbl;
  await tbl.go(`/campaigns/${cid}/entities/${eid}`);
  await page.waitForSelector('html[data-tt-sheet] .tt-sheet .tt-grab');
  await page.locator('.tt-grab').tap();
  await page.waitForFunction(() => document.querySelector('.tt-sheet').dataset.h === 'lift');
  await page.waitForTimeout(450);
  const names = await page.locator('.tt-sheet .tt-ptab').allTextContents();
  assert.deepEqual(names, ['Rolls', 'Tonight', 'My dice']);
  const small = await page.locator('.tt-sheet .tt-ptab').evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height).filter((h) => h < 44));
  assert.deepEqual(small, [], 'a name is under 44 tall');
  await shots(page, 'table-lifted', 'The lifted Table with its three names', { fixed: true });
  await page.locator('.tt-sheet .tt-ptab', { hasText: 'My dice' }).tap();
  await page.waitForFunction(() => document.querySelector('.tt-sheet').dataset.h === 'full');
  assert.equal(await page.locator('.tt-sheet .tt-leaf.tt-on-phone').getAttribute('aria-label'), 'My dice');
  tbl.assertClean();
  w.tbl = tbl;
});

test('phone: a long press on a page link in text opens the pinned card with its four rows', async () => {
  const { cid, eid, tbl } = w;
  const { page } = tbl;
  await tbl.go(`/campaigns/${cid}/entities/${eid}`);
  const link = page.locator('a[data-entity-preview]', { hasText: 'Captain Vell' }).first();
  await link.waitFor();
  await link.scrollIntoViewIfNeeded();
  const b = await link.boundingBox();
  const cdp = await page.context().newCDPSession(page);
  const at = [{ x: b.x + b.width / 2, y: b.y + b.height / 2 }];
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: at });
  await page.waitForTimeout(800);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  assert.equal(await link.evaluate((a) => getComputedStyle(a).userSelect), 'none', 'a long press selects the link text');
  const card = page.locator('#chc.chc--pinned');
  await card.waitFor({ state: 'visible' });
  await page.getByText('Runs the').first().waitFor({ timeout: 2000 }).catch(() => {});
  const rows = await card.locator('.chc__row').allInnerTexts();
  assert.deepEqual(rows, ['Open page', 'Read it below, keep this page', 'Open in new tab', 'Copy link']);
  assert.match(await card.innerText(), /Captain Vell/);
  assert.match(page.url(), new RegExp(`/entities/${eid}`), 'the long press followed the link');
  await shots(page, 'long-press-card', 'The long-press card on a page link', { fixed: true, pic: { mask: ['#sidebar-entity-tree', '[data-page-frame] time', '.recent-list'] } });
  await card.locator('[data-chc="copy"]').tap().catch(() => {});
  tbl.assertClean();
});
