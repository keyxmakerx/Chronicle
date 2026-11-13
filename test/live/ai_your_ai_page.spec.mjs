// ai_your_ai_page.spec.mjs — the Your AI page: pause and resume, changing how
// far the AI may go, "What it did", the copy-and-paste address, a request
// reviewed in place, reduced motion and tables reaching the AI. Stands in for
// the operator check in #1776.
//
// Covers: 1 (layout, as a picture and the status parts), 2 (Pause and Resume
// against the MCP endpoint), 3 (widening asks first, narrowing doesn't, and
// the AI's access follows), 4 (one sitting, times in the person's own zone),
// 5 (the #copy-and-paste address opens, Escape and a click outside close),
// 6 (review in place, Not now, Apply, "Made 2 of 2 changes."), 7 (reduced
// motion: the review's unfold is cut to nothing, and kept when motion is allowed), 8 (a table reaches the AI as rows and a header).
// Needs a person: "Show older sittings" (needs sittings 30 minutes apart, so
// real waiting or database access) and the hover peek on a real mouse.
// Note: the issue describes an older layout. The page is now tabs (What it
// did, Settings) with requests in the AI panel, the "Copy and paste" card is
// the AI button's tool sheet, and the old #copy-and-paste address still opens
// it; the checks use those equivalents.
//
// RUN: tools/live-checks.sh test/live/ai_your_ai_page.spec.mjs

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

/** A campaign with a key at `level` (and an AI that has said hello). */
async function setup(tag, level, name) {
  const owner = await person({ tag, name });
  const cid = await newCampaign(owner, name + "'s campaign");
  const ai = await makeKey(owner, cid, level);
  return { owner, cid, ai, page: owner.page };
}

test('layout: the status, the panel pointer and both tabs', async () => {
  const { owner, cid, page } = await setup('aipage1', LEVEL.ask, 'Layout Owner');
  await owner.go(`/campaigns/${cid}/ai`);
  const dot = page.locator('.aic-head .aic-dot');
  assert.equal(await dot.getAttribute('data-state'), 'connected');
  await page.getByRole('heading', { name: 'Claude Code is connected' }).waitFor();
  await page.locator('.aic-head').getByRole('button', { name: 'Pause' }).waitFor();
  await page.getByRole('tab', { name: 'What it did' }).waitFor();
  await page.getByRole('tab', { name: 'Settings' }).waitFor();
  await page.getByRole('button', { name: 'Open it' }).waitFor();
  await picture(page, 'ai-your-ai-page', { check: '#1776', about: 'Your AI page on a wide screen, connected', mask: ['.aic-head .aic-hint', '.aic-sit-when', '.aic-strip', '.aic-sit-what', 'h1 + *'] });
  owner.assertClean();
});

test('pause refuses the AI and resume brings it back', async () => {
  const { owner, cid, ai, page } = await setup('aipage2', LEVEL.everything, 'Pause Owner');
  assert.ok(!(await ai.call('campaign_outline')).isError, 'the AI cannot read before pausing');
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('.aic-head').getByRole('button', { name: 'Pause' }).click();
  await page.locator('.aic-head .aic-dot[data-state=paused]').waitFor();
  await page.locator('.aic-head').getByRole('button', { name: 'Resume' }).waitFor();
  const paused = await ai.call('campaign_outline');
  assert.equal(paused.status, 403);
  assert.match(paused.text, /paused/i, 'the AI is not told the key is paused');
  await picture(page, 'ai-your-ai-paused', { check: '#1776', about: 'Your AI page while the key is paused (amber dot)', mask: ['.aic-head .aic-hint', '.aic-sit-when', '.aic-strip', '.aic-sit-what', '.aic-head + p', 'h1 + *'] });
  await page.locator('.aic-head').getByRole('button', { name: 'Resume' }).click();
  await page.locator('.aic-head .aic-dot[data-state=connected]').waitFor();
  const back = await ai.call('campaign_outline');
  assert.ok(!back.isError, 'the AI is still refused after Resume: ' + back.out);
  owner.assertClean();
});

test('widening what the AI sees asks first; narrowing does not', async () => {
  const { owner, cid, ai, page } = await setup('aipage3', LEVEL.players, 'Access Owner');
  const secret = await newPage(owner, cid, 'Character', 'Hidden Hanna');
  const hide = await owner.api(`/campaigns/${cid}/entities/bulk-visibility`, { method: 'POST', body: JSON.stringify({ entity_ids: [secret], private: true }), headers: { 'Content-Type': 'application/json' } });
  assert.ok(hide.ok, 'could not hide the page: ' + JSON.stringify(hide.body));
  const sees = async () => (await ai.call('list_pages')).out.includes('Hidden Hanna');
  assert.equal(await sees(), false, 'a players-level key sees a hidden page');

  await owner.go(`/campaigns/${cid}/ai`);
  await page.getByRole('tab', { name: 'Settings' }).click();
  await page.getByRole('radio', { name: LEVEL.everything }).click();
  const ask = page.locator('.aic-ask[role=alert]');
  await ask.waitFor();
  assert.match(await ask.innerText(), /sees your .*-only pages and secrets/);
  assert.equal(await sees(), false, 'widening took effect before the owner confirmed');
  await picture(page, 'ai-your-ai-widen-ask', { check: '#1776', about: 'The "Let it see what you see" question before widening', mask: ['.aic-head .aic-hint', '.aic-row-k .aic-hint', '.aic-sit-when'], locator: page.locator('.aic-levels') });
  await ask.getByRole('button', { name: 'Let it see what you see' }).click();
  await page.getByText(/Saved\. Works from its next call/).waitFor();
  assert.equal(await sees(), true, 'the AI did not get the wider access after Save');

  await page.getByRole('tab', { name: 'Settings' }).click();
  await page.getByRole('radio', { name: LEVEL.players }).click();
  await page.getByText(/Saved\. Works from its next call/).waitFor();
  assert.equal(await page.locator('.aic-ask[role=alert]').isVisible(), false, 'narrowing asked for a confirmation');
  assert.equal(await sees(), false, 'the AI kept the old access after narrowing');
  owner.assertClean();
});

test('What it did: one sitting for a burst of calls, in the person\'s own time zone', async () => {
  const { owner, cid, ai, page } = await setup('aipage4', LEVEL.everything, 'History Owner');
  const zone = 'Pacific/Auckland';
  const set = await owner.api('/account/timezone', { method: 'PUT', body: JSON.stringify({ timezone: zone }), headers: { 'Content-Type': 'application/json' } });
  assert.ok(set.ok, 'could not set the time zone');
  for (const t of ['campaign_outline', 'list_pages', 'list_tags', 'players', 'house_rules']) await ai.call(t);
  await owner.go(`/campaigns/${cid}/ai`);
  assert.equal(await page.locator('.aic-sit').count(), 1, 'a burst of calls is not one sitting');
  await page.locator('.aic-sit summary').click();
  const rows = page.locator('.aic-sit [data-call]');
  assert.ok(await rows.count() >= 5, 'not every call is listed under the sitting');
  const t = rows.first().locator('time');
  const utc = new Date(await t.getAttribute('datetime'));
  const want = new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit', timeZone: zone }).format(utc);
  assert.equal((await t.innerText()).replace(/\s/g, ' ').toLowerCase(), want.replace(/\s/g, ' ').toLowerCase().replace(/^0/, ''), 'the time is not shown in the person\'s zone');
  owner.assertClean();
});

test('the #copy-and-paste address opens the tool over the page', async () => {
  const { owner, cid, page } = await setup('aipage5', LEVEL.ask, 'Paste Owner');
  await owner.go(`/campaigns/${cid}/ai`);
  await owner.go(`/campaigns/${cid}`);
  await owner.go(`/campaigns/${cid}/ai#copy-and-paste`);
  await page.locator('#ai-panel.is-open').waitFor();
  await page.getByRole('heading', { name: 'Write a request' }).waitFor();
  await picture(page, 'ai-copy-and-paste-open', { check: '#1776', about: 'Copy and paste open over the Your AI page', mask: ['.aic-head .aic-hint', '.aic-sit-when', 'h1 + *'] });
  await page.keyboard.press('Escape');
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });
  assert.ok(!page.url().includes('#copy-and-paste'), 'the address still ends in #copy-and-paste after closing');
  // A reload of the address opens it again, and a click outside closes it.
  await owner.go(`/campaigns/${cid}`);
  await owner.go(`/campaigns/${cid}/ai#copy-and-paste`);
  await page.locator('#ai-panel.is-open').waitFor();
  await page.locator('.ai-panel-scrim').click({ position: { x: 20, y: 300 } });
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });
  owner.assertClean();
});

test('a review opens in the card and applies without leaving the page; reduced motion shortens it', async () => {
  const { owner, cid, ai, page } = await setup('aipage6', LEVEL.ask, 'Review Owner');
  const sent = await ai.call('ask_for_changes', { summary: 'A page and a table', changes: table('Inn names', 3) + `\n---\nname: Mara Vell\ntype: Character\n---\nShe waits at the inn.\n` });
  assert.ok(!sent.isError, sent.out);
  await owner.go(`/campaigns/${cid}/ai`);
  await page.evaluate(() => { window.__stayed = true; });
  const url = page.url();
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  const card = page.locator('#ai-panel section.aic-request');
  await card.getByRole('button', { name: 'Review' }).click();
  await card.getByText('Add the page Mara Vell').waitFor();
  assert.equal(await card.locator('.aic-req-actions').first().isVisible(), false, 'the card\'s own buttons stay while reviewing');
  assert.equal(page.url(), url, 'review navigated');
  assert.ok(await rowDuration(card) <= 0.001, 'the review still animates with reduced motion');
  // Pointing at a page row shows the start of its text beside it.
  const pageRow = card.locator('.air-row', { hasText: 'Mara Vell' }).first();
  await pageRow.hover();
  await page.waitForTimeout(300);
  assert.equal(await pageRow.locator('.air-peek').evaluate((e) => getComputedStyle(e).opacity), '1', 'the text peek does not show on hover');
  // Not now folds back.
  await card.getByRole('button', { name: /Close the review/ }).click();
  await card.getByRole('button', { name: 'Review' }).waitFor();
  await card.getByRole('button', { name: 'Review' }).click();
  await card.getByText('Add the page Mara Vell').waitFor();
  await card.getByRole('button', { name: 'Apply' }).click();
  await page.locator('#ai-panel .aic-done').waitFor();
  assert.match(await page.locator('#ai-panel .aic-done-line').innerText(), /Made 2 of 2 changes\./);
  assert.equal(await card.count(), 0, 'the card stays after Apply');
  assert.equal(page.url(), url, 'Apply navigated away');
  assert.equal(await page.evaluate(() => window.__stayed === true), true, 'Apply reloaded the page');
  await page.getByText('See what changed').click();
  await page.locator('#ai-panel .aic-done-more').getByText('Mara Vell').first().waitFor();
  owner.assertClean();
});

/** Length in seconds of a review row's entrance, as the page computes it. */
const rowDuration = (card) => card.locator('.aic-req-review .air-row').first().evaluate((f) => parseFloat(getComputedStyle(f).animationDuration));

test('the review keeps its unfold when the person allows motion', async () => {
  const owner = await person({ tag: 'aipage7', name: 'Motion Owner', context: { reducedMotion: 'no-preference' } });
  const cid = await newCampaign(owner, "Motion Owner's campaign");
  const ai = await makeKey(owner, cid, LEVEL.ask);
  assert.ok(!(await ai.call('ask_for_changes', { summary: 'One table', changes: table('Motion names', 2) })).isError);
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  const card = page.locator('#ai-panel section.aic-request');
  await card.getByRole('button', { name: 'Review' }).click();
  await card.getByText('Motion names').first().waitFor();
  assert.ok(await rowDuration(card) >= 0.2, 'the review rows do not animate even though motion is allowed');
});

test('a table reaches the AI with a header row and one row per line', async () => {
  const { owner, cid, ai, page } = await setup('aipage8', LEVEL.ask, 'Table Owner');
  const body = '| Door | Guard |\n|---|---|\n| North | Mara |\n| South | Dan |\n';
  const sent = await ai.call('ask_for_changes', { summary: 'The Way In', changes: `---\nname: The Way In\ntype: Location\n---\n${body}` });
  assert.ok(!sent.isError, sent.out);
  await owner.go(`/campaigns/${cid}/ai`);
  await page.locator('[data-ai-open]').first().click();
  await page.locator('#ai-panel.is-open').waitFor();
  const card = page.locator('#ai-panel section.aic-request');
  await card.getByRole('button', { name: 'Review' }).click();
  await card.getByRole('button', { name: 'Apply' }).click();
  await page.locator('#ai-panel .aic-done').waitFor();
  const read = await ai.call('read_page', { name: 'The Way In' });
  assert.ok(!read.isError, read.out);
  const lines = read.out.split('\n').map((l) => l.trim());
  assert.ok(lines.some((l) => /^\|\s*Door\s*\|\s*Guard\s*\|$/.test(l)), 'the header row is not on its own line:\n' + read.out);
  assert.ok(lines.some((l) => /^\|\s*:?-{2,}:?\s*\|\s*:?-{2,}:?\s*\|$/.test(l)), 'there is no header separator line:\n' + read.out);
  assert.ok(lines.some((l) => /^\|\s*North\s*\|\s*Mara\s*\|$/.test(l)), 'a body row runs together:\n' + read.out);
  owner.assertClean();
});
