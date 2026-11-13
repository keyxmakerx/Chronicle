// everyday_motion.spec.mjs — menus run at the everyday speed, not Tailwind's
// fixed 100ms and 75ms (.ai/ui-standard.md, "Every interaction").
//
// A harness page with the real Alpine, the real .mn-* rules and tokens read
// from input.css, and the account menu's transition attributes. The first
// test pins the harness to layouts/app.templ so a menu that goes back to a
// fixed duration fails here.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/everyday_motion.spec.mjs

import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(repoRoot, p), 'utf8');
const css = read('static/css/input.css');

function between(a, b) {
  const i = css.indexOf(a), j = css.indexOf(b, i);
  assert.ok(i >= 0 && j > i, `${a} ... ${b} moved in input.css`);
  return css.slice(i + a.length, j);
}
const menuCSS = between('/* menu-motion:start */', '/* menu-motion:end */');
const tokenLines = css.split('\n')
  .filter((l) => /^\s*--(ease-out|ease-in|dur-everyday|dur-everyday-out):/.test(l)).join('\n');
assert.equal(tokenLines.split('\n').length, 4, 'the everyday tokens moved in input.css');
// The Calm and Off guards, as shipped.
const guardStart = css.indexOf('html[data-cz-reduce] *:not(.czp *), html[data-cz-reduce]');
const guardEnd = css.indexOf('/* Admin Trash', guardStart);
assert.ok(guardStart > 0 && guardEnd > guardStart, 'the Calm and Off guards moved in input.css');
const guardCSS = css.slice(guardStart, guardEnd).replace(/\/\*[\s\S]*$/, '');

const alpinePath = join(repoRoot, 'static/vendor/alpine.min.js');

const page = (attrs = '') => `<!doctype html><html${attrs}><head><meta charset="utf-8">
<style>:root{${tokenLines}} [x-cloak]{display:none!important}${menuCSS}${guardCSS}</style></head>
<body>
  <div x-data="{ open: false }" class="relative">
    <button type="button" id="btn" @click="open = !open">Menu</button>
    <div id="panel" x-show="open" x-cloak
      x-transition:enter="mn-enter" x-transition:enter-start="mn-from" x-transition:enter-end="mn-to"
      x-transition:leave="mn-leave" x-transition:leave-start="mn-to" x-transition:leave-end="mn-from">items</div>
  </div>
</body></html>`;

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

async function open(attrs, rootStyle) {
  const ctx = await browser.newContext();
  const p = await ctx.newPage();
  await p.setContent(page(attrs));
  if (rootStyle) await p.addStyleTag({ content: `:root{${rootStyle}}` });
  await p.addScriptTag({ path: alpinePath });
  await p.waitForFunction(() => window.Alpine && document.querySelector('#panel')._x_isShown !== undefined);
  return { p, close: () => ctx.close() };
}

// The running transitions on the panel right after a toggle.
async function toggleAndRead(p, how) {
  await p.evaluate(() => { window.__t = []; });
  if (how === 'mouse') await p.click('#btn');
  else { await p.keyboard.press('Tab'); await p.keyboard.press('Enter'); }
  // Alpine starts the transition a frame or two after the click.
  await p.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  return p.evaluate(() => document.getAnimations()
    .filter((a) => a.transitionProperty)
    .map((a) => ({ prop: a.transitionProperty, dur: a.effect.getComputedTiming().duration })));
}

test('the account menu reads the everyday tokens, not fixed durations', () => {
  const app = read('internal/templates/layouts/app.templ');
  assert.ok(!/duration-(75|100)\b/.test(app), 'app.templ still has a fixed 75ms or 100ms duration');
  assert.ok(!app.includes('transition-all'), 'app.templ still has transition-all');
  assert.equal((app.match(/x-transition:enter="mn-enter"/g) || []).length, 4);
  assert.equal((app.match(/x-transition:leave="mn-leave"/g) || []).length, 4);
});

test('a menu opens in 120ms and closes in 80ms on a click', async () => {
  const { p, close } = await open();
  const opening = await toggleAndRead(p, 'mouse');
  const closing = await (async () => { await p.waitForTimeout(250); return toggleAndRead(p, 'mouse'); })();
  await close();
  assert.ok(opening.length > 0, 'no transition ran on open');
  for (const t of opening) assert.equal(t.dur, 120, `${t.prop} opened in ${t.dur}`);
  assert.ok(closing.length > 0, 'no transition ran on close');
  for (const t of closing) assert.equal(t.dur, 80, `${t.prop} closed in ${t.dur}`);
});

test('the owner\'s Motion speed retimes the menu', async () => {
  const { p, close } = await open('', '--dur-everyday:200ms');
  const opening = await toggleAndRead(p, 'mouse');
  await close();
  for (const t of opening) assert.equal(t.dur, 200);
});

test('a menu opened from the keyboard appears at once', async () => {
  const { p, close } = await open();
  const opening = await toggleAndRead(p, 'keyboard');
  const shown = await p.$eval('#panel', (e) => getComputedStyle(e).display !== 'none');
  await close();
  assert.ok(shown, 'the menu did not open');
  for (const t of opening) assert.equal(t.dur, 0, `${t.prop} still took ${t.dur}`);
});

test('under Calm a menu only fades, and under Off it appears', async () => {
  const calm = await open(' data-view-motion="calm"');
  const c = await toggleAndRead(calm.p, 'mouse');
  await calm.close();
  assert.ok(c.length > 0);
  assert.ok(c.every((t) => t.prop === 'opacity'), `Calm moved ${c.map((t) => t.prop)}`);

  const off = await open(' data-motion="off"');
  const o = await toggleAndRead(off.p, 'mouse');
  await off.close();
  for (const t of o) assert.ok(t.dur < 1, `${t.prop} took ${t.dur} under Off`);
});
