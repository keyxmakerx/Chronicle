// _dice_page.mjs — the page the dice browser tests share: Chronicle.dice
// loaded the way the layout does it, with every file served from static/, and
// the engine on its own with every set and crit loaded. The crits play in
// their own files so CI can run them on runners of their own: each one plays
// in real time on a software GPU.

import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

export { pw, chromePath, skip };

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const HTML = (motion) => `<!doctype html><html${motion ? ` data-motion="${motion}"` : ''}><head><meta charset="utf-8">
<style>html,body{margin:0;height:100%}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}</style></head><body>
<header id="app-topbar" style="height:56px"></header><button id="roll" style="position:absolute;left:40px;bottom:40px">Roll</button>
<template id="chronicle-dice-src" data-src="/static/js/dice.js" data-three-src="/static/vendor/three-dice.min.js" data-cannon-src="/static/vendor/cannon-es.js"></template>
<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>
</body></html>`;

// Software rendering draws slowly, so the pages are small: the crits' timelines run on frame time.
export const SIZE = { width: 800, height: 500 };

export async function page(t, motion, viewport = SIZE, browser = null) {
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

// The engine straight from its module, with every set and crit loaded, on a small page.
export async function enginePage(t, browser) {
  const p = await page(t, null, { width: 480, height: 320 }, browser);
  await p.evaluate(async () => {
    window.E = await import('/static/js/dice/engine.js');
    E.configure({ three: '/static/vendor/three-dice.min.js', cannon: '/static/vendor/cannon-es.js' });
    await Promise.all([E.loadSets(E.SET_KEYS), E.loadCrits(E.CRIT_KEYS.succ.concat(E.CRIT_KEYS.fail))]);
  });
  return p;
}


// Plays one kind's six crits, each with its grand on the die that counts and a
// same-kind echo, split across two browsers so each draws on its own software
// GPU. One success and one failure also carry a die of the other kind, which
// plays the table's echo after the grand's peak. Indexes run over the whole
// crit list, so each crit keeps the set it has always been thrown with.
export async function playCrits(t, want) {
  const first = await enginePage(t);
  const second = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
  t.after(() => second.close());
  const pages = [first, await enginePage(t, second)];
  const parts = await Promise.all(pages.map((p, part) => p.evaluate(async ({ part, want }) => {
    const all = E.CRIT_KEYS.succ.concat(E.CRIT_KEYS.fail), ran = [];
    const mine = all.map((key, i) => i).filter((i) => E.CRIT_KEYS[want].includes(all[i]));
    for (let n = part; n < mine.length; n += 2) {
      const i = mine[n], key = all[i], kind = want, other = kind === 'succ' ? 'fail' : 'succ';
      const v = (k) => (k === 'succ' ? 20 : 1);
      const dice = [
        { sides: 20, kind: E.SET_KEYS[i], value: v(kind), crit: kind, counts: true },
        { sides: 20, kind: E.SET_KEYS[(i + 6) % 12], value: v(kind), crit: kind }
      ];
      if (i === 0 || i === 7) dice.push({ sides: 20, kind: E.SET_KEYS[(i + 3) % 12], value: v(other), crit: other });
      const res = await E.roll({ dice, succFx: kind === 'succ' ? key : 'phoenix', failFx: kind === 'fail' ? key : 'grasp', quality: { mode: 'basic' }, hold: 0 });
      ran.push({ key, grand: res.grand, echoes: res.echoes.length, want: dice.length - 1 });
    }
    return ran;
  }, { part, want })));
  const out = parts.flat();
  for (const r of out) {
    assert.equal(r.grand, 0, `${r.key}: the die that counts takes the grand`);
    assert.equal(r.echoes, r.want, `${r.key}: every other crit die echoes`);
  }
  return out.map((r) => r.key).sort();
}
