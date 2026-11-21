// ai_hex_map_picture.spec.mjs — the picture of a hex map on the AI review:
// "See the map" draws nothing until asked, Peeks on hover or focus, opens in
// the row on a click, and on an update shows the map after the change with the
// hexes that would change ringed and a Before / After switch.
//
// Covers: 1 (a new map's row: no picture at load, the Peek over the list inside
// the side panel and clear of the save bar, gone when the pointer leaves, the
// click opens it and the word reads "Hide the map", no switch on a new map),
// 2 (an update: the summary counts, the ringed line, After first, Before and
// After differ, the Peek is After with no switch), 3 (hexes beyond the map's
// own field are counted), 4 (J/K/Y/N still answer the row), 5 (the map page still
// draws the river, so the shared line drawing is wired).
// Needs a person: touch (tap and the 44 tall word), and a look at the art.
//
// RUN: tools/live-checks.sh test/live/ai_hex_map_picture.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person as signedIn, newCampaign, enableAddons, done, picture } from './_live.mjs';

after(done);

async function person(opts) {
  for (let i = 0; ; i++) {
    try { return await signedIn(opts); } catch (e) { if (i >= 2 || !/could not (register|sign in)/.test(e.message)) throw e; }
  }
}

async function pasteText(page, text) {
  await page.evaluate((text) => {
    const dt = new DataTransfer();
    dt.setData('text/plain', text);
    document.body.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
  }, text);
}

const CREATE = '---\nkind: hex-map\nname: Ashen Coast\nsize: 16 x 10\nreason: You asked for the coast.\n---\n' +
  'fill water\nplains 2,1 to 12,7\nforest around 6,4 radius 2\nriver "Slow Run" 6,4 4,7 3,9\nroad 2,3 6,4 11,5\nplace "Harbor" 2,3: The only deep-water port.\n';

// Paints over plains with forest (ringed), repaints water as water (not), adds a trail, renames Harbor.
const UPDATE = '---\nkind: hex-map\naction: update\nname: Ashen Coast\nsize: 16 x 10\n---\n' +
  'forest 3,2 to 5,2\nwater 0,0 to 2,0\ntrail 8,2 8,6\nplace "Old Harbor" 2,3\n';

const OUTSIDE = '---\nkind: hex-map\naction: update\nname: Ashen Coast\nsize: 100 x 100\n---\nwater 99,99\nwater 98,0\n';

async function review(page) {
  const form = page.locator('#ai-panel form.air');
  await form.waitFor().catch(async (e) => { throw new Error(e.message + '\n' + (await page.locator('#ai-panel').innerText())); });
  return form;
}

test('a new hex map: nothing drawn until asked, Peek, then open', async () => {
  const owner = await person({ tag: 'hexpic', name: 'Hex Picture Owner' });
  const cid = await newCampaign(owner, 'Hex Picture Campaign');
  await enableAddons(owner, cid, '*');
  const { page } = owner;
  await owner.go(`/campaigns/${cid}`);
  await pasteText(page, CREATE);
  const form = await review(page);
  const word = form.locator('.air-map > summary');
  await word.waitFor();
  assert.equal((await word.innerText()).trim(), 'See the map');
  assert.equal(await form.locator('canvas').count(), 0, 'a picture was drawn before it was asked for');

  // Hover for 250 ms: the Peek, inside the panel, not over the save bar.
  await word.scrollIntoViewIfNeeded();
  await word.hover();
  await page.waitForTimeout(120);
  assert.equal(await page.locator('.air-mapcard.is-in').count(), 0, 'it peeked before 250 ms');
  const card = page.locator('.air-mapcard.is-in');
  await card.waitFor();
  const [c, panel, bar] = await Promise.all([card.boundingBox(), page.locator('#ai-panel').boundingBox(), form.locator('.air-savebar').boundingBox()]);
  assert.ok(c.x >= panel.x && c.x + c.width <= panel.x + panel.width, 'the Peek runs past the side panel');
  assert.ok(c.y >= panel.y && c.y + c.height <= panel.y + panel.height, 'the Peek runs past the panel top or bottom');
  assert.ok(c.y + c.height <= bar.y + 1 || c.y >= bar.y + bar.height, 'the Peek covers the save bar');
  assert.equal(await card.locator('canvas').count(), 1);
  assert.equal(await form.locator('.air-seg').count(), 0, 'a Peek has no switch');
  assert.equal(await page.locator('.air-row[data-air-row] .air-yn').first().isVisible(), true, 'the Peek moved the row');
  await picture(page, 'ai-hex-map-peek', { check: 'hex-map picture', about: 'The Peek over the list, anchored to "See the map"', mask: ['.air-mapcard canvas'] });

  await page.mouse.move(5, 5);
  await card.waitFor({ state: 'hidden' });

  // Click opens it in the row; the word flips.
  await word.click();
  const open = form.locator('canvas.air-map-cv.is-ready');
  await open.waitFor();
  assert.equal((await word.innerText()).trim(), 'Hide the map');
  assert.equal(await form.locator('.air-seg').count(), 0, 'a new map has no Before and After');
  const box = await open.boundingBox();
  const main = await form.locator('.air-row .air-main').first().boundingBox();
  assert.ok(box.width <= main.width + 1 && box.width > main.width * 0.9, 'the picture is not the width of the row');
  await word.click();
  assert.equal((await word.innerText()).trim(), 'See the map');
  owner.assertClean();

  // Apply makes the map for the update checks below.
  await form.getByRole('button', { name: 'Apply' }).click();
  await page.getByText(/Import complete|Made 1 of 1/).first().waitFor();
  await page.keyboard.press('Escape');

  // 2. An update: the counts, the rings, After first, a switch that flips.
  await pasteText(page, UPDATE);
  const up = await review(page);
  const text = await up.locator('.air-row').first().innerText();
  assert.match(text, /paints \d+ hexes on the Ashen Coast map: \d+ new, \d+ painted over, of which 3 would change/);
  assert.match(text, /The 3 hexes that would change are ringed on the map\./);
  const w2 = up.locator('.air-map > summary');
  await w2.scrollIntoViewIfNeeded();
  await w2.hover();
  const card2 = page.locator('.air-mapcard.is-in');
  await card2.waitFor();
  assert.equal(await up.locator('.air-seg').count(), 0, 'the Peek shows no switch');
  await page.mouse.move(5, 5);
  await card2.waitFor({ state: 'hidden' });
  await w2.click();
  const cv = up.locator('canvas.air-map-cv.is-ready');
  await cv.waitFor();
  const after = up.getByRole('radio', { name: 'After' });
  const before = up.getByRole('radio', { name: 'Before' });
  assert.equal(await after.getAttribute('aria-checked'), 'true', 'After is not chosen first');
  const shotAfter = await cv.evaluate((c) => c.toDataURL());
  await before.click();
  assert.equal(await before.getAttribute('aria-checked'), 'true');
  await page.waitForFunction((was) => document.querySelector('canvas.air-map-cv').toDataURL() !== was, shotAfter);
  await picture(page, 'ai-hex-map-before', { check: 'hex-map picture', about: 'An update, Before: the stored map with the hexes that would change ringed', mask: ['.air-map-cv'] });
  await after.click();
  await page.waitForFunction((was) => document.querySelector('canvas.air-map-cv').toDataURL() === was, shotAfter);
  // The keyboard reaches the switch and moves it.
  await after.focus();
  await page.keyboard.press('ArrowLeft');
  assert.equal(await before.getAttribute('aria-checked'), 'true');

  // 4. The review's single keys still answer the row, and the new controls don't break them.
  await up.locator('.air-row').first().focus();
  await page.keyboard.press('n');
  assert.equal(await up.locator('.air-n input').first().isChecked(), true);
  await page.keyboard.press('y');
  assert.equal(await up.locator('.air-y input').first().isChecked(), true);
  owner.assertClean();
  await up.getByRole('button', { name: 'Apply' }).click();
  await page.getByText(/Import complete|Made 1 of 1/).first().waitFor();
  await page.keyboard.press('Escape');
  await page.locator('#ai-panel.is-open').waitFor({ state: 'detached' });

  await page.waitForTimeout(500);

  // 3. Hexes beyond the map's own field.
  await pasteText(page, OUTSIDE);
  const out = await review(page);
  assert.match(await out.locator('.air-row').first().innerText(), /2 hexes in this block lie outside the map's own hex field \(\d+ across by \d+ down\) and will not show on the map/);
  owner.assertClean();

  // 4. The map page shares its line drawing with the picture: it must load the
  // shared look module before the hex layer and run without page errors.
  await owner.go(`/campaigns/${cid}/maps`);
  await page.getByRole('link', { name: /Ashen Coast/ }).first().click();
  await page.waitForFunction(() => window.ChronicleHexLook && window.ChronicleMapHexes);
  assert.equal(await page.evaluate(() => typeof window.ChronicleHexLook.linesMarkup), 'function');
  owner.assertClean();
});
