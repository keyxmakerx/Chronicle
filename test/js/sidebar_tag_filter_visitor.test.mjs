// sidebar_tag_filter_visitor.test.mjs — the sidebar's saved filters are a
// member's: the page must not ask for them as a visitor, whose request the
// server refuses.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_tag_filter.js'), 'utf8');

/** Runs the script on a category page and returns the URLs it fetched. */
async function fetched(member) {
  const urls = [];
  const handlers = {};
  const searchDiv = { querySelector: () => null };
  const document = {
    addEventListener: (type, fn) => { handlers[type] = fn; },
    getElementById: () => null,
    querySelector: () => searchDiv,
    querySelectorAll: () => [],
    documentElement: { hasAttribute: (n) => member && n === 'data-campaign-member' },
  };
  const Chronicle = {
    apiFetch: (url) => {
      urls.push(url);
      return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
    },
  };
  const window = { addEventListener() {}, location: { pathname: '/campaigns/c1/npc' } };
  vm.runInContext(src, vm.createContext({ window, document, Chronicle }));
  handlers['htmx:afterSettle']({ detail: { target: { id: 'sidebar-cat-content' } } });
  await new Promise((r) => setTimeout(r, 0));
  return urls;
}

test('a visitor does not request saved filters', async () => {
  const urls = await fetched(false);
  assert.deepEqual(urls, ['/campaigns/c1/tags']);
});

test('a member still requests saved filters', async () => {
  const urls = await fetched(true);
  assert.ok(urls.includes('/campaigns/c1/saved-filters'));
});
