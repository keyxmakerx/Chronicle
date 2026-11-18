// people_invite_and_removed.spec.mjs — nobody joins a campaign without their
// own yes, and removing someone closes the join link behind them.
//
// The owner's "Invite someone" box makes an invite; the person is a member
// only after opening it. Removing them replaces the join link, the new link
// turns them away, and "Allow back" lets it work for them again.
//
// RUN: tools/live-checks.sh test/live/people_invite_and_removed.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, done, picture, BASE } from './_live.mjs';

after(done);

test('adding sends an invite, and removing replaces the join link and turns them away', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Invite Only');
  const { page } = owner;
  const list = page.locator('#member-list');
  const box = page.locator('#invite-box');

  // A join link is on before anyone is added.
  const first = await owner.api(`/campaigns/${cid}/join-code`, { method: 'POST' });
  assert.equal(first.status, 200);

  // Inviting makes an invite; the person is not a member yet.
  await owner.go(`/campaigns/${cid}/members`);
  await box.locator('input[name=email]').fill(player.email);
  await box.locator('input[name=email]').press('Enter');
  const link = box.getByLabel('Invite to send');
  await link.waitFor();
  const acceptUrl = await link.inputValue();
  const sentToAccount = (await box.getByRole('status').innerText()).split('\n')[0].replace(player.email, '<email>');
  assert.equal(await list.getByText('Tobin Hale').count(), 0, 'the person joined without accepting');

  // An address with no account gets the same answer, so the owner learns nothing about who has one.
  await box.locator('input[name=email]').fill('nobody-here-' + Date.now() + '@example.test');
  await box.locator('input[name=email]').press('Enter');
  await box.getByLabel('Invite to send').waitFor();
  const sentToStranger = (await box.getByRole('status').innerText()).split('\n')[0].replace(/nobody-here-\d+@example\.test/, '<email>');
  assert.equal(sentToStranger, sentToAccount, 'the answer differs for an address with an account');
  await picture(page, 'people-invite-made', { about: 'People page after inviting someone: an invite, not a member', fullPage: true, mask: ['[aria-label="Invite to send"]', '#invite-box li'] });

  // They accept and are in.
  await player.page.goto(acceptUrl);
  await player.page.getByText('Welcome!').waitFor();
  await owner.go(`/campaigns/${cid}/members`);
  assert.ok(await list.getByText('Tobin Hale').count(), 'the person is not a member after accepting');

  // Removing replaces the join link and lists them under Removed.
  page.once('dialog', (d) => d.accept());
  await list.locator('form[hx-delete] button').click();
  await list.getByRole('heading', { name: 'Removed' }).waitFor();
  await picture(page, 'people-removed', { about: 'People page after removing someone: the notice and the Removed list', fullPage: true, mask: ['#invite-box li'] });

  const old = await player.page.goto(BASE + '/join/' + first.body.code);
  assert.ok(old.status() >= 400, 'the old join link still works after a removal');

  // The link on the page is the new one, and it turns the removed person away.
  await owner.go(`/campaigns/${cid}/members`);
  await box.locator('input[aria-label="Invite link"]').waitFor({ state: 'attached' });
  const shown = (await page.content()).match(/\/join\/([A-Za-z0-9_-]+)/);
  assert.ok(shown && shown[1] !== first.body.code, 'the page does not show a new join link');
  const refused = await player.page.goto(BASE + '/join/' + shown[1]);
  assert.equal(refused.status(), 403, 'the new join link let the removed person in');

  // Allowed back, the same link works for them.
  await list.getByRole('button', { name: 'Allow back' }).click();
  await list.getByRole('heading', { name: 'Removed' }).waitFor({ state: 'detached' });
  await player.page.goto(BASE + '/join/' + shown[1]);
  await owner.go(`/campaigns/${cid}/members`);
  assert.ok(await list.getByText('Tobin Hale').count(), 'the person could not join after being allowed back');
  owner.assertClean();
});
