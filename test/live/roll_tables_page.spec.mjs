// roll_tables_page.spec.mjs — the Rolling tables page in Campaign settings:
// the list of every table with Find, the New table panel (Blank, Examples,
// Copy one), rename, move and remove from a row's three dots, and the DM
// team only rule. What the builder saves (when it can come up, who sees a
// day's roll) is read back from the server.
//
// Set LIVE_SHOTS to a folder to also save pictures of the page there.
//
// RUN: tools/live-checks.sh test/live/roll_tables_page.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, BASE } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';

async function fillExamples(page, name) {
  await page.getByRole('tab', { name: 'Examples' }).click();
  await page.locator('[data-name]').fill(name);
  const lines = ['Gulls circle the {place} wreck', 'A wharf eel in the nets', 'Salt crusts the shrine'];
  const briefs = ['Old sailors will not load cargo until they scatter.', 'The harbourmaster has it burned.', 'Jera scrapes it into jars.'];
  for (let i = 0; i < 3; i++) {
    await page.locator('.rtp-ex [data-n]').nth(i).fill(lines[i]);
    await page.locator('.rtp-ex [data-b]').nth(i).fill(briefs[i]);
  }
}

test('build a table from examples, keep when and who sees it, then rename, move and remove', async () => {
  const owner = await person({ tag: 'tables-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Rolling tables check');
  const page = owner.page;
  const mine = async () => (await owner.api(`/campaigns/${cid}/roll-tables`)).body.tables;

  const res = await owner.go(`/campaigns/${cid}/roll-tables/manage`);
  assert.equal(res.status(), 200);
  await page.getByRole('heading', { name: 'Rolling tables' }).waitFor();
  await page.locator('.rtp-grp', { hasText: 'Your tables' }).waitFor();
  await page.locator('.rtp-grp', { hasText: 'Happenings' }).waitFor();
  assert.ok(await page.locator('.rtp-more').count() > 0, 'long groups fold behind Show all');

  // Find narrows the list across every group.
  await page.getByLabel('Find a table').fill('omens');
  await page.locator('.rtp-nm', { hasText: /^Omens$/ }).waitFor();
  assert.equal(await page.locator('.rtp-more').count(), 0, 'a search shows every match');
  await page.getByLabel('Find a table').fill('');

  // New table, Examples: test rolls follow the lines, save keeps the conditions.
  await page.getByRole('button', { name: 'New table' }).click();
  await page.locator('[data-panel]').waitFor({ state: 'visible' });
  await fillExamples(page, 'Dockside omens');
  await page.locator('[data-tests] b').first().waitFor();
  const first = await page.locator('[data-tests] b').allTextContents();
  assert.ok(first.length >= 1 && first.every((t) => !/[{}]/.test(t) && !/problem/i.test(t)), 'test rolls are real and fill the braces: ' + first);
  await page.locator('[data-when]').selectOption('full');
  await page.getByRole('button', { name: 'Only you' }).click();
  await page.locator('[data-save]').click();
  await page.locator('.rtp-row', { hasText: 'Dockside omens' }).waitFor();
  let tables = await mine();
  assert.equal(tables.length, 1);
  assert.equal(tables[0].visibility, 'dm_only');
  assert.deepEqual(tables[0].entries[0].when, { moon: { name: 'any', phase: 'full', within: 1 } });
  assert.equal(tables[0].entries.length, 3);

  // Fewer than three examples is refused in words, and nothing is saved.
  await page.getByRole('button', { name: 'New table' }).click();
  await page.getByRole('tab', { name: 'Examples' }).click();
  await page.locator('[data-name]').fill('Too short');
  await page.locator('.rtp-ex [data-n]').first().fill('Only one');
  await page.locator('[data-save]').click();
  await page.getByText('Write three or more examples.').waitFor();
  assert.equal((await mine()).length, 1);

  // Blank: one line is enough; no conditions are saved.
  await page.getByRole('tab', { name: 'Blank' }).click();
  await page.locator('[data-name]').fill('Tavern names');
  await page.locator('.rtp-ex [data-n]').first().fill('The Rusty Anchor');
  await page.locator('[data-save]').click();
  await page.locator('.rtp-row', { hasText: 'Tavern names' }).waitFor();
  tables = await mine();
  assert.deepEqual(tables.map((t) => t.name), ['Dockside omens', 'Tavern names']);
  assert.equal(tables[1].entries[0].when, undefined);

  // Copy one keeps the lines.
  await page.getByRole('button', { name: 'New table' }).click();
  await page.getByRole('tab', { name: 'Copy one' }).click();
  await page.locator('[data-copyq]').fill('rumours');
  await page.locator('[data-copy="rumours"]').click();
  await page.locator('[data-save]').click();
  await page.locator('.rtp-row', { hasText: 'Rumours (copy)' }).waitFor();
  tables = await mine();
  assert.equal(tables.length, 3);
  assert.ok(tables[2].entries.length > 3);

  // Rename from the three dots, then move it to the top.
  const row = (n) => page.locator('.rtp-row', { hasText: n });
  await row('Tavern names').getByRole('button', { name: /More for/ }).click();
  await page.getByRole('menuitem', { name: 'Rename' }).click();
  await page.locator('[data-rename]').fill('Inn names');
  await page.locator('[data-rename]').press('Enter');
  await row('Inn names').waitFor();
  assert.deepEqual((await mine()).map((t) => t.name), ['Dockside omens', 'Inn names', 'Rumours (copy)']);
  await row('Inn names').getByRole('button', { name: /More for/ }).click();
  await page.getByRole('menuitem', { name: 'Move up' }).click();
  await page.waitForFunction(() => document.querySelector('.rtp-grp .rtp-nm')?.textContent === 'Inn names');
  assert.deepEqual((await mine()).map((t) => t.name), ['Inn names', 'Dockside omens', 'Rumours (copy)']);
  // The saved table kept its conditions through rename and move.
  assert.equal((await mine())[1].visibility, 'dm_only');
  assert.ok((await mine())[1].entries[0].when);

  // Remove asks first; keeping it changes nothing, confirming removes it.
  await row('Inn names').getByRole('button', { name: /More for/ }).click();
  await page.getByRole('menuitem', { name: 'Remove' }).click();
  await page.getByText('Remove “Inn names”?').waitFor();
  await page.getByRole('button', { name: 'Keep editing' }).click();
  assert.equal((await mine()).length, 3);
  await row('Inn names').getByRole('button', { name: /More for/ }).click();
  await page.getByRole('menuitem', { name: 'Remove' }).click();
  await page.locator('.ask-slip [data-drop]').click();
  await page.waitForFunction(() => ![...document.querySelectorAll('.rtp-nm')].some((e) => e.textContent === 'Inn names'));
  assert.deepEqual((await mine()).map((t) => t.name), ['Dockside omens', 'Rumours (copy)']);

  // A built-in table opens read-only and offers a copy.
  await page.locator('.rtp-nm', { hasText: /^Omens$/ }).click();
  await page.getByRole('button', { name: 'Copy it to your tables' }).waitFor();
  owner.assertClean();
});

test('players and scribes do not get the page, and Game lists it for the owner', async () => {
  const owner = await person({ tag: 'tables-gate-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Rolling tables gate');
  const scribe = await person({ tag: 'tables-scribe', name: 'Sam Scribe' });
  const player = await person({ tag: 'tables-player', name: 'Pia Player' });
  await addMember(owner, cid, scribe, 'scribe');
  await addMember(owner, cid, player, 'player');
  for (const who of [scribe, player]) {
    const res = await who.go(`/campaigns/${cid}/roll-tables/manage`);
    assert.equal(res.status(), 403, who.name);
    assert.equal(await who.page.getByText('New table').count(), 0, 'no builder for ' + who.name);
  }
  await owner.go(`/campaigns/${cid}/extensions`);
  const link = owner.page.locator(`a[href="/campaigns/${cid}/roll-tables/manage"]`).first();
  await link.waitFor({ state: 'attached' });
});

test('pictures of the page', { skip: !SHOTS }, async () => {
  const owner = await person({ tag: 'tables-shots', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Saltreach docks');
  const page = owner.page;
  await owner.go(`/campaigns/${cid}/roll-tables/manage`);
  await page.locator('.rtp-grp').first().waitFor();
  // Two own tables, so the rows have their dots, then the builder open on Examples.
  await owner.api(`/campaigns/${cid}/roll-tables`, { method: 'PUT', body: { tables: [
    { id: 'lastwell-trial-flaws', name: 'Lastwell trial flaws', entries: ['a', 'b', 'c', 'd'].map((n) => ({ name: 'Flaw ' + n, weight: 1 })) },
    { id: 'saltreach-names', name: 'Saltreach names', entries: Array.from({ length: 12 }, (_, i) => ({ name: 'Name ' + i, weight: 1 })) },
  ] } });
  await owner.go(`/campaigns/${cid}/roll-tables/manage`);
  await page.locator('.rtp-row', { hasText: 'Saltreach names' }).waitFor();
  await page.getByRole('button', { name: 'New table' }).click();
  await fillExamples(page, 'Dockside omens');
  await page.locator('[data-when]').selectOption('full');
  await page.locator('[data-tests] b').first().waitFor();
  for (const [view, size] of [['desktop', { width: 1100, height: 1000 }], ['phone', { width: 390, height: 844 }]]) {
    for (const scheme of ['light', 'dark']) {
      await page.setViewportSize(size);
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'reduce' });
      await page.evaluate((s) => {
        document.documentElement.classList.toggle('dark', s === 'dark');
        try { localStorage.setItem('theme', s); } catch (e) { /* none */ }
      }, scheme);
      await page.waitForTimeout(200);
      await page.screenshot({ path: `${SHOTS}/tables-${view}-${scheme}.png`, fullPage: view === 'desktop' });
    }
  }
  await page.setViewportSize({ width: 1100, height: 1000 });
  await page.getByRole('button', { name: 'Close' }).click();
  await page.locator('.ask-slip [data-drop]').click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => document.documentElement.classList.remove('dark'));
  await page.evaluate(() => { window.scrollTo(0, 0); document.querySelectorAll('*').forEach((e) => { if (e.scrollTop) e.scrollTop = 0; }); });
  await page.waitForTimeout(250);
  await page.screenshot({ path: `${SHOTS}/tables-phone-list-light.png`, fullPage: true });
  await page.setViewportSize({ width: 1100, height: 1000 });
  await page.waitForTimeout(150);
  await page.getByRole('button', { name: /More for Saltreach names/ }).click();
  await page.waitForTimeout(150);
  await page.screenshot({ path: `${SHOTS}/tables-desktop-menu.png` });
});
