// hero_about_step.spec.mjs — the hero creator's optional About your hero step.
//
// When the hero's page type has pronouns, gender, looks or a backstory, the
// creator asks for them in a step of their own before Name and review. Every
// box may stay empty, a GM-only field is never asked, what the player writes
// lands on the hero, and the player can change it afterwards.
//
// RUN: tools/live-checks.sh test/live/hero_about_step.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done, picture } from './_live.mjs';

after(done);

test('About your hero: asked before Name and review, saved on the hero, editable later', async () => {
  const owner = await person({ tag: 'about-owner' });
  const player = await person({ tag: 'about-player' });
  const cid = await newCampaign(owner, 'About your hero');
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, player, 'player');

  // The character page types gain the About fields, one of them GM-only, as
  // a system package's character preset would give them. A PUT without
  // fields changes nothing and answers with the type as it stands.
  const list = await owner.api(`/campaigns/${cid}/entity-types`, { headers: { 'HX-Request': 'true' } });
  const ids = [...new Set((String(list.body).match(/entity-types\/\d+/g) || []).map((m) => m.split('/')[1]))];
  let changed = 0;
  for (const id of ids) {
    const t = await owner.api(`/campaigns/${cid}/entity-types/${id}`, { method: 'PUT', body: {} });
    if (!t.ok || !/character/.test(t.body.slug || '')) continue;
    const had = t.body.fields || [];
    const add = [
      { key: 'pronouns', label: 'Pronouns', type: 'text' },
      { key: 'gender', label: 'Gender', type: 'text', gm_only: true },
      { key: 'appearance', label: 'What they look like', type: 'textarea' },
      { key: 'backstory', label: 'Backstory', type: 'textarea' },
    ].filter((f) => !had.some((h) => h.key === f.key));
    const put = await owner.api(`/campaigns/${cid}/entity-types/${id}`, { method: 'PUT', body: { fields: had.concat(add) } });
    assert.ok(put.ok, 'type update answered ' + put.status);
    changed++;
  }
  assert.ok(changed >= 1, 'a character type to give the fields to');

  const refused = await owner.api(`/campaigns/${cid}/characters/new`, { method: 'POST', body: { name: 'Nope', about: { level: '20' } } });
  assert.equal(refused.status, 400, 'a box the step does not offer is refused');
  const hidden = await owner.api(`/campaigns/${cid}/characters/new`, { method: 'POST', body: { name: 'Nope', about: { gender: 'x' } } });
  assert.equal(hidden.status, 400, 'a GM-only field is not offered, so it is refused');

  const { page } = player;
  await player.go(`/campaigns/${cid}/characters/new`);
  await page.locator('.hc-app').waitFor();
  await page.locator('.hc-rail .hc-row', { hasText: 'About your hero' }).click();
  await page.locator('[data-about-step]').waitFor();
  assert.equal(await page.locator('[data-about="gender"]').count(), 0, 'the GM-only field is not asked');
  await page.fill('[data-about="pronouns"]', 'she/her');
  await page.fill('[data-about="appearance"]', 'Tall, an ash-grey braid.');
  await picture(page, 'hero-about-step');
  await page.click('[data-about-step] [data-act="next"]');
  await page.fill('[data-name]', 'Wren');
  await Promise.all([page.waitForURL(/\/entities\//), page.click('[data-act="create"]')]);
  const eid = page.url().match(/entities\/([^/?#]+)/)[1];

  const got = (await player.api(`/campaigns/${cid}/entities/${eid}/fields`)).body.fields_data || {};
  assert.equal(got.pronouns, 'she/her');
  assert.equal(got.appearance, 'Tall, an ash-grey braid.');
  assert.ok(!got.backstory, 'an empty box writes nothing');

  const later = await player.api(`/campaigns/${cid}/entities/${eid}/fields`, { method: 'PUT', body: { fields_patch: { pronouns: 'they/them', backstory: 'Raised on the coast.' } } });
  assert.ok(later.ok, 'the player changes it later; answered ' + later.status + ' ' + JSON.stringify(later.body));
  const after2 = (await player.api(`/campaigns/${cid}/entities/${eid}/fields`)).body.fields_data || {};
  assert.equal(after2.pronouns, 'they/them');
  assert.equal(after2.backstory, 'Raised on the coast.');
  player.assertClean();
});
