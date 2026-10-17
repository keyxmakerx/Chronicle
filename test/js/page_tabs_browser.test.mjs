// page_tabs_browser.test.mjs — the tabbed panel under a system-rendered page.
//
// Runs widgets/page_tabs.js in a real Chromium against the panel's markup
// (data attributes as page_tabs.templ writes them) and pieces that load late,
// as the real widgets and lazy loads do.
//
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

// The Notes tab's selectors, read from the template so the test fails if they
// drift from what entity_notes.js draws.
const TEMPL = readFileSync(join(root, 'internal/plugins/entities/page_tabs.templ'), 'utf8');
const NOTES_TAB = TEMPL.match(/Key: "notes".*Count: "([^"]+)", Act: `([^`]+)`/);
const [, NOTES_COUNT, NOTES_ACT] = NOTES_TAB;


function tab(key, label, count, act) {
  const sel = key === 'notes' ? 'aria-selected="true"' : 'aria-selected="false" tabindex="-1"';
  return `<button type="button" class="pt-tab" role="tab" id="pt-tab-${key}" ${sel} data-pt-tab="${key}" data-pt-count='${count}' data-pt-act='${act}'>${label} <span class="pt-n" data-pt-n></span></button>`;
}
const MARKUP = `<section class="pt" data-widget="page-tabs"><div class="pt-bar"><div class="pt-tabs" role="tablist">
${tab('notes', 'Notes', NOTES_COUNT, NOTES_ACT)}
${tab('items', 'Items', 'li[data-item-id]', '[data-open-give]')}
${tab('files', 'Files', '[data-pf-row]', '[data-pf-attach]')}
${tab('posts', 'Posts', '.card[data-post-id]', '[data-action="create-post"]')}
${tab('links', 'Links', '', '.rel-add-btn')}
</div><button type="button" class="pt-act" data-pt-press hidden><span data-pt-press-label></span></button></div>
<div class="pt-pane" data-pt-pane="notes"><div data-widget="entity-notes"></div></div>
<div class="pt-pane" data-pt-pane="items" hidden><div hx-get="/panel" hx-trigger="load" hx-swap="outerHTML"></div></div>
<div class="pt-pane" data-pt-pane="files" hidden><div hx-get="/files" hx-trigger="load" hx-swap="outerHTML"></div><p class="pt-empty" data-pt-empty hidden>No files yet.</p></div>
<div class="pt-pane" data-pt-pane="posts" hidden><div data-widget="entity-posts"></div></div>
<div class="pt-pane" data-pt-pane="links" hidden><div class="pt-links">
<div class="pt-grp" data-pt-grp=".rel-item"><h3>Relations</h3><p data-pt-none hidden>None yet</p><div data-widget="relations"></div></div>
<div class="pt-grp" data-pt-grp=".space-y-2 > a"><h3>Mentioned on</h3><p data-pt-none hidden>None</p><div hx-get="/backlinks" hx-trigger="load" hx-swap="innerHTML"></div></div>
</div></div></section>`;

// What each piece draws once its fetch lands, shaped as the real widgets draw it.
const PIECES = {
  notesEmpty: `document.querySelector('[data-widget="entity-notes"]').innerHTML = '<section class="en-block"><header class="en-head"><h3>Notes</h3><button type="button" data-en-act="new" onclick="this.dataset.pressed = 1">New note</button></header><div data-en-comp></div><div class="en-list" data-en-list><div class="en-empty">No notes yet.</div></div></section>'`,
  notes: `document.querySelector('[data-en-list]').innerHTML = '<article class="en-note" data-en-id="n1">Owes the smith</article><article class="en-note" data-en-id="n2">Hates boats</article><article class="en-note is-leaving" data-en-id="n3">Deleted</article>'`,
  items: `(function () { var m = document.querySelector('[data-pt-pane="items"] [hx-trigger="load"]'); m.dispatchEvent(new CustomEvent('htmx:afterRequest', { bubbles: true })); m.outerHTML = '<section id="armory-panel-e1"><button data-open-give>Give an item</button><ul data-items><li data-item-id="1">Rope</li><li data-item-id="2">Torch</li></ul></section>'; })()`,
  itemsNone: `(function () { var m = document.querySelector('[data-pt-pane="items"] [hx-trigger="load"]'); m.dispatchEvent(new CustomEvent('htmx:afterRequest', { bubbles: true })); m.outerHTML = ''; })()`,
  files204: `document.querySelector('[data-pt-pane="files"] [hx-trigger="load"]').dispatchEvent(new CustomEvent('htmx:afterRequest', { bubbles: true }))`,
  posts: `document.querySelector('[data-widget="entity-posts"]').innerHTML = '<div class="flex"><h3>Posts</h3><button data-action="create-post">Add Post</button></div><div class="card" data-post-id="p1">Backstory</div>'`,
  links: `document.querySelector('[data-widget="relations"]').innerHTML = '<div class="rel-item">Ally: Bren</div><button class="rel-add-btn">Add Relation</button>'; document.querySelector('[data-pt-pane="links"] [hx-trigger="load"]').dispatchEvent(new CustomEvent('htmx:afterRequest', { bubbles: true }))`,
};

async function page(t) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  t.after(() => browser.close());
  const p = await browser.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  t.after(() => assert.deepEqual(errors, []));
  await p.setContent('<!doctype html><html><head><style>[hidden]{display:none}</style></head><body>' + MARKUP + '</body></html>');
  await p.addScriptTag({ content: 'window.Chronicle = { register: function (n, impl) { impl.init(document.querySelector("[data-widget=" + n + "]")); } };' });
  await p.addScriptTag({ content: readFileSync(join(root, 'static/js/widgets/page_tabs.js'), 'utf8') });
  return p;
}
async function load(p, ...names) {
  for (const n of names) await p.evaluate(PIECES[n]);
  await p.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
}
const state = (p) => p.evaluate(() => {
  const tabs = [...document.querySelectorAll('[data-pt-tab]')];
  return {
    selected: tabs.find((x) => x.getAttribute('aria-selected') === 'true').dataset.ptTab,
    open: [...document.querySelectorAll('[data-pt-pane]')].filter((x) => !x.hidden).map((x) => x.dataset.ptPane),
    counts: Object.fromEntries(tabs.map((x) => [x.dataset.ptTab, x.hidden ? 'hidden' : x.querySelector('[data-pt-n]').textContent])),
    press: document.querySelector('[data-pt-press]').hidden ? null : document.querySelector('[data-pt-press-label]').textContent,
  };
});

test('opens on the first tab with something in it, as the pieces load', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty', 'files204', 'items', 'posts', 'links');
  const s = await state(p);
  assert.equal(s.selected, 'items');
  assert.deepEqual(s.open, ['items']);
  assert.deepEqual(s.counts, { notes: '0', items: '2', files: '0', posts: '1', links: '1' });
  assert.equal(s.press, 'Give an item');
});

test('a tab the viewer picked holds while later pieces load', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty');
  await p.click('#pt-tab-posts');
  await load(p, 'items', 'posts');
  assert.equal((await state(p)).selected, 'posts');
});

test('the action button presses the piece\'s own button and wears its label', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty');
  assert.equal((await state(p)).press, 'New note');
  await p.click('[data-pt-press]');
  assert.equal(await p.getAttribute('[data-en-act="new"]', 'data-pressed'), '1');
  await p.click('#pt-tab-links');
  await load(p, 'links');
  assert.equal((await state(p)).press, 'Add Relation');
});

test('an armory with no panel hides Items; files with none say so', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty', 'itemsNone', 'files204');
  assert.equal((await state(p)).counts.items, 'hidden');
  await p.click('#pt-tab-files');
  assert.equal(await p.isVisible('[data-pt-pane="files"] [data-pt-empty]'), true);
});

test('arrow keys move between visible tabs', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty', 'itemsNone');
  await p.focus('#pt-tab-notes');
  await p.keyboard.press('ArrowRight');
  assert.equal((await state(p)).selected, 'files');
  await p.keyboard.press('End');
  assert.equal((await state(p)).selected, 'links');
});

test('the Notes tab counts the notes widget\'s rows, not one leaving', { skip }, async (t) => {
  const p = await page(t);
  await load(p, 'notesEmpty', 'notes', 'itemsNone', 'files204');
  const s = await state(p);
  assert.equal(s.counts.notes, '2');
  assert.equal(s.selected, 'notes');
});

test('the Notes tab selectors match what entity_notes.js draws', () => {
  const js = readFileSync(join(root, 'static/js/widgets/entity_notes.js'), 'utf8');
  assert.match(js, /row\.className = 'en-note';\s*row\.setAttribute\('data-en-id'/);
  assert.match(js, /data-en-act="new"/);
  assert.equal(NOTES_ACT, '[data-en-act="new"]');
});
