// entity_notes_refresh_browser.test.mjs — a page refresh after someone
// else's save keeps the Player Notes widget, and a note the reader has not
// sent survives it, however the refresh lands: kept in place, the notes'
// part replaced, the mount's settings changed, or the whole page swapped.
//
// Runs the real boot.js, page_live.js, entity_notes.js and the vendored htmx
// in Chromium, with a stand-in socket and notes API. Needs playwright-core and
// a Chromium (see _browser_env.mjs); skips locally, and fails under CI, when
// either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// A hero page: a heading block, and a panel holding the notes mount and a
// second piece, as the tabs under a system sheet do. `v` says what the
// server's copy holds this time: `entry` the heading block's words, `other`
// the panel's other piece, `dm` the notes mount's role setting, `alone` the
// notes mount as its own part of the column, `swapped` the columns swapped.
function page(v) {
  const notes = '<div data-widget="entity-notes" data-entity-id="e1" data-campaign-id="c1"' +
    ' data-endpoint="/campaigns/c1/entities/e1/notes" data-current-user-id="u1"' + (v.dm ? ' data-sees-dm-notes="true"' : '') + '></div>';
  const entry = '<section data-block="entry"><p>' + (v.entry || 'Bren') + '</p></section>';
  const panel = v.alone ? notes : '<section data-widget="page-tabs"><div>' + notes + '</div><p id="other">' + (v.other || 'Links') + '</p></section>';
  const cols = v.swapped
    ? '<div data-page-col>' + panel + '</div><div data-page-col>' + entry + '</div>'
    : '<div data-page-col>' + entry + '</div><div data-page-col>' + panel + '</div>';
  return '<!doctype html><html><head><title>Bren</title></head><body>' +
    '<main id="main-content" style="height:600px;overflow-y:auto">' +
    '<div hidden data-widget="page-live" data-entity-id="e1" data-campaign-id="c1"></div>' +
    '<div data-page-frame data-whole><h1>Bren</h1><div data-page-row>' + cols + '</div></div></main>' +
    '<script src="/static/vendor/htmx.min.js" defer></script><script src="/static/js/boot.js" defer></script>' +
    '<script src="/static/js/widgets/page_tabs.js" defer></script>' +
    '<script src="/static/js/widgets/entity_notes.js" defer></script><script src="/static/js/widgets/page_live.js" defer></script></body></html>';
}

const NOTES = [{ id: 'n1', authorUserId: 'u1', audience: 'private', title: 'Mine', bodyHtml: '<p>First words</p>', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }];

async function setup(t, versions) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage({ viewport: { width: 1000, height: 700 } });
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  let served = 0;
  p.listReads = 0;
  await p.route('http://chronicle.test/**', async (r) => {
    const u = new URL(r.request().url());
    if (u.pathname.startsWith('/static/')) {
      const f = join(root, u.pathname);
      return existsSync(f) ? r.fulfill({ path: f }) : r.fulfill({ status: 404, body: '' });
    }
    if (u.pathname === '/campaigns/c1/entities/e1/notes') { p.listReads++; return r.fulfill({ contentType: 'application/json', body: JSON.stringify(NOTES) }); }
    if (u.pathname === '/campaigns/c1/members') return r.fulfill({ contentType: 'application/json', body: JSON.stringify([{ user_id: 'u1', display_name: 'Ana' }]) });
    const v = versions[Math.min(served++, versions.length - 1)];
    await r.fulfill({ contentType: 'text/html', body: page(v) });
  });
  await p.addInitScript(() => {
    window._socks = [];
    window.WebSocket = function () { window._socks.push(this); this._on = {}; setTimeout(() => this._fire('open', {}), 0); };
    window.WebSocket.prototype.addEventListener = function (t, f) { this._on[t] = f; };
    window.WebSocket.prototype._fire = function (t, e) { if (this._on[t]) this._on[t](e); };
    window.WebSocket.prototype.close = function () { this.closed = true; };
  });
  await p.goto('http://chronicle.test/campaigns/c1/entities/e1');
  await p.waitForSelector('.en-note[data-en-id="n1"]');
  return p;
}

// Types a note into New note (or into the edit of the reader's own note),
// then clicks away so nothing has focus, as #2686 describes.
async function writeDraft(p, edit) {
  await p.click(edit ? '.en-note[data-en-id="n1"] [data-en-act="edit"]' : '[data-en-act="new"]');
  await p.fill('[data-en-f="title"]', 'Half a title');
  await p.fill('.en-textarea', 'Half a thought');
  await p.click('h1');
  await p.evaluate(() => {
    const w = document.querySelector('[data-widget="entity-notes"]');
    w._kept = true;
    w.querySelector('.en-block')._kept = true;
    window._socksBefore = window._socks.length;
  });
}

// Someone else saves the page; waits until the fresh copy has landed.
async function refresh(p) {
  await p.evaluate(() => window._socks.filter((s) => !s.closed).forEach((s) => s._fire('message', { data: JSON.stringify({ type: 'page.changed', campaignId: 'c1', resourceId: 'e1' }) })));
  await p.waitForFunction(() => !document.querySelector('[data-page-live-swap]') && window.Chronicle && Chronicle.served && /data-page-row/.test(Chronicle.served), null, { timeout: 15000 });
  await p.waitForTimeout(2200);
  return p.evaluate(() => {
    const w = document.querySelector('[data-widget="entity-notes"]');
    const form = w.querySelector('form.en-comp');
    return {
      widgetKept: !!w._kept, drawingKept: !!(w.querySelector('.en-block') || {})._kept,
      form: form && form.getAttribute('aria-label'),
      title: form && form.querySelector('[data-en-f="title"]').value,
      body: form && form.querySelector('.en-textarea').value,
      focused: document.activeElement && document.activeElement.tagName,
      newSocks: window._socks.length - window._socksBefore,
      openSocks: window._socks.filter((s) => !s.closed).length,
      other: (document.getElementById('other') || {}).textContent,
      entry: document.querySelector('[data-block="entry"]').textContent,
    };
  });
}

test('a refresh that leaves the notes as they were keeps the widget, its socket and the unsent note', { skip }, async (t) => {
  const p = await setup(t, [{}, { entry: 'Bren the Bold' }]);
  await writeDraft(p);
  const reads = p.listReads;
  const r = await refresh(p);
  assert.equal(r.entry, 'Bren the Bold', 'the changed block came in');
  assert.deepEqual([r.widgetKept, r.drawingKept], [true, true], 'the notes widget was neither replaced nor redrawn');
  assert.deepEqual([r.form, r.title, r.body], ['New note', 'Half a title', 'Half a thought']);
  assert.deepEqual([r.newSocks, r.openSocks], [0, 2], 'no new socket: the page watcher and the notes one stay open');
  assert.ok(p.listReads > reads, 'the notes were read again, since they are fetched, not served');
});

test('a refresh that replaces the panel holding the notes carries the unsent note into the new copy', { skip }, async (t) => {
  const p = await setup(t, [{}, { other: 'Links changed' }]);
  await writeDraft(p);
  const r = await refresh(p);
  assert.equal(r.other, 'Links changed');
  assert.equal(r.widgetKept, false, 'the panel, notes and all, was replaced');
  assert.deepEqual([r.form, r.title, r.body], ['New note', 'Half a title', 'Half a thought']);
  assert.notEqual(r.focused, 'INPUT', 'the reopened note does not take focus');
  assert.notEqual(r.focused, 'TEXTAREA');
  assert.equal(r.openSocks, 2, 'the old copy closed its socket');
});

test('an edit of the reader\'s own note survives a replaced panel as an edit', { skip }, async (t) => {
  const p = await setup(t, [{}, { other: 'Links changed' }]);
  await writeDraft(p, true);
  const r = await refresh(p);
  assert.equal(r.widgetKept, false);
  assert.deepEqual([r.form, r.title, r.body], ['Edit note', 'Half a title', 'Half a thought']);
  assert.equal(await p.textContent('[data-en-act="save"]'), 'Save changes');
});

test('changed settings on a mount that is its own part remount it in place, the note carried', { skip }, async (t) => {
  const p = await setup(t, [{ alone: true }, { alone: true, dm: true }]);
  await writeDraft(p);
  const r = await refresh(p);
  assert.equal(r.widgetKept, true, 'the mount element stays');
  assert.equal(r.drawingKept, false, 'drawn again for the new settings');
  assert.equal(await p.getAttribute('[data-widget="entity-notes"]', 'data-sees-dm-notes'), 'true', 'the fresh settings were taken');
  assert.ok(await p.$('[data-en-filter="dm"]'), 'and drawn with: a DM filter shows');
  assert.deepEqual([r.form, r.title, r.body], ['New note', 'Half a title', 'Half a thought']);
  assert.deepEqual([r.newSocks, r.openSocks], [1, 2], 'one socket swapped for another');
});

test('a refresh that swaps the whole page carries the unsent note too', { skip }, async (t) => {
  const p = await setup(t, [{}, { swapped: true }]);
  await writeDraft(p);
  const r = await refresh(p);
  assert.equal(r.widgetKept, false);
  assert.deepEqual([r.form, r.title, r.body], ['New note', 'Half a title', 'Half a thought']);
});

test('a carried note is dropped once it is older than the carry window', { skip }, async (t) => {
  const p = await setup(t, [{}]);
  await writeDraft(p);
  const opened = await p.evaluate(async () => {
    const w = document.querySelector('[data-widget="entity-notes"]');
    Chronicle.destroyWidget(w);
    await new Promise((ok) => setTimeout(ok, 3300));
    Chronicle.mountWidget(w);
    await new Promise((ok) => setTimeout(ok, 500));
    return !!w.querySelector('form.en-comp');
  });
  assert.equal(opened, false);
});
