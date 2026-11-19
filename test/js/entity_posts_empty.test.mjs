// entity_posts_empty.test.mjs — what the Posts block says on a page that has
// no posts: a writer is told what a post is for, a reader only that there are
// none.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/entity_posts.js'), 'utf8');

/** Mounts the widget on a stand-in element whose page has the given posts,
 *  and returns what it drew once the list has loaded. */
async function draw(config, posts) {
  let widget = null;
  const Chronicle = {
    register(name, w) { widget = w; },
    apiFetch() { return Promise.resolve({ status: 200, ok: true, json: () => Promise.resolve(posts) }); },
    escapeHtml: (s) => String(s),
    gmTerm: () => 'GM',
  };
  const noop = () => {};
  const document = { addEventListener: noop, removeEventListener: noop };
  vm.runInNewContext(src, { Chronicle, window: { Chronicle }, document, console, setTimeout });
  const el = { innerHTML: '', querySelector: () => null, querySelectorAll: () => [], addEventListener: noop };
  widget.init(el, { endpoint: '/campaigns/c1/entities/e1/posts', campaignId: 'c1', entityId: 'e1', ...config });
  for (let i = 0; i < 5; i++) await new Promise((r) => setTimeout(r, 0));
  return el.innerHTML.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
}

test('a writer is told what a post is for', async () => {
  const words = await draw({ editable: true }, []);
  assert.match(words, /No posts yet\. Add a post for anything that needs its own section on this page\./);
  assert.doesNotMatch(words, /sub-notes/i);
});

test('a reader is only told there are none', async () => {
  const words = await draw({ editable: false }, []);
  assert.match(words, /No posts yet\.$/);
  assert.doesNotMatch(words, /Add a post/);
});

test('a page with posts says neither', async () => {
  const words = await draw({ editable: true }, [{ id: 'p1', name: 'Rumours', isPrivate: false }]);
  assert.match(words, /Rumours/);
  assert.doesNotMatch(words, /No posts yet/);
});
