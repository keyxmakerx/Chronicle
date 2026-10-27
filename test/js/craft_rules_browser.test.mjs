// craft_rules_browser.test.mjs — the crafting rules editor in a real
// Chromium, mounted by boot.js the way the Design Lab mounts it. The demo
// calendar draws its timeline and says on which days the recipe can't be
// made; a rule opens and changes; a common rule and rules pasted from an AI
// arrive (the pasted ones as drafts); the brief copied for an AI reads back
// as the same rules; How it looks places an effect on a spot; a real
// calendar is read from Chronicle's calendar API, and an unreadable one
// falls back to the demo; destroying the widget empties it.
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
const SCRIPTS = ['item_look_fx', 'craft_rules_logic', 'craft_rules_stage', 'craft_rules_editor', 'craft_rules_looks', 'craft_rules'].map((f) => `/static/js/widgets/${f}.js`);
const TYPES = { '.css': 'text/css', '.js': 'application/javascript', '.woff2': 'font/woff2', '.png': 'image/png', '.jpg': 'image/jpeg', '.svg': 'image/svg+xml' };

function html(attrs, motion) {
  return `<!doctype html><html${motion ? ` data-motion="${motion}"` : ''}><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css">
<style>body{margin:0;padding:16px}main{max-width:1100px}</style></head><body><main><div data-widget="craft_rules" ${attrs}></div></main>
<script id="chronicle-widget-scripts" type="application/json">${JSON.stringify({ craft_rules: SCRIPTS })}</script>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} };</script>
<script src="/static/js/widgets/sky_world.js"></script>
<script src="/static/js/boot.js"></script>
</body></html>`;
}

// api: (url) => body for requests outside /static, standing in for Chronicle's API.
async function open(t, { attrs = '', api = null, motion = '', width = 1200 } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  const errors = [], asked = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(attrs, motion) });
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
  await page.goto('http://lab.test/', { waitUntil: 'domcontentloaded', timeout: 90000 });
  await page.waitForSelector('.crr-rlist');
  return { page, errors, asked };
}
const count = (page, sel) => page.$$eval(sel, (els) => els.length);
// Clicked in the page: a busy runner can hold the page longer than a Playwright click waits.
const click = (page, sel) => page.evaluate((s) => document.querySelector(s).click(), sel);

test('the demo timeline says when the recipe can be made', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  assert.match(await page.textContent('.crr-banner'), /Demo calendar/);
  assert.equal(await count(page, '.crr-tl-day'), 30, 'thirty days ahead');
  assert.equal(await count(page, '.crr-rc'), 7, 'the Moonlit draught demo has seven rules');
  // The Ember is full two days from today, so the recipe can't be made then.
  assert.match(await page.textContent('.crr-dayp'), /Can be made/);
  await click(page, '.crr-tl-day[data-d="2"]');
  await page.waitForSelector('.crr-dayp .crr-verd.no');
  assert.equal(await page.getAttribute('.crr-tl-day[data-d="2"]', 'aria-pressed'), 'true');
  // A rule that needs the hand-set moon says it waits for the Director.
  assert.match(await page.textContent('.crr-rc[data-i="2"]'), /Waits for you to set The Veil to Open/);
  // Another recipe swaps the rules.
  await click(page, '[data-recipe="0"]');
  assert.equal(await count(page, '.crr-rc'), 1);
  assert.deepEqual(errors, []);
});

test('a rule opens, changes, and reads as its sentence', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  await click(page, '.crr-rc[data-i="0"] .crr-rhd');
  await page.waitForSelector('.crr-rc[data-i="0"].is-open');
  assert.equal(await page.getAttribute('.crr-rc[data-i="0"] .crr-rhd', 'aria-expanded'), 'true');
  await click(page, '.crr-rc[data-i="0"] [data-act="eff"][data-v="bane"]');
  assert.match(await page.textContent('.crr-rc[data-i="0"] .crr-rsent'), /project roll has a bane/);
  await click(page, '.crr-rc[data-i="0"] [data-act="chance"][data-v="1 in 4"]');
  assert.match(await page.textContent('.crr-rc[data-i="0"] .crr-rsent'), /1 in 4/);
  // Escape closes the open card.
  await page.focus('.crr-rc[data-i="0"] .crr-rhd');
  await page.keyboard.press('Escape');
  await page.waitForSelector('.crr-rc[data-i="0"]:not(.is-open)');
  assert.deepEqual(errors, []);
});

test('a common rule and pasted AI rules arrive, the pasted ones as drafts', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  await click(page, '.crr-sech [data-act="new"]');
  await page.waitForSelector('.crr-starts:not([hidden])');
  await click(page, '.crr-st[data-v="0"]');
  assert.equal(await count(page, '.crr-rc'), 8);
  assert.match(await page.textContent('.crr-rc[data-i="7"] .crr-rsent'), /The Lantern/, 'starters use the calendar’s own names');

  await click(page, '[data-act="open-paste"]');
  await page.fill('.crr-area', '```json\n{"rules":[{"when":{"kind":"moon","moon":"The Ember","phase":"new"},"then":{"kind":"edge"},"players":"whenItHappens"},{"when":{"kind":"comet"},"then":{"kind":"edge"}}]}\n```');
  await click(page, '[data-act="read-paste"]');
  assert.match(await page.textContent('.crr-paste-msg'), /Added 1 draft below\. 1 couldn’t be read/);
  assert.equal(await count(page, '.crr-rc.is-draft'), 1);
  await click(page, '.crr-rc.is-draft [data-act="approve"]');
  assert.equal(await count(page, '.crr-rc.is-draft'), 0);
  assert.match(await page.textContent('.crr-rc[data-i="8"] .crr-rsent'), /The Ember/);
  // Words that aren't rule data say so.
  await page.fill('.crr-area', 'make it spooky');
  await click(page, '[data-act="read-paste"]');
  assert.match(await page.textContent('.crr-paste-msg'), /isn’t rule data/);
  assert.deepEqual(errors, []);
});

test('the brief copied for an AI reads back as the same rules', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  // This page isn't a secure context, so there is no clipboard: the brief is
  // shown to select and copy by hand, as on a self-hosted server without HTTPS.
  assert.equal(await page.evaluate(() => !!navigator.clipboard), false);
  await click(page, '[data-act="copy-ai"]');
  await page.waitForSelector('.crr-brief:not([hidden])');
  assert.match(await page.textContent('.crr-paste-msg'), /Select the text below/);
  const out = await page.evaluate(() => {
    const brief = JSON.parse(document.querySelector('.crr-brief').textContent);
    const back = CraftRules.parseAI(JSON.stringify({ rules: brief.rules }), { spots: ItemLookFX.MOMENT_SPOTS, looks: ItemLookFX.LOOKS });
    return { moons: brief.calendar.moons.map((m) => m.name), rules: brief.rules.length, back: back.rules.length, dropped: back.dropped };
  });
  assert.deepEqual(out.moons, ['The Lantern', 'The Ember', 'The Veil']);
  assert.equal(out.rules, 7);
  assert.equal(out.back, 7);
  assert.equal(out.dropped, 0);
  assert.deepEqual(errors, []);
});

test('How it looks puts an effect on a spot', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  await click(page, '[data-tab="looks"]');
  await page.waitForSelector('.crr-stage canvas');
  assert.equal(await page.isVisible('[data-pane="rules"]'), false);
  assert.equal(await count(page, '.crr-spot'), 5);
  // Tap an effect, then a spot: the phone way, which a pointer can use too.
  await click(page, '.crr-lk2[data-k="frost"]');
  assert.equal(await page.getAttribute('.crr-lk2[data-k="frost"]', 'aria-pressed'), 'true');
  await click(page, '.crr-spot[data-spot="window"]');
  assert.match(await page.textContent('.crr-spotlist'), /Frost shard/);
  assert.match(await page.getAttribute('.crr-spot[data-spot="window"]', 'aria-label'), /Frost shard/);
  assert.equal(await page.isVisible('[data-act="reset"]'), true, 'a changed moment offers the default back');
  await click(page, '[data-act="reset"]');
  assert.doesNotMatch(await page.textContent('.crr-spotlist'), /Frost shard/);
  // A rule's moment is in the rail; a rule only the Director sees is not.
  const rail = await page.textContent('.crr-orail');
  assert.match(rail, /Edge on the roll/);
  assert.doesNotMatch(rail, /A quirk/);
  assert.deepEqual(errors, []);
});

test('with motion off the bench still draws', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t, { motion: 'off' });
  await click(page, '[data-tab="looks"]');
  await page.waitForSelector('.crr-stage canvas');
  await click(page, '[data-act="play"]');
  await page.waitForTimeout(300);
  assert.deepEqual(errors, []);
});

// A real calendar: a month of thirty days, a repeating event, a rolled event
// with its results, a day of weather from a custom kind.
const CAL = {
  name: 'Harbour reckoning', mode: 'fantasy', months: ['Ice', 'Thaw', 'Bloom', 'Sun', 'Gold', 'Ash'].map((n) => ({ name: n, days: 30 })),
  moons: [{ id: 1, name: 'Tide', cycle_days: 20, phase_offset: 3, color: '#9ecbff' }],
  seasons: [{ name: 'Wet', start_month: 1, start_day: 1, end_month: 3, end_day: 30 }, { name: 'Dry', start_month: 4, start_day: 1, end_month: 6, end_day: 30 }],
  current_year: 7, current_month: 2, current_day: 20,
};
function calendarAPI(u) {
  const p = u.pathname, q = u.searchParams, base = '/campaigns/c1/calendars/k1';
  if (p === base) return CAL;
  if (p === base + '/events') {
    return q.get('month') === '2' ? [{ id: 'e1', name: 'Net mending', year: 7, month: 2, day: 1, occurrences: [{ year: 7, month: 2, day: 22, skipped: false }, { year: 7, month: 2, day: 29, skipped: true }] }] : [];
  }
  if (p === base + '/events/index') return { data: [{ id: 'e1', name: 'Net mending' }, { id: 'e2', name: 'Gull omen' }], truncated: false };
  if (p === base + '/rolls') return q.get('month') === '2' ? { e2: [{ year: 7, month: 2, day: 25, result: 'Storm gulls' }] } : {};
  if (p === base + '/events/e2/roll') return { config: { results: [{ name: 'Calm gulls' }, { name: 'Storm gulls' }] }, nights: [] };
  if (p === base + '/weather/days') return q.get('month') === '2' ? [{ year: 7, month: 2, day: 21, preset_label: 'Salt fog', icon: 'fog' }] : [];
  if (p === base + '/weather/settings') return { kinds: [{ name: 'Salt fog', icon: 'fog' }] };
  return null;
}

test('a real calendar is read from the calendar API', { skip, timeout: 180000 }, async (t) => {
  const { page, errors, asked } = await open(t, { attrs: 'data-campaign-id="c1" data-calendar-id="k1"', api: calendarAPI });
  assert.match(await page.textContent('.crr-banner'), /Harbour reckoning/);
  const out = await page.evaluate(() => {
    const tl = document.querySelector('.crr-tl').textContent;
    return { tl, sentences: [...document.querySelectorAll('.crr-rsent')].map((e) => e.textContent).join(' | ') };
  });
  assert.match(out.tl, /THAW|Thaw/);
  assert.match(out.tl, /Tide/);
  assert.match(out.sentences, /Tide/, 'the sample rules are written in this calendar’s names');
  // Today is Thaw 20: the repeating event is on the 22nd but skipped on the
  // 29th, and the rolled event came up on the 25th.
  const marks = await page.$$eval('.crr-tl-cell[data-d] span[title]', (els) => els.map((e) => [+e.parentElement.getAttribute('data-d'), e.title]));
  assert.deepEqual(marks.filter((m) => /Net mending|Gull omen/.test(m[1])), [[2, 'Net mending'], [5, 'Gull omen: Storm gulls']]);
  // Both months the thirty days cross were asked for, and the rolled event's results.
  assert.ok(asked.some((a) => a.startsWith('/campaigns/c1/calendars/k1/events?year=7&month=3')));
  assert.ok(asked.includes('/campaigns/c1/calendars/k1/events/e2/roll?year=7'));
  const W = await page.evaluate(() => {
    const el = document.querySelector('[data-widget="craft_rules"]');
    // The world the editor built, read back through the AI brief it copies.
    return new Promise((res) => {
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: (s) => { res(JSON.parse(s)); return Promise.resolve(); } } });
      el.querySelector('[data-act="copy-ai"]').click();
    });
  });
  assert.ok(W.calendar.events.includes('Net mending') && W.calendar.events.includes('Gull omen'));
  assert.ok(W.calendar.rolledEvents.some((e) => e.name === 'Gull omen' && e.results.includes('Calm gulls') && e.results.includes('Storm gulls')));
  assert.ok(W.calendar.weather.includes('Salt fog'));
  assert.deepEqual(errors, []);
});

test('an unreadable calendar falls back to the demo and says so', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t, { attrs: 'data-campaign-id="c1" data-calendar-id="nope"' });
  assert.match(await page.textContent('.crr-banner.is-bad'), /couldn’t be read/);
  assert.equal(await count(page, '.crr-rc'), 7);
  assert.deepEqual(errors, []);
});

test('destroying the widget empties it', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await open(t);
  await click(page, '[data-tab="looks"]');
  await page.waitForSelector('.crr-stage canvas');
  const out = await page.evaluate(async () => {
    const el = document.querySelector('[data-widget="craft_rules"]');
    Chronicle.destroyWidget(el);
    await new Promise((r) => setTimeout(r, 300));
    return { html: el.innerHTML, handle: el._crr, ghosts: document.querySelectorAll('.crr-ghost').length };
  });
  assert.equal(out.html, '');
  assert.equal(out.handle, null);
  assert.equal(out.ghosts, 0);
  assert.deepEqual(errors, []);
});
