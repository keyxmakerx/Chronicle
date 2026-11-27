// recent_activity.spec.mjs — the campaign's Recent Activity on Campaign
// settings' Data page, on a real server: an owner's changes come back as one
// sitting that says what changed in words, and a conflict the Foundry module
// reported through the sync history shows under Foundry Sync, opens to say
// which side was kept, and is found by the Conflicts chip.
//
// RUN: tools/live-checks.sh test/live/recent_activity.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, api, done, BASE } from './_live.mjs';

after(done);

test('changes read as a sitting in words, and a Foundry conflict opens in place', async () => {
  const owner = await person({ tag: 'ra-owner', name: 'Owner Echo' });
  const cid = await newCampaign(owner, 'Saltreach');
  const page = owner.page;
  const eid = await newPage(owner, cid, 'Character', 'Mira Thane');

  // A rename through the campaign form logs which field changed.
  await owner.go(`/campaigns/${cid}/settings`);
  const put = await api(page, `/campaigns/${cid}`, { method: 'PUT', form: { name: 'Saltreach Docks' } });
  assert.ok(put.status < 400, `renaming the campaign answered ${put.status}`);

  // A sync key, and the module's report of a settled conflict.
  const made = await api(page, `/campaigns/${cid}/api-keys`, { method: 'POST', form: { name: 'Foundry', perm_read: 'on', perm_write: 'on', perm_sync: 'on' } });
  const key = (String(made.body).match(/id="api-key-value"[^>]*>\s*([^<\s]+)/) || [])[1];
  assert.ok(key, 'no API key was shown');
  const res = await fetch(`${BASE}/api/v1/campaigns/${cid}/sync/history`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ events: [{ direction: 'to_chronicle', kind: 'character', resourceId: eid, name: 'Mira Thane', action: 'Foundry kept',
      call: 'foundry conflict', status: 'conflict', ok: true, message: 'The newest edit wins. Fields the Foundry edit sent: name, entry.' }] }),
  });
  assert.ok(res.ok, `reporting the conflict answered ${res.status}`);

  await owner.go(`/campaigns/${cid}/data`);
  await page.locator('#activity-content').scrollIntoViewIfNeeded();
  const ra = page.locator('#ra');
  await ra.waitFor();

  // The owner's sitting: a summary line, then each change in words.
  const mine = page.locator('.ra-sit').filter({ hasText: 'Owner Echo' });
  assert.match(await mine.locator('.ra-what').innerText(), /Changed .*setting/);
  await mine.locator('summary').first().click();
  await mine.getByText("Changed the campaign's name").waitFor();
  assert.ok(await mine.locator('.ra-row a', { hasText: 'Mira Thane' }).count(), 'the created page is not a link');
  assert.doesNotMatch(await ra.innerText(), /changed the campaign\b(?!'s)/i, 'a change still reads as the area words');

  // Foundry Sync's sitting, and its conflict opened in place.
  const fd = page.locator('.ra-sit').filter({ hasText: 'Foundry Sync' });
  assert.match(await fd.locator('.ra-pills').innerText(), /1 conflict/);
  await fd.locator('summary').first().click();
  await fd.locator('.ra-cf > summary').click();
  const box = fd.locator('.ra-cfbox');
  await box.waitFor();
  assert.match(await box.innerText(), /Foundry, kept/i);
  assert.match(await box.innerText(), /Fields the Foundry edit sent: name, entry\./);
  assert.equal(await box.getByRole('link', { name: "Compare in the page's history" }).getAttribute('href'), `/campaigns/${cid}/entities/${eid}?history=1`);

  // The Conflicts chip leaves only the conflict.
  await page.locator('.ra-chip', { hasText: 'Conflicts' }).click();
  await page.waitForFunction(() => [...document.querySelectorAll('#ra-list [data-row]')].filter((r) => r.offsetParent).length === 1);
  assert.equal(await mine.isVisible(), false, "the owner's sitting stays under the Conflicts chip");

  // History links land on the page itself with its History panel open, the
  // old bare-panel address included.
  await box.getByRole('link', { name: "Compare in the page's history" }).click();
  await page.locator('#page-history').waitFor();
  assert.equal(await page.locator('#page-history-slot').count(), 1, 'the panel opened inside the page, not on its own');
  await page.goto(`${BASE}/campaigns/${cid}/entities/${eid}/history`);
  await page.locator('#page-history').waitFor();
  assert.match(page.url(), /\?history=1$/);
  owner.assertClean();
});
