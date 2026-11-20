// calendar_year_track.spec.mjs — the Edit drawer's year track: an owner fills
// the rest of the year and keeps it, what is saved is director-only and marked
// as generated, a player sees none of it, Undo takes it all away again, and a
// second fill replaces the first instead of stacking on it.
//
// RUN: tools/live-checks.sh test/live/calendar_year_track.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

const listOf = (body) => (Array.isArray(body) ? body : (body && body.data) || []);

// This check writes several hundred events inside a minute, which is more
// than the server lets one person write. The page then waits, as it would
// for anyone, so the later steps allow for one such wait.
const LONG = { timeout: 150000 };

// Every event of the calendar's current year as `who` is allowed to read it.
async function yearEvents(who, base, year, months) {
  // A repeating event is listed under every month it lands in, so each is counted once.
  const byId = new Map();
  for (let m = 1; m <= months; m++) {
    const r = await who.api(`${base}/events?year=${year}&month=${m}`);
    if (r.ok) listOf(r.body).forEach((e) => byId.set(e.id, e));
  }
  return [...byId.values()];
}

test('fill the rest of the year, keep it, see it director-only, and undo it', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const player = await person({ tag: 'ply', name: 'Tobin Hale' });
  const cid = await newCampaign(owner, 'Year Track');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, '*');
  const made = await owner.api(`/campaigns/${cid}/calendars/presets/harptos`, { method: 'POST', body: {} });
  assert.equal(made.status, 201);
  const cal = made.body.calendar;
  const base = `/campaigns/${cid}/calendars/${cal.id}`;
  const months = (cal.months || made.body.months || []).length || 16;
  const { page } = owner;

  await owner.go(`${base}/view`);
  const utc = page.getByRole('button', { name: 'Use UTC time' });
  if (await utc.count()) { await utc.first().click(); await owner.go(`${base}/view`); }
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'the new calendar already has events');

  await page.locator('#cal5-edittab').click();
  const yearBtn = page.locator('[data-deck-year]');
  await yearBtn.waitFor();
  assert.equal(await yearBtn.getAttribute('aria-expanded'), 'false');
  await yearBtn.click();
  assert.equal(await yearBtn.getAttribute('aria-expanded'), 'true');

  await page.getByRole('button', { name: 'Fill the rest of the year' }).click();
  const keep = page.locator('[data-yt="keep"]');
  await keep.waitFor();
  assert.match(await page.locator('.yt-barwrap').innerText(), /Nothing is saved until you keep it\./);
  assert.equal(await page.locator('[data-deck-year]').isDisabled(), true, 'The year can still be pressed while a draft exists');
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'the draft was saved before Keep');
  await picture(page, 'year-track-draft', { check: '#2222', about: 'Edit drawer with a fill draft', mask: ['header'] });

  // Keep is done when the draft bar has gone. An Undo button alone does not
  // say so: the one from an earlier Keep can still be on screen.
  const refusedWrites = [];
  page.on('response', (r) => {
    const q = r.request();
    if (q.method() !== 'GET' && r.status() >= 400) refusedWrites.push(`${q.method()} ${new URL(r.url()).pathname.replace(base, '')} ${r.status()}`);
  });
  const keptAll = async () => {
    const live = page.locator('#cal5-live');
    const done = keep.waitFor({ state: 'detached', ...LONG }).then(() => false);
    const stopped = live.filter({ hasText: /could not|Nothing could be saved|The rest are still a draft/ }).waitFor(LONG).then(() => true, () => false);
    if (await Promise.race([done, stopped])) {
      assert.fail(`Keep stopped part way: "${await live.innerText()}"; refused writes: ${[...new Set(refusedWrites.map((w) => w.replace(/[0-9a-f-]{36}/g, ':id')))].join(', ') || 'none'}`);
    }
    await live.filter({ hasText: /^Kept/ }).waitFor(LONG);
  };
  await keep.click();
  await keptAll();
  const undo = page.getByRole('button', { name: 'Undo' });
  await undo.waitFor();
  assert.match(await page.locator('body').innerText(), /Kept \d+ events?/);

  const kept = await yearEvents(owner, base, cal.current_year, months);
  assert.ok(kept.length > 0, 'Keep saved no events');
  for (const e of kept) {
    assert.equal(e.visibility, 'dm_only', `${e.name} was saved visible to players`);
    const p = JSON.parse(e.payload);
    assert.equal(p.type, 'generated');
    assert.ok(['events', 'sky'].includes(p.lane), `unexpected lane ${p.lane}`);
    assert.ok(p.sig, 'no sig');
  }
  const seen = await yearEvents(player, base, cal.current_year, months);
  assert.equal(seen.length, 0, 'a player can read generated events');

  await undo.click();
  await page.locator('#cal5-live').filter({ hasText: 'Undone.' }).waitFor();
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'Undo left events behind');

  // A second fill replaces the first one's events instead of stacking on them.
  const fill = page.getByRole('button', { name: 'Fill the rest of the year' });
  await fill.click();
  await keep.waitFor();
  await keep.click();
  await keptAll();
  const first = await yearEvents(owner, base, cal.current_year, months);
  assert.ok(first.length > 0, 'the first fill saved no events');
  await fill.click();
  await keep.waitFor();
  const said = await page.locator('.yt-barwrap').innerText();
  const m = /It replaces (\d+) events? from an earlier fill\./.exec(said);
  assert.ok(m, `the second fill does not say what it replaces: ${said}`);
  assert.equal(Number(m[1]), first.length, 'the second fill does not replace all of the first');
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, first.length, 'the second draft changed what is saved before Keep');
  await keep.click();
  await keptAll();
  const second = await yearEvents(owner, base, cal.current_year, months);
  const firstIds = new Set(first.map((e) => e.id));
  assert.equal(second.filter((e) => firstIds.has(e.id)).length, 0, 'events of the first fill are still there after the second');
  assert.ok(second.length > 0, 'the second fill saved no events');

  // Undoing the second fill puts the first back, still director-only and still marked.
  await undo.click();
  await page.locator('#cal5-live').filter({ hasText: 'Undone.' }).waitFor(LONG);
  const back = await yearEvents(owner, base, cal.current_year, months);
  assert.equal(back.length, first.length, 'Undo did not put the first fill back');
  for (const e of back) {
    assert.equal(e.visibility, 'dm_only', `${e.name} came back visible to players`);
    assert.equal(JSON.parse(e.payload).type, 'generated');
  }
  assert.equal((await yearEvents(player, base, cal.current_year, months)).length, 0, 'a player can read generated events after Undo');
  owner.assertClean();
});
