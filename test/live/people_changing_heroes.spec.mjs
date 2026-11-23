// people_changing_heroes.spec.mjs — Campaign settings › People holds
// "Changing heroes", when players may change their own hero's picks: the
// owner sets the times there, the Characters page reads the same rule, a
// refused rule says why and keeps what was typed, and nobody but the owner
// can reach it.
//
// Set LIVE_SHOTS to a folder to also save pictures of the section there,
// light and dark, on a desktop and a phone.
//
// RUN: tools/live-checks.sh test/live/people_changing_heroes.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { join } from 'node:path';
import { person, newCampaign, addMember, done, BASE } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';

// Saves the section's form and waits for its redraw.
async function save(page) {
  await Promise.all([
    page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith('/members/hero-changes')),
    page.locator('#hero-changes button[type="submit"]').click(),
  ]);
  await page.locator('#hero-changes').waitFor();
}

test('the owner sets when players may change their hero on People, and the Characters page agrees', async () => {
  const owner = await person({ tag: 'heroes-owner', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Saltreach docks');
  const page = owner.page;

  await owner.go(`/campaigns/${cid}/members`);
  const section = page.locator('#hero-changes');
  await section.waitFor();
  assert.equal((await section.locator('h2').textContent()).trim(), 'Changing heroes');
  assert.match(await section.locator('.sb-hc-sum').textContent(), /any time/);
  assert.equal(await section.getByText('go to the Characters page').count(), 0, 'no rule, no hero to reopen');

  await section.getByLabel('Only at these times').check();
  await section.locator('#sb-hc-making').check();
  await section.getByLabel('Days after the hero is made').fill('5');
  await save(page);
  assert.equal(await section.locator('.sb-hc-sum').textContent(), "Players can change their hero's picks for the first 5 days after making it.");
  assert.equal((await section.locator('[role="status"]').textContent()).trim(), 'Saved.');
  assert.equal(await section.getByText('go to the Characters page').count(), 1);

  await owner.go(`/campaigns/${cid}/members`);
  assert.ok(await page.getByLabel('Only at these times').isChecked(), 'the rule is kept');
  assert.equal(await page.getByLabel('Days after the hero is made').inputValue(), '5');

  await owner.go(`/campaigns/${cid}/characters`);
  assert.match(await page.locator('[data-hc-summary]').textContent(), /for the first 5 days after making it/);

  // Any time again clears it.
  await owner.go(`/campaigns/${cid}/members`);
  await page.getByLabel('Any time').check();
  await save(page);
  assert.match(await page.locator('#hero-changes .sb-hc-sum').textContent(), /any time/);
  owner.assertClean();
});

test('a refused rule says how to fix it and keeps what was typed', async () => {
  const owner = await person({ tag: 'heroes-refuse', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Gull Rock');
  const page = owner.page;
  await owner.go(`/campaigns/${cid}/members`);
  const section = page.locator('#hero-changes');
  await section.getByLabel('Only at these times').check();
  await section.locator('#sb-hc-level').check();
  await section.getByLabel('First level that opens the picks').fill('4');
  await section.getByLabel('Days after a new level').fill('0');
  // The browser's own number check would stop a 0 first; the server's answer
  // is what this check is about.
  await page.evaluate(() => document.querySelector('#hero-changes form').setAttribute('novalidate', ''));
  await save(page);
  assert.equal((await section.locator('[role="alert"]').textContent()).trim(), 'Say how many days a new level keeps the picks open.');
  assert.equal(await section.getByLabel('First level that opens the picks').inputValue(), '4');
  assert.match(await section.locator('.sb-hc-sum').textContent(), /any time/, 'nothing was stored');
  owner.assertClean();
});

test('only the owner reaches it', async () => {
  const owner = await person({ tag: 'heroes-gate', name: 'Owner Ines' });
  const scribe = await person({ tag: 'heroes-scribe', name: 'Scribe Oren' });
  const player = await person({ tag: 'heroes-player', name: 'Player Pell' });
  const cid = await newCampaign(owner, 'Low Quay');
  await addMember(owner, cid, scribe, 'scribe');
  await addMember(owner, cid, player, 'player');
  for (const who of [scribe, player]) {
    const res = await who.api(`/campaigns/${cid}/members/hero-changes`, {
      method: 'POST', form: { when: 'set', after_making: '1', making_days: '3' }, headers: { 'HX-Request': 'true' },
    });
    assert.ok(res.status === 403 || res.status === 404, `${who.name} answered ${res.status}`);
  }
  await owner.go(`/campaigns/${cid}/members`);
  assert.match(await owner.page.locator('#hero-changes .sb-hc-sum').textContent(), /any time/, 'nothing was stored');
});

test('pictures of the section', { skip: !SHOTS }, async () => {
  const owner = await person({ tag: 'heroes-shots', name: 'Owner Ines' });
  const cid = await newCampaign(owner, 'Saltreach docks');
  const page = owner.page;
  await owner.go(`/campaigns/${cid}/members`);
  await page.getByLabel('Only at these times').check();
  await page.locator('#sb-hc-making').check();
  await page.locator('#sb-hc-level').check();
  await save(page);
  for (const [view, size] of [['desktop', { width: 1280, height: 1000 }], ['phone', { width: 390, height: 844 }]]) {
    for (const scheme of ['light', 'dark']) {
      await page.setViewportSize(size);
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'reduce' });
      await page.goto(`${BASE}/campaigns/${cid}/members`, { waitUntil: 'networkidle' });
      await page.evaluate((s) => {
        document.documentElement.classList.toggle('dark', s === 'dark');
        try { localStorage.setItem('theme', s); } catch (e) { /* none */ }
      }, scheme);
      await page.waitForTimeout(200);
      await page.screenshot({ path: join(SHOTS, `people-${view}-${scheme}.png`), fullPage: true });
      await page.locator('#hero-changes').screenshot({ path: join(SHOTS, `hero-changes-${view}-${scheme}.png`) });
    }
  }
});
