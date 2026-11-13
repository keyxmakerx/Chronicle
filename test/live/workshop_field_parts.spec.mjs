// workshop_field_parts.spec.mjs — Homebrew Workshop recipes that carry sheet
// fields, worked-out rules and local words save what they describe. Stands in
// for the operator check in #1638 (the merged #1632).
//
// The Workshop page opens straight at /campaigns/<id>/homebrew with no game
// system or add-on needed; only its link inside the Rulebook needs an
// installed system (none on a scratch server), so these open it by address.
// Covers steps 2-6 by saving recipes through the Workshop's own recipe API,
// because the Field, Item fields and Local words parts have no form in the
// Workshop by design: the inspector names where each is set up and links
// there, which the last subtest checks. Step 1 (the Rulebook > More
// route) needs an installed system package.
//
// RUN: tools/live-checks.sh test/live/workshop_field_parts.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

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
  assert.doesNotMatch(await page.locator('#ai-import-review-host').innerText(), /can't be imported/i);
  await apply.click();
  await page.getByText('Import complete').waitFor();
  const links = await page.locator('#ai-import-review-host a:has-text("Open")').evaluateAll((as) => as.map((a) => [a.closest('li,div,tr')?.innerText || '', a.getAttribute('href')]));
  const ids = {};
  for (const [t, href] of links) {
    const id = (href.match(/entities\/([0-9a-f-]{36})/) || [])[1];
    if (id) ids[t.split('\n')[0].trim()] = id;
  }
  return ids;
}

async function typeIdOf(who, cid, eid) {
  await who.go(`/campaigns/${cid}/entities/${eid}`);
  return who.page.locator('#sidebar-entity-tree').getAttribute('data-entity-type-id');
}

async function setLayout(who, cid, typeId, blocks) {
  const layout = { rows: [{ id: 'r1', columns: [{ id: 'c1', width: 12, blocks: blocks.map((type, i) => ({ id: 'b' + i, type })) }] }] };
  const res = await who.api(`/campaigns/${cid}/entity-types/${typeId}/layout`, { method: 'PUT', body: { layout } });
  assert.ok(res.ok, 'layout save answered ' + res.status);
}

const part = (key, kind, label, data) => ({ key, kind, label, see: 'director', data });
const save = (who, cid, name, parts, prior) => who.api(`/campaigns/${cid}/homebrew/recipes` + (prior ? '/' + prior.id : ''), {
  method: prior ? 'PUT' : 'POST',
  body: { name, doc: { parts, rules: [] }, ...(prior ? { version: prior.version } : {}) },
});

let worldPromise;
function world() {
  worldPromise = worldPromise || (async () => {
    const owner = await person({ tag: 'wsp', name: 'Workshop Owner' });
    const player = await person({ tag: 'wspp', name: 'Workshop Player' });
    const cid = await newCampaign(owner, 'Workshop fields check');
    await addMember(owner, cid, player, 'player');
    await enableAddons(owner, cid, '*');
    const made = await owner.api(`/campaigns/${cid}/entity-types`, { method: 'POST', headers: { 'HX-Request': 'true' }, form: { name: 'Creature', name_plural: 'Creatures', icon: 'fa-paw', color: '#aa3333' } });
    assert.ok(made.ok);
    await pasteAndApply(owner, cid, `---\nkind: category-field\ncategory: Characters\nname: Echelon\nkey: echelon\ntype: number\n---\n`);
    const ids = await pasteAndApply(owner, cid, `---
name: Mara Vell
type: character
visibility: public
fields: {echelon: 1}
---
A guild courier.
---
name: Ember Wolf
type: creature
visibility: public
---
A wolf.
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
`);
    const types = {};
    for (const n of ['Mara Vell', 'Wolf Pelt', 'Grain Stall']) types[n] = await typeIdOf(owner, cid, ids[n]);
    await setLayout(owner, cid, types['Mara Vell'], ['title', 'attributes']);
    await setLayout(owner, cid, types['Wolf Pelt'], ['title', 'attributes']);
    await setLayout(owner, cid, types['Grain Stall'], ['title', 'shop_inventory']);
    const rel = await owner.api(`/campaigns/${cid}/entities/${ids['Grain Stall']}/relations`, {
      method: 'POST',
      body: { targetEntityId: ids['Wolf Pelt'], relationType: 'sells', reverseRelationType: 'sold by', metadata: { price: 5, currency: 'Mark', quantity: null, in_stock: true } },
    });
    assert.ok(rel.ok);
    return { owner, player, cid, ids, types };
  })();
  return worldPromise;
}

const fieldsOf = async (who, cid, typeId) => (await who.api(`/campaigns/${cid}/entity-types/${typeId}/layout`)).body.fields || [];
const fact = (page, label) => page.locator('.cb-fact', { hasText: label });

let guild;

test('2. a Field part saves, and the character sheet shows Guild rank: Copper', async () => {
  const { owner, cid, ids, types } = await world();
  const res = await save(owner, cid, 'Guild', [part('guild_rank', 'field', 'Guild rank', {
    category: 'Characters', label: 'Guild rank', type: 'rule', rule: { follows: 'echelon', rows: [{ is: '1', gives: 'Copper' }] },
  })]);
  assert.equal(res.status, 201, 'the save answered ' + res.status + ' ' + JSON.stringify(res.body));
  guild = res.body;
  await owner.go(`/campaigns/${cid}/entities/${ids['Mara Vell']}`);
  await fact(owner.page, 'Guild rank').getByText('Copper').waitFor();
  const n = (await fieldsOf(owner, cid, types['Mara Vell'])).filter((f) => f.label === 'Guild rank').length;
  assert.equal(n, 1);
  await picture(owner.page, 'workshop-guild-rank', { check: '#1638', about: 'Character sheet with Guild rank: Copper made by a recipe', mask: ['#sidebar-entity-tree'] });
});

test('3. saving the same recipe again leaves exactly one Guild rank field', async () => {
  const { owner, cid, types } = await world();
  const again = await save(owner, cid, 'Guild', guild.doc.parts, guild);
  assert.ok(again.ok, 'the second save answered ' + again.status + ' ' + JSON.stringify(again.body));
  const n = (await fieldsOf(owner, cid, types['Mara Vell'])).filter((f) => f.label === 'Guild rank').length;
  assert.equal(n, 1, 'Guild rank should still be one field');
});

test('4. Item fields add a creature link and a select; the link only offers creatures', async () => {
  const { owner, cid, ids, types } = await world();
  const res = await save(owner, cid, 'Items', [part('item_fields', 'item', 'Item fields', {
    category: 'Items',
    fields: [
      { label: 'Source creature', type: 'link', linksTo: 'Creatures' },
      { label: 'Remnant kind', type: 'select', options: ['Bone', 'Shell'] },
    ],
  })]);
  assert.equal(res.status, 201, 'the save answered ' + res.status + ' ' + JSON.stringify(res.body));
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${ids['Wolf Pelt']}`);
  // Empty fields show only while editing.
  await page.locator('[data-widget="attributes"] .chronicle-editor__edit-btn', { hasText: 'Edit' }).click();
  const box = page.locator('#attr-source_creature');
  await box.waitFor();
  await page.locator('#attr-remnant_kind').waitFor();
  assert.deepEqual(await page.locator('#attr-remnant_kind option:not([value=""])').evaluateAll((os) => os.map((o) => o.value)), ['Bone', 'Shell']);
  await box.click();
  await page.waitForFunction(() => document.querySelectorAll('#attr-source_creature-pages option').length > 0);
  const offered = await page.locator('#attr-source_creature-pages option').evaluateAll((os) => os.map((o) => o.value));
  assert.deepEqual(offered, ['Ember Wolf'], 'the link should offer creature pages only');
});

test('5. a Local words part makes Mark read Grain in a Saltreach shop', async () => {
  const { owner, player, cid, ids } = await world();
  const res = await save(owner, cid, 'Words', [part('salt_words', 'words', 'Saltreach words', {
    placeId: ids.Saltreach, words: [{ word: 'Mark', local: 'Grain' }],
  })]);
  assert.equal(res.status, 201, 'the save answered ' + res.status + ' ' + JSON.stringify(res.body));
  await player.go(`/campaigns/${cid}/entities/${ids['Grain Stall']}`);
  const price = player.page.locator('.shop-inv-price').first();
  await price.waitFor();
  await player.page.waitForFunction(() => /Grain/.test(document.querySelector('.shop-inv-price')?.textContent || ''));
  assert.equal((await price.innerText()).trim(), '5 Grain');
});

test('6. turning a text field into a number field is refused by name, and the sheet is unchanged', async () => {
  const { owner, cid, types } = await world();
  const field = (type) => part('title_line', 'field', 'Title line', { category: 'Characters', label: 'Title line', type });
  const first = await save(owner, cid, 'Titles', [field('text')]);
  assert.equal(first.status, 201, JSON.stringify(first.body));
  const second = await save(owner, cid, 'Titles', [field('number')], first.body);
  assert.ok(!second.ok, 'the save was not refused (status ' + second.status + ')');
  assert.match(JSON.stringify(second.body), /title.line/i, 'the refusal does not name the field: ' + JSON.stringify(second.body));
  const f = (await fieldsOf(owner, cid, types['Mara Vell'])).find((x) => x.label === 'Title line');
  assert.equal(f && f.type, 'text', 'the sheet field changed type');
});

test('the Workshop opens by address and its Field part says where it is set up, with a link that loads', async () => {
  const { owner, cid } = await world();
  const { page } = owner;
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  const saved = await save(owner, cid, 'Notes', [part('rank_note', 'field', 'Rank note', { category: 'Characters', label: 'Rank note', type: 'text' })]);
  assert.equal(saved.status, 201, JSON.stringify(saved.body));
  await owner.go(`/campaigns/${cid}/homebrew`);
  await page.locator('.hbw-item').filter({ hasText: 'Notes' }).first().click();
  await page.locator('.hbw-card, .hbw-node').filter({ hasText: 'Rank note' }).first().click();
  const where = page.locator('.hbw p').filter({ hasText: /is set on the/i });
  await where.first().waitFor();
  assert.match(await where.first().innerText(), /Rank note is set on the Characters sheet/);
  assert.equal(await page.locator('.hbw input[aria-label*="Category" i], .hbw label:has-text("Category")').count(), 0, 'the Field part grew a form');
  await picture(page, 'workshop-field-part', { check: '#1638', about: 'Workshop with a Field part selected, naming where it is set up' });
  const href = await where.first().getByRole('link').getAttribute('href');
  assert.ok(href && href.startsWith(`/campaigns/${cid}/`), 'the link points somewhere else: ' + href);
  const res = await owner.go(href);
  assert.equal(res.status(), 200, 'the link answered ' + res.status());
  assert.deepEqual(errors, [], 'page errors: ' + errors.join('; '));
});
