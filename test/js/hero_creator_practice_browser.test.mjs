// hero_creator_practice_browser.test.mjs — the hero creator in practice mode.
//
// Runs hero_creator.js in a real Chromium against a stubbed practice plan. It
// pins what a player making a practice hero sees and sends: the band, the
// "Not offered here" seal, an Ask first entry picked as a plain pick with no
// ask flow, the button words, a draft of its own, a change saved with PUT, and
// a copy telling the server to read the player's hero again.
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
const C = '/campaigns/c1/characters/practice';
const js = (n) => readFileSync(join(root, 'internal/plugins/entities/static/js', n));

const ch = (key, name, extra = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
const PLAN = () => ({ practice: true, systemName: 'Draw Steel', steps: [
  { fieldKey: 'class', label: 'Class', choices: [ch('conduit', 'Conduit'), ch('tactician', 'Tactician', { state: 'ask_first' }), ch('fury', 'Fury', { state: 'not_offered' })] },
  { fieldKey: 'kit', label: 'Kit', choices: [ch('a', 'Arcane Archer')] },
] });

const css = () => readFileSync(join(root, 'internal/plugins/entities/static/css/hero_creator.css'));
const html = (attrs) => `<!doctype html><html><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-practice="true" data-campaign-id="c1" data-plan-url="${C}/plan" data-create-url="${C}"
  data-csrf="tok" data-type-name="practice hero" data-for-self="false" ${attrs}><p class="hc-loading__text">Opening</p></div>
<script>window.__went = []; window.Chronicle = { register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)), escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director', go: (u) => window.__went.push(u) };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { attrs = '', width = 1100, reply = { status: 201, body: { id: 'p9', url: C + '/p9' } } } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const pg = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const seen = { send: [] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/plan') return ok(PLAN());
    if (p.startsWith(C) && req.method() !== 'GET') { seen.send.push({ method: req.method(), path: p, body: JSON.parse(req.postData()) }); return ok(reply.body, reply.status); }
    return route.fulfill({ contentType: 'text/html', body: html(attrs) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  return { pg, seen };
}
const text = (pg, sel) => pg.locator(sel).first().innerText();

test('the band says what a practice hero is', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal((await text(pg, '.hc-band')).trim(), 'Practice hero. Not in the party, not sent to Foundry.');
});

const gcard = (pg, name) => pg.locator('.hc-gcard', { hasText: name });
const rail = (pg, key) => pg.locator('.hc-rail .hc-row[data-k="' + key + '"]');
const lastStep = (pg) => pg.locator('.hc-steps .hc-row').last();
const pick = async (pg, name) => { await gcard(pg, name).click(); await pg.click('.hc-right [data-act="choose"]'); };

test('entries the rules keep are marked, and an Ask first entry is a plain pick', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await pg.locator('.hc-gcard[data-i]').count(), 3, 'every entry is listed');
  assert.match(await gcard(pg, 'Fury').innerText(), /Not offered here/);
  assert.match(await gcard(pg, 'Tactician').innerText(), /Ask first/);
  await gcard(pg, 'Tactician').click();
  assert.match(await text(pg, '.hc-right [data-act="choose"]'), /^Choose Tactician/, 'no Ask for flow');
  await pg.click('.hc-right [data-act="choose"]');
  assert.match(await rail(pg, 'class').innerText(), /Tactician/);
  assert.doesNotMatch(await rail(pg, 'class').innerText(), /waiting/);
  assert.equal(await pg.locator('.hc-notes').innerText(), '', 'nothing waits for a Director');
  await rail(pg, 'class').click();
  assert.equal(await pg.locator('.hc-notes').innerText(), '', 'nothing waits for a Director');
  await gcard(pg, 'Fury').click();
  assert.match(await text(pg, '.hc-right'), /Not offered at this table/);
  await pg.click('.hc-right [data-act="choose"]');
  assert.match(await rail(pg, 'class').innerText(), /Fury.*not offered here/);
});

test('on a phone an Ask first card is also a plain pick', { skip }, async (t) => {
  const { pg } = await open(t, { width: 390 });
  assert.equal(await pg.locator('.hc-app').getAttribute('data-view'), 'phone');
  assert.equal((await text(pg, '.hc-band')).trim(), 'Practice hero. Not in the party, not sent to Foundry.');
  await pg.locator('.hc-card', { hasText: 'Tactician' }).click();
  assert.match(await text(pg, '.hc-sfoot [data-act="choose"]'), /^Choose Tactician/, 'no Ask for flow');
  await pg.click('.hc-sfoot [data-act="choose"]');
  await pg.waitForFunction(() => /Step 2 of 3/.test(document.querySelector('.hc-sbtn').innerText));
  assert.match(await text(pg, '.hc-chip'), /tactician/i);
  assert.equal(await pg.locator('.hc-notes').innerText(), '', 'nothing waits for a Director');
});

test('keeping it saves to the practice address and goes to the practice page', { skip }, async (t) => {
  const { pg, seen } = await open(t);
  await pick(pg, 'Fury');
  await lastStep(pg).click();
  assert.match(await text(pg, '.hc-rev__t'), /Name your practice hero/);
  assert.equal((await text(pg, '[data-act="create"]')).trim(), 'Keep this practice hero');
  assert.doesNotMatch(await text(pg, '.hc-note'), /on the sheet later/);
  await pg.fill('[data-name]', 'Old Marrow');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(seen.send, [{ method: 'POST', path: C, body: { name: 'Old Marrow', picks: { class: { name: 'Fury', options: [] } } } }]);
  assert.deepEqual(await pg.evaluate(() => window.__went), [C + '/p9']);
});

test('the draft has its own key and is cleared once kept', { skip }, async (t) => {
  const { pg } = await open(t);
  await pick(pg, 'Conduit');
  const keys = await pg.evaluate(() => Object.keys(localStorage));
  assert.deepEqual(keys, ['chronicle:hero-practice:c1']);
  await lastStep(pg).click();
  await pg.fill('[data-name]', 'X');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.deepEqual(await pg.evaluate(() => Object.keys(localStorage)), []);
});

test('changing one starts from it and saves with PUT', { skip }, async (t) => {
  const pre = JSON.stringify({ name: 'Old Marrow', picks: { class: { name: 'Fury' }, kit: { name: 'Gone Kit' } } });
  const { pg, seen } = await open(t, { attrs: `data-update-url="${C}/p9" data-preload='${pre}'`, reply: { status: 200, body: { id: 'p9', url: C + '/p9' } } });
  assert.match(await text(pg, '.hc-right [data-act="choose"]'), /^Keep Fury/);
  assert.match(await rail(pg, 'class').innerText(), /Fury/);
  assert.doesNotMatch(await rail(pg, 'kit').innerText(), /Gone Kit/, 'a pick the plan no longer offers is dropped');
  assert.match(await rail(pg, 'kit').innerText(), /Optional/);
  await lastStep(pg).click();
  assert.equal((await text(pg, '[data-act="create"]')).trim(), 'Save changes');
  assert.equal(await pg.inputValue('[data-name]'), 'Old Marrow');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.equal(seen.send[0].method, 'PUT');
  assert.equal(seen.send[0].path, C + '/p9');
  assert.equal(seen.send[0].body.copy, undefined);
  assert.deepEqual(seen.send[0].body.picks, { class: { name: 'Fury', options: [] } });
  assert.deepEqual(await pg.evaluate(() => Object.keys(localStorage)), [], 'a change keeps no draft');
});

test('a copy of the player\'s hero tells the server to read it again', { skip }, async (t) => {
  const pre = JSON.stringify({ name: 'Tess', copy: true, picks: { class: { name: 'Conduit' } } });
  const { pg, seen } = await open(t, { attrs: `data-preload='${pre}'` });
  await lastStep(pg).click();
  assert.equal(await pg.inputValue('[data-name]'), 'Tess');
  await pg.click('[data-act="create"]');
  await pg.waitForFunction(() => window.__went.length > 0);
  assert.equal(seen.send[0].method, 'POST');
  assert.equal(seen.send[0].body.copy, true);
});

test('a refusal is shown where the button was', { skip }, async (t) => {
  const { pg } = await open(t, { reply: { status: 409, body: { message: 'You have three practice heroes. Throw one away to start another.' } } });
  await lastStep(pg).click();
  await pg.fill('[data-name]', 'Fourth');
  await pg.click('[data-act="create"]');
  await pg.waitForSelector('[data-error]:not([hidden])');
  assert.match(await text(pg, '[data-error]'), /You have three practice heroes/);
  assert.equal(await pg.locator('[data-act="create"]').isDisabled(), false);
});
