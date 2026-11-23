// hero_rules_not_here.spec.mjs: an entry a Director marks Not here leaves the
// hero builder and creator for a player, and for a Director viewing as one.
//
// The walk installs the Draw Steel package (the server fetches it from
// GitHub; with GitHub out of reach the walk is skipped), marks the Time
// Raider ancestry and the Censor class Not here for everyone through the
// choice-rules API the Own entries page writes with, and checks:
//   - a player: the creator's Ancestry and Class steps, in Cards and in List,
//     leave the entry out, count one fewer to choose from and show
//     "1 not here"; the builder's panel on their hero does the same, and so
//     does the sheet's own picker list;
//   - the owner with "view as player" on: the same lists as the player;
//   - the owner in their own view: every entry, and no "not here" line.
//
// RUN: tools/live-checks.sh test/live/hero_rules_not_here.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, siteAdmin, newCampaign, addMember, enableAddons, done, picture, ADMIN } from './_live.mjs';

after(done);

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const j = (who, url, method, body) => who.api(url, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: JSON_HEADERS });

// The marked entries, by the step they are on.
const MARKED = { ancestry: { key: 'time-raider', name: 'Time Raider' }, class: { key: 'censor', name: 'Censor' } };

// The package is added once for the whole server; the admin must have
// confirmed their password within the last minutes first.
async function installDrawSteel() {
  const admin = await siteAdmin();
  await admin.go('/admin/packages');
  const reauth = await admin.api('/account/reauth', { method: 'POST', form: { password: ADMIN.password } });
  assert.ok(reauth.ok, 'the admin could not confirm their password: ' + JSON.stringify(reauth.body));
  const have = await admin.api('/admin/packages', { headers: { Accept: 'application/json' } });
  if (!/Chronicle-Draw-Steel/i.test(typeof have.body === 'string' ? have.body : JSON.stringify(have.body))) {
    const added = await admin.api('/admin/packages', { method: 'POST', form: { repo_url: 'https://github.com/keyxmakerx/Chronicle-Draw-Steel', type: 'system' } });
    const said = JSON.stringify(added.body).slice(0, 300);
    // GitHub out of reach or rate limited is the run's surroundings, not a
    // fault in the page: the walk is skipped and says why.
    if (!added.ok && /GitHub (didn't respond|had a temporary problem)|GitHub's rate limit/.test(said)) {
      await admin.ctx.close();
      return said;
    }
    assert.ok(added.ok, 'the Draw Steel package was not added: ' + said);
  }
  admin.assertClean();
  await admin.ctx.close();
  return '';
}

// A hero the person plays, made in the creator. A Director's hero is the
// table's until they take it, and the builder shows only heroes the viewer has.
async function heroFor(who, cid, name, take) {
  await who.go(`/campaigns/${cid}/characters/new`);
  const made = await j(who, `/campaigns/${cid}/characters/new`, 'POST', { name, picks: {} });
  assert.equal(made.status, 201, 'the hero was not made: ' + JSON.stringify(made.body));
  if (take) {
    await who.api(`/campaigns/${cid}/characters/playable`, { method: 'POST', form: { entity_id: made.body.id, playable: '1' } });
    const claim = await who.api(`/campaigns/${cid}/entities/${made.body.id}/claim`, { method: 'POST' });
    assert.ok(claim.ok, 'the owner could not take their hero: ' + JSON.stringify(claim.body));
  }
  return made.body.id;
}

// One creator step in one mode: the "N to choose from" count, the names on
// the cards or rows, and the foot line.
async function creatorStep(who, fieldKey, mode) {
  const { page } = who;
  await page.locator(`.hc-rail .hc-row[data-k="${fieldKey}"]`).click();
  await page.waitForFunction((k) => document.querySelector(`.hc-rail .hc-row[data-k="${k}"]`)?.getAttribute('aria-current') === 'step', fieldKey);
  await page.locator(`.hc-mode [data-mode="${mode}"]`).click();
  await page.locator(mode === 'cards' ? '.hc-gcard[data-i]' : '.hc-lrow[data-i]').first().waitFor();
  return page.evaluate((mode) => {
    const of = (document.querySelector('.hc-of') || {}).textContent || '';
    return {
      count: Number((of.match(/(\d+)\s+to choose from/i) || [])[1]),
      names: [...document.querySelectorAll(mode === 'cards' ? '.hc-gcard[data-i]' : '.hc-lrow[data-i]')].map((e) => e.textContent),
      foot: ((document.querySelector('.hc-nh [data-act="nothere"]') || {}).textContent || '').trim(),
    };
  }, mode);
}

async function creator(who, cid) {
  await who.go(`/campaigns/${cid}/characters/new`);
  await who.page.locator('.hc-app .hc-rail').waitFor();
  const seen = {};
  for (const k of Object.keys(MARKED)) {
    for (const mode of ['cards', 'list']) seen[k + ':' + mode] = await creatorStep(who, k, mode);
  }
  return seen;
}

// The builder's panel for one blank of the viewer's hero.
async function builder(who, cid) {
  const { page } = who;
  await who.go(`/campaigns/${cid}/characters`);
  const seen = {};
  for (const k of Object.keys(MARKED)) {
    await page.locator(`[data-widget="hero-builder"] [data-blank="${k}"]`).first().click();
    await page.locator('.hb-list .hb-opt').first().waitFor();
    seen[k] = await page.evaluate(() => ({
      names: [...document.querySelectorAll('.hb-list .hb-opt')].map((e) => e.textContent),
      foot: ((document.querySelector('.hb-nh [data-act="nothere"]') || {}).textContent || '').trim(),
    }));
    await page.locator('.hb-x').first().click();
  }
  return seen;
}

const shows = (names, name) => names.some((n) => n.includes(name));

function assertLimited(tag, full, c, b) {
  for (const [k, m] of Object.entries(MARKED)) {
    for (const mode of ['cards', 'list']) {
      const s = c[k + ':' + mode];
      assert.ok(!shows(s.names, m.name), `${tag}: ${m.name} is on the creator's ${k} step in ${mode}`);
      assert.equal(s.count, full[k] - 1, `${tag}: the creator's ${k} step counts ${s.count} to choose from in ${mode}`);
      assert.equal(s.names.length, s.count, `${tag}: the ${k} step's ${mode} do not match its count`);
      assert.equal(s.foot, '1 not here', `${tag}: no "1 not here" under the creator's ${k} step in ${mode}`);
    }
    assert.ok(!shows(b[k].names, m.name), `${tag}: ${m.name} is in the builder's ${k} panel`);
    assert.equal(b[k].names.length, full[k] - 1, `${tag}: the builder's ${k} panel lists ${b[k].names.length}`);
    assert.equal(b[k].foot, '1 not here', `${tag}: no "1 not here" under the builder's ${k} panel`);
  }
}

test('an entry marked Not here leaves the hero builder and creator for a player and a Director viewing as one', async (t) => {
  const unreachable = await installDrawSteel();
  if (unreachable) { t.skip('this server could not fetch the Draw Steel package: ' + unreachable); return; }

  const look = { context: { deviceScaleFactor: 1 } };
  const owner = await person({ tag: 'hnh-d', name: 'Director Dara', ...look });
  const pat = await person({ tag: 'hnh-p', name: 'Pat Player', ...look });
  const cid = await newCampaign(owner, 'Not here');
  await enableAddons(owner, cid, '*');
  await enableAddons(owner, cid, ['drawsteel']);
  await addMember(owner, cid, pat, 'player');

  // The whole list, as the owner's own tools read it, before any mark.
  const plan = (await owner.api(`/campaigns/${cid}/characters/new/plan`)).body;
  const full = {};
  for (const [k, m] of Object.entries(MARKED)) {
    const step = (plan.steps || []).find((s) => s.fieldKey === k);
    assert.ok(step && step.choices.some((c) => c.key === m.key), `the package has no ${m.name} on its ${k} step`);
    full[k] = step.choices.length;
  }

  // Not here for everyone, as the Own entries page saves it.
  await owner.go(`/campaigns/${cid}/characters/entries`);
  for (const [k, m] of Object.entries(MARKED)) {
    const put = await j(owner, `/campaigns/${cid}/choice-rules`, 'PUT', { fieldKey: k, marks: [{ source: 'package', key: m.key, target: '', state: 'not_offered' }] });
    assert.ok(put.ok, `marking ${m.name} Not here failed: ` + JSON.stringify(put.body));
  }

  // The player: the creator first, since a player who has a hero makes no
  // other; then the builder, on the hero they made.
  const patCreator = await creator(pat, cid);
  const patHero = await heroFor(pat, cid, 'Pat Hero', false);
  assertLimited('the player', full, patCreator, await builder(pat, cid));
  await heroFor(owner, cid, 'Dara Hero', true);
  const picks = await pat.api(`/campaigns/${cid}/character-choices/ancestry`);
  assert.ok(!picks.body.choices.some((c) => c.key === 'time-raider'), 'the sheet picker offers the player Time Raider');
  await pat.go(`/campaigns/${cid}/entities/${patHero}`);
  pat.assertClean();

  // The owner viewing as a player.
  await owner.go(`/campaigns/${cid}`);
  const on = await owner.api(`/campaigns/${cid}/toggle-view-mode`, { method: 'POST' });
  assert.ok(on.ok && (await owner.ctx.cookies()).some((c) => c.name === 'chronicle_view_as_player' && c.value === cid), 'view as player did not switch on');
  assertLimited('the owner viewing as a player', full, await creator(owner, cid), await builder(owner, cid));
  await owner.page.locator('[data-widget="hero-builder"] [data-blank="ancestry"]').first().click();
  await owner.page.locator('.hb-list .hb-opt').first().waitFor();
  await picture(owner.page, 'hero-rules-not-here-view-as', { check: 'hero_rules_not_here', about: 'The builder’s ancestry panel for a Director viewing as a player: Time Raider gone, 1 not here.' });
  const viewPicks = await owner.api(`/campaigns/${cid}/character-choices/ancestry`);
  assert.ok(!viewPicks.body.choices.some((c) => c.key === 'time-raider'), 'the sheet picker offers the owner viewing as a player Time Raider');

  // The owner in their own view: everything, and no foot line.
  const off = await owner.api(`/campaigns/${cid}/toggle-view-mode`, { method: 'POST' });
  assert.ok(off.ok && !(await owner.ctx.cookies()).some((c) => c.name === 'chronicle_view_as_player'), 'view as player did not switch off');
  const c = await creator(owner, cid);
  const b = await builder(owner, cid);
  for (const [k, m] of Object.entries(MARKED)) {
    for (const mode of ['cards', 'list']) {
      const s = c[k + ':' + mode];
      assert.ok(shows(s.names, m.name), `the owner: ${m.name} is missing from the creator's ${k} step in ${mode}`);
      assert.equal(s.count, full[k], `the owner: the creator's ${k} step counts ${s.count} in ${mode}`);
      assert.equal(s.foot, '', `the owner: a "not here" line under the creator's ${k} step in ${mode}`);
    }
    assert.ok(shows(b[k].names, m.name), `the owner: ${m.name} is missing from the builder's ${k} panel`);
    assert.equal(b[k].foot, '', `the owner: a "not here" line under the builder's ${k} panel`);
  }
  owner.assertClean();
});
