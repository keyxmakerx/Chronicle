// page_header.test.mjs — the system-page header's module loads on its own and
// exposes every call the server-rendered header makes (each control's inline
// handler names one).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function load() {
  const listeners = {};
  const window = { Chronicle: {} };
  const document = {
    querySelector: () => null,
    addEventListener: (type, fn) => { (listeners[type] = listeners[type] || []).push(fn); },
    removeEventListener: () => {},
  };
  const ctx = { window, document, matchMedia: () => ({ matches: true }), CSS: { escape: (s) => s }, CustomEvent: class {} };
  for (const f of ['page_name_fold.js', 'page_header.js']) {
    vm.runInNewContext(readFileSync(join(root, 'static/js/widgets', f), 'utf8'), ctx);
  }
  return { H: window.Chronicle.PageHeader, listeners };
}

test('every call the header controls make exists', () => {
  const { H } = load();
  for (const m of ['toggleMore', 'closeMore', 'nameFoldFromMenu', 'askDelete', 'keepDelete', 'claim']) {
    assert.equal(typeof H[m], 'function', m);
  }
});

test('chronicle:change-image opens the upload for that entity only, and is bound once', () => {
  const clicked = [];
  const mounts = ['e1', 'e2'].map((id) => ({ getAttribute: () => id, click: () => clicked.push(id) }));
  const listeners = [];
  const window = { Chronicle: {} };
  const document = {
    querySelector: () => null,
    querySelectorAll: () => mounts,
    addEventListener: (type, fn) => listeners.push([type, fn]),
    removeEventListener: () => {},
  };
  const ctx = { window, document, matchMedia: () => ({ matches: true }) };
  const src = readFileSync(join(root, 'static/js/widgets/page_header.js'), 'utf8');
  vm.runInNewContext(src, ctx);
  vm.runInNewContext(src, ctx); // loaded twice (a swapped page): still one listener
  const bound = listeners.filter(([t]) => t === 'chronicle:change-image');
  assert.equal(bound.length, 1);
  bound[0][1]({ detail: { entityId: 'e2' } });
  bound[0][1]({ detail: { entityId: 'nobody' } });
  bound[0][1]({ detail: {} });
  assert.deepEqual(clicked, ['e2']);
});
