// account_my_view.spec.mjs — the My view card on Account Settings: dark mode,
// text size and calmer motion apply at once, are saved to the account (a
// second browser signed in as the same person sees them), and stay private to
// the viewer. Stands in for the operator check in #1024.
//
// Covers steps 1 to 5. The "on your phone" part of step 2 is a second browser
// context as the same account, not a real phone; whether the campaign header
// itself stops moving needs eyes on a real screen (the check asserts the
// per-viewer motion flag the header obeys).
//
// RUN: tools/live-checks.sh test/live/account_my_view.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, done, picture } from './_live.mjs';

after(done);

const attr = (page, name) => page.evaluate((n) => document.documentElement.getAttribute(n), name);
async function toggleThemeFrom(page, open, inputSel) {
  const was = await isDark(page);
  await open();
  const input = page.locator(inputSel);
  await input.waitFor({ state: 'visible' });
  await input.fill('toggle theme');
  const hit = page.getByText('Toggle Theme').first();
  await hit.waitFor();
  await hit.click();
  await page.waitForFunction((w) => document.documentElement.classList.contains('dark') !== w, was);
  assert.equal(await isDark(page), !was, 'Toggle Theme flips light and dark');
}
const isDark = (page) => page.evaluate(() => document.documentElement.classList.contains('dark'));

test('My view saves per account and stays private to the viewer', async () => {
  const owner = await person({ tag: 'myview-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'My view check');
  const player = await person({ tag: 'myview', name: 'Mara Vell' });
  await addMember(owner, cid, player, 'player');

  await player.go('/account');
  const card = player.page.locator('#my-view');
  await card.waitFor();
  const groups = ['Light or dark', 'Motion', 'Text size', 'Contrast'];
  for (const g of groups) await card.getByText(g, { exact: true }).waitFor();
  await picture(player.page, 'account-my-view', { check: '#1024', about: 'My view card with its four rows', locator: card });

  // Step 2: Dark applies at once, is saved, and shows in another browser.
  assert.equal(await isDark(player.page), false, 'starts light');
  await card.getByRole('button', { name: 'Dark', exact: true }).click();
  await card.getByText('Saved to your account').waitFor();
  assert.equal(await isDark(player.page), true, 'page turns dark at once');
  const phone = await person({ email: player.email, password: player.password, register: false });
  await phone.go('/account');
  assert.equal(await isDark(phone.page), true, 'dark on the second browser too');

  // Step 3: Largest text, then Standard puts it back.
  const size = () => player.page.evaluate(() => parseFloat(getComputedStyle(document.documentElement).fontSize));
  const before = await size();
  await card.getByRole('button', { name: 'Largest' }).click();
  await card.getByText('Saved to your account').waitFor();
  assert.ok(await size() > before * 1.2, 'text gets noticeably bigger');
  await player.go(`/campaigns/${cid}`);
  assert.ok(await size() > before * 1.2, 'and stays bigger on another page');
  await player.go('/account');
  await card.getByRole('button', { name: 'Standard' }).first().click();
  await card.getByText('Saved to your account').waitFor();
  assert.equal(await size(), before, 'Standard puts it back');

  // Step 4: Calmer motion is the player's alone.
  await card.getByRole('button', { name: 'Calm' }).click();
  await card.getByText('Saved to your account').waitFor();
  await player.go(`/campaigns/${cid}`);
  assert.equal(await attr(player.page, 'data-view-motion'), 'calm', 'the player is on calm motion');
  await owner.go(`/campaigns/${cid}`);
  assert.equal(await attr(owner.page, 'data-view-motion'), null, 'the owner is unaffected');

  // Step 5: Toggle Theme from the top-bar search flips light/dark.
  await player.go(`/campaigns/${cid}`);
  await toggleThemeFrom(player.page, () => player.page.locator('#topbar-search-trigger').click(), '#topbar-search-input');
  player.assertClean();
  owner.assertClean();
});

// Ctrl+K opens the older entity-search modal, whose own key handler bypasses
// the top-bar search that lists commands, so Toggle Theme is not offered.
test('Ctrl+K offers Toggle Theme', { todo: 'Ctrl+K opens the old search modal (static/js/search_modal.js) with no commands; the top-bar search that lists them (topbar_search.js) opens only from its button' }, async () => {
  const who = await person({ tag: 'myview-k' });
  const cid = await newCampaign(who, 'Ctrl K check');
  await who.go(`/campaigns/${cid}`);
  await toggleThemeFrom(who.page, () => who.page.keyboard.press('Control+k'), '#topbar-search-input');
});
