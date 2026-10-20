// calendar_sky_rail_browser.test.mjs — the owner's sky menu on the calendar.
//
// Runs calendar_view.js and the sky in a real Chromium as the campaign owner.
// The Moons button slides a menu of the sun and each moon over the calendar,
// a hidden moon carrying the amber edge, and the menu follows the sky as it
// folds; pressing a body lifts a tray over the month; each pick saves at once
// to its route; Escape puts the tray away, then the menu; the menu's last
// entry opens the moon phases; a refused save puts the old value back. A
// player's Moons button opens the phases, and they never download the menu.
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

const CAL = {
  id: 'k1', campaign_id: 'c1', mode: 'fantasy', name: 'Reckoning of the Ashen Reach',
  current_year: 1043, current_month: 2, current_day: 15, current_hour: 22, current_minute: 0,
  hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
  months: ['Frostfall', 'Ashfall', 'Greening'].map((name, i) => ({ id: i + 1, name, days: 30, sort_order: i, leap_year_days: 0 })),
  weekdays: ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven'].map((name) => ({ name })),
  sun: { kind: 'yellow' },
  moons: [
    { id: 1, name: 'Selune', cycle_days: 30.4, phase_offset: 3, color: '#e8e8f0', hidden_from_players: false },
    { id: 2, name: "Sanguin'mor", cycle_days: 47, phase_offset: 20, color: '#c0242a', tint: '0.9', hidden_from_players: true },
  ],
  seasons: [], eras: [],
};

const SKY = ['sky_world', 'sky_looks', 'sky_moon', 'sky_events', 'sky_2d', 'sky_shaders', 'sky_gl', 'sky_pane']
  .map((f) => `<script src="/static/js/widgets/${f}.js"></script>`).join('\n');
const attr = (v) => JSON.stringify(v).replace(/&/g, '&amp;').replace(/'/g, '&#39;');
const html = (role) => `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/calendar-view.css">
<style>html,body{margin:0}</style></head><body>
<main id="main-content"><div style="max-width:64rem;margin:0 auto;padding:24px 16px">
<div class="cal-v5" data-widget="calendar_view" data-campaign-id="c1" data-calendar-id="k1" data-can-edit="${role >= 3}" data-can-author-dm-only="${role >= 3}"
  data-role="${role}" data-user-id="u0" data-api-base="/campaigns/c1/calendars/k1" data-sky-rail-src="/static/js/widgets/calendar_sky_rail.js">
<div id="calendar-config" class="hidden" data-calendar='${attr(CAL)}' data-events="[]"></div>
</div></div></main>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/boot.js"></script>
<script src="/static/js/motion_rest.js"></script>
${SKY}
<script src="/static/js/widgets/calendar_era_blend.js"></script>
<script src="/static/js/widgets/calendar_view.js"></script>
</body></html>`;

// fail: a route tail ('/sun') the fake server refuses with a message.
async function page(t, { role = 3, fail = null } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1200, height: 900 } });
  const errors = [], puts = [], loaded = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.route('**/*', (route) => {
    const req = route.request(), u = new URL(req.url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(role) });
    if (u.pathname.startsWith('/static/')) {
      loaded.push(u.pathname);
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    if (req.method() === 'PUT') {
      const tail = u.pathname.replace('/campaigns/c1/calendars/k1', '');
      puts.push({ tail, body: JSON.parse(req.postData() || '{}') });
      if (fail && tail === fail) return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ message: 'The server said no.' }) });
      return route.fulfill({ status: 200, body: '' });
    }
    const body = u.pathname.endsWith('/events') ? { data: [], total: 0 } : [];
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
  });
  await p.goto('http://chronicle.test/');
  await p.waitForSelector('#cal5-stage .day');
  return { p, puts, loaded };
}

const menu = async (p) => { await p.click('#cal5-moonbtn'); await p.waitForSelector('#cal5-skyrail.open .src'); };
const settle = (p) => p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
// The nth write the page sent, once it has gone out.
const lastPut = async (p, puts, n) => { for (let i = 0; i < 100 && puts.length < n; i++) await p.waitForTimeout(20); return puts[n - 1]; };

test('the Moons button slides out a menu of the sun and each moon, the hidden moon marked', { skip }, async (t) => {
  const { p } = await page(t);
  await p.waitForSelector('#cal5-skyrail .src', { state: 'attached' });
  assert.equal(await p.isVisible('#cal5-skyrail .src'), false, 'the menu starts put away');
  const headTop = await p.$eval('#cal5-cal > .head', (h) => h.getBoundingClientRect().top);
  await menu(p);
  assert.equal(await p.$eval('#cal5-moonbtn', (b) => b.getAttribute('aria-expanded')), 'true');
  const chips = await p.$$eval('#cal5-skyrail .src', (cs) => cs.map((c) => ({ id: c.dataset.sr, name: c.querySelector('.srn').textContent, dm: c.hasAttribute('data-dm-only') })));
  assert.deepEqual(chips, [
    { id: 'sun', name: 'The sun', dm: false },
    { id: '1', name: 'Selune', dm: false },
    { id: '2', name: "Sanguin'mor", dm: true },
  ]);
  await settle(p);
  const at = await p.evaluate(() => {
    const btn = document.querySelector('#cal5-moonbtn').getBoundingClientRect(), bar = document.querySelector('#cal5-skyrail .srin').getBoundingClientRect();
    return { belowButton: bar.top >= btn.bottom, head: document.querySelector('#cal5-cal > .head').getBoundingClientRect().top };
  });
  assert.deepEqual(at, { belowButton: true, head: headTop }, 'the menu floats under the button without moving the calendar');
  assert.equal(await p.$('#cal5-skyrail [data-sr-player]'), null, 'seeing it as a player is the deck’s job');
});

test('folding the sky carries the menu up with it', { skip }, async (t) => {
  const { p } = await page(t);
  await menu(p);
  await settle(p);
  const before = await p.$eval('#cal5-skyrail .srin', (e) => e.getBoundingClientRect().top);
  await p.click('#cal5-skybtn');
  await settle(p);
  await p.waitForTimeout(100);
  const after = await p.$eval('#cal5-skyrail .srin', (e) => e.getBoundingClientRect().top);
  const btn = await p.$eval('#cal5-moonbtn', (b) => b.getBoundingClientRect().bottom);
  assert.ok(after < before - 40, `the menu rose with the sky (${before} -> ${after})`);
  assert.ok(after >= btn, 'and still hangs under the button');
  assert.equal(await p.$eval('#cal5-skyrail', (r) => r.classList.contains('open')), true);
});

test('a moon opens a tray over the month and its strength saves when let go', { skip }, async (t) => {
  const { p, puts } = await page(t);
  await menu(p);
  await p.click('#cal5-sr-2');
  await p.waitForSelector('#cal5-skytray.open [data-sr-strength]');
  await settle(p);
  const over = await p.evaluate(() => {
    const tray = document.querySelector('#cal5-skytray').getBoundingClientRect(), stage = document.querySelector('#cal5-stage').getBoundingClientRect();
    return tray.bottom > stage.top && tray.top < stage.bottom;
  });
  assert.ok(over, 'the tray hovers over the month');
  await p.$eval('[data-sr-strength]', (r) => { r.value = '0.5'; r.dispatchEvent(new Event('input', { bubbles: true })); r.dispatchEvent(new Event('change', { bubbles: true })); });
  const put = await lastPut(p, puts, 1);
  assert.deepEqual(put, { tail: '/moons/2/look', body: { color: '#c0242a', strength: 0.5 } });
  await p.waitForSelector('#cal5-skytray .sts:has-text("Saved")');
  assert.equal(await p.$eval('#cal5-sr-2 .srr', (e) => e.style.getPropertyValue('--p')), '50');
});

test('the sun saves its kind at once, and Escape or a press elsewhere puts the tray away', { skip }, async (t) => {
  const { p, puts } = await page(t);
  await menu(p);
  await p.click('#cal5-sr-sun');
  await p.waitForSelector('#cal5-skytray.open [data-sr-kind="red"]');
  await p.click('[data-sr-kind="red"]');
  const put = await lastPut(p, puts, 1);
  assert.deepEqual(put, { tail: '/sun', body: { kind: 'red', color: null } });
  assert.equal(await p.$eval('[data-sr-kind="red"]', (b) => b.getAttribute('aria-checked')), 'true');
  await p.keyboard.press('Escape');
  await p.waitForSelector('#cal5-skytray:not(.open)', { state: 'attached' });
  await settle(p);
  assert.equal(await p.evaluate(() => document.activeElement && document.activeElement.id), 'cal5-sr-sun');
  await p.click('#cal5-sr-1');
  await p.waitForSelector('#cal5-skytray.open');
  await settle(p);
  await p.click('#cal5-stage .day >> nth=20');
  await p.waitForSelector('#cal5-skytray:not(.live)', { state: 'attached' });
  assert.equal(await p.$eval('#cal5-skyrail', (r) => r.classList.contains('open')), false, 'a press elsewhere puts the menu away too');
  await menu(p);
  await p.focus('#cal5-sr-sun');
  await p.keyboard.press('Escape');
  assert.equal(await p.$eval('#cal5-skyrail', (r) => r.classList.contains('open')), false);
  assert.equal(await p.evaluate(() => document.activeElement && document.activeElement.id), 'cal5-moonbtn');
});

test('hiding a moon saves and marks it, and the menu leads on to the moon phases', { skip }, async (t) => {
  const { p, puts } = await page(t);
  await menu(p);
  await p.click('#cal5-sr-1');
  await p.click('#cal5-skytray [data-sr-hidden]');
  const put = await lastPut(p, puts, 1);
  assert.deepEqual(put, { tail: '/moons/1/hidden', body: { hidden: true } });
  await p.waitForSelector('#cal5-sr-1[data-dm-only]');
  await p.click('#cal5-skyrail [data-sr-phases]');
  await p.waitForSelector('#cal5-mv.open');
  assert.equal(await p.$eval('#cal5-skyrail', (r) => r.classList.contains('open')), false);
  assert.equal(await p.$eval('#cal5-skytray', (t) => t.classList.contains('open')), false);
});

test('a refused save puts the old colour strength back and says why', { skip }, async (t) => {
  const { p, puts } = await page(t, { fail: '/moons/2/look' });
  await menu(p);
  await p.click('#cal5-sr-2');
  await p.waitForSelector('#cal5-skytray.open [data-sr-strength]');
  await p.$eval('[data-sr-strength]', (r) => { r.value = '0.2'; r.dispatchEvent(new Event('input', { bubbles: true })); r.dispatchEvent(new Event('change', { bubbles: true })); });
  await lastPut(p, puts, 1);
  await p.waitForFunction(() => document.querySelector('[data-sr-strength]') && document.querySelector('[data-sr-strength]').value === '0.9');
  await p.waitForFunction(() => document.body.textContent.includes('The server said no.'));
});

test('a player’s Moons button opens the phases, and they never load the menu', { skip }, async (t) => {
  const { p, loaded } = await page(t, { role: 1 });
  await p.waitForTimeout(300);
  assert.equal(await p.$('#cal5-skyrail'), null);
  assert.equal(await p.$('#cal5-skytray'), null);
  await p.click('#cal5-moonbtn');
  await p.waitForSelector('#cal5-mv.open');
  assert.ok(!loaded.includes('/static/js/widgets/calendar_sky_rail.js'));
});
