// table_sheet.spec.mjs — the Table as a sheet resting on the bottom edge of a
// phone (390 by 844, touch).
//
// The real table.js and table.css run against a stub of the Chronicle global,
// a fake socket and a page with the .tt-host markup from layouts/app.templ.
// The first test pins that markup to the template.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/table_sheet.spec.mjs

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
const tableCSS = read('internal/plugins/rolls/static/css/table.css');
const tableJS = read('internal/plugins/rolls/static/js/table.js');
const inputCSS = read('static/css/input.css');
const tokenLines = inputCSS.split('\n')
  .filter((l) => /^\s*--(dur-[a-z]+|ease-[a-z]+|elev-[a-z]+|color-[a-z-]+):/.test(l)).join('\n');

const ROLLS = [{ id: 'r1', rollerName: 'Mira Vale', label: 'Stealth', formula: '1d20+3', total: 17, createdAt: '2026-11-11T20:00:00Z', dice: [{ sides: 20, value: 14 }], modifier: 3 }];

const page = (attrs = '') => `<!doctype html><html${attrs}><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>:root{${tokenLines} --color-bg-primary:#fff;--color-bg-secondary:#f9fafb;--color-bg-tertiary:#f3f4f6;--color-border:#e5e7eb;--color-text-primary:#111;--color-text-body:#333;--color-text-secondary:#666;--color-text-muted:#888;--color-accent:#6366f1;--color-accent-rgb:99 102 241;--color-warn-rgb:217 119 6;--color-bad-rgb:220 38 38;--dur-slide:360ms}
html,body{margin:0;height:100%;font:14px sans-serif}
header{height:56px;position:relative;border-bottom:1px solid #ddd}
#main-content{height:calc(100% - 56px);overflow-y:auto;box-sizing:border-box;padding:12px}
.jot-tab{position:fixed;right:1.5rem;bottom:1.5rem;width:60px;height:36px}
.tt-n{display:inline-grid} [hidden]{display:none!important}
${tableCSS}</style></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1" data-director="false" data-gm-term="Director">
<button type="button" class="btn-ghost btn-icon tt-tab" data-tt-tab aria-expanded="false" aria-controls="tt-drawer"><span class="tt-n" data-tt-count hidden></span></button></div></header>
<main id="main-content"><p>Page text</p><input id="pagefield" type="text" aria-label="A field on the page"><div style="height:1400px">tall</div><p id="last">The last line</p></main>
<button class="jot-tab">Jot</button>
<script>
window.__sockets = [];
window.WebSocket = function () { var s = { l: {}, addEventListener: function (t, f) { this.l[t] = f; }, close: function () {} }; window.__sockets.push(s); setTimeout(function () { s.l.open && s.l.open(); }, 0); return s; };
window.Chronicle = {
  register: function (n, o) { window.__reg = o; },
  apiFetch: function (u) { var body = /settings/.test(u) ? { look: 'quick' } : { rolls: ${JSON.stringify(ROLLS)}, more: false }; return Promise.resolve({ ok: true, json: function () { return Promise.resolve(body); } }); },
  escapeHtml: function (s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); },
  tableDice: { motion: function () { return document.documentElement.getAttribute('data-motion') || 'full'; }, svg: function () { return '<svg viewBox="0 0 8 8"></svg>'; }, throwInto: function () { return Promise.resolve(); } }
};
</script>
<script>${tableJS}</script>
<script>window.__reg.init(document.querySelector('.tt-host'));</script>
</body></html>`;

let browser;
before(async () => { browser = await chromium.launch(); });
after(async () => { await browser?.close(); });

async function phone(attrs, opts = {}) {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true, ...opts });
  const p = await ctx.newPage();
  await p.setContent(page(attrs));
  await p.waitForSelector('#tt-drawer.tt-sheet');
  await p.waitForFunction(() => document.querySelector('[data-tt-last]').textContent.includes('Mira'));
  return { p, close: () => ctx.close() };
}
const settle = (p) => p.waitForTimeout(450);
const top = (p) => p.$eval('#tt-drawer', (e) => Math.round(e.getBoundingClientRect().top));
const heightOf = (p) => p.$eval('#tt-drawer', (e) => e.dataset.h);
// A finger on the grab line, as real touch events.
async function drag(p, fromY, toY, steps = 8) {
  const cdp = await p.context().newCDPSession(p);
  const pt = (y) => [{ x: 195, y }];
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: pt(fromY) });
  for (let i = 1; i <= steps; i++) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: pt(fromY + ((toY - fromY) * i) / steps) });
    await p.waitForTimeout(40);
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
}

test('the harness header matches layouts/app.templ', () => {
  const app = read('internal/templates/layouts/app.templ');
  for (const frag of ['class="tt-host"', 'data-widget="table"', 'data-tt-tab', 'data-tt-count', 'aria-controls="tt-drawer"']) {
    assert.ok(app.includes(frag), `app.templ no longer has ${frag}`);
  }
});

test('on a phone the Table rests on the bottom edge showing the last roll and who made it', async () => {
  const { p, close } = await phone();
  await settle(p);
  const box = await p.$eval('#tt-drawer', (e) => ({ top: e.getBoundingClientRect().top, h: e.dataset.h }));
  const line = await p.$eval('[data-tt-last]', (e) => e.textContent);
  const tabShown = await p.$eval('[data-tt-tab]', (e) => getComputedStyle(e).display !== 'none');
  const pad = await p.$eval('#main-content', (e) => parseFloat(getComputedStyle(e).paddingBottom));
  const jot = await p.$eval('.jot-tab', (e) => Math.round(innerHeight - e.getBoundingClientRect().bottom));
  await close();
  assert.equal(box.h, 'rest');
  assert.equal(Math.round(844 - box.top), 38, 'the resting line is 38 tall');
  assert.match(line, /Mira Vale · Stealth · 17/);
  assert.equal(tabShown, false, "the header's Table button goes away on a phone");
  assert.equal(pad, 38, 'the page keeps room equal to the resting height');
  assert.equal(jot, Math.round(1.5 * 14) + 38, "Jot's button moves up by the resting height");
});

test('a tap lifts it one height, a second opens it fully, Escape rests it', async () => {
  const { p, close } = await phone();
  await p.tap('[data-tt-grab]');
  await settle(p);
  const lift = { h: await heightOf(p), top: await top(p) };
  await p.tap('[data-tt-grab]');
  await settle(p);
  const full = { h: await heightOf(p), top: await top(p) };
  await p.keyboard.press('Escape');
  await settle(p);
  const rest = await heightOf(p);
  await close();
  assert.equal(lift.h, 'lift');
  assert.ok(Math.abs(lift.top - 844 * 0.4) < 4, `lifted top ${lift.top}`);
  assert.equal(full.h, 'full');
  assert.ok(Math.abs(full.top - 56) <= 2, `full reaches just under the header, at ${full.top}`);
  assert.equal(rest, 'rest');
});

test('lifted shows the dice and the roll field; full shows the log', async () => {
  const { p, close } = await phone();
  await p.tap('[data-tt-grab]');
  await settle(p);
  const lifted = await p.evaluate(() => ({ dice: document.querySelectorAll('.tt-dbtn').length, field: !!document.getElementById('tt-fx'),
    log: getComputedStyle(document.getElementById('tt-log')).display }));
  await p.tap('[data-tt-grab]');
  await settle(p);
  const fullLog = await p.$eval('#tt-log', (e) => getComputedStyle(e).display);
  await close();
  assert.equal(lifted.dice, 6);
  assert.ok(lifted.field);
  assert.equal(lifted.log, 'none');
  assert.notEqual(fullLog, 'none');
});

test('dragging follows the finger and lets go at the nearest height', async () => {
  const { p, close } = await phone();
  await settle(p);
  const y0 = 844 - 19;
  await drag(p, y0, y0 - 330);
  await settle(p);
  const up = await heightOf(p);
  await drag(p, await top(p) + 19, 844 - 19);
  await settle(p);
  const down = await heightOf(p);
  await close();
  assert.equal(up, 'lift');
  assert.equal(down, 'rest');
});

test('a typed roll survives closing', async () => {
  const { p, close } = await phone();
  await p.tap('[data-tt-grab]');
  await settle(p);
  await p.fill('#tt-fx', '2d10+2');
  await p.keyboard.press('Escape');
  await settle(p);
  const rested = await heightOf(p);
  await p.tap('[data-tt-grab]');
  await settle(p);
  const kept = await p.inputValue('#tt-fx');
  await close();
  assert.equal(rested, 'rest');
  assert.equal(kept, '2d10+2');
});

test('a tap on the page above returns it to resting', async () => {
  const { p, close } = await phone();
  await p.tap('[data-tt-grab]');
  await settle(p);
  await p.tap('#main-content p');
  await settle(p);
  const h = await heightOf(p);
  await close();
  assert.equal(h, 'rest');
});

test('a new roll changes the resting line and does not lift the sheet', async () => {
  const { p, close } = await phone();
  await p.evaluate(() => window.__sockets[0].l.message({ data: JSON.stringify({ type: 'roll.made', payload: { id: 'r2', rollerName: 'Tomas', label: 'Attack', formula: '1d20', total: 9, createdAt: '2026-11-11T20:05:00Z', dice: [{ sides: 20, value: 9 }] } }) }));
  await p.waitForFunction(() => document.querySelector('[data-tt-last]').textContent.includes('Tomas'));
  const h = await heightOf(p);
  const badge = await p.$eval('[data-tt-gcount]', (e) => ({ hidden: e.hidden, n: e.textContent }));
  await close();
  assert.equal(h, 'rest');
  assert.deepEqual(badge, { hidden: false, n: '1' });
});

test('the resting line keeps the last line of a page clear', async () => {
  const { p, close } = await phone();
  await p.$eval('#main-content', (e) => { e.scrollTop = e.scrollHeight; });
  const gap = await p.evaluate(() => document.getElementById('tt-drawer').getBoundingClientRect().top - document.getElementById('last').getBoundingClientRect().bottom);
  await close();
  assert.ok(gap >= 0, `the last line sits ${-gap}px under the sheet`);
});

test('every control inside is at least 44 by 44 when lifted and full', async () => {
  const { p, close } = await phone();
  const small = [];
  for (const step of ['lift', 'full']) {
    await p.tap('[data-tt-grab]');
    await settle(p);
    const found = await p.evaluate(() => [...document.querySelectorAll('#tt-sheetbody button, #tt-sheetbody input, #tt-sheetbody select')]
      .filter((e) => e.offsetParent !== null).map((e) => { const r = e.getBoundingClientRect(); return { c: e.className || e.id, w: Math.round(r.width), h: Math.round(r.height) }; })
      .filter((r) => r.w < 44 || r.h < 44));
    small.push(...found.map((f) => ({ step, ...f })));
  }
  await close();
  assert.deepEqual(small, [], 'controls under 44px');
});

test('under Off the heights change with no animation', async () => {
  const { p, close } = await phone(' data-motion="off"');
  await p.tap('[data-tt-grab]');
  await p.waitForTimeout(60);
  const t = await top(p);
  await close();
  assert.ok(Math.abs(t - 844 * 0.4) < 4, `Off was still moving at ${t}`);
});

test('under Calm the sheet changes height at once and fades', async () => {
  const { p, close } = await phone(' data-motion="calm"');
  await p.tap('[data-tt-grab]');
  await p.waitForTimeout(60);
  const r = await p.$eval('#tt-drawer', (e) => ({ top: Math.round(e.getBoundingClientRect().top), anim: getComputedStyle(e).animationName }));
  await close();
  assert.ok(Math.abs(r.top - 844 * 0.4) < 4);
  assert.equal(r.anim, 'tt-fade');
});

test('at 1440 wide the Table still opens from the header, unchanged', async () => {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const p = await ctx.newPage();
  await p.setContent(page());
  await p.waitForFunction(() => window.__reg);
  await p.click('[data-tt-tab]');
  await p.waitForSelector('#tt-drawer');
  const r = await p.evaluate(() => ({ sheet: document.getElementById('tt-drawer').classList.contains('tt-sheet'), attr: document.documentElement.hasAttribute('data-tt-sheet'),
    tab: getComputedStyle(document.querySelector('[data-tt-tab]')).display, grab: !!document.querySelector('[data-tt-grab]'), parent: document.getElementById('tt-drawer').parentNode.tagName }));
  await ctx.close();
  assert.deepEqual(r, { sheet: false, attr: false, tab: 'inline-block', grab: false, parent: 'HEADER' });
});
