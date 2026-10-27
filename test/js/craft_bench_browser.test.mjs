// craft_bench_browser.test.mjs — the crafting bench widget in a real Chromium,
// mounted by boot.js the way a page mounts it. A play mount draws the desk,
// bakes its light and plays a step when an ingredient is dragged into the
// mortar; two benches on one page keep their ids apart; build mode opens the
// editor, and a look from the tray redraws the room; every tray picture the
// editor asks for exists; destroying the widget empties it. The desk's light
// never uses a point light, which phone GPUs draw as black (see craft_bench.ai.md).
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { skip } from './_browser_env.mjs';
import { open, baked, root } from './_craft_harness.mjs';

async function bench(t, opts) {
  const b = await open(opts);
  t.after(() => b.browser.close());
  return b;
}

test('a play mount bakes its light and plays a step', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, {});
  await baked(page);
  const from = await page.evaluate(() => {
    const box = (s) => { const r = document.querySelector(s).getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; };
    document.querySelector('.cr-stage').scrollIntoView();
    return { moss: box('[data-item="bloodmoss"]'), mortar: box('.cr-mortar') };
  });
  await page.mouse.move(...from.moss);
  await page.mouse.down();
  await page.mouse.move(...from.mortar, { steps: 12 });
  await page.mouse.up();
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_bench"]')._crb.state().i === 1, null, { timeout: 30000 });
  const out = await page.evaluate(() => ({
    pointLights: document.querySelectorAll('fePointLight, feSpotLight').length,
    say: document.querySelector('.cr-say').textContent,
  }));
  assert.equal(out.pointLights, 0, 'the desk lights with distant lights only');
  assert.match(out.say, /grind/i);
  assert.deepEqual(errors, []);
});

test('two benches on one page keep their ids apart', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: ['data-mode="play"', 'data-mode="play"'] });
  await baked(page, 0);
  await baked(page, 1);
  const out = await page.evaluate(() => {
    const svgs = [...document.querySelectorAll('.cr-svg')];
    const ids = svgs.map((s) => [...s.querySelectorAll('[id]')].map((e) => e.id));
    const all = ids.flat();
    // Every url(#…) and href="#…" in a scene points at an id in that scene.
    const strays = svgs.map((s, n) => {
      const own = new Set(ids[n]);
      const refs = [...s.outerHTML.matchAll(/(?:url\(#|href="#)([^)"]+)/g)].map((m) => m[1]);
      return refs.filter((r) => !own.has(r));
    });
    return { count: svgs.length, total: all.length, unique: new Set(all).size, strays };
  });
  assert.equal(out.count, 2);
  assert.ok(out.total > 20, 'the scenes have ids to keep apart');
  assert.equal(out.unique, out.total, 'no id appears twice on the page');
  assert.deepEqual(out.strays, [[], []]);
  assert.deepEqual(errors, []);
});

test('build mode opens the editor and a look redraws the room', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: ['data-mode="build" data-thumbs="/static/img/crafting/desk/"'] });
  await page.waitForSelector('.crb .cr-svg');
  await baked(page);
  const status = await page.evaluate(() => {
    window.__redrawn = 0;
    document.querySelector('.cr-svg').addEventListener('craft:redrawn', () => window.__redrawn++);
    return document.querySelector('.crb-status').textContent;
  });
  assert.match(status, /Nothing is saved/);
  await page.click('[data-look="hut"]');
  await page.waitForFunction(() => window.__redrawn > 0, null, { timeout: 30000 });
  const pressed = await page.getAttribute('[data-look="hut"]', 'aria-pressed');
  assert.equal(pressed, 'true');
  // Picking a piece opens its card, and the card is hidden until then.
  assert.equal(await page.isVisible('.crb-pcard'), false);
  await page.click('[data-tab="pieces"]');
  await page.click('.crb-prow[data-id="cat"]');
  await page.waitForSelector('.crb-pcard:not([hidden])');
  assert.deepEqual(errors, []);
});

test('every tray picture the editor asks for exists', { skip, timeout: 180000 }, async (t) => {
  const { page } = await bench(t, { mounts: ['data-mode="build" data-thumbs="/static/img/crafting/desk/"'] });
  await page.waitForSelector('.crb-ttabs button');
  const srcs = new Set();
  for (const tab of await page.$$('.crb-ttabs button')) {
    await tab.click();
    for (const s of await page.$$eval('.crb-th img', (imgs) => imgs.map((i) => i.getAttribute('src')))) srcs.add(s);
  }
  assert.ok(srcs.size > 20, 'the tray shows pictures');
  const missing = [...srcs].filter((s) => !existsSync(join(root, s)));
  assert.deepEqual(missing, [], 'run node tools/craft-thumbs.mjs to draw them');
});

test('destroying the widget empties it and stops its scene', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, {});
  await baked(page);
  const out = await page.evaluate(async () => {
    const el = document.querySelector('[data-widget="craft_bench"]');
    Chronicle.destroyWidget(el);
    await new Promise((r) => setTimeout(r, 500));
    return { html: el.innerHTML, handle: el._crb };
  });
  assert.equal(out.html, '');
  assert.equal(out.handle, null);
  assert.deepEqual(errors, []);
});

// The paper journal skin (craft_journal.js) on the same engine.
const JOURNAL = 'data-mode="play" data-skin="journal"';
const ready = (page, n = 0) => page.waitForFunction((n) => {
  const st = document.querySelectorAll('[data-widget="craft_bench"] .cr-stage')[n];
  return !!st && st.classList.contains('pj-ready');
}, n, { timeout: 90000 });

test('the journal draws its page, plays a step and keeps its ids apart from a desk', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: [JOURNAL, 'data-mode="play"'] });
  await ready(page);
  await baked(page, 1);
  const from = await page.evaluate(() => {
    const st = document.querySelector('.crj');
    st.scrollIntoView();
    const box = (s) => { const r = st.querySelector(s).getBoundingClientRect(); return [r.x + r.width / 2, r.y + r.height / 2]; };
    return { moss: box('[data-item="bloodmoss"]'), mortar: box('.cr-mortar') };
  });
  await page.mouse.move(...from.moss);
  await page.mouse.down();
  await page.mouse.move(...from.mortar, { steps: 12 });
  await page.mouse.up();
  await page.waitForFunction(() => document.querySelector('[data-widget="craft_bench"]')._crb.state().i === 1, null, { timeout: 30000 });
  const out = await page.evaluate(() => {
    const svgs = [...document.querySelectorAll('.cr-svg')];
    const ids = svgs.map((s) => [...s.querySelectorAll('[id]')].map((e) => e.id)), all = ids.flat();
    const strays = svgs.map((s, n) => {
      const own = new Set(ids[n]);
      return [...s.outerHTML.matchAll(/(?:url\(#|href="#)([^)"]+)/g)].map((m) => m[1]).filter((r) => !own.has(r));
    });
    return {
      pointLights: document.querySelectorAll('fePointLight, feSpotLight').length,
      say: document.querySelector('.crj .cr-say').textContent, unique: new Set(all).size, total: all.length, strays,
      date: document.querySelector('.crj .p-date').textContent, deskPaper: document.querySelectorAll('.cr-svg')[1].querySelectorAll('.p-ink').length,
    };
  });
  assert.equal(out.pointLights, 0, 'the journal lights with distant lights only');
  assert.match(out.say, /grind/i);
  assert.equal(out.unique, out.total, 'no id appears twice on the page');
  assert.deepEqual(out.strays, [[], []]);
  assert.match(out.date, /^Frost 8, 100/, 'the date comes from the calendar');
  assert.equal(out.deskPaper, 0, 'the desk draws none of the journal');
  assert.deepEqual(errors, []);
});

test('the journal reads the players’ calendar and never draws a hidden moon', { skip, timeout: 180000 }, async (t) => {
  const asked = [];
  const months = ['Thaw', 'Seedtime', 'Rains', 'Blossom', 'Highsun', 'Harvest', 'Leaffall', 'Mists', 'Firstfrost', 'Deepwinter', 'Longnight', 'Waking'].map((name) => ({ name, days: 24 }));
  const cal = {
    id: 'cal1', name: 'Therin', mode: 'fantasy', hours_per_day: 24, minutes_per_hour: 60, current_year: 100, current_month: 9, current_day: 8, current_hour: 22,
    epoch_name: 'AS', months, weather: { preset_id: 'clear', preset_label: 'Clear' },
    moons: [{ id: 1, name: 'Lacrimosa', cycle_days: 24, phase_offset: 1, color: '#e5e7eb', phase_mode: 'auto' },
      { id: 2, name: 'Umbra', cycle_days: 30, phase_offset: 0, color: '#222222', phase_mode: 'auto', hidden_from_players: true }],
  };
  const api = (u) => {
    asked.push(u.pathname + u.search);
    if (u.pathname === '/campaigns/c1/calendars/cal1') return cal;
    if (u.pathname === '/campaigns/c1/calendars/cal1/weather/days') return [{ year: 100, month: 9, day: 8, preset_id: 'rain', preset_label: 'Rain' }];
    return null;
  };
  const { page, errors } = await bench(t, { mounts: [JOURNAL + ' data-campaign-id="c1" data-calendar-id="cal1"'], api });
  await ready(page);
  await page.waitForFunction(() => /Rain/i.test((document.querySelector('.p-date') || {}).textContent || ''), null, { timeout: 30000 });
  const out = await page.evaluate(() => {
    const svg = document.querySelector('.cr-svg'), skin = document.querySelector('[data-widget="craft_bench"]')._crb.skin;
    return { date: svg.querySelector('.p-date').textContent, moons: svg.querySelectorAll('[data-prop^="moon-"]').length, umbra: /Umbra/.test(svg.outerHTML + JSON.stringify(skin.info)), moon: skin.info['moon-0'] };
  });
  assert.ok(asked.includes('/campaigns/c1/calendars/cal1?audience=players'), 'the calendar is read as players see it');
  assert.equal(out.date, 'Firstfrost 8, 100 AS · rain');
  assert.equal(out.moons, 1);
  assert.equal(out.umbra, false, 'a moon hidden from players is never drawn or named');
  assert.equal(out.moon.name, 'Lacrimosa');
  assert.deepEqual(errors, []);
});

test('a finished recipe is sealed, and a blot stays with its recipe', { skip, timeout: 180000 }, async (t) => {
  const { page, errors } = await bench(t, { mounts: [JOURNAL] });
  await ready(page);
  const sealed = await page.evaluate(async () => {
    const c = document.querySelector('[data-widget="craft_bench"]')._crb, wait = (ms) => new Promise((r) => setTimeout(r, ms));
    for (let n = 0; n < 12; n++) {
      c.reset(); await wait(200); c.skip(); await wait(100);
      document.querySelector('.cr-acts .cr-btn-primary').click(); await wait(1500);
      if (c.state().points < c.state().r.goal) continue;
      const said = document.querySelector('.cr-say').textContent;
      [...document.querySelectorAll('.cr-acts .cr-btn')].find((b) => /seal/i.test(b.textContent)).click();
      await wait(3500);
      return { said, after: document.querySelector('.cr-say').textContent, wax: document.querySelectorAll('.cr-book .pj-wax').length };
    }
    return null;
  });
  assert.ok(sealed, 'the recipe finished');
  assert.match(sealed.said, /seal/i, 'a finished recipe asks for its seal first');
  assert.match(sealed.after, /^Finished/);
  assert.equal(sealed.wax, 1);
  const blots = await page.evaluate(async () => {
    const c = document.querySelector('[data-widget="craft_bench"]')._crb, wait = (ms) => new Promise((r) => setTimeout(r, ms)), n = () => document.querySelectorAll('.pj-blots .pj-blot').length;
    c.reset('heal'); await wait(200);
    c.skin.onFeedback('miss', 884, 400, c.state());
    const out = [n()];
    c.reset('heal'); await wait(200); out.push(n());
    c.reset('caustic'); await wait(1500); out.push(n());
    c.reset('heal'); await wait(1500); out.push(n());
    return out;
  });
  assert.deepEqual(blots, [1, 1, 0, 1], 'the blot stays on its own recipe’s page');
  assert.deepEqual(errors, []);
});

test('every journal tray picture the editor asks for exists', { skip, timeout: 180000 }, async (t) => {
  const { page } = await bench(t, { mounts: ['data-mode="build" data-skin="journal" data-thumbs="/static/img/crafting/journal/"'] });
  await page.waitForSelector('.crb-ttabs button');
  const srcs = new Set();
  for (const tab of await page.$$('.crb-ttabs button')) {
    await tab.click();
    for (const s of await page.$$eval('.crb-th img', (imgs) => imgs.map((i) => i.getAttribute('src')))) srcs.add(s);
  }
  assert.ok(srcs.size > 40, 'the tray shows pictures');
  const missing = [...srcs].filter((s) => !existsSync(join(root, s)));
  assert.deepEqual(missing, [], 'run node tools/craft-thumbs.mjs --skin=journal to draw them');
});
