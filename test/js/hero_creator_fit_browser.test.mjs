// hero_creator_fit_browser.test.mjs — words and cards that must fit.
//
// Runs hero_creator.js in a real Chromium against a stubbed plan. It pins two
// layout promises: on a computer the review's stat labels never break inside
// a word (the tiles wrap to rows instead), and on a 320px phone every card in
// the hand keeps its content, "Read the rules" included, inside the card, even
// with long names in a step of 100, and a one-word name wider than the card is
// set smaller rather than cut at the edge. On a phone a mark that is a button
// is a full 44px target.
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
const C = '/campaigns/c1';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));
const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));

const LONG = 'You somehow learned far more than anyone expected, and the knowledge sits heavy: people come to you for answers you would rather not give, and some of them do not take no for an answer.';
const ch = (key, name, properties = {}, summary = LONG) => ({ key, slug: key, name, summary, source: 'package', properties });
const NAMES = ['Advanced Studies', 'Infernal Contract... But, Like, Bad', 'Slight Case of Lycanthropy', 'Following in the Footsteps', 'Amnesia', 'Misunderstood', 'Shipwrecked'];
const PLAN = () => ({ systemName: 'Draw Steel', steps: [
  { fieldKey: 'complication', label: 'Complication', choices: Array.from({ length: 100 }, (_, i) => ({ ...ch('c' + i, NAMES[i % NAMES.length] + (i >= NAMES.length ? ' ' + i : '')), ...(i % 3 === 0 ? { state: 'ask_first' } : {}) })) },
  { fieldKey: 'ancestry', label: 'Ancestry', choices: [ch('dwarf', 'Dwarf', { speed: 5, size: '1M', stability: 0 })] },
  { fieldKey: 'class', label: 'Class', choices: [ch('censor', 'Censor', { starting_stamina: 21, recoveries: 12 })] },
  { fieldKey: 'kit', label: 'Kit', choices: [ch('dual', 'Dual Wielder', { stamina_bonus: 6, speed_bonus: 2 })] },
] });

const html = () => `<!doctype html><html data-gm-term="Director"><head><meta charset="utf-8"><style>body{margin:0}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<main style="padding:0 12px"><div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"><p class="hc-loading__text">Opening</p></div></main>
<script>window.Chronicle = { register: (n, d) => { document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, width, height) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height }, reducedMotion: 'reduce' });
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === C + '/characters/new/plan') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(PLAN()) });
    return route.fulfill({ contentType: 'text/html', body: html() });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return pg;
}

test('on a computer no stat label breaks inside a word', { skip }, async (t) => {
  const pg = await open(t, 1280, 860);
  for (const k of ['ancestry', 'class', 'kit']) {
    await pg.click('.hc-rail .hc-row[data-k="' + k + '"]');
    await pg.locator('.hc-gcard[data-i]').first().click();
    await pg.click('.hc-right [data-act="choose"]');
    await pg.waitForTimeout(200);
  }
  await pg.locator('.hc-rail .hc-row').last().click();
  await pg.waitForSelector('.hc-hero .hc-stat');
  const labels = await pg.$$eval('.hc-stat span', (els) => els.map((e) => {
    const r = document.createRange(); r.selectNodeContents(e);
    const lines = new Set([...r.getClientRects()].map((x) => Math.round(x.top))).size;
    return { text: e.textContent, lines, clipped: e.scrollWidth > e.clientWidth + 1 };
  }));
  assert.ok(labels.length >= 9, 'the hero card and the rail both show their stats');
  assert.ok(labels.some((l) => l.text === 'Recoveries'), 'Recoveries is spelled out');
  for (const l of labels) assert.deepEqual([l.text, l.lines, l.clipped], [l.text, 1, false], l.text + ' sits on one line, whole');
  const sofar = await pg.$$eval('.hc-sofar .hc-stat', (els) => els.map((e) => getComputedStyle(e).flexDirection));
  assert.equal(new Set(sofar).size, 1, 'the four tiles under "Your hero so far" are laid out alike');
});

test('at 320 wide every card keeps its content, "Read the rules" included', { skip }, async (t) => {
  const pg = await open(t, 320, 520);
  await pg.waitForSelector('.hc-hand .hc-card[data-i]');
  const n = await pg.locator('.hc-hand .hc-card[data-i]').count();
  assert.equal(n, 100);
  const bad = [];
  for (const i of [0, 1, 2, 3, 4, 9, 46, 99]) {
    await pg.evaluate((i) => { const c = document.querySelectorAll('.hc-hand .hc-card[data-i]')[i], h = c.parentNode; h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2; }, i);
    await pg.waitForTimeout(120);
    const r = await pg.evaluate((i) => {
      const c = document.querySelectorAll('.hc-hand .hc-card[data-i]')[i], m = c.querySelector('.hc-more');
      return { name: c.querySelector('h3').textContent, over: c.scrollHeight - c.clientHeight, moreOut: m.getBoundingClientRect().bottom - c.getBoundingClientRect().bottom };
    }, i);
    if (r.over > 1 || r.moreOut > 0) bad.push(r);
  }
  assert.deepEqual(bad, [], 'no card is taller inside than out');
});

test('at 320 wide a one-word name is never cut at the card edge', { skip }, async (t) => {
  const pg = await open(t, 320, 520);
  await pg.waitForSelector('.hc-hand .hc-card[data-i]');
  const cut = [];
  for (const i of [5, 6, 12, 13]) {
    await pg.evaluate((i) => { const c = document.querySelectorAll('.hc-hand .hc-card[data-i]')[i], h = c.parentNode; h.scrollLeft = c.offsetLeft - (h.clientWidth - c.offsetWidth) / 2; }, i);
    await pg.waitForTimeout(120);
    const r = await pg.evaluate((i) => {
      const h = document.querySelectorAll('.hc-hand .hc-card[data-i]')[i].querySelector('h3'), word = h.textContent.split(' ')[0];
      const probe = document.createElement('span'); probe.textContent = word; probe.style.whiteSpace = 'nowrap'; h.appendChild(probe);
      const w = probe.getBoundingClientRect().width; probe.remove();
      return { name: h.textContent, out: Math.round(w - h.clientWidth) };
    }, i);
    if (r.out > 0) cut.push(r);
  }
  assert.deepEqual(cut, [], 'every name fits across its card');
});

test('on a phone a mark that is a button presses 44px tall', { skip }, async (t) => {
  const pg = await open(t, 390, 844);
  await pg.waitForSelector('.hc-hand .hc-card[data-i]');
  await pg.locator('.hc-hand .hc-card[data-i]').first().click();
  await pg.waitForSelector('.hc-sheet button.hc-mark');
  const marks = await pg.$$eval('button.hc-mark', (els) => els.filter((e) => e.offsetParent).map((e) => ({ text: e.textContent.trim(), h: e.getBoundingClientRect().height })));
  assert.ok(marks.length > 0);
  for (const m of marks) assert.ok(m.h >= 44, m.text + ' is ' + m.h + 'px tall');
});
