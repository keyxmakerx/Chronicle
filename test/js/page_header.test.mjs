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
