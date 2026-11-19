// hidden_tools_named.spec.mjs — the four finished page pieces (quest board,
// notice boards, trackers, tests) are named on Game & features, each with a
// "Put on a page" button. The button opens a small list of categories with
// the one the piece is made for suggested first; choosing one puts the piece
// at the end of that category's pages. Top-bar search finds them by name, and
// the DM Screen's empty Trackers line opens the same list.
//
// RUN: tools/live-checks.sh test/live/hidden_tools_named.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, enableAddons, picture, done } from './_live.mjs';

after(done);

const FORMS = [
  ['phone', { width: 390, height: 844 }],
  ['desk', { width: 1280, height: 800 }],
];
const SCHEMES = ['light', 'dark'];

// One picture per form and colour scheme, named <feature>-<state>-<scheme>-<form>.
async function shoot(page, state, about, opts = {}) {
  for (const scheme of SCHEMES) {
    for (const [form, size] of FORMS) {
      await page.emulateMedia({ colorScheme: scheme });
      await page.setViewportSize(size);
      await page.waitForTimeout(250);
      if (opts.before) await opts.before(form);
      await picture(page, `hidden-tools-${state}-${scheme}-${form}`, { about, ...opts.pic });
    }
  }
  await page.emulateMedia({ colorScheme: 'light' });
  await page.setViewportSize(FORMS[1][1]);
}

test('Game & features names the four pieces, and Put on a page places the quest board', async () => {
  const owner = await person({ tag: 'htn', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Hidden Tools');
  const { page } = owner;
  await page.setViewportSize(FORMS[1][1]);

  // A new campaign has no Quests category; the owner makes one.
  await owner.go(`/campaigns/${cid}/extensions`);
  const made = await owner.api(`/campaigns/${cid}/entity-types`, { method: 'POST', body: { name: 'Quest', name_plural: 'Quests', icon: 'fa-scroll' } });
  assert.ok(made.status < 400, `making the Quests category answered ${made.status}`);

  // 1. The four rows, each with its one line and a button.
  await owner.go(`/campaigns/${cid}/extensions`);
  const section = page.locator('[data-hub-page-pieces]');
  await section.getByRole('heading', { name: 'For your pages' }).waitFor();
  for (const [name, line] of [
    ['Quest board', 'A quest’s page as a cork board: the notice, a map scrap, the reward.'],
    ['Notice boards', 'Boards in a town or tavern that players flip through.'],
    ['Trackers', 'Numbers, steps and clocks you tick during play.'],
    ['Tests', 'Montages and chases, scored as you go.'],
  ]) {
    const row = section.locator('li', { hasText: name }).first();
    await row.getByText(line).waitFor();
    assert.equal(await row.getByRole('button', { name: /Put on a page/ }).count(), 1, `${name} has no button`);
  }
  await shoot(page, 'rows', 'Game & features with the For your pages section', {
    before: () => section.scrollIntoViewIfNeeded(),
    pic: { fullPage: false },
  });

  // 2. The chooser for the quest board suggests Quests, first.
  await section.locator('li', { hasText: 'Quest board' }).getByRole('button', { name: /Put on a page/ }).click();
  const pop = page.locator('.put-pop');
  await pop.getByRole('heading', { name: 'Put the quest board on' }).waitFor();
  await pop.locator('.pop-row').first().waitFor();
  const rows = await pop.locator('.pop-row .pop-name').allTextContents();
  assert.equal(rows[0], 'Quests pages', 'Quests must be listed first');
  assert.equal(await pop.locator('.pop-row').first().locator('.pop-tag').textContent(), 'Suggested');
  await pop.getByText('It goes at the end of each page. Move it later in the Customizer.').waitFor();
  await shoot(page, 'chooser', 'The Put on a page chooser for the quest board, Quests suggested', {
    before: async () => {
      // A new viewport re-places the open list; make sure it is still open.
      if (!(await pop.count())) {
        await section.locator('li', { hasText: 'Quest board' }).getByRole('button', { name: /Put on a page/ }).click();
        await pop.locator('.pop-row').first().waitFor();
      }
    },
  });

  // 3. Choosing Quests puts the board at the foot of a Quests page.
  const questPage = await newPage(owner, cid, 'Quest', 'Clear the cellar');
  await owner.go(`/campaigns/${cid}/extensions`);
  await section.locator('li', { hasText: 'Quest board' }).getByRole('button', { name: /Put on a page/ }).click();
  await pop.locator('.pop-row').first().waitFor();
  await pop.locator('.pop-row').first().click();
  await page.getByText(/Put the quest board at the end of Quests pages/).first().waitFor();
  await pop.waitFor({ state: 'detached' });

  // Putting it again says so instead of stacking a second one.
  await section.locator('li', { hasText: 'Quest board' }).getByRole('button', { name: /Put on a page/ }).click();
  await pop.locator('.pop-row').first().waitFor();
  assert.equal(await pop.locator('.pop-row').first().locator('.pop-tag').textContent(), 'Suggested');
  await pop.locator('.pop-row').first().click();
  await page.getByText('The quest board is already on Quests pages.').first().waitFor();

  await owner.go(`/campaigns/${cid}/entities/${questPage}`);
  const board = page.locator('[data-widget="quest_board"]');
  await board.first().waitFor({ state: 'attached' });
  assert.equal(await board.count(), 1, 'the quest board is on the page more than once');
  const atFoot = await board.first().evaluate((el) => {
    const all = [...document.querySelectorAll('[data-block-type], [data-widget]')];
    const mine = all.findIndex((n) => n === el || n.contains(el));
    return mine >= all.length - 4;
  });
  assert.ok(atFoot, 'the quest board is not at the foot of the page');
  await page.waitForTimeout(600);
  await board.first().scrollIntoViewIfNeeded();
  await shoot(page, 'placed', 'A Quests page with the quest board at its foot', { before: () => board.first().scrollIntoViewIfNeeded() });

  // 4. Top-bar search finds the features by their names and everyday words.
  await owner.go(`/campaigns/${cid}`);
  for (const [typed, expect] of [['tracker', 'Go to Trackers'], ['quest board', 'Go to Quest board'], ['test', 'Go to Tests'], ['notice board', 'Go to Notice boards'], ['delete', 'Go to Trash'], ['history', 'Go to Activity log']]) {
    await page.keyboard.press('Control+Shift+P');
    const input = page.locator('input[placeholder="Type a command…"]');
    await input.waitFor();
    await input.fill(typed);
    await page.getByText(expect, { exact: false }).first().waitFor();
    await page.keyboard.press('Escape');
    await input.waitFor({ state: 'hidden' });
  }
  owner.assertClean();
});

test('the DM Screen’s empty Trackers line opens the same chooser', async () => {
  const owner = await person({ tag: 'htd', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Screen Trackers');
  const { page } = owner;
  await enableAddons(owner, cid, '*');
  await page.setViewportSize(FORMS[1][1]);
  await owner.go(`/campaigns/${cid}`);
  await page.locator('[data-dms-toggle]').click();
  const open = page.locator('[data-dms-open="homebrew"]').first();
  await open.waitFor();
  await open.click();
  const line = page.locator('[data-put-block="trackers"]');
  await line.waitFor();
  await page.getByText('No page has a tracker yet.').waitFor();
  await line.click();
  const pop = page.locator('.put-pop');
  await pop.getByRole('heading', { name: 'Put the Trackers on' }).waitFor();
  await pop.locator('.pop-row').first().waitFor();
  await shoot(page, 'screen-line', 'The DM Screen Trackers line with the chooser open', {
    before: async () => {
      if (!(await pop.count())) {
        await line.click();
        await pop.locator('.pop-row').first().waitFor();
      }
    },
  });
  // Choosing the Adventures category (suggested for trackers when it exists).
  const names = await pop.locator('.pop-row .pop-name').allTextContents();
  assert.ok(names.length > 0, 'the chooser lists no category');
  await pop.locator('.pop-row').first().click();
  await page.getByText(/Put the Trackers at the end of/).first().waitFor();
  owner.assertClean();
});
