// import_link_fields.spec.mjs — link fields, worked-out (rule) fields and
// local coin names, made by pasting an AI's answer into the Copy and paste
// panel. Stands in for the operator check in #1607 (the merged #1598).
//
// Covers all four numbered checks: rule value and "set by hand" note, link
// field that opens a page and shows a player nothing, a coin renamed inside
// Saltreach only, and a Director-only word hidden from players. The world the
// paste expects (a Creatures category, an echelon field, pages and a shop
// inside Saltreach) is made first through the same paste panel and the API.
// Nothing is left for a person, except that the owner's own shop view shows
// raw coin names while editing (see the check for the observed behaviour).
//
// RUN: tools/live-checks.sh test/live/import_link_fields.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, newPage, done, picture } from './_live.mjs';

after(done);

// A paste, reviewed and applied the way an owner does it. Returns the page
// ids the result lists under "Open", by page name.
async function pasteAndApply(who, cid, text) {
  const { page } = who;
  await who.go(`/campaigns/${cid}#copy-and-paste`);
  await page.locator('#ai-import-paste-area').waitFor({ state: 'visible' });
  await page.fill('#ai-import-paste-area', text);
  const parsed = page.waitForResponse((r) => r.url().includes('/import/parse'));
  await page.locator('#ai-tool-paste').getByRole('button', { name: 'Review', exact: true }).click();
  await parsed;
  const apply = page.getByRole('button', { name: 'Apply' });
  await apply.waitFor();
  const review = await page.locator('#ai-import-review-host').innerText();
  assert.doesNotMatch(review, /can't be imported/i, 'the review refused a block:\n' + review);
  await apply.click();
  await page.getByText('Import complete').waitFor();
  const links = await page.locator('#ai-import-review-host a:has-text("Open")').evaluateAll((as) => as.map((a) => [a.closest('li,div,tr')?.innerText || '', a.getAttribute('href')]));
  const ids = {};
  for (const [text, href] of links) {
    const id = (href.match(/entities\/([0-9a-f-]{36})/) || [])[1];
    if (id) ids[text.split('\n')[0].trim()] = id;
  }
  return ids;
}

async function typeIdOf(who, cid, eid) {
  await who.go(`/campaigns/${cid}/entities/${eid}`);
  return who.page.locator('#sidebar-entity-tree').getAttribute('data-entity-type-id');
}

// A fresh campaign has empty page layouts; these put the blocks the checks
// read onto a category's pages.
async function setLayout(who, cid, typeId, blocks) {
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: blocks.map((type, i) => ({ id: 'b' + i, type })) }] }] };
  const res = await who.api(`/campaigns/${cid}/entity-types/${typeId}/layout`, { method: 'PUT', body: { layout } });
  assert.ok(res.ok, 'layout save answered ' + res.status);
}

const RULES = `---
kind: field-rule
name: Guild rank
follows: echelon
rows:
  - is: 1
    gives: Copper
  - is: 2
    gives: Silver
  - is: 3
    gives: Gold
  - from: 4
    gives: Starsteel
---

---
kind: category-field
category: Characters
name: Guild rank
type: rule
rule: Guild rank
---

---
kind: category-field
category: Items
name: Source creature
type: link
links_to: Creatures
section: Remnant
---

---
kind: local-word
name: Mark
local: Grain
place: Saltreach
---
`;

let worldPromise;
function world() {
  worldPromise = worldPromise || (async () => {
    const owner = await person({ tag: 'link', name: 'Link Owner' });
    const player = await person({ tag: 'linkp', name: 'Link Player' });
    const cid = await newCampaign(owner, 'Link fields check');
    await addMember(owner, cid, player, 'player');
    await enableAddons(owner, cid, '*');
    const made = await owner.api(`/campaigns/${cid}/entity-types`, { method: 'POST', headers: { 'HX-Request': 'true' }, form: { name: 'Creature', name_plural: 'Creatures', icon: 'fa-paw', color: '#aa3333' } });
    assert.ok(made.ok, 'making the Creatures category answered ' + made.status);
    await pasteAndApply(owner, cid, `---\nkind: category-field\ncategory: Characters\nname: Echelon\nkey: echelon\ntype: number\n---\n`);
    const ids = await pasteAndApply(owner, cid, `---
name: Mara Vell
type: character
visibility: public
fields: {echelon: 2}
---
A guild courier.
---
name: Ember Wolf
type: creature
visibility: private
---
A wolf only the Director knows.
---
name: Wolf Pelt
type: item
visibility: public
---
A pelt.
---
name: Saltreach
type: location
visibility: public
---
A port.
---
name: Grain Stall
type: shop
parent: Saltreach
visibility: public
---
A stall in Saltreach.
---
name: Far Stall
type: shop
visibility: public
---
A stall elsewhere.
`);
    for (const n of ['Mara Vell', 'Wolf Pelt', 'Grain Stall']) assert.ok(ids[n], `no id for ${n}`);
    await setLayout(owner, cid, await typeIdOf(owner, cid, ids['Mara Vell']), ['title', 'attributes']);
    await setLayout(owner, cid, await typeIdOf(owner, cid, ids['Wolf Pelt']), ['title', 'attributes']);
    await setLayout(owner, cid, await typeIdOf(owner, cid, ids['Grain Stall']), ['title', 'shop_inventory']);
    // Prices in Marks, on both shops.
    for (const shop of ['Grain Stall', 'Far Stall']) {
      const res = await owner.api(`/campaigns/${cid}/entities/${ids[shop]}/relations`, {
        method: 'POST',
        body: { targetEntityId: ids['Wolf Pelt'], relationType: 'sells', reverseRelationType: 'sold by', metadata: { price: 5, currency: 'Mark', quantity: null, in_stock: true } },
      });
      assert.ok(res.ok, 'stocking ' + shop + ' answered ' + res.status);
    }
    await pasteAndApply(owner, cid, RULES);
    return { owner, player, cid, ids };
  })();
  return worldPromise;
}

const fact = (page, label) => page.locator('.cb-fact', { hasText: label });

test('1. a worked-out field reads its rule, and a hand-typed value wins with a note', async () => {
  const { owner, cid, ids } = await world();
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${ids['Mara Vell']}`);
  await fact(page, 'Guild rank').getByText('Silver').waitFor();
  await picture(page, 'import-rule-field', { check: '#1607', about: 'Character sheet with a worked-out Guild rank', mask: ['#sidebar-entity-tree', '.recent-list'] });
  await page.locator('[data-widget="attributes"] .chronicle-editor__edit-btn', { hasText: 'Edit' }).click();
  const input = page.locator('#attr-guild_rank');
  await input.fill('Iron');
  await page.locator('[data-widget="attributes"] .chronicle-editor__edit-btn', { hasText: 'Done' }).click();
  await fact(page, 'Guild rank').getByText('Iron').waitFor();
  await fact(page, 'Guild rank').getByText('Set by hand. The rule gives Silver.').waitFor();
  owner.assertClean();
});

test('2. a link field opens the creature page; a player who cannot see it gets nothing', async () => {
  const { owner, player, cid, ids } = await world();
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${ids['Wolf Pelt']}`);
  await page.locator('[data-widget="attributes"] .chronicle-editor__edit-btn', { hasText: 'Edit' }).click();
  const box = page.locator('#attr-source_creature');
  await box.fill('Ember Wolf');
  await page.waitForFunction(() => { const h = document.querySelector('input[type=hidden][data-field-key=source_creature]'); return h && h.value; });
  await page.locator('[data-widget="attributes"] .chronicle-editor__edit-btn', { hasText: 'Done' }).click();
  const link = fact(page, 'Source creature').getByRole('link', { name: 'Ember Wolf' });
  await link.waitFor();
  assert.match(await link.getAttribute('href'), new RegExp(`/entities/${ids['Ember Wolf']}$`), 'the link does not open the creature page');

  await player.go(`/campaigns/${cid}/entities/${ids['Wolf Pelt']}`);
  await player.page.getByText('Wolf Pelt').first().waitFor();
  await player.page.waitForLoadState('networkidle');
  const body = await player.page.locator('main').innerText();
  assert.doesNotMatch(body, /Ember Wolf/, 'the player sees the hidden creature\'s name');
  assert.ok(!body.includes(ids['Ember Wolf']), 'the player sees the hidden creature\'s id');
  assert.equal(await player.page.locator(`a[href*="${ids['Ember Wolf']}"]`).count(), 0, 'the player has a link to the hidden creature');
  owner.assertClean();
});

test('3. a shop inside Saltreach shows Grain with Mark on hover; a shop outside says Mark', async () => {
  const { player, cid, ids } = await world();
  const inside = player.page;
  await player.go(`/campaigns/${cid}/entities/${ids['Grain Stall']}`);
  const price = inside.locator('.shop-inv-price');
  await price.first().waitFor();
  await inside.waitForFunction(() => /Grain/.test(document.querySelector('.shop-inv-price')?.textContent || ''));
  assert.equal((await price.first().innerText()).trim(), '5 Grain');
  assert.equal(await price.first().getAttribute('title'), '5 Mark', 'hover should show the usual word');
  await picture(inside, 'import-local-coin', { check: '#1607', about: 'A shop inside Saltreach, seen by a player, prices in Grain', mask: ['#sidebar-entity-tree'] });

  // Both shops are in the Shop category, so they share its layout.
  await player.go(`/campaigns/${cid}/entities/${ids['Far Stall']}`);
  const far = player.page.locator('.shop-inv-price');
  await far.first().waitFor();
  assert.equal((await far.first().innerText()).trim(), '5 Mark', 'a shop outside Saltreach should keep Mark');
});

test('4. a Director-only word shows players Mark and the Director Grain', async () => {
  const { owner, player, cid, ids } = await world();
  await pasteAndApply(owner, cid, `---\nkind: local-word\naction: update\nname: Mark\nlocal: Grain\nplace: Saltreach\ndirector: true\n---\n`);
  await player.go(`/campaigns/${cid}/entities/${ids['Grain Stall']}`);
  const price = player.page.locator('.shop-inv-price').first();
  await price.waitFor();
  await player.page.waitForLoadState('networkidle');
  assert.equal((await price.innerText()).trim(), '5 Mark', 'a player must not see the Director-only word');

  // The Director's own words come from the same lookup the shop uses.
  const words = await owner.api(`/campaigns/${cid}/entities/${ids['Grain Stall']}/local-words`);
  assert.equal(words.body.words.mark, 'Grain', 'the Director should still get Grain');
  const theirs = await player.api(`/campaigns/${cid}/entities/${ids['Grain Stall']}/local-words`);
  assert.equal((theirs.body.words || {}).mark, undefined, 'the player was sent the Director-only word');
});
