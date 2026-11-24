// workshop_page_tracker.spec.mjs — a Homebrew Workshop tracker part can use a
// tracker the campaign already keeps on pages instead of keeping its own.
// Opens the Workshop by address (no game system needed), switches a part to
// "A tracker on pages", saves, and checks the recipe keeps no tracker of its
// own and that Log a respite moves the page tracker.
//
// RUN: tools/live-checks.sh test/live/workshop_page_tracker.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture } from './_live.mjs';

after(done);

test('a tracker part uses a page tracker, and a respite moves it', async () => {
  const owner = await person({ tag: 'wpt', name: 'Workshop Owner' });
  const cid = await newCampaign(owner, 'Page tracker check');
  const { page } = owner;
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));

  const hero = await newPage(owner, cid, 'Character', 'Saatraaol');
  const kind = await owner.api(`/campaigns/${cid}/tracker-kinds`, { method: 'POST', body: {
    name: 'Respites since dose', shape: 'count', min: 0, max: 3, visibility: 'directors',
    bands: [{ at: 3, text: 'Restless' }] } });
  assert.equal(kind.status, 201, 'kind: ' + kind.status + ' ' + JSON.stringify(kind.body));
  const placed = await owner.api(`/campaigns/${cid}/trackers/page/${hero}`, { method: 'POST', body: { kindId: kind.body.id } });
  assert.ok(placed.ok, 'place: ' + placed.status + ' ' + JSON.stringify(placed.body));

  const made = await owner.api(`/campaigns/${cid}/homebrew/recipes`, { method: 'POST', body: { name: 'The Red Tide', doc: {
    parts: [{ key: 'respites', kind: 'tracker', label: 'Respites', see: 'director', data: { shape: 'count', min: 0, max: 5 } }],
    rules: [{ when: { event: 'respite.logged', args: {} }, timing: 'now', then: [{ action: 'step', part: 'respites', args: { by: 1 } }] }] } } });
  assert.equal(made.status, 201, 'recipe: ' + made.status + ' ' + JSON.stringify(made.body));

  await owner.go(`/campaigns/${cid}/homebrew`);
  await page.locator('.hbw-card[data-card="p:respites"]').click();
  await page.getByRole('button', { name: 'A tracker on pages' }).click();
  const pick = page.locator('select', { has: page.locator('option', { hasText: 'Respites since dose' }) });
  await pick.waitFor();
  await page.getByText('Rules step and read Respites since dose').waitFor();
  await picture(page, 'workshop-part-uses-page-tracker', { check: '#2813', about: 'A tracker part set to use "Respites since dose", a tracker kept on pages' });
  await page.getByRole('button', { name: 'Save recipe' }).click();
  await page.waitForResponse((r) => r.url().includes('/homebrew/recipes/') && r.request().method() === 'PUT');

  const stored = await owner.api(`/campaigns/${cid}/homebrew/recipes/${made.body.id}`);
  assert.equal(stored.body.doc.parts[0].data.link, 'Respites since dose', JSON.stringify(stored.body.doc.parts[0]));
  const kinds = await owner.api(`/campaigns/${cid}/tracker-kinds`);
  assert.deepEqual(kinds.body.kinds.map((k) => k.name), ['Respites since dose'], 'the recipe kept a tracker of its own');

  const pressed = await owner.api(`/campaigns/${cid}/homebrew/press/respite.logged`, { method: 'POST' });
  assert.ok(pressed.ok, 'press: ' + pressed.status);
  const trackers = await owner.api(`/campaigns/${cid}/trackers/page/${hero}`);
  const t = trackers.body.trackers.find((x) => x.name === 'Respites since dose');
  assert.equal(t && t.value, 1, 'the page tracker did not move: ' + JSON.stringify(trackers.body.trackers));
  assert.deepEqual(errors, []);
});
