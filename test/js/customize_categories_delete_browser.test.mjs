// customize_categories_delete_browser.test.mjs — each category card in
// Customize › Categories has a ⋯ menu beside it (never inside the card's
// link) that deletes the category into the draft. An empty category asks only
// for a confirm; one with pages names where they move (move_to), offering
// only places that keep DM-only pages hidden; one with kinds says why it
// can't go yet.
//
// Runs customize_categories.js in a real Chromium with a stand-in for the
// Customizer frame. Needs playwright-core and a Chromium (see
// _browser_env.mjs); skips locally, and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const CATS = [
  { id: 1, name: 'Place', namePlural: 'Places', icon: 'fa-map-pin', color: '#10b981', pages: 12 },
  { id: 2, name: 'Town', namePlural: 'Towns', icon: 'fa-city', color: '#10b981', parentId: 1, pages: 3 },
  { id: 3, name: 'Lore', namePlural: 'Lore', icon: 'fa-book', color: '#0ea5e9', pages: 29 },
  { id: 4, name: 'Adventure', namePlural: 'Adventures', icon: 'fa-dungeon', color: '#b45309', pages: 11, dmOnly: true },
  { id: 5, name: 'Adventure', namePlural: 'Adventures', icon: 'fa-box', color: '#6366f1', pages: 0 },
  { id: 6, name: 'Rumour', namePlural: 'Rumours', icon: 'fa-scroll', color: '#ec4899', pages: 2, trashed: 1 },
];

function page(draft) {
  const state = JSON.stringify({ base: '/c/x/customize', categories: CATS, inMenu: {} }).replace(/'/g, '&#39;');
  return `<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="/customize_shell.css"></head><body>
<div data-widget="customize-categories" data-state='${state}'></div>
<script>
  var draft = ${JSON.stringify(draft || {})};
  window.sets = [];
  window.Chronicle = {
    escapeHtml: function (s) { return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;'); },
    gmTerm: function () { return 'Director'; },
    customizeDraft: function () { return draft; },
    customizeSet: function (k, v) {
      if (v == null) delete draft[k]; else draft[k] = v;
      window.sets.push(JSON.parse(JSON.stringify(v)));
      document.dispatchEvent(new CustomEvent('chronicle:customize-draft', { detail: {} }));
    },
    register: function (_n, w) { w.init(document.querySelector('[data-widget]')); },
  };
</script>
<script src="/customize_categories.js"></script>
</body></html>`;
}

async function open(draft) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const pg = await browser.newPage();
  await pg.route('http://t.test/**', (route) => {
    const p = new URL(route.request().url()).pathname;
    if (p === '/customize_categories.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/widgets/customize_categories.js')) });
    if (p === '/customize_shell.css') return route.fulfill({ contentType: 'text/css', body: readFileSync(join(root, 'static/css/customize_shell.css')) });
    return route.fulfill({ contentType: 'text/html', body: page(draft) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('.czs-cat-cell');
  return { browser, pg };
}

// moreFor opens the ⋯ menu of the nth card with that name.
async function moreFor(pg, name, nth = 0) {
  await pg.getByRole('button', { name: 'More for ' + name }).nth(nth).click();
  await pg.waitForSelector('.czs-menu');
}
const lastSet = (pg) => pg.evaluate(() => window.sets.at(-1));

test('every card has a ⋯ button beside its link, never inside it', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    const cells = await pg.$$eval('.czs-cat-cell', (els) => els.map((c) => ({
      inLink: !!c.querySelector('a button'), more: !!c.querySelector(':scope > button[data-more][aria-haspopup="menu"]'),
    })));
    assert.ok(cells.length >= 5);
    for (const c of cells) {
      assert.equal(c.inLink, false, 'no button inside the card link');
      assert.equal(c.more, true, 'a ⋯ sits beside the link');
    }
  } finally { await browser.close(); }
});

test('an empty category deletes with one confirm and Keep it brings it back', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    await moreFor(pg, 'Adventures', 1);
    assert.equal(await pg.getByRole('button', { name: 'More for Adventures' }).nth(1).getAttribute('aria-expanded'), 'true');
    await pg.getByRole('menuitem', { name: 'Delete…' }).click();
    const dlg = pg.getByRole('alertdialog');
    await dlg.waitFor();
    assert.match(await dlg.innerText(), /It has no pages, so nothing else is lost/);
    assert.equal(await dlg.locator('select').count(), 0, 'no page move to choose');
    await dlg.getByRole('button', { name: 'Delete category' }).click();
    const v = await lastSet(pg);
    assert.deepEqual(v.items, { 5: { delete: true } });
    assert.equal(v._changes[0].label, 'Delete Adventures');
    assert.match(await pg.locator('.czs-cat-cell').filter({ hasText: '0 pages' }).innerText(), /Deleted when you save/);

    await moreFor(pg, 'Adventures', 1);
    await pg.getByRole('menuitem', { name: 'Keep it' }).click();
    assert.equal(await lastSet(pg), null, 'the draft section is empty again');
  } finally { await browser.close(); }
});

test('a category with pages names where they move, and the draft carries move_to', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    await moreFor(pg, 'Rumours');
    await pg.getByRole('menuitem', { name: 'Delete…' }).click();
    const dlg = pg.getByRole('alertdialog');
    assert.match(await dlg.innerText(), /Its 3 pages \(1 of them in the Trash\) move to another category first, so no page is deleted/);
    const options = await dlg.locator('select option').allInnerTexts();
    // Itself is not offered; a kind is named under its parent; the DM-only
    // category is a fine place for visible pages.
    assert.deepEqual(options, ['Places', 'Places › Towns', 'Lore', 'Adventures (only the Director team sees it)', 'Adventures']);
    await dlg.locator('select').selectOption('3');
    await dlg.getByRole('button', { name: 'Move pages and delete' }).click();
    const v = await lastSet(pg);
    assert.deepEqual(v.items, { 6: { delete: true, move_to: '3' } });
    assert.equal(v._changes[0].label, 'Delete Rumours, its pages move to Lore');
    assert.match(await pg.locator('.czs-cat-cell').last().innerText(), /Pages move to Lore/);
  } finally { await browser.close(); }
});

test('DM-only pages are offered only another DM-only category, so none is offered here', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    await moreFor(pg, 'Adventures', 0);
    await pg.getByRole('menuitem', { name: 'Delete…' }).click();
    const dlg = pg.getByRole('alertdialog');
    assert.match(await dlg.innerText(), /can’t be deleted yet/);
    assert.match(await dlg.innerText(), /for the Director team only/);
    assert.equal(await dlg.getByRole('button', { name: /Delete/ }).count(), 0);
    await dlg.getByRole('button', { name: 'Close' }).click();
    assert.equal(await pg.evaluate(() => window.sets.length), 0, 'nothing went into the draft');
  } finally { await browser.close(); }
});

test('a category with kinds says to delete them first', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    await moreFor(pg, 'Places');
    await pg.getByRole('menuitem', { name: 'Delete…' }).click();
    const dlg = pg.getByRole('alertdialog');
    assert.match(await dlg.innerText(), /Delete its kinds first: Towns/);
    assert.equal(await dlg.locator('[data-ok]').count(), 0);
  } finally { await browser.close(); }
});

test('the menu works from the keyboard and Escape returns to its button', { skip }, async () => {
  const { browser, pg } = await open();
  try {
    await pg.getByRole('button', { name: 'More for Lore' }).focus();
    await pg.keyboard.press('Enter');
    await pg.waitForSelector('.czs-menu');
    assert.equal(await pg.evaluate(() => document.activeElement.textContent), 'Settings');
    await pg.keyboard.press('ArrowDown');
    assert.equal(await pg.evaluate(() => document.activeElement.textContent), 'Delete…');
    await pg.keyboard.press('Escape');
    assert.equal(await pg.locator('.czs-menu').count(), 0);
    assert.equal(await pg.evaluate(() => document.activeElement.getAttribute('aria-label')), 'More for Lore');
  } finally { await browser.close(); }
});
