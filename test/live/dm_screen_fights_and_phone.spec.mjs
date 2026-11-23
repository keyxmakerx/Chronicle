// dm_screen_fights_and_phone.spec.mjs — the DM Screen's Fight card lists
// every fight, and on a phone the screen is a stack of named lines.
//
// The owner makes two Adventures pages, one with a Ready fight and one with a
// fight still being planned. The Fight card shows the Ready one first with
// Start and the other under "Still being planned"; "Plan a fight" asks which
// adventure; on a phone the eight sections are one named line each and a tap
// unfolds one in place.
//
// RUN: tools/live-checks.sh test/live/dm_screen_fights_and_phone.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, enableAddons, done, picture } from './_live.mjs';

after(done);

const PHONE = { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true };
const DESK = { viewport: { width: 1280, height: 800 } };
const LINES = ['The world', 'Fight', 'The party', 'Card decks', 'Reveal', 'Notes', 'Adventures', 'Homebrew'];

async function theme(page, dark) {
  await page.emulateMedia({ colorScheme: dark ? 'dark' : 'light' });
  await page.evaluate((d) => document.documentElement.classList.toggle('dark', d), dark);
  await page.waitForTimeout(100);
}

// Both looks of the current state, named <feature>-<state>-<light|dark>-<phone|desk>.
async function pictures(page, state, size, opts = {}) {
  for (const dark of [false, true]) {
    await theme(page, dark);
    await picture(page, `dm-screen-${state}-${dark ? 'dark' : 'light'}-${size}`, { about: opts.about || state, ...opts });
  }
  await theme(page, false);
}

async function setup(who, name) {
  const cid = await newCampaign(who, name);
  await enableAddons(who, cid, '*');
  const cellar = await newPage(who, cid, 'Adventure', 'Clear the cellar');
  const road = await newPage(who, cid, 'Adventure', 'The Shatter Road');
  const rats = (n) => Array.from({ length: n }, (_, i) => ({ id: 'r' + i, entityId: '', name: 'Rat ' + i, count: 1, group: 'A', start: 'seen' }));
  const make = (eid, body) => who.api(`/campaigns/${cid}/pages/${eid}/encounters`, { method: 'POST', body: { kind: 'fight', heroes: [], stages: [], ...body } });
  const a = await make(cellar, { name: 'Cellar ambush', ready: true, creatures: rats(6) });
  const b = await make(road, { name: 'Toll bridge', ready: false, creatures: [] });
  assert.equal(a.status, 201, 'the Ready fight was not made');
  assert.equal(b.status, 201, 'the planned fight was not made');
  return { cid, cellar, road };
}

test('the Fight card lists every fight, Ready first with Start, and Plan a fight asks which adventure', async () => {
  const owner = await person({ tag: 'dmf', name: 'Mara Vell', context: DESK });
  const { cid } = await setup(owner, 'Saltreach fights');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-dms-toggle]').click();
  await page.locator('.dms-dock').waitFor({ state: 'visible' });
  await page.locator('[data-dms-open="fight"]').click();
  const card = page.locator('[data-dms-sec="fight"]');
  await card.locator('.erun-none .erun-pick').first().waitFor();

  // Ready on top with its adventure and foe count and a Start button.
  const labels = await card.locator('.erun-lbl').allInnerTexts();
  assert.deepEqual(labels.map((t) => t.toLowerCase()), ['ready to run', 'still being planned']);
  const ready = card.locator('.erun-pick').nth(0);
  assert.match(await ready.innerText(), /Cellar ambush\s+Clear the cellar · 6 foes/);
  assert.equal(await ready.getByRole('button', { name: /^Start/ }).count(), 1);
  const planned = card.locator('.erun-pick').nth(1);
  assert.match(await planned.innerText(), /Toll bridge\s+The Shatter Road · no foes yet/);
  assert.equal(await planned.getByRole('button', { name: /^Start/ }).count(), 0, 'a fight still being planned has no Start');
  await pictures(page, 'fight-card', 'desk', { about: 'The Fight card with both groups', locator: page.locator('.dms-dock') });

  // Plan a fight asks which adventure; the adventures, then a new one.
  await card.getByRole('button', { name: 'Plan a fight' }).click();
  await card.getByText('For which adventure?').waitFor();
  await card.locator('.erun-choice').first().waitFor();
  const choices = await card.locator('.erun-choice').allInnerTexts();
  assert.ok(choices.includes('Clear the cellar') && choices.includes('The Shatter Road'), 'the adventures are not offered');
  assert.equal(choices[choices.length - 1], 'A new adventure…');
  const newHref = await card.getByRole('link', { name: 'A new adventure…' }).getAttribute('href');
  assert.match(newHref, new RegExp(`/campaigns/${cid}/entities/new\\?type=\\d+`));
  await pictures(page, 'plan-chooser', 'desk', { about: 'The "For which adventure?" chooser', locator: page.locator('.dms-dock') });

  // Choosing an adventure opens its page with the builder on a new fight.
  await card.locator('.erun-choice', { hasText: 'Clear the cellar' }).click();
  await page.locator('.dms-sheet').waitFor();
  await page.locator('.enc-drawer[role=dialog]').waitFor();
  assert.equal(await page.locator('.enc-drawer .enc-name').inputValue(), 'New encounter');
  await page.keyboard.press('Escape');

  // Tapping a fight opens it on its page.
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-dms-sec="fight"] .erun-open').first().waitFor({ state: 'attached' });
  await page.evaluate(() => { const b = document.querySelector('[data-dms-open="fight"]'); if (b.getAttribute('aria-pressed') !== 'true') b.click(); });
  await page.locator('[data-dms-sec="fight"] .erun-open').first().click();
  await page.locator('.dms-sheet').waitFor();
  await page.locator('.dms-sheet').getByText('Clear the cellar').first().waitFor();
  owner.assertClean();
});

test('on a phone the DM Screen is eight named lines and a tap unfolds one in place', async () => {
  const owner = await person({ tag: 'dmp', name: 'Mara Vell', context: PHONE });
  const { cid } = await setup(owner, 'Saltreach phone');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);

  // It opens from the Tools shelf, as before.
  await page.waitForFunction(() => { const h = document.querySelector('[data-widget="topbar-tools"]'); return !!h && !!h._tbDestroy; });
  await page.locator('#tb-tools-tab').tap();
  await page.locator('#tb-shelf').waitFor({ state: 'visible' });
  await page.waitForTimeout(300);
  await page.locator('#tb-shelf .tb-tool', { hasText: 'Screen' }).tap();
  const dock = page.locator('.dms-dock');
  await dock.waitFor({ state: 'visible' });
  await page.locator('[data-dms-sec="world"]').waitFor({ state: 'visible' });

  // No icon bar; every section is a line with its name, and it is 44 tall.
  assert.equal(await dock.locator('.dms-rail').isVisible(), false, 'the icon bar is still on a phone');
  const names = await dock.locator('[data-dms-sec]:visible .dms-sec-t > span:not(.dms-sec-sum):not(.dms-sec-badge)').allInnerTexts();
  assert.deepEqual(names.map((n) => n.toLowerCase()), LINES.map((n) => n.toLowerCase()));
  for (const h of await dock.locator('[data-dms-sec] .dms-sec-t').evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height))) {
    assert.ok(h >= 44, `a line is ${h} tall`);
  }

  // What each folded line holds, from what the screen already has.
  const sum = async (id) => (await dock.locator(`[data-dms-sec="${id}"] .dms-sec-sum`).innerText()).trim();
  assert.equal(await sum('fight'), '1 ready to run');
  assert.equal(await sum('reveal'), 'Nothing hidden');
  assert.equal(await sum('homebrew'), 'House rules');

  // The world starts unfolded; a tap on Reveal unfolds it, a second tap folds it.
  await page.waitForTimeout(300);
  await pictures(page, 'phone-folded', 'phone', { about: 'The phone DM Screen: named lines, The world unfolded' });
  const reveal = page.locator('[data-dms-sec="reveal"]');
  const head = reveal.locator('[data-dms-fold]');
  assert.equal(await head.getAttribute('aria-expanded'), 'false');
  await head.tap();
  await page.waitForFunction(() => document.querySelector('[data-dms-sec="reveal"] [data-dms-fold]').getAttribute('aria-expanded') === 'true');
  assert.equal(await reveal.locator('.dms-sec-b').isVisible(), true);
  await page.waitForTimeout(300);
  await pictures(page, 'phone-unfolded', 'phone', { about: 'The phone DM Screen with Reveal unfolded' });
  await head.tap();
  await page.waitForFunction(() => document.querySelector('[data-dms-sec="reveal"] [data-dms-fold]').getAttribute('aria-expanded') === 'false');
  await page.waitForFunction(() => document.querySelector('[data-dms-sec="reveal"] .dms-sec-b').hidden);

  // Dragging a finger to scroll opens nothing.
  const before = await dock.locator('[data-dms-sec][data-dms-sec] [data-dms-fold][aria-expanded="true"]').count();
  const box = await dock.locator('.dms-cards').boundingBox();
  await page.touchscreen.tap(box.x + 5, box.y + 5).catch(() => {});
  await page.evaluate(() => { const c = document.querySelector('.dms-cards'); c.scrollTop = 40; });
  assert.equal(await dock.locator('[data-dms-fold][aria-expanded="true"]').count(), before, 'a scroll opened a line');

  // The fight line unfolds to the same card, with Start.
  await page.locator('[data-dms-sec="fight"] [data-dms-fold]').tap();
  await page.locator('[data-dms-sec="fight"] .erun-pick').first().waitFor();

  // The screen's own close mark puts the whole screen away.
  await page.locator('.dms-phonehead-x').tap();
  await dock.waitFor({ state: 'hidden' });
  owner.assertClean();
});
