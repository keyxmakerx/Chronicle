// table_shortcut_browser.test.mjs — the Table's single-key shortcuts (T opens
// and closes the drawer, R opens it on the roller) against the real table.js in
// a real Chromium. A full-window surface marks <html data-full-surface> while
// it covers the page; the shortcuts stand down then, so the drawer never opens
// where nobody can see it, and work again when the mark goes.
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
const script = readFileSync(join(root, 'internal/plugins/rolls/static/js/table.js'));

const PAGE = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1"><button type="button" class="tt-tab" data-tt-tab aria-expanded="false">Table</button></div></header>
<input id="field"><main id="main">page</main>
<script>window.Chronicle = {
  tableDice: new Proxy({}, { get: (_, k) => (k === 'motion' ? () => 'off' : () => '') }), escapeHtml: (v) => String(v), motionMs: (n, f) => f,
  apiFetch: () => Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve(null) }),
  register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)),
};</script><script src="/table.js"></script></body></html>`;

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const pg = await ctx.newPage();
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/table.js') return route.fulfill({ contentType: 'text/javascript', body: script });
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  await pg.goto('http://t.test/');
  await pg.waitForFunction(() => document.querySelector('.tt-host')._table);
  return pg;
}

const drawer = (pg) => pg.evaluate(() => !!document.getElementById('tt-drawer'));

test('T and R open the Table; a covering surface stands them down; leaving brings them back', { skip }, async (t) => {
  const pg = await open(t);
  assert.equal(await drawer(pg), false);
  await pg.keyboard.press('t');
  assert.equal(await drawer(pg), true, 'T opens the drawer');
  await pg.keyboard.press('t');
  await pg.waitForFunction(() => !document.getElementById('tt-drawer'));

  await pg.evaluate(() => document.documentElement.setAttribute('data-full-surface', ''));
  await pg.keyboard.press('t');
  await pg.keyboard.press('r');
  await pg.waitForTimeout(150);
  assert.equal(await drawer(pg), false, 'neither key opens it behind a covering surface');

  await pg.evaluate(() => document.documentElement.removeAttribute('data-full-surface'));
  await pg.keyboard.press('r');
  assert.equal(await drawer(pg), true, 'R works again once the surface is gone');
});

test('a typed letter is still never taken, with or without the mark', { skip }, async (t) => {
  const pg = await open(t);
  await pg.focus('#field');
  await pg.keyboard.type('tr');
  assert.equal(await pg.inputValue('#field'), 'tr');
  assert.equal(await drawer(pg), false);
});
