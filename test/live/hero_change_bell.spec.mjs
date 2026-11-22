// hero_change_bell.spec.mjs — the Director's bell for a player changing their hero.
//
// A player changes their pronouns twice and their backstory once. The owner
// has one unread bell for that hero, with the first old value and the newest
// one, and it never quotes the backstory. After the owner reads it, the next
// change is a new bell. Only a change counts: filling a blank rings nothing.
// The scratch site has no game system, so no builder picks are driven here.
//
// RUN: DB_NAME=chronicle_live_bell LIVE_PORT=18095 LIVE_SMOKE=0 tools/live-checks.sh test/live/hero_change_bell.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done } from './_live.mjs';

after(done);

test('Changing heroes: one unread bell per hero, folded; a read bell starts a new one', async () => {
  const owner = await person({ tag: 'hb-owner' });
  const player = await person({ tag: 'hb-player' });
  const cid = await newCampaign(owner, 'Bell for heroes');
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, player, 'player');

  // Give the character page types the fields a game system's preset would.
  const list = await owner.api(`/campaigns/${cid}/entity-types`, { headers: { 'HX-Request': 'true' } });
  const ids = [...new Set((String(list.body).match(/entity-types\/\d+/g) || []).map((m) => m.split('/')[1]))];
  let changed = 0;
  for (const id of ids) {
    const t = await owner.api(`/campaigns/${cid}/entity-types/${id}`, { method: 'PUT', body: {} });
    if (!t.ok || !/character/.test(t.body.slug || '')) continue;
    const had = t.body.fields || [];
    const add = [
      { key: 'pronouns', label: 'Pronouns', type: 'text' },
      { key: 'backstory', label: 'Backstory', type: 'textarea' },
    ].filter((f) => !had.some((h) => h.key === f.key));
    const put = await owner.api(`/campaigns/${cid}/entity-types/${id}`, { method: 'PUT', body: { fields: had.concat(add) } });
    assert.ok(put.ok, 'type update answered ' + put.status);
    changed++;
  }
  assert.ok(changed >= 1, 'a character type to give the fields to');

  const made = await player.api(`/campaigns/${cid}/characters/new`, { method: 'POST', body: { name: 'Wren', about: { pronouns: 'she/her' } } });
  assert.ok(made.ok, 'player makes a hero; answered ' + made.status + ' ' + JSON.stringify(made.body));
  const eid = made.body.id;
  const fields = `/campaigns/${cid}/entities/${eid}/fields`;
  const change = async (patch) => {
    const r = await player.api(fields, { method: 'PUT', body: { fields_patch: patch } });
    assert.ok(r.ok, 'the player saves ' + JSON.stringify(patch) + '; answered ' + r.status + ' ' + JSON.stringify(r.body));
  };
  const bells = async () => (await owner.api('/notifications')).body.filter((n) => n.type === 'hero_changed');

  // Filling a blank backstory rings nothing; making the hero rang nothing.
  await change({ backstory: 'Raised on the coast.' });
  assert.equal((await bells()).length, 0, 'filling a blank rang the bell');

  await change({ pronouns: 'they/them' });
  await change({ pronouns: 'he/him' });
  await change({ backstory: 'Raised by wolves, far from the coast.' });
  let rows = await bells();
  assert.equal(rows.length, 1, 'the owner should have exactly one bell for the hero, got ' + rows.length);
  assert.match(rows[0].message, / changed Wren$/);
  assert.equal(rows[0].detail, 'Pronouns: she/her → he/him; Backstory rewritten');
  assert.ok(!JSON.stringify(rows[0]).includes('wolves'), 'the bell quotes the backstory');
  assert.ok(!JSON.stringify(rows[0]).includes('"state"') && !JSON.stringify(rows[0]).includes('fields'), 'the bell serves its kept state');
  assert.ok(rows[0].link.endsWith(`/entities/${eid}`));
  assert.equal(rows[0].read, false);

  // Putting a pick back where it began drops it out of the bell.
  await change({ pronouns: 'she/her' });
  rows = await bells();
  assert.equal(rows.length, 1);
  assert.equal(rows[0].detail, 'Backstory rewritten');

  // The player never gets a bell for their own change.
  assert.equal((await player.api('/notifications')).body.filter((n) => n.type === 'hero_changed').length, 0);

  // Read: the next change is a new bell.
  const read = await owner.api(`/notifications/${rows[0].id}/read`, { method: 'POST' });
  assert.ok(read.ok, 'marking read answered ' + read.status);
  await change({ pronouns: 'they/them' });
  rows = await bells();
  assert.equal(rows.length, 2, 'a read bell must start a new one, got ' + rows.length);
  const fresh = rows.filter((r) => !r.read);
  assert.equal(fresh.length, 1);
  assert.equal(fresh[0].detail, 'Pronouns: she/her → they/them');

  owner.assertClean();
  player.assertClean();
});
