// dice_browser.test.mjs — Chronicle.dice in a real Chromium, loaded the way
// the layout does it: boot.js's stub fetches dice.js on the first roll, the
// engine, three.js and cannon-es load from the URLs the layout lists, and a
// set or crit file is fetched only when a roll needs it. A throw draws over
// the page and resolves with the decided values once the dice land; a press
// skips to the result and a second press clears it; Calm and Off resolve at
// once; and every set throws without an error. The crits play in
// dice_crits_succ_browser.test.mjs and dice_crits_fail_browser.test.mjs.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { skip, SIZE, page, enginePage } from './_dice_page.mjs';

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
