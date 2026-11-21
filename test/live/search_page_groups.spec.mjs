// search_page_groups.spec.mjs — the Search page answers "where does this come
// up?": what is named for the words first, then every page that mentions them,
// each with its marked line. A page hidden from players carries the lock and
// is never listed or quoted for a player, and a sentence the Director marked
// secret is never quoted, and finds nothing, for anyone.
//
// RUN: tools/live-checks.sh test/live/search_page_groups.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, newPage, done } from './_live.mjs';

after(done);

const SECRET = 'Duskharrow cult meets beneath the quay';

async function seed() {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Search groups');
  await addMember(owner, cid, player, 'player');

  const named = await newPage(owner, cid, 'Character', 'Tidewrights');
  const ledger = await newPage(owner, cid, 'Character', 'Harbour ledger');
  const vault = await newPage(owner, cid, 'Character', 'Hidden vault');

  const write = async (id, html) => {
    const r = await owner.api(`/campaigns/${cid}/entities/${id}/entry`, {
      method: 'PUT', body: { entry: JSON.stringify({ type: 'doc', content: [] }), entry_html: html },
    });
    assert.ok(r.ok, `writing a page answered ${r.status}`);
  };
  await write(named, '<p>A guild of harbour engineers.</p>');
  await write(ledger, `<p>The Tidewrights owe three seasons of berth fees.</p><p><span data-secret="true">${SECRET}</span></p>`);
  await write(vault, '<p>The Tidewrights keep their real ledger in this vault.</p>');
  // New pages start Director-only; the first two are shown to everyone, the
  // vault stays hidden.
  for (const id of [named, ledger]) {
    const r = await owner.api(`/campaigns/${cid}/entities/${id}/permissions`, { method: 'PUT', body: { visibility: 'default', is_private: false } });
    assert.ok(r.ok, `showing a page answered ${r.status}`);
  }
  return { owner, player, cid, ids: { named, ledger, vault } };
}

const group = (page, id) => page.locator(`#${id} .sp-row`);

test('the owner sees the named group, the mention group with marked lines, and the lock', async () => {
  const { owner, cid } = await seed();
  await owner.go(`/campaigns/${cid}/search?q=tidewrights`);
  await owner.page.waitForSelector('#sp-named .sp-row');

  assert.deepEqual(await group(owner.page, 'sp-named').locator('.sp-name').allInnerTexts(), ['Tidewrights']);
  const mention = group(owner.page, 'sp-mention');
  await mention.first().waitFor();
  const names = (await mention.locator('.sp-name').allInnerTexts()).map((s) => s.trim()).sort();
  assert.deepEqual(names, ['Harbour ledger', 'Hidden vault']);

  const ledger = mention.filter({ hasText: 'Harbour ledger' });
  assert.match(await ledger.locator('.sp-line').innerText(), /Tidewrights owe three seasons/);
  assert.equal(await ledger.locator('mark.sp-mark').first().innerText(), 'Tidewrights');
  assert.equal(await ledger.locator('.fa-lock').count(), 0, 'a shown page has no lock');

  const vault = mention.filter({ hasText: 'Hidden vault' });
  assert.equal(await vault.locator('.fa-lock').count(), 1, 'the hidden page carries the lock');
  assert.equal(await vault.getAttribute('data-dm-only'), '');

  assert.doesNotMatch(await owner.page.locator('body').innerText(), new RegExp(SECRET.split(' ')[0]), 'a secret sentence is never quoted, even to the owner');
  owner.assertClean();
});

test('a word that is only inside a secret sentence finds nothing, for the owner too', async () => {
  const { owner, cid } = await seed();
  await owner.go(`/campaigns/${cid}/search?q=duskharrow`);
  await owner.page.getByText(/Nothing matches/).waitFor();
  assert.equal(await owner.page.locator('.sp-row').count(), 0);
  owner.assertClean();
});

test('a player never sees the hidden page, a lock, or the secret line', async () => {
  const { player, cid } = await seed();
  await player.go(`/campaigns/${cid}/search?q=tidewrights`);
  await player.page.waitForSelector('#sp-named .sp-row');
  const mention = group(player.page, 'sp-mention');
  await mention.first().waitFor();
  assert.deepEqual((await mention.locator('.sp-name').allInnerTexts()).map((s) => s.trim()), ['Harbour ledger']);
  assert.equal(await player.page.locator('.fa-lock, [data-dm-only]').count(), 0);
  assert.doesNotMatch(await player.page.locator('body').innerText(), /Hidden vault|Duskharrow/);

  // The raw answer holds neither, whatever the page draws.
  for (const scope of ['named', 'mention']) {
    for (const q of ['tidewrights', 'duskharrow']) {
      const r = await player.api(`/campaigns/${cid}/entities/search?q=${q}&scope=${scope}`);
      const text = JSON.stringify(r.body);
      assert.ok(!/Duskharrow|Hidden vault/.test(text), `the ${scope} answer for ${q} leaks: ${text}`);
    }
  }

  await player.go(`/campaigns/${cid}/search?q=duskharrow`);
  await player.page.getByText(/Nothing matches/).waitFor();
  player.assertClean();
});

test('the pickers narrow the search and say what is picked', async () => {
  const { owner, cid } = await seed();
  await owner.go(`/campaigns/${cid}/search?q=tidewrights`);
  await owner.page.waitForSelector('#sp-named .sp-row');
  const category = owner.page.getByRole('button', { name: /All categories/ });
  await category.click();
  await owner.page.getByRole('option', { name: 'Locations' }).click();
  await owner.page.waitForFunction(() => document.querySelectorAll('.sp-row').length === 0);
  assert.match(await owner.page.locator('.sp-tools .sp-btn').first().innerText(), /Locations/);
  assert.match(owner.page.url(), /type=\d+/);
  owner.assertClean();
});

test('the tag picker narrows to the tagged pages and says which tag is picked', async () => {
  const { owner, cid, ids } = await seed();
  const json = { 'Content-Type': 'application/json' };
  const made = await owner.api(`/campaigns/${cid}/tags`, { method: 'POST', body: { name: 'Coast', color: '#0d9488' }, headers: json });
  assert.ok(made.ok, `making a tag answered ${made.status}`);
  const list = (await owner.api(`/campaigns/${cid}/tags`)).body;
  const tagId = list.find((t) => t.name === 'Coast').id;
  const set = await owner.api(`/campaigns/${cid}/entities/${ids.ledger}/tags`, { method: 'PUT', body: { tagIds: [tagId] }, headers: json });
  assert.ok(set.ok, `tagging a page answered ${set.status}`);

  await owner.go(`/campaigns/${cid}/search?q=tidewrights`);
  await owner.page.waitForSelector('#sp-named .sp-row');
  await owner.page.getByRole('button', { name: /^(Any tag|Coast)/ }).click();
  await owner.page.getByRole('option', { name: 'Coast' }).click();
  await owner.page.waitForFunction(() => document.querySelectorAll('#sp-named .sp-row').length === 0);
  assert.deepEqual((await group(owner.page, 'sp-mention').locator('.sp-name').allInnerTexts()).map((s) => s.trim()), ['Harbour ledger']);
  assert.match(await owner.page.locator('.sp-tools .sp-btn').nth(1).innerText(), /Coast/);
  assert.match(owner.page.url(), /tags=coast/);
  owner.assertClean();
});
