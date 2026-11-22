// ai_import_answer_all_browser.test.mjs — the review's "Yes to all but
// removals" button on a connected AI's record rows
// (fixtures/ai_import_answer_all_review.html, kept current by a Go test).
// Changes to existing records start on No; the button must turn them to Yes
// like any other row. It leaves a removal and a block already applied on No:
// those are answered one by one.
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
const fragment = readFileSync(join(root, 'test', 'js', 'fixtures', 'ai_import_answer_all_review.html'), 'utf8');
const SCRIPTS = { '/htmx.js': 'static/vendor/htmx.min.js', '/boot.js': 'static/js/boot.js', '/alpine.js': 'static/vendor/alpine.min.js', '/review.js': 'static/js/widgets/ai_import_review.js' };
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
      return route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css"><script src="/htmx.js" defer></script><script src="/boot.js" defer></script><script src="/review.js" defer></script><script src="/alpine.js" defer></script></head><body style="margin:0;padding:12px">${fragment}</body></html>` });
    }
    // The review's own widget and Alpine, as the page loads them: the buttons
    // are Alpine methods of the form, not inline handlers.
    if (SCRIPTS[u.pathname]) return route.fulfill({ contentType: 'application/javascript', body: readFileSync(join(root, SCRIPTS[u.pathname])) });
    const file = join(root, u.pathname);
    if (existsSync(file)) return route.fulfill({ contentType: TYPES[u.pathname.slice(u.pathname.lastIndexOf('.'))] || 'application/octet-stream', body: readFileSync(file) });
    return route.fulfill({ status: 404, body: '' });
  });
  page.setDefaultTimeout(30000);
  await page.goto('http://review.test/', { waitUntil: 'load' });
  await page.waitForFunction(() => document.querySelector('#ai-import-review')?._x_dataStack);
  return { page, errors };
}

const on = (page, i) => page.$eval(`input[name="rec_${i}_include"][value="on"]`, (e) => e.checked);

test('Yes to all but removals turns changes and adds to Yes, not removals or repeats', { skip, timeout: 120000 }, async (t) => {
  const { page, errors } = await open(t);
  assert.deepEqual([await on(page, 0), await on(page, 1), await on(page, 2), await on(page, 3)], [false, true, false, false], 'start: a connected AI\'s change is on No');
  await page.click('button:has-text("Yes to all but removals")');
  assert.deepEqual([await on(page, 0), await on(page, 1), await on(page, 2), await on(page, 3)], [true, true, false, false]);
  await page.click('button:has-text("No to all")');
  assert.deepEqual([await on(page, 0), await on(page, 1), await on(page, 2), await on(page, 3)], [false, false, false, false]);
  assert.deepEqual(errors, []);
});
