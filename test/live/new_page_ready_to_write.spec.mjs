// new_page_ready_to_write.spec.mjs — a page made a moment ago opens ready to
// write: its text is open with the cursor in it, as if Edit had been pressed,
// and opening the page any other time finds it at rest.
//
// RUN: tools/live-checks.sh test/live/new_page_ready_to_write.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done } from './_live.mjs';

after(done);

const editing = (page) => page.evaluate(() => {
  const text = document.querySelector('[data-widget="editor"]');
  return !!text && text.classList.contains('chronicle-editor--editing');
});
const words = (page) => page.locator('[data-widget="editor"] .ProseMirror').innerText().then((t) => t.trim());

test('Create page opens the new page with the cursor in its text', async () => {
  const owner = await person({ tag: 'npw', name: 'Mara Vell' });
  const { page } = owner;
  const cid = await newCampaign(owner, 'Ready To Write');
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');

  await page.waitForFunction(() => document.querySelector('[data-widget="editor"]')?.classList.contains('chronicle-editor--editing'));
  assert.equal(await page.locator('[data-widget="editor"]').getByRole('button', { name: 'Done' }).count(), 1, 'the text shows Done, as after pressing Edit');

  // No click first: the cursor is already in the text.
  await page.keyboard.type('A watchtower on the north mole.');
  assert.equal(await words(page), 'A watchtower on the north mole.');

  // The address is the page's own again, so a reload or a copied link opens
  // the page at rest.
  assert.equal(new URL(page.url()).pathname, `/campaigns/${cid}/entities/${eid}`);
  assert.equal(new URL(page.url()).search, '');

  // Done closes it as usual, and the words are kept.
  await Promise.all([
    page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/entities/${eid}/entry`) && r.ok()),
    page.locator('[data-widget="editor"]').getByRole('button', { name: 'Done' }).click(),
  ]);
  assert.equal(await editing(page), false);

  await page.reload({ waitUntil: 'networkidle' });
  assert.equal(await editing(page), false, 'a page opened again is at rest');
  assert.equal(await words(page), 'A watchtower on the north mole.');
  owner.assertClean();
});
