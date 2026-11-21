// page_live_refresh.spec.mjs — a page someone else saves reaches an open
// reader without a reload, and waits while that reader has unsaved work.
//
// RUN: tools/live-checks.sh test/live/page_live_refresh.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, newPage, done } from './_live.mjs';

after(done);

const doc = (t) => JSON.stringify({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: t }] }] });

test('an open page updates when the owner saves it, and holds while the reader has unsaved work', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Live refresh');
  await addMember(owner, cid, player, 'player');
  const eid = await newPage(owner, cid, 'Character', 'Quiet Reader');
  const shared = await owner.api(`/campaigns/${cid}/entities/${eid}/permissions`, { method: 'PUT', body: { visibility: 'default', is_private: false } });
  assert.ok(shared.ok, `sharing the page answered ${shared.status}`);
  const save = (text) => owner.api(`/campaigns/${cid}/entities/${eid}/entry`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entry: doc(text), entry_html: `<p>${text}</p>` }),
  });

  await player.go(`/campaigns/${cid}/entities/${eid}`);
  assert.ok(await player.page.locator('[data-widget="page-live"]').count(), 'the page carries the live-refresh widget');
  // Let the socket open before the first save.
  await player.page.waitForTimeout(1500);

  assert.ok((await save('first words ZQONE')).ok);
  await player.page.getByText('first words ZQONE').first().waitFor({ timeout: 10000 });

  // Unsaved work on the reader's side: the new version waits.
  await player.page.evaluate(() => Chronicle.markDirty('live-check'));
  assert.ok((await save('second words ZQTWO')).ok);
  await player.page.waitForTimeout(7000);
  assert.equal(await player.page.getByText('second words ZQTWO').count(), 0, 'a dirty page was refreshed under the reader');

  // Once the work is saved, the refresh arrives.
  await player.page.evaluate(() => Chronicle.markClean('live-check'));
  await player.page.getByText('second words ZQTWO').first().waitFor({ timeout: 12000 });
  owner.assertClean();
  player.assertClean();
});
