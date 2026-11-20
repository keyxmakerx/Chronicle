// trackers_chosen_players.spec.mjs — a Director shows a tracker to chosen
// players and lets one of them change it; another player never sees it.
// Covers: the "Who sees this" panel, a player's plus with a note, the hidden
// player's page JSON, the Director's log naming the writer and striking it,
// and a player moved back to "May look" losing the controls and the route.
//
// Set TRACKER_SHOTS to a folder to also save the review screenshots there.
//
// RUN: tools/live-checks.sh test/live/trackers_chosen_players.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { person, newCampaign, addMember, enableAddons, newPage, done, BASE } from './_live.mjs';

after(done);

const SHOTS = process.env.TRACKER_SHOTS || '';
if (SHOTS) mkdirSync(SHOTS, { recursive: true });
const HI = { deviceScaleFactor: 2 };
const shot = async (page, name, locator) => {
  if (!SHOTS) return;
  await page.waitForTimeout(250);
  await (locator || page).screenshot({ path: `${SHOTS}/${name}.png`, ...(locator ? {} : { fullPage: false }) });
};

const block = (page) => page.locator('[data-widget="trackers"]');
const row = (page, name) => block(page).locator('section.tk-row', { has: page.locator('.tk-name', { hasText: name }) });

test('chosen players: see, change with a note, hidden from others, log names the writer', async () => {
  const owner = await person({ tag: 'cpo', name: 'Director Dee', context: HI });
  const tam = await person({ tag: 'cpt', name: 'Tam' });
  const ilse = await person({ tag: 'cpi', name: 'Ilse' });
  const cid = await newCampaign(owner, 'Chosen players check');
  await addMember(owner, cid, tam, 'player');
  await addMember(owner, cid, ilse, 'player');
  await enableAddons(owner, cid, '*');
  const eid = await newPage(owner, cid, 'Note', 'Hunger ledger');
  const typeId = await owner.page.locator('#sidebar-entity-tree').getAttribute('data-entity-type-id');
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: [{ id: 'b0', type: 'title' }, { id: 'b1', type: 'trackers' }] }] }] };
  assert.ok((await owner.api(`/campaigns/${cid}/entity-types/${typeId}/layout`, { method: 'PUT', body: { layout } })).ok);
  assert.ok((await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: { entity_ids: [eid], private: false } })).ok);

  // Director makes the tracker and opens the panel.
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const ob = block(owner.page);
  await ob.getByRole('button', { name: 'Add a tracker' }).click();
  if (await ob.getByRole('tab', { name: 'Make a new one' }).count()) await ob.getByRole('tab', { name: 'Make a new one' }).click();
  await ob.getByLabel('Name', { exact: true }).fill('Vampiric hunger');
  await ob.getByLabel('Lowest').fill('0');
  await ob.getByLabel('Highest').fill('10');
  await ob.getByRole('button', { name: 'Make and add it' }).click();
  const dRow = row(owner.page, 'Vampiric hunger');
  await dRow.waitFor();
  await dRow.getByRole('button', { name: 'Director only' }).click();
  await dRow.getByLabel('Chosen players').check();
  await dRow.getByText('No players chosen').waitFor();
  await dRow.locator('label.tk-radio', { hasText: 'Tam' }).locator('input').check();
  await dRow.getByLabel('What Tam may do').selectOption('change');
  await dRow.getByRole('button', { name: 'Only Tam' }).waitFor();
  assert.equal(await dRow.getAttribute('data-dm-only'), '', 'the amber edge stays for the Director');
  await shot(owner.page, 'director-panel-light', dRow);

  // Tam sees it, may press plus with a note.
  await tam.go(`/campaigns/${cid}/entities/${eid}`);
  const tRow = row(tam.page, 'Vampiric hunger');
  await tRow.waitFor();
  await tRow.getByText('Only you and the Director').waitFor();
  assert.equal(await tRow.getAttribute('data-dm-only'), null, 'no Director-only edge on a player copy');
  assert.equal(await tRow.getByRole('button', { name: 'Who sees this' }).count(), 0);
  await tRow.getByLabel('Why, for the log').fill('Fed on the stag');
  await shot(tam.page, 'tam-view-light', block(tam.page));
  await tRow.getByRole('button', { name: 'Add one' }).click();
  await tRow.locator('.tk-val', { hasText: '1' }).waitFor();
  await tRow.getByRole('button', { name: /^Log/ }).click();
  await tRow.locator('li.tk-line', { hasText: 'Fed on the stag' }).waitFor();
  assert.match(await tRow.locator('li.tk-line').first().innerText(), /You/);
  assert.equal(await tRow.getByRole('button', { name: 'Strike this line' }).count(), 0);
  tam.assertClean();

  // Ilse sees nothing and the JSON does not carry it.
  await ilse.go(`/campaigns/${cid}/entities/${eid}`);
  // A player with nothing to see gets an empty, hidden block.
  await ilse.page.waitForTimeout(500);
  assert.equal(await block(ilse.page).isVisible(), false, 'no block for a player who sees no tracker');
  assert.equal(await row(ilse.page, 'Vampiric hunger').count(), 0);
  const json = await ilse.api(`/campaigns/${cid}/trackers/page/${eid}`);
  assert.equal(json.status, 200);
  assert.doesNotMatch(JSON.stringify(json.body), /Vampiric|hunger/i);
  const pages = await ilse.api(`/campaigns/${cid}/trackers/pages`);
  assert.doesNotMatch(JSON.stringify(pages.body), /Vampiric|hunger/i);
  const trackerId = (await owner.api(`/campaigns/${cid}/trackers/page/${eid}`)).body.trackers[0].id;
  assert.equal((await ilse.api(`/campaigns/${cid}/trackers/${trackerId}`, { method: 'PATCH', body: { name: 'x' } })).status, 404, 'a hidden tracker is 404');
  const forged = await ilse.api(`/campaigns/${cid}/trackers/page/${eid}/log`, { method: 'POST', body: { trackerId, op: 'add', amount: 1, note: '' } });
  assert.equal(forged.status, 404, 'a hidden tracker cannot be logged to');

  // Director: the line names Tam; strike it and the value goes back.
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await dRow.getByRole('button', { name: /^Log/ }).click();
  const line = dRow.locator('li.tk-line', { hasText: 'Tam' });
  await line.waitFor();
  assert.match(await line.innerText(), /Fed on the stag/);
  await line.getByRole('button', { name: 'Strike this line' }).click();
  await dRow.locator('.tk-val', { hasText: '0' }).waitFor();

  // Back to "May look": controls vanish, the route answers 403.
  await dRow.getByRole('button', { name: 'Only Tam' }).click();
  await dRow.getByLabel('What Tam may do').selectOption('look');
  await dRow.getByRole('button', { name: 'Only Tam' }).waitFor();
  await tam.go(`/campaigns/${cid}/entities/${eid}`);
  await tRow.waitFor();
  assert.equal(await tRow.getByRole('button', { name: /Add one|Take one off/ }).count(), 0);
  assert.equal(await tRow.getByLabel('Why, for the log').count(), 0);
  const denied = await tam.api(`/campaigns/${cid}/trackers/page/${eid}/log`, { method: 'POST', body: { trackerId, op: 'add', amount: 1, note: '' } });
  assert.equal(denied.status, 403);
  owner.assertClean();
  ilse.assertClean();
  await shot(tam.page, 'tam-view-look-only-light', block(tam.page));

  // Review pictures: Tam (change allowed again) in dark and on a phone.
  await dRow.getByLabel('What Tam may do').selectOption('change');
  await dRow.getByRole('button', { name: 'Only Tam' }).waitFor();
  if (SHOTS) {
    const email = tam.email;
    for (const [name, ctx, scheme] of [['tam-view-dark', { ...HI, colorScheme: 'dark' }], ['tam-view-phone-390', { ...HI, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true }]]) {
      const t2 = await person({ email, name: 'Tam', context: ctx });
      await t2.go(`/campaigns/${cid}/entities/${eid}`);
      const r2 = row(t2.page, 'Vampiric hunger');
      await r2.waitFor();
      await shot(t2.page, name, name.includes('phone') ? undefined : block(t2.page));
      await t2.ctx.close();
    }
  }
  assert.ok(BASE);
});
