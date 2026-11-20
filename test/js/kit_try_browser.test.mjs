// kit_try_browser.test.mjs — "Try a style kit here": a tried kit reaches
// the page the way a saved one does, nothing is written until Keep, the
// try-out follows a reload in the same tab, and Put it back (or Escape)
// returns the page exactly as the server drew it.
//
// Runs static/js/kit_try.js in a real Chromium against a stubbed server.
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips
// locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const KITS = [['material', 'Material'], ['sharp', 'Sharp'], ['ornate', 'Ornate'], ['flowing', 'Flowing'], ['lumen', 'Lumen'], ['sketchbook', 'Sketchbook']]
  .map(([id, label]) => ({ id, label }));
const CSS = ':root[data-kit-try="ornate"]{--kit-hf:serif;}';

// The server drew the saved kit (Sketchbook) on <html>; the page has a
// title, so a kit dresses it, unless it is a workshop.
const page = (workshop) => `<!doctype html><html data-cz-page="annals" data-cz-kit="sketchbook"><head>
<script>
window.writes = [];
window.Chronicle = { apiFetch: function (url, opts) {
  if (opts && opts.method) {
    window.writes.push(opts.method + ' ' + url + ' ' + JSON.stringify(opts.body));
    sessionStorage.setItem('writes', JSON.stringify(window.writes));
  }
  var body = /keep$/.test(url) ? { href: '/kept' } : { kits: ${JSON.stringify(KITS)}, css: ${JSON.stringify(CSS)}, entrance: 'bloom' };
  return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(body); } });
} };
</script>
<script src="/kit_try.js"></script></head><body>
<span data-topbar-name>Saltgrave</span>
<main id="main-content"><div class="cb-title"><h1>Saltgrave Harbour</h1></div>${workshop ? '<div data-workshop></div>' : ''}</main>
<button id="brush">brush</button>
</body></html>`;

const attrs = () => [document.documentElement.getAttribute('data-cz-kit'), document.documentElement.getAttribute('data-cz-entrance'), document.documentElement.getAttribute('data-kit-try')];

async function withTab(fn) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const tab = await browser.newPage();
    await tab.route('http://t.test/**', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/kit_try.js') return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/js/kit_try.js')) });
      return route.fulfill({ contentType: 'text/html; charset=utf-8', body: page(p === '/workshop') });
    });
    await fn(tab);
  } finally { await browser.close(); }
}

test('a kit dresses a titled page but never a workshop', { skip }, async () => {
  await withTab(async (tab) => {
    await tab.goto('http://t.test/');
    assert.equal(await tab.evaluate(() => Chronicle.kitTry.dressed()), true);
    await tab.goto('http://t.test/workshop');
    assert.equal(await tab.evaluate(() => Chronicle.kitTry.dressed()), false);
  });
});

test('a tried kit is drawn as a saved one, and Put it back restores the page', { skip }, async () => {
  await withTab(async (tab) => {
    await tab.goto('http://t.test/');
    await tab.evaluate(() => Chronicle.kitTry.open({ url: '/c/customize/kit-try', campaign: 'c1', onEnd: () => document.getElementById('brush').focus() }));
    // The kit the page wears is marked and focused, and nothing changed yet.
    assert.equal(await tab.evaluate(() => document.activeElement.getAttribute('data-kit')), 'sketchbook');
    assert.deepEqual(await tab.evaluate(attrs), ['sketchbook', null, 'sketchbook']);
    assert.match(await tab.textContent('[data-kit-try-label]'), /Saltgrave Harbour in Sketchbook · as saved/);
    assert.equal(await tab.isDisabled('[data-kit-keep]'), true);

    await tab.keyboard.press('ArrowLeft');
    await tab.keyboard.press('ArrowLeft');
    assert.equal(await tab.evaluate(() => document.activeElement.getAttribute('data-kit')), 'flowing');
    await tab.click('[data-kit="ornate"]');
    assert.deepEqual(await tab.evaluate(attrs), ['ornate', null, 'ornate']);
    assert.equal(await tab.evaluate(() => document.querySelector('style[data-kit-try-css]').textContent), CSS);
    assert.match(await tab.textContent('[data-kit-try-label]'), /Saltgrave Harbour in Ornate · not saved/);
    // Material takes the card entrance back, as the server writes it only then.
    await tab.click('[data-kit="material"]');
    assert.deepEqual(await tab.evaluate(attrs), [null, 'bloom', 'material']);
    assert.deepEqual(await tab.evaluate(() => window.writes), [], 'trying writes nothing');

    // A reload in the same tab carries the try-out over.
    await tab.click('[data-kit="lumen"]');
    await tab.reload();
    await tab.evaluate(() => Chronicle.kitTry.resume({ url: '/c/customize/kit-try', campaign: 'c1', onEnd: () => document.getElementById('brush').focus() }));
    assert.deepEqual(await tab.evaluate(attrs), ['lumen', null, 'lumen']);
    assert.equal(await tab.isVisible('[data-kit-try-strip]'), true);

    await tab.focus('[data-kit="lumen"]');
    await tab.keyboard.press('Escape');
    assert.deepEqual(await tab.evaluate(attrs), ['sketchbook', null, null]);
    assert.equal(await tab.evaluate(() => !!document.querySelector('[data-kit-try-strip], style[data-kit-try-css]')), false);
    assert.equal(await tab.evaluate(() => sessionStorage.getItem('chronicle-kit-try')), null);
    assert.equal(await tab.evaluate(() => document.activeElement.id), 'brush');
  });
});

test('Keep this kit is the only write, and leads where the server says', { skip }, async () => {
  await withTab(async (tab) => {
    await tab.goto('http://t.test/');
    await tab.evaluate(() => Chronicle.kitTry.open({ url: '/c/customize/kit-try', campaign: 'c1' }));
    await tab.click('[data-kit="ornate"]');
    await Promise.all([tab.waitForURL('http://t.test/kept'), tab.click('[data-kit-keep]')]);
    assert.equal(await tab.evaluate(() => sessionStorage.getItem('chronicle-kit-try')), null);
    assert.deepEqual(await tab.evaluate(() => JSON.parse(sessionStorage.getItem('writes'))), ['POST /c/customize/kit-try/keep {"kit":"ornate"}']);
  });
});

// Customize › Look's example frame: the parent dresses the framed page; the
// frame never takes the tab's try-out, and nothing is stored.
const framed = `<!doctype html><html data-cz-page="annals" data-cz-kit="sketchbook"><head><meta charset="utf-8">
<script>window.Chronicle = { apiFetch: function () { return Promise.reject(new Error('no fetch in a frame')); } };</script>
<script src="/kit_try.js"></script></head><body><main id="main-content"><div class="cb-title"><h1>Brother Odwin</h1></div></main></body></html>`;
const parent = `<!doctype html><html><head><meta charset="utf-8">
<script>window.Chronicle = {};</script><script src="/kit_try.js"></script></head><body>
<iframe id="f" src="/c1/framed"></iframe></body></html>`;

test('dress puts a kit on a framed page, stores nothing and shows no strip', { skip }, async () => {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const tab = await browser.newPage();
    await tab.route('http://t.test/**', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/kit_try.js') return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/js/kit_try.js')) });
      if (p === '/c1/framed') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: framed });
      return route.fulfill({ contentType: 'text/html; charset=utf-8', body: parent });
    });
    // The tab has a try-out of its own, which the frame must leave alone.
    const held = JSON.stringify({ c: 'c1', url: '/campaigns/c1/customize/kit-try', kit: 'lumen', kits: KITS, css: CSS, entrance: '' });
    await tab.goto('http://t.test/');
    await tab.evaluate((v) => sessionStorage.setItem('chronicle-kit-try', v), held);
    await tab.evaluate(() => new Promise((r) => { const f = document.getElementById('f'); f.onload = r; f.src = '/c1/framed?x=1'; }));
    const frame = tab.frames().find((f) => f.url().includes('/c1/framed'));
    await frame.evaluate(() => Chronicle.kitTry.resume({ url: '/campaigns/c1/customize/kit-try', campaign: 'c1' }));
    assert.deepEqual(await frame.evaluate(attrs), ['sketchbook', null, null], 'a framed page never takes the tab try-out');

    const dressed = () => tab.evaluate(() => {
      const d = document.getElementById('f').contentDocument, h = d.documentElement;
      return {
        attrs: [h.getAttribute('data-cz-kit'), h.getAttribute('data-cz-entrance'), h.getAttribute('data-kit-try')],
        frame: h.hasAttribute('data-kit-frame'), saved: h.getAttribute('data-kit-saved'),
        styles: d.querySelectorAll('style[data-kit-try-css]').length, inert: d.body.inert,
        strip: !!d.querySelector('[data-kit-try-strip]'),
      };
    });
    await tab.evaluate((css) => Chronicle.kitTry.dress(document.getElementById('f').contentDocument, { kit: 'ornate', css, entrance: 'bloom' }), CSS);
    assert.deepEqual(await dressed(), { attrs: ['ornate', null, 'ornate'], frame: true, saved: 'sketchbook', styles: 1, inert: true, strip: false });
    // A second press swaps the kit in place: Material takes the entrance back.
    await tab.evaluate((css) => Chronicle.kitTry.dress(document.getElementById('f').contentDocument, { kit: 'material', css, entrance: 'bloom' }), CSS);
    assert.deepEqual(await dressed(), { attrs: [null, 'bloom', 'material'], frame: true, saved: 'sketchbook', styles: 1, inert: true, strip: false });
    assert.equal(await tab.evaluate(() => sessionStorage.getItem('chronicle-kit-try')), held, 'the tab try-out is untouched');
    assert.equal(await tab.evaluate(() => !!document.querySelector('[data-kit-try-strip]')), false, 'the parent shows no strip');
  } finally { await browser.close(); }
});

test('walk opens a page with the try-out on in the kit', { skip }, async () => {
  await withTab(async (tab) => {
    await tab.goto('http://t.test/look');
    await Promise.all([tab.waitForURL('http://t.test/c1/page'), tab.evaluate((k) => Chronicle.kitTry.walk({ url: '/c/customize/kit-try', campaign: 'c1', kit: 'flowing', href: '/c1/page', kits: k, css: '', entrance: '' }), KITS)]);
    const s = await tab.evaluate(() => JSON.parse(sessionStorage.getItem('chronicle-kit-try')));
    assert.deepEqual([s.c, s.kit, s.url], ['c1', 'flowing', '/c/customize/kit-try']);
  });
});

// kit_try_early.js: the try-out is on before the first paint, where it may be.
const early = (ok) => `<!doctype html><html data-cz-page="annals" data-cz-kit="sketchbook" data-cz-entrance="bloom"${ok ? ' data-kit-try-ok' : ''}><head><meta charset="utf-8">
<script src="/kit_try_early.js"></script>
<script>var h = document.documentElement; window.seen = [h.getAttribute('data-cz-kit'), h.getAttribute('data-cz-entrance'), h.getAttribute('data-kit-try'), h.getAttribute('data-kit-saved'), h.getAttribute('data-kit-saved-entrance'), !!document.querySelector('style[data-kit-try-css]')];</script>
</head><body>${ok === 'frame' ? '<iframe id="f" src="/campaigns/c1/framed"></iframe>' : ''}</body></html>`;

test('kit_try_early puts an open try-out on before the first paint, only where it may', { skip }, async () => {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  try {
    const tab = await browser.newPage();
    await tab.route('http://t.test/**', (route) => {
      const p = new URL(route.request().url()).pathname;
      if (p === '/kit_try_early.js') return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/js/kit_try_early.js')) });
      if (p === '/campaigns/c1/no-brush') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: early(false) });
      if (p === '/campaigns/c1/with-frame') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: early('frame') });
      return route.fulfill({ contentType: 'text/html; charset=utf-8', body: early(true) });
    });
    await tab.goto('http://t.test/campaigns/c1');
    await tab.evaluate(() => sessionStorage.setItem('chronicle-kit-try', JSON.stringify({ c: 'c1', url: '/campaigns/c1/customize/kit-try', kit: 'ornate', css: ':root{}', kits: [], entrance: '' })));
    const seen = async (path) => { await tab.goto('http://t.test' + path); return tab.evaluate(() => window.seen); };

    assert.deepEqual(await seen('/campaigns/c1/entities/e1'), ['ornate', null, 'ornate', 'sketchbook', 'bloom', true]);
    assert.deepEqual(await seen('/campaigns/c2/entities/e1'), ['sketchbook', 'bloom', null, null, null, false], 'another campaign');
    assert.deepEqual(await seen('/campaigns/c1/no-brush'), ['sketchbook', 'bloom', null, null, null, false], 'no brush on the page');
    await seen('/campaigns/c1/with-frame');
    const frame = tab.frames().find((f) => f.url().includes('/framed'));
    await frame.waitForFunction(() => window.seen);
    assert.deepEqual(await frame.evaluate(() => window.seen), ['sketchbook', 'bloom', null, null, null, false], 'a framed page');
  } finally { await browser.close(); }
});
