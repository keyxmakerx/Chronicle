// design_gallery_browser.test.mjs — the server gallery inside the saved-designs
// library (static/js/widgets/design_library_gallery.js), in a real Chromium
// against an in-page fake of the /designs routes: the Share sheet, the status
// chips and reasons, Update and Unpublish, and the Browse sheet with its
// filters, Copy and Report, by keyboard and at phone width.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip } from './_browser_env.mjs';
import { launch, openArranged } from './_design_library_harness.mjs';

const theme = (name) => ({ version: 1, name, skeleton: { shape: 'alcove', opening: 'window-left', display: 'pegboard', counter: 'straight' },
  theme: { wall: 'stone', wood: '#6b4527', cloth: '#5a2024', accent: '#ff9a50', trim: '#c89a4a', light: 'forge', warmth: 0.8, outside: 'dusk' }, props: ['anvil'] });
const mine = (id, name) => ({ id, kind: 'room-theme', name, note: '', systemId: 'drawsteel', makerName: 'Alice', data: theme(name), origin: 'made', shared: false });
const card = (id, name, extra) => Object.assign({ id, kind: 'room-theme', systemId: 'drawsteel', name, note: 'A warm room.', makerName: 'Sunniva Ostrander', summary: 'wall: stone · props: 1', publishedAt: '2026-11-01T10:00:00Z', mine: false, sameSystem: true, lacks: [], reported: false }, extra);

async function arranged(t, opts) {
  const browser = await launch();
  t.after(() => browser.close());
  const r = await openArranged(browser, opts);
  t.after(() => assert.deepEqual(r.errors, []));
  return r.page;
}
const calls = (p) => p.evaluate(() => window.__srv.calls);
const sheet = (p) => p.locator('.dl-sheet');

test('sharing a clean design puts it in Your gallery entries as Visible', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Cinderhold Forge')] } });
  await p.waitForSelector('[data-dg="open-publish"]');
  assert.match(await p.locator('.dl').innerText(), /your gallery entries \(0\)/i);
  await p.click('[data-dg="open-publish"]');
  await sheet(p).locator('#dg-name').waitFor();
  assert.equal(await p.inputValue('#dg-name'), 'Cinderhold Forge');
  assert.equal(await p.evaluate(() => document.activeElement.id), 'dg-sh-t', 'focus moves into the sheet');
  await p.fill('#dg-name', 'Forge of Embers');
  await p.fill('#dg-note', 'A forge that glows.');
  assert.equal(await p.textContent('#dg-cnt'), '19 / 140');
  await p.click('[data-dgs="publish"]');
  await p.waitForSelector('.dg-row');
  assert.ok((await calls(p)).includes('POST /designs/m1/gallery {"name":"Forge of Embers","note":"A forge that glows.","makerName":""}'));
  assert.equal(await sheet(p).count(), 0);
  assert.match(await p.locator('.dg-row').innerText(), /Forge of Embers\s+Visible/);
  assert.equal(await p.evaluate(() => document.activeElement.getAttribute('data-dg')), 'open-publish', 'focus returns to the opener');
});

test('a held design says Waiting for review with the reasons in plain words', { skip }, async (t) => {
  const reasons = ['The text contains a web address or invite link.'];
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Forge')], gal: { own: [], cards: [], status: 'pending', reasons } } });
  await p.click('[data-dg="open-publish"]');
  await p.fill('#dg-note', 'see my site');
  await p.click('[data-dgs="publish"]');
  await p.waitForSelector('.dg-row');
  const row = await p.locator('.dg-row').innerText();
  assert.match(row, /Waiting for review/);
  assert.match(row, /web address or invite link/);
  assert.match(await p.locator('[data-dg-msg]').innerText(), /waiting for review/);
});

test('a refusal from the server shows inline and the sheet stays open', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Forge')], gal: { own: [], cards: [], status: 'visible', reasons: [], refuse: 'name must be at most 60 characters' } } });
  await p.click('[data-dg="open-publish"]');
  await p.click('[data-dgs="publish"]');
  await p.waitForFunction(() => document.querySelector('#dg-perr') && document.querySelector('#dg-perr').textContent.length > 0);
  assert.equal(await p.textContent('#dg-perr'), 'name must be at most 60 characters');
  assert.equal(await sheet(p).count(), 1);
  await p.keyboard.press('Escape');
  assert.equal(await sheet(p).count(), 0);
});

test('Update in gallery re-sends, and Unpublish asks first', { skip }, async (t) => {
  const own = [{ id: 'g1', sourceDesignId: 'm1', kind: 'room-theme', systemId: 'drawsteel', name: 'Forge', note: 'old', makerName: 'Alice', status: 'rejected', reasons: ['An admin took this design down.'] }];
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Forge')], gal: { own, cards: [], status: 'pending', reasons: ['A design that was not approved was sent in again.'] } } });
  await p.waitForSelector('.dg-row');
  assert.match(await p.locator('.dg-row').innerText(), /Not approved/);
  await p.click('[data-dg="update"]');
  assert.equal(await p.inputValue('#dg-note'), 'old');
  assert.equal(await p.locator('#dg-design').isDisabled(), true);
  await p.fill('#dg-note', 'new note');
  await p.click('[data-dgs="publish"]');
  await p.waitForFunction(() => /Waiting for review/.test(document.querySelector('.dg-row').textContent));
  assert.ok((await calls(p)).includes('PUT /designs/m1/gallery {"name":"Forge","note":"new note","makerName":"Alice"}'));
  await p.click('[data-dg="unpublish"]');
  assert.match(await p.locator('.dg-row').innerText(), /Copies already made stay with their owners/);
  await p.click('[data-dg="unpublish-no"]');
  assert.equal(await p.locator('.dg-row').count(), 1);
  await p.click('[data-dg="unpublish"]');
  await p.click('[data-dg="unpublish-ok"]');
  await p.waitForFunction(() => !document.querySelector('.dg-row'));
  assert.ok((await calls(p)).includes('DELETE /designs/gallery/g1'));
});

test('browsing filters, copies and reports', { skip }, async (t) => {
  const cards = [card('c1', 'Moonsilver Ring', { lacks: ['Uses moonsilver, which this campaign lacks.'] }), card('c2', 'Tavern', { systemId: 'dnd5e', sameSystem: false, makerName: 'Cara' }), card('c3', 'My Own', { mine: true })];
  const p = await arranged(t, { seed: { mine: [mine('m1', 'Forge')], takeData: theme('x'), gal: { own: [], cards, status: 'visible', reasons: [] } } });
  await p.click('[data-dg="open-browse"]');
  await sheet(p).locator('.dg-card').first().waitFor();
  const text = await sheet(p).innerText();
  assert.match(text, /Uses moonsilver/);
  assert.match(text, /Made for D&D 5e\. This campaign uses Draw Steel/);
  assert.match(text, /3 designs made for Draw Steel, newest first/);
  assert.equal(await sheet(p).locator('.dg-card', { hasText: 'My Own' }).locator('[data-dgs="copy"]').count(), 0, 'no copy on your own card');
  await sheet(p).locator('[data-dgs="sys"][data-v="all"]').click();
  await p.waitForFunction(() => window.__srv.calls.some((c) => c === 'QUERY page=1&system=all'));
  await sheet(p).locator('[data-dgs="kind"][data-v="crafting"]').click();
  await p.waitForFunction(() => window.__srv.calls.some((c) => c === 'QUERY page=1&system=all&kind=crafting'));
  await p.fill('#dg-b-q', 'moon');
  await p.waitForFunction(() => window.__srv.calls.some((c) => c === 'QUERY page=1&system=all&kind=crafting&q=moon'));

  await sheet(p).locator('.dg-card', { hasText: 'Moonsilver Ring' }).locator('[data-dgs="copy"]').click();
  await p.waitForFunction(() => /In my designs/.test(document.querySelector('.dl-sheet').textContent));
  assert.ok((await calls(p)).includes('POST /designs/gallery/c1/copy'));
  await p.waitForSelector('[data-dl-pick]:has-text("Moonsilver Ring")');

  await sheet(p).locator('.dg-card', { hasText: 'Tavern' }).locator('[data-dgs="report-open"]').click();
  assert.equal(await p.evaluate(() => document.activeElement.id), 'dg-rnote');
  await p.check('input[name="dg-reason"][value="spam"]');
  await p.fill('#dg-rnote', 'an advert');
  await p.click('[data-dgs="report-send"]');
  await p.waitForFunction(() => /You reported this/.test(document.querySelector('.dl-sheet').textContent));
  assert.ok((await calls(p)).includes('POST /designs/gallery/c2/report {"reason":"spam","note":"an advert"}'));
});

test('a report the server refuses is explained inline', { skip }, async (t) => {
  const p = await arranged(t, { seed: { mine: [], gal: { own: [], cards: [card('c1', 'Tavern')], status: 'visible', reasons: [], reportErr: 'you already reported this design' } } });
  await p.click('[data-dg="open-browse"]');
  await sheet(p).locator('[data-dgs="report-open"]').click();
  await p.click('[data-dgs="report-send"]');
  await p.waitForFunction(() => document.querySelector('#dg-rerr') && document.querySelector('#dg-rerr').textContent);
  assert.equal(await p.textContent('#dg-rerr'), 'you already reported this design');
});

test('the sheets fit a 400px screen and the tab key stays inside', { skip }, async (t) => {
  const p = await arranged(t, { width: 400, height: 760, seed: { mine: [mine('m1', 'Forge')], gal: { own: [], cards: [card('c1', 'A very long design name that keeps going and going to test wrapping')], status: 'visible', reasons: [] } } });
  await p.click('[data-dg="open-browse"]');
  await sheet(p).locator('.dg-card').waitFor();
  assert.equal(await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'no page scroll sideways');
  const bg = await sheet(p).evaluate((e) => getComputedStyle(e).backgroundColor);
  assert.notEqual(bg, 'rgba(0, 0, 0, 0)', 'the sheet is opaque');
  for (let i = 0; i < 12; i++) await p.keyboard.press('Tab');
  assert.equal(await p.evaluate(() => !!document.activeElement.closest('.dl-sheet')), true);
});
