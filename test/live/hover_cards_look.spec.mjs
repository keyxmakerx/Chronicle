// hover_cards_look.spec.mjs — the shared hover card opens as cream paper with a
// second sheet behind it on a linked page, and Customize > Hover cards >
// Night turns it dark with gold lettering. Stands in for the operator check
// in #1173.
//
// Covers steps 1 and 3. Step 2 (click a highlighted rule word on the Rules
// page) needs an installed game-system package with a rulebook; the Draw Steel
// spec in this folder, if present, picks it up.
//
// RUN: tools/live-checks.sh test/live/hover_cards_look.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture } from './_live.mjs';

after(done);

let w;
let customizeErrors = [];

// Opens the card by resting on the link, the way a person does.
async function openCard(who) {
  const { page } = who;
  await who.go(`/campaigns/${w.cid}/entities/${w.from}`);
  await page.locator('a[data-mention-id]').first().hover();
  const card = page.locator('#chc.chc--show');
  await card.waitFor();
  await page.locator('#chc .chc__title').waitFor();
  return card;
}

const cardLook = (page) => page.locator('#chc').evaluate((c) => {
  const inner = c.querySelector('.chc__in');
  const title = c.querySelector('.chc__kind');
  return {
    look: c.dataset.look,
    bg: getComputedStyle(inner).backgroundColor,
    bgImage: getComputedStyle(inner).backgroundImage,
    title: getComputedStyle(title).color,
    sheet: getComputedStyle(c, '::before').content,
    text: c.innerText,
  };
});
const rgb = (s) => (s.match(/[\d.]+/g) || []).slice(0, 3).map(Number);

test('a linked page opens a paper card with a second sheet behind it', async () => {
  const owner = await person({ tag: 'own', name: 'Mara Vell' });
  const cid = await newCampaign(owner, 'Hover Cards');
  const target = await newPage(owner, cid, 'Character', 'Thalrik Stone');
  const from = await newPage(owner, cid, 'Location', 'Ravenhold');
  w = { owner, cid, from, target };
  const url = `/campaigns/${cid}/entities/${target}`;
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [
    { type: 'text', text: 'Ruled by ' },
    { type: 'text', text: 'Thalrik Stone', marks: [{ type: 'link', attrs: { href: url, 'data-mention-id': target, 'data-entity-preview': url + '/preview' } }] },
  ] }] };
  const saved = await owner.api(`/campaigns/${cid}/entities/${from}/entry`, { method: 'PUT', body: { entry: JSON.stringify(doc), entry_html: '' } });
  assert.ok(saved.ok, `saving the link answered ${saved.status}`);

  await openCard(owner);
  const look = await cardLook(owner.page);
  assert.equal(look.look, 'paper', 'a campaign that never chose a look should show Paper');
  assert.match(look.text, /Thalrik Stone/);
  assert.match(look.text, /CHARACTER/i, 'the card does not say what kind of page it is');
  assert.notEqual(look.bgImage, 'none', 'the paper card has no paper texture');
  const [r, g, b] = rgb(look.bg === 'rgba(0, 0, 0, 0)' ? 'rgb(240, 230, 205)' : look.bg);
  assert.ok(r > 200 && g > 190 && b > 150, `the card is not cream: ${look.bg}`);
  assert.notEqual(look.sheet, 'none', 'no second sheet peeks out behind the card');
  await picture(owner.page, 'hover-card-paper', { check: '#1173', about: 'Link hover card in the Paper look', locator: owner.page.locator('#chc') });
  owner.assertClean();
});

test('Customize > Hover cards > Night makes the card dark with gold lettering', async () => {
  const { owner, cid } = w;
  const { page } = owner;
  const before = owner.errors.length;
  await owner.go(`/campaigns/${cid}/customize/look#hover`);
  const night = page.locator('label.hc-tile', { hasText: 'Night' });
  await night.waitFor();
  await night.click();
  await page.getByRole('button', { name: 'Save changes', exact: true }).locator('visible=true').first().click();
  await page.getByText(/Everyone in the campaign now sees your changes/).first().waitFor({ timeout: 20000 });

  await openCard(owner);
  const look = await cardLook(page);
  assert.equal(look.look, 'night', 'the card stays paper after saving Night');
  const [br, bg, bb] = rgb(look.bg);
  assert.ok(br < 70 && bg < 70 && bb < 80, `the card is not dark: ${look.bg}`);
  const [tr, tg, tb] = rgb(look.title);
  assert.ok(tr > 150 && tr > tb + 40, `the gold kind label is not gold: ${look.title}`);
  await picture(page, 'hover-card-night', { check: '#1173', about: 'Link hover card in the Night look', locator: page.locator('#chc') });

  // Put the campaign back to Paper.
  await owner.go(`/campaigns/${cid}/customize/look#hover`);
  await page.locator('label.hc-tile', { hasText: 'Paper' }).click();
  await page.getByRole('button', { name: 'Save changes', exact: true }).locator('visible=true').first().click();
  await page.getByText(/Everyone in the campaign now sees your changes/).first().waitFor({ timeout: 20000 });
  await openCard(owner);
  assert.equal((await cardLook(page)).look, 'paper');
  customizeErrors = owner.errors.splice(before);
});

test('picking a hover card look raises no script errors', () => {
  assert.deepEqual(customizeErrors, []);
});
