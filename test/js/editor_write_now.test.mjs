// editor_write_now.test.mjs — pins what happens to an address that means
// "write this now" (?write=1, where Create page and Home's "Write it up"
// send people). The editor opens straight into editing once, then drops the
// mark from the address, so a reload or a copied link opens the page at rest
// like any other.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor.js'), 'utf8');

// Runs the editor's own forgetWriteNow against an address and returns the
// address the tab shows afterwards, and what was handed to the history.
function forget(href, state = { htmx: true }) {
  const start = src.indexOf('function forgetWriteNow()');
  const end = src.indexOf('\n  }\n', start) + 4;
  assert.ok(start > 0 && end > start, 'forgetWriteNow not found');
  const calls = [];
  const window = {
    location: { href },
    history: { state, replaceState: (s, title, url) => calls.push({ state: s, url }) },
  };
  new Function('window', 'URL', src.slice(start, end) + '\nforgetWriteNow();')(window, URL);
  return calls;
}

test('the mark leaves the address once the editor has opened', () => {
  const calls = forget('https://chronicle.test/campaigns/c1/entities/e1?write=1');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, '/campaigns/c1/entities/e1');
});

test('the rest of the address stays: other marks, and the part of the page it points at', () => {
  assert.equal(forget('https://chronicle.test/campaigns/c1/sessions/s2?write=1#session-recap')[0].url, '/campaigns/c1/sessions/s2#session-recap');
  assert.equal(forget('https://chronicle.test/campaigns/c1/entities/e1?era=3&write=1&hl=1')[0].url, '/campaigns/c1/entities/e1?era=3&hl=1');
});

test('the history entry is replaced, not added, and keeps what it held', () => {
  const held = { htmx: true };
  const calls = forget('https://chronicle.test/campaigns/c1/entities/e1?write=1', held);
  assert.equal(calls[0].state, held, 'Back still leaves the page in one press');
});

test('an address without the mark is left alone', () => {
  assert.equal(forget('https://chronicle.test/campaigns/c1/entities/e1').length, 0);
  assert.equal(forget('https://chronicle.test/campaigns/c1/entities/e1?write=0').length, 0);
  assert.equal(forget('https://chronicle.test/campaigns/c1/entities/e1?rewrite=1').length, 0);
});

test('the mark is dropped right after the editor opens for it, and only then', () => {
  assert.match(src, /if \(state\.startEditing\) \{\s*state\.startEditing = false;\s*enterEditMode\(state\);\s*forgetWriteNow\(\);\s*\}/);
  assert.equal(src.match(/forgetWriteNow\(\)/g).length, 2, 'one definition and one call');
});

test('the page hands the mark to its text only for someone who may write it', () => {
  const show = readFileSync(join(root, 'internal/plugins/entities/show.templ'), 'utf8');
  assert.match(show, /if startsWriting\(ctx\) && canWriteDescription\(ctx, cc, entity\) \{\s*data-start-editing="true"\s*\}/);
});
