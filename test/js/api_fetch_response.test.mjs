// api_fetch_response.test.mjs — Chronicle.apiFetch resolves to the raw fetch
// Response, so a widget must read its JSON before using it. The groups and
// entity-posts widgets once used the Response as data, and their lists never
// filled. These tests boot each widget against a stubbed apiFetch that, like
// the real one, resolves to a Response, and check the loaded rows render.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const widget = (name) => readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', name), 'utf8');

function response(status, data) {
  return { ok: status >= 200 && status < 300, status, json: () => Promise.resolve(data) };
}

// boot mounts one widget with apiFetch answering `answer(url, opts)`, and
// returns the element plus every request made.
async function boot(file, slug, config, answer) {
  const calls = [];
  let impl;
  const el = { innerHTML: '', querySelector: () => null, querySelectorAll: () => [], addEventListener() {} };
  const Chronicle = {
    register: (name, i) => { if (name === slug) impl = i; },
    apiFetch: (url, opts) => { calls.push({ url, opts: opts || {} }); return Promise.resolve(answer(url, opts || {})); },
    escapeHtml: (s) => String(s == null ? '' : s),
    escapeAttr: (s) => String(s == null ? '' : s),
    notify() {},
  };
  const sandbox = { Chronicle, console, document: { addEventListener() {}, querySelectorAll: () => [] }, window: {} };
  vm.runInNewContext(widget(file), sandbox);
  impl.init(el, config);
  await new Promise((r) => setTimeout(r, 0));
  return { el, calls };
}

test('groups widget lists the groups the API returns', async () => {
  const { el } = await boot('groups.js', 'groups', { groupsEndpoint: '/campaigns/c1/groups', membersJson: '[]' },
    () => response(200, { groups: [{ id: 1, name: 'Party A', members: [] }] }));
  assert.match(el.innerHTML, /Party A/);
  assert.doesNotMatch(el.innerHTML, /No groups yet/);
});

test('groups widget shows the server message when the load fails', async () => {
  const { el } = await boot('groups.js', 'groups', { groupsEndpoint: '/campaigns/c1/groups', membersJson: '[]' },
    () => response(403, { message: 'insufficient permissions' }));
  assert.match(el.innerHTML, /insufficient permissions/);
});

test('entity-posts widget lists the posts the API returns', async () => {
  const { el } = await boot('entity_posts.js', 'entity-posts', { endpoint: '/campaigns/c1/entities/e1/posts', editable: true },
    () => response(200, [{ id: 'p1', name: 'Post One', isPrivate: false }]));
  assert.match(el.innerHTML, /Post One/);
});
