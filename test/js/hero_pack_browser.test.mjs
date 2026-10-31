// hero_pack_browser.test.mjs — the hero's containers (static/js/widgets/hero_pack.js).
//
// Pressing a tile opens its card; "Carried" and a drag move the tile to
// another box at once and post the new place; a refusal puts it back. Runs in
// a real Chromium on markup shaped like character_panel.templ's.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const JS = readFileSync(join(root, 'static/js/widgets/hero_pack.js'), 'utf8');

const box = (k, tiles) => '<div class="hp-box" data-hp-box="' + k + '"><span data-hp-n></span><ul data-hp-tiles>' + tiles + '</ul><p data-hp-empty' + (tiles ? ' hidden' : '') + '>Nothing here</p></div>';
const tile = (id, n) => '<li data-hp-tile data-item-id="' + id + '" tabindex="0" role="button" aria-expanded="false" draggable="true">' + (n > 1 ? '<span class="hp-ct">' + n + '</span>' : '') + id + '</li>';
const card = (id, at) => '<div data-hp-detail="' + id + '" hidden><button data-move>Move</button><button data-hp-close>x</button>' +
  ['hand', 'worn', 'belt', 'pack'].map((k) => '<button data-hp-carry="' + k + '" aria-pressed="' + (k === at) + '">' + k + '</button>').join('') + '</div>';
const PANEL = '<section id="p"><div data-hero-pack data-place-url="/campaigns/c/armory/characters/h/items/">' +
  box('hand', '') + box('worn', '') + box('belt', tile('potion', 3)) + box('pack', tile('rope', 1)) +
  '</div><div>' + card('potion', 'belt') + card('rope', 'pack') + '</div></section>';

async function page(t, ok) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><body>' + PANEL + '</body></html>');
  await p.evaluate((ok) => {
    window.posts = [];
    window.Chronicle = {
      notify: (m) => { window.told = m; },
      apiFetch: (url, o) => { window.posts.push([url, o.method, o.body.get('place')]);
        return Promise.resolve(ok ? { ok: true } : { ok: false, json: () => Promise.resolve({ message: 'No.' }) }); }
    };
  }, ok);
  await p.addScriptTag({ content: JS });
  return p;
}
const boxOf = (p, id) => p.evaluate((id) => document.querySelector('[data-item-id="' + id + '"]').closest('[data-hp-box]').dataset.hpBox, id);

test('a tile opens its card, and only one card is open', { skip }, async (t) => {
  const p = await page(t, true);
  await p.click('[data-item-id="potion"]');
  assert.equal(await p.isVisible('[data-hp-detail="potion"]'), true);
  await p.click('[data-item-id="rope"]');
  assert.equal(await p.isVisible('[data-hp-detail="potion"]'), false);
  assert.equal(await p.isVisible('[data-hp-detail="rope"]'), true);
  await p.click('[data-hp-detail="rope"] [data-hp-close]');
  assert.equal(await p.isVisible('[data-hp-detail="rope"]'), false);
  await p.focus('[data-item-id="potion"]');
  await p.keyboard.press('Enter');
  assert.equal(await p.isVisible('[data-hp-detail="potion"]'), true);
});

test('Carried moves the tile and posts the place', { skip }, async (t) => {
  const p = await page(t, true);
  await p.click('[data-item-id="potion"]');
  await p.click('[data-hp-detail="potion"] [data-hp-carry="hand"]');
  assert.equal(await boxOf(p, 'potion'), 'hand');
  assert.deepEqual(await p.evaluate(() => window.posts), [['/campaigns/c/armory/characters/h/items/potion/place', 'POST', 'hand']]);
  assert.equal(await p.textContent('[data-hp-box="hand"] [data-hp-n]'), '3 items');
  assert.equal(await p.isVisible('[data-hp-box="belt"] [data-hp-empty]'), true);
  assert.equal(await p.getAttribute('[data-hp-detail="potion"] [data-hp-carry="hand"]', 'aria-pressed'), 'true');
});

test('a drag moves the tile', { skip }, async (t) => {
  const p = await page(t, true);
  await p.dragAndDrop('[data-item-id="rope"]', '[data-hp-box="worn"]');
  assert.equal(await boxOf(p, 'rope'), 'worn');
  assert.equal((await p.evaluate(() => window.posts))[0][2], 'worn');
});

test('a refusal puts the tile back and says why', { skip }, async (t) => {
  const p = await page(t, false);
  await p.click('[data-item-id="rope"]');
  await p.click('[data-hp-detail="rope"] [data-hp-carry="hand"]');
  await p.waitForFunction(() => window.told);
  assert.equal(await boxOf(p, 'rope'), 'pack');
  assert.equal(await p.evaluate(() => window.told), 'No.');
  assert.equal(await p.getAttribute('[data-hp-detail="rope"] [data-hp-carry="pack"]', 'aria-pressed'), 'true');
});
