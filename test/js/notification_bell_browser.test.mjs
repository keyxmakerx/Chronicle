// notification_bell_browser.test.mjs — marking one notification read from the
// bell: its row-end icon marks it without leaving the page, clicking a row
// with no link marks it too, and the badge is refetched after each.
//
// Runs boot.js and the widget in a real Chromium against a stubbed server.
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// The bell's markup, as the app topbar (layouts/app.templ) renders it.
const PAGE = `<!doctype html><html><head>
<script src="/htmx.js"></script><script src="/boot.js"></script><script src="/bell.js"></script></head><body>
<div class="relative" data-widget="notification-bell" data-csrf="t">
  <button type="button" data-notif-toggle aria-label="Notifications"><span data-notif-badge></span></button>
  <div data-notif-panel hidden>
    <button type="button" data-notif-markall>Mark all read</button>
    <div data-notif-list></div>
  </div>
</div>
</body></html>`;

const ITEMS = [
  { id: 'n1', message: 'Bren gave you a sword', link: '/elsewhere', read: false, type: 'item_given', createdAt: '2026-11-01T00:00:00Z' },
  { id: 'n2', message: 'No link here', link: '', read: false, type: 'proposal_created', createdAt: '2026-11-01T00:00:00Z' },
  { id: 'n3', message: 'Already read', link: '', read: true, type: 'proposal_created', createdAt: '2026-11-01T00:00:00Z' },
];

async function open({ failRead = false } = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const page = await browser.newPage();
  page.setDefaultTimeout(5000);
  const state = { reads: [], badgeHits: 0, unread: 2 };
  await page.route('http://t.test/**', (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/bell.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/widgets/notification_bell.js')) });
    if (p === '/notifications') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(ITEMS) });
    if (p === '/notifications/badge') {
      state.badgeHits++;
      return route.fulfill({ contentType: 'text/html', body: state.unread ? `<span data-count="${state.unread}">${state.unread}</span>` : '' });
    }
    const m = p.match(/^\/notifications\/([^/]+)\/read$/);
    if (m && req.method() === 'POST') {
      state.reads.push(m[1]);
      if (failRead) return route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"x"}' });
      state.unread--;
      return route.fulfill({ contentType: 'application/json', body: '{"status":"ok"}' });
    }
    return route.fulfill({ contentType: 'text/html', body: PAGE });
  });
  try {
    await page.goto('http://t.test/');
    await page.click('[data-notif-toggle]');
    await page.waitForSelector('[data-notif-row="n1"]');
  } catch (e) {
    // Close here: the caller's finally never runs when open() throws, and a
    // live browser would keep the test process from exiting.
    await browser.close();
    throw e;
  }
  return { browser, page, state };
}

const rowState = (page, id) => page.evaluate((id) => {
  const row = document.querySelector('[data-notif-row="' + id + '"]');
  const btn = row.querySelector('[data-notif-markone]');
  return { read: row.getAttribute('data-notif-read'), iconHidden: btn.classList.contains('hidden') };
}, id);

test('the row-end icon marks one read, stays on the page and refreshes the badge', { skip }, async () => {
  const { browser, page, state } = await open();
  try {
    assert.deepEqual(await rowState(page, 'n1'), { read: 'false', iconHidden: false });
    assert.deepEqual(await rowState(page, 'n3'), { read: 'true', iconHidden: true });
    await page.click('[data-notif-row="n1"] [data-notif-markone]');
    await page.waitForFunction(() => document.querySelector('[data-notif-badge]').textContent.trim() === '1');
    assert.deepEqual(state.reads, ['n1']);
    assert.equal(new URL(page.url()).pathname, '/', 'the icon does not follow the link');
    assert.equal(await page.isVisible('[data-notif-panel]'), true, 'the panel stays open');
    assert.deepEqual(await rowState(page, 'n1'), { read: 'true', iconHidden: true });
    assert.deepEqual(await rowState(page, 'n2'), { read: 'false', iconHidden: false });
  } finally { await browser.close(); }
});

test('clicking a row with no link marks it read', { skip }, async () => {
  const { browser, page, state } = await open();
  try {
    await page.click('[data-notif-row="n2"] button:not([data-notif-markone])');
    await page.waitForFunction(() => document.querySelector('[data-notif-badge]').textContent.trim() === '1');
    assert.deepEqual(state.reads, ['n2']);
    assert.deepEqual(await rowState(page, 'n2'), { read: 'true', iconHidden: true });
    // A second click sends nothing more.
    await page.click('[data-notif-row="n2"] button:not([data-notif-markone])');
    await page.waitForTimeout(100);
    assert.deepEqual(state.reads, ['n2']);
  } finally { await browser.close(); }
});

test('clicking a linked row sends the read before following the link', { skip }, async () => {
  const { browser, page, state } = await open();
  try {
    await Promise.all([page.waitForURL('http://t.test/elsewhere'), page.click('[data-notif-row="n1"] a')]);
    await page.waitForTimeout(100);
    assert.deepEqual(state.reads, ['n1']);
  } finally { await browser.close(); }
});

test('a refused read puts the row back to unread', { skip }, async () => {
  const { browser, page } = await open({ failRead: true });
  try {
    await page.click('[data-notif-row="n2"] [data-notif-markone]');
    await page.waitForFunction(() => document.querySelector('[data-notif-row="n2"]').getAttribute('data-notif-read') === 'false');
    assert.deepEqual(await rowState(page, 'n2'), { read: 'false', iconHidden: false });
  } finally { await browser.close(); }
});
