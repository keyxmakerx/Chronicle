// ai_import_similar_browser.test.mjs — the review's line for an add that is
// much like a page the campaign has (reviewRowSimilar in
// internal/plugins/ai_workspace/import_review.templ), driven on the review's
// own markup (fixtures/ai_import_similar_review.html, kept current by a Go
// test). "Update that page instead" must change the form fields the commit
// reads, and "Add a new page instead" must put them back; "See both" opens
// the two texts.
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
const fragment = readFileSync(join(root, 'test', 'js', 'fixtures', 'ai_import_similar_review.html'), 'utf8');
const TYPES = { '.css': 'text/css', '.js': 'application/javascript', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' };

async function open(t, width = 1000) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') {
      return route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css"></head><body style="margin:0;padding:12px">${fragment}</body></html>` });
    }
    const file = join(root, u.pathname);
    if (existsSync(file)) return route.fulfill({ contentType: TYPES[u.pathname.slice(u.pathname.lastIndexOf('.'))] || 'application/octet-stream', body: readFileSync(file) });
    return route.fulfill({ status: 404, body: '' });
  });
  page.setDefaultTimeout(30000);
  await page.goto('http://review.test/', { waitUntil: 'load' });
  return { page, errors };
}
const field = (page, name) => page.$eval(`[name="${name}"]`, (e) => e.value);
const visible = (page, sel) => page.$eval(sel, (e) => !e.hidden && e.offsetParent !== null);

test('the line names the page and offers both ways forward', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  const line = await page.textContent('.air-sim');
  assert.match(line, /Much like a page you have:/);
  assert.match(line, /New Player Guide, 9 words differ/);
  assert.equal(await visible(page, '[data-sim-swap]'), true);
  assert.equal(await visible(page, '.air-sim-both'), false);
  assert.deepEqual(errors, []);
});

test('Update that page instead turns the add into a change, and back', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  assert.equal(await field(page, 'page_0_action'), 'create');
  await page.click('button:has-text("Update that page instead")');
  assert.equal(await field(page, 'page_0_name'), 'New Player Guide');
  assert.equal(await field(page, 'page_0_action'), 'update');
  assert.equal(await page.$eval('[data-air-row]', (r) => r.dataset.airVerb), 'Change');
  assert.equal((await page.textContent('[data-air-row] .air-pill')).trim(), 'Change');
  assert.match(await page.textContent('[data-air-row] .air-what'), /^\s*Change the page\s+New Player Guide\s*$/);
  assert.match(await page.textContent('.air-sim'), /Changing New Player Guide instead of adding a page/);
  // The page has GM-only parts, so the tick the commit demands now shows and counts.
  assert.equal(await visible(page, '[data-sim-gm]'), true);
  assert.equal(await page.$eval('[data-air-row]', (r) => r.hasAttribute('data-air-gm')), true);
  assert.equal(await page.evaluate(() => document.activeElement.textContent.trim()), 'Add a new page instead', 'focus follows the control');

  await page.click('button:has-text("Add a new page instead")');
  assert.equal(await field(page, 'page_0_name'), 'New Player Guide (Imported)');
  assert.equal(await field(page, 'page_0_action'), 'create');
  assert.equal(await page.$eval('[data-air-row]', (r) => r.dataset.airVerb), 'Add');
  assert.equal(await visible(page, '[data-sim-gm]'), false);
  assert.equal(await page.$eval('[data-air-row]', (r) => r.hasAttribute('data-air-gm')), false);
  assert.match(await page.textContent('[data-air-row] .air-what'), /Add the page\s+New Player Guide \(Imported\)/);
  assert.deepEqual(errors, []);
});

test('See both opens the two texts, marks what differs, and closes', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  await page.click('button:has-text("See both")');
  assert.equal(await visible(page, '.air-sim-both'), true);
  assert.equal(await page.getAttribute('button:has-text("Hide both")', 'aria-expanded'), 'true');
  assert.deepEqual(await page.$$eval('.air-sim-col h5', (e) => e.map((x) => x.textContent)), ['The page you have', 'What your AI sent']);
  // One differing line a side, marked in words.
  assert.deepEqual(await page.$$eval('.air-sim-col p.is-only', (e) => e.map((x) => x.textContent.replace(/\s+/g, ' ').trim())),
    ['Only here Bring a hero of level 1.', 'Only here Bring a hero of level 2.']);
  await page.click('button:has-text("Hide both")');
  assert.equal(await visible(page, '.air-sim-both'), false);
  assert.deepEqual(errors, []);
});

test('on a phone the two texts stack and nothing runs off the screen', { skip, timeout: 120000 }, async (t) => {
  const { page } = await open(t, 360);
  await page.click('button:has-text("See both")');
  const [a, b] = await page.$$eval('.air-sim-col', (cols) => cols.map((c) => { const r = c.getBoundingClientRect(); return { x: r.x, y: r.y, right: r.right }; }));
  assert.ok(b.y > a.y && Math.abs(a.x - b.x) < 1, 'stacked in one column');
  assert.ok(Math.max(a.right, b.right) <= 360, 'inside the screen');
  const h = await page.$$eval('.air-sim-qs .air-quiet', (bs) => bs.map((x) => x.getBoundingClientRect().height));
  assert.ok(h.length === 2 && h.every((x) => x >= 24), 'controls have a body');
});
