// sidebar_tree_menu_browser.test.mjs — the row menu on a category panel's
// pages and folders: what each viewer is offered, and that each choice sends
// the request its route expects.
//
// Runs the real htmx, sidebar_tree.js and sidebar_row_menu.js in Chromium on
// the markup SidebarEntityList draws (sidebar_list.templ). Only the network is
// stubbed. Needs playwright-core and a Chromium (see _browser_env.mjs); skips
// locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

/** The tree as the server draws it; flags are the viewer's data-can-* attributes. */
function tree(flags, extraFolder) {
  return `<div id="sidebar-entity-tree" class="sidebar-entity-list" data-campaign-id="c1" data-entity-type-id="7" ${flags}>
  <div class="sidebar-tree-item sidebar-tree-folder tree-row is-folder" data-node-id="n1" data-is-folder="true" data-sort-order="0" data-entity-name="Lore"><span class="tree-lb">Lore</span></div>
  <div class="sidebar-tree-item sidebar-tree-folder tree-row is-folder" data-node-id="n2" data-is-folder="true" data-parent-id="n1" data-sort-order="0" data-entity-name="Gods"><span class="tree-lb">Gods</span></div>
  ${extraFolder ? '<div class="sidebar-tree-item sidebar-tree-folder tree-row is-folder" data-node-id="n9" data-is-folder="true" data-parent-id="n1" data-sort-order="1" data-entity-name="New folder"><span class="tree-lb">New folder</span></div>' : ''}
  <a href="/campaigns/c1/entities/e1" class="sidebar-tree-item tree-row is-page" data-entity-id="e1" data-sort-order="1" data-entity-name="Mara Vell"><span class="tree-lb">Mara Vell</span></a>
  <a href="/campaigns/c1/entities/e2" class="sidebar-tree-item tree-row is-page" data-entity-id="e2" data-parent-node-id="n1" data-sort-order="0" data-entity-name="Sun Temple"><span class="tree-lb">Sun Temple</span></a>
</div>`;
}

function page(flags) {
  return `<!doctype html><html><head><script src="/htmx.js"></script></head><body>
<script>
window.sent = [];
window.notes = [];
window.Chronicle = {
  apiFetch: function (url, opts) {
    sent.push({ url: url, method: opts.method, body: opts.body === undefined ? null : opts.body });
    var json = url === '/campaigns/c1/sidebar-nodes' ? { id: 'n9', name: 'New folder' } : { status: 'ok' };
    return Promise.resolve({ ok: true, status: 200, json: function () { return Promise.resolve(json); } });
  },
  notify: function (m, k) { notes.push(k + ': ' + m); }
};
</script>
<aside id="sidebar"><div id="sidebar-drill" class="nav-drill is-open"><div id="sidebar-cat-content">
  <input type="search" name="q" hx-get="/search?flags=${encodeURIComponent(flags)}" hx-target="#sidebar-cat-results">
  <div id="sidebar-cat-results">${tree(flags, false)}</div>
</div></div></aside>
<script src="/sidebar_tree.js"></script><script src="/sidebar_row_menu.js"></script>
</body></html>`;
}

async function open(flags) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const pg = await browser.newPage();
  await pg.route('http://t.test/**', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/htmx.js') return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (u.pathname.endsWith('.js')) return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/js', u.pathname.slice(1))) });
    if (u.pathname === '/search') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: tree(u.searchParams.get('flags'), true) });
    return route.fulfill({ contentType: 'text/html; charset=utf-8', body: page(flags) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.sidebar-tree-node[data-entity-id="e1"]');
  return { browser, pg };
}

const row = (sel) => `#sidebar-entity-tree .sidebar-tree-node[${sel}] > .sidebar-tree-item`;
const menuLabels = (pg) => pg.$$eval('.rowmenu .rowmenu-item', (bs) => bs.map((b) => (b.disabled ? '(off) ' : '') + b.textContent.trim()));
const pick = (pg, label) => pg.locator('.rowmenu .rowmenu-item', { hasText: label }).first().click();

const EDITOR = 'data-can-edit="true"';
const OWNER = 'data-can-edit="true" data-can-delete-folders="true"';

test('a player gets open, new tab and copy link on a page, and the browser menu on a folder', { skip }, async () => {
  const { browser, pg } = await open('');
  try {
    await pg.click(row('data-entity-id="e1"'), { button: 'right' });
    assert.deepEqual(await menuLabels(pg), ['Open', 'Open in new tab', 'Copy link']);
    await pg.keyboard.press('Escape');
    assert.equal(await pg.$('.rowmenu'), null);
    assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('data-entity-id')), 'e1');
    // Nothing to offer on a folder, so the menu stays out of the way.
    await pg.click(row('data-node-id="n1"'), { button: 'right' });
    assert.equal(await pg.$('.rowmenu'), null);
    assert.equal(await pg.$('.sidebar-tree-node[data-node-id="n1"] > .tree-more'), null);
  } finally { await browser.close(); }
});

test('an editor moves a page into a folder from the menu', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.click(row('data-entity-id="e1"'), { button: 'right' });
    assert.deepEqual(await menuLabels(pg), ['Open', 'Open in new tab', 'Copy link', 'New page inside', 'New folder here', 'Rename', 'Move to folder…']);
    await pick(pg, 'Move to folder');
    // The page is at the top level already, so that choice is off.
    assert.deepEqual(await menuLabels(pg), ['Back', '(off) Top level', 'Lore', 'Gods']);
    await pick(pg, 'Gods');
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent), [{ url: '/campaigns/c1/entities/e1/reorder', method: 'PUT', body: { parent_id: null, parent_node_id: 'n2', sort_order: 1000000 } }]);
  } finally { await browser.close(); }
});

test('a folder cannot be moved into itself or a folder inside it', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.click(row('data-node-id="n1"'), { button: 'right' });
    assert.deepEqual(await menuLabels(pg), ['New folder inside', 'Rename', 'Move to folder…', 'Order inside…']);
    await pick(pg, 'Move to folder');
    assert.deepEqual(await menuLabels(pg), ['Back', '(off) Top level']);
    await pick(pg, 'Back');
    assert.deepEqual(await menuLabels(pg), ['New folder inside', 'Rename', 'Move to folder…', 'Order inside…']);
  } finally { await browser.close(); }
});

test('renaming a page saves only its name', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.click(row('data-entity-id="e2"'), { button: 'right' });
    await pick(pg, 'Rename');
    const input = pg.locator('.sidebar-tree-node[data-entity-id="e2"] input');
    await input.fill('Moon Temple');
    await input.press('Enter');
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent), [{ url: '/campaigns/c1/entities/e2/metadata', method: 'PUT', body: { name: 'Moon Temple' } }]);
  } finally { await browser.close(); }
});

test('a new folder inside a folder is made, placed, and opened for its name', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.click(row('data-node-id="n1"'), { button: 'right' });
    await pick(pg, 'New folder inside');
    await pg.waitForSelector('.sidebar-tree-node[data-node-id="n9"] input');
    assert.deepEqual(await pg.evaluate(() => sent), [
      { url: '/campaigns/c1/sidebar-nodes', method: 'POST', body: { name: 'New folder', entity_type_id: 7 } },
      { url: '/campaigns/c1/sidebar-nodes/n9/reorder', method: 'PUT', body: { parent_id: 'n1' } },
    ]);
  } finally { await browser.close(); }
});

test('only the owner deletes a folder, and only after a second step', { skip }, async () => {
  const { browser, pg } = await open(OWNER);
  try {
    await pg.click(row('data-node-id="n2"'), { button: 'right' });
    assert.deepEqual(await menuLabels(pg), ['New folder inside', 'Rename', 'Move to folder…', 'Delete folder']);
    await pick(pg, 'Delete folder');
    assert.deepEqual(await pg.evaluate(() => sent), []);
    assert.deepEqual(await menuLabels(pg), ['Back', 'Delete "Gods"', '(off) Its pages move to the top level.']);
    await pick(pg, 'Delete "Gods"');
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent), [{ url: '/campaigns/c1/sidebar-nodes/n2', method: 'DELETE', body: null }]);
  } finally { await browser.close(); }
});

test('the dots beside a page open the same menu', { skip }, async () => {
  const { browser, pg } = await open('');
  try {
    await pg.click('.sidebar-tree-node[data-entity-id="e1"] > .tree-more', { force: true });
    assert.deepEqual(await menuLabels(pg), ['Open', 'Open in new tab', 'Copy link']);
    assert.equal(await pg.getAttribute('.sidebar-tree-node[data-entity-id="e1"] > .tree-more', 'aria-expanded'), 'true');
  } finally { await browser.close(); }
});
