// timeline_river.spec.mjs — the River time line draws coloured dots and kind
// chips, opens an event on the right, folds the calendar's month under the
// line, closes the month then the event on Escape, and hides Director-only
// dots (and the arcs that lead to them) from players. Stands in for the
// operator check in #1463.
//
// Covers steps 1 and 2 with a real player account. Dragging to move and
// scrolling to zoom are driven with the mouse wheel and a drag on the stage
// and checked by the date range changing. "View as player" is not used.
//
// RUN: tools/live-checks.sh test/live/timeline_river.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

const PUBLIC_EVENTS = [
  { name: 'The Gate Falls', year: 1520, month: 2, day: 4, category: 'Battle' },
  { name: 'The Long Winter', year: 1521, month: 5, day: 12, category: 'Weather' },
  { name: 'Crowning of Aldra', year: 1522, month: 9, day: 20, category: 'Politics' },
];
const SECRET = { name: 'The Poisoned Cup', year: 1521, month: 8, day: 1, category: 'Politics', visibility: 'dm_only' };

async function build() {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'River Check');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const made = await owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} });
  assert.equal(made.status, 201);
  const calendarId = made.body.calendar.id;
  await owner.go(`/campaigns/${cid}/timelines`);
  const url = await owner.page.evaluate(async ({ cid, calendarId }) => {
    const b = new FormData(); b.append('name', 'Fall of the Gate'); b.append('calendar_id', calendarId); b.append('visibility', 'everyone');
    return (await Chronicle.apiFetch(`/campaigns/${cid}/timelines`, { method: 'POST', body: b })).url;
  }, { cid, calendarId });
  const tid = (url.match(/timelines\/([0-9a-f-]{36})/) || [])[1];
  assert.ok(tid, 'the time line was not created');
  const ids = {};
  for (const e of [...PUBLIC_EVENTS, SECRET]) {
    const res = await owner.api(`/campaigns/${cid}/timelines/${tid}/standalone-events`, { method: 'POST', body: { visibility: 'everyone', ...e } });
    assert.equal(res.status, 201, `creating ${e.name} answered ${res.status}`);
    ids[e.name] = res.body.id;
  }
  const link = (a, b) => owner.api(`/campaigns/${cid}/timelines/${tid}/connections`, { method: 'POST', body: { source_id: ids[a], target_id: ids[b], source_type: 'standalone', target_type: 'standalone' } });
  for (const [a, b] of [['The Gate Falls', 'The Long Winter'], ['The Long Winter', 'The Poisoned Cup'], ['The Poisoned Cup', 'Crowning of Aldra']]) {
    const r = await link(a, b);
    assert.ok(r.ok, `linking ${a} to ${b} answered ${r.status}`);
  }
  return { owner, player, cid, tid };
}

let world;
const open = async (who) => {
  await who.go(`/campaigns/${world.cid}/timelines/${world.tid}`);
  await who.page.locator('.tlr .ev').first().waitFor();
};
const dots = (page) => page.locator('.tlr .ev');
const stableMask = ['.tlr-range', '.tlr-monthlink', '.t-today'];

test('the Director sees every dot, the kind chips, and the side panel and month fold work', async () => {
  world = await build();
  const { owner } = world;
  const { page } = owner;
  await open(owner);

  assert.equal(await dots(page).count(), 4, 'one dot per event, the secret one included');
  assert.equal(await page.locator('.tlr-empty').count(), 0, 'an empty card instead of the line');
  await page.locator('[data-x="menu"]').click();
  assert.ok(await page.locator('.tlr-kind').count() >= 3, 'no kind chips');
  await page.locator('[data-x="menu"]').click();
  await picture(page, 'timeline-river-director', { check: '#1463', about: 'River time line as the Director: dots, arcs, one Director-only', mask: stableMask });

  // Zoom with the wheel and move with a drag: the visible range text changes.
  const range = page.locator('[data-tlr="range"]');
  const before = await range.innerText();
  const box = await page.locator('[data-tlr="stage"]').boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, -600);
  await page.waitForFunction((t) => document.querySelector('[data-tlr="range"]').textContent !== t, before);
  const zoomed = await range.innerText();
  await page.mouse.down(); await page.mouse.move(box.x + box.width / 2 - 250, box.y + box.height / 2, { steps: 8 }); await page.mouse.up();
  await page.waitForFunction((t) => document.querySelector('[data-tlr="range"]').textContent !== t, zoomed);

  // Clicking a dot opens it on the right.
  await page.locator('[data-x="menu"]').click();
  await page.locator('[data-fit="all"]').click();
  await page.keyboard.press('Escape');
  await page.locator('.tlr .ev[data-id]').filter({ has: page.locator('.core') }).first().focus();
  await page.keyboard.press('Enter');
  const side = page.locator('[data-tlr="side"]');
  await side.locator('h2').waitFor();
  assert.ok((await side.locator('h2').innerText()).length > 0);

  // The month link folds the calendar's month down; Escape closes the month
  // first, then the event.
  const monthlink = page.locator('[data-tlr="monthlink"]');
  await monthlink.waitFor({ state: 'visible' });
  assert.match(await monthlink.innerText(), /^Show .+ on the calendar$/);
  await monthlink.click();
  await page.locator('[data-tlr="cal"] .day').first().waitFor();
  assert.equal(await monthlink.getAttribute('aria-expanded'), 'true');
  await picture(page, 'timeline-river-month-fold', { check: '#1463', about: 'An event open on the right with the calendar month folded under the line', mask: stableMask });
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => document.querySelector('[data-tlr="monthlink"]').getAttribute('aria-expanded') === 'false');
  assert.ok(await side.locator('h2').count(), 'Escape closed the event along with the month');
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => !document.querySelector('[data-tlr="side"] h2'));
  owner.assertClean();
});

test('a player sees no Director-only dots and no arcs that lead to one', async () => {
  const { player, owner } = world;
  await open(owner);
  const ownerArcs = await owner.page.locator('.tlr .arc').count();
  assert.equal(ownerArcs, 3, 'the Director should see all three arcs');

  await open(player);
  const { page } = player;
  assert.equal(await dots(page).count(), 3, 'the player sees a Director-only dot');
  const names = await dots(page).evaluateAll((els) => els.map((e) => e.getAttribute('aria-label')));
  assert.ok(names.every((n) => !/Poisoned Cup|Director only/.test(n)), 'a Director-only event is named for a player: ' + names.join(' | '));
  assert.equal(await page.locator('.tlr .arc').count(), 1, 'an arc leads to a dot the player cannot see');
  assert.equal(await page.locator('.tlr .arc.dm').count(), 0);
  assert.doesNotMatch(await page.locator('.tlr').innerText(), /Poisoned Cup|Director only/);
  // The data the line draws from carries nothing of the secret event either.
  const data = await player.api(`/campaigns/${world.cid}/timelines/${world.tid}/data`);
  assert.doesNotMatch(JSON.stringify(data.body), /Poisoned Cup/);
  await picture(page, 'timeline-river-player', { check: '#1463', about: 'River time line as a player: no Director-only dot, no dangling arc', mask: stableMask });
  player.assertClean();
});
