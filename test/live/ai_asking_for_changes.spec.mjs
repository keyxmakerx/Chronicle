// ai_asking_for_changes.spec.mjs — a connected AI can ask for changes that
// wait for the owner's yes, and only a key made with "ask" can. Stands in for
// the operator check in #1766.
//
// Covers: 1 (the picker's asking level and the ChatGPT note; the default is now the safest level, not asking), 2 (ask over MCP, the
// waiting card, "What it did"), 3 (review one request, Yes and No rows,
// Apply), 4 (Not now, Turn down), 5 (a removal starts on No and needs its own
// tick), 6 (an Only look key has no ask tool), 7 (Disconnect keeps what is
// waiting).
// Needs a person: nothing. The step texts about Claude Code are replaced by
// the same JSON-RPC calls an AI app makes; what a real AI says is out of scope.
// Note: the issue's wording predates the current page. Requests are answered
// in the AI panel (the AI button), the key level is a "How far may your AI
// go?" picker, and there is no "Change" fieldset; the equivalents are used.
//
// RUN: tools/live-checks.sh test/live/ai_asking_for_changes.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person as signedIn, newCampaign, newPage, done, picture, BASE } from './_live.mjs';

after(done);

/** person() with a retry: sign-up occasionally lands before its redirect settles. */
async function person(opts) {
  for (let i = 0; ; i++) {
    try { return await signedIn(opts); } catch (e) { if (i >= 2 || !/could not (register|sign in)/.test(e.message)) throw e; }
  }
}

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

async function openPanel(page) {
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  return page.locator('#ai-panel');
}

const table = (name, n = 6) => `---\nkind: table\nname: ${name}\n---\n` + Array.from({ length: n }, (_, i) => `- ${name} ${i + 1}`).join('\n') + '\n';
const campaignTables = async (ai) => (await ai.call('rolling_tables')).out;

test('asking for changes: key, request, review, apply, turn down, removals, only look, disconnect', async () => {
  const owner = await person({ tag: 'aiask', name: 'Ask Owner' });
  const cid = await newCampaign(owner, 'Ask For Changes');
  const { page } = owner;

  // 1. The connect form offers a level that asks, and says what ChatGPT can't do.
  await owner.go(`/campaigns/${cid}/ai`);
  await page.getByRole('radio', { name: LEVEL.ask }).waitFor();
  // The picker now starts on the safest level, not on asking; asking is one pick away.
  assert.equal(await page.getByRole('radio', { name: LEVEL.players }).getAttribute('aria-checked'), 'true');
  assert.ok(await page.getByText(/ChatGPT Plus and Pro only let connectors read/).first().isVisible(), 'the ChatGPT note is missing');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  await owner.go(`/campaigns/${cid}/ai`);
  await page.getByText('Claude Code is connected').waitFor();
  await page.getByRole('tab', { name: 'Settings' }).click();
  assert.match(await page.locator('.aic-row-k .aic-hint').first().innerText(), /asks for changes/, 'the status line does not say "asks for changes"');

  // 2. The AI asks; nothing changes yet; the card, and What it did, say so.
  const tools = (await ai.rpc('tools/list', {})).json.result.tools.map((t) => t.name);
  assert.ok(tools.includes('ask_for_changes'), 'ask_for_changes is not offered');
  const sent = await ai.call('ask_for_changes', { summary: 'Six tavern names', changes: table('Tavern names') });
  assert.ok(!sent.isError, sent.out);
  assert.match(sent.out, /Nothing has changed yet/);
  assert.match(await campaignTables(ai), /no rolling tables|None|0/i, 'the table exists before the owner said yes');
  assert.doesNotMatch(await campaignTables(ai), /Tavern names/, 'the table exists before the owner said yes');
  await owner.go(`/campaigns/${cid}/ai`);
  let panel = await openPanel(page);
  await panel.getByRole('heading', { name: 'Claude Code asks for 1 change' }).waitFor();
  const card = panel.locator('section.aic-request');
  assert.match(await card.innerText(), /“Six tavern names”.*1 to add/s);
  await picture(page, 'ai-asking-waiting-card', { check: '#1766', about: 'The Waiting for you card in the AI panel', mask: ['.aic-request .aic-hint'], hide: ['#ai-panel + *'] });
  await page.keyboard.press('Escape');
  await page.getByRole('tab', { name: 'What it did' }).click();
  await page.getByText(/asked for changes once/).first().waitFor();

  // 3. Review, read what a page writes, say Yes to some and No to others.
  const two = await ai.call('ask_for_changes', { summary: 'A page and a table', changes: table('Bar names', 4) + `\n---\nname: Mara Vell\ntype: Character\n---\nShe waits at the bar.\n` });
  assert.ok(!two.isError, two.out);
  await owner.go(`/campaigns/${cid}/ai`);
  panel = await openPanel(page);
  const cardTwo = panel.locator('section.aic-request', { hasText: 'A page and a table' });
  await cardTwo.getByRole('button', { name: 'Review' }).click();
  await cardTwo.getByText('Add the page Mara Vell').waitFor();
  await cardTwo.getByText('Read what it writes').click();
  await cardTwo.getByText('She waits at the bar.').first().waitFor();
  await cardTwo.getByRole('radiogroup', { name: /Bar names/ }).locator('label.air-n').click();
  await cardTwo.getByRole('button', { name: 'Apply' }).click();
  await panel.locator('.aic-done').waitFor();
  await panel.getByText('See what changed').waitFor();
  assert.equal(await cardTwo.count(), 0, 'the card stays after Apply');
  await owner.go(`/campaigns/${cid}/entities`);
  assert.ok(await page.getByText('Mara Vell').first().isVisible(), 'the Yes page was not made');
  const after3 = await campaignTables(ai);
  assert.doesNotMatch(after3, /Bar names/, 'a No row was applied');

  // 4. Not now keeps it waiting; Turn down removes it and changes nothing.
  await owner.go(`/campaigns/${cid}/ai`);
  panel = await openPanel(page);
  const first = panel.locator('section.aic-request', { hasText: 'Six tavern names' });
  await first.getByRole('button', { name: 'Review' }).click();
  await first.getByRole('button', { name: /Close the review/ }).click();
  await first.getByRole('button', { name: 'Review' }).waitFor();
  assert.ok(await first.isVisible(), 'Not now removed the card');
  await first.getByRole('button', { name: 'Turn down' }).first().click();
  await first.getByRole('button', { name: 'Turn down' }).last().click();
  await first.waitFor({ state: 'detached' });
  assert.doesNotMatch(await campaignTables(ai), /Tavern names/, 'turning down still made the table');

  // 5. A removal starts on No and needs its own tick.
  const doomed = await newPage(owner, cid, 'Character', 'Doomed Dan');
  assert.ok(doomed);
  const rm = await ai.call('ask_for_changes', { summary: 'Remove Dan', changes: `---\naction: delete\nname: Doomed Dan\ntype: Character\nreason: not needed\n---\n` });
  assert.ok(!rm.isError, rm.out);
  await owner.go(`/campaigns/${cid}/ai`);
  panel = await openPanel(page);
  const rmCard = panel.locator('section.aic-request', { hasText: 'Remove Dan' });
  await rmCard.getByRole('button', { name: 'Review' }).click();
  const row = rmCard.getByRole('radiogroup', { name: /Doomed Dan/ });
  await row.waitFor();
  assert.equal(await row.locator('label.air-n input').isChecked(), true, 'the removal row does not start on No');
  await row.locator('label.air-y').click();
  await rmCard.locator('[data-air-confirm]').waitFor();
  assert.equal(await rmCard.locator('[data-air-confirm]').isChecked(), false, 'the removal tick starts ticked');
  assert.equal(await rmCard.locator('[data-air-confirm]').evaluate((e) => e.required), true, 'the removal tick is not required');
  await rmCard.getByRole('button', { name: /Close the review/ }).click();
  await rmCard.getByRole('button', { name: 'Turn down' }).first().click();
  await rmCard.getByRole('button', { name: 'Turn down' }).last().click();
  await rmCard.waitFor({ state: 'detached' });

  // 7 (before 6, so the key still works): Disconnect keeps what is waiting.
  const keep = await ai.call('ask_for_changes', { summary: 'Left for later', changes: table('Left tables', 2) });
  assert.ok(!keep.isError, keep.out);
  await owner.go(`/campaigns/${cid}/ai`);
  await page.getByRole('tab', { name: 'Settings' }).click();
  await page.getByRole('button', { name: 'Disconnect', exact: true }).first().click();
  await page.getByText('What it already asked for stays in the AI panel for you to answer.').waitFor();
  await page.locator('.aic-ask').getByRole('button', { name: 'Disconnect' }).click();
  await page.getByText('No AI connected yet').waitFor();
  panel = await openPanel(page);
  const kept = panel.locator('section.aic-request', { hasText: 'Left for later' });
  await kept.waitFor(); // fails with a timeout if the card vanished with the key
  await kept.getByRole('button', { name: 'Review' }).click();
  await kept.getByRole('button', { name: /Close the review/ }).waitFor();
  await kept.getByRole('button', { name: /Close the review/ }).click();
  await kept.getByRole('button', { name: 'Turn down' }).first().click();
  await kept.getByRole('button', { name: 'Turn down' }).last().click();
  await kept.waitFor({ state: 'detached' });
  assert.equal((await ai.call('campaign_outline')).status, 401, 'a disconnected key still works');

  // 6. An Only look key can look and cannot ask.
  const look = await makeKey(owner, cid, LEVEL.everything);
  const lookTools = (await look.rpc('tools/list', {})).json.result.tools.map((t) => t.name);
  assert.ok(!lookTools.includes('ask_for_changes'), 'an Only look key is offered ask_for_changes');
  const refused = await look.call('ask_for_changes', { summary: 'Nope', changes: table('Never', 2) });
  assert.ok(refused.isError || refused.status >= 400, 'an Only look key was allowed to ask');
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  assert.equal(await page.locator('#ai-panel section.aic-request').count(), 0, 'a request is waiting from an Only look key');
  owner.assertClean();
});
