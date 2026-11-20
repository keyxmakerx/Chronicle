// search_page.test.mjs — the Search page's component:
// internal/plugins/entities/static/js/search_page.js.
//
// Pins what it asks of SearchAPI and how it draws the answer: two groups
// (named, then mention) asked separately, each result opening at its own
// same-site url, a page hidden from players carrying the lock and the amber
// edge (read from `restricted`), the quoted line drawn from marked runs with
// every run escaped, a group folding to five rows until "Show all", paging in
// the 20 the server answers with, and the pickers' labels. Runs the script in
// a vm against stand-in groups and apiFetch.

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
function load(answer, search = '') {
  const els = { 'sp-named': { innerHTML: '' }, 'sp-mention': { innerHTML: '' } };
  const urls = [];
  const Chronicle = {
    escapeAttr: esc,
    escapeHtml: esc,
    register: () => {},
    apiFetch: (url) => {
      urls.push(url);
      if (url.includes('/tags')) return Promise.resolve({ json: () => Promise.resolve([]) });
      return Promise.resolve({ json: () => Promise.resolve(answer(url)) });
    },
  };
  const window = { location: { search, pathname: '/campaigns/c1/search' } };
  const sandbox = {
    window, Chronicle, URLSearchParams, encodeURIComponent, clearTimeout, setTimeout,
    history: { replaceState: () => {} },
    document: { getElementById: (id) => els[id] || null },
  };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  const data = window.searchPageData('c1', true);
  data.$watch = () => {};
  return { data, els, urls };
}

const settle = () => new Promise((r) => setTimeout(r, 0));
const hrefs = (html) => [...html.matchAll(/href="([^"]*)"/g)].map((m) => m[1]);

function draw(items) {
  const { data, els } = load(() => ({}));
  data.groups.named.items = items;
  data.groups.named.open = true;
  data.renderGroup('named');
  return els['sp-named'].innerHTML;
}

test('a result opens at its own same-site url, and a page at the page route', () => {
  const html = draw([
    { id: 'p1', name: 'Harbour', type_name: 'Page' },
    { id: 'm1', name: 'Harbour chart', type_name: 'Map', url: '/campaigns/c1/maps/m1' },
    { id: 'x', name: 'Offsite', url: 'https://evil.example/' },
    { id: 'y', name: 'Protocol relative', url: '//evil.example/' },
    { id: 'z', name: 'Backslash', url: '/\\evil.example/' },
    { id: 'q"', name: 'Quote', url: '/campaigns/c1/maps/a"b' },
  ]);
  assert.deepEqual(hrefs(html), [
    '/campaigns/c1/entities/p1',
    '/campaigns/c1/maps/m1',
    '/campaigns/c1/entities/x',
    '/campaigns/c1/entities/y',
    '/campaigns/c1/entities/z',
    '/campaigns/c1/maps/a&quot;b',
  ]);
});

test('the lock and the amber edge mark a page hidden from players, read from restricted', () => {
  const rows = draw([
    { id: 'a', name: 'Hidden', restricted: 'dm_only' },
    { id: 'b', name: 'Open' },
    { id: 'c', name: 'Old flag', is_private: true },
  ]).split('<a ').slice(1);
  assert.match(rows[0], /fa-lock/);
  assert.match(rows[0], /data-dm-only/);
  assert.doesNotMatch(rows[1], /fa-lock|data-dm-only/);
  assert.doesNotMatch(rows[2], /fa-lock|fa-eye-slash|data-dm-only/, 'is_private is not in the answer and is not read');
});

test('the name comes first, the lock after it, the kind next, the picture last', () => {
  const row = draw([{ id: 'a', name: 'Captain Ilsa Varn', restricted: 'dm_only', type_name: 'Character', type_label: 'NPC', image: '/media/abc?s=1' }]);
  const at = (needle) => row.indexOf(needle);
  assert.ok(at('Captain Ilsa Varn') < at('fa-lock'), 'lock follows the name');
  assert.ok(at('fa-lock') < at('sp-kind'), 'kind comes after the lock');
  assert.ok(at('sp-kind') < at('<img'), 'picture is last');
  assert.match(row, /Character · NPC/);
});

test('a session is a game night, and only a same-site picture is drawn', () => {
  const row = draw([
    { id: 's', name: 'Salt night', type_name: 'Session' },
    { id: 'p', name: 'Offsite pic', type_name: 'Page', image: 'https://evil.example/x.png' },
  ]);
  assert.match(row, /Game night/);
  assert.doesNotMatch(row, /Session/);
  assert.doesNotMatch(row, /<img/, 'an off-site picture address is never drawn');
});

test('the quoted line is drawn from runs: marked ones wrapped, every run escaped', () => {
  const row = draw([{
    id: 'a', name: 'Ledger',
    snippet: [{ text: '…owes the ' }, { text: 'Tide', mark: true }, { text: 'wrights <script>x()</script> & more' }],
  }]);
  assert.match(row, /<mark class="sp-mark">Tide<\/mark>/);
  assert.match(row, /wrights &lt;script&gt;x\(\)&lt;\/script&gt; &amp; more/);
  assert.doesNotMatch(row, /<script>/);
  assert.equal((row.match(/<mark/g) || []).length, 1);
});

test('a name with markup is escaped', () => {
  assert.doesNotMatch(draw([{ id: 'a', name: '<img src=x onerror=1>' }]), /<img src=x/);
});

test('two groups are asked for, each with its own scope, and the mention group needs a word', async () => {
  const { data, urls } = load(() => ({ results: [], total: 0 }), '?q=tide');
  data.init();
  await settle(); await settle();
  const asked = urls.filter((u) => u.includes('/entities/search'));
  assert.equal(asked.length, 2);
  assert.ok(asked.some((u) => u.includes('scope=named') && u.includes('q=tide')));
  assert.ok(asked.some((u) => u.includes('scope=mention') && u.includes('q=tide')));
  assert.ok(asked.every((u) => !u.includes('per_page')), 'the server ignores per_page');

  // With only a category picked, there is nothing to mention.
  const filtered = load(() => ({ results: [], total: 0 }), '?type=3');
  filtered.data.init();
  await settle(); await settle();
  const f = filtered.urls.filter((u) => u.includes('/entities/search'));
  assert.equal(f.length, 1);
  assert.match(f[0], /scope=named/);
  assert.match(f[0], /type=3/);
  assert.doesNotMatch(f[0], /[?&]q=/);
});

test('nothing is asked for before there is a word or a filter', async () => {
  const { data, urls } = load(() => ({}));
  data.init();
  await settle();
  assert.equal(urls.filter((u) => u.includes('/entities/search')).length, 0);
  assert.equal(data.showEmpty(), false);
});

test('tags go to the server as slugs, joined, and the picker says what is picked', () => {
  const { data } = load(() => ({}));
  data.allTags = [{ slug: 'coast', name: 'Coast' }, { slug: 'debt', name: 'In debt' }];
  assert.equal(data.tagLabel(), 'Any tag');
  data.activeTags = ['coast'];
  assert.equal(data.tagLabel(), 'Coast');
  data.activeTags = ['coast', 'debt'];
  assert.equal(data.tagLabel(), '2 tags');
  data.query = 'tide';
  assert.match(data.groupUrl('named'), /tags=coast,debt/);
});

test('the category picker names what is picked', () => {
  const { data } = load(() => ({}));
  data.typeNames = { 3: 'Characters' };
  assert.equal(data.typeLabel(), 'All categories');
  data.typeFilter = '3';
  assert.equal(data.typeLabel(), 'Characters');
});

test('a long tag list gets a find, a short one is shown whole', () => {
  const { data } = load(() => ({}));
  data.allTags = Array.from({ length: 12 }, (_, i) => ({ slug: 't' + i, name: i === 4 ? 'Smugglers' : 'Tag ' + i }));
  data.tagFind = 'smug';
  assert.deepEqual(data.visibleTags().map((t) => t.slug), ['t4']);
  data.allTags = data.allTags.slice(0, 5);
  assert.equal(data.visibleTags().length, 5, 'a short list ignores the find');
});

test('a group shows five rows until Show all, which needs no new request', async () => {
  const items = Array.from({ length: 8 }, (_, i) => ({ id: 'p' + i, name: 'Page ' + i }));
  const { data, els, urls } = load(() => ({ results: items, total: 8, entity_total: 8 }), '?q=page');
  data.init();
  await settle(); await settle();
  assert.equal((els['sp-named'].innerHTML.match(/<a /g) || []).length, 5);
  assert.equal(data.moreFor('named'), true);
  const before = urls.length;
  data.showAll('named');
  assert.equal((els['sp-named'].innerHTML.match(/<a /g) || []).length, 8);
  assert.equal(data.moreFor('named'), false);
  assert.equal(urls.length, before);
});

test('a group with few rows has no Show all', async () => {
  const { data } = load(() => ({ results: [{ id: 'a', name: 'A' }], total: 1, entity_total: 1 }), '?q=aa');
  data.init();
  await settle(); await settle();
  assert.equal(data.moreFor('named'), false);
  assert.equal(data.moreFor('mention'), false);
});

test('paging counts in the 20 the server answers and steps through the pages only', async () => {
  const answers = {
    1: { results: [{ id: 'a', name: 'A' }], total: 23, entity_total: 21 },
    2: { results: [{ id: 'b', name: 'B' }], total: 21, entity_total: 21 },
  };
  const { data, urls } = load((url) => (url.includes('scope=mention') ? { results: [], total: 0 } : answers[/page=(\d+)/.exec(url)[1]]), '?q=harbour');
  data.init();
  await settle(); await settle();
  assert.equal(data.pagesOf('named'), 2, '21 pages need a second screen');
  assert.equal(data.groups.named.total, 23, 'the group count holds the maps and events');

  data.showAll('named');
  data.turn('named', 1);
  await settle(); await settle();
  assert.equal(data.groups.named.page, 2);
  assert.ok(urls.some((u) => u.includes('scope=named') && u.includes('page=2')));
  assert.equal(data.groups.named.total, 23, 'the count keeps the maps and events page 1 reported');
  data.turn('named', 1);
  assert.equal(data.groups.named.page, 2, 'there is no page 3');
});

test('twenty pages and some maps need no pager', async () => {
  const { data } = load(() => ({ results: [], total: 22, entity_total: 20 }), '?q=harbour');
  data.init();
  await settle(); await settle();
  assert.equal(data.pagesOf('named'), 1);
});

test('an answer to an older search is dropped', async () => {
  let release;
  const slow = new Promise((r) => { release = r; });
  const { data, els } = load((url) => (url.includes('q=old')
    ? slow.then(() => ({ results: [{ id: 'o', name: 'Old' }], total: 1, entity_total: 1 }))
    : { results: [{ id: 'n', name: 'New' }], total: 1, entity_total: 1 }));
  data.query = 'old';
  data.search();
  data.query = 'new';
  data.search();
  await settle(); await settle();
  release();
  await settle(); await settle();
  assert.match(els['sp-named'].innerHTML, /New/);
  assert.doesNotMatch(els['sp-named'].innerHTML, /Old/);
});

test('the count line says "1 result" for one, and "N results" otherwise', () => {
  const { data } = load(() => ({}));
  for (const [named, mention, want] of [[0, 0, '0 results'], [1, 0, '1 result'], [1, 1, '2 results'], [24, 77, '101 results']]) {
    data.groups.named.total = named;
    data.groups.mention.total = mention;
    assert.equal(data.resultsLabel(), want);
  }
});

test('nothing found says so and offers to make the page, only with a word', async () => {
  const { data } = load(() => ({ results: [], total: 0, entity_total: 0 }), '?q=tide');
  data.init();
  await settle(); await settle();
  assert.equal(data.showEmpty(), true);
  assert.match(data.emptyText(), /Nothing matches “tide”\. Try fewer or different words\./);
  assert.equal(data.createHref(), '/campaigns/c1/entities/new?name=tide');
});
