// sidebar_editor.test.mjs — contracts for the owner's sidebar editor's
// draft: which section may hold which row, how a row moves by the keyboard
// and by its pin, what Save sends (PUT /campaigns/:id/sidebar-config, read by
// campaigns.validateSidebarItems and NormalizeNav), and what the "What
// players see" card leaves out.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_editor.js'), 'utf8');

/** Loads sidebar_editor.js on a page with no sidebar; returns its draft API. */
function load() {
  const noop = () => {};
  const document = {
    addEventListener: noop,
    getElementById: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    documentElement: { classList: { contains: () => false } },
    body: { classList: { contains: () => false, toggle: noop } },
  };
  const window = { addEventListener: noop, matchMedia: () => ({ matches: false }) };
  const sandbox = { window, document, JSON };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return window.Chronicle.navEditor;
}

const plain = (v) => JSON.parse(JSON.stringify(v));

/** A campaign's arrangement as the server hands it to the owner. */
function model() {
  return {
    sections: [
      { id: 'pinned', kind: 'pinned', label: 'Pinned', items: [
        { key: 'app:notes', kind: 'app', label: 'Journal', icon: 'fa-book-open' },
      ] },
      { id: 'apps', kind: 'apps', label: 'Apps', items: [
        { key: 'app:maps', kind: 'app', label: 'Maps', icon: 'fa-map' },
        { key: 'app:timeline', kind: 'app', label: 'Timeline', icon: 'fa-timeline', off: true },
        { key: 'app:armory', kind: 'app', label: 'Armory', icon: 'fa-shield-halved', hidden: true },
        { key: 'link:lnk_1', kind: 'link', label: ' Wiki ', url: ' https://example.com ', icon: 'fa-globe' },
      ] },
      { id: 'categories', kind: 'categories', label: 'Categories', items: [
        { key: 'cat:1', kind: 'category', label: 'Locations', color: '#f87171' },
        { key: 'cat:2', kind: 'category', label: 'Factions' },
      ] },
      { id: 'sec_prep', kind: 'custom', label: 'Prep', items: [
        { key: 'cat:3', kind: 'category', label: 'Villains' },
      ] },
    ],
    off: [{ key: 'app:timeline', kind: 'app', label: 'Timeline', off: true }],
  };
}

const keys = (d, id) => d.sections.find((s) => s.id === id).items.map((r) => r.key);
const shown = (d, id) => d.sections.find((s) => s.id === id).items.filter((r) => !r.off).map((r) => r.key);

test('a section holds only the rows it may', () => {
  const ed = load();
  assert.equal(ed.accepts('apps', 'app'), true);
  assert.equal(ed.accepts('apps', 'link'), true);
  assert.equal(ed.accepts('apps', 'category'), false);
  assert.equal(ed.accepts('categories', 'category'), true);
  assert.equal(ed.accepts('categories', 'app'), false);
  assert.equal(ed.accepts('pinned', 'category'), true);
  assert.equal(ed.accepts('custom', 'link'), true);
  assert.equal(ed.homeFor('category'), 'categories');
  assert.equal(ed.homeFor('link'), 'apps');
});

test('an arrow key moves a row one shown place, stepping over turned-off apps', () => {
  const ed = load();
  const d = model();
  assert.equal(ed.stepRow(d, 'app:armory', -1).id, 'apps');
  // Timeline is off: Armory passes Maps, and Timeline keeps its place after Maps.
  assert.deepEqual(keys(d, 'apps'), ['app:armory', 'app:maps', 'app:timeline', 'link:lnk_1']);
  // Back down: the arrangement is exactly as it was.
  ed.stepRow(d, 'app:armory', 1);
  assert.deepEqual(keys(d, 'apps'), ['app:maps', 'app:timeline', 'app:armory', 'link:lnk_1']);
});

test('at either end a row crosses into the next section that can hold it', () => {
  const ed = load();
  const d = model();
  assert.equal(ed.stepRow(d, 'app:maps', -1).id, 'pinned');
  assert.deepEqual(keys(d, 'pinned'), ['app:notes', 'app:maps']);
  assert.equal(ed.stepRow(d, 'app:maps', 1).id, 'apps');
  assert.equal(shown(d, 'apps')[0], 'app:maps');
  // A category skips Apps, which cannot hold it.
  assert.equal(ed.stepRow(d, 'cat:1', -1).id, 'pinned');
  assert.equal(ed.stepRow(d, 'cat:2', 1).id, 'sec_prep');
  assert.deepEqual(keys(d, 'sec_prep'), ['cat:2', 'cat:3']);
  // The last row of the last section has nowhere to go.
  assert.equal(ed.stepRow(d, 'cat:3', 1), null);
});

test('a pin moves a row to the end of Pinned, and back to the top of its home', () => {
  const ed = load();
  const d = model();
  assert.equal(ed.togglePin(d, 'cat:3').id, 'pinned');
  assert.deepEqual(keys(d, 'pinned'), ['app:notes', 'cat:3']);
  assert.equal(ed.togglePin(d, 'cat:3').id, 'categories');
  assert.equal(keys(d, 'categories')[0], 'cat:3');
  assert.equal(ed.togglePin(d, 'app:notes').id, 'apps');
  assert.equal(keys(d, 'apps')[0], 'app:notes');
});

test('a dropped row lands before the shown row at its place', () => {
  const ed = load();
  const d = model();
  assert.equal(ed.moveRowTo(d, 'link:lnk_1', 'apps', 0), true);
  assert.deepEqual(keys(d, 'apps'), ['link:lnk_1', 'app:maps', 'app:timeline', 'app:armory']);
  assert.equal(ed.moveRowTo(d, 'cat:1', 'apps', 0), false, 'Apps cannot hold a category');
  assert.equal(ed.moveRowTo(d, 'cat:1', 'sec_prep', 5), true);
  assert.deepEqual(keys(d, 'sec_prep'), ['cat:3', 'cat:1']);
});

test('removing a section sends its rows home', () => {
  const ed = load();
  const d = model();
  ed.moveRowTo(d, 'link:lnk_1', 'sec_prep', 0);
  assert.equal(ed.removeSection(d, 'sec_prep'), true);
  assert.equal(d.sections.some((s) => s.id === 'sec_prep'), false);
  assert.equal(keys(d, 'apps').at(-1), 'link:lnk_1');
  assert.equal(keys(d, 'categories').at(-1), 'cat:3');
  assert.equal(ed.removeSection(d, 'apps'), false, 'built-in sections stay');
});

test('Save writes each section ahead of its rows, and every row names its section', () => {
  const ed = load();
  const items = plain(ed.itemsFromDraft(model()));
  assert.deepEqual(items, [
    { type: 'app', slug: 'notes', visible: true, section: 'pinned' },
    { type: 'app', slug: 'maps', visible: true, section: 'apps' },
    { type: 'app', slug: 'timeline', visible: true, section: 'apps' },
    { type: 'app', slug: 'armory', visible: false, section: 'apps' },
    { type: 'link', id: 'lnk_1', label: 'Wiki', url: 'https://example.com', icon: 'fa-globe', visible: true, section: 'apps' },
    { type: 'category', type_id: 1, visible: true, section: 'categories' },
    { type: 'category', type_id: 2, visible: true, section: 'categories' },
    { type: 'section', id: 'sec_prep', label: 'Prep', visible: true },
    { type: 'category', type_id: 3, visible: true, section: 'sec_prep' },
  ]);
});

test('an unnamed section is saved as Untitled', () => {
  const ed = load();
  const d = model();
  const sec = ed.addSection(d);
  sec.label = '   ';
  const item = plain(ed.itemsFromDraft(d)).find((i) => i.type === 'section' && i.id === sec.id);
  assert.equal(item.label, 'Untitled');
  assert.match(sec.id, /^sec_[a-z0-9]+$/);
});

test('a new link needs a name and an address before Save', () => {
  const ed = load();
  const d = model();
  assert.equal(ed.incompleteLink(d), null);
  const row = ed.addLink(d);
  assert.equal(keys(d, 'apps').at(-1), row.key);
  assert.equal(ed.incompleteLink(d).key, row.key);
  row.label = 'Rules';
  row.url = '/campaigns/c1/entities';
  assert.equal(ed.incompleteLink(d), null);
});

test('players see neither hidden rows nor turned-off apps, nor empty sections', () => {
  const ed = load();
  const d = model();
  d.sections.find((s) => s.id === 'sec_prep').items[0].hidden = true;
  const view = plain(ed.playerView(d));
  assert.deepEqual(view.map((s) => s.id), ['pinned', 'apps', 'categories']);
  assert.deepEqual(view[1].rows.map((r) => r.key), ['app:maps', 'link:lnk_1']);
});

test('the editor starts only from a whole arrangement', () => {
  const ed = load();
  const navEl = (raw) => ({ getAttribute: () => raw });
  assert.equal(ed.readModel(navEl(null)), null);
  assert.equal(ed.readModel(navEl('{nope')), null);
  assert.equal(ed.readModel(navEl(JSON.stringify({ sections: [{ id: 'apps', items: [] }] }))), null);
  const m = ed.readModel(navEl(JSON.stringify({ sections: [
    { id: 'pinned', kind: 'pinned', label: 'Pinned' },
    { id: 'apps', kind: 'apps', label: 'Apps', items: [] },
    { id: 'categories', kind: 'categories', label: 'Categories', items: [] },
  ] })));
  assert.deepEqual(plain(m.sections[0].items), []);
  assert.deepEqual(plain(m.off), []);
});
