// dice_own_look.spec.mjs: a player's dice go with them, walked as the people
// at a table do. The Director sets the campaign's die, lets players pick and
// puts a short list of sets on offer. A player picks a set on offer with
// their account; a set off the offer and every campaign setting are refused
// them. The player's roll reaches the Director's socket, the reply and the
// log carrying the player's dice, and the Director's Table names them. Taking
// the set off the offer throws the next roll in the campaign's dice, while the
// player's pick is kept for when it comes back.
//
// Set LIVE_SHOTS to a folder to also save pictures there (at twice the size).
//
// RUN: tools/live-checks.sh test/live/dice_own_look.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { person, newCampaign, addMember, done } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';
const JSON_HEADERS = { 'Content-Type': 'application/json' };
const j = (who, url, method, body) => who.api(url, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: JSON_HEADERS });
const sharp = { context: { deviceScaleFactor: 2, colorScheme: 'dark' } };

async function shot(page, name, locator) {
  if (!SHOTS) return;
  mkdirSync(SHOTS, { recursive: true });
  await (locator || page).screenshot({ path: `${SHOTS}/${name}.png`, animations: 'disabled' });
}

test('a player\'s dice go with them, inside what the Director offers', async () => {
  const owner = await person({ tag: 'dice-dm', name: 'Dee Director', ...sharp });
  const mira = await person({ tag: 'dice-mira', name: 'Mira', ...sharp });
  const cid = await newCampaign(owner, 'The Sunken Gate');
  await addMember(owner, cid, mira);
  const base = `/campaigns/${cid}/rolls`;

  // The Director: Neon for everyone, players may pick from three sets.
  let res = await j(owner, base + '/settings', 'PUT', { look: 'rune', diceSet: 'neon', playersPick: true, diceOffered: ['neon', 'clockwork', 'oak'] });
  assert.equal(res.status, 200, JSON.stringify(res.body));

  // A player never sets the campaign's dice, the offer or whether players pick.
  await mira.go(`/campaigns/${cid}`);
  for (const body of [{ diceSet: 'clockwork' }, { diceOffered: [] }, { playersPick: false }]) {
    res = await j(mira, base + '/settings', 'PUT', body);
    assert.equal(res.status, 403, `a player changed ${JSON.stringify(body)}`);
  }

  // Their own pick is checked against the offer and kept with their account.
  res = await j(mira, base + '/my-dice', 'PUT', { set: 'bone', colour: '', critSucc: '', critFail: '' });
  assert.equal(res.status, 400, 'a set off the offer was taken');
  res = await j(mira, base + '/my-dice', 'PUT', { set: 'clockwork', colour: '', critSucc: 'phoenix', critFail: '' });
  assert.equal(res.status, 200, JSON.stringify(res.body));
  assert.equal(res.body.effective.set, 'clockwork');

  // The Director's page listens on the socket while Mira rolls.
  await owner.go(`/campaigns/${cid}`);
  const frames = [];
  owner.page.on('websocket', (ws) => ws.on('framereceived', (f) => { try { const m = JSON.parse(f.payload); if (m.type === 'roll.made') frames.push(m.payload); } catch { /* not JSON */ } }));
  await owner.page.reload({ waitUntil: 'networkidle' });
  await owner.page.waitForTimeout(500);

  res = await j(mira, base, 'POST', { formula: '2d10+2', label: 'Power roll · Might', mode: 'public' });
  assert.equal(res.status, 201, JSON.stringify(res.body));
  assert.deepEqual({ set: res.body.rollerDice.set, succ: res.body.rollerDice.critSucc, own: res.body.rollerDice.own }, { set: 'clockwork', succ: 'phoenix', own: true });
  const rollId = res.body.id;
  for (let i = 0; i < 50 && !frames.some((f) => f.id === rollId); i++) await owner.page.waitForTimeout(100);
  const live = frames.find((f) => f.id === rollId);
  assert.ok(live, 'the roll reached the Director\'s socket');
  assert.equal(live.rollerDice.set, 'clockwork', 'the live roll carries Mira\'s dice');
  const log = await j(owner, base);
  assert.equal(log.body.rolls.at(-1).rollerDice.set, 'clockwork', 'the log carries Mira\'s dice');

  // The Director's Table names whose dice the roll was thrown in.
  await owner.page.click('[data-tt-tab]');
  const row = owner.page.locator(`[data-tt-row="${rollId}"]`).first();
  await row.waitFor();
  await row.click();
  await owner.page.waitForFunction((id) => /Mira’s Clockwork/.test((document.getElementById('tt-det-' + id) || {}).textContent || ''), rollId, { timeout: 20000 });
  await shot(owner.page, 'director-sees-miras-dice', owner.page.locator('#tt-drawer'));

  // The Director's side: the Players section of Customize › Dice.
  await owner.go(`/campaigns/${cid}/customize/x/dice`);
  await owner.page.locator('[data-dcz-picks]').waitFor();
  const players = owner.page.locator('section[aria-labelledby="dcz-h-players"]');
  await players.scrollIntoViewIfNeeded();
  await shot(owner.page, 'owner-players-and-sets-on-offer', players);

  // Mira's picker in the Table's My dice leaf, on Rune 3D.
  await mira.go(`/campaigns/${cid}`);
  await mira.page.click('[data-tt-tab]');
  await mira.page.locator('[data-tt-look="rune"]').first().click();
  await mira.page.locator('[data-tt-dpick="set"]').waitFor();
  const sets = await mira.page.$$eval('[data-tt-dpick="set"] option', (os) => os.map((o) => o.value));
  assert.deepEqual(sets.filter(Boolean).sort(), ['clockwork', 'neon', 'oak'], 'the picker lists only the sets on offer');
  assert.equal(await mira.page.locator('[data-tt-dpick="set"]').inputValue(), 'clockwork', 'the pick came back from the account');
  await shot(mira.page, 'player-picks-own-dice', mira.page.locator('.tt-picks'));

  // Off the offer: the next roll is thrown in Neon, and the pick is kept.
  res = await j(owner, base + '/settings', 'PUT', { diceOffered: ['neon', 'oak'] });
  assert.equal(res.status, 200);
  res = await j(mira, base, 'POST', { formula: 'd20', mode: 'public' });
  assert.equal(res.body.rollerDice.set, 'neon', 'a set off the offer is thrown as the campaign\'s');
  assert.equal(res.body.rollerDice.critSucc, 'phoenix', 'the crit Mira picked still counts');
  res = await j(mira, base + '/my-dice', 'GET');
  assert.equal(res.body.pick.set, 'clockwork', 'the pick is kept for when the set comes back');

  owner.assertClean();
  mira.assertClean();
});
