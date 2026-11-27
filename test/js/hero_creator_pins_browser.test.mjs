// hero_creator_pins_browser.test.mjs — pinned entries and the player's sort.
//
// Runs hero_creator.js in a real Chromium against a stubbed plan. It pins
// that pinned entries lead with a gap after them, that the sort word's A to Z
// keeps them first and is remembered in this browser, that the pin controls
// appear only on a plan that says the viewer may pin, and what the pin
// control sends.
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

const ch = (key, name, extra = {}) => ({ key, slug: key, name, summary: name + ' summary.', source: 'package', properties: {}, ...extra });
// As the server serves it: pins first, then the book A to Z, then the campaign's.
const PLAN = (canPin, pins = ['package:human', 'campaign:7']) => {
  const all = [ch('devil', 'Devil'), ch('dwarf', 'Dwarf'), ch('human', 'Human'), ch('7', 'Khepri', { source: 'campaign' }), ch('9', 'Aurin', { source: 'campaign' })];
  const at = (c) => pins.indexOf(c.source + ':' + c.key);
  const lead = all.filter((c) => at(c) >= 0).sort((a, b) => at(a) - at(b)).map((c) => ({ ...c, pinned: true }));
  return { systemName: 'Draw Steel', canPin, steps: [{ fieldKey: 'ancestry', label: 'Ancestry', choices: lead.concat(all.filter((c) => at(c) < 0)) }] };
};

const html = () => `<!doctype html><html><head><meta charset="utf-8"><style>.sr-only{position:absolute;width:1px;height:1px;overflow:hidden}</style><link rel="stylesheet" href="/hero_creator.css"></head><body>
<div class="hc" data-widget="hero-creator" data-hero-creator="true" data-campaign-id="c1" data-plan-url="${C}/characters/new/plan" data-create-url="${C}/characters/new"
  data-csrf="tok" data-type-name="Character" data-for-self="true"><p class="hc-loading__text">Opening</p></div>
<script>window.Chronicle = { register: (n, d) => { document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)); }, escapeHtml: (v) => String(v == null ? '' : v).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])), gmTerm: () => 'Director' };</script>
<script src="/hero_rules_lines.js"></script><script src="/hero_creator.js"></script></body></html>`;

async function open(t, { canPin = false, width = 1100, page = null } = {}) {
  const browser = page ? null : await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  if (browser) t.after(() => browser.close());
  const pg = page || await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' });
  const seen = { put: [], pins: ['package:human', 'campaign:7'] };
  await pg.route('http://t.test/**', (route) => {
    const req = route.request(), p = new URL(req.url()).pathname;
    const ok = (b) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(b) });
    if (p === '/hero_creator.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_creator.js') });
    if (p === '/hero_creator.css') return route.fulfill({ contentType: 'text/css', body: css() });
    if (p === '/hero_rules_lines.js') return route.fulfill({ contentType: 'text/javascript', body: js('hero_rules_lines.js') });
    if (p === C + '/characters/new/plan') return ok(PLAN(canPin, seen.pins));
    if (p === C + '/character-choices/ancestry/pins' && req.method() === 'PUT') {
      const b = JSON.parse(req.postData());
      seen.put.push({ body: b, csrf: req.headers()['x-csrf-token'] });
      const k = b.source + ':' + b.key;
      seen.pins = seen.pins.filter((x) => x !== k).concat(b.pinned ? [k] : []);
      return ok({ pins: seen.pins });
    }
    return route.fulfill({ contentType: 'text/html', body: html() });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.hc-app');
  await pg.click('[data-mode="list"]');
  return { pg, seen };
}
const order = (pg) => pg.$$eval('.hc-mid .hc-lrow[data-i]', (rs) => rs.map((r) => r.querySelector('b').textContent + (r.classList.contains('is-pinlast') ? '|' : '')).join(','));

test('pinned entries lead, with the gap after the last one', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.equal(await order(pg), 'Human,Khepri|,Devil,Dwarf,Aurin');
});

test('A to Z sorts the rest, keeps the pins first, and is remembered here', { skip }, async (t) => {
  const { pg } = await open(t);
  assert.match(await pg.locator('.hc-sortb').innerText(), /Sort:\s*Book first/);
  await pg.click('.hc-sortb');
  assert.equal(await pg.locator('.hc-sortmenu [aria-checked="true"]').innerText().then((s) => /Book first/.test(s)), true);
  await pg.click('.hc-smi[data-sort="az"]');
  assert.equal(await pg.locator('.hc-sortmenu').count(), 0, 'the menu closes');
  assert.equal(await order(pg), 'Human,Khepri|,Aurin,Devil,Dwarf');
  assert.match(await pg.locator('.hc-sortb').innerText(), /A to Z/);
  assert.equal(await pg.evaluate(() => localStorage.getItem('chronicle:hero-sort:c1:ancestry')), 'az');
  await pg.reload();
  await pg.waitForSelector('.hc-app');
  await pg.click('[data-mode="list"]');
  assert.equal(await order(pg), 'Human,Khepri|,Aurin,Devil,Dwarf', 'the sort holds after a reload');
  await pg.keyboard.press('Escape');
  await pg.click('.hc-sortb');
  await pg.keyboard.press('Escape');
  assert.equal(await pg.locator('.hc-sortmenu').count(), 0, 'Escape closes the menu');
  assert.equal(await pg.evaluate(() => document.activeElement.classList.contains('hc-sortb')), true, 'focus goes back to the word');
});

test('a player sees no pin controls', { skip }, async (t) => {
  const { pg } = await open(t, { canPin: false });
  assert.equal(await pg.locator('.hc-rpin').count(), 0);
  assert.equal(await pg.locator('.hc-panepin').count(), 0);
});

test('the Director pins from the row and unpins from the pane', { skip }, async (t) => {
  const { pg, seen } = await open(t, { canPin: true });
  assert.equal(await pg.locator('.hc-rpin').count(), 5, 'every row has a pin');
  assert.equal(await pg.locator('.hc-rpin.is-on').count(), 2, 'pinned rows show theirs');
  await pg.locator('.hc-lrow', { hasText: 'Dwarf' }).hover();
  await pg.locator('.hc-lrow', { hasText: 'Dwarf' }).locator('.hc-rpin').click();
  await pg.waitForFunction(() => document.querySelector('.hc-mid .hc-lrow[data-i="2"] b').textContent === 'Dwarf');
  assert.deepEqual(seen.put[0], { body: { source: 'package', key: 'dwarf', pinned: true }, csrf: 'tok' });
  assert.equal(await order(pg), 'Human,Khepri,Dwarf|,Devil,Aurin');

  await pg.locator('.hc-lrow', { hasText: 'Khepri' }).click();
  assert.match(await pg.locator('.hc-right .hc-panepin').innerText(), /Pinned to the top\s*Unpin/);
  await pg.click('.hc-right [data-act="unpin"]');
  await pg.waitForFunction(() => document.querySelector('.hc-mid .hc-lrow[data-i="1"] b').textContent === 'Dwarf');
  assert.deepEqual(seen.put[1].body, { source: 'campaign', key: '7', pinned: false });
  assert.equal(await order(pg), 'Human,Dwarf|,Devil,Khepri,Aurin', 'an unpinned entry goes back to its served place');
  assert.match(await pg.locator('.hc-right .hc-panepin').innerText(), /Pin to the top/);
  assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-act')), 'pin', 'focus stays on the pane word');
});
