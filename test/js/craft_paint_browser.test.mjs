// craft_paint_browser.test.mjs — the painted forge (craft_paint widget) in a real
// Chromium, mounted by boot.js the way a page mounts it. Every effect level
// paints the lit forge, and the richer levels add light the plain scene lacks;
// a device with few cores starts at canvas glow, and slow frames step the level
// down to a still frame with Play; only one stage
// plays at a time and one off screen pauses; Calm waits for Play and Off shows
// only the still frame; the window reads the calendar as players see it; a
// device the page already found slow starts below the graphics card; and
// destroying the widget empties it.
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
const SCRIPTS = ['sky_world', 'sky_looks', 'sky_moon', 'sky_events', 'sky_2d', 'sky_shaders', 'gl_shared', 'sky_gl', 'sky_pane', 'craft_paint', 'craft_paint_forge', 'craft_paint_stage'];
const TYPES = { '.css': 'text/css', '.js': 'application/javascript' };

function html(mounts, htmlAttrs) {
  return `<!doctype html><html ${htmlAttrs}><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css">
<style>html,body{margin:0;background:#111}main{width:640px}.gap{height:1600px}</style></head><body><main>
${mounts.map((a) => `<div data-widget="craft_paint" data-name="the forge" ${a}></div>`).join('\n')}
<div class="gap"></div><p id="end">end</p></main>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} }; window.SkyGLAllowSoftware = true;</script>
<script src="/static/js/boot.js"></script>
${SCRIPTS.map((f) => `<script src="/static/js/widgets/${f}.js"></script>`).join('\n')}
</body></html>`;
}

async function open(t, { mounts = ['data-level="canvas"'], htmlAttrs = '', api = null, init = null } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width: 640, height: 820 } });
  const errors = [], asked = [];
  page.on('pageerror', (e) => errors.push(e.message));
  if (init) await page.addInitScript(init);
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(mounts, htmlAttrs) });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: TYPES[u.pathname.slice(u.pathname.lastIndexOf('.'))] || 'application/octet-stream', body: readFileSync(file) });
    }
    asked.push(u.pathname + u.search);
    const body = api && api(u);
    if (body != null) return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
    return route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
  });
  page.setDefaultTimeout(60000);
  await page.goto('http://paint.test/', { waitUntil: 'domcontentloaded', timeout: 90000 });
  await page.waitForFunction(() => document.querySelectorAll('.cpaint').length > 0);
  return { page, errors, asked };
}

// The average brightness of the stage's canvas, 0 to 255; the stage's canvas is always 2D.
const brightness = (page, n = 0) => page.evaluate((n) => {
  const cv = document.querySelectorAll('.cpaint-cv')[n], d = cv.getContext('2d').getImageData(0, 0, cv.width, cv.height).data;
  let s = 0;
  for (let i = 0; i < d.length; i += 16) s += d[i] * .3 + d[i + 1] * .59 + d[i + 2] * .11;
  return s / (d.length / 16);
}, n);
const state = (page, n = 0) => page.evaluate((n) => {
  const el = document.querySelectorAll('[data-widget="craft_paint"]')[n], cover = el.querySelector('.cpaint-cover');
  return { level: el.getAttribute('data-level'), running: el._paint.running(), cover: cover.hidden ? null : cover.textContent.trim(),
    pause: !el.querySelector('.cpaint-pause').hidden, steps: el._paint.steps() };
}, n);
const frames = (page, n) => page.evaluate((n) => new Promise((r) => { let k = 0; (function f() { if (++k > n) r(); else requestAnimationFrame(f); })(); }), n);

test('every level paints the lit forge, and the richer levels add light', { skip, timeout: 240000 }, async (t) => {
  const { page, errors } = await open(t, { mounts: ['data-level="basic" data-lab'] });
  const seen = {};
  for (const level of ['basic', 'canvas', 'gpu', 'still']) {
    await page.evaluate((l) => { const el = document.querySelector('[data-widget="craft_paint"]'); el._paint.setLevel(l); if (l !== 'still') el._paint.play(); }, level);
    await frames(page, 20);
    const s = await state(page);
    seen[level] = { level: s.level, light: await brightness(page) };
  }
  assert.equal(seen.basic.level, 'basic');
  assert.equal(seen.canvas.level, 'canvas');
  assert.equal(seen.gpu.level, 'gpu', 'this browser gives the shared WebGL2 context');
  assert.equal(seen.still.level, 'still');
  for (const l of Object.keys(seen)) assert.ok(seen[l].light > 18, `${l} paints a lit room, not a black box (${seen[l].light})`);
  assert.ok(seen.canvas.light > seen.basic.light, 'canvas glow adds bloom and rays');
  assert.ok(seen.gpu.light > seen.basic.light, 'the graphics card adds bloom and rays');
  const still = await state(page);
  assert.equal(still.running, false);
  assert.match(still.cover, /Play the forge/);
  assert.deepEqual(errors, []);
});

test('slow frames step the level down to a still frame with Play', { skip, timeout: 240000 }, async (t) => {
  // A device with few cores by its own figures starts at canvas glow, and the readout says why.
  const { page, errors } = await open(t, { mounts: ['data-lab'], init: () => Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 2 }) });
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]')._paint.running());
  const start = await state(page);
  assert.equal(start.level, 'canvas');
  assert.match(start.steps.join(' '), /few cores/);
  await page.click('.cpaint-slow');
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]').getAttribute('data-level') === 'still', null, { timeout: 120000 });
  const s = await state(page);
  assert.equal(s.running, false);
  assert.match(s.cover, /Paused to keep this page smooth/);
  assert.equal(s.steps.filter((x) => /stepped down/.test(x)).length, 2, s.steps.join(' | '));
  assert.match(await page.textContent('.cpaint-read'), /Still frame/);
  // The Lab's pretend slowness never tells the rest of the page the device is slow.
  assert.equal(await page.evaluate(() => sessionStorage.getItem('chronicle.sky.basic')), null);
  // Play anyway runs the plainest moving level and stops judging.
  await page.click('.cpaint-slow');
  await page.click('.cpaint-cover');
  const after = await state(page);
  assert.equal(after.level, 'basic');
  assert.equal(after.running, true);
  assert.deepEqual(errors, []);
});

test('one stage plays at a time, and one off screen pauses', { skip, timeout: 240000 }, async (t) => {
  const { page, errors } = await open(t, { mounts: ['data-level="canvas"', 'data-level="canvas"'] });
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]')._paint.running());
  let a = await state(page, 0), b = await state(page, 1);
  assert.equal(b.running, false, 'the second waits');
  assert.match(b.cover, /Play the forge/);
  await page.locator('.cpaint-cover').nth(1).click();
  a = await state(page, 0); b = await state(page, 1);
  assert.equal(b.running, true);
  assert.equal(a.running, false, 'starting one pauses the other');
  assert.match(a.cover, /Play the forge/);
  // Clicking a playing scene starts a fresh run of strikes, and never pauses it.
  await page.locator('.cpaint').nth(1).click({ position: { x: 200, y: 120 } });
  assert.equal((await state(page, 1)).running, true);
  const away = () => page.evaluate(() => document.getElementById('end').scrollIntoView());
  const back = () => page.evaluate(() => document.querySelectorAll('.cpaint')[1].scrollIntoView());
  await away();
  await page.waitForFunction(() => !document.querySelectorAll('[data-widget="craft_paint"]')[1]._paint.running());
  // The pause button pauses for good: back in view, it stays paused.
  await back();
  await page.waitForFunction(() => document.querySelectorAll('[data-widget="craft_paint"]')[1]._paint.running());
  await page.locator('.cpaint-pause').nth(1).click();
  await away();
  await frames(page, 10);
  await back();
  await frames(page, 10);
  assert.equal((await state(page, 1)).running, false);
  assert.deepEqual(errors, []);
});

test('Calm motion waits for Play', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t, { htmlAttrs: 'data-view-motion="calm"' });
  await frames(page, 30);
  const s = await state(page);
  assert.equal(s.running, false);
  assert.match(s.cover, /Calm motion is on/);
  await page.click('.cpaint-cover');
  assert.equal((await state(page)).running, true);
  assert.deepEqual(errors, []);
});

test('Off motion shows only the still frame', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t, { htmlAttrs: 'data-motion="off"', mounts: ['data-level="gpu"'] });
  await frames(page, 30);
  await page.locator('.cpaint').click({ position: { x: 200, y: 120 } });
  const s = await state(page);
  assert.equal(s.level, 'still');
  assert.equal(s.running, false);
  assert.equal(s.cover, null, 'no Play button');
  assert.equal(s.pause, false);
  assert.ok(await brightness(page) > 18, 'the still frame is the lit forge');
  assert.deepEqual(errors, []);
});

test('the window reads the calendar as players see it, and a device found slow starts below the graphics card', { skip, timeout: 180000 }, async (t) => {
  const cal = { name: 'Therin', current_year: 1491, current_month: 1, current_day: 14, current_hour: 21, current_minute: 40, hours_per_day: 24, minutes_per_hour: 60,
    months: [{ name: 'Hammer', days: 30 }], weekdays: [{ name: 'One' }], moons: [{ id: 1, name: 'Selune', cycle_days: 30, phase_offset: 0, color: '#d9e1ec', size: 1 }] };
  const { page, errors, asked } = await open(t, {
    mounts: ['data-campaign-id="c1" data-calendar-id="k1"'],
    // A strong device by its own figures, so only the session's slow verdict lowers the start.
    init: () => {
      Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 16 });
      Object.defineProperty(navigator, 'deviceMemory', { get: () => 16 });
      sessionStorage.setItem('chronicle.sky.basic', '1');
    },
    api: (u) => (u.pathname === '/campaigns/c1/calendars/k1' ? cal : u.pathname.endsWith('/weather/days') ? [] : null),
  });
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]')._paint.running());
  const s = await state(page);
  assert.equal(s.level, 'canvas');
  assert.match(s.steps.join(' '), /already found slow/);
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]')._paint.running());
  await frames(page, 10);
  assert.ok(asked.includes('/campaigns/c1/calendars/k1?audience=players'), asked.join(', '));
  assert.ok(asked.some((x) => x.startsWith('/campaigns/c1/calendars/k1/weather/days?') && x.includes('audience=players')), asked.join(', '));
  assert.ok(!asked.some((x) => x === '/campaigns/c1/calendars/k1'), 'never the Director\'s view of the calendar');
  assert.deepEqual(errors, []);
});

test('destroying the widget empties it and stops its scene', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_paint"]')._paint.running());
  const out = await page.evaluate(async () => {
    const el = document.querySelector('[data-widget="craft_paint"]');
    Chronicle.destroyWidget(el);
    await new Promise((r) => setTimeout(r, 300));
    return { html: el.innerHTML, handle: el._paint };
  });
  assert.equal(out.html, '');
  assert.equal(out.handle, null);
  assert.deepEqual(errors, []);
});
