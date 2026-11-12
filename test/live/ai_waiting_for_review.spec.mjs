// ai_waiting_for_review.spec.mjs — the AI's read-only "Waiting for review"
// lookup. Stands in for the operator check in #1837.
//
// Covers: 1 (the three lists; the AI's own waiting request, then what the
// owner applied and turned down, and the owner-only rule), 3 (the AI has no
// way to approve: no tool for it, and the lookup says approving happens in
// Chronicle).
// Needs a person: 2 (a player's unaccepted draft marked as a draft in the
// ancestry/kit list needs an installed game-system package), and the filled
// lists of players' handed-in entries and stash or buy requests, which need
// that package and a shop with a player buying.
//
// RUN: tools/live-checks.sh test/live/ai_waiting_for_review.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person as signedIn, newCampaign, done } from './_live.mjs';

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


const table = (name, n = 3) => `---\nkind: table\nname: ${name}\n---\n` + Array.from({ length: n }, (_, i) => `- ${name} ${i + 1}`).join('\n') + '\n';

test('the lookup lists what waits, what was answered, and cannot approve anything', async () => {
  const owner = await person({ tag: 'aiwait', name: 'Waiting Owner' });
  const cid = await newCampaign(owner, 'Waiting For Review');
  const { page } = owner;
  const ai = await makeKey(owner, cid, LEVEL.ask);
  const tools = (await ai.rpc('tools/list', {})).json.result.tools.map((t) => t.name);
  assert.ok(tools.includes('waiting_for_review'), 'the AI is not offered waiting_for_review');
  assert.ok(!tools.some((n) => /approve|accept|apply|turn_down/.test(n)), 'the AI is offered a way to approve: ' + tools.join(','));

  // 1. Three lists, empty at first.
  const empty = (await ai.call('waiting_for_review')).out;
  assert.match(empty, /### Changes your AI asked for, waiting on the owner \(0\)/);
  assert.match(empty, /### Entries players wrote \(0 handed in, 0 still being written\)/);
  assert.match(empty, /### Players' stash moves and requests to buy \(0\)/);

  // A request waits, one is applied, one is turned down.
  for (const [summary, name] of [['Keep me waiting', 'Waiting names'], ['Apply me', 'Applied names'], ['Turn me down', 'Refused names']]) {
    const r = await ai.call('ask_for_changes', { summary, changes: table(name) });
    assert.ok(!r.isError, r.out);
  }
  const waiting = (await ai.call('waiting_for_review')).out;
  assert.match(waiting, /waiting on the owner \(3\)/);
  for (const s of ['Keep me waiting', 'Apply me', 'Turn me down']) assert.ok(waiting.includes(s), s + ' is not listed');

  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  const card = (t) => page.locator('#ai-panel section.aic-request', { hasText: t });
  await card('Apply me').getByRole('button', { name: 'Review' }).click();
  await card('Apply me').getByRole('button', { name: 'Apply' }).click();
  await page.locator('#ai-panel .aic-done').waitFor();
  await card('Turn me down').getByRole('button', { name: 'Turn down' }).first().click();
  await card('Turn me down').getByRole('button', { name: 'Turn down' }).last().click();
  await card('Turn me down').waitFor({ state: 'detached' });

  const after = (await ai.call('waiting_for_review')).out;
  assert.match(after, /waiting on the owner \(1\)/);
  assert.match(after, /Keep me waiting/);
  assert.match(after, /### Your AI's requests the owner answered lately/);
  assert.match(after, /"?Apply me"?.*applied/);
  assert.match(after, /"?Turn me down"?.*turned down by the owner, nothing changed/);

  // 3. Approving is not something the lookup does; it says so.
  assert.match(after, /Only a Director can say yes or no to these, in Chronicle/);
  assert.match(after, /Nothing here can be accepted or turned down from a lookup/);
  const approve = await ai.call('approve', { name: 'Keep me waiting' });
  assert.ok(approve.isError, 'the AI could approve something');

  // The queue is the owner's business: a key that only sees what players see
  // is told it is left out.
  const safe = await makeKey(owner, await newCampaign(owner, 'Waiting Safe'), LEVEL.players);
  const refused = await safe.call('waiting_for_review');
  assert.match(refused.out, /left out in Safe privacy/, 'a players-level key was answered: ' + refused.out);
  owner.assertClean();
});
