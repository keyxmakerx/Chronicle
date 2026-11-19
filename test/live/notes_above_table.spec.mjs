// notes_above_table.spec.mjs — on a touch screen the Table rests as a line on
// the window's bottom edge, over whatever is under it. The Jot notes sheet
// keeps its last row, New jot on this page, above that line: on a phone, and
// on a tablet held upright with the sheet floating or pinned to a side.
//
// RUN: tools/live-checks.sh test/live/notes_above_table.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, enableAddons, done } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };
const TABLET = { viewport: { width: 768, height: 1024 }, isMobile: true, hasTouch: true };

// Where the open sheet and its New jot button sit against the Table's line,
// and whether a finger on the button's lower edge reaches the button.
const against = (page) => page.evaluate(() => {
  const sheet = document.querySelector('.notes-panel').getBoundingClientRect();
  const line = document.querySelector('.tt-sheet .tt-grab').getBoundingClientRect();
  const add = document.querySelector('.notes-panel .jot-add-btn');
  const box = add.getBoundingClientRect();
  const hit = document.elementFromPoint(box.left + box.width / 2, box.bottom - 2);
  return { sheetBottom: Math.round(sheet.bottom), line: Math.round(line.top), addBottom: Math.round(box.bottom), reached: !!hit && (hit === add || add.contains(hit)) };
});

async function openNotes(context, name) {
  const owner = await person({ tag: 'nat', name: 'Mara Vell', context });
  const cid = await newCampaign(owner, name);
  await enableAddons(owner, cid, ['notes', 'jot-notes']);
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await page.waitForSelector('html[data-tt-sheet] .tt-sheet .tt-grab');
  await page.locator('.jot-tab').tap();
  await page.waitForSelector('.notes-panel:not(.notes-panel-hidden) .jot-add-btn');
  await page.waitForTimeout(400);
  return { owner, page, cid };
}

// Tapping New jot makes the jot at once; the server's answer is the proof
// that the tap reached the button.
async function tapNewJot(page, cid) {
  const [res] = await Promise.all([
    page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === `/campaigns/${cid}/notes`),
    page.locator('.notes-panel .jot-add-btn').tap(),
  ]);
  assert.ok(res.ok(), `making the jot answered ${res.status()}`);
}

test('phone: New jot sits above the Table\'s line and a tap makes a jot', async () => {
  const { owner, page, cid } = await openNotes(PHONE, 'Notes Above Table');
  const at = await against(page);
  assert.ok(at.addBottom <= at.line, `New jot ends at ${at.addBottom}, under the Table's line at ${at.line}`);
  assert.ok(at.reached, 'the Table\'s line takes the tap meant for New jot');
  await tapNewJot(page, cid);
  owner.assertClean();
});

test('upright tablet: the floating sheet ends above the Table\'s line', async () => {
  const { owner, page } = await openNotes(TABLET, 'Notes Above Table Float');
  const at = await against(page);
  assert.ok(at.sheetBottom <= at.line, `the sheet ends at ${at.sheetBottom}, under the Table's line at ${at.line}`);
  assert.ok(at.reached);

  // Pushed down by the keyboard as far as it goes, it still stops short.
  await page.locator('.notes-header').focus();
  for (let i = 0; i < 8; i++) await page.keyboard.press('Shift+ArrowDown');
  const low = await against(page);
  assert.ok(low.sheetBottom <= low.line, `pushed down, the sheet ends at ${low.sheetBottom}, under the line at ${low.line}`);
  owner.assertClean();
});

test('upright tablet: pinned to a side, the sheet ends above the line and New jot works', async () => {
  const { owner, page, cid } = await openNotes(TABLET, 'Notes Above Table Pinned');
  await page.locator('.notes-dock-btn[data-dock="r"]').tap();
  await page.waitForSelector('.notes-panel.notes-panel-docked');
  await page.waitForTimeout(400);
  const at = await against(page);
  assert.ok(at.sheetBottom <= at.line, `the pinned sheet ends at ${at.sheetBottom}, under the Table's line at ${at.line}`);
  assert.ok(at.reached, 'the Table\'s line takes the tap meant for New jot');
  await tapNewJot(page, cid);
  owner.assertClean();
});
