// dice_browser.test.mjs — Chronicle.dice in a real Chromium, loaded the way
// the layout does it: boot.js's stub fetches dice.js on the first roll, the
// vendored three.js and cannon-es load from the URLs the layout lists, a throw
// draws over the page and resolves with the decided values once the dice
// land, the canvas clears itself away, and Calm and Off resolve at once.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const HTML = (motion) => `<!doctype html><html${motion ? ` data-motion="${motion}"` : ''}><head><meta charset="utf-8">
<style>html,body{margin:0;height:100%}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}</style></head><body>
<header id="app-topbar" style="height:56px"></header><button id="roll" style="position:absolute;left:40px;bottom:40px">Roll</button>
<template id="chronicle-dice-src" data-src="/static/js/dice.js" data-three-src="/static/vendor/three.module.min.js" data-cannon-src="/static/vendor/cannon-es.js"></template>
<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>
</body></html>`;

async function page(t, motion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1200, height: 760 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: HTML(motion) });
    const file = join(root, u.pathname);
    if (!u.pathname.startsWith('/static/') || !existsSync(file)) return route.fulfill({ status: 404, body: '' });
    return route.fulfill({ contentType: 'application/javascript', body: readFileSync(file) });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForFunction(() => window.Chronicle && window.Chronicle.dice);
  return p;
}

test('dice.js is fetched on the first roll, not with the page', { skip }, async (t) => {
  const p = await page(t, 'off');
  const before = await p.evaluate(() => !!document.querySelector('script[src*="dice.js"]'));
  const res = await p.evaluate(() => Chronicle.dice.roll([{ label: 'd20', dice: [{ sides: 20 }] }], { results: [[17]] }));
  const after = await p.evaluate(() => document.querySelectorAll('script[src*="dice.js"]').length);
  assert.equal(before, false);
  assert.deepEqual(res, [{ label: 'd20', values: [17] }]);
  assert.equal(after, 1);
});

const GROUPS = [{ label: 'Power roll', dice: [{ sides: 10 }, { sides: 10 }] }, { label: 'Damage', dice: [{ sides: 6 }, { sides: 20 }, { sides: 4 }] }];
const RESULTS = [[9, 10], [6, 20, 3]];

test('a throw draws, lands on the decided values, then clears away', { skip }, async (t) => {
  const p = await page(t, null);
  const out = await p.evaluate(async ({ GROUPS, RESULTS }) => {
    const at = performance.now();
    const seen = { canvas: false };
    const watch = new MutationObserver(() => { if (document.querySelector('canvas.chronicle-dice')) seen.canvas = true; });
    watch.observe(document.body, { childList: true });
    const res = await Chronicle.dice.roll(GROUPS, { results: RESULTS, from: document.getElementById('roll') });
    return { res, took: performance.now() - at, canvas: seen.canvas };
  }, { GROUPS, RESULTS });
  assert.deepEqual(out.res, [{ label: 'Power roll', values: [9, 10] }, { label: 'Damage', values: [6, 20, 3] }]);
  assert.ok(out.canvas, 'the dice were drawn over the page');
  assert.ok(out.took > 300, `the promise waited for the dice to land (${Math.round(out.took)}ms)`);
  await p.waitForFunction(() => document.getElementById('chronicle-dice-live')?.textContent === 'Power roll: 9, 10. Damage: 6, 20, 3');
  await p.waitForFunction(() => !document.querySelector('canvas.chronicle-dice'), null, { timeout: 20000 });
});

test('a new roll replaces one still on the table', { skip }, async (t) => {
  const p = await page(t, null);
  const both = await p.evaluate(async () => {
    const a = Chronicle.dice.roll([{ label: 'a', dice: [{ sides: 20 }] }], { results: [[1]] });
    await new Promise((r) => setTimeout(r, 400));
    const b = Chronicle.dice.roll([{ label: 'b', dice: [{ sides: 8 }] }], { results: [[8]] });
    return Promise.all([a, b]);
  });
  assert.deepEqual(both, [[{ label: 'a', values: [1] }], [{ label: 'b', values: [8] }]]);
  await p.waitForFunction(() => document.querySelectorAll('canvas.chronicle-dice').length === 0, null, { timeout: 20000 });
});

for (const motion of ['calm', 'off']) {
  test(`${motion} motion resolves at once with the same values`, { skip }, async (t) => {
    const p = await page(t, motion);
    const out = await p.evaluate(async ({ GROUPS, RESULTS }) => {
      const res = await Chronicle.dice.roll(GROUPS, { results: RESULTS });
      return { res, drawnYet: !!document.querySelector('canvas.chronicle-dice') };
    }, { GROUPS, RESULTS });
    assert.deepEqual(out.res.map((r) => r.values), RESULTS);
    // Resolved before any dice were drawn: nothing waits on the drawing.
    assert.equal(out.drawnYet, false);
    if (motion === 'off') {
      await new Promise((r) => setTimeout(r, 300));
      assert.equal(await p.evaluate(() => document.querySelectorAll('canvas').length), 0, 'Off draws nothing');
    } else {
      // Calm still sets the dice down, without throwing them.
      await p.waitForSelector('canvas.chronicle-dice', { state: 'attached', timeout: 20000 });
    }
  });
}
