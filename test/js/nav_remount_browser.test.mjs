// nav_remount_browser.test.mjs — following a boosted link to another page
// mounts the new page's widgets once and never starts the old page's again.
//
// htmx fires htmx:pushedIntoHistory, and boot.js chronicle:navigated, before
// the swap. The jots panel (notes.js) remounts itself on that event; if it
// scanned wider than its own element it would start the page being left,
// whose widgets boot.js has just destroyed, and the swap would then remove
// them still running: a socket left open per navigation.
//
// Runs the real boot.js, htmx, jot_place.js, notes.js, entity_notes.js and
// page_live.js in Chromium, with a stand-in socket and API. Needs
// playwright-core and a Chromium (see _browser_env.mjs); skips locally, and
// fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// A page as the layout draws one: a boosted sidebar, #main-content holding
// the page watcher and the page's notes, and the jots panel outside it.
function page(id) {
  const links = ['e1', 'e2', 'e3'].map((e) => '<a id="to-' + e + '" href="/campaigns/c1/entities/' + e + '">' + e + '</a>').join('');
  return '<!doctype html><html><head><title>' + id + '</title></head><body>' +
    '<nav hx-boost="true" hx-target="#main-content" hx-select="#main-content > *" hx-swap="innerHTML">' + links + '</nav>' +
    '<main id="main-content">' +
    '<div hidden data-widget="page-live" data-entity-id="' + id + '" data-campaign-id="c1"></div>' +
    '<h1>' + id + '</h1>' +
    '<div data-widget="entity-notes" data-entity-id="' + id + '" data-campaign-id="c1" data-endpoint="/campaigns/c1/entities/' + id + '/notes" data-current-user-id="u1"></div>' +
    '</main>' +
    '<div data-widget="notes" data-campaign-id="c1" data-entity-id="' + id + '" data-user-id="u1"></div>' +
    '<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>' +
    '<script src="/static/js/widgets/jot_place.js" defer></script><script src="/static/js/widgets/notes.js" defer></script>' +
    '<script src="/static/js/widgets/entity_notes.js" defer></script><script src="/static/js/widgets/page_live.js" defer></script></body></html>';
}

async function setup(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1000, height: 700 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  p.reads = [];
  await p.route('http://chronicle.test/**', async (r) => {
    const u = new URL(r.request().url());
    if (u.pathname.startsWith('/static/')) {
      const f = join(root, u.pathname);
      return existsSync(f) ? r.fulfill({ path: f }) : r.fulfill({ status: 404, body: '' });
    }
    const m = u.pathname.match(/^\/campaigns\/c1\/entities\/(e\d)$/);
    if (m) return r.fulfill({ contentType: 'text/html', body: page(m[1]) });
    p.reads.push(u.pathname);
    return r.fulfill({ contentType: 'application/json', body: '[]' });
  });
  await p.addInitScript(() => {
    window._socks = [];
    window.WebSocket = function (url) { this.url = url; window._socks.push(this); this._on = {}; setTimeout(() => this._fire('open', {}), 0); };
    window.WebSocket.prototype.addEventListener = function (t, f) { this._on[t] = f; };
    window.WebSocket.prototype._fire = function (t, e) { if (this._on[t]) this._on[t](e); };
    window.WebSocket.prototype.close = function () { this.closed = true; };
  });
  await p.goto('http://chronicle.test/campaigns/c1/entities/e1');
  await p.waitForSelector('[data-widget="entity-notes"] .en-block');
  // Every entity-notes mount from here on, by page.
  await p.evaluate(() => {
    window._inits = [];
    const w = Chronicle.widgets['entity-notes'];
    const init = w.init;
    w.init = function (el) { window._inits.push(el.getAttribute('data-entity-id')); return init.apply(this, arguments); };
  });
  return p;
}

async function go(p, id) {
  await p.click('#to-' + id);
  await p.waitForFunction((id) => document.title === id || document.querySelector('h1').textContent === id, id);
  await p.waitForSelector('[data-widget="entity-notes"][data-entity-id="' + id + '"] .en-block');
  await p.waitForTimeout(300);
  return p.evaluate(() => ({
    inits: window._inits.slice(),
    open: window._socks.filter((s) => !s.closed).length,
  }));
}

test('leaving a page by a boosted link never starts its widgets again', { skip }, async (t) => {
  const p = await setup(t);
  const start = await p.evaluate(() => window._socks.filter((s) => !s.closed).length);
  const r = await go(p, 'e2');
  assert.deepEqual(r.inits, ['e2'], 'the old page\'s notes were not mounted again on the way out');
  assert.equal(r.open, start, 'sockets stay level: the old page\'s notes closed theirs, the new one opened one');
});

test('sockets stay level across five navigations', { skip }, async (t) => {
  const p = await setup(t);
  const start = await p.evaluate(() => window._socks.filter((s) => !s.closed).length);
  let r;
  for (const id of ['e2', 'e3', 'e1', 'e2', 'e3']) r = await go(p, id);
  assert.deepEqual(r.inits, ['e2', 'e3', 'e1', 'e2', 'e3'], 'one mount per page shown');
  assert.equal(r.open, start);
});

test('a note left unsent comes back when the reader returns to the page at once', { skip }, async (t) => {
  const p = await setup(t);
  await p.click('[data-en-act="new"]');
  await p.fill('.en-textarea', 'Half a thought');
  await go(p, 'e2');
  await go(p, 'e1');
  await p.waitForSelector('form.en-comp');
  assert.equal(await p.inputValue('.en-textarea'), 'Half a thought', 'no stray copy of the old page took the note on the way out');
});
