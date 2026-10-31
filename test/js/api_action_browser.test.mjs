// api_action_browser.test.mjs — Chronicle.apiAction settles on the outcome a
// button reports: a refused write rejects with the server's message, and a
// password re-confirmation 403 opens the prompt and re-sends after it.
//
// Runs boot.js in a real Chromium against a stubbed server. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script></head><body>
<script>window.prompts = 0; window.addEventListener('reauth-required', function () { window.prompts++; });</script>
</body></html>`;

async function open() {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  let guarded = 0;
  await page.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/ok') return route.fulfill({ contentType: 'application/json', body: '{"pinned":3}' });
    if (p === '/refused') return route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal","message":"disk full"}' });
    if (p === '/guarded') {
      guarded++;
      if (guarded === 1) {
        return route.fulfill({ status: 403, headers: { 'HX-Trigger': 'reauth-required' }, contentType: 'application/json', body: '{"error":"reauth_required"}' });
      }
      return route.fulfill({ contentType: 'application/json', body: '{"pinned":1}' });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await page.goto('http://t.test/');
  await page.waitForFunction(() => window.Chronicle && typeof window.Chronicle.apiAction === 'function');
  return { browser, page };
}

test('a 2xx resolves with the parsed body', { skip }, async () => {
  const { browser, page } = await open();
  try {
    const got = await page.evaluate(() => Chronicle.apiAction('/ok', { method: 'POST' }));
    assert.deepEqual(got, { pinned: 3 });
  } finally { await browser.close(); }
});

test('a refused write rejects with the server message', { skip }, async () => {
  const { browser, page } = await open();
  try {
    const got = await page.evaluate(() => Chronicle.apiAction('/refused', { method: 'POST' }).then(() => 'resolved', (e) => e.message));
    assert.equal(got, 'disk full');
  } finally { await browser.close(); }
});

test('a re-confirmation 403 opens the prompt and re-sends once confirmed', { skip }, async () => {
  const { browser, page } = await open();
  try {
    const got = await page.evaluate(() => {
      const p = Chronicle.apiAction('/guarded', { method: 'POST' });
      return new Promise((r) => setTimeout(r, 100)).then(() => {
        const prompted = window.prompts;
        window.dispatchEvent(new CustomEvent('reauth-confirmed'));
        return p.then((body) => ({ prompted, body }));
      });
    });
    assert.deepEqual(got, { prompted: 1, body: { pinned: 1 } });
  } finally { await browser.close(); }
});

test('cancelling the prompt rejects', { skip }, async () => {
  const { browser, page } = await open();
  try {
    const got = await page.evaluate(() => {
      const p = Chronicle.apiAction('/guarded', { method: 'POST' });
      return new Promise((r) => setTimeout(r, 100)).then(() => {
        window.dispatchEvent(new CustomEvent('reauth-cancelled'));
        return p.then(() => 'resolved', (e) => e.message);
      });
    });
    assert.equal(got, 'password not confirmed');
  } finally { await browser.close(); }
});
