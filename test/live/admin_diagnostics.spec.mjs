// admin_diagnostics.spec.mjs — the Site admin AI helper's checks run inside a
// real server and answer with reports, not errors. Stands in for the
// operator check in #735.
//
// RUN: tools/live-checks.sh test/live/admin_diagnostics.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { siteAdmin, newCampaign, done, picture } from './_live.mjs';

after(done);

const CALLS = ['host.assets', 'host.embedded', 'host.errors', 'host.errors-summary', 'host.widgets', 'host.plugins', 'host.deploy-check'];

test('every host and campaign check runs through paste, review and approve', async () => {
  const admin = await siteAdmin();
  const cid = await newCampaign(admin, 'Diagnostics check');
  const { page } = admin;

  // A missing page is the "something went wrong" the error checks report on.
  await admin.go(`/campaigns/${cid}/entities/00000000-0000-0000-0000-000000000000`);

  await admin.go('/admin/diagnostics/workspace');
  await picture(page, 'admin-ai-helper', { check: '#735', about: 'Site admin, AI helper page' });
  await page.getByRole('button', { name: "Paste an AI's request" }).click();
  const batch = { v: 1, note: 'live check', full_dump: true, calls: [{ name: 'campaign.surfaces', arg: cid }, ...CALLS.map((name) => ({ name }))] };
  await page.fill('#dw-batch-input', JSON.stringify(batch));
  await page.getByRole('button', { name: 'Review the request' }).click();
  const run = page.getByRole('button', { name: /Approve & run 8 call/ });
  await run.waitFor();
  await run.click();
  const result = page.locator('#dw-review');
  await result.getByText('batch result').waitFor();
  const text = await result.innerText();

  for (const name of ['campaign.surfaces', ...CALLS]) {
    assert.match(text, new RegExp('✓ `?' + name.replace('.', '\\.')), `${name} did not run`);
    assert.match(text, new RegExp('## ' + name.replace('.', '\\.')), `${name} has no report section`);
  }
  assert.doesNotMatch(text, /✗/, 'a check was refused or failed');
  assert.doesNotMatch(text, /NOT REGISTERED/, 'campaign.surfaces found a page nobody registered');
  assert.doesNotMatch(text, /disagree/i, 'campaign.surfaces found a handler disagreement');
  assert.doesNotMatch(text, /Provider not wired/, 'an error check is not wired to its provider');
  admin.assertClean();
});
