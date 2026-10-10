// sidebar_row_menu.test.mjs — what a sidebar row's menu offers: the owner's
// pin, hide, move and Edit sidebar on their own rows, a member's own pin, and
// Open in new tab on rows that go somewhere. Moves stay inside the row's own
// section, so Move up never quietly pins a row.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const js = (f) => readFileSync(path.join(here, '..', '..', 'static', 'js', f), 'utf8');

/** Loads the editor and the menu on a page with no sidebar. */
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
  vm.runInContext(js('sidebar_editor.js'), sandbox);
  vm.runInContext(js('sidebar_row_menu.js'), sandbox);
  return { editor: window.Chronicle.navEditor, menu: window.Chronicle.navRowMenu };
}

function model() {
  return {
    sections: [
      { id: 'pinned', kind: 'pinned', label: 'Pinned', items: [
        { key: 'app:notes', kind: 'app', label: 'Journal' },
      ] },
      { id: 'apps', kind: 'apps', label: 'Apps', items: [
        { key: 'app:maps', kind: 'app', label: 'Maps' },
        { key: 'app:armory', kind: 'app', label: 'Armory', hidden: true },
      ] },
      { id: 'categories', kind: 'categories', label: 'Categories', items: [
        { key: 'cat:1', kind: 'category', label: 'Locations' },
        { key: 'cat:2', kind: 'category', label: 'Factions' },
      ] },
    ],
    off: [],
  };
}

const ids = (items) => JSON.parse(JSON.stringify(items)).map((it) => (it.sep ? '|' : it.id + (it.disabled ? '(off)' : '')));
const labels = (items) => JSON.parse(JSON.stringify(items)).filter((it) => !it.sep).map((it) => it.label);

test('the owner gets pin, hide, move and Edit sidebar on their rows', () => {
  const { editor, menu } = load();
  const cases = [
    { name: 'an app with a page', info: { key: 'app:armory', href: '/campaigns/c/armory' },
      want: ['open-tab', 'pin-all', 'show', '|', 'up', 'down(off)', '|', 'edit'] },
    { name: 'a category: no new tab, and the first one cannot move up out of its section', info: { key: 'cat:1', href: '' },
      want: ['pin-all', 'hide', '|', 'up(off)', 'down', '|', 'edit'] },
    { name: 'the first app cannot move up into Pinned', info: { key: 'app:maps', href: '' },
      want: ['pin-all', 'hide', '|', 'up(off)', 'down', '|', 'edit'] },
    { name: 'the last category cannot move down', info: { key: 'cat:2', href: '' },
      want: ['pin-all', 'hide', '|', 'up', 'down(off)', '|', 'edit'] },
    { name: 'a shown row offers to hide it', info: { key: 'app:maps', href: '/x' },
      want: ['open-tab', 'pin-all', 'hide', '|', 'up(off)', 'down', '|', 'edit'] },
    { name: 'a fixed row (Dashboard) has only a new tab and Edit sidebar', info: { key: 'dashboard', href: '/campaigns/c' },
      want: ['open-tab', '|', 'edit'] },
  ];
  for (const c of cases) {
    const got = menu.itemsFor({ ...c.info, model: model(), pin: null, editor });
    assert.deepEqual(ids(got), c.want, c.name);
  }
});

test('a pinned row reads Unpin; an unpinned one pins for everyone', () => {
  const { editor, menu } = load();
  assert.equal(labels(menu.itemsFor({ key: 'app:notes', href: '/n', model: model(), pin: null, editor }))[1], 'Unpin');
  assert.equal(labels(menu.itemsFor({ key: 'app:maps', href: '/m', model: model(), pin: null, editor }))[1], 'Pin to top for everyone');
});

test('a member gets their own pin and never the owner\'s actions', () => {
  const { editor, menu } = load();
  assert.deepEqual(ids(menu.itemsFor({ key: 'app:maps', href: '/m', model: null, pin: false, editor })), ['open-tab', 'pin-me']);
  assert.deepEqual(labels(menu.itemsFor({ key: 'cat:1', href: '', model: null, pin: true, editor })), ['Unpin']);
});

test('a row with nothing to offer opens no menu', () => {
  const { editor, menu } = load();
  assert.equal(menu.itemsFor({ key: 'cat:1', href: '', model: null, pin: null, editor }).length, 0);
});

test('working out Move does not change the arrangement', () => {
  const { editor, menu } = load();
  const m = model();
  const before = JSON.stringify(m);
  menu.itemsFor({ key: 'cat:1', href: '', model: m, pin: null, editor });
  assert.equal(JSON.stringify(m), before);
});
