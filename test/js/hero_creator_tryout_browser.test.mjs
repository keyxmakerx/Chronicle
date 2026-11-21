// hero_creator_tryout_browser.test.mjs — the hero creator in try-out mode.
//
// Runs hero_creator.js in a real Chromium against a stubbed visitor plan. It
// pins what a look-around visitor sees and what leaves the page: the band, the
// button words, that nothing is sent with any method but GET, that the result
// lands in localStorage under its key in its shape, that a stored hero opens
// the page filled in, and the plain sentence when the browser keeps nothing.
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
const TRY = '/campaigns/c1/characters/try';
const BACK = '/campaigns/c1/characters';
const KEY = 'chronicle:hero-tryout:c1:u9';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));

const ch = (key, name, extra = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
const PLAN = () => ({ practice: true, tryout: true, systemName: 'Draw Steel', steps: [
  { fieldKey: 'ancestry', label: 'Ancestry', choices: [ch('dwarf', 'Dwarf'), ch('human', 'Human')] },
  { fieldKey: 'class', label: 'Class', choices: [ch('conduit', 'Conduit'), ch('fury', 'Fury')] },
] });

const html = () => `<!doctype html><html><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-tryout="true" data-campaign-id="c1" data-user-id="u9"
  data-plan-url="${TRY}/plan" data-back-url="${BACK}" data-type-name="practice hero" data-for-self="false"><p class="hc-loading__text">Opening</p></div>
<script>window.__went = []; window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director', go: (u) => window.__went.push(u) };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { stored, init } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width: 1100, height: 900 } });
  const requests = [];
  if (stored) await pg.addInitScript(([k, v]) => { if (!sessionStorage.getItem('__seeded')) { localStorage.setItem(k, v); sessionStorage.setItem('__seeded', '1'); } }, [KEY, JSON.stringify(stored)]);
  if (init) await pg.addInitScript(init);
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    requests.push({ method: req.method(), path: p });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === TRY + '/plan') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(PLAN()) });
    return route.fulfill({ contentType: 'text/html', body: html() });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-page');
  return { pg, requests };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const pick = async (pg, name) => { await pg.locator('.hc-row', { hasText: name }).click(); await pg.click('[data-act="choose"]'); };

test('the band and the button say it is kept here', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal((await text(pg, '.hc-band')).trim(), 'Trying it out. Kept in this browser only.');
  await pick(pg, 'Dwarf');
  await pg.click('[data-act="next"]');
  await pick(pg, 'Conduit');
  await pg.click('[data-act="next"]');
  assert.equal((await text(pg, '[data-act="create"]')).trim(), 'Keep it on this device');
  assert.match(await text(pg, '.hc-note'), /stays in this browser/);
});

test('keeping it stores the hero under its key and sends nothing', { skip }, async (t) => {
  const { pg, requests } = await open(t);
  await pick(pg, 'Dwarf');
  await pg.click('[data-act="next"]');
  await pick(pg, 'Conduit');
  await pg.click('[data-act="next"]');
  await pg.fill('[data-name]', 'Old Marrow');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(await pg.evaluate(() => window.__went), [BACK]);
  const rec = JSON.parse(await pg.evaluate((k) => localStorage.getItem(k), KEY));
  assert.deepEqual(Object.keys(rec).sort(), ['name', 'picks', 'savedAt', 'summary', 'v']);
  assert.equal(rec.v, 1);
  assert.equal(rec.name, 'Old Marrow');
  assert.equal(rec.summary, 'Dwarf Conduit');
  assert.deepEqual(rec.picks, { ancestry: { name: 'Dwarf', options: [] }, class: { name: 'Conduit', options: [] } });
  assert.ok(!Number.isNaN(Date.parse(rec.savedAt)), 'savedAt is a date');
  assert.deepEqual(await pg.evaluate(() => Object.keys(localStorage)), [KEY], 'no draft is left beside it');
  const writes = requests.filter((r) => r.method !== 'GET');
  assert.deepEqual(writes, [], 'nothing was sent with a method other than GET');
  assert.deepEqual([...new Set(requests.filter((r) => r.path.startsWith('/campaigns')).map((r) => r.path))], [TRY + '/plan'], 'the only campaign address read is the visitor plan');
});

test('a stored try-out opens the page filled in, and keeping again replaces it', { skip }, async (t) => {
  const stored = { v: 1, name: 'Old Marrow', picks: { ancestry: { name: 'Dwarf' }, class: { name: 'Gone Class' } }, summary: 'Dwarf', savedAt: '2026-01-01T00:00:00.000Z' };
  const { pg, requests } = await open(t, { stored });
  assert.match(await text(pg, '.hc-chosen'), /Dwarf/);
  await pg.click('[data-act="next"]');
  assert.match(await text(pg, '.hc-chosen'), /No class chosen yet/, 'a pick the plan no longer offers is dropped');
  await pick(pg, 'Fury');
  await pg.click('[data-act="next"]');
  assert.equal(await pg.inputValue('[data-name]'), 'Old Marrow');
  await pg.fill('[data-name]', 'Old Marrow II');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  const rec = JSON.parse(await pg.evaluate((k) => localStorage.getItem(k), KEY));
  assert.equal(rec.name, 'Old Marrow II');
  assert.equal(rec.summary, 'Dwarf Fury');
  assert.deepEqual(requests.filter((r) => r.method !== 'GET'), []);
});

test('a browser that keeps nothing gets the plain sentence and the button stays usable', { skip }, async (t) => {
  const { pg, requests } = await open(t, { init: () => { Storage.prototype.setItem = function () { throw new Error('QuotaExceededError'); }; } });
  await pg.click('[data-act="next"]');
  await pg.click('[data-act="next"]');
  await pg.fill('[data-name]', 'Old Marrow');
  await pg.click('[data-act="create"]');
  await pg.waitForSelector('[data-error]:not([hidden])');
  assert.equal((await text(pg, '[data-error]')).trim(), "This browser won't keep things, so the try-out can't be saved here.");
  assert.equal(await pg.locator('[data-act="create"]').isDisabled(), false);
  assert.deepEqual(await pg.evaluate(() => window.__went), [], 'it did not go back as if it had kept it');
  assert.deepEqual(requests.filter((r) => r.method !== 'GET'), []);
});

test('a browser that will not even read storage says so above the steps', { skip }, async (t) => {
  const { pg } = await open(t, { init: () => { Storage.prototype.getItem = function () { throw new Error('SecurityError'); }; } });
  assert.match(await text(pg, '.hc-band'), /This browser won't keep things, so the try-out can't be saved here\./);
});

test('a notice another page left is shown once above the steps', { skip }, async (t) => {
  const { pg } = await open(t, { init: () => { sessionStorage.setItem('chronicle:hero-handoff:c1', JSON.stringify({ message: '"Fury" is not on the class list' })); } });
  assert.match(await text(pg, '.hc-band--note'), /"Fury" is not on the class list/);
  assert.equal(await pg.evaluate(() => sessionStorage.getItem('chronicle:hero-handoff:c1')), null, 'read once');
});
