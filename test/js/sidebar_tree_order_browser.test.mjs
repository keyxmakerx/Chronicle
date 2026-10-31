// sidebar_tree_order_browser.test.mjs — each list in a category's page panel
// keeps its own order, and a list kept by hand moves its rows with arrows,
// Alt+Up/Down and the row menu, keeping the arrows on the row that moved.
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

// The arrows' own rules from input.css, so their show and hide is the real one.
const css = readFileSync(join(root, 'static/css/input.css'), 'utf8');
const stepCSS = css.slice(css.indexOf('/* A list kept by hand'), css.indexOf("/* The category's order picker"));

/** A category kept by hand at the top level; folder n1 is newest first. */
function tree(flags, rootOrder) {
  return `<div class="tree-order"><select data-tree-order><option value="az">A to Z</option><option value="hand" selected>By hand</option><option value="new">Newest first</option></select></div>
<div id="sidebar-entity-tree" class="sidebar-entity-list" data-campaign-id="c1" data-entity-type-id="7" data-root-order="${rootOrder}" ${flags}>
  <div class="sidebar-tree-item sidebar-tree-folder tree-row is-folder" data-node-id="n1" data-is-folder="true" data-sort-order="0" data-entity-name="Lore" data-created="10" data-child-order="new"><span class="tree-lb">Lore</span></div>
  <a href="/campaigns/c1/entities/e1" class="sidebar-tree-item tree-row is-page" data-entity-id="e1" data-sort-order="2" data-entity-name="Ash" data-created="1"><span class="tree-lb">Ash</span></a>
  <a href="/campaigns/c1/entities/e2" class="sidebar-tree-item tree-row is-page" data-entity-id="e2" data-sort-order="0" data-entity-name="Bryn" data-created="2"><span class="tree-lb">Bryn</span></a>
  <a href="/campaigns/c1/entities/e3" class="sidebar-tree-item tree-row is-page" data-entity-id="e3" data-sort-order="1" data-entity-name="Cass" data-created="3"><span class="tree-lb">Cass</span></a>
  <a href="/campaigns/c1/entities/e4" class="sidebar-tree-item tree-row is-page" data-entity-id="e4" data-parent-node-id="n1" data-entity-name="Old" data-created="5"><span class="tree-lb">Old</span></a>
  <a href="/campaigns/c1/entities/e5" class="sidebar-tree-item tree-row is-page" data-entity-id="e5" data-parent-node-id="n1" data-entity-name="Young" data-created="9"><span class="tree-lb">Young</span></a>
</div>`;
}

function page(flags, rootOrder) {
  return `<!doctype html><html><head><script src="/htmx.js"></script><style>${stepCSS}</style></head><body>
<script>
window.sent = [];
window.notes = [];
window.Chronicle = {
  apiFetch: function (url, opts) {
    sent.push({ url: url, method: opts.method, body: opts.body === undefined ? null : opts.body });
    var ok = !window.failNext;
    window.failNext = false;
    return Promise.resolve({ ok: ok, status: ok ? 200 : 500, json: function () { return Promise.resolve({ status: 'ok' }); } });
  },
  notify: function (m, k) { notes.push(k + ': ' + m); }
};
</script>
<aside id="sidebar"><div id="sidebar-drill" class="nav-drill is-open"><div id="sidebar-cat-content">
  <input type="search" name="q" hx-get="/search?flags=${encodeURIComponent(flags)}&order=${rootOrder}" hx-target="#sidebar-cat-results">
  <div id="sidebar-cat-results">${tree(flags, rootOrder)}</div>
</div></div></aside>
<script src="/sidebar_tree.js"></script><script src="/sidebar_row_menu.js"></script>
</body></html>`;
}

async function open(flags, rootOrder = 'hand') {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const pg = await browser.newPage();
  await pg.route('http://t.test/**', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/htmx.js') return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (u.pathname.endsWith('.js')) return route.fulfill({ contentType: 'text/javascript; charset=utf-8', body: readFileSync(join(root, 'static/js', u.pathname.slice(1))) });
    if (u.pathname === '/search') return route.fulfill({ contentType: 'text/html; charset=utf-8', body: tree(u.searchParams.get('flags'), u.searchParams.get('order')) });
    return route.fulfill({ contentType: 'text/html; charset=utf-8', body: page(flags, rootOrder) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.sidebar-tree-node[data-entity-id="e1"]');
  return { browser, pg };
}

const EDITOR = 'data-can-edit="true"';
const topNames = (pg) => pg.$$eval('#sidebar-entity-tree > .sidebar-tree-node > .sidebar-tree-item .tree-lb', (ls) => ls.map((l) => l.textContent));
const folderNames = (pg) => pg.$$eval('.sidebar-tree-node[data-node-id="n1"] .sidebar-tree-children .tree-lb', (ls) => ls.map((l) => l.textContent));
const menuLabels = (pg) => pg.$$eval('.rowmenu .rowmenu-item', (bs) => bs.map((b) => (b.disabled ? '(off) ' : '') + (b.getAttribute('aria-checked') === 'true' ? '[x] ' : '') + b.textContent.trim()));

test('each list sorts in its own order: by hand at the top, newest first in the folder', { skip }, async () => {
  const { browser, pg } = await open('');
  try {
    assert.deepEqual(await topNames(pg), ['Lore', 'Bryn', 'Cass', 'Ash']);
    assert.deepEqual(await folderNames(pg), ['Young', 'Old']);
    // A player gets no arrows.
    assert.equal(await pg.$('.tree-step-btn'), null);
  } finally { await browser.close(); }
});

test('an editor clicks an arrow again and again; the arrows stay with the moved page', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    // Only the hand-kept list gets arrows; the folder's own pages do not.
    assert.equal(await pg.$('.sidebar-tree-node[data-entity-id="e4"] .tree-step-btn'), null);
    const up = '.sidebar-tree-node[data-entity-id="e1"] > .tree-step > [data-step="-1"]';
    assert.equal(await pg.isDisabled('.sidebar-tree-node[data-entity-id="e2"] > .tree-step > [data-step="-1"]'), true, 'the first page cannot go up');
    // The folder is first, but a page never steps above it.
    await pg.click(up, { force: true });
    await pg.mouse.move(0, 0);
    assert.deepEqual(await topNames(pg), ['Lore', 'Bryn', 'Ash', 'Cass']);
    assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('aria-label')), 'Move Ash up');
    await pg.waitForTimeout(250); // the arrows' fade
    assert.equal(await pg.$eval(up, (b) => getComputedStyle(b.parentElement).opacity), '1', 'arrows stay shown on the moved row');
    await pg.click(up, { force: true });
    assert.deepEqual(await topNames(pg), ['Lore', 'Ash', 'Bryn', 'Cass']);
    // At the top the up arrow turns off and focus moves to the down arrow.
    assert.equal(await pg.isDisabled(up), true);
    assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('aria-label')), 'Move Ash down');
    await pg.waitForFunction(() => sent.length === 2);
    assert.deepEqual(await pg.evaluate(() => sent), [
      { url: '/campaigns/c1/sidebar-order/move', method: 'PUT', body: { kind: 'page', id: 'e1', next_to: 'e3', after: false } },
      { url: '/campaigns/c1/sidebar-order/move', method: 'PUT', body: { kind: 'page', id: 'e1', next_to: 'e2', after: false } },
    ]);
  } finally { await browser.close(); }
});

test('Alt+Down on a focused page moves it and keeps focus on it; a plain arrow does nothing', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.focus('.sidebar-tree-node[data-entity-id="e2"] > a');
    await pg.keyboard.press('ArrowDown');
    assert.deepEqual(await topNames(pg), ['Lore', 'Bryn', 'Cass', 'Ash']);
    await pg.keyboard.press('Alt+ArrowDown');
    assert.deepEqual(await topNames(pg), ['Lore', 'Cass', 'Bryn', 'Ash']);
    assert.equal(await pg.evaluate(() => document.activeElement.textContent.trim()), 'Bryn');
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent[0].body), { kind: 'page', id: 'e2', next_to: 'e3', after: true });
  } finally { await browser.close(); }
});

test('the row menu moves a page and picks how a folder orders its pages', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.click('.sidebar-tree-node[data-entity-id="e3"] > a', { button: 'right' });
    const labels = await menuLabels(pg);
    assert.deepEqual(labels.slice(-2), ['Move up', 'Move down']);
    await pg.locator('.rowmenu .rowmenu-item', { hasText: 'Move down' }).click();
    assert.deepEqual(await topNames(pg), ['Lore', 'Bryn', 'Ash', 'Cass']);

    await pg.click('.sidebar-tree-node[data-node-id="n1"] > .sidebar-tree-item', { button: 'right' });
    await pg.locator('.rowmenu .rowmenu-item', { hasText: 'Order inside' }).click();
    assert.deepEqual(await menuLabels(pg), ['Back', 'A to Z', 'By hand', '[x] Newest first']);
    await pg.locator('.rowmenu .rowmenu-item', { hasText: 'A to Z' }).click();
    await pg.waitForFunction(() => sent.length === 2);
    assert.deepEqual(await pg.evaluate(() => sent[1]), { url: '/campaigns/c1/sidebar-order', method: 'PUT', body: { kind: 'folder', id: 'n1', mode: 'az' } });
  } finally { await browser.close(); }
});

test('the category picker saves its order; a failed move says so and redraws the saved order', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    await pg.selectOption('[data-tree-order]', 'new');
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent[0].body), { kind: 'category', id: '7', mode: 'new' });

    await pg.waitForSelector('.sidebar-tree-node[data-entity-id="e1"]');
    await pg.evaluate(() => { window.failNext = true; });
    await pg.click('.sidebar-tree-node[data-entity-id="e3"] > .tree-step > [data-step="1"]', { force: true });
    await pg.waitForFunction(() => notes.length === 1);
    assert.match(await pg.evaluate(() => notes[0]), /^error: The move could not be saved/);
    await pg.waitForFunction(() => [...document.querySelectorAll('#sidebar-entity-tree > .sidebar-tree-node > .sidebar-tree-item .tree-lb')].map((l) => l.textContent).join() === 'Lore,Bryn,Cass,Ash');
  } finally { await browser.close(); }
});

test('dragging a page reorders a list kept by hand; dropping it on a folder files it there', { skip }, async () => {
  const { browser, pg } = await open(EDITOR);
  try {
    const rowOf = (id) => `.sidebar-tree-node[data-entity-id="${id}"] > a`;
    // Ash onto the lower half of Cass: Ash goes after Cass.
    const cass = await pg.locator(rowOf('e3')).boundingBox();
    await pg.locator(rowOf('e1')).dragTo(pg.locator(rowOf('e3')), { targetPosition: { x: 20, y: cass.height - 3 } });
    assert.deepEqual(await topNames(pg), ['Lore', 'Bryn', 'Cass', 'Ash']);
    await pg.waitForFunction(() => sent.length === 1);
    assert.deepEqual(await pg.evaluate(() => sent[0]), { url: '/campaigns/c1/sidebar-order/move', method: 'PUT', body: { kind: 'page', id: 'e1', next_to: 'e3', after: true } });

    // Bryn onto the folder: filed at the end of Lore.
    await pg.locator(rowOf('e2')).dragTo(pg.locator('.sidebar-tree-node[data-node-id="n1"] > .sidebar-tree-item'));
    await pg.waitForFunction(() => sent.length === 2);
    assert.deepEqual(await pg.evaluate(() => sent[1]), { url: '/campaigns/c1/entities/e2/reorder', method: 'PUT', body: { parent_id: null, parent_node_id: 'n1', sort_order: 1000000 } });
    assert.equal(await pg.$('.drop-before, .drop-after, .drop-into, .is-dragging'), null, 'no drop marks are left behind');
  } finally { await browser.close(); }
});

test('a player cannot drag rows, and a list kept A to Z takes no reorder drop', { skip }, async () => {
  const { browser, pg } = await open('');
  try {
    assert.equal(await pg.$('#sidebar-entity-tree [draggable="true"]'), null);
  } finally { await browser.close(); }
  const ed = await open(EDITOR, 'az');
  try {
    const rowOf = (id) => `.sidebar-tree-node[data-entity-id="${id}"] > a`;
    const before = await topNames(ed.pg);
    await ed.pg.locator(rowOf('e1')).dragTo(ed.pg.locator(rowOf('e3')));
    assert.deepEqual(await topNames(ed.pg), before);
    assert.deepEqual(await ed.pg.evaluate(() => sent), []);
  } finally { await ed.browser.close(); }
});
