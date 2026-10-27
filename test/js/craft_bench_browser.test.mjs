// craft_bench_browser.test.mjs — the crafting bench widget in a real Chromium,
// mounted by boot.js the way a page mounts it. A play mount draws the desk,
// bakes its light and plays a step when an ingredient is dragged into the
// mortar; two benches on one page keep their ids apart; build mode opens the
// editor, and a look from the tray redraws the room; every tray picture the
// editor asks for exists; destroying the widget empties it. The desk's light
// never uses a point light, which phone GPUs draw as black (see craft_bench.ai.md).
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { skip } from './_browser_env.mjs';
import { open, baked, root } from './_craft_harness.mjs';

async function bench(t, opts) {
  const b = await open(opts);
  t.after(() => b.browser.close());
  return b;
}

test('a play mount bakes its light and plays a step', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, {});
  await baked(page);
  const from = await page.evaluate(() => {
    const box = (s) => { const r = document.querySelector(s).getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; };
    document.querySelector('.cr-stage').scrollIntoView();
    return { moss: box('[data-item="bloodmoss"]'), mortar: box('.cr-mortar') };
  });
  await page.mouse.move(...from.moss);
  await page.mouse.down();
  await page.mouse.move(...from.mortar, { steps: 12 });
  await page.mouse.up();
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_bench"]')._crb.state().i === 1, null, { timeout: 10000 });
  const out = await page.evaluate(() => ({
    pointLights: document.querySelectorAll('fePointLight, feSpotLight').length,
    say: document.querySelector('.cr-say').textContent,
  }));
  assert.equal(out.pointLights, 0, 'the desk lights with distant lights only');
  assert.match(out.say, /grind/i);
  assert.deepEqual(errors, []);
});

test('two benches on one page keep their ids apart', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: ['data-mode="play"', 'data-mode="play"'] });
  await baked(page, 0);
  await baked(page, 1);
  const out = await page.evaluate(() => {
    const svgs = [...document.querySelectorAll('.cr-svg')];
    const ids = svgs.map((s) => [...s.querySelectorAll('[id]')].map((e) => e.id));
    const all = ids.flat();
    // Every url(#…) and href="#…" in a scene points at an id in that scene.
    const strays = svgs.map((s, n) => {
      const own = new Set(ids[n]);
      const refs = [...s.outerHTML.matchAll(/(?:url\(#|href="#)([^)"]+)/g)].map((m) => m[1]);
      return refs.filter((r) => !own.has(r));
    });
    return { count: svgs.length, total: all.length, unique: new Set(all).size, strays };
  });
  assert.equal(out.count, 2);
  assert.ok(out.total > 20, 'the scenes have ids to keep apart');
  assert.equal(out.unique, out.total, 'no id appears twice on the page');
  assert.deepEqual(out.strays, [[], []]);
  assert.deepEqual(errors, []);
});

test('build mode opens the editor and a look redraws the room', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: ['data-mode="build" data-thumbs="/static/img/crafting/desk/"'] });
  await page.waitForSelector('.crb .cr-svg');
  await baked(page);
  const status = await page.evaluate(() => {
    window.__redrawn = 0;
    document.querySelector('.cr-svg').addEventListener('craft:redrawn', () => window.__redrawn++);
    return document.querySelector('.crb-status').textContent;
  });
  assert.match(status, /Nothing is saved/);
  await page.click('[data-look="hut"]');
  await page.waitForFunction(() => window.__redrawn > 0, null, { timeout: 10000 });
  const pressed = await page.getAttribute('[data-look="hut"]', 'aria-pressed');
  assert.equal(pressed, 'true');
  // Picking a piece opens its card.
  await page.click('.crb-prow[data-id="cat"]');
  await page.waitForSelector('.crb-pcard:not([hidden])');
  assert.deepEqual(errors, []);
});

test('every tray picture the editor asks for exists', { skip, timeout: 180000 }, async (t) => {
  const { page } = await bench(t, { mounts: ['data-mode="build" data-thumbs="/static/img/crafting/desk/"'] });
  await page.waitForSelector('.crb-ttabs button');
  const srcs = new Set();
  for (const tab of await page.$$('.crb-ttabs button')) {
    await tab.click();
    for (const s of await page.$$eval('.crb-th img', (imgs) => imgs.map((i) => i.getAttribute('src')))) srcs.add(s);
  }
  assert.ok(srcs.size > 20, 'the tray shows pictures');
  const missing = [...srcs].filter((s) => !existsSync(join(root, s)));
  assert.deepEqual(missing, [], 'run node tools/craft-thumbs.mjs to draw them');
});

test('destroying the widget empties it and stops its scene', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, {});
  await baked(page);
  const out = await page.evaluate(async () => {
    const el = document.querySelector('[data-widget="craft_bench"]');
    Chronicle.destroyWidget(el);
    await new Promise((r) => setTimeout(r, 500));
    return { html: el.innerHTML, handle: el._crb };
  });
  assert.equal(out.html, '');
  assert.equal(out.handle, null);
  assert.deepEqual(errors, []);
});
