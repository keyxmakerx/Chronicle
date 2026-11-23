// table_cutscenes_browser.test.mjs — the Table's Tonight list on a phone: it
// reads what was shown (the sheet used to leave it on "Reading…"), and lists
// the cut scenes this person can watch again, gone once their time runs out.
//
// Runs the real table.js in Chromium at phone width. Needs playwright-core and
// a Chromium (see _browser_env.mjs); skips locally, and fails under CI, when
// either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const script = readFileSync(join(root, 'internal/plugins/rolls/static/js/table.js'));

const PAGE = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<header><div class="tt-host" data-widget="table" data-campaign-id="c1" data-user-id="u1" data-cutscenes="true"><button type="button" class="tt-tab" data-tt-tab aria-expanded="false">Table</button></div></header>
<main id="main">page</main>
<script>
const now = Date.now(), iso = (ms) => new Date(now + ms).toISOString();
window.watched = [];
window.Chronicle = {
  tableDice: new Proxy({}, { get: (_, k) => (k === 'motion' ? () => 'off' : () => '') }), escapeHtml: (v) => String(v).replace(/</g, '&lt;'), motionMs: (n, f) => f,
  apiFetch: (url) => {
    const body = url.endsWith('/table/shown') ? { items: [{ id: 'n1', kind: 'note', text: 'The door is warm.', shownAt: iso(-600e3) }] }
      : url.endsWith('/cutscenes/mine') ? { data: [
          { showingId: 'sh1', name: 'The Eye Opens', shownAt: iso(-60e3), rewatchUntil: iso(3600e3), watched: false },
          { showingId: 'sh2', name: 'Long gone', shownAt: iso(-90000e3), rewatchUntil: iso(-1000), watched: true },
          { showingId: 'sh3', name: 'Soon gone', shownAt: iso(-30e3), rewatchUntil: iso(1500), watched: true }
        ], total: 3 }
      : url.indexOf('/rolls') >= 0 ? { rolls: [] } : null;
    return Promise.resolve({ ok: !!body, status: body ? 200 : 404, json: () => Promise.resolve(body) });
  },
  cutscenes: { watch: (id) => { window.watched.push(id); return Promise.resolve(); } },
  register: (n, d) => document.querySelectorAll('[data-widget="' + n + '"]').forEach((el) => d.init(el)),
};</script><script src="/table.js"></script></body></html>`;

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true });
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

const tonight = (pg) => pg.evaluate(() => { const l = document.getElementById('tt-tonight'); return l ? l.textContent : ''; });

test('on a phone, Tonight reads what was shown and lists cut scenes to watch', { skip }, async (t) => {
  const pg = await open(t);
  assert.equal(await pg.evaluate(() => document.documentElement.hasAttribute('data-tt-sheet')), true, 'the phone sheet is in use');
  await pg.evaluate(() => document.querySelector('.tt-host')._table.openTable('tonight'));
  await pg.waitForFunction(() => /The door is warm/.test((document.getElementById('tt-tonight') || {}).textContent || ''));
  await pg.waitForFunction(() => /The Eye Opens/.test(document.getElementById('tt-tonight').textContent));
  const text = await tonight(pg);
  assert.doesNotMatch(text, /Reading/);
  assert.match(text, /The Eye Opens\s*Missed it\s*until/);
  assert.doesNotMatch(text, /Long gone/, 'a showing past its time is not listed');
  // Oldest first: the note, then the scenes.
  assert.ok(text.indexOf('The door is warm') < text.indexOf('The Eye Opens'));

  await pg.click('[data-tt-cut="sh1"]');
  assert.deepEqual(await pg.evaluate(() => window.watched), ['sh1']);

  await pg.evaluate(() => window.dispatchEvent(new CustomEvent('chronicle:cutscene-watched', { detail: { showingId: 'sh1' } })));
  await pg.waitForFunction(() => /Watch again/.test(document.querySelector('[data-cut="sh1"]').textContent));
  assert.doesNotMatch(await pg.evaluate(() => document.querySelector('[data-cut="sh1"]').textContent), /Missed it/);

  // A row goes by itself once it can no longer be played.
  assert.match(await tonight(pg), /Soon gone/);
  await pg.waitForFunction(() => !/Soon gone/.test(document.getElementById('tt-tonight').textContent), null, { timeout: 5000 });
});
