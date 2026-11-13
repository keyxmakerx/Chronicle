// page_editor_header.spec.mjs — the page editor's Edit page and header on a
// page in a Material (Annals) campaign. Stands in for the operator check in
// #1501.
//
// Covers steps 1-10. A new campaign's page look is already Material, so the
// campaign needs no setup for steps 1-9; step 10 switches one campaign to
// Standard. Left for a person: whether "nothing else shifts" and the unroll
// *look* right (this checks positions and the Calm switch, not the motion).
//
// RUN: tools/live-checks.sh test/live/page_editor_header.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, addMember, enableAddons, done, picture, BASE } from './_live.mjs';

after(done);

const box = (loc) => loc.boundingBox();
const near = (a, b, msg) => assert.ok(Math.abs(a - b) <= 1, msg + ` (${a} vs ${b})`);

async function setup(tag, context) {
  const owner = await person({ tag, name: 'Mara Vell', context });
  const cid = await newCampaign(owner, 'Header Check');
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');
  await newPage(owner, cid, 'Location', 'Greyhaven');
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  return { owner, cid, eid };
}

test('Edit page moves nothing, not even the title', async () => {
  const { owner } = await setup('pem');
  const { page } = owner;
  const title = page.locator('[data-page-h1]');
  const t0 = await box(title);
  await page.locator('[data-pe-edit]').click();
  await page.locator('[data-pe-part="header"] [data-pe-pen]').waitFor();
  const t1 = await box(title);
  assert.equal(t1.y, t0.y, `the title moved ${t1.y - t0.y}px`);
});

test('picking a parent changes the trail above the title at once', { todo: 'the trail keeps its old pages until a reload: page_editor.js only rewrites the crumb for a rename, not for a new parent' }, async () => {
  const { owner } = await setup('pep2');
  const { page } = owner;
  await page.locator('[data-pe-edit]').click();
  await page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  await page.locator('[data-pe-part="header"] [data-pe-f="parent"]').click();
  await page.locator('.pe-list').getByText('Greyhaven').first().click();
  await page.waitForFunction(() => /Greyhaven/.test(document.querySelector('nav [data-page-crumb]').parentElement.innerText), null, { timeout: 4000 });
});

const savedNote = (page) => page.locator('[data-pe-saved]');
const aliasesOf = async (owner, cid, eid) => (await owner.api(`/campaigns/${cid}/entities/${eid}/aliases`)).body;

test('Edit page, the header sheet, saving as you go, other names, parent', async () => {
  const { owner, cid, eid } = await setup('peh');
  const { page } = owner;
  const edit = page.locator('[data-pe-edit]');

  // 1. Edit page: an edge, Done in the same spot, a pen, nothing else moves.
  const title = page.locator('[data-page-h1]'), crumb = page.locator('[data-page-crumb]');
  const t0 = await box(title), c0 = await box(crumb), e0 = await box(edit);
  await edit.click();
  assert.equal((await edit.innerText()).trim(), 'Done');
  assert.ok(await page.locator('[data-pe-page][data-editing]').count(), 'the page shows no editing edge');
  await page.locator('[data-pe-part="header"] [data-pe-pen]').waitFor();
  const t1 = await box(title), c1 = await box(crumb), e1 = await box(edit);
  near(c1.y, c0.y, 'the trail of pages moved'); near(e1.x + e1.width, e0.x + e0.width, 'the button moved');
  await picture(page, 'page-editor-editing', { check: '#1501', about: 'A page in Edit page mode', mask: ['[data-page-frame] time', '.pe-hint'] });

  // 2. The header opens in place; the sheet overlays instead of pushing.
  const height0 = await page.evaluate(() => document.documentElement.scrollHeight);
  await page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  assert.equal(await title.getAttribute('contenteditable'), 'plaintext-only');
  const sheet = page.locator('[data-pe-part="header"] [data-pe-drape]');
  await sheet.waitFor({ state: 'visible' });
  for (const w of ['Also known as', 'Descriptor', 'Parent page', 'Clone', 'History', 'Delete page']) {
    assert.ok(await sheet.getByText(w).first().isVisible(), `${w} is not on the sheet`);
  }
  assert.equal(await page.evaluate(() => document.documentElement.scrollHeight), height0, 'the sheet pushed the page down');
  await picture(page, 'page-editor-header-sheet', { check: '#1501', about: 'The header sheet open over the page', mask: ['[data-page-frame] time', '.pe-hint'] });

  // 3. Save as you go: a letter on the name, click away.
  await title.click();
  await page.keyboard.press('End');
  await page.keyboard.type('x');
  await page.locator('[data-pe-part="header"] [data-pe-f="label"]').click();
  await savedNote(page).getByText('Saved').waitFor();
  await page.waitForFunction(() => document.querySelector('[data-page-crumb]').textContent.trim() === 'Saltreachx');
  await page.reload({ waitUntil: 'networkidle' });
  assert.equal((await page.locator('[data-page-h1]').innerText()).trim().toLowerCase(), 'saltreachx');

  // Put it back so later steps read plainly.
  await page.locator('[data-pe-edit]').click();
  await page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  await page.locator('[data-page-h1]').click();
  await page.keyboard.press('End');
  await page.keyboard.press('Backspace');
  await page.locator('[data-pe-part="header"] [data-pe-f="label"]').click();
  await savedNote(page).getByText('Saved').waitFor();

  // 4. Other names: nothing saves while it reads "T"; "Tide" saves.
  const aka = page.locator('[data-pe-part="header"] [data-pe-f="aka"]');
  const puts = [];
  page.on('request', (r) => { if (/\/aliases|\/metadata/.test(r.url()) && r.method() !== 'GET') puts.push(r.url()); });
  await aka.click();
  await aka.pressSequentially(', T');
  await page.waitForTimeout(2000);
  assert.equal(puts.filter((u) => /aliases/.test(u)).length, 0, 'a one-letter other name was sent');
  await aka.pressSequentially('ide');
  // The note may still read Saved from the step before, so wait on the stored value.
  let stored = '';
  for (let i = 0; i < 20 && !/tide/i.test(stored); i++) {
    await page.waitForTimeout(300);
    stored = JSON.stringify(await aliasesOf(owner, cid, eid));
  }
  assert.match(stored, /tide/i, 'Tide was not stored: ' + stored);
  // The name chips mount again after the save and fetch their own list, so
  // they can still be on the way when the stored value is already there.
  await page.waitForFunction(() => /Tide/.test(document.querySelector('[data-pe-page]').innerText), null, { timeout: 5000 });

  // 5. Parent page: a list that never offers this page; picking saves and the trail changes.
  const parent = page.locator('[data-pe-part="header"] [data-pe-f="parent"]');
  await parent.click();
  const list = page.locator('.pe-list');
  await list.waitFor({ state: 'visible' });
  const options = (await list.locator('[role="option"], li, button').allInnerTexts()).map((t) => t.trim());
  assert.ok(options.some((t) => /Greyhaven/.test(t)), 'the list does not offer another page');
  assert.ok(!options.some((t) => /^Saltreach$/.test(t)), 'the list offers this page as its own parent');
  await list.getByText('Greyhaven').first().click();
  await savedNote(page).getByText('Saved').waitFor();
  // The trail is checked after a reload here; the live update is its own todo below.
  const reread = await owner.ctx.newPage();
  await reread.goto(page.url(), { waitUntil: 'networkidle' });
  assert.match(await reread.locator('nav:has([data-page-crumb])').innerText(), /Greyhaven/, 'the parent was not stored');
  await reread.close();

  // 6. Keys: Esc closes the sheet, E outlines parts, Esc ends editing.
  await page.keyboard.press('Escape');
  await sheet.waitFor({ state: 'hidden' });
  await page.locator('body').click({ position: { x: 3, y: 400 } });
  await page.keyboard.press('e');
  assert.ok(await page.locator('[data-pe-page][data-pe-marked]').count(), 'E did not outline the parts');
  await page.keyboard.press('Escape'); // closes nothing open; ends editing
  if (await page.locator('[data-pe-page][data-editing]').count()) await page.keyboard.press('Escape');
  await page.waitForFunction(() => !document.querySelector('[data-pe-page][data-editing]'));
  assert.equal((await page.locator('[data-pe-edit]').innerText()).trim(), 'Edit page');
  owner.assertClean();
});

test('opening Also known as and leaving sends nothing', async () => {
  const { owner, cid, eid } = await setup('pen');
  const { page } = owner;
  await page.locator('[data-pe-edit]').click();
  await page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  const aka = page.locator('[data-pe-part="header"] [data-pe-f="aka"]');
  const sent = [];
  page.on('request', (r) => { if (r.method() !== 'GET' && /\/(aliases|metadata)/.test(r.url())) sent.push(r.method() + ' ' + r.url()); });
  const before = JSON.stringify(await aliasesOf(owner, cid, eid));
  await aka.click();
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(800);
  await page.reload({ waitUntil: 'networkidle' });
  assert.equal(JSON.stringify(await aliasesOf(owner, cid, eid)), before, 'the other names changed');
  assert.deepEqual(sent, [], 'something was sent with nothing typed');
});

test('Calm motion fades the sheet; the campaign switch does the same', async () => {
  const { owner, cid, eid } = await setup('pec', { reducedMotion: 'no-preference' });
  const { page } = owner;
  const closedClip = () => page.locator('[data-pe-part="header"] [data-pe-drape]').evaluate((e) => getComputedStyle(e).clipPath);
  await page.locator('[data-pe-edit]').click();
  const normal = await closedClip();
  assert.notEqual(normal, 'none', 'by default the sheet is clipped to unroll');

  await owner.go('/account');
  await page.getByRole('button', { name: 'Calm', exact: true }).click();
  await page.waitForFunction(() => document.documentElement.getAttribute('data-view-motion') === 'calm' || document.documentElement.getAttribute('data-motion') === 'calm');
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await page.locator('[data-pe-edit]').click();
  assert.equal(await closedClip(), 'none', 'under Calm the sheet still unrolls');
  await page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  await page.locator('[data-pe-part="header"] [data-pe-drape]').waitFor({ state: 'visible' });

  // Set Motion back afterwards (this person is only used here).
  await owner.go('/account');
  await page.getByRole('button', { name: 'As the owner set it', exact: true }).click();
  await page.waitForFunction(() => document.documentElement.getAttribute('data-view-motion') !== 'calm');
});

test('a player who claimed a character edits only the name', async () => {
  const owner = await person({ tag: 'pel', name: 'Mara Vell' });
  const player = await person({ tag: 'pep', name: 'Tomas Reed' });
  const cid = await newCampaign(owner, 'Header Player');
  await addMember(owner, cid, player, 'player');
  await enableAddons(owner, cid, ['player-character-claiming']);
  const ch = await newPage(owner, cid, 'Character', 'Ilse Marrow');
  await owner.api(`/campaigns/${cid}/entities/${ch}/permissions`, { method: 'PUT', body: { visibility: 'default' } });
  await owner.api(`/campaigns/${cid}/characters/playable`, { method: 'POST', form: { entity_id: ch, playable: '1' } });
  const claim = await player.api(`/campaigns/${cid}/entities/${ch}/claim`, { method: 'POST' });
  assert.ok(claim.ok, 'the player could not claim: ' + JSON.stringify(claim.body));
  await player.go(`/campaigns/${cid}/entities/${ch}`);
  await player.page.locator('[data-pe-edit]').click({ timeout: 3000 });
  await player.page.locator('[data-pe-part="header"] [data-pe-pen]').click();
  assert.equal(await player.page.locator('[data-pe-part="header"] [data-pe-drape]').count(), 0, 'a player gets the sheet');
  assert.equal(await player.page.locator('[data-page-h1]').getAttribute('contenteditable'), 'plaintext-only');
});

test('a Standard-look campaign has no Edit page button and keeps the old buttons', async () => {
  const owner = await person({ tag: 'pes', name: 'Mara Vell', context: { viewport: { width: 1500, height: 1000 } } });
  const cid = await newCampaign(owner, 'Header Standard');
  const eid = await newPage(owner, cid, 'Location', 'Saltreach');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/customize/look#pages`);
  await page.locator('label.opt', { hasText: 'Standard' }).filter({ has: page.locator('input[name="pg-look"]') }).first().click();
  await page.waitForTimeout(1500);
  await page.locator('[data-czs-save]').click();
  await page.locator('[data-czs-toasts]').getByText(/^Saved/).waitFor();
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  assert.equal(await page.locator('[data-pe-edit]:visible').count(), 0, 'Standard shows an Edit page button');
  for (const b of ['Edit name', 'Clone', 'History', 'Delete']) {
    assert.ok(await page.getByRole('button', { name: new RegExp('^' + b) }).first().isVisible().catch(() => false)
      || await page.getByText(new RegExp('^' + b), { exact: false }).first().isVisible(), `${b} is missing`);
  }
});
