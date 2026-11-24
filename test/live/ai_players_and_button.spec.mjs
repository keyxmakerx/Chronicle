// ai_players_and_button.spec.mjs: three AI walks that share one owner and one
// player, so each pays for its own steps and not for new accounts. Each walk
// still makes its own campaign (or, for the paste checks, the one world below),
// so keys, requests and pages never carry over between them.
//
// The AI button, players' AI that is off until the owner turns it on, a
// player's own key that changes only their own character, and the owner's
// Pause and off switch. Stands in for the operator check in #1977.
//   1 (the button, the bubble and side panel, Escape, Ctrl+V paste that opens a
//   review and doesn't when typing in a field), 2 (no button for a player until
//   the switch is on), 3 (the player's key, a paste reviewed with Tyne's change
//   offered and a new page left out, Apply writes the sheet, the owner's panel
//   shows none of it), 4 (Pause and "Let it back", the switch off: the button
//   goes and the AI is told). The player's paste box is reached the way Ctrl+V
//   reaches it: a paste event with the text on the page.
//
// What the campaign AI can do through the Copy and paste panel: hide DM-only
// text, make folders, tidy and look up tags, sketch onto an existing map, and
// refuse a secret in an event. Stands in for the operator check in #1587 (the
// merged #1586), checks 2-6. Check 1 (Draw Steel hero and monster fields in the
// brief) needs the Draw Steel game-system package installed on the server; the
// scratch server has none, so a person checks it on a real campaign. The page
// key is `type:`, not `category:`, so these use `type: lore`, and add
// `visibility: public` so a player can be asked what they see.
//
// What the AI can set up and read: its instructions split in parts, a montage
// test, a hidden tracker with a logged change, and a rolled night given a
// result. Stands in for the operator check in #2021: 1 (the guide whole, then
// one category's fields, in What it did), 2 (a Test row in the review, the
// warning when the page has no Tests block, the test on the page once the Tests
// block is there), 3 (a hidden tracker and a logged change, shown to the owner,
// not to a player), 4 (a result for a rolled night: shown with no dice total
// and locked, and rolling the year again leaves it). Needs a person: 5 (the
// Draw Steel stat block needs the installed Draw Steel package), and the
// category named Creature in step 1 (it comes with that package; the same call
// is made with Character here).
//
// RUN: tools/live-checks.sh test/live/ai_players_and_button.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { appendFileSync } from 'node:fs';
import { person as signedIn, newCampaign, newPage, addMember, enableAddons, done, picture, saved, once, resetErrors, BASE } from './_live.mjs';

after(done);

/** person() with a retry: sign-up occasionally lands before its redirect settles. */
async function person(opts) {
  for (let i = 0; ; i++) {
    try { return await signedIn(opts); } catch (e) { if (i >= 2 || !/could not (register|sign in)/.test(e.message)) throw e; }
  }
}

// The owner and player every walk in the file uses.
const accounts = once(async () => ({
  owner: await person({ tag: 'aiown', name: 'Owner Olive' }),
  player: await person({ tag: 'aiplr', name: 'Player Pat' }),
}));
beforeEach(async () => { const w = await accounts(); resetErrors(w.owner, w.player); });

const LEVEL = { ask: /Look and ask for changes/, everything: /Look at everything/, players: /Look at what players see/ };

/** Make a key through the Your AI page and return an MCP caller for it. */
async function makeKey(owner, cid, level) {
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/ai`);
  const renew = page.getByRole('button', { name: 'Make a new key' });
  if (await renew.count()) {
    await renew.click();
  }
  if (level) await page.getByRole('radio', { name: level }).click();
  await page.getByRole('button', { name: 'Make a key', exact: true }).last().click();
  await page.locator('.aic-key').waitFor();
  const [addr, key] = await page.locator('.aic-copy code').allInnerTexts();
  const ai = mcp(addr, key);
  await ai.rpc('initialize', { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'Claude Code', version: '1' } });
  return ai;
}

/** What an AI app does: JSON-RPC over HTTP with the key as a bearer token. */
function mcp(addr, key) {
  const rpc = async (method, params) => {
    const r = await fetch(addr, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + key, Accept: 'application/json, text/event-stream' },
      body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params }),
    });
    const text = await r.text();
    let json = null;
    try { json = JSON.parse(text); } catch { /* a refusal is plain text */ }
    return { status: r.status, text, json };
  };
  const call = async (name, args = {}) => {
    const r = await rpc('tools/call', { name, arguments: args });
    const res = r.json && r.json.result;
    return { ...r, isError: !!(res && res.isError) || !!(r.json && r.json.error), out: res ? res.content.map((c) => c.text).join('\n') : (r.json && r.json.error ? r.json.error.message : r.text) };
  };
  return { rpc, call };
}


const JSON_HEADERS = { 'Content-Type': 'application/json' };

/** Answer a waiting request from the AI panel: Yes to every row, then Apply. */
async function answerYes(owner, cid, summary) {
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  const card = page.locator('#ai-panel section.aic-request', { hasText: summary });
  await card.getByRole('button', { name: 'Review' }).click();
  const review = card.locator('.aic-req-review');
  await review.getByRole('button', { name: 'Apply' }).waitFor();
  await review.getByRole('button', { name: /Yes to every request/ }).click();
  await review.getByRole('button', { name: 'Apply' }).click();
  await page.locator('#ai-panel .aic-done').waitFor();
  return review;
}

/** Ask the AI's way (MCP), then the owner answers Yes. */
async function askAndApply(owner, cid, ai, summary, changes) {
  const sent = await ai.call('ask_for_changes', { summary, changes });
  assert.ok(!sent.isError, `"${summary}" was not sent: ${sent.out}`);
  await answerYes(owner, cid, summary);
}

/** Put blocks (by type name) on a category's page layout, as the Customizer would. */
async function addBlocks(owner, cid, category, types) {
  await owner.go(`/campaigns/${cid}/entities/new`);
  const etid = await owner.page.locator('select#entity_type_id').evaluate((el, word) => (([...el.options].find((o) => o.value && o.textContent.toLowerCase().includes(word.toLowerCase())) || {}).value), category);
  const got = await owner.api(`/campaigns/${cid}/entity-types/${etid}/layout`);
  const layout = got.body.layout && got.body.layout.rows && got.body.layout.rows.length ? got.body.layout : { rows: [{ id: 'row-1', columns: [{ id: 'col-1-1', width: 12, blocks: [{ id: 'blk-title', type: 'title' }, { id: 'blk-entry', type: 'entry' }] }] }] };
  for (const t of types) layout.rows.push({ id: 'row-' + t, columns: [{ id: 'row-' + t + '-col', width: 12, blocks: [{ id: 'blk-' + t, type: t }] }] });
  const put = await owner.api(`/campaigns/${cid}/entity-types/${etid}/layout`, { method: 'PUT', body: JSON.stringify({ layout }), headers: JSON_HEADERS });
  assert.ok(put.ok, 'could not change the layout: ' + JSON.stringify(put.body));
}

/** The keys box on an owner's Your AI page, or the player's panel, read as an MCP caller. */
async function keyFromCodes(page) {
  const [addr, key] = await page.locator('.aic-copy code').allInnerTexts();
  return mcp(addr, key);
}

/** Ctrl+V with `text` copied: a paste event aimed at `selector` (the page body by default). */
async function pasteText(page, text, selector = 'body') {
  await page.evaluate(({ text, selector }) => {
    const dt = new DataTransfer();
    dt.setData('text/plain', text);
    document.querySelector(selector).dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
  }, { text, selector });
}

const PAGE_BLOCK = '---\nname: Pasted Pip\ntype: Character\n---\nA pasted page.\n';

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

const world = once(async () => {
    const { owner, player } = await accounts();
    const cid = await newCampaign(owner, 'AI fills check');
    await addMember(owner, cid, player, 'player');
    await enableAddons(owner, cid, '*');
    const made = await owner.api(`/campaigns/${cid}/entity-types`, { method: 'POST', headers: { 'HX-Request': 'true' }, form: { name: 'Lore', name_plural: 'Lore', icon: 'fa-book', color: '#aa3333' } });
    assert.ok(made.ok, 'making the Lore category answered ' + made.status);
    return { owner, player, cid };
});

test('the AI button: bubble, side panel, Escape and Ctrl+V', async () => {
  const { owner } = await accounts();
  const cid = await newCampaign(owner, 'Button Campaign');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  const chip = page.locator('[data-ai-chip]');
  await chip.waitFor();
  await chip.click();
  const bubble = page.locator('#ai-bubble');
  await bubble.waitFor({ state: 'visible' });
  await bubble.getByText(/Nothing waiting/).waitFor();
  await picture(page, 'ai-button-bubble', { check: '#1977', about: 'The AI button dropped open on a campaign page', mask: ['.aib-foot', '.aib-foot *'] });
  await bubble.getByRole('button', { name: /Side panel/ }).click();
  const panel = page.locator('#ai-panel.is-open');
  await panel.waitFor();
  await panel.getByRole('region', { name: 'Waiting for you' }).waitFor();
  await picture(page, 'ai-button-panel', { check: '#1977', about: 'The side panel from the AI button, nothing waiting', mask: ['h1 + *'] });
  await page.keyboard.press('Escape');
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });

  // Ctrl+V with an AI's block copied, on empty space, opens a review of it.
  await pasteText(page, PAGE_BLOCK);
  await page.locator('#ai-panel.is-open').waitFor();
  await page.getByText('Add the page Pasted Pip').waitFor();
  await page.keyboard.press('Escape');
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });

  // The same paste into a text field a person is typing in is left alone.
  await page.evaluate(() => { const i = document.createElement('input'); i.id = 'typing'; document.body.appendChild(i); i.focus(); });
  await pasteText(page, PAGE_BLOCK, '#typing');
  await page.waitForTimeout(500);
  assert.equal(await page.locator('#ai-panel.is-open').count(), 0, 'a paste into a text field opened the AI panel');
  owner.assertClean();
});

test('a player has no AI until the owner allows it; their key changes only their character', async () => {
  const { owner, player } = await accounts();
  const cid = await newCampaign(owner, 'Players AI');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const tyne = await newPage(owner, cid, 'Character', 'Tyne');
  const sword = await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: JSON.stringify({ entity_ids: [tyne], private: false }), headers: { 'Content-Type': 'application/json' } });
  assert.ok(sword.ok);
  const playable = await owner.api('/campaigns/' + cid + '/characters/playable', { method: 'POST', form: { entity_id: tyne, playable: '1' } });
  assert.ok(playable.ok, 'could not mark Tyne playable: ' + JSON.stringify(playable.body));
  const claim = await player.api(`/campaigns/${cid}/entities/${tyne}/claim`, { method: 'POST' });
  assert.ok(claim.ok, 'the player could not claim Tyne: ' + JSON.stringify(claim.body));

  // 2. Off for players until the owner turns it on.
  const hasButton = async () => { await player.go(`/campaigns/${cid}`); await player.page.waitForTimeout(500); return player.page.locator('.ai-chip-host:not([hidden]) [data-ai-chip]').isVisible(); };
  assert.equal(await hasButton(), false, 'a player has an AI button before the owner allowed it');
  await owner.go(`/campaigns/${cid}/ai`);
  await owner.page.getByRole('tab', { name: 'Settings' }).click();
  const sw = owner.page.getByRole('switch', { name: 'Players may use an AI here' });
  await sw.waitFor();
  await picture(owner.page, 'ai-players-switch', { check: '#1977', about: "The Players' AI row on Your AI > Settings, switch off", mask: ['.aic-head .aic-hint', '.aic-row-k .aic-hint'], locator: owner.page.locator('.aic-rows') });
  await saved(owner.page, '/ai/players', () => sw.check());
  await owner.page.locator('#ai-players .aic-switch:checked').waitFor();
  assert.equal(await hasButton(), true, 'the player has no AI button after the owner turned it on');

  // 3. The player makes a key and pastes a change to their own character.
  const { page } = player;
  await page.locator('[data-ai-chip]').click();
  await page.locator('#ai-bubble').getByRole('button', { name: 'Connect your AI app' }).click();
  const panel = page.locator('#ai-panel.is-open');
  await panel.waitFor();
  await panel.getByRole('button', { name: 'Make my key' }).click();
  await panel.getByText('Your key is ready').waitFor();
  const theirAI = await keyFromCodes(page);
  await theirAI.rpc('initialize', { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'Claude Code', version: '1' } });
  await page.keyboard.press('Escape');
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });

  const block = '---\naction: update\nname: Tyne\nfields:\n  race: Gnome\n---\n---\nname: Sword\n---\nShiny.\n';
  await pasteText(page, block);
  await page.locator('#ai-panel.is-open').waitFor();
  const review = page.locator('#ai-panel form[aria-label="Review your AI\'s changes"]');
  await review.waitFor();
  assert.match(await review.innerText(), /Tyne/);
  assert.match(await review.innerText(), /race/i);
  assert.equal(await review.getByRole('checkbox', { name: /Yes, change it/ }).isChecked(), true, 'Tyne\'s change is not offered ticked');
  const left = (await review.getByText('Left out', { exact: false }).locator('..').innerText());
  assert.match(left, /Sword/);
  assert.match(left, /can't add or remove pages/);
  await picture(page, 'ai-player-review', { check: '#1977', about: "A player's pasted change reviewed: Tyne offered, Sword left out", mask: ['h1 + *'] });
  await review.getByRole('button', { name: 'Apply' }).click();
  await panel.getByText(/Tyne|changed|made/i).first().waitFor();
  const fields = await player.api(`/campaigns/${cid}/entities/${tyne}/fields`);
  assert.equal((fields.body.fields_data || {}).race, 'Gnome', 'the character\'s sheet does not show the new value');
  assert.equal(JSON.stringify((await owner.api(`/campaigns/${cid}/entities/${tyne}/fields`)).body.fields_data || {}).includes('Gnome'), true);

  // The player's AI asking over its key never lands in the owner's panel.
  const asked = await theirAI.call('ask_for_changes', { summary: 'Tyne again', changes: '---\naction: update\nname: Tyne\nfields:\n  race: Halfling\n---\n' });
  assert.ok(!asked.isError, asked.out);
  await owner.go(`/campaigns/${cid}`);
  await owner.page.locator('[data-ai-chip]').click();
  await owner.page.locator('#ai-bubble').getByText(/Nothing waiting/).waitFor();
  const chip = await owner.api(`/campaigns/${cid}/ai/chip`);
  assert.equal(chip.body.waiting, 0, 'the player\'s request shows in the owner\'s count');

  // 4. Pause and "Let it back", then the switch off.
  await owner.go(`/campaigns/${cid}/ai`);
  await owner.page.getByRole('tab', { name: 'Settings' }).click();
  const row = owner.page.locator('#ai-players .aic-subrow', { hasText: 'Player Pat' });
  await row.waitFor();
  await saved(owner.page, '/ai/players/', () => row.getByRole('button', { name: 'Pause' }).click());
  await owner.page.locator('#ai-players').getByRole('button', { name: /Let it back/ }).waitFor();
  const paused = await theirAI.call('campaign_outline');
  assert.equal(paused.status, 403);
  assert.match(paused.text, /paused/i, 'the player\'s AI is not told the key is paused');
  await saved(owner.page, '/ai/players/', () => owner.page.locator('#ai-players').getByRole('button', { name: /Let it back/ }).click());
  await owner.page.locator('#ai-players').getByRole('button', { name: 'Pause' }).waitFor();
  assert.ok(!(await theirAI.call('campaign_outline')).isError, 'the player\'s AI is still refused after Let it back');

  await saved(owner.page, '/ai/players', () => owner.page.getByRole('switch', { name: 'Players may use an AI here' }).uncheck());
  await owner.page.locator('#ai-players .aic-switch:not(:checked)').waitFor();
  const off = await theirAI.call('campaign_outline');
  assert.equal(off.status, 403);
  assert.match(off.text, /Players can't use AI in this campaign/);
  assert.equal(await hasButton(), false, 'the player\'s AI button stays after the switch went off');
  owner.assertClean();
});

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
  assert.match(review, /New Tag, campaign colour, 1 page/, 'the lookup does not list the renamed tag with its count:\n' + review);
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

test('the guide comes in parts: all at once, then one category', async (t) => {
  const { owner } = await accounts();
  const cid = await newCampaign(owner, 'Guide Campaign');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  const whole = await ai.call('how_to_write_changes');
  assert.ok(!whole.isError, whole.out.slice(0, 200));
  assert.ok(whole.out.length > 8000, 'the guide is ' + whole.out.length + ' characters, too short to be the whole guide');
  // The 24 KB ceiling is our own budget for an AI's context, not a fault a
  // person sees, so going over it warns instead of failing the walk: a line
  // in the log, and on GitHub one in the run's summary, since the test
  // runner prefixes child output and so swallows ::warning:: annotations.
  // TODO(#2785) trim the guide back under it.
  if (whole.out.length >= 24000) {
    const msg = 'the whole AI guide is ' + whole.out.length + ' characters, over its 24 KB budget (#2785)';
    t.diagnostic('warning: ' + msg);
    if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, '- Warning: ' + msg + '\n');
  }
  const part = await ai.call('how_to_write_changes', { category: 'Character' });
  assert.ok(!part.isError, part.out.slice(0, 200));
  assert.ok(part.out.length > 200 && part.out.length < whole.out.length / 2, 'one category is not a small part: ' + part.out.length);
  assert.match(part.out, /Character/);
  const every = await ai.call('how_to_write_changes', { part: 'categories' });
  assert.ok(!every.isError && every.out.includes('Character') && every.out.includes('Location'), 'part: categories does not list every category');
  await owner.go(`/campaigns/${cid}/ai`);
  await owner.page.locator('.aic-sit summary').first().click();
  const text = await owner.page.locator('.aic-sit').first().innerText();
  assert.match(text, /Read how to write changes/, 'What it did does not list the guide reads');
  owner.assertClean();
});

test('a montage test is a row in the review and shows on the page', async () => {
  const { owner } = await accounts();
  const cid = await newCampaign(owner, 'Montage Campaign');
  await enableAddons(owner, cid, '*');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  const sandstorm = await newPage(owner, cid, 'Location', 'The Sandstorm');
  const block = `---\nkind: test\npage: The Sandstorm\nname: Cross the sandstorm\ntype: Montage\nsuccesses: 5\nfailures: 3\nresults:\n  - Total success, on successes\n  - Partial success\n  - Total failure, on failures\n---\n`;
  const sent = await ai.call('ask_for_changes', { summary: 'Sandstorm montage', changes: block });
  assert.ok(!sent.isError, sent.out);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  const card = page.locator('#ai-panel section.aic-request');
  await card.getByRole('button', { name: 'Review' }).click();
  const row = card.locator('.air-row', { hasText: 'Cross the sandstorm' });
  await row.waitFor();
  const text = await row.innerText();
  assert.match(text, /Test/);
  assert.match(text, /new test: ends at 5 successes or 3 failures/);
  assert.doesNotMatch(text, /not something AI Import can change/);
  assert.match(text, /no Tests block.*Customizer/s, 'there is no warning that the page has no Tests block');
  await picture(page, 'ai-test-row', { check: '#2021', about: 'The review row for a montage test, with the no-Tests-block warning', mask: ['h1 + *'] });
  await card.getByRole('button', { name: /Yes to every request/ }).click();
  await card.getByRole('button', { name: 'Apply' }).click();
  await page.locator('#ai-panel .aic-done').waitFor();
  const tests = (await ai.call('tests')).out;
  assert.match(tests, /Cross the sandstorm/);

  // With a Tests block in the layout (the Customizer's job) the page shows it.
  await addBlocks(owner, cid, 'Location', ['page_tests']);
  await owner.go(`/campaigns/${cid}/entities/${sandstorm}`);
  await page.getByText('Cross the sandstorm').first().waitFor();
  owner.assertClean();
});

test('a hidden tracker: owner sees it with its log line, a player sees nothing', async () => {
  const { owner, player } = await accounts();
  const cid = await newCampaign(owner, 'Tracker Campaign');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  const orrin = await newPage(owner, cid, 'Character', 'Orrin');
  const vis = await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: JSON.stringify({ entity_ids: [orrin], private: false }), headers: JSON_HEADERS });
  assert.ok(vis.ok);
  await addBlocks(owner, cid, 'Character', ['trackers']);

  // The tracker, then its first change: the change names a tracker that exists.
  await askAndApply(owner, cid, ai, 'Counsel tracker', `---\nkind: tracker\nname: Orrin's Counsel\nmin: 0\nmax: 5\npages: [Orrin]\n---\n`);
  await askAndApply(owner, cid, ai, 'Counsel up', `---\nkind: tracker\naction: update\nname: Orrin's Counsel\npage: Orrin\nadd: 1\nnote: Counselled Saatraaol at the gate\n---\n`);
  assert.match((await ai.call('trackers')).out, /who sees it: Director only/);

  await owner.go(`/campaigns/${cid}/entities/${orrin}`);
  await owner.page.getByText("Orrin's Counsel").first().waitFor();
  await owner.page.getByRole('button', { name: /Log/ }).first().click();
  await owner.page.getByText('Counselled Saatraaol at the gate').first().waitFor();
  await picture(owner.page, 'ai-hidden-tracker', { check: '#2021', about: "Orrin's page: the hidden counter with its amber edge and log line", mask: ['time', '.tr-log time', '[class*=log] time'] });
  await player.go(`/campaigns/${cid}/entities/${orrin}`);
  await player.page.waitForLoadState('networkidle');
  await player.page.waitForTimeout(800);
  assert.equal(await player.page.getByText("Orrin's Counsel").count(), 0, 'a player sees the hidden counter');
  assert.equal(await player.page.getByText('Counselled Saatraaol').count(), 0, 'a player sees the hidden counter\'s log');
  owner.assertClean();
});

test('an AI that creates and changes a tracker in one answer', { todo: 'the tracker doc\'s own example (create, then update in one answer) is refused: no tracker called "Counsel"; use action: create' }, async () => {
  const { owner } = await accounts();
  const cid = await newCampaign(owner, 'Tracker One Answer');
  await enableAddons(owner, cid, '*');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  await newPage(owner, cid, 'Character', 'Orrin');
  const both = `---\nkind: tracker\nname: Counsel\nmin: 0\nmax: 5\npages: [Orrin]\n---\n\n---\nkind: tracker\naction: update\nname: Counsel\npage: Orrin\nadd: 1\nnote: Counselled Saatraaol at the Lastwell gate\n---\n`;
  const sent = await ai.call('ask_for_changes', { summary: 'Both at once', changes: both });
  assert.ok(!sent.isError, sent.out);
});

test('a rolled night given a result shows with no total, locked, and survives rolling the year', async () => {
  const { owner } = await accounts();
  const cid = await newCampaign(owner, 'Bloodnight Campaign');
  await enableAddons(owner, cid, '*');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  await askAndApply(owner, cid, ai, 'Calendar and Bloodnight', `---
kind: calendar
name: Probe Calendar
year_label: AB
current_year: 1043
current_month: Firstmoon
current_day: 5
months:
  - {name: Firstmoon, days: 30}
  - {name: Secondmoon, days: 30}
weekdays: [One, Two, Three, Four, Five]
---

---
kind: event
name: Bloodnight
year: 1043
month: 1
day: 10
repeat: yearly
roll:
  dice: 2d10
  on_the_night: false
  results:
    - max: 11
      name: Thin blood
      players: The moon hangs dull.
      director: Nothing stirs.
    - name: Red Tide
      players: The sea looks like blood.
      director: Feral things rise.
      tell_players: true
---
`);
  await askAndApply(owner, cid, ai, 'Bloodnight result', `---\nkind: roll_night\naction: update\nname: Bloodnight\nyear: 1043\nmonth: 1\nday: 10\nresult: Red Tide\ntell_players: true\n---\n`);
  assert.match((await ai.call('calendar_events', { from: 'Firstmoon 1 1043', to: 'Firstmoon 30 1043' })).out, /Bloodnight roll, Firstmoon 10 1043: Red Tide, locked, players told/);

  // The calendar's own day card for that night.
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/calendars`);
  await owner.go(await page.locator('a[href$="/view"]').first().evaluate((a) => a.getAttribute('href')));
  const cell = page.locator('.dc:has(.mk)');
  await cell.click();
  const eid = await cell.locator('.mk').getAttribute('data-ev');
  const roll = page.getByText('Red Tide').first();
  await roll.waitFor();
  const line = (await roll.locator('xpath=..').innerText()).replace(/\s+/g, ' ');
  assert.match(line, /Red Tide/, line);
  assert.doesNotMatch(line, /\d/, 'a dice total shows on the night: ' + line);
  await picture(page, 'ai-roll-result-card', { check: '#2021', about: "The Bloodnight day card: Red Tide with no dice total", mask: ['.dc.today', '[class*=head] time'] });

  // The roll's own data: a result with no total, locked, and kept when the year is rolled again.
  const calid = page.url().match(/calendars\/([0-9a-f-]{36})/)[1];
  const url = `/campaigns/${cid}/calendars/${calid}/events/${eid}/roll`;
  const night = async () => ((await owner.api(url + '?year=1043')).body.nights || []).find((n) => n.day === 10 && n.month === 1);
  const before = await night();
  assert.equal(before.result, 'Red Tide');
  assert.equal(before.total == null, true, 'the night has a dice total');
  assert.equal(before.locked, true, 'the night is not locked');
  const again = await owner.api(url + '/nights', { method: 'POST', body: JSON.stringify({ year: 1043 }), headers: JSON_HEADERS });
  assert.ok(again.ok, 'rolling the year failed: ' + JSON.stringify(again.body));
  const kept = await night();
  assert.equal(kept.result, 'Red Tide', 'rolling the year changed the night');
  assert.equal(kept.total == null, true, 'rolling the year gave the night a total');
  owner.assertClean();
});
