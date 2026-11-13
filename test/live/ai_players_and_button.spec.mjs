// ai_players_and_button.spec.mjs — the AI button, players' AI that is off until
// the owner turns it on, a player's own key that changes only their own
// character, and the owner's Pause and off switch. Stands in for the operator
// check in #1977.
//
// Covers: 1 (the button, the bubble and side panel, Escape, Ctrl+V paste that
// opens a review and doesn't when typing in a field), 2 (no button for a
// player until the switch is on), 3 (the player's key, a paste reviewed with
// Tyne's change offered and a new page left out, Apply writes the sheet, the
// owner's panel shows none of it), 4 (Pause and "Let it back", the switch
// off: the button goes and the AI is told).
// Needs a person: nothing. The player's paste box is reached the way Ctrl+V
// reaches it: a paste event with the text on the page.
//
// RUN: tools/live-checks.sh test/live/ai_players_and_button.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person as signedIn, newCampaign, newPage, addMember, enableAddons, done, picture, BASE } from './_live.mjs';

after(done);

/** person() with a retry: sign-up occasionally lands before its redirect settles. */
async function person(opts) {
  for (let i = 0; ; i++) {
    try { return await signedIn(opts); } catch (e) { if (i >= 2 || !/could not (register|sign in)/.test(e.message)) throw e; }
  }
}

const LEVEL = {};
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

test('the AI button: bubble, side panel, Escape and Ctrl+V', async () => {
  const owner = await person({ tag: 'aibtn', name: 'Button Owner' });
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
  const owner = await person({ tag: 'aiown', name: 'Owner Olive' });
  const player = await person({ tag: 'aiplr', name: 'Player Pat' });
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
  await sw.check();
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
  const asked = await theirAI.call('ask_for_changes', { summary: 'Tyne again', changes: '---\naction: update\nname: Tyne\nfields:\n  background: Hermit\n---\n' });
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
  await row.getByRole('button', { name: 'Pause' }).click();
  await owner.page.locator('#ai-players').getByRole('button', { name: /Let it back/ }).waitFor();
  const paused = await theirAI.call('campaign_outline');
  assert.equal(paused.status, 403);
  assert.match(paused.text, /paused/i, 'the player\'s AI is not told the key is paused');
  await owner.page.locator('#ai-players').getByRole('button', { name: /Let it back/ }).click();
  await owner.page.locator('#ai-players').getByRole('button', { name: 'Pause' }).waitFor();
  assert.ok(!(await theirAI.call('campaign_outline')).isError, 'the player\'s AI is still refused after Let it back');

  await owner.page.getByRole('switch', { name: 'Players may use an AI here' }).uncheck();
  await owner.page.locator('#ai-players .aic-switch:not(:checked)').waitFor();
  const off = await theirAI.call('campaign_outline');
  assert.equal(off.status, 403);
  assert.match(off.text, /Players can't use AI in this campaign/);
  assert.equal(await hasButton(), false, 'the player\'s AI button stays after the switch went off');
  owner.assertClean();
});
