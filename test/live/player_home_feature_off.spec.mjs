// player_home_feature_off.spec.mjs — a new player's first campaign home, with
// the calendar off (how every campaign starts), draws no block that can only
// stay a loading bar and no error toast: a block for a feature that is off is
// not drawn at all.
//
// RUN: tools/live-checks.sh test/live/player_home_feature_off.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, BASE } from './_live.mjs';

after(done);

test('a player\'s home with the calendar off has no loading bar that never fills and no addon error', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Calendar off home');
  await addMember(owner, cid, player, 'player');

  // Any request on the player's own page that the server refuses is a block
  // that asked for something that is not there.
  const refused = [];
  player.page.on('response', (r) => {
    if (r.url().startsWith(BASE) && r.status() >= 400) refused.push(`${r.status()} ${r.url().replace(BASE, '')}`);
  });

  await player.go(`/campaigns/${cid}`);
  await player.page.waitForLoadState('networkidle');

  assert.equal(await player.page.locator('[aria-busy="true"]').count(), 0, 'a block is still loading');
  assert.equal(await player.page.locator('.animate-pulse').count(), 0, 'a loading bar is still on the home');
  assert.equal(await player.page.getByText(/addon is not enabled/i).count(), 0, 'the addon error is on the page');
  assert.deepEqual(refused.filter((u) => /when-we-play|last-time|getting-started|today-in-world|next-game|when-we-could-play/.test(u)), [],
    'the home asked for a block whose feature is off');
  player.assertClean();
});
