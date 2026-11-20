// calendar_year_track.spec.mjs — the Edit drawer's year track: an owner fills
// the rest of the year and keeps it, what is saved is director-only and marked
// as generated, a player sees none of it, and Undo takes it all away again.
//
// RUN: tools/live-checks.sh test/live/calendar_year_track.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

const listOf = (body) => (Array.isArray(body) ? body : (body && body.data) || []);

// Every event of the calendar's current year as `who` is allowed to read it.
async function yearEvents(who, base, year, months) {
  const out = [];
  for (let m = 1; m <= months; m++) {
    const r = await who.api(`${base}/events?year=${year}&month=${m}`);
    if (r.ok) out.push(...listOf(r.body));
  }
  return out;
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
  assert.equal(await page.locator('[data-deck-year]').isDisabled(), true, 'The year stays enabled while a draft exists');
  assert.equal((await yearEvents(owner, base, cal.current_year, months)).length, 0, 'the draft was saved before Keep');
  await picture(page, 'year-track-draft', { check: '#2222', about: 'Edit drawer with a fill draft', mask: ['header'] });

  await keep.click();
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
  owner.assertClean();
});
