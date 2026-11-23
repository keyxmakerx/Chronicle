// cutscene_engine_browser.test.mjs — a Cut scene played for real in Chromium:
// it takes the whole window as a dialog, keeps the page out of reach, shows
// its words as text and never as markup, lets the player out after two
// seconds and gives the page back as it was. At phone width and with reduced
// motion too, and without a console error.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const ENGINE = readFileSync(join(root, 'static/js/cutscene_engine.js'));

const PAGE = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<main id="main"><button id="opener">Watch</button><p>The page underneath.</p></main>
<script src="/engine.js"></script></body></html>`;

const SCENE = (look, eye) => ({
  id: 's1', name: 'The Eye Opens',
  background: { kind: 'fall-asleep', seconds: 1, params: { look } },
  overlays: [{ kind: 'eye', at: 0.5, seconds: 0.5, params: { look: eye } }, { kind: 'embers', at: 0.5, seconds: 0.5 }, { kind: 'dragon', at: 0 }],
  text: [{ say: '<img src=x onerror="window.pwned=1">', at: 0.6, seconds: 0, style: 'whisper' }, { say: 'Below.', at: 0.6, seconds: 0, style: 'title' }],
  ending: { kind: 'tap' }
});

async function open(t, viewport, reducedMotion) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport, reducedMotion: reducedMotion ? 'reduce' : 'no-preference' });
  const pg = await ctx.newPage();
  const errors = [];
  pg.on('pageerror', (e) => errors.push(String(e)));
  pg.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/engine.js') return route.fulfill({ contentType: 'text/javascript', body: ENGINE });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await pg.goto('http://t.test/');
  return { pg, errors };
}

function start(pg, scene) {
  return pg.evaluate((scene) => {
    document.getElementById('opener').focus();
    window.shut = [];
    window.run = ChronicleCutscene.play(document.body, scene, { onClose: (why) => window.shut.push(why), label: 'Cut scene: test' });
  }, scene);
}

for (const [look, eye] of [['drift', 'gather'], ['eyelids', 'almond'], ['ink', 'rift']]) {
  test(`${look} with the ${eye} eye: a dialog over everything, closed by Esc after two seconds`, { skip }, async (t) => {
    const { pg, errors } = await open(t, { width: 1280, height: 800 });
    await start(pg, SCENE(look, eye));
    const state = await pg.evaluate(() => {
      const d = document.querySelector('.cs-cut');
      const r = d.getBoundingClientRect();
      return { role: d.getAttribute('role'), modal: d.getAttribute('aria-modal'), label: d.getAttribute('aria-label'), focus: document.activeElement === d,
        cover: r.width === innerWidth && r.height === innerHeight, inert: document.getElementById('main').hasAttribute('inert'),
        surface: document.documentElement.hasAttribute('data-full-surface') };
    });
    assert.deepEqual(state, { role: 'dialog', modal: 'true', label: 'Cut scene: test', focus: true, cover: true, inert: true, surface: true });

    // No way out for the first two seconds: Esc does nothing, the close mark is hidden.
    await pg.keyboard.press('Escape');
    assert.equal(await pg.evaluate(() => !!document.querySelector('.cs-cut') && window.shut.length), 0);
    assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-x')).visibility), 'hidden');

    await pg.waitForFunction(() => window.run.time() >= 2.05);
    // Words are text, never markup.
    const words = await pg.evaluate(() => [...document.querySelectorAll('.cs-cut-line')].map((l) => l.textContent));
    assert.deepEqual(words, ['<img src=x onerror="window.pwned=1">', 'Below.']);
    assert.equal(await pg.evaluate(() => document.querySelectorAll('.cs-cut-words img').length + (window.pwned || 0)), 0);
    assert.equal(await pg.evaluate(() => getComputedStyle(document.querySelector('.cs-cut-x')).visibility), 'visible');
    // Everything has arrived, so a tap scene offers its quiet Close.
    assert.equal(await pg.evaluate(() => document.querySelector('.cs-cut-end').classList.contains('cs-cut-on')), true);

    await pg.keyboard.press('Escape');
    assert.deepEqual(await pg.evaluate(() => window.shut), ['escape']);
    await pg.waitForFunction(() => !document.querySelector('.cs-cut'));
    const after = await pg.evaluate(() => ({ focus: document.activeElement.id, inert: document.getElementById('main').hasAttribute('inert'), surface: document.documentElement.hasAttribute('data-full-surface') }));
    assert.deepEqual(after, { focus: 'opener', inert: false, surface: false });
    assert.deepEqual(errors, []);
  });
}

test('at phone width the scene fits, a tap closes it at the end, and stop() ends it quietly', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 390, height: 844 });
  await start(pg, SCENE('drift', 'gather'));
  await pg.waitForFunction(() => window.run.time() >= 2.05);
  const fit = await pg.evaluate(() => {
    const x = document.querySelector('.cs-cut-x').getBoundingClientRect(), w = document.querySelector('.cs-cut-words').getBoundingClientRect();
    return { x: x.right <= innerWidth && x.width >= 44 && x.height >= 44, words: w.left >= 0 && w.right <= innerWidth, scroll: document.documentElement.scrollWidth <= innerWidth };
  });
  assert.deepEqual(fit, { x: true, words: true, scroll: true });
  await pg.mouse.click(195, 300);
  assert.deepEqual(await pg.evaluate(() => window.shut), ['tap']);

  await start(pg, { background: { kind: 'black' }, text: [{ say: 'Wait.', at: 0, seconds: 0 }], ending: { kind: 'tap' } });
  await pg.evaluate(() => window.run.stop());
  assert.deepEqual(await pg.evaluate(() => window.shut), ['stopped']);
  await pg.waitForFunction(() => !document.querySelector('.cs-cut'));
  assert.deepEqual(errors, []);
});

test('reduced motion crossfades: no film grain, no backdrop blur, and it still closes by itself', { skip }, async (t) => {
  const { pg, errors } = await open(t, { width: 1280, height: 800 }, true);
  await start(pg, { ...SCENE('drift', 'gather'), ending: { kind: 'auto', after: 0.2 } });
  const calm = await pg.evaluate(() => ({ calm: document.querySelector('.cs-cut').classList.contains('cs-cut-calm'), grain: !!document.querySelector('.cs-cut-grain'), veil: !!document.querySelector('.cs-cut-veil') }));
  assert.deepEqual(calm, { calm: true, grain: false, veil: false });
  await pg.waitForFunction(() => window.shut.length === 1, null, { timeout: 6000 });
  assert.deepEqual(await pg.evaluate(() => window.shut), ['auto']);
  assert.deepEqual(errors, []);
});
