// sidebar_nav_folds.test.mjs — contract for the fold cookie sidebar_nav.js
// writes and the server reads back (layouts.ParseNavFolds) to paint a
// viewer's folds on page load. Both sides must agree on the "id:1|id:0" form
// and on what they refuse: the cookie is client-written.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'sidebar_nav.js'), 'utf8');

/** Loads sidebar_nav.js on a page with no campaign sidebar and returns its
 *  Chronicle.nav API. */
function load() {
  const noop = () => {};
  const document = {
    readyState: 'complete',
    cookie: '',
    body: { classList: { contains: () => false } },
    documentElement: { classList: { contains: () => false } },
    getElementById: () => null,
    addEventListener: noop,
  };
  const window = {
    matchMedia: () => ({ matches: false, addEventListener: noop }),
  };
  const sandbox = { window, document, performance: { now: () => 0 } };
  vm.createContext(sandbox);
  vm.runInContext(src, sandbox);
  return window.Chronicle.nav;
}

test('parseFolds reads sections and sub-category folds', () => {
  const nav = load();
  assert.deepEqual({ ...nav.parseFolds('apps:0|manage:1|sub-12:1') }, { apps: false, manage: true, 'sub-12': true });
});

test('parseFolds skips anything malformed, as the server does', () => {
  const nav = load();
  assert.deepEqual({ ...nav.parseFolds('apps:2|:1|bad id:1|x|sec_a:0') }, { sec_a: false });
  assert.deepEqual({ ...nav.parseFolds('<b>:1|apps:0') }, { apps: false });
  assert.deepEqual({ ...nav.parseFolds('a'.repeat(2049)) }, {});
  assert.deepEqual({ ...nav.parseFolds('') }, {});
});

test('parseFolds reads at most 64 parts', () => {
  const nav = load();
  const raw = Array.from({ length: 70 }, (_, i) => 'sub-' + i + ':1').join('|');
  assert.equal(Object.keys(nav.parseFolds(raw)).length, 64);
});

test('serializeFolds writes a stable form that parses back', () => {
  const nav = load();
  const folds = { manage: true, apps: false, 'sub-3': true };
  const raw = nav.serializeFolds(folds);
  assert.equal(raw, 'apps:0|manage:1|sub-3:1');
  assert.deepEqual({ ...nav.parseFolds(raw) }, folds);
});

test('serializeFolds never writes an id the server would refuse', () => {
  const nav = load();
  assert.equal(nav.serializeFolds({ 'bad id': true, 'x;y': false, apps: true }), 'apps:1');
});
