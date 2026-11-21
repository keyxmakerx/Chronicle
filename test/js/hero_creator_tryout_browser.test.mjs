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

const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));
const html = (skin, mo) => `<!doctype html><html${mo ? ' data-motion="' + mo + '"' : ''}><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-tryout="true" data-campaign-id="c1" data-user-id="u9"
  data-plan-url="${TRY}/plan" data-back-url="${BACK}" data-type-name="practice hero" data-for-self="false"${skin ? ' data-skin="' + skin + '"' : ''}><p class="hc-loading__text">Opening</p></div>
<script>window.__went = []; window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director', go: (u) => window.__went.push(u) };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { stored, init, skin, mo, width = 1100, reduced = true } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: reduced ? 'reduce' : 'no-preference' });
  const requests = [];
  await pg.route('https://fonts.googleapis.com/**', (route) => route.abort());
  if (stored) await pg.addInitScript(([k, v]) => { if (!sessionStorage.getItem('__seeded')) { localStorage.setItem(k, v); sessionStorage.setItem('__seeded', '1'); } }, [KEY, JSON.stringify(stored)]);
  if (init) await pg.addInitScript(init);
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    requests.push({ method: req.method(), path: p });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === TRY + '/plan') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(PLAN()) });
    return route.fulfill({ contentType: 'text/html', body: html(skin, mo) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, requests };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();
const pick = async (pg, name) => { await pg.locator('.hc-gcard', { hasText: name }).click(); await pg.click('.hc-right [data-act="choose"]'); };
const rail = (pg, key) => pg.locator('.hc-rail .hc-row[data-k="' + key + '"]');
const lastStep = (pg) => pg.locator('.hc-steps .hc-row').last();

test('the band and the button say it is kept here', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal((await text(pg, '.hc-band')).trim(), 'Trying it out. Kept in this browser only.');
  await pick(pg, 'Dwarf');
  await pick(pg, 'Conduit');
  await pg.waitForSelector('[data-act="create"]');
  assert.equal((await text(pg, '[data-act="create"]')).trim(), 'Keep it on this device');
  assert.match(await text(pg, '.hc-note'), /stays in this browser/);
});

test('keeping it stores the hero under its key and sends nothing', { skip }, async (t) => {
  const { pg, requests } = await open(t);
  await pick(pg, 'Dwarf');
  await pick(pg, 'Conduit');
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
  assert.match(await text(pg, '.hc-right [data-act="choose"]'), /^Keep Dwarf/, 'the open entry is the stored pick');
  assert.match(await text(pg, '.hc-rail .hc-row'), /Dwarf/);
  assert.doesNotMatch(await rail(pg, 'class').innerText(), /Gone Class/, 'a pick the plan no longer offers is dropped');
  assert.match(await rail(pg, 'class').innerText(), /Optional/);
  await rail(pg, 'class').click();
  await pick(pg, 'Fury');
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
  await lastStep(pg).click();
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

test('a mount that names no skin wears the plain one, and a named skin changes the tokens', { skip }, async (t) => {
  const tok = (pg) => pg.evaluate(() => getComputedStyle(document.querySelector('.hc')).getPropertyValue('--hc-accent').trim());
  const plain = await open(t);
  assert.equal(await plain.pg.evaluate(() => document.querySelector('.hc').hasAttribute('data-skin')), false);
  assert.notEqual(await tok(plain.pg), '#6b2a5e');
  assert.equal(await plain.pg.evaluate(() => !!document.getElementById('hc-fonts')), false, 'plain fetches no extra faces');
  const paper = await open(t, { skin: 'paper' });
  assert.equal(await tok(paper.pg), '#6b2a5e');
  assert.equal(await paper.pg.evaluate(() => !!document.getElementById('hc-fonts')), true, 'paper asks for its faces');
  const ornate = await open(t, { skin: 'ornate' });
  assert.equal(await tok(ornate.pg), '#6f5a1c');
});

test('the phone shows a step header and a chip row of what is picked', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390 });
  assert.equal(await pg.locator('.hc-app').getAttribute('data-view'), 'phone');
  assert.match(await text(pg, '.hc-sbtn'), /Step 1 of 3\s*·\s*Ancestry/);
  assert.equal(await pg.locator('.hc-chip').count(), 1, 'only the step being chosen is in the row yet');
  assert.match(await text(pg, '.hc-chip'), /Choosing/);
  await pg.locator('.hc-card', { hasText: 'Dwarf' }).click();
  await pg.click('.hc-sfoot [data-act="choose"]');
  await pg.waitForFunction(() => /Step 2 of 3/.test(document.querySelector('.hc-sbtn').innerText));
  assert.match(await text(pg, '.hc-sbtn'), /Class/);
  const chips = await pg.locator('.hc-chip').allInnerTexts();
  assert.match(chips[0], /ancestry\s*dwarf/i);
  assert.match(chips[1], /class\s*choosing/i);
  await pg.locator('.hc-chip', { hasText: 'Dwarf' }).click();
  assert.match(await text(pg, '.hc-sbtn'), /Step 1 of 3/, 'a chip goes back to its step');
});

test('the motion level follows the site setting and the device', { skip }, async (t) => {
  const level = (pg) => pg.evaluate(() => window.ChronicleHeroCreator.motion());
  const dataMo = (pg) => pg.locator('.hc-app').getAttribute('data-mo');
  const full = await open(t, { reduced: false, mo: 'full' });
  assert.equal(await level(full.pg), 'lively');
  assert.equal(await dataMo(full.pg), 'lively');
  const calm = await open(t, { reduced: false, mo: 'calm' });
  assert.equal(await level(calm.pg), 'gentle');
  assert.equal(await dataMo(calm.pg), 'gentle');
  const off = await open(t, { reduced: false, mo: 'off' });
  assert.equal(await level(off.pg), 'still');
  assert.equal(await dataMo(off.pg), 'still');
  const device = await open(t, { reduced: true, mo: 'full' });
  assert.equal(await level(device.pg), 'gentle', 'a device that asks for less motion gets Gentle');
});

test('rules words are normalised the way the glossary keys them', { skip }, async (t) => {
  const { pg } = await open(t);
  const out = await pg.evaluate(() => {
    const H = window.ChronicleHeroCreator;
    return {
      keys: [H.termKey('Bleeding'), H.termKey('  Power   Roll '), H.termKey('Free\tStrike'), H.termKey('')],
      plain: H.plain('Take {@condition bleeding|bleed} or {@combat power roll}.'),
      known: H.marked('Take {@condition Bleeding|bleed} now', { bleeding: { name: 'Bleeding' } }),
      unknown: H.marked('Take {@condition bleeding} <b>x</b>', {}),
    };
  });
  assert.deepEqual(out.keys, ['bleeding', 'power-roll', 'free-strike', '']);
  assert.equal(out.plain, 'Take bleed or power roll.');
  assert.match(out.known, /<button type="button" class="hc-term" data-term="bleeding"[^>]*>bleed<\/button>/);
  assert.equal(out.unknown, 'Take bleeding &lt;b&gt;x&lt;/b&gt;', 'an unknown word is plain and everything is escaped');
});
