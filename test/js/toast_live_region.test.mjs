// toast_live_region.test.mjs — pins that toasts reach screen readers: the
// stack is a polite live region, an error interrupts, and the close button
// has a name.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/notifications.js'), 'utf8');

test('the toast stack is announced', () => {
  assert.match(src, /container\.setAttribute\('aria-live', 'polite'\)/);
  assert.match(src, /if \(type === 'error'\) toast\.setAttribute\('role', 'alert'\)/);
  assert.match(src, /close\.setAttribute\('aria-label', 'Dismiss'\)/);
});
