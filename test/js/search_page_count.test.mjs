// search_page_count.test.mjs — the campaign search page's count line: one
// page found is "1 result", any other number is "N results".

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'internal/plugins/entities/static/js/search_page.js'), 'utf8');

/** Loads search_page.js and returns a fresh page component, as Alpine would
 *  make it from x-data="searchPageData('c1')". */
function component() {
  const window = { location: { search: '' } };
  const Chronicle = { register() {}, apiFetch() { return Promise.resolve({ json: () => ({}) }); } };
  vm.runInNewContext(src, { window, Chronicle, URLSearchParams, document: { getElementById: () => null } });
  return window.searchPageData('c1');
}

test('the count line says "1 result" for one, and "N results" otherwise', () => {
  const page = component();
  for (const [total, want] of [[0, '0 results'], [1, '1 result'], [2, '2 results'], [24, '24 results'], [101, '101 results']]) {
    page.total = total;
    assert.equal(page.resultsLabel(), want);
  }
});

test('the count follows what the search returned', async () => {
  const window = { location: { search: '?q=keep', pathname: '/campaigns/c1/search' } };
  const Chronicle = {
    register() {},
    apiFetch(url) {
      if (url.includes('/tags')) return Promise.resolve({ json: () => [] });
      return Promise.resolve({ json: () => ({ results: [{ id: 'e1', name: 'Tidewatch Keep' }], total: 1 }) });
    },
    escapeHtml: (s) => String(s), escapeAttr: (s) => String(s),
  };
  const grid = { innerHTML: '' };
  vm.runInNewContext(src, { window, Chronicle, URLSearchParams, document: { getElementById: () => grid }, history: { replaceState() {} }, setTimeout, clearTimeout });
  const page = window.searchPageData('c1');
  page.$watch = () => {};
  page.init();
  await new Promise((r) => setTimeout(r, 0));
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(page.total, 1);
  assert.equal(page.resultsLabel(), '1 result');
  assert.match(grid.innerHTML, /Tidewatch Keep/);
});
