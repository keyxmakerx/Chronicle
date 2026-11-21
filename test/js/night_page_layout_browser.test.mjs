// night_page_layout_browser.test.mjs — a date-less night's own page at phone
// width: the line under the title wraps rather than squeezing its date
// wording into a narrow column, nothing scrolls sideways, and the Real date
// field and button are at least 44px tall. The page is the real Templ output
// (dumped by a Go test), styled with the built static/css/app.css; the CDN
// fonts and icons are not fetched. Skips locally, and fails under CI, when
// playwright-core or a Chromium is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync, existsSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function renderPage() {
  const out = join(mkdtempSync(join(tmpdir(), 'night-page-')), 'page.html');
  execFileSync('go', ['test', './internal/plugins/sessions', '-run', '^TestDumpNightPageForBrowser$', '-count=1'],
    { cwd: root, env: { ...process.env, NIGHT_PAGE_OUT: out }, stdio: 'pipe' });
  return readFileSync(out, 'utf8');
}

test('the night page at 360 wide wraps its meta line and keeps the date card tappable', { skip: skip || (!existsSync(join(root, 'static/css/app.css')) && 'static/css/app.css is not built (make tailwind)') }, async (t) => {
  const html = renderPage();
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 360, height: 800 } });
  await p.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.hostname !== 'chronicle.test') return route.abort();
    if (u.pathname === '/page') return route.fulfill({ contentType: 'text/html', body: html });
    const file = join(root, u.pathname);
    if (u.pathname.startsWith('/static/') && existsSync(file)) {
      return route.fulfill({ contentType: u.pathname.endsWith('.css') ? 'text/css' : 'application/javascript', body: readFileSync(file) });
    }
    return route.fulfill({ status: 404, body: '' });
  });
  await p.goto('http://chronicle.test/page', { waitUntil: 'domcontentloaded' });

  const words = await p.$eval('[data-testid="no-real-date"]', (el) => {
    const r = el.getBoundingClientRect();
    return { h: r.height, w: r.width, line: parseFloat(getComputedStyle(el).lineHeight) };
  });
  assert.ok(words.h <= words.line * 2.2, `the date wording takes ${Math.round(words.h / words.line)} lines at ${Math.round(words.w)}px wide`);

  const scrollsSideways = await p.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  assert.equal(scrollsSideways, false, 'the page scrolls sideways');

  for (const sel of ['#night-real-date', '#night-real-date-btn']) {
    const h = await p.$eval(sel, (el) => el.getBoundingClientRect().height);
    assert.ok(h >= 44, `${sel} is ${h}px tall`);
  }
});
