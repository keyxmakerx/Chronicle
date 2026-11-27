// homebrew_ask.spec.mjs — a house rule asks the Director before it goes on.
//
// Mira, a player the Director lets change the Luck tracker, adds one to it.
// The Director's recipe answers Luck reaching 1 with "Ask the Director: draw a
// card", then steps Luck by 2. The rule stops at the ask: Mira's "Your asks"
// page says her Director is answering, and the DM Screen's Homebrew drawer
// shows the answer card under "Waiting on you". The Director says Yes and
// changes the step to 3 ("Yes, and…"); Luck lands on 4 and Mira reads "Yes,
// with changes". A second ask Mira takes back herself, and a third the
// Director answers with a written reply.
//
// Set ASK_SHOTS to a folder to also save the review pictures there.
//
// RUN: tools/live-checks.sh test/live/homebrew_ask.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { person, newCampaign, addMember, enableAddons, newPage, done } from './_live.mjs';

after(done);

const SHOTS = process.env.ASK_SHOTS || '';
if (SHOTS) mkdirSync(SHOTS, { recursive: true });
const HI = { deviceScaleFactor: 2, viewport: { width: 1280, height: 900 } };

async function shot(page, name, locator) {
  if (!SHOTS) return;
  for (const dark of [true, false]) {
    await page.emulateMedia({ colorScheme: dark ? 'dark' : 'light' });
    await page.evaluate((d) => document.documentElement.classList.toggle('dark', d), dark);
    await page.waitForTimeout(250);
    await (locator || page).screenshot({ path: `${SHOTS}/${name}-${dark ? 'dark' : 'light'}.png` });
  }
}

async function luck(owner, cid, eid) {
  const r = await owner.api(`/campaigns/${cid}/trackers/page/${eid}`);
  return r.body.trackers.find((t) => t.name === 'Luck');
}

test('a rule asks the Director; Yes, and… changes what follows; Mira sees what came back', async () => {
  const owner = await person({ tag: 'hao', name: 'Dana', context: HI });
  const mira = await person({ tag: 'ham', name: 'Mira', context: HI });
  const cid = await newCampaign(owner, 'Fortunes');
  await addMember(owner, cid, mira, 'player');
  await enableAddons(owner, cid, '*');
  const eid = await newPage(owner, cid, 'Character', 'Mira of the Reach');
  assert.ok((await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: { entity_ids: [eid], private: false } })).ok);
  const kind = await owner.api(`/campaigns/${cid}/tracker-kinds`, { method: 'POST', body: { name: 'Luck', shape: 'count', min: 0, max: 10, visibility: 'directors' } });
  assert.equal(kind.status, 201, JSON.stringify(kind.body));
  assert.ok((await owner.api(`/campaigns/${cid}/trackers/page/${eid}`, { method: 'POST', body: { kindId: kind.body.id } })).ok);
  await mira.go(`/campaigns/${cid}`);
  const miraId = await mira.page.locator('[data-user-id]').first().getAttribute('data-user-id');
  const t0 = await luck(owner, cid, eid);
  const patch = await owner.api(`/campaigns/${cid}/trackers/${t0.id}`, { method: 'PATCH', body: { visibility: 'players', players: [{ userId: miraId, canChange: true }] } });
  assert.ok(patch.ok, JSON.stringify(patch.body));

  const recipe = await owner.api(`/campaigns/${cid}/homebrew/recipes`, { method: 'POST', body: { name: 'Lucky draw', doc: {
    parts: [{ key: 'luck', kind: 'tracker', label: 'Luck', see: 'party', data: { link: 'Luck' } }],
    rules: [{ when: { event: 'tracker.reached', part: 'luck', args: { value: 1 } }, timing: 'now',
      then: [{ action: 'ask-director', args: { ask: 'wants to draw a card' } }, { action: 'step', part: 'luck', args: { by: 2 } }] }] } } });
  assert.equal(recipe.status, 201, JSON.stringify(recipe.body));

  // Mira's move sets the rule off; it stops at the ask.
  const log = (amount) => mira.api(`/campaigns/${cid}/trackers/page/${eid}/log`, { method: 'POST', body: { trackerId: t0.id, op: 'add', amount, note: '' } });
  assert.ok((await log(1)).ok);
  assert.equal((await luck(owner, cid, eid)).value, 1, 'the step after the ask ran before an answer');
  await mira.go(`/campaigns/${cid}/homebrew/asks`);
  const card = mira.page.locator('.hba-card').first();
  await card.locator('.hba-pill').waitFor();
  assert.equal(await card.locator('.hba-title').textContent(), 'Draw a card');
  assert.match(await card.locator('.hba-pill').textContent(), /is answering$/);
  assert.equal(await card.getByRole('button', { name: 'Take back my ask' }).count(), 1);
  await shot(mira.page, 'player-waiting', mira.page.locator('main, #main-content').first());

  // A player can't answer, even over the API.
  const asksMine = await mira.api(`/campaigns/${cid}/homebrew/asks/mine`);
  const runId = asksMine.body.data[0].id;
  assert.equal((await mira.api(`/campaigns/${cid}/homebrew/runs/${runId}/answer`, { method: 'POST', body: { answer: 'yes' } })).status, 403);

  // The Director answers in the DM Screen's Homebrew drawer.
  await owner.go(`/campaigns/${cid}`);
  await owner.page.locator('[data-dms-toggle]').click();
  await owner.page.locator('.dms-dock').waitFor({ state: 'visible' });
  await owner.page.locator('.dms-rail [data-dms-open="homebrew"]').click();
  const ask = owner.page.locator('.hbd-ask').first();
  await ask.waitFor();
  assert.equal(await ask.locator('.hbd-ask-title').textContent(), 'Mira wants to draw a card');
  await ask.getByRole('button', { name: 'Yes', exact: true }).click();
  await ask.getByRole('button', { name: '+ Change how many' }).click();
  await ask.getByLabel('Step a tracker: By').fill('3');
  assert.match(await ask.locator('.hbd-send').textContent(), /^Send: yes, with changes, then step a tracker/);
  await shot(owner.page, 'director-answer-card', ask);
  await ask.locator('.hbd-send').click();
  await owner.page.locator('.hbd-status', { hasText: 'Sent: yes.' }).waitFor();
  await owner.page.locator('.hbd-ask').waitFor({ state: 'detached' });
  assert.equal((await luck(owner, cid, eid)).value, 4, 'Yes, and… did not step Luck by 3');

  await mira.go(`/campaigns/${cid}/homebrew/asks`);
  await card.locator('.hba-said').waitFor();
  assert.equal(await card.locator('.hba-pill').textContent(), 'Yes, with changes');
  assert.match(await card.locator('.hba-changes').textContent(), /By: 3 \(was 2\)/);
  await shot(mira.page, 'player-answered', mira.page.locator('main, #main-content').first());

  // A second ask Mira takes back herself; nothing more runs.
  await owner.api(`/campaigns/${cid}/trackers/page/${eid}/log`, { method: 'POST', body: { trackerId: t0.id, op: 'set', amount: 0, note: '' } });
  assert.ok((await log(1)).ok);
  await mira.go(`/campaigns/${cid}/homebrew/asks`);
  await card.getByRole('button', { name: 'Take back my ask' }).click();
  await card.locator('.hba-pill', { hasText: 'Taken back' }).waitFor();
  assert.equal((await luck(owner, cid, eid)).value, 1);

  // A third the Director answers in words.
  await owner.api(`/campaigns/${cid}/trackers/page/${eid}/log`, { method: 'POST', body: { trackerId: t0.id, op: 'set', amount: 0, note: '' } });
  assert.ok((await log(1)).ok, 'the third move');
  const waiting = await owner.api(`/campaigns/${cid}/homebrew/drawer`);
  const third = waiting.body.waiting.find((r) => r.ask);
  const other = await owner.api(`/campaigns/${cid}/homebrew/runs/${third.id}/answer`, { method: 'POST', body: { answer: 'other', reply: 'After the fight.' } });
  assert.ok(other.ok, JSON.stringify(other.body));
  await mira.go(`/campaigns/${cid}/homebrew/asks`);
  await card.locator('.hba-said-t', { hasText: 'After the fight.' }).waitFor();
  assert.equal((await luck(owner, cid, eid)).value, 1);
  owner.assertClean();
  mira.assertClean();
});
