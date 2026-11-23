// customize_grow.spec.mjs — Customize's grow: a pressed picture on the wall
// flies to the workshop's stage, holds over it until the workshop has drawn,
// and cross-fades into it; "Everything" steps the workshop back at once and
// flies the picture home into its tile. Focus lands on the workshop's name
// going in and on the tile coming back, and an interrupted move never leaves
// its copy behind.
//
// The real customize_shell.js and customize_wall.css run in a harness page
// with a stub Chronicle and htmx; a swap is played by replacing
// #main-content and sending htmx's own events, as htmx does.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/customize_grow.spec.mjs

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
const js = read('static/js/widgets/customize_shell.js');
const css = read('static/css/customize_wall.css');

const tile = (key, name) => `<a class="czw-tile" href="/c/customize/${key}" data-czw-key="${key}">
  <span class="czw-pic" data-czw-pic><div class="czw-mini"><div class="czw-page">${name}</div></div></span>
  <b class="czw-name">${name}</b><span class="czw-line">line</span></a>`;
const WALL = `<div class="czs czs-is-wall" data-area="wall"><div class="czs-stage"><div class="czs-body" data-czs-stage>
  <div class="czw" data-czw><div class="czw-grid">${tile('frame', 'Site frame')}${tile('colours', 'Colours and type')}${tile('cat-1', 'Characters')}</div>
  <div style="height:1400px"></div>${tile('home-everyone', 'Everyone')}</div></div></div></div>`;
const WORKSHOP = (key, busy) => `<div class="czs czs-is-workshop" data-area="${key === 'colours' ? 'look' : 'categories'}" data-wall-key="${key}"><div class="czs-stage">
  <div class="czs-top" data-czs-top><nav class="czs-tabs-top"><a class="czs-back czs-back-all" href="/c/customize" data-czs-back>Everything</a>
  <span class="czs-tabw"><span class="czs-catname is-static" tabindex="-1">Part</span></span></nav></div>
  <div class="czs-body" data-czs-stage><div class="work"${busy ? ' aria-busy="true"' : ''}>Workshop</div></div></div></div>`;

const page = `<!doctype html><html><head><meta charset="utf-8">
<style>:root{--dur-micro:120ms;--dur-standard:200ms;--dur-large:280ms;--dur-slide:360ms;--ease-out:ease-out;--ease-in-out:ease-in-out;--z-raised:10;--elev-dragged:none}
body{margin:0} #main-content{position:fixed;inset:60px 0 0 200px;overflow:auto} .czs-top{height:50px} .czw-grid{display:grid;grid-template-columns:repeat(3,200px);gap:16px;padding:20px}
.czs-body{animation:czs-settle 280ms} @keyframes czs-settle{from{opacity:0}}
${css}</style></head><body><main id="main-content">${WALL}</main>
<script>
window.__aborts = 0;
window.htmx = { trigger: function (el, name) { if (name === 'htmx:abort') window.__aborts++; } };
window.Chronicle = { register: function () {}, escapeHtml: function (s) { return String(s); }, isMounted: function () { return true; } };
// A swap as htmx makes it: the new content, then afterSwap, then afterSettle.
window.__swap = function (html) {
  var main = document.getElementById('main-content');
  main.innerHTML = html;
  document.dispatchEvent(new CustomEvent('htmx:afterSwap', { detail: { target: main } }));
  setTimeout(function () { document.dispatchEvent(new CustomEvent('htmx:afterSettle', { detail: { target: main } })); }, 20);
};
document.addEventListener('click', function (e) { if (e.target.closest('a')) e.preventDefault(); });
</script>
<script>${js}</script></body></html>`;

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

async function open(motion) {
  const ctx = await browser.newContext({ viewport: { width: 1200, height: 800 }, reducedMotion: motion === 'reduce' ? 'reduce' : 'no-preference' });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  await p.setContent(page);
  if (motion === 'calm' || motion === 'off') await p.evaluate((m) => document.documentElement.setAttribute('data-motion', m), motion);
  return { p, errors, close: () => ctx.close() };
}
const state = (p) => p.evaluate(() => {
  const g = document.querySelector('.czw-fly');
  const stage = document.querySelector('.czs-is-workshop [data-czs-stage]');
  const wall = document.querySelector('[data-czw]');
  return {
    ghosts: document.querySelectorAll('.czw-fly').length,
    ghost: g && (() => { const r = g.getBoundingClientRect(); return { left: Math.round(r.left), top: Math.round(r.top), width: Math.round(r.width) }; })(),
    fly: document.documentElement.getAttribute('data-czs-fly'),
    stageOpacity: stage && +getComputedStyle(stage).opacity,
    wallOpacity: wall && +getComputedStyle(wall).opacity,
    focus: document.activeElement && (document.activeElement.getAttribute('data-czw-key') || document.activeElement.className),
  };
});

test('a pressed picture flies to the stage, holds while the workshop loads, then fades into it', async () => {
  const { p, errors, close } = await open('full');
  await p.click('.czw-tile[data-czw-key="cat-1"]');
  let s = await state(p);
  assert.equal(s.ghosts, 1, 'the press makes one flying copy at once');
  assert.equal(s.fly, 'grow');
  assert.equal(await p.evaluate(() => document.querySelector('[data-czw-key="cat-1"] [data-czw-pic]').style.visibility), 'hidden', 'the pressed picture is not left behind');
  await p.waitForTimeout(450);
  s = await state(p);
  // Laid out where the stage will be: below the bar (50px) over the content column.
  assert.deepEqual(s.ghost, { left: 200, top: 110, width: 1000 }, 'the copy lands where the stage will be');
  await p.evaluate((h) => window.__swap(h), WORKSHOP('cat-1', true));
  await p.waitForTimeout(400);
  s = await state(p);
  assert.equal(s.ghosts, 1, 'the copy holds while the workshop is busy');
  assert.equal(s.stageOpacity, 0, 'the half-drawn workshop stays hidden under the copy');
  await p.evaluate(() => document.querySelector('.work').removeAttribute('aria-busy'));
  await p.waitForTimeout(450);
  s = await state(p);
  assert.equal(s.ghosts, 0, 'the copy is gone once the workshop has drawn');
  assert.equal(s.fly, null);
  assert.equal(s.stageOpacity, 1);
  assert.equal(s.focus, 'czs-catname is-static', 'focus lands on the workshop\'s name');
  assert.deepEqual(errors, []);
  await close();
});

test('a workshop that never says it is ready is shown after 1.5 s', async () => {
  const { p, close } = await open('full');
  await p.click('.czw-tile[data-czw-key="cat-1"]');
  await p.evaluate((h) => window.__swap(h), WORKSHOP('cat-1', true));
  await p.waitForTimeout(1000);
  assert.equal((await state(p)).ghosts, 1);
  await p.waitForTimeout(1000);
  const s = await state(p);
  assert.equal(s.ghosts, 0);
  assert.equal(s.stageOpacity, 1);
  await close();
});

test('Escape before the workshop arrives stops the request and leaves no copy', async () => {
  const { p, close } = await open('full');
  await p.click('.czw-tile[data-czw-key="colours"]');
  await p.waitForTimeout(80);
  await p.keyboard.press('Escape');
  await p.waitForTimeout(50);
  const s = await state(p);
  assert.equal(s.ghosts, 0);
  assert.equal(s.fly, null);
  assert.equal(s.wallOpacity, 1, 'the wall is back');
  assert.equal(await p.evaluate(() => window.__aborts), 1, 'the request is stopped');
  assert.equal(await p.evaluate(() => document.querySelector('[data-czw-key="colours"] [data-czw-pic]').style.visibility), '');
  await close();
});

test('a second press mid-flight replaces the first copy', async () => {
  const { p, close } = await open('full');
  await p.click('.czw-tile[data-czw-key="colours"]');
  await p.waitForTimeout(60);
  await p.evaluate(() => document.querySelector('.czw-tile[data-czw-key="frame"]').click());
  assert.equal((await state(p)).ghosts, 1);
  await p.evaluate((h) => window.__swap(h), WORKSHOP('frame', false));
  // The copy goes once the fly lands and fades, about 0.6 s of animation that
  // a busy runner stretches; wait for it to go, but under the 1.5 s cap so the
  // fallback cannot be what removes it.
  await p.waitForFunction(() => !document.querySelector('.czw-fly'), null, { timeout: 1300 }).catch(() => {});
  assert.equal((await state(p)).ghosts, 0);
  await close();
});

test('Everything steps the workshop back at once, then flies the picture home and focuses its tile', async () => {
  const { p, errors, close } = await open('full');
  await p.evaluate((h) => window.__swap(h), WORKSHOP('home-everyone', false));
  await p.waitForTimeout(500);
  await p.click('[data-czs-back]');
  await p.waitForTimeout(90);
  let s = await state(p);
  assert.ok(s.stageOpacity < 0.95, `the workshop does not step back within 100 ms (${s.stageOpacity})`);
  await p.evaluate((h) => window.__swap(h), WALL);
  await p.waitForTimeout(30);
  s = await state(p);
  assert.equal(s.ghosts, 1, 'the picture flies home');
  assert.equal(s.focus, 'home-everyone', 'focus is on the tile you came from');
  // The tile sits far down the wall; it is brought into view.
  const inView = await p.evaluate(() => { const r = document.querySelector('[data-czw-key="home-everyone"]').getBoundingClientRect(); return r.top >= 60 && r.bottom <= innerHeight; });
  assert.ok(inView, 'the tile is not in view');
  await p.waitForTimeout(400);
  s = await state(p);
  assert.equal(s.ghosts, 0);
  assert.equal(s.fly, null);
  assert.equal(s.wallOpacity, 1);
  assert.deepEqual(errors, []);
  await close();
});

test('Calm fades the picture in place; Off and reduced motion swap with no copy, focus still lands', async () => {
  const calm = await open('calm');
  await calm.p.click('.czw-tile[data-czw-key="cat-1"]');
  await calm.p.waitForTimeout(40);
  const anims = await calm.p.evaluate(() => document.querySelector('.czw-fly').getAnimations().map((a) => a.effect.getKeyframes().map((k) => k.transform || 'none')));
  assert.ok(anims.flat().every((t) => t === 'none'), 'Calm moves the picture');
  await calm.close();
  for (const m of ['off', 'reduce']) {
    const { p, close } = await open(m);
    await p.click('.czw-tile[data-czw-key="cat-1"]');
    assert.equal((await state(p)).ghosts, 0, m + ' makes a copy');
    await p.evaluate((h) => window.__swap(h), WORKSHOP('cat-1', false));
    await p.waitForTimeout(80);
    assert.equal((await state(p)).focus, 'czs-catname is-static', m + ': focus is not on the name');
    await close();
  }
});
