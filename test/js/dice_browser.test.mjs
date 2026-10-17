// dice_browser.test.mjs — Chronicle.dice in a real Chromium, loaded the way
// the layout does it: boot.js's stub fetches dice.js on the first roll, the
// engine, three.js and cannon-es load from the URLs the layout lists, and a
// set or crit file is fetched only when a roll needs it. A throw draws over
// the page and resolves with the decided values once the dice land; a press
// skips to the result and a second press clears it; Calm and Off resolve at
// once; and every set and every crit plays without an error.
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
<template id="chronicle-dice-src" data-src="/static/js/dice.js" data-three-src="/static/vendor/three-dice.min.js" data-cannon-src="/static/vendor/cannon-es.js"></template>
<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>
</body></html>`;

// Software rendering draws slowly, so the pages are small: the crits' timelines run on frame time.
const SIZE = { width: 800, height: 500 };

async function page(t, motion, viewport = SIZE, browser = null) {
  if (!browser) {
    browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    t.after(() => browser.close());
  }
  const p = await browser.newPage({ viewport });
  p.browser = browser;
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  // three.js reports a shader that fails to compile on the console, not as a page error.
  p.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  t.after(() => assert.deepEqual(errors, []));
  p.fetched = [];
  // A test sets p.delay[path] = ms to make that file arrive late.
  p.delay = {};
  await p.route('**/*', async (route) => {
    const u = new URL(route.request().url());
    if (p.delay[u.pathname]) await new Promise((r) => setTimeout(r, p.delay[u.pathname]));
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: HTML(motion) });
    const file = join(root, u.pathname);
    if (!u.pathname.startsWith('/static/') || !existsSync(file)) return route.fulfill({ status: 404, body: '' });
    p.fetched.push(u.pathname);
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
  assert.deepEqual(res, [{ label: 'd20', values: [17], crits: [null] }]);
  assert.equal(after, 1);
});

const GROUPS = [{ label: 'Power roll', dice: [{ sides: 10 }, { sides: 10 }] }, { label: 'Damage', dice: [{ sides: 6 }, { sides: 12 }, { sides: 4 }] }];
const RESULTS = [[9, 10], [6, 11, 3]];

test('a throw draws, lands on the decided values, then clears away', { skip }, async (t) => {
  const p = await page(t, null);
  const out = await p.evaluate(async ({ GROUPS, RESULTS }) => {
    const at = performance.now();
    const seen = { canvas: false, settled: [], ended: 0 };
    const watch = new MutationObserver(() => { if (document.querySelector('canvas.chronicle-dice')) seen.canvas = true; });
    watch.observe(document.body, { childList: true });
    const res = await Chronicle.dice.roll(GROUPS, {
      results: RESULTS, from: document.getElementById('roll'),
      onSettle: (pts) => seen.settled.push(pts), onEnd: () => { seen.ended++; seen.endedWithCanvas = !!document.querySelector('canvas.chronicle-dice'); }
    });
    seen.took = performance.now() - at;
    await new Promise((r) => { const w = () => (seen.ended ? r() : setTimeout(w, 100)); w(); });
    return { res, seen };
  }, { GROUPS, RESULTS });
  assert.deepEqual(out.res, [{ label: 'Power roll', values: [9, 10], crits: [null, null] }, { label: 'Damage', values: [6, 11, 3], crits: [null, null, null] }]);
  assert.ok(out.seen.canvas, 'the dice were drawn over the page');
  assert.ok(out.seen.took > 300, `the promise waited for the dice to land (${Math.round(out.seen.took)}ms)`);
  assert.equal(out.seen.settled.length, 1, 'onSettle fires once');
  assert.equal(out.seen.settled[0].length, 5, 'with a resting place for each die');
  out.seen.settled[0].forEach((pt) => assert.ok(pt.x > 0 && pt.x < SIZE.width && pt.y > 56 && pt.y < SIZE.height, `${JSON.stringify(pt)} is on the page, below the top bar`));
  assert.equal(out.seen.ended, 1, 'onEnd fires once');
  assert.equal(out.seen.endedWithCanvas, false, 'onEnd waits until the dice are gone');
  await p.waitForFunction(() => document.getElementById('chronicle-dice-live')?.textContent === 'Power roll: 9, 10. Damage: 6, 11, 3');
});

test('a set or crit file is fetched only when a roll needs it', { skip }, async (t) => {
  const p = await page(t, null);
  const res = await p.evaluate(() => Chronicle.dice.roll([{ label: 'Attack', dice: [{ sides: 20, set: 'cage', colour: 'iron' }] }], { results: [[20]], succ: 'storm' }));
  assert.deepEqual(res, [{ label: 'Attack', values: [20], crits: ['succ'] }]);
  const dice = p.fetched.filter((f) => f.startsWith('/static/js/dice/')).sort();
  // The table's crit fail comes too: an other-kind echo plays it.
  assert.deepEqual(dice, ['/static/js/dice/crits/grasp.js', '/static/js/dice/crits/storm.js', '/static/js/dice/engine.js', '/static/js/dice/sets/cage.js']);
  await p.waitForFunction(() => !document.querySelector('canvas.chronicle-dice'), null, { timeout: 120000 });
});

test('a new roll replaces one still on the table', { skip }, async (t) => {
  const p = await page(t, null);
  await p.evaluate(() => {
    window.__a = { ended: 0 };
    window.__ra = Chronicle.dice.roll([{ label: 'a', dice: [{ sides: 20 }] }], { results: [[1]], onEnd: () => window.__a.ended++ });
  });
  await p.waitForSelector('canvas.chronicle-dice', { state: 'attached', timeout: 30000 });
  const out = await p.evaluate(async () => {
    let most = 1;
    const mo = new MutationObserver(() => { most = Math.max(most, document.querySelectorAll('canvas.chronicle-dice').length); });
    mo.observe(document.body, { childList: true });
    const b = await Chronicle.dice.roll([{ label: 'b', dice: [{ sides: 8 }] }], { results: [[8]] });
    return { a: await window.__ra, b, most, aEnded: window.__a.ended };
  });
  assert.deepEqual(out.a, [{ label: 'a', values: [1], crits: ['fail'] }]);
  assert.deepEqual(out.b, [{ label: 'b', values: [8], crits: [null] }]);
  assert.equal(out.most, 1, 'the old table goes before the new one is drawn');
  assert.equal(out.aEnded, 1, 'the replaced roll ends');
  await p.waitForFunction(() => document.querySelectorAll('canvas.chronicle-dice').length === 0, null, { timeout: 120000 });
});

test('a throw whose files are late goes on without its dice', { skip }, async (t) => {
  const p = await page(t, null);
  p.delay['/static/vendor/three-dice.min.js'] = 8000;
  const out = await p.evaluate(async () => {
    const at = performance.now(), seen = { settled: [], ended: 0 };
    const res = await Chronicle.dice.roll([{ label: 'x', dice: [{ sides: 6 }] }], {
      results: [[4]], onSettle: (pts) => seen.settled.push(pts), onEnd: () => seen.ended++
    });
    seen.took = performance.now() - at;
    // Past the moment the library arrives: the late roll must not be thrown then.
    await new Promise((r) => setTimeout(r, 7000));
    seen.canvas = document.querySelectorAll('canvas.chronicle-dice').length;
    return { res, seen };
  });
  assert.deepEqual(out.res, [{ label: 'x', values: [4], crits: [null] }]);
  assert.ok(out.seen.took > 3500 && out.seen.took < 6000, `the roll waited about 4s (${Math.round(out.seen.took)}ms)`);
  assert.deepEqual(out.seen.settled, [[]]);
  assert.equal(out.seen.ended, 1);
  assert.equal(out.seen.canvas, 0);
});

test('stop() before the files arrive means those dice are never thrown', { skip }, async (t) => {
  const p = await page(t, null);
  p.delay['/static/js/dice.js'] = 800;
  const out = await p.evaluate(async () => {
    let drawn = false, ended = 0;
    const mo = new MutationObserver(() => { if (document.querySelector('canvas.chronicle-dice')) drawn = true; });
    mo.observe(document.body, { childList: true });
    const r = Chronicle.dice.roll([{ label: 'x', dice: [{ sides: 12 }] }], { results: [[7]], onEnd: () => ended++ });
    setTimeout(() => Chronicle.dice.stop(), 200);
    const res = await r;
    await new Promise((w) => setTimeout(w, 3000));
    return { res, drawn, ended };
  });
  assert.deepEqual(out.res, [{ label: 'x', values: [7], crits: [null] }]);
  assert.equal(out.drawn, false);
  assert.equal(out.ended, 1);
});

test('a press skips to the result, which stays until a second press', { skip }, async (t) => {
  const p = await page(t, null);
  await p.evaluate(() => {
    window.__settled = 0;
    window.__roll = Chronicle.dice.roll([{ label: 'x', dice: [{ sides: 20 }, { sides: 20 }] }], { results: [[20, 7]], onSettle: () => { window.__settled = performance.now(); } });
  });
  await p.waitForSelector('canvas.chronicle-dice', { state: 'attached', timeout: 30000 });
  const pressed = await p.evaluate(() => performance.now());
  await p.mouse.click(600, 400);
  await p.waitForFunction(() => window.__settled > 0, null, { timeout: 10000 });
  const settledAfter = await p.evaluate((at) => window.__settled - at, pressed);
  assert.ok(settledAfter < 1500, `the dice land at once on a press (${Math.round(settledAfter)}ms)`);
  // Skipped dice stay with their numbers lit, longer than an unskipped rest.
  await new Promise((r) => setTimeout(r, 2000));
  assert.equal(await p.evaluate(() => !!document.querySelector('canvas.chronicle-dice')), true, 'still on the page 2s later');
  await p.mouse.click(600, 400);
  await p.waitForFunction(() => !document.querySelector('canvas.chronicle-dice'), null, { timeout: 2000 });
});

test('the catalogue lists every set, colour and crit with the defaults', { skip }, async (t) => {
  const p = await page(t, 'off');
  const c = await p.evaluate(() => Chronicle.dice.catalogue());
  assert.deepEqual(c.sets.map((s) => s.key), ['jewelled', 'confetti', 'acrylic', 'cage', 'heartlight', 'gothic', 'obsidian', 'moonstone', 'oak', 'nebula', 'bone', 'scale']);
  for (const s of c.sets) {
    assert.ok(s.name && s.show && s.showWhat, `${s.key} has a name and a show`);
    assert.ok(s.colours.length >= 3, `${s.key} has colours`);
    for (const col of s.colours) assert.ok(col.key && col.name && /^#[0-9a-f]{6}$/i.test(col.swatch[0]) && /^#[0-9a-f]{6}$/i.test(col.glow), `${s.key}/${col.key}`);
  }
  assert.deepEqual(c.crits.succ.map((x) => x.key), ['storm', 'phoenix', 'dawn', 'starfall', 'bloom', 'hoard']);
  assert.deepEqual(c.crits.fail.map((x) => x.key), ['grasp', 'frost', 'shatter', 'hex', 'crows', 'abyss']);
  for (const x of c.crits.succ.concat(c.crits.fail)) assert.ok(x.name && x.what && x.echoWhat, x.key);
  assert.deepEqual(c.defaults, { set: 'jewelled', colour: 'onyx', succ: 'phoenix', fail: 'grasp', quality: 'auto' });
});

// The engine straight from its module, with every set and crit loaded, on a small page.
async function enginePage(t, browser) {
  const p = await page(t, null, { width: 480, height: 320 }, browser);
  await p.evaluate(async () => {
    window.E = await import('/static/js/dice/engine.js');
    E.configure({ three: '/static/vendor/three-dice.min.js', cannon: '/static/vendor/cannon-es.js' });
    await Promise.all([E.loadSets(E.SET_KEYS), E.loadCrits(E.CRIT_KEYS.succ.concat(E.CRIT_KEYS.fail))]);
  });
  return p;
}

test('every set throws and lights its number, on every shape of die', { skip, timeout: 300000 }, async (t) => {
  const p = await enginePage(t);
  const res = await p.evaluate(() => {
    const sides = [4, 6, 8, 10, 100, 12, 20];
    return E.roll({
      dice: E.SET_KEYS.map((kind, j) => ({ sides: sides[j % 7], kind, value: sides[j % 7] === 100 ? 90 : 3 })),
      quality: { mode: 'basic' }, hold: 0
    });
  });
  assert.equal(res.drawn, true);
  assert.equal(res.values.length, 12);
});

test('every crit plays its grand and its echo, without an error', { skip, timeout: 600000 }, async (t) => {
  // Two browsers, so each draws on its own software GPU, share the twelve crits.
  const first = await enginePage(t);
  const second = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
  t.after(() => second.close());
  const pages = [first, await enginePage(t, second)];
  const parts = await Promise.all(pages.map((p, part) => p.evaluate(async (part) => {
    const all = E.CRIT_KEYS.succ.concat(E.CRIT_KEYS.fail), ran = [];
    for (let i = part; i < all.length; i += 2) {
      const key = all[i], kind = E.CRIT_KEYS.succ.includes(key) ? 'succ' : 'fail', other = kind === 'succ' ? 'fail' : 'succ';
      const v = (k) => (k === 'succ' ? 20 : 1);
      // The grand on the die that counts and a same-kind echo. One success and one failure also have a
      // die of the other kind, which plays the table's echo after the grand's peak.
      const dice = [
        { sides: 20, kind: E.SET_KEYS[i], value: v(kind), crit: kind, counts: true },
        { sides: 20, kind: E.SET_KEYS[(i + 6) % 12], value: v(kind), crit: kind }
      ];
      if (i === 0 || i === 7) dice.push({ sides: 20, kind: E.SET_KEYS[(i + 3) % 12], value: v(other), crit: other });
      const res = await E.roll({ dice, succFx: kind === 'succ' ? key : 'phoenix', failFx: kind === 'fail' ? key : 'grasp', quality: { mode: 'basic' }, hold: 0 });
      ran.push({ key, grand: res.grand, echoes: res.echoes.length, want: dice.length - 1 });
    }
    return ran;
  }, part)));
  const out = parts.flat();
  for (const r of out) {
    assert.equal(r.grand, 0, `${r.key}: the die that counts takes the grand`);
    assert.equal(r.echoes, r.want, `${r.key}: every other crit die echoes`);
  }
  assert.deepEqual(out.map((r) => r.key).sort(), ['abyss', 'bloom', 'crows', 'dawn', 'frost', 'grasp', 'hex', 'hoard', 'phoenix', 'shatter', 'starfall', 'storm']);
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
      // Calm still sets the dice down, without throwing them, and fades them in.
      await p.waitForSelector('canvas.chronicle-dice', { state: 'attached', timeout: 30000 });
    }
  });
}
