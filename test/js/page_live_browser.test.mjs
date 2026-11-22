// page_live_browser.test.mjs — the page refresh after someone saves keeps the
// reader where they were, even when a widget on the fresh markup draws itself
// only after its own data arrives.
//
// Runs static/js/widgets/page_live.js with the vendored htmx in a real
// Chromium. Needs playwright-core and a Chromium (see _browser_env.mjs); skips
// locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (f) => readFileSync(join(root, f), 'utf8');
const HTMX = read('static/vendor/htmx.min.js');
const LIVE = read('static/js/widgets/page_live.js');
const { holdFor } = createRequire(import.meta.url)('../../static/js/widgets/page_live.js');

test('holdFor is what the content lacks of the old height, never negative', () => {
  assert.equal(holdFor(3000, 800), 2200);
  assert.equal(holdFor(3000, 3200), 0);
  assert.equal(holdFor(3000, 2999.6), 0);
});

// The page: a scrolling #main-content as in the app layout, a tall sheet and
// the page-live mount. The refreshed copy has the sheet empty; it draws itself
// `late` ms after the swap, as a widget waiting on its data does.
function pageHtml(fresh) {
  const sheet = fresh ? '<div id="sheet" data-late style="height:0"></div>' : '<div id="sheet" style="height:3000px"></div>';
  // The scroller's size is on the element itself, as the app's classes are.
  return '<!doctype html><html><head></head><body style="margin:0">' +
    '<main id="main-content" style="height:600px;overflow-y:auto;padding:16px"><h1>Bren</h1>' + sheet + '<div id="notes" style="height:400px">Notes</div><div id="live"></div></main></body></html>';
}

async function setup(t, late) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 800, height: 700 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let served = 0;
  await p.route('http://chronicle.test/**', (r) => r.fulfill({ contentType: 'text/html', body: pageHtml(served++ > 0) }));
  await p.goto('http://chronicle.test/campaigns/c1/entities/e1');
  await p.addScriptTag({ content: HTMX });
  // A stand-in for Chronicle's widget registry and the page's socket.
  await p.evaluate((lateMs) => {
    window.Chronicle = { register: (n, w) => { window._live = w; } };
    window.WebSocket = function () { window._sock = this; this._on = {}; setTimeout(() => this._fire('open', {}), 0); };
    window.WebSocket.prototype.addEventListener = function (t, f) { this._on[t] = f; };
    window.WebSocket.prototype._fire = function (t, e) { if (this._on[t]) this._on[t](e); };
    window.WebSocket.prototype.close = function () {};
    document.addEventListener('htmx:afterSwap', () => {
      setTimeout(() => { const s = document.querySelector('[data-late]'); if (s) s.style.height = '3000px'; }, lateMs);
    });
  }, late);
  await p.addScriptTag({ content: LIVE });
  await p.evaluate(() => window._live.init(document.getElementById('live'), { campaignId: 'c1', entityId: 'e1' }));
  return p;
}

// Scrolls to the notes under the sheet, sends page.changed, and samples the
// scroll position every frame until the refresh has settled.
async function refreshWhileScrolled(p) {
  const top = await p.evaluate(() => { const m = document.getElementById('main-content'); m.scrollTop = 2900; return m.scrollTop; });
  await p.evaluate(() => {
    window._tops = [];
    const m = () => document.getElementById('main-content');
    (function tick() { window._tops.push(m().scrollTop); if (window._tops.length < 240) requestAnimationFrame(tick); })();
    window._sock._fire('message', { data: JSON.stringify({ type: 'page.changed', campaignId: 'c1', resourceId: 'e1' }) });
  });
  await p.waitForFunction(() => window._tops.length >= 240, null, { timeout: 15000 });
  const tops = await p.evaluate(() => window._tops);
  const after = await p.evaluate(() => { const m = document.getElementById('main-content'); return { top: m.scrollTop, held: !!document.querySelector('[data-page-live-hold]'), fresh: !!document.querySelector('[data-late]') }; });
  return { top, tops, after };
}

test('a widget that draws late: the reader stays where they were through the refresh', { skip }, async (t) => {
  const p = await setup(t, 400);
  const r = await refreshWhileScrolled(p);
  assert.ok(r.after.fresh, 'the page was refreshed');
  assert.equal(Math.min(...r.tops), r.top, 'never pulled up: ' + [...new Set(r.tops)].join(','));
  assert.equal(r.after.top, r.top);
  assert.equal(r.after.held, false, 'the held height is let go once the page has grown back');
});

test('a reader who scrolls during the refresh is left where they scrolled', { skip }, async (t) => {
  const p = await setup(t, 1500);
  await p.evaluate(() => { document.getElementById('main-content').scrollTop = 2900; });
  await p.evaluate(() => window._sock._fire('message', { data: JSON.stringify({ type: 'page.changed', campaignId: 'c1', resourceId: 'e1' }) }));
  await p.waitForFunction(() => !!document.querySelector('[data-late]'), null, { timeout: 10000 });
  await p.mouse.move(400, 300);
  await p.mouse.wheel(0, -1000);
  await p.waitForTimeout(300);
  const mid = await p.evaluate(() => document.getElementById('main-content').scrollTop);
  assert.ok(mid < 2900, 'the wheel moved the page: ' + mid);
  await p.waitForTimeout(1600);
  const end = await p.evaluate(() => ({ top: document.getElementById('main-content').scrollTop, held: !!document.querySelector('[data-page-live-hold]') }));
  assert.equal(end.top, mid, 'not pulled back');
  assert.equal(end.held, false);
});
