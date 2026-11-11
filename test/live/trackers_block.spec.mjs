// trackers_block.spec.mjs — the Trackers block on a page: add it, make a
// number tracker and a clock, step and log them, strike a log line, and see
// what a player sees. Stands in for the operator check in #1599.
//
// Covers steps 1-6. The block is placed through the category layout API
// rather than the Customizer's drag-and-drop. Left for a person: step 7
// (the Annals style kit's accent colour on the clock and bands; the
// pictures show the default look only), and the in-game date on log lines,
// which needs a calendar with a current date.
//
// RUN: tools/live-checks.sh test/live/trackers_block.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, newPage, done, picture } from './_live.mjs';

after(done);

let worldPromise;
function world() {
  worldPromise = worldPromise || (async () => {
    const owner = await person({ tag: 'trk', name: 'Tracker Owner' });
    const player = await person({ tag: 'trkp', name: 'Tracker Player' });
    const cid = await newCampaign(owner, 'Trackers check');
    await addMember(owner, cid, player, 'player');
    await enableAddons(owner, cid, '*');
    const eid = await newPage(owner, cid, 'Note', 'Ink ledger');
    const typeId = await owner.page.locator('#sidebar-entity-tree').getAttribute('data-entity-type-id');
    const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: [{ id: 'b0', type: 'title' }, { id: 'b1', type: 'trackers' }] }] }] };
    const saved = await owner.api(`/campaigns/${cid}/entity-types/${typeId}/layout`, { method: 'PUT', body: { layout } });
    assert.ok(saved.ok, 'layout save answered ' + saved.status);
    // New notes start GM-only; a player needs to be able to open this one.
    const open = await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: { entity_ids: [eid], private: false } });
    assert.ok(open.ok, 'making the page public answered ' + open.status);
    return { owner, player, cid, eid };
  })();
  return worldPromise;
}

const block = (page) => page.locator('[data-widget="trackers"]');
const row = (page, name) => block(page).locator('section.tk-row', { has: page.locator('.tk-name', { hasText: name }) });

test('1-4. add the block, make Ink, step it to its top, read and strike the log', async () => {
  const { owner, cid, eid } = await world();
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const b = block(page);
  await b.getByText('Trackers', { exact: true }).waitFor();
  await b.getByRole('button', { name: 'Add a tracker' }).waitFor();

  // 2. A number tracker.
  await b.getByRole('button', { name: 'Add a tracker' }).click();
  const hasTabs = await b.getByRole('tab', { name: 'Make a new one' }).count();
  if (hasTabs) await b.getByRole('tab', { name: 'Make a new one' }).click();
  await b.getByLabel('Name', { exact: true }).fill('Ink');
  await b.getByLabel('Lowest').fill('0');
  await b.getByLabel('Highest').fill('3');
  await b.getByLabel('Bands').fill('3: Inked');
  await b.getByRole('button', { name: 'Make and add it' }).click();
  const ink = row(page, 'Ink');
  await ink.waitFor();
  assert.equal((await ink.locator('.tk-val').innerText()).trim(), '0');
  await ink.getByText('Director only').waitFor();
  assert.equal(await ink.getAttribute('data-dm-only'), '', 'the row should be marked Director only');
  await picture(page, 'trackers-ink-row', { check: '#1599', about: 'Ink tracker at 0, Director only', locator: ink });

  // 3. Step it three times; the plus greys out at the top and the band shows.
  const plus = ink.getByRole('button', { name: 'Add one' });
  for (let i = 1; i <= 3; i++) {
    await plus.click();
    await ink.locator('.tk-val', { hasText: String(i) }).waitFor();
  }
  assert.equal(await plus.isDisabled(), true, 'plus should grey out at the highest value');
  await ink.locator('.tk-now', { hasText: 'Inked' }).waitFor();
  await ink.getByRole('button', { name: /^Log · 3/ }).click();
  assert.equal(await ink.locator('li.tk-line:not(.tk-none)').count(), 3, 'three log lines');

  // 4. Strike the newest line: value drops to 2; bring it back: 3 again.
  await ink.locator('li.tk-line').first().getByRole('button', { name: 'Strike this line' }).click();
  await ink.locator('.tk-val', { hasText: '2' }).waitFor();
  await ink.locator('li.tk-line.struck').waitFor();
  await ink.locator('li.tk-line.struck').getByRole('button', { name: 'Bring this line back' }).click();
  await ink.locator('.tk-val', { hasText: '3' }).waitFor();
  assert.equal(await ink.locator('li.tk-line.struck').count(), 0);
  owner.assertClean();
});

test('5-6. a clock fills in wedges; shown to players it appears without the log notes or buttons, and Ink stays hidden', async () => {
  const { owner, player, cid, eid } = await world();
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const b = block(page);
  await b.getByRole('button', { name: 'Add a tracker' }).click();
  await b.getByRole('tab', { name: 'Make a new one' }).click();
  await b.getByLabel('Name', { exact: true }).fill('Doom');
  await b.getByLabel('A clock').check();
  await b.getByLabel('Segments').fill('6');
  await b.getByRole('button', { name: 'Make and add it' }).click();
  const doom = row(page, 'Doom');
  await doom.waitFor({ timeout: 5000 }).catch(async (e) => { throw new Error('Doom not made; status: ' + await b.locator('.tk-status').innerText() + '; ' + e.message); });
  const tick = doom.getByRole('button', { name: 'Tick the clock' });
  await tick.click();
  await doom.locator('.tk-seg.on').first().waitFor();
  await tick.click();
  await page.waitForFunction(() => document.querySelectorAll('.tk-row .tk-seg.on').length === 2);
  assert.equal(await doom.locator('.tk-seg').count(), 6);
  const fill = await doom.locator('.tk-seg.on').first().evaluate((el) => getComputedStyle(el).fill);
  assert.notEqual(fill, 'none', 'a filled wedge should have a colour');
  await picture(page, 'trackers-clock', { check: '#1599', about: 'Clock with two of six wedges filled', locator: doom });

  // 6. Show the clock to players only.
  await doom.getByRole('button', { name: 'More for Doom' }).click();
  await doom.getByRole('button', { name: 'Show to players' }).click();
  await doom.getByText('Players see this').waitFor();

  await player.go(`/campaigns/${cid}/entities/${eid}`);
  const pb = block(player.page);
  await row(player.page, 'Doom').waitFor({ timeout: 4000 }).catch(async (e) => { throw new Error((await player.page.locator('main').innerText()).slice(0, 400) + ' // ' + e.message); });
  assert.equal(await row(player.page, 'Ink').count(), 0, 'Ink should not show to a player');
  assert.equal(await pb.getByRole('button', { name: /Tick the clock|Add one|Take one off/ }).count(), 0, 'a player has step buttons');
  assert.equal(await pb.getByRole('button', { name: 'Add a tracker' }).count(), 0, 'a player can add a tracker');
  assert.equal(await row(player.page, 'Doom').locator('.tk-seg.on').count(), 2, 'the player should see two wedges');
  const log = row(player.page, 'Doom').getByRole('button', { name: /Log/ });
  if (await log.count()) {
    await log.click();
    const lines = await row(player.page, 'Doom').locator('li.tk-line:not(.tk-none)').allInnerTexts();
    for (const l of lines) assert.doesNotMatch(l, /started again|Why/i, 'a note reached the player');
  }
  await picture(player.page, 'trackers-player-view', { check: '#1599', about: 'Player view of the page: the clock shows, Ink does not', locator: pb });
});
