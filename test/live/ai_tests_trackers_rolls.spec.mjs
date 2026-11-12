// ai_tests_trackers_rolls.spec.mjs — what the AI can set up and read: its
// instructions split in parts, a montage test, a hidden tracker with a logged
// change, and a rolled night given a result. Stands in for the operator check
// in #2021 (the AI's tests, hidden trackers, rolled-night results and split
// guide).
//
// Covers: 1 (the guide whole, then one category's fields, in What it did),
// 2 (a Test row in the review, the warning when the page has no Tests block,
// the test on the page once the Tests block is there), 3 (a hidden tracker
// and a logged change, shown to the owner, not to a player), 4 (a result for
// a rolled night: shown with no dice total and locked, and rolling the year
// again leaves it).
// Needs a person: 5 (the Draw Steel stat block needs the installed Draw Steel
// package), and the category named Creature in step 1 (it comes with that
// package; the same call is made with Character here).
//
// RUN: tools/live-checks.sh test/live/ai_tests_trackers_rolls.spec.mjs

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

test('the guide comes in parts: all at once, then one category', async () => {
  const owner = await person({ tag: 'ai21a', name: 'Guide Owner' });
  const cid = await newCampaign(owner, 'Guide Campaign');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  const whole = await ai.call('how_to_write_changes');
  assert.ok(!whole.isError, whole.out.slice(0, 200));
  assert.ok(whole.out.length > 8000 && whole.out.length < 24000, 'the guide is ' + whole.out.length + ' characters, not about 16 KB');
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
  const owner = await person({ tag: 'ai21b', name: 'Test Owner' });
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
  const owner = await person({ tag: 'ai21c', name: 'Tracker Owner' });
  const player = await person({ tag: 'ai21p', name: 'Tracker Player' });
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
  assert.match((await ai.call('trackers')).out, /hidden from players/);

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
  const owner = await person({ tag: 'ai21d', name: 'Tracker Owner' });
  const cid = await newCampaign(owner, 'Tracker One Answer');
  await enableAddons(owner, cid, '*');
  const ai = await makeKey(owner, cid, LEVEL.ask);
  await newPage(owner, cid, 'Character', 'Orrin');
  const both = `---\nkind: tracker\nname: Counsel\nmin: 0\nmax: 5\npages: [Orrin]\n---\n\n---\nkind: tracker\naction: update\nname: Counsel\npage: Orrin\nadd: 1\nnote: Counselled Saatraaol at the Lastwell gate\n---\n`;
  const sent = await ai.call('ask_for_changes', { summary: 'Both at once', changes: both });
  assert.ok(!sent.isError, sent.out);
});

test('a rolled night given a result shows with no total, locked, and survives rolling the year', async () => {
  const owner = await person({ tag: 'ai21e', name: 'Roll Owner' });
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
