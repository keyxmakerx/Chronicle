// page_live_parts_browser.test.mjs — a page refresh after a save replaces
// only the parts of the page that changed, and swaps the whole page only when
// the arrangement of its parts changed.
//
// Runs the real boot.js, page_live.js and the vendored htmx in Chromium, with
// a stand-in socket. Needs playwright-core and a Chromium (see
// _browser_env.mjs); skips locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// The app's own rules for a page arriving (the frame that waits unseen for
// its widgets, the fade-in of swapped-in markup) and for Calm and Off.
const CSS = readFileSync(join(root, 'static/css/input.css'), 'utf8');
const between = (from, to) => { const i = CSS.indexOf(from); const j = CSS.indexOf(to, i); assert.ok(i >= 0 && j > i, 'input.css: ' + from); return CSS.slice(i, j); };
const STYLE = ':root{--dur-page:90ms;--ease-out:ease-out}' +
  between('@layer utilities {\n  .htmx-added {', '/* ====') +
  between('[data-whole] {', '/* Settle:') +
  between('html[data-cz-reduce] *:not(.czp *),', '/* Menus built on keyframes');
// What theme.js writes on <html> for each motion setting.
const MOTION = { full: '', calm: ' data-motion="calm" data-view-motion="calm"', off: ' data-motion="off" data-view-motion="calm"' };

// A page with a header, a row of two columns and a test widget, `probe`,
// that draws itself a moment after it mounts, as a widget waiting on its data
// does. `v` says what the server's copy holds this time.
function page(v, motion = 'full') {
  const entry = '<section data-block="entry"><p>' + v.entry + '</p><input id="note"></section>';
  const details = '<section data-block="details" style="height:900px"><p>Details</p></section>';
  const probe = '<div data-widget="' + (v.widget || 'probe') + '" data-v="' + (v.probe || 1) + '" style="min-height:0"></div>';
  const col1 = v.swapped ? details : entry + probe;
  // A tabbed panel, as under a hero sheet: it keeps itself (update) and holds
  // a widget that does not.
  const panel = '<section data-widget="panel"><h2 id="tabs">Tabs</h2><div data-widget="probe" data-inner data-v="9"></div></section>';
  const col2 = (v.swapped ? entry + probe : details) + panel;
  return '<!doctype html><html' + MOTION[motion] + '><head><title>' + (v.title || 'Bren') + '</title><style>' + STYLE + '</style></head><body style="margin:0">' +
    '<main id="main-content" style="height:600px;overflow-y:auto;padding:16px">' +
    '<div hidden data-widget="page-live" data-entity-id="e1" data-campaign-id="c1"></div>' +
    '<div data-page-frame data-whole><h1 id="name">Bren</h1><div data-page-row><div data-page-col>' + col1 + '</div><div data-page-col>' + col2 + '</div></div>' +
    '<p id="foot">Footer</p></div></main>' +
    '<script src="/static/vendor/htmx.min.js"></script><script src="/static/js/boot.js"></script>' +
    '<script>' +
    'window._inits = 0; window._updates = 0;' +
    'function draw(el, word) { el.textContent = ""; el.style.height = "0"; setTimeout(function () { el.style.height = "700px"; el.textContent = word + " " + el.getAttribute("data-v"); }, 120); }' +
    'Chronicle.register("probe", { init: function (el) { window._inits++; draw(el, "drawn"); } });' +
    'Chronicle.register("panel", { init: function () { window._panels = (window._panels || 0) + 1; }, update: function () {} });' +
    'Chronicle.register("probe-up", { init: function (el) { window._inits++; draw(el, "drawn"); },' +
    '  update: function (el, fresh) { window._updates++; el.setAttribute("data-v", fresh.getAttribute("data-v")); el.textContent = "updated " + el.getAttribute("data-v"); } });' +
    '</script><script src="/static/js/widgets/page_live.js"></script></body></html>';
}

// versions: the server's copy for the first load, then for each refresh.
async function setup(t, versions, slow = 150, motion = 'full') {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 900, height: 700 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let served = 0;
  await p.route('http://chronicle.test/**', async (r) => {
    const u = new URL(r.request().url());
    if (u.pathname.startsWith('/static/')) {
      const f = join(root, u.pathname);
      return existsSync(f) ? r.fulfill({ path: f }) : r.fulfill({ status: 404, body: '' });
    }
    if (u.pathname === '/campaigns/c1/entities') {
      return r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><head><title>Pages</title></head><body><main id="main-content"><h1>Pages</h1></main></body></html>' });
    }
    const v = versions[Math.min(served, versions.length - 1)];
    if (served++ > 0) await new Promise((ok) => setTimeout(ok, slow));
    await r.fulfill({ contentType: 'text/html', body: page(v, motion) });
  });
  await p.addInitScript(() => {
    window._socks = [];
    window.WebSocket = function () { window._socks.push(this); this._on = {}; setTimeout(() => this._fire('open', {}), 0); };
    window.WebSocket.prototype.addEventListener = function (t, f) { this._on[t] = f; };
    window.WebSocket.prototype._fire = function (t, e) { if (this._on[t]) this._on[t](e); };
    window.WebSocket.prototype.close = function () { this.closed = true; };
  });
  await p.goto('http://chronicle.test/campaigns/c1/entities/e1');
  await p.waitForFunction(() => document.querySelector('[data-v]') && /drawn/.test(document.querySelector('[data-v]').textContent) &&
    getComputedStyle(document.querySelector('[data-page-frame]')).opacity === '1');
  p.served = () => served - 1;
  return p;
}

const save = () => window._socks.filter((s) => !s.closed).forEach((s) => s._fire('message', { data: JSON.stringify({ type: 'page.changed', campaignId: 'c1', resourceId: 'e1' }) }));

// Tags every part so a test can tell kept nodes from replaced ones, fires
// the save, samples each frame until the refresh has settled, then reports.
async function refresh(p, frames = 150, top) {
  await p.evaluate(([fire, n, top]) => {
    const m = document.getElementById('main-content');
    if (top != null) m.scrollTop = top;
    window._top = m.scrollTop;
    ['#name', '[data-block="entry"]', '[data-block="details"]', '[data-v]', '#foot', '[data-page-frame]', '#tabs', '[data-inner]'].forEach((q) => { document.querySelector(q)._kept = true; });
    window._low = Infinity; window._dim = 1; window._n = 0;
    (function tick() {
      window._low = Math.min(window._low, m.scrollTop);
      let o = 1;
      for (let el = document.getElementById('name'); el; el = el.parentElement) o *= parseFloat(getComputedStyle(el).opacity);
      window._dim = Math.min(window._dim, o);
      if (++window._n < n) requestAnimationFrame(tick);
    })();
    (0, eval)(fire)();
  }, [save.toString(), frames, top]);
  await p.waitForFunction((n) => window._n >= n, frames, { timeout: 20000 });
  await p.waitForTimeout(300);
  return p.evaluate(() => {
    const kept = (q) => !!(document.querySelector(q) || {})._kept;
    const m = document.getElementById('main-content');
    return {
      kept: { name: kept('#name'), entry: kept('[data-block="entry"]'), details: kept('[data-block="details"]'), probe: kept('[data-v]'), frame: kept('[data-page-frame]') },
      panel: { tabs: kept('#tabs'), inner: kept('[data-inner]'), inits: window._panels },
      entry: document.querySelector('[data-block="entry"]').textContent,
      probe: document.querySelector('[data-v]').textContent,
      inits: window._inits, updates: window._updates,
      title: document.title, top: window._top, low: window._low, end: m.scrollTop, dim: window._dim,
      mark: m.hasAttribute('data-page-live-swap'), held: !!document.querySelector('[data-page-live-hold]'),
      watchers: window._socks.filter((s) => !s.closed).length,
    };
  });
}

test('a save that changes one block replaces only that block', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'After', title: 'Bren the Bold' }]);
  const r = await refresh(p);
  assert.equal(p.served(), 1);
  assert.equal(r.entry, 'After');
  assert.equal(r.title, 'Bren the Bold', 'the title follows the fresh copy');
  assert.deepEqual(r.kept, { name: true, entry: false, details: true, probe: false, frame: true },
    'the changed block and the widget (which may have changed behind its mount) are replaced, the rest kept');
  assert.equal(r.inits, 4, 'both widgets (the one in the panel too) mounted once more');
  assert.equal(r.dim, 1, 'the heading never dimmed');
  assert.deepEqual([r.mark, r.held, r.watchers], [false, false, 1]);
});

test('a widget that can update itself keeps its drawing', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Same', widget: 'probe-up', probe: 1 }, { entry: 'Same', widget: 'probe-up', probe: 2 }]);
  const r = await refresh(p);
  assert.equal(r.probe, 'updated 2');
  assert.deepEqual(r.kept, { name: true, entry: true, details: true, probe: true, frame: true });
  assert.deepEqual([r.inits, r.updates], [3, 1], 'it updated; only the widget in the panel mounted again');
});

test('a save that changes the arrangement swaps the whole page', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'Before', swapped: true }]);
  const r = await refresh(p);
  assert.deepEqual(r.kept, { name: false, entry: false, details: false, probe: false, frame: false });
  assert.equal(r.dim, 1, 'the heading never dimmed');
  assert.deepEqual([r.mark, r.held], [false, false]);
});

test('a reader at the bottom stays there while a replaced widget draws again', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'After' }]);
  const r = await refresh(p, 150, 99999);
  assert.ok(r.top > 0);
  assert.equal(r.low, r.top, 'never pulled up');
  assert.equal(r.end, r.top);
  assert.equal(r.held, false);
});

test('two saves in a row each update in place, with one watcher', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'One' }, { entry: 'Two' }, { entry: 'Three' }]);
  await refresh(p);
  const r = await refresh(p);
  assert.equal(p.served(), 2);
  assert.equal(r.entry, 'Three');
  assert.equal(r.kept.name, true);
  assert.deepEqual([r.dim, r.watchers, r.mark], [1, 1, false]);
});

test('a link clicked during a slow refresh wins', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'After' }], 600);
  await p.evaluate((fire) => {
    const a = document.createElement('a');
    a.href = '/campaigns/c1/entities'; a.textContent = 'Pages';
    a.setAttribute('hx-boost', 'true'); a.setAttribute('hx-target', '#main-content'); a.setAttribute('hx-select', '#main-content > *');
    document.body.appendChild(a);
    window.htmx.process(a);
    document.addEventListener('htmx:beforeRequest', function once(e) {
      if (!e.detail.requestConfig.headers['X-Page-Live']) return;
      document.removeEventListener('htmx:beforeRequest', once);
      setTimeout(() => a.click(), 300);
    });
    (0, eval)(fire)();
  }, save.toString());
  await p.waitForTimeout(3000);
  const r = await p.evaluate(() => ({ path: location.pathname, h1: document.querySelector('#main-content h1').textContent, mark: document.getElementById('main-content').hasAttribute('data-page-live-swap') }));
  assert.deepEqual(r, { path: '/campaigns/c1/entities', h1: 'Pages', mark: false });
});

test('a panel whose markup is the same stays, and the widget inside it is mounted afresh', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'After' }]);
  const r = await refresh(p);
  assert.deepEqual(r.panel, { tabs: true, inner: false, inits: 1 }, 'the panel and its tabs stay; only the widget inside is replaced');
  assert.equal(r.inits, 4, 'the two probes, each mounted once more');
});

test('a field the reader starts typing in while the refresh is on its way is never replaced', { skip }, async (t) => {
  const p = await setup(t, [{ entry: 'Before' }, { entry: 'After' }], 600);
  await p.evaluate((fire) => {
    document.querySelector('[data-block="entry"]')._kept = true;
    document.addEventListener('htmx:beforeRequest', function once(e) {
      if (!e.detail.requestConfig.headers['X-Page-Live']) return;
      document.removeEventListener('htmx:beforeRequest', once);
      setTimeout(() => { const f = document.getElementById('note'); f.focus(); f.value = 'half a thought'; }, 200);
    });
    (0, eval)(fire)();
  }, save.toString());
  await p.waitForTimeout(2500);
  const typing = await p.evaluate(() => ({
    kept: !!document.querySelector('[data-block="entry"]')._kept,
    value: document.getElementById('note').value,
    focused: document.activeElement.id,
    entry: document.querySelector('[data-block="entry"] p').textContent,
    mark: document.getElementById('main-content').hasAttribute('data-page-live-swap'),
    held: !!document.querySelector('[data-page-live-hold]'),
  }));
  assert.equal(p.served(), 1, 'the refresh did arrive');
  assert.deepEqual(typing, { kept: true, value: 'half a thought', focused: 'note', entry: 'Before', mark: false, held: false });
  // Once the reader leaves the field, the refresh comes again.
  await p.evaluate(() => document.getElementById('note').blur());
  await p.waitForFunction(() => document.querySelector('[data-block="entry"] p').textContent === 'After', null, { timeout: 10000 });
  assert.equal(p.served(), 2);
});

for (const motion of ['full', 'calm', 'off']) {
  test('with motion ' + motion + ' the heading never dims, updated in place or swapped whole', { skip }, async (t) => {
    const p = await setup(t, [{ entry: 'One' }, { entry: 'Two' }, { entry: 'Two', swapped: true }], 150, motion);
    const a = await refresh(p);
    assert.equal(a.entry, 'Two');
    assert.equal(a.kept.name, true);
    assert.equal(a.dim, 1, 'in place');
    const b = await refresh(p);
    assert.equal(b.kept.name, false, 'the arrangement changed, so the page was swapped whole');
    assert.equal(b.dim, 1, 'swapped whole');
  });
}
