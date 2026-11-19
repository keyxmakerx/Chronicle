// leave_campaign.spec.mjs — a player leaves a campaign from My campaigns, the
// character goes back to the table, and the owner is told once.
//
// A player with a claimed character and a private note opens My campaigns,
// the three dots on the campaign's card, Leave campaign, reads the box and
// leaves. The card goes, access ends, the owner gets one note and sees the
// character under Free to pick, and the join link still brings the player back
// with the private note intact. An owner's own campaign offers Hand over.
//
// RUN: tools/live-checks.sh test/live/leave_campaign.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, addMember, enableAddons, done, picture, BASE } from './_live.mjs';

after(done);

const PHONE = { width: 390, height: 844 };
const DESK = { width: 1280, height: 800 };

/** Every picture is taken light and dark, on a phone and a computer. */
async function everywhere(page, name, about, prepare, opts = {}) {
  for (const scheme of ['light', 'dark']) {
    for (const [size, viewport] of [['phone', PHONE], ['desk', DESK]]) {
      await page.setViewportSize(viewport);
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'reduce' });
      await prepare();
      await picture(page, `${name}-${scheme}-${size}`, { about, ...opts });
    }
  }
}

test('leaving from My campaigns frees the character, tells the owner once and can be undone with the link', async () => {
  const owner = await person({ tag: 'lvo', name: 'Mira Vale' });
  const tam = await person({ tag: 'lvp', name: 'Tam Reed' });
  const cid = await newCampaign(owner, 'Therin');
  await addMember(owner, cid, tam, 'player');
  await enableAddons(owner, cid, '*');
  const link = await owner.api(`/campaigns/${cid}/join-code`, { method: 'POST' });
  assert.equal(link.status, 200);
  const code = link.body.code;

  // Tam has a character and a private note, and runs a campaign of their own.
  const ch = await newPage(owner, cid, 'Character', 'Brannoc');
  await owner.api(`/campaigns/${cid}/entities/${ch}/permissions`, { method: 'PUT', body: { visibility: 'default' } });
  await owner.api(`/campaigns/${cid}/characters/playable`, { method: 'POST', form: { entity_id: ch, playable: '1' } });
  const claim = await tam.api(`/campaigns/${cid}/entities/${ch}/claim`, { method: 'POST' });
  assert.ok(claim.ok, 'Tam could not claim Brannoc: ' + JSON.stringify(claim.body));
  const note = await tam.api(`/campaigns/${cid}/notes`, {
    method: 'POST', body: JSON.stringify({ title: 'What Brannoc saw', content: [{ type: 'text', value: 'The bridge was already burning.' }] }),
    headers: { 'Content-Type': 'application/json' },
  });
  assert.equal(note.status, 201, 'Tam could not write a private note: ' + JSON.stringify(note.body));
  const own = await newCampaign(tam, 'The Ember Coast');

  // 1. My campaigns: the dots, the menu, the box.
  const { page } = tam;
  await tam.go('/campaigns');
  const card = page.locator(`[data-campaign-card="${cid}"]`);
  const ownCard = page.locator(`[data-campaign-card="${own}"]`);
  await card.waitFor();

  await ownCard.getByRole('button', { name: 'Actions for The Ember Coast' }).click();
  assert.ok(await ownCard.getByRole('menuitem', { name: 'Hand over…' }).isVisible(), 'an owner is not offered Hand over');
  assert.equal(await ownCard.getByRole('menuitem', { name: /Leave campaign/ }).count(), 0, 'an owner is offered Leave');
  assert.match(await ownCard.getByRole('menuitem', { name: 'Hand over…' }).getAttribute('href'), new RegExp(`/campaigns/${own}/transfer$`));
  await page.keyboard.press('Escape');

  const dots = card.getByRole('button', { name: 'Actions for Therin' });
  await dots.click();
  const menu = card.getByRole('menu');
  await menu.waitFor();
  assert.deepEqual(await menu.getByRole('menuitem').allInnerTexts(), ['Open', 'Leave campaign…']);
  assert.equal(await menu.getByRole('menuitem', { name: 'Hand over…' }).count(), 0, 'a player is offered Hand over');
  await everywhere(page, 'leave-menu', 'My campaigns with the three dots menu open on a card: Open and Leave campaign', async () => {
    await tam.go('/campaigns');
    await card.getByRole('button', { name: 'Actions for Therin' }).click();
    await card.getByRole('menu').waitFor();
  }, { hide: ['time'] });

  await menu.getByRole('menuitem', { name: 'Leave campaign…' }).click();
  const box = page.getByRole('dialog', { name: 'Leave Therin?' });
  await box.waitFor();
  const words = await box.innerText();
  for (const line of [
    'Brannoc stays with the campaign. The DM can give them to someone else.',
    'Your posts and shared notes stay, with your name on them.',
    'Your private notes and journal are put away. They come back if you rejoin.',
    'You can come back with an invite or the campaign’s join link.',
  ]) assert.ok(words.includes(line.replace('’', "'")) || words.includes(line), `the box lacks: ${line}`);
  assert.deepEqual(await box.getByRole('button').allInnerTexts(), ['Stay', 'Leave Therin']);
  await everywhere(page, 'leave-box', 'The box that asks before leaving, with what stays and what is put away', async () => {
    await tam.go('/campaigns');
    await card.getByRole('button', { name: 'Actions for Therin' }).click();
    await card.getByRole('menuitem', { name: 'Leave campaign…' }).click();
    await page.getByRole('dialog', { name: 'Leave Therin?' }).waitFor();
  }, { hide: ['time'] });

  // Stay closes it and changes nothing.
  await box.getByRole('button', { name: 'Stay' }).click();
  await box.waitFor({ state: 'hidden' });
  assert.equal((await tam.api(`/campaigns/${cid}/notes`)).status, 200, 'Stay changed Tam’s access');
  assert.equal(await card.count(), 1);

  // 2. Leave. The card goes and a toast says so.
  await page.setViewportSize(DESK);
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' });
  await tam.go('/campaigns');
  await card.getByRole('button', { name: 'Actions for Therin' }).click();
  await card.getByRole('menuitem', { name: 'Leave campaign…' }).click();
  await page.getByRole('dialog', { name: 'Leave Therin?' }).getByRole('button', { name: 'Leave Therin' }).click();
  await card.waitFor({ state: 'detached' });
  await page.getByText('You left Therin.').waitFor();
  await tam.go('/campaigns');
  assert.equal(await page.locator(`[data-campaign-card="${cid}"]`).count(), 0, 'Therin is still in Tam’s list');
  assert.equal(await page.locator(`[data-campaign-card="${own}"]`).count(), 1, 'Tam’s own campaign went too');
  // Access ended at once.
  const gone = await tam.page.goto(BASE + `/campaigns/${cid}`);
  assert.equal(gone.status(), 403, 'Tam can still open Therin');

  // 3. The owner: not removed, one note, the character free to pick.
  await owner.go(`/campaigns/${cid}/members`);
  assert.equal(await owner.page.locator('#member-list').getByText('Tam Reed').count(), 0, 'Tam is still on People');
  assert.equal(await owner.page.getByRole('heading', { name: 'Removed' }).count(), 0, 'Tam was listed as removed');
  const list = await owner.api('/notifications');
  const left = list.body.filter((n) => n.type === 'member_left');
  assert.equal(left.length, 1, 'the owner should have exactly one note');
  assert.equal(left[0].message, 'Tam Reed left Therin');
  assert.equal(left[0].detail, "Brannoc is free to pick. Tam Reed's posts and notes stay.");
  assert.equal(left[0].action, 'Give Brannoc to someone');
  assert.ok(left[0].link.includes(`/campaigns/${cid}/characters`));

  const { page: op } = owner;
  const openBell = async () => {
    await owner.go(`/campaigns/${cid}`);
    // On a phone the bell sits behind the Tools tab; its shelf entry presses it.
    if (!(await op.locator('[data-notif-toggle]').isVisible())) {
      await op.locator('[data-tb-tab]').click();
      await op.locator('[data-tb-for="bell"]').click();
    } else {
      await op.locator('[data-notif-toggle]').click();
    }
    await op.getByText('Tam Reed left Therin').waitFor();
  };
  await everywhere(op, 'leave-owner-note', 'The owner’s bell: Tam left Therin, Brannoc is free to pick, Give Brannoc to someone', openBell);

  await op.setViewportSize(DESK);
  await openBell();
  await op.getByText('Give Brannoc to someone').click();
  await op.waitForURL(/\/characters/);
  const freeList = op.locator('.pw-freelist');
  await freeList.getByText('Brannoc').waitFor();
  await everywhere(op, 'leave-owner-free-to-pick', 'The owner’s Characters page: Brannoc under Free to pick, ready to hand on', async () => {
    await owner.go(`/campaigns/${cid}/characters?open=players`);
    await op.locator('.pw-freelist').getByText('Brannoc').waitFor();
  });

  // Other players are not told: Mira is the only one with a note.
  assert.equal((await tam.api('/notifications')).body.filter((n) => n.type === 'member_left').length, 0, 'the leaver was sent the note');

  // 4. The join link still works, and the private note is back.
  const back = await tam.page.goto(BASE + '/join/' + code);
  assert.ok(back.status() < 400, 'the join link stopped working after leaving');
  await tam.go(`/campaigns/${cid}`);
  const notes = await tam.api(`/campaigns/${cid}/notes`);
  assert.equal(notes.status, 200);
  assert.ok(JSON.stringify(notes.body).includes('What Brannoc saw'), 'the private note did not come back');
  // Brannoc is still free: coming back does not hand it back by itself.
  await owner.go(`/campaigns/${cid}/characters?open=players`);
  await op.locator('.pw-freelist').getByText('Brannoc').waitFor();

  owner.assertClean();
  tam.assertClean();
});

test('the owner of a campaign cannot leave it, and a request cannot name another person', async () => {
  const owner = await person({ tag: 'lvx', name: 'Mira Vale' });
  const other = await person({ tag: 'lvy', name: 'Sela Dunn' });
  const cid = await newCampaign(owner, 'Held Fast');
  await addMember(owner, cid, other, 'player');
  const refused = await owner.api(`/campaigns/${cid}/leave`, { method: 'POST' });
  assert.equal(refused.status, 400, 'the owner was allowed to leave');
  await owner.go(`/campaigns/${cid}/members`);
  assert.ok(await owner.page.locator('#member-list').getByText('Sela Dunn').count(), 'someone else left when the owner asked');
  // A stranger is turned away.
  const stranger = await person({ tag: 'lvz', name: 'Nobody Here' });
  assert.equal((await stranger.api(`/campaigns/${cid}/leave`, { method: 'POST' })).status, 403);
  // Without the page's token the post is refused.
  const bare = await other.page.evaluate(async (id) => (await fetch(`/campaigns/${id}/leave`, { method: 'POST' })).status, cid);
  assert.equal(bare, 403, 'a post without the token went through');
});
