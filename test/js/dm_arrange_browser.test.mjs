// dm_arrange_browser.test.mjs: Arrange mode, saved layouts and the Style
// drawer on the docked screen (static/js/widgets/dm_arrange.js, with
// dm_layout.js and dm_screen.js), in a real browser. The real scripts and the
// dock's own rules cut from input.css and style_kits.css run against a small
// fake server written to the screen-prefs and my-look routes. It pins:
// entering arrange mode, a card's height, hiding a card (tinted while
// arranging, off the rail after Done), saving a layout, switching back to
// Default, the arrow keys on a grip, the Style drawer changing the dock's kit
// and surface, the server copy coming back on a fresh browser, and a failed
// save said once.

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

// The dock's block of input.css, as the site builds it.
function dockCSS() {
  const css = read('static/css/input.css');
  const from = css.indexOf('/* DM Screen (internal/plugins/dmscreen');
  const to = css.indexOf('/* ---- Pictures inside editor text');
  if (from < 0 || to < from) throw new Error('the DM Screen rules moved in input.css');
  return css.slice(from, to);
}

const FILES = {
  '/js/dm_layout.js': read('static/js/widgets/dm_layout.js'),
  '/js/dm_arrange.js': read('static/js/widgets/dm_arrange.js'),
  '/js/dm_screen.js': read('static/js/widgets/dm_screen.js'),
  '/css/dock.css': dockCSS(),
  '/css/style_kits.css': read('static/css/style_kits.css'),
};

const TOKENS = `:root { --color-text-primary: #1b1b1f; --color-text-secondary: #444; --color-card-bg: #fff; --color-accent: #2563eb;
  --surface-overlay: rgb(6 7 11 / .55); --surface-text: #1b1b1f; --surface-text-muted: #555; --surface-raised: #f1f1f4; --surface-border: #d4d4da;
  --color-ok-rgb: 22 163 74; --color-warn-rgb: 217 119 6;
  --dur-instant: 80ms; --dur-micro: 120ms; --dur-everyday: 120ms; --dur-standard: 200ms; --dur-slide: 360ms; --ease-out: cubic-bezier(0.16, 1, 0.3, 1); --ease-in: cubic-bezier(0.4, 0, 1, 1);
  --elev-hover: 0 4px 10px rgb(0 0 0 / .18); --elev-dragged: 0 12px 28px rgb(0 0 0 / .3); }
  *, *::before, *::after { box-sizing: border-box; } body { margin: 0; font: 16px/1.4 system-ui, sans-serif; background: #fafafa; } [hidden] { display: none !important; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }`;

const SECTIONS = [['world', 'World'], ['party', 'Party'], ['notes', 'Notes']];

function panel() {
  const rail = SECTIONS.map(([id, t]) => `<button type="button" class="dms-ic" data-dms-open="${id}" aria-pressed="false" aria-label="${t}" title="${t}">${t[0]}</button>`).join('');
  const cards = SECTIONS.map(([id, t]) => `<section class="dms-sec" data-dms-sec="${id}" aria-labelledby="dms-h-${id}" hidden>
<header class="dms-sec-h"><span class="dms-sec-grip" data-dms-grip aria-hidden="true">::</span>
<button type="button" class="dms-sec-t" data-dms-fold aria-expanded="true"><span id="dms-h-${id}">${t}</span><span class="dms-sec-sum">about ${t}</span></button>
<button type="button" class="dms-ic dms-sec-x" data-dms-shut aria-label="Close ${t}">x</button></header>
<div class="dms-sec-b">${'<p>A line of ' + t + '.</p>'.repeat(40)}</div></section>`).join('');
  return `<div class="dms" data-dms-root><nav class="dms-rail" aria-label="Screen sections">
<button type="button" class="dms-ic" data-dms-tuck aria-label="Tuck away">&gt;</button><hr>${rail}</nav>
<div class="dms-col" data-dms-col><div class="dms-colhead"><span class="dms-count" data-dms-count></span><button type="button" class="dms-quiet" data-dms-closeall>Close all</button></div>
<p class="dms-empty" data-dms-empty hidden>Pick a section.</p><div class="dms-cards" data-dms-cards>${cards}</div></div></div>`;
}

let browser;
after(async () => { if (browser) await browser.close(); });

// open starts a page on a fake server. srv holds what the server keeps and
// records every write; pass the same srv to a second open to play a second
// browser of the same person.
async function open(srv) {
  browser = browser || await pw.chromium.launch({ executablePath: chromePath });
  const ctx = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  const page = await ctx.newPage();
  page.setDefaultTimeout(8000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  const body = `<!doctype html><html data-motion="off"><head><meta charset="utf-8"><style>${TOKENS}</style>
<link rel="stylesheet" href="/css/dock.css"><link rel="stylesheet" href="/css/style_kits.css"></head><body>
<header id="app-topbar" style="height:56px"><div data-widget="dm-screen" data-url="/campaigns/c1/dm-screen"><button type="button" data-dms-toggle aria-expanded="false">Screen</button></div></header>
<main id="main-content"></main>
<script>
var widgets = {};
window.__notes = [];
window.Chronicle = {
  register: function (n, d) { widgets[n] = d; },
  mountWidgets: function () {}, destroyWidget: function () {},
  escapeHtml: function (s) { return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); },
  apiFetch: function (u, o) { o = o || {}; return fetch(u, { method: o.method, headers: Object.assign({ 'Content-Type': 'application/json' }, o.headers), body: o.body && typeof o.body === 'object' ? JSON.stringify(o.body) : o.body }); },
  notify: function (m, k) { window.__notes.push(k + ': ' + m); }, motionMs: function (n, d) { return d; }, reducedMotion: function () { return true; }, gmTerm: function () { return 'DM'; }
};
</script><script src="/js/dm_layout.js"></script><script src="/js/dm_arrange.js"></script><script src="/js/dm_screen.js"></script>
<script>widgets['dm-screen'].init(document.querySelector('[data-widget="dm-screen"]'));</script></body></html>`;
  await page.route('http://chronicle.test/**', async (r) => {
    const req = r.request();
    const path = new URL(req.url()).pathname;
    const method = req.method();
    srv.calls.push(method + ' ' + path);
    const json = (o, status) => r.fulfill({ status: status || 200, contentType: 'application/json', body: JSON.stringify(o) });
    if (path === '/') return r.fulfill({ contentType: 'text/html', body });
    if (FILES[path]) return r.fulfill({ contentType: path.endsWith('.css') ? 'text/css' : 'text/javascript', body: FILES[path] });
    if (path === '/campaigns/c1/dm-screen') return r.fulfill({ contentType: 'text/html', body: panel() });
    if (path === '/campaigns/c1/screen-prefs/dm/kits.css') return r.fulfill({ contentType: 'text/css', body: '.dms-kt[data-dms-kit] { outline: 0; }' });
    if (path === '/campaigns/c1/screen-prefs/dm') {
      if (method === 'GET') return json({ screen: 'dm', stored: !!srv.prefs, prefs: srv.prefs || { style: {} }, kit: srv.kit });
      if (srv.failPut) return json({ message: 'no' }, 500);
      const b = JSON.parse(req.postData());
      srv.puts.push(b);
      srv.prefs = Object.assign({}, srv.prefs || { style: {} });
      for (const k of Object.keys(b)) { if (b[k] === null) delete srv.prefs[k]; else srv.prefs[k] = b[k]; }
      return json({ screen: 'dm', stored: true, prefs: srv.prefs, kit: srv.kit });
    }
    if (path === '/campaigns/c1/my-look/dmscreen' && method === 'PUT') {
      srv.looks.push(JSON.parse(req.postData()));
      return json({});
    }
    return r.fulfill({ status: 404, body: '' });
  });
  await page.goto('http://chronicle.test/');
  await page.click('[data-dms-toggle]');
  await page.waitForSelector('[data-dms-arrange]');
  page.done = async () => { await ctx.close(); assert.deepEqual(errors, [], 'the page threw'); };
  return page;
}

function server() {
  return { prefs: null, puts: [], looks: [], calls: [], failPut: false,
    kit: { campaign: 'material', canPick: true, part: 'dmscreen' } };
}

const sizeOf = (page, id) => page.getAttribute(`[data-dms-sec="${id}"]`, 'data-dms-size');
const visible = (page, sel) => page.evaluate((s) => { const e = document.querySelector(s); return !!e && e.getClientRects().length > 0; }, sel);
async function waitPut(page, srv, pred) {
  for (let i = 0; i < 60; i++) { if (srv.puts.some(pred)) return; await new Promise((r) => setTimeout(r, 50)); }
  throw new Error('no matching save; saves were ' + JSON.stringify(srv.puts));
}

test('arrange: a height, a hidden card, a saved layout and back to Default', { skip }, async () => {
  const srv = server();
  const page = await open(srv);
  await page.waitForSelector('[data-dms-sec="world"]:not([hidden])');
  // Nothing stored yet: this browser's screen becomes Default's start.
  await waitPut(page, srv, (b) => b.home && b.home.open.join() === 'world,party');

  await page.click('[data-dms-arrange]');
  assert.ok(await page.evaluate(() => document.querySelector('.dms-dock').classList.contains('dms-arranging')));
  assert.equal(await page.getAttribute('[data-dms-arrange]', 'aria-pressed'), 'true');
  assert.ok(await visible(page, '[data-dms-arrbar]'), 'the arranging bar shows');
  assert.equal(await page.textContent('[data-dms-layname]'), 'Default');
  assert.ok(await visible(page, '[data-dms-sec="world"] [data-dms-sz="S"]'), 'card tools show');

  // A short card: its body scrolls inside 9rem.
  await page.click('[data-dms-sec="world"] [data-dms-sz="S"]');
  assert.equal(await sizeOf(page, 'world'), 'S');
  assert.equal(await page.getAttribute('[data-dms-sec="world"] [data-dms-sz="S"]', 'aria-pressed'), 'true');
  const body = await page.evaluate(() => { const b = document.querySelector('[data-dms-sec="world"] .dms-sec-b'); const cs = getComputedStyle(b); return { max: cs.maxHeight, oy: cs.overflowY, h: b.getBoundingClientRect().height }; });
  assert.equal(body.max, '144px');
  assert.equal(body.oy, 'auto');
  assert.ok(body.h <= 145, 'the body is held to the short height');

  // Save the screen as a layout.
  await page.click('[data-dms-laymenu]');
  await page.waitForSelector('[data-dms-menu] [data-dms-lsave]');
  await page.click('[data-dms-menu] [data-dms-lsave]');
  await page.fill('[data-dms-menu] input', 'fight night');
  await page.keyboard.press('Enter');
  await page.waitForSelector('[data-dms-menu]', { state: 'detached' });
  assert.equal(await page.textContent('[data-dms-layname]'), 'fight night');

  // On the saved layout: hide Party. It stays in view, tinted, while arranging.
  await page.click('[data-dms-sec="party"] [data-dms-hide]');
  assert.ok(await page.evaluate(() => document.querySelector('[data-dms-sec="party"]').classList.contains('dms-hid')));
  assert.ok(await visible(page, '[data-dms-sec="party"]'), 'a hidden card stays in view while arranging');
  assert.equal(await page.getAttribute('[data-dms-sec="party"] [data-dms-hide]', 'aria-label'), 'Show Party');
  assert.equal(await page.textContent('[data-dms-layname]'), 'fight night, changed');
  await page.click('[data-dms-laymenu]');
  await page.click('[data-dms-menu] [data-dms-lupdate]');
  assert.equal(await page.textContent('[data-dms-layname]'), 'fight night');

  // Done: the hidden card and its rail icon go.
  await page.click('[data-dms-done]');
  assert.ok(!(await page.evaluate(() => document.querySelector('.dms-dock').classList.contains('dms-arranging'))));
  assert.ok(!(await visible(page, '[data-dms-sec="party"]')), 'hidden after Done');
  assert.ok(!(await visible(page, '.dms-rail [data-dms-open="party"]')), 'off the rail after Done');
  assert.ok(!(await visible(page, '[data-dms-arrbar]')));

  // Back to Default: Party is there, World keeps the height set on Default.
  await page.click('[data-dms-laymenu]');
  await page.click('[data-dms-menu] [data-dms-lay=""]');
  await page.waitForSelector('[data-dms-sec="party"]:not([hidden])');
  assert.ok(await visible(page, '.dms-rail [data-dms-open="party"]'));
  assert.equal(await sizeOf(page, 'world'), 'S');
  // And to the saved one again: Party hides.
  await page.click('[data-dms-laymenu]');
  await page.click('[data-dms-menu] [data-dms-lay="fight night"]');
  assert.ok(!(await visible(page, '[data-dms-sec="party"]')));

  // What reached the account: the layout, with Party hidden, and Default
  // still current after the last switch went out.
  await waitPut(page, srv, (b) => Array.isArray(b.layouts) && b.layouts.some((l) => l.name === 'fight night' && (l.hidden || []).includes('party')));
  await waitPut(page, srv, (b) => b.current === 'fight night');
  await page.done();
});

test('a focused grip moves its card with the arrow keys, said aloud', { skip }, async () => {
  const srv = server();
  const page = await open(srv);
  await page.click('[data-dms-arrange]');
  // The column shows the newest first: Party on top, then World.
  await page.focus('[data-dms-sec="party"] [data-dms-grip]');
  assert.equal(await page.getAttribute('[data-dms-sec="party"] [data-dms-grip]', 'role'), 'button');
  await page.keyboard.press('ArrowDown');
  const order = await page.evaluate(() => [...document.querySelectorAll('[data-dms-cards] [data-dms-sec]:not([hidden])')]
    .sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top).map((s) => s.getAttribute('data-dms-sec')));
  assert.deepEqual(order, ['world', 'party']);
  await page.waitForFunction(() => /Party is 2 of 2 in the column/.test(document.querySelector('[data-dms-live]').textContent));
  assert.equal(await page.evaluate(() => document.activeElement.closest('[data-dms-sec]').getAttribute('data-dms-sec')), 'party', 'focus stays on the moved grip');
  // Escape leaves arrange mode, and the grip stops being a control.
  await page.keyboard.press('Escape');
  assert.ok(!(await page.evaluate(() => document.querySelector('.dms-dock').classList.contains('dms-arranging'))));
  assert.equal(await page.getAttribute('[data-dms-sec="party"] [data-dms-grip]', 'tabindex'), null);
  await page.done();
});

test('dragging a card while arranging shows an outline of its shape where it lands', { skip }, async () => {
  const srv = server();
  const page = await open(srv);
  await page.waitForSelector('[data-dms-sec="world"]:not([hidden])');
  await page.click('[data-dms-arrange]');
  // Short cards, so both fit in the window.
  await page.click('[data-dms-sec="world"] [data-dms-sz="S"]');
  await page.click('[data-dms-sec="party"] [data-dms-sz="S"]');
  const grip = await page.locator('[data-dms-sec="party"] [data-dms-grip]').boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + 4, grip.y + 30, { steps: 4 });
  // The lifted card leaves its place and the outline makes room where it
  // would land, so World is measured again as the pointer goes.
  for (let i = 0; i < 3; i++) {
    const world = await page.locator('[data-dms-sec="world"]').boundingBox();
    await page.mouse.move(world.x + world.width / 2, world.y + world.height - 10, { steps: 3 });
  }
  assert.equal(await page.locator('.dms-dropghost').count(), 1, 'one outline while dragging');
  assert.ok(await page.evaluate(() => document.querySelector('[data-dms-sec="party"]').classList.contains('dms-lifted')));
  await page.mouse.up();
  assert.equal(await page.locator('.dms-dropghost').count(), 0, 'the outline goes on the drop');
  const order = await page.evaluate(() => [...document.querySelectorAll('[data-dms-cards] [data-dms-sec]:not([hidden])')]
    .sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top).map((s) => s.getAttribute('data-dms-sec')));
  assert.deepEqual(order, ['world', 'party']);
  await page.done();
});

test('the Style drawer changes the dock\'s kit and surface', { skip }, async () => {
  const srv = server();
  const page = await open(srv);
  await page.click('[data-dms-styletab]');
  await page.waitForSelector('#dms-style-dm:not([hidden]) [data-dms-kitpick="ornate"]');
  assert.equal(await page.getAttribute('[data-dms-styletab]', 'aria-expanded'), 'true');
  assert.equal(await page.getAttribute('[data-dms-kitpick=""]', 'aria-pressed'), 'true', 'Follow the campaign is the start');

  await page.click('[data-dms-kitpick="ornate"]');
  assert.equal(await page.getAttribute('.dms-dock', 'data-dms-kit'), 'ornate');
  const paper = await page.evaluate(() => getComputedStyle(document.querySelector('.dms-dock')).getPropertyValue('--surface-raised').trim());
  assert.equal(paper, '#fbf5e8', 'the dock wears the kit\'s paper');
  assert.equal(await page.getAttribute('[data-dms-kitpick="ornate"]', 'aria-pressed'), 'true');
  await page.waitForFunction(() => document.querySelector('link[data-dms-kits]'));
  for (let i = 0; i < 40 && !srv.looks.length; i++) await new Promise((r) => setTimeout(r, 50));
  assert.deepEqual(srv.looks, [{ value: 'ornate' }], 'the pick goes through the member look');

  await page.click('[data-dms-surf="glass"]');
  assert.equal(await page.getAttribute('.dms-dock', 'data-dms-surface'), 'glass');
  const blur = await page.evaluate(() => getComputedStyle(document.querySelector('.dms-dock .dms-col')).backdropFilter);
  assert.match(blur, /blur/);
  await page.click('[data-dms-dens="compact"]');
  assert.equal(await page.evaluate(() => getComputedStyle(document.querySelector('.dms-dock')).getPropertyValue('--dms-gap').trim()), '5px');
  await waitPut(page, srv, (b) => b.style && b.style.surface === 'glass');

  // Follow the campaign puts the campaign's kit back on the dock.
  await page.click('[data-dms-kitpick=""]');
  assert.equal(await page.getAttribute('.dms-dock', 'data-dms-kit'), 'material');
  for (let i = 0; i < 40 && srv.looks.length < 2; i++) await new Promise((r) => setTimeout(r, 50));
  assert.deepEqual(srv.looks[1], { value: null });

  await page.keyboard.press('Escape');
  await page.waitForSelector('#dms-style-dm', { state: 'hidden' });
  assert.equal(await page.evaluate(() => document.activeElement.hasAttribute('data-dms-styletab')), true, 'focus returns to the tab');
  await page.done();
});

test('where the kit can\'t be picked, the Kit row says whose choice it is', { skip }, async () => {
  const srv = server();
  srv.kit = { campaign: 'sharp', canPick: false };
  const page = await open(srv);
  await page.click('[data-dms-styletab]');
  await page.waitForSelector('#dms-style-dm:not([hidden]) .dms-sd-note');
  assert.match(await page.textContent('#dms-style-dm .dms-sd-note'), /Follows the campaign’s choice: Sharp/);
  assert.equal(await page.$('[data-dms-kitpick]'), null);
  assert.equal(await page.$('link[data-dms-kits]'), null, 'no kit faces to load');
  await page.done();
});

test('a fresh browser gets the account\'s layouts; a failed save is said once', { skip }, async () => {
  const srv = server();
  srv.prefs = { current: 'Downtime', layouts: [{ name: 'Downtime', open: ['notes'], sizes: { notes: 'M' } }], style: { surface: 'felt' } };
  const page = await open(srv);
  await page.waitForSelector('[data-dms-sec="notes"]:not([hidden])');
  assert.ok(!(await visible(page, '[data-dms-sec="world"]')), 'the saved layout replaces this browser\'s');
  assert.equal(await sizeOf(page, 'notes'), 'M');
  assert.equal(await page.getAttribute('.dms-dock', 'data-dms-surface'), 'felt');

  srv.failPut = true;
  await page.click('[data-dms-arrange]');
  await page.click('[data-dms-sec="notes"] [data-dms-sz="S"]');
  await page.waitForFunction(() => window.__notes.length === 1, null, { timeout: 4000 });
  await page.click('[data-dms-sec="notes"] [data-dms-sz="L"]');
  await new Promise((r) => setTimeout(r, 900));
  const notes = await page.evaluate(() => window.__notes);
  assert.equal(notes.length, 1, 'said once');
  assert.match(notes[0], /kept in this browser/);
  // The browser keeps it all the same.
  assert.equal(await sizeOf(page, 'notes'), 'L');
  await page.done();
});
