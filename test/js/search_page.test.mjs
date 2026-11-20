// search_page.test.mjs — the Search page's component:
// internal/plugins/entities/static/js/search_page.js.
//
// Pins what it reads from SearchAPI: each result opens at its own same-site
// url, a page hidden from players carries the lock (the answer's
// `restricted`, not `is_private`), and paging counts in the 20 the server
// answers with, stepping through the pages' own count (entity_total) rather
// than a total that also holds the unpaged maps and events. Runs the script
// in a vm against a stand-in grid and apiFetch.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'internal', 'plugins', 'entities', 'static', 'js', 'search_page.js'), 'utf8');

const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

/** Loads the script; apiFetch answers each URL with answer(url). */
function load(answer) {
  const grid = { innerHTML: '' };
  const urls = [];
  const Chronicle = {
    escapeAttr: esc,
    escapeHtml: esc,
    register: () => {},
    apiFetch: (url) => { urls.push(url); return Promise.resolve({ json: () => Promise.resolve(answer(url)) }); },
  };
  const window = { location: { search: '', pathname: '/campaigns/c1/search' } };
  const sandbox = {
    window, Chronicle, URLSearchParams, encodeURIComponent, clearTimeout, setTimeout,
    history: { replaceState: () => {} },
    document: { getElementById: (id) => (id === 'search-results-grid' ? grid : null) },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const data = window.searchPageData('c1');
  return { data, grid, urls };
}

const hrefs = (html) => [...html.matchAll(/href="([^"]*)"/g)].map((m) => m[1]);

test('a result opens at its own same-site url, and a page at the page route', () => {
  const { data, grid } = load(() => ({}));
  data.results = [
    { id: 'p1', name: 'Harbour', type_name: 'Page' },
    { id: 'm1', name: 'Harbour chart', type_name: 'Map', url: '/campaigns/c1/maps/m1' },
    { id: 'x', name: 'Offsite', url: 'https://evil.example/' },
    { id: 'y', name: 'Protocol relative', url: '//evil.example/' },
    { id: 'z', name: 'Backslash', url: '/\\evil.example/' },
    { id: 'q"', name: 'Quote', url: '/campaigns/c1/maps/a"b' },
  ];
  data.renderGrid();
  assert.deepEqual(hrefs(grid.innerHTML), [
    '/campaigns/c1/entities/p1',
    '/campaigns/c1/maps/m1',
    '/campaigns/c1/entities/x',
    '/campaigns/c1/entities/y',
    '/campaigns/c1/entities/z',
    '/campaigns/c1/maps/a&quot;b',
  ]);
});

test('the lock marks a page hidden from players, read from restricted', () => {
  const { data, grid } = load(() => ({}));
  data.results = [
    { id: 'a', name: 'Hidden', restricted: 'dm_only' },
    { id: 'b', name: 'Open' },
    { id: 'c', name: 'Old flag', is_private: true },
  ];
  data.renderGrid();
  const cards = grid.innerHTML.split('<a ').slice(1);
  assert.match(cards[0], /fa-lock/);
  assert.doesNotMatch(cards[1], /fa-lock/);
  assert.doesNotMatch(cards[2], /fa-lock|fa-eye-slash/, 'is_private is not in the answer and is not read');
});

test('paging counts in the 20 the server answers and steps through the pages only', async () => {
  const answers = {
    1: { results: [{ id: 'a', name: 'A' }], total: 23, entity_total: 21 },
    2: { results: [{ id: 'b', name: 'B' }], total: 21, entity_total: 21 },
  };
  const { data, urls } = load((url) => answers[/page=(\d+)/.exec(url)[1]]);
  await data.search();
  await new Promise((r) => setImmediate(r));
  assert.ok(!urls[0].includes('per_page'), 'the server ignores per_page, so none is asked for');
  assert.equal(data.hasPager(), true, '21 pages need a second screen');
  assert.equal(data.hasNext(), true);
  assert.equal(data.total, 23);

  data.nextPage();
  await new Promise((r) => setImmediate(r));
  assert.equal(data.page, 2);
  assert.equal(data.hasNext(), false, 'page 2 holds the 21st page');
  assert.equal(data.total, 23, 'the count line keeps the maps and events page 1 reported');
});

test('twenty pages and some maps need no pager', async () => {
  const { data } = load(() => ({ results: [], total: 22, entity_total: 20 }));
  await data.search();
  await new Promise((r) => setImmediate(r));
  assert.equal(data.hasPager(), false);
  assert.equal(data.hasNext(), false);
});
