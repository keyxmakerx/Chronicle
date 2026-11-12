// hero_bench_browser.test.mjs — "On the bench" in the hero's pack
// (internal/plugins/crafting/static/js/hero_bench.js).
//
// The box shows a tile per project being made and drapes the Crafting block
// down on a press. Hover shows a peek that leaves with the pointer; a press
// keeps it open; Escape, the close button or a second press closes it. Runs in
// a real Chromium on markup shaped like character_panel.templ's benchBox.
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
const JS = readFileSync(join(root, 'internal/plugins/crafting/static/js/hero_bench.js'), 'utf8');

const PANEL = '<section id="p"><div class="hp-wrap" style="position:relative;width:600px"><div class="hp-boxes">' +
  '<div class="hp-box hp-bench" data-widget="hero-bench" data-campaign-id="c" data-entity-id="h" data-hp-bench>' +
  '<div class="hp-box-h"><span>On the bench</span><span data-hb-n></span></div>' +
  '<button type="button" class="hb-face" data-hb-face aria-expanded="false" aria-controls="hb-drape-h">' +
  '<span data-hb-tiles></span><span data-hb-hint>Open crafting</span></button></div></div>' +
  '<div class="hb-drape" id="hb-drape-h" data-hb-drape hidden></div></div></section>';

const proj = (id, name, points, goal, rollsReady, extra) => Object.assign({ id, name, points, goal, rollsReady, status: 'active' }, extra);

async function page(t, character) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ reducedMotion: 'reduce' });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><body>' + PANEL + '</body></html>');
  await p.evaluate((character) => {
    window.fetched = []; window.mounted = 0;
    window.CraftingBlock = { blockers: (r) => (r.blocked ? ['no'] : []) };
    window.Chronicle = {
      register: (n, impl) => { window.impl = impl; },
      escapeHtml: (x) => x.replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
      apiFetch: (url) => { window.fetched.push(url); return Promise.resolve({ ok: true, json: () => Promise.resolve(character) }); },
      mountWidget: () => { window.mounted++; },
      destroyWidget: () => {}
    };
  }, character);
  await p.addScriptTag({ content: JS });
  await p.evaluate(() => { const el = document.querySelector('[data-widget="hero-bench"]'); window.impl.init(el, { campaignId: 'c', entityId: 'h' }); });
  await p.waitForFunction(() => !document.querySelector('[data-hb-tiles]').textContent.includes('Loading'));
  return p;
}

const busy = { character: { name: 'Nell' }, recipes: [], pack: {}, projects: [
  proj('1', 'Healing potion', 12, 30, 0),
  proj('2', 'Iron sword', 5, 20, 1),
  proj('3', 'Old rope', 20, 20, 0, { status: 'done' })
] };

test('a tile for each project being made, with its progress and a waiting roll marked', { skip }, async (t) => {
  const p = await page(t, busy);
  assert.equal(await p.locator('.hb-tile').count(), 2);
  assert.match(await p.textContent('.hb-tile >> nth=0'), /Healing potion\s*12 \/ 30/);
  assert.equal(await p.getAttribute('.hb-tile >> nth=0', 'style'), '--hb-fill:40%');
  assert.equal(await p.locator('.hb-tile.is-ready').count(), 1);
  assert.equal(await p.textContent('[data-hb-n]'), '2 being made · 1 roll ready');
  assert.deepEqual(await p.evaluate(() => window.fetched), ['/campaigns/c/crafting/characters/h']);
});

test('with nothing being made, one slot says how many could be made', { skip }, async (t) => {
  const p = await page(t, { character: { name: 'Nell' }, pack: {}, projects: [], recipes: [{ id: 'a' }, { id: 'b' }, { id: 'c', blocked: true }, { id: 'd' }] });
  assert.equal(await p.locator('.hb-tile.is-slot').count(), 1);
  assert.match(await p.textContent('.hb-tile.is-slot'), /Nothing being made\.\s*3 you could make\./);
  assert.equal(await p.textContent('[data-hb-n]'), '');
});

test('a press drapes the Crafting block down, a second press closes it', { skip }, async (t) => {
  const p = await page(t, busy);
  assert.equal(await p.isVisible('[data-hb-drape]'), false);
  await p.click('[data-hb-face]');
  assert.equal(await p.isVisible('[data-hb-drape]'), true);
  assert.equal(await p.getAttribute('[data-hb-face]', 'aria-expanded'), 'true');
  assert.equal(await p.evaluate(() => window.mounted), 1);
  assert.equal(await p.textContent('[data-hb-hint]'), 'Close crafting');
  assert.equal(await p.locator('[data-hb-drape] .hb-close').count(), 1);
  await p.click('[data-hb-face]');
  assert.equal(await p.isVisible('[data-hb-drape]'), false);
  assert.equal(await p.getAttribute('[data-hb-face]', 'aria-expanded'), 'false');
  await p.click('[data-hb-face]');
  assert.equal(await p.evaluate(() => window.mounted), 1, 'the block is mounted once, not on every open');
});

test('Escape and the close button close it, and focus returns to the box', { skip }, async (t) => {
  const p = await page(t, busy);
  await p.click('[data-hb-face]');
  await p.keyboard.press('Escape');
  assert.equal(await p.isVisible('[data-hb-drape]'), false);
  assert.equal(await p.evaluate(() => document.activeElement.hasAttribute('data-hb-face')), true);
  await p.click('[data-hb-face]');
  await p.click('.hb-close');
  assert.equal(await p.isVisible('[data-hb-drape]'), false);
});

test('Escape leaves the drape open while the block has a menu open', { skip }, async (t) => {
  const p = await page(t, busy);
  await p.click('[data-hb-face]');
  await p.evaluate(() => { const d = document.querySelector('[data-hb-drape]'); d.insertAdjacentHTML('beforeend', '<div class="cra"><button aria-expanded="true">more</button></div>'); });
  await p.keyboard.press('Escape');
  assert.equal(await p.isVisible('[data-hb-drape]'), true);
});

test('resting the pointer shows a peek that leaves with it; a press keeps it', { skip }, async (t) => {
  const p = await page(t, busy);
  await p.hover('[data-hb-face]');
  await p.waitForSelector('[data-hb-drape]:not([hidden])');
  assert.equal(await p.getAttribute('[data-hb-face]', 'aria-expanded'), 'false', 'a peek is not a press');
  await p.mouse.move(700, 500);
  await p.waitForSelector('[data-hb-drape][hidden]', { state: 'attached' });
  await p.hover('[data-hb-face]');
  await p.waitForSelector('[data-hb-drape]:not([hidden])');
  await p.click('[data-hb-face]');
  await p.mouse.move(700, 500);
  await p.waitForTimeout(500);
  assert.equal(await p.isVisible('[data-hb-drape]'), true, 'a press keeps it open');
});

test('a tap opens it where there is no hover', { skip }, async (t) => {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ hasTouch: true, isMobile: true, viewport: { width: 390, height: 800 } });
  const p = await ctx.newPage();
  await p.setContent('<!doctype html><html><body>' + PANEL + '</body></html>');
  await p.evaluate((c) => {
    window.mounted = 0;
    window.Chronicle = { escapeHtml: (x) => x, register: (n, i) => { window.impl = i; }, apiFetch: () => Promise.resolve({ ok: true, json: () => Promise.resolve(c) }), mountWidget: () => { window.mounted++; }, destroyWidget: () => {} };
  }, busy);
  await p.addScriptTag({ content: JS });
  await p.evaluate(() => window.impl.init(document.querySelector('[data-widget="hero-bench"]'), { campaignId: 'c', entityId: 'h' }));
  await p.waitForSelector('.hb-tile');
  await p.tap('[data-hb-face]');
  assert.equal(await p.isVisible('[data-hb-drape]'), true);
  await p.tap('[data-hb-face]');
  await p.waitForSelector('[data-hb-drape][hidden]', { state: 'attached' });
});
