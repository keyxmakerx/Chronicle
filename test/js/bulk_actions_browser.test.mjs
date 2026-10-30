// bulk_actions_browser.test.mjs — the bulk bar on a page list: each change
// is picked from a list (never a typed id), applies at once, and the Undo
// message after the reload replays the reverse of what the server changed.
//
// Runs static/js/widgets/bulk_actions.js in a real Chromium with a stubbed
// API. Needs playwright-core and a Chromium (see _browser_env.mjs).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const BULK_JS = readFileSync(join(root, 'static/js/widgets/bulk_actions.js'), 'utf8');

const LIST = '<div id="list" data-campaign-id="c1" data-role="3" data-can-hide="true"' +
  ' data-entity-types=\'[{"id":1,"name":"NPC"},{"id":2,"name":"Place"}]\' data-tags="[]">' +
  '<table><thead><tr><th>Name</th></tr></thead><tbody>' +
  ['e1', 'e2', 'e3', 'e4'].map((id) => '<tr data-entity-id="' + id + '"><td>' + id + '</td></tr>').join('') +
  '</tbody></table></div><div id="toasts"></div>';

const STUB = `
  window.calls = []; window.reloads = 0;
  window.replies = {
    '/campaigns/c1/entities/bulk-type': { updated: 2, previous: { e1: 2, e2: 1 } },
    '/campaigns/c1/entities/bulk-tags': { processed: 2, added: { e1: [7] } },
    '/campaigns/c1/entities/bulk-delete': { deleted: 2, deletedIds: ['e1', 'e2'] },
    '/campaigns/c1/tags': [{ id: 7, name: 'smugglers' }],
  };
  window.Chronicle = {
    register: (name, w) => { window.widget = w; },
    escapeHtml: (s) => String(s).replace(/[&<>"']/g, (c) => '&#' + c.charCodeAt(0) + ';'),
    notify: (msg, type, opts) => {
      const d = document.createElement('div'); d.className = 'toast ' + type;
      if (opts && opts.html) d.innerHTML = msg; else d.textContent = msg;
      document.getElementById('toasts').appendChild(d);
    },
    apiFetch: (url, opts) => {
      window.calls.push({ url, method: (opts && opts.method) || 'GET', body: opts && opts.body });
      const r = window.replies[url] || {};
      return Promise.resolve({ ok: true, json: () => Promise.resolve(r) });
    },
  };
  Object.defineProperty(window, 'reloadPage', { value: () => { window.reloads++; } });
`;

async function open(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  p.on('dialog', (d) => { errors.push('native dialog: ' + d.message()); d.dismiss(); });
  await p.route('**/*', (r) => r.fulfill({ contentType: 'text/html', body: '<!doctype html><html><body>' + LIST + '</body></html>' }));
  await p.goto('http://chronicle.test/campaigns/c1/entities');
  await p.addScriptTag({ content: STUB });
  // A reload is the page asking to show the change; here it re-mounts in place.
  await p.addScriptTag({ content: BULK_JS.replace(/window\.location\.reload\(\)/g, 'window.reloadPage()') });
  await p.evaluate(() => window.widget.init(document.getElementById('list')));
  return p;
}
const tick = (p, id) => p.click('input[data-bulk-id="' + id + '"]');
const remount = (p) => p.evaluate(() => {
  document.querySelectorAll('.bulk-action-bar, .bulk-picker').forEach((n) => n.remove());
  const old = document.getElementById('list'); const fresh = old.cloneNode(false); fresh.innerHTML = old.innerHTML;
  fresh.querySelectorAll('.bulk-checkbox, .bulk-select-all').forEach((n) => n.remove());
  old.replaceWith(fresh); window.widget.init(fresh);
});

test('changing the category picks from a list, and Undo puts each page back', { skip }, async (t) => {
  const p = await open(t);
  await tick(p, 'e1'); await tick(p, 'e2');
  assert.match(await p.textContent('.bulk-action-bar'), /2 selected/);
  await p.click('[data-bulk-action="type"]');
  await p.fill('.bulk-picker input', 'pl');
  await p.keyboard.press('Enter');
  await p.waitForFunction(() => window.reloads === 1);
  const set = await p.evaluate(() => window.calls.find((c) => c.url.endsWith('bulk-type')));
  assert.deepEqual(set.body, { entity_ids: ['e1', 'e2'], entity_type_id: 2 });

  await remount(p);
  await p.click('#toasts button');
  await p.waitForFunction(() => window.reloads === 2);
  const undo = await p.evaluate(() => window.calls.filter((c) => c.url.endsWith('bulk-type')).slice(1).map((c) => c.body));
  assert.deepEqual(undo.sort((a, b) => a.entity_type_id - b.entity_type_id),
    [{ entity_ids: ['e2'], entity_type_id: 1 }, { entity_ids: ['e1'], entity_type_id: 2 }]);
});

test('a tag is found by name, and Undo takes it off only where it was added', { skip }, async (t) => {
  const p = await open(t);
  await tick(p, 'e1'); await tick(p, 'e2');
  await p.click('[data-bulk-action="add-tags"]');
  await p.waitForSelector('.bulk-picker [role="option"]');
  await p.fill('.bulk-picker input', 'smug');
  await p.keyboard.press('Enter');
  await p.waitForFunction(() => window.reloads === 1);
  await remount(p);
  await p.click('#toasts button');
  await p.waitForFunction(() => window.reloads === 2);
  const tagCalls = await p.evaluate(() => window.calls.filter((c) => c.url.endsWith('bulk-tags')).map((c) => c.body));
  assert.deepEqual(tagCalls, [
    { entity_ids: ['e1', 'e2'], tag_ids: [7], action: 'add' },
    { entity_ids: ['e1'], tag_ids: [7], action: 'remove' },
  ]);
});

test('Trash asks nothing, and Undo restores each page from the Trash', { skip }, async (t) => {
  const p = await open(t);
  await tick(p, 'e1'); await tick(p, 'e2');
  await p.click('[data-bulk-action="delete"]');
  await p.waitForFunction(() => window.reloads === 1);
  await remount(p);
  assert.match(await p.textContent('#toasts'), /Moved 2 pages to the Trash/);
  await p.click('#toasts button');
  await p.waitForFunction(() => window.reloads === 2);
  const restores = await p.evaluate(() => window.calls.filter((c) => c.url.includes('/trash/')).map((c) => c.url));
  assert.deepEqual(restores, ['/campaigns/c1/trash/e1/restore', '/campaigns/c1/trash/e2/restore']);
});

test('shift-click ticks the range, and Select all takes the rest', { skip }, async (t) => {
  const p = await open(t);
  await tick(p, 'e1');
  await p.click('input[data-bulk-id="e3"]', { modifiers: ['Shift'] });
  assert.match(await p.textContent('.bulk-action-bar'), /3 selected/);
  await p.click('[data-bulk-action="all"]');
  assert.match(await p.textContent('.bulk-action-bar'), /4 selected/);
});

test('nothing in the widget opens a browser dialog', () => {
  assert.doesNotMatch(BULK_JS, /\b(prompt|confirm|alert)\(/);
});
