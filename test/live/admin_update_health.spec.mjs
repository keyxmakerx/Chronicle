// admin_update_health.spec.mjs — update health on a running site: a changed
// feature used ten times without an error passes on its own and is listed
// behind the quiet line under Needs you, not as a row in it (#2145).
//
// The AI helper page is one of the changed features the code lists. Other
// checks running at the same time use it too, so counts here are "at least".
// The failing and not-used-yet rows need a broken feature or two weeks, so
// they are checked by the Go tests instead.
//
// RUN: tools/live-checks.sh test/live/admin_update_health.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { siteAdmin, done, picture } from './_live.mjs';

after(done);

test('a changed feature used ten times passes and stays out of Needs you', async () => {
  const admin = await siteAdmin();
  const { page } = admin;
  for (let i = 0; i < 10; i++) await admin.go('/admin/diagnostics/workspace');

  await admin.go('/admin');
  const card = page.locator('.card', { has: page.getByRole('heading', { name: 'Needs you' }) });
  const line = card.locator('[data-update-health]');
  await line.waitFor();
  assert.match(await line.innerText(), /passed by use/, 'the quiet line counts the pass');
  assert.equal(await card.getByText("Site admin's AI helper").isVisible(), false, 'passes are folded away');

  await line.getByText('Show them').click();
  const row = line.locator('li', { hasText: "Site admin's AI helper" });
  await row.waitFor();
  const uses = Number((await row.innerText()).match(/(\d+) uses?/)[1]);
  assert.ok(uses >= 10, `counted ${uses} uses`);
  assert.equal(await card.locator('li', { hasText: "Site admin's AI helper has failed" }).count(), 0, 'no failure row');
  await picture(page, 'admin-update-health', {
    check: '#2145', about: 'Needs you with the passed changed features shown', locator: card,
    mask: ['[data-update-health] .font-mono', '[data-update-health] summary', 'main .card ul.divide-y'],
  });
  admin.assertClean();
});
