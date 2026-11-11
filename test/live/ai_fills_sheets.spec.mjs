// ai_fills_sheets.spec.mjs — what the campaign AI can do through the Copy
// and paste panel: hide DM-only text, make folders, tidy and look up tags,
// sketch onto an existing map, and refuse a secret in an event. Stands in for
// the operator check in #1587 (the merged #1586).
//
// Covers checks 2-6. Check 1 (Draw Steel hero and monster fields in the
// brief) needs the Draw Steel game-system package installed on the server;
// the scratch server has none, so a person checks it on a real campaign.
// The issue's page examples say `category: Lore`, which the importer does not
// read (the page key is `type:`), so these use `type: lore`, and add
// `visibility: public` so a player can be asked what they see.
//
// RUN: tools/live-checks.sh test/live/ai_fills_sheets.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

// Reviews a paste and, when `apply` is set, applies it. Returns the review
// text and, once applied, the page ids the result lists by name.
async function paste(who, cid, text, { apply = true } = {}) {
  const { page } = who;
  await who.go(`/campaigns/${cid}#copy-and-paste`);
  await page.locator('#ai-import-paste-area').waitFor({ state: 'visible' });
  await page.fill('#ai-import-paste-area', text);
  const parsed = page.waitForResponse((r) => r.url().includes('/import/parse'));
  await page.locator('#ai-tool-paste').getByRole('button', { name: 'Review', exact: true }).click();
  await parsed;
  const host = page.locator('#ai-import-review-host');
  await host.locator('h3, h4, p, li, div').first().waitFor();
  if (!apply) {
    await page.waitForTimeout(300);
    return { review: await host.innerText(), ids: {} };
  }
  const button = page.getByRole('button', { name: 'Apply' });
  await button.waitFor();
  const review = await host.innerText();
  await button.click();
  await page.getByText('Import complete').waitFor();
  const links = await host.locator('a:has-text("Open")').evaluateAll((as) => as.map((a) => [a.closest('li,div,tr')?.innerText || '', a.getAttribute('href')]));
  const ids = {};
  for (const [t, href] of links) {
    const id = (href.match(/entities\/([0-9a-f-]{36})/) || [])[1];
    if (id) ids[t.split('\n')[0].trim()] = id;
  }
  return { review, ids };
}

async function typeIdOf(who, cid, eid) {
  await who.go(`/campaigns/${cid}/entities/${eid}`);
  return who.page.locator('#sidebar-entity-tree').getAttribute('data-entity-type-id');
}

let worldPromise;
function world() {
  worldPromise = worldPromise || (async () => {
    const owner = await person({ tag: 'fill', name: 'Fill Owner' });
    const player = await person({ tag: 'fillp', name: 'Fill Player' });
    const cid = await newCampaign(owner, 'AI fills check');
    await addMember(owner, cid, player, 'player');
    await enableAddons(owner, cid, '*');
    const made = await owner.api(`/campaigns/${cid}/entity-types`, { method: 'POST', headers: { 'HX-Request': 'true' }, form: { name: 'Lore', name_plural: 'Lore', icon: 'fa-book', color: '#aa3333' } });
    assert.ok(made.ok, 'making the Lore category answered ' + made.status);
    return { owner, player, cid };
  })();
  return worldPromise;
}

test('2. a DM-only passage shows the owner marked GM-only and never reaches a player', async () => {
  const { owner, player, cid } = await world();
  const { ids } = await paste(owner, cid, `---
name: Test Secret
type: lore
visibility: public
---
The mayor is kind. {gm{He leads the cult.}gm}
`);
  const eid = ids['Test Secret'];
  assert.ok(eid, 'the import did not list Test Secret');
  const typeId = await typeIdOf(owner, cid, eid);
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: [{ id: 'b0', type: 'title' }, { id: 'b1', type: 'entry' }] }] }] };
  const saved = await owner.api(`/campaigns/${cid}/entity-types/${typeId}/layout`, { method: 'PUT', body: { layout } });
  assert.ok(saved.ok, 'layout save answered ' + saved.status);

  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const mine = owner.page.locator('main');
  await mine.getByText('He leads the cult.').waitFor();
  assert.doesNotMatch(await mine.innerText(), /\{gm\{|\}gm\}/, 'the braces show on the owner\'s page');
  await picture(owner.page, 'ai-gm-only-owner', { check: '#1587', about: 'Owner view: the passage marked GM-only', mask: ['#sidebar-entity-tree'] });

  await player.go(`/campaigns/${cid}/entities/${eid}`);
  await player.page.getByText('The mayor is kind.').waitFor();
  const theirs = await player.page.locator('body').innerText();
  assert.doesNotMatch(theirs, /He leads the cult/, 'the player sees the secret sentence');
  assert.doesNotMatch(theirs, /\{gm\{|\}gm\}/, 'the player sees the braces');
  assert.doesNotMatch(await player.page.content(), /He leads the cult/, 'the secret is in the player\'s page source');
});

test('3. a folder record and a page in it show as a Gods folder in the Lore sidebar', async () => {
  const { owner, cid } = await world();
  const { ids } = await paste(owner, cid, `---
kind: folder
category: Lore
name: Gods
---

---
name: Sun Hymn
type: lore
folder: Gods
tags: [Old Tag]
visibility: public
---
A hymn to the sun.
`);
  await owner.go(`/campaigns/${cid}/entities/${ids['Sun Hymn']}`);
  const tree = owner.page.locator('#sidebar-entity-tree');
  const folder = tree.getByText('Gods', { exact: true });
  await folder.waitFor();
  // Folders start folded when the open page is outside them; this page is
  // inside, so its row shows under the folder.
  await tree.getByText('Sun Hymn', { exact: true }).waitFor();
  const box = async (t) => (await t.boundingBox());
  const f = await box(folder.first()); const h = await box(tree.getByText('Sun Hymn', { exact: true }).first());
  assert.ok(h.x > f.x - 1 && h.y > f.y, 'the page should sit below the Gods folder');
  await picture(owner.page, 'ai-folder-sidebar', { check: '#1587', about: 'Lore sidebar with a Gods folder holding the page', locator: tree });
});

test('4. a tag record renames the tag everywhere and a lookup lists the tags with page counts', async () => {
  const { owner, cid } = await world();
  await paste(owner, cid, `---\nkind: tag\naction: update\nname: Old Tag\nrename_to: New Tag\n---\n`);
  const { review } = await paste(owner, cid, `---\nkind: lookup\nwhat: tags\n---\n`, { apply: false });
  assert.match(review, /New Tag, #[0-9a-f]{6}, 1 page/, 'the lookup does not list the renamed tag with its count:\n' + review);
  assert.doesNotMatch(review, /Old Tag/, 'the old tag name is still listed');
  const page = owner.page;
  await owner.go(`/campaigns/${cid}/entities?q=Sun%20Hymn`);
  assert.equal(await page.getByText('Old Tag').count(), 0, 'the old tag still shows on pages');
});

test('5. a map update is added onto the existing map and leaves hand drawings alone', async () => {
  const { owner, cid } = await world();
  const made = await owner.api(`/campaigns/${cid}/maps`, { method: 'POST', body: { name: 'Hand Map' } });
  assert.ok(made.ok, 'making the map answered ' + made.status);
  const mid = made.body.id;
  const mine = await owner.api(`/campaigns/${cid}/maps/${mid}/drawings`, { method: 'POST', body: { drawing_type: 'text', points: [{ x: 10, y: 10 }], stroke_color: '#000000', stroke_width: 2, text_content: 'Mine', font_size: 16, visibility: 'everyone' } });
  assert.ok(mine.ok, 'a hand drawing answered ' + mine.status);

  const { review } = await paste(owner, cid, `---
kind: map
action: update
name: Hand Map
grid: 30 x 20
---
room "Test" 2,2 to 6,6
`);
  assert.match(review, /adds 1 shape and 0 pins onto the Hand Map map/, 'the review row does not say the sketch is added onto the map:\n' + review);
  const drawings = (await owner.api(`/campaigns/${cid}/maps/${mid}/drawings`)).body;
  const texts = drawings.map((d) => d.text_content);
  assert.ok(texts.includes('Mine'), 'the hand drawing was removed');
  assert.ok(texts.includes('Test'), 'no box labelled Test appeared; drawings: ' + JSON.stringify(drawings));
});

test('6. a secret in a calendar event is refused and nothing is imported', async () => {
  const { owner, cid } = await world();
  const { review } = await paste(owner, cid, `---
kind: event
name: Secret Feast
year: 1492
month: 1
day: 1
---
Feast. {gm{the mayor is the cult leader}gm}
`, { apply: false });
  assert.match(review, /DM-only passages only work in page text/, 'no refusal in:\n' + review);
  assert.equal(await owner.page.getByRole('button', { name: 'Apply' }).isDisabled(), true, 'Apply should have nothing to apply');
});
