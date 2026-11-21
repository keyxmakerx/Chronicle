// first_hour.spec.mjs — what a brand-new campaign starts with, and the owner's
// setup line counting down as steps are really done. Stands in for the
// operator checks in #2439 and #2440.
//
// A new player's "Read first" step waits on the First steps card (#2256), so
// nothing here checks it.
//
// RUN: tools/live-checks.sh test/live/first_hour.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, newPage, done, picture } from './_live.mjs';

after(done);

const ON = [
  'calendar', 'sessions', 'notes', 'jot-notes', 'maps', 'timeline',
  'media-gallery', 'npcs', 'player-character-claiming', 'attributes', 'player-notes',
];
const OFF = ['armory', 'crafting', 'transformations', 'sync-api'];

test('A new campaign starts with the usual eleven features on', async () => {
  const owner = await person({ tag: 'fho', name: 'Mara Voss' });
  const cid = await newCampaign(owner, 'Saltreach Features');
  const list = (await owner.api(`/campaigns/${cid}/addons`)).body;
  const state = Object.fromEntries(list.filter((a) => a.addon_category !== 'system').map((a) => [a.addon_slug, a.enabled]));
  for (const slug of ON) assert.equal(state[slug], true, `${slug} should start on`);
  for (const slug of OFF) assert.equal(state[slug], false, `${slug} should start off`);
  assert.equal(Object.values(state).filter(Boolean).length, 11, 'eleven features should be on');
  owner.assertClean();
});

test('A new page shows no empty controls at rest and quiet words while editing; the setup line counts down', async () => {
  const owner = await person({ tag: 'fhs', name: 'Mara Voss', context: { viewport: { width: 1280, height: 900 } } });
  const player = await person({ tag: 'fhp', name: 'Rin Noor' });
  const cid = await newCampaign(owner, 'Saltreach Setup');
  const { page } = owner;

  // The setup line: 0 of 4, the next step named, the four rows behind "All four".
  await owner.go(`/campaigns/${cid}`);
  const line = page.locator('[data-block="setup-steps"]');
  await line.waitFor();
  assert.match(await line.innerText(), /Getting your table ready\s+0 of 4 done\s+Next: Choose your game/);
  assert.equal(await line.locator('.su-row.is-done').count(), 0, 'no step starts done');
  assert.equal(await page.getByText('Get your table ready').count(), 0, 'the old text note is still drawn');
  await picture(page, 'first-hour-setup-line-0-of-4', { check: '#2440', about: 'Owner home with the setup line at 0 of 4', mask: ['.su-next'] });
  await line.getByRole('button', { name: 'All four' }).click();
  await page.locator('.su[data-open="true"] .su-row').first().waitFor();
  assert.equal(await line.locator('.su-row').count(), 4);

  // Passing on "no game system" ticks it where it was pressed.
  await line.getByRole('button', { name: 'No game system' }).click();
  await page.locator('[data-block="setup-steps"] .su-count', { hasText: '1 of 4 done' }).waitFor();
  assert.match(await page.locator('[data-block="setup-steps"]').innerText(), /No game system/);

  // An invitation sent: counted on the next look at the home.
  await addMember(owner, cid, player, 'player');
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-block="setup-steps"] .su-count', { hasText: '2 of 4 done' }).waitFor();
  assert.match(await page.locator('[data-block="setup-steps"]').innerText(), /Next: Pick a look/);

  // A new page: nothing to add at rest, quiet words in the editor.
  const pid = await newPage(owner, cid, 'Location', 'Gull Rock Light');
  await owner.go(`/campaigns/${cid}/entities/${pid}`);
  await page.locator('[data-pe-edit]').waitFor();
  await page.locator('[data-widget="tag-picker"][data-tp-empty], [data-widget="tag-picker"] .tp-chip').first().waitFor({ state: 'attached' });
  for (const sel of ['.tp-add-btn', '.rel-add-btn', '.pl-add']) {
    assert.equal(await page.locator(sel).filter({ visible: true }).count(), 0, `${sel} shows at rest on an empty page`);
  }
  await picture(page, 'first-hour-new-page-at-rest', { check: '#2439', about: 'A new page at rest: no empty add controls', mask: ['.pe-hint'] });
  await page.locator('[data-pe-edit]').click();
  for (const word of ['Add a tag', 'Add a relation', 'Add a place']) {
    await page.getByRole('button', { name: word }).first().waitFor({ state: 'visible' });
  }
  await picture(page, 'first-hour-new-page-editing', { check: '#2439', about: 'The same page while editing: the quiet words', mask: ['.pe-hint'] });

  // Keep the look and write the page: the line leaves when all four are done.
  await owner.go(`/campaigns/${cid}`);
  const again = page.locator('[data-block="setup-steps"]');
  await again.waitFor();
  assert.match(await again.locator('.su-count').innerText(), /3 of 4 done/);
  await again.getByRole('button', { name: 'All four' }).click();
  await again.getByRole('button', { name: 'Keep this one' }).click();
  await page.locator('[data-block="setup-steps"]').waitFor({ state: 'detached' }).catch(() => {});
  await owner.go(`/campaigns/${cid}`);
  assert.equal(await page.locator('[data-block="setup-steps"]').count(), 0, 'the line should leave when all four are done');

  // A player never sees it.
  await player.go(`/campaigns/${cid}`);
  assert.equal(await player.page.locator('[data-block="setup-steps"]').count(), 0);
  owner.assertClean();
  player.assertClean();
});
